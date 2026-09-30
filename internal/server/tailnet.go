package server

import (
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/session"
)

// loginPath is where the login form posts. It exists only under the
// tailnet host: the guard answers it there and never registers it on the
// mux, so over loopback /login is an ordinary client-side route serving
// the UI bundle, exactly as it did before this daemon could be reached
// from anywhere else.
const loginPath = "/login"

// sessionCookie is the name of the cookie a browser presents instead of
// the token. The __Host- prefix is not decoration: it makes the browser
// itself refuse the cookie unless it is Secure, Path=/ and carries no
// Domain, so the cookie is bound to the one host that set it and cannot
// be widened to a sibling name later, by this daemon or anything else.
const sessionCookie = "__Host-mdn_session"

// maxLoginForm bounds the body of a login post. A token and a redirect
// path are a few hundred bytes; this is room to spare.
const maxLoginForm = 8 << 10

// WithTailnetHost gives the daemon one extra Host name to answer to, for
// requests a `tailscale serve` proxy forwards to the loopback listener.
// Empty, the default, leaves the daemon loopback-only.
func WithTailnetHost(host string) Option {
	return func(s *Server) { s.tailnetHost = strings.ToLower(strings.TrimSpace(host)) }
}

// guardTailnet applies the rule for requests arriving under the extra
// Host name. It reports whether the request may go on to the mux.
//
// Loopback is a single-user machine, where anything that can reach the
// port already runs as the user. The tailnet name is not: what reaches it
// is whatever the tailnet ACL admits, so nothing here is served without
// proof that the caller holds the token — the header for an API client,
// the session cookie for a browser that presented the token once.
func (s *Server) guardTailnet(w http.ResponseWriter, r *http.Request) bool {
	if path.Clean("/"+r.URL.Path) == loginPath {
		s.loginHandler(w, r)
		return false
	}
	presented, carried := bearer(r)
	// A session a day old or more is reissued on the response, but only
	// once the request is known to be served: one the Origin check or the
	// allow-list refuses gets its refusal and keeps the cookie it had.
	var reissue string
	switch {
	case carried && s.validToken(presented):
		// An API client. The Origin exemption the token buys on loopback
		// carries over: a page cannot set an Authorization header
		// cross-origin without a preflight, and the daemon still answers
		// none.
	case carried:
		s.logRefusal(r, "refused (401 unauthorized)", "bearer token is not the current token")
		writeUnauthorized(w)
		return false
	default:
		ok, why := s.sessionOf(r, &reissue)
		if !ok {
			s.challenge(w, r, why)
			return false
		}
		// A browser. Here the Origin check does apply: SameSite=Lax
		// already withholds the cookie from a cross-site write, but one
		// client-side attribute should not be the only thing between a
		// foreign page and a write to the notes.
		if origin := r.Header.Get("Origin"); origin != "" && !s.isTailnetOrigin(origin) {
			s.logRefusal(r, "refused (403 cross_origin)", fmt.Sprintf("Origin %s is not https://%s", quoted(origin, 100), s.tailnetHost))
			writeGuardError(w, http.StatusForbidden, "cross_origin",
				"cross-origin request refused; present the bearer token to write from another origin")
			return false
		}
	}
	if !remoteAllowed(r) {
		s.logRefusal(r, "refused (403 loopback_only)", "served on loopback only")
		writeGuardError(w, http.StatusForbidden, "loopback_only",
			"this endpoint is served on loopback only; it is not reachable under "+s.tailnetHost)
		return false
	}
	if reissue != "" {
		setSessionCookie(w, reissue)
	}
	return true
}

// isTailnetOrigin reports whether origin is the daemon's own origin on the
// tailnet name. `tailscale serve` terminates TLS, so that origin is https
// and only https; the http form would mean something else is proxying.
func (s *Server) isTailnetOrigin(origin string) bool {
	o := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(origin), "/"))
	return s.tailnetHost != "" && o == "https://"+s.tailnetHost
}

// sessionOf reports whether the request carries a live session cookie
// and, when it does not, why not, in words fit for the log. The cookie is
// checked against a key derived from the token in force now, so a rotation
// ends it on this very request, and against nothing else, so a restart
// does not (davison/md-notes#231). When the session is a day old or more,
// *reissue is set to a fresh one for the response to carry, so a session
// in use never reaches its idle limit.
func (s *Server) sessionOf(r *http.Request, reissue *string) (bool, string) {
	if s.token == nil || s.tailnetHost == "" {
		return false, "no token configured"
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false, "no session cookie"
	}
	key := s.token.Derive(session.KeyPurpose)
	now := s.now()
	due, err := session.Verify(key, s.tailnetHost, c.Value, now)
	switch {
	case err == nil:
		if due {
			*reissue = session.Issue(key, s.tailnetHost, now)
		}
		return true, ""
	case errors.Is(err, session.ErrSignature):
		// Checked again under the token this daemon held before its last
		// rotation, only to say so: the answer is a refusal either way.
		if prev := s.token.DerivePrevious(session.KeyPurpose); prev != nil {
			if _, perr := session.Verify(prev, s.tailnetHost, c.Value, now); perr == nil || !errors.Is(perr, session.ErrSignature) {
				return false, "signed with the token before the last rotation"
			}
		}
		return false, err.Error() + " (another token, a token replaced while the daemon was stopped, another tailnet_host, or altered)"
	default:
		return false, err.Error()
	}
}

// logRefusal writes one line about a tailnet request that was refused or
// sent to the login page: who, what, the answer and why. The path is
// logged without its query, and nothing the caller presented as a
// credential is — the reason is always one of the daemon's own phrases,
// and anything in it that came from the caller went through logSafe.
// Bounded by refusalLog.
func (s *Server) logRefusal(r *http.Request, answer, why string) {
	addr := clientAddr(r)
	ok, leftOut, from := s.refusals.allow(addr+"\x00"+answer+"\x00"+why, s.now())
	if leftOut > 0 {
		s.log.Printf("tailnet: %d tailnet refusals not logged in the minute from %s", leftOut, from.Format("15:04:05"))
	}
	if !ok {
		return
	}
	s.log.Printf("tailnet: %s %s %s %s: %s", logSafe(addr, 64), logSafe(r.Method, 16), logSafe(path.Clean("/"+r.URL.Path), 200), answer, why)
}

// logSafe makes a string the caller chose fit for one line of the log:
// cut to n runes, on a rune boundary, and then every character that is not
// printable — a line break, a carriage return, an escape that would start
// a terminal sequence, U+0085, U+2028 — written as its Go escape, and a
// backslash or quote escaped too, so the escapes cannot be imitated. A
// request path is percent-decoded before the daemon sees it, so without
// this a caller could write lines of its own into the log (review finding
// 1 on PR #241).
func logSafe(v string, n int) string {
	cut := ""
	if utf8.RuneCountInString(v) > n {
		runes := []rune(v)
		v, cut = string(runes[:n]), "…"
	}
	q := strconv.Quote(v)
	return q[1:len(q)-1] + cut
}

// quoted is logSafe inside quotes, for a value that is not a path.
func quoted(v string, n int) string { return `"` + logSafe(v, n) + `"` }

// setSessionCookie is the one place the cookie's attributes are written,
// for a login and for a reissue alike. Max-Age is the idle limit, so the
// browser forgets the cookie when the daemon would refuse it.
func setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   int(session.Idle.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// challenge answers a request under the tailnet name that proved nothing.
// A person typing the daemon's name into a browser gets the login page;
// everything else — a fetch, the events stream, any call under /api/ —
// gets the 401 an API client can act on, because a login page arriving
// where JSON was expected is not an improvement on an error.
func (s *Server) challenge(w http.ResponseWriter, r *http.Request, why string) {
	if !isNavigation(r) {
		s.logRefusal(r, "refused (401 unauthorized)", why)
		writeUnauthorized(w)
		return
	}
	s.logRefusal(r, "sent to the login page", why)
	w.Header().Set("WWW-Authenticate", `Bearer realm="mdn"`)
	s.writeLoginPage(w, r, http.StatusUnauthorized, loginForm{Redirect: redirectTarget(r)})
}

// isNavigation reports whether the request is a browser loading a page,
// as opposed to a subresource, a fetch or an API call. Sec-Fetch-Mode is
// the browser saying so itself; the Accept header is the fallback for a
// client too old to send it.
func isNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if strings.HasPrefix(path.Clean("/"+r.URL.Path), "/api/") {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return strings.EqualFold(mode, "navigate")
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// loginHandler serves the login page and takes the form it posts. It runs
// before any authentication check, since its whole purpose is to be
// reachable without one.
func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.writeLoginPage(w, r, http.StatusOK, loginForm{Redirect: sanitizeRedirect(r.URL.Query().Get("redirect"))})
		return
	case http.MethodPost:
	default:
		s.logRefusal(r, "refused (405 method_not_allowed)", "the login form is posted")
		writeGuardError(w, http.StatusMethodNotAllowed, "method_not_allowed", "the login form is posted")
		return
	}
	// A cross-site form post must not be able to log this browser in as
	// somebody else's session; browsers always send Origin on a POST.
	if origin := r.Header.Get("Origin"); origin != "" && !s.isTailnetOrigin(origin) {
		s.logRefusal(r, "refused (403 cross_origin)", fmt.Sprintf("Origin %s is not https://%s", quoted(origin, 100), s.tailnetHost))
		writeGuardError(w, http.StatusForbidden, "cross_origin", "cross-origin login refused")
		return
	}
	// The login form is the one thing under this name that runs before
	// anything has been proved, so it reads a form and not a megabyte:
	// ParseForm's own default is 10 MiB.
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginForm)
	if err := r.ParseForm(); err != nil {
		s.logRefusal(r, "refused (400)", "the login form could not be read")
		s.writeLoginPage(w, r, http.StatusBadRequest, loginForm{Error: "That form could not be read. Try again."})
		return
	}
	redirect := sanitizeRedirect(r.PostFormValue("redirect"))
	// The cookie is Secure, so a browser that reached the daemon over
	// plain http would discard it and come straight back to this page.
	// Saying so is better than the login loop that would otherwise be the
	// only symptom.
	if !forwardedHTTPS(r) {
		s.logRefusal(r, "refused (400)", "not over https: no X-Forwarded-Proto: https from the proxy, so the Secure cookie would be discarded")
		s.writeLoginPage(w, r, http.StatusBadRequest, loginForm{
			Redirect: redirect,
			Error:    "This request did not arrive over https, so the session cookie would be discarded. Reach the daemon through `tailscale serve`, which terminates TLS.",
		})
		return
	}
	// One read for both answers: deriving the key separately would let
	// a rotation landing in between sign this session with the key of a
	// token the browser never presented, which is the one case "a
	// rotation ends every session" has to cover.
	var key []byte
	var ok bool
	if s.token != nil {
		key, ok = s.token.AuthenticateDerive(strings.TrimSpace(r.PostFormValue("token")), session.KeyPurpose)
	}
	addr := clientAddr(r)
	if !ok {
		wait, refuse := s.logins.failed(addr)
		if refuse {
			s.logRefusal(r, "refused (429 too_many_attempts)", "too many failed logins")
			w.Header().Set("Retry-After", strconv.Itoa(int(s.logins.window.Seconds())))
			writeGuardError(w, http.StatusTooManyRequests, "too_many_attempts",
				"too many failed logins; wait a minute and try again")
			return
		}
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-r.Context().Done():
				return
			}
		}
		s.log.Printf("tailnet login refused from %s: not the current token", logSafe(addr, 64))
		s.writeLoginPage(w, r, http.StatusUnauthorized, loginForm{
			Redirect: redirect,
			Error:    "That is not the current token. `mdn token` prints it on the machine running the daemon.",
		})
		return
	}
	s.logins.succeeded(addr)
	// A cookie the browser held before is simply replaced: it is a signed
	// value with nothing behind it to delete, and the browser keeps only
	// the newest.
	setSessionCookie(w, session.Issue(key, s.tailnetHost, s.now()))
	s.log.Printf("tailnet login from %s (%s)", logSafe(addr, 64), browserFamily(r.UserAgent()))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// forwardedHTTPS reports whether the request reached the proxy over TLS.
// `tailscale serve` terminates TLS on the tailnet name and says so in
// X-Forwarded-Proto. The header is trusted only because the listener is
// loopback: the only things that can set it are the proxy and a process
// already running as the user.
func forwardedHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// clientAddr names who is logging in, for the daemon's log. `tailscale
// serve` puts the tailnet address of the calling node in X-Forwarded-For;
// without it the peer is the proxy itself, on loopback.
func clientAddr(r *http.Request) string {
	if fwd, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ","); strings.TrimSpace(fwd) != "" {
		return strings.TrimSpace(fwd)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// redirectTarget is the path a login page should return the browser to:
// what it asked for in the first place.
func redirectTarget(r *http.Request) string {
	return sanitizeRedirect(r.URL.RequestURI())
}

// sanitizeRedirect confines a redirect to this daemon. Anything carrying a
// scheme, a host, or a leading "//" — which a browser reads as a host —
// becomes the home page rather than an open redirect back out onto the
// tailnet.
func sanitizeRedirect(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") ||
		strings.HasPrefix(raw, "/\\") || strings.ContainsAny(raw, "\r\n") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.User != nil {
		return "/"
	}
	cleaned := path.Clean(u.EscapedPath())
	if cleaned == "." || !strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "//") {
		return "/"
	}
	if cleaned == loginPath {
		return "/"
	}
	out := cleaned
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

// remoteAllowed reports whether the endpoint may be reached under the
// tailnet name at all. It is an allow-list, not a list of exclusions: the
// credential over the network reaches the UI's own API — reads, source
// saves, creating and deleting a note, clipping a page, the events
// stream, search and tags — and an endpoint nobody has considered in this
// light is loopback-only until somebody does.
func remoteAllowed(r *http.Request) bool {
	read := r.Method == http.MethodGet || r.Method == http.MethodHead
	p := path.Clean("/" + r.URL.Path)
	rest, isAPI := strings.CutPrefix(p, "/api/")
	if !isAPI {
		// The UI bundle, and every client-side route that falls back to
		// its index.html.
		return read
	}
	switch {
	case rest == "roots":
		// Listing the roots, yes. Registering one, no: that is the path
		// from this credential to any directory on the machine. This case
		// is the path `/api/roots` exactly; unregistering a root is
		// `/api/roots/{slug}`, which no case names and the default below
		// refuses, so M7-R2's DELETE is loopback-only without anything
		// being added here.
		return read
	case rest == "clip":
		// A clip writes one file into `clips_dir` inside the notes root,
		// which is narrower than the note the source create below already
		// admits: the path is the daemon's to choose, not the caller's.
		// M6-R1; #39 refused this when no write at all crossed the name.
		return r.Method == http.MethodPost
	case strings.HasPrefix(rest, "r/"):
		_, sub, ok := strings.Cut(strings.TrimPrefix(rest, "r/"), "/")
		if !ok {
			return false
		}
		// Writing a note is writing a note, whether it replaces, creates
		// or removes one: the credential over the network reaches the
		// same files the source save already reaches, and no more. Every
		// other write under a root stays on the machine.
		// An image upload is narrower still: the daemon picks the
		// directory and the name, and only a checked image is written.
		// M11-R2 puts it behind the same checks as every other write, and
		// a paste on the phone is the same paste as on the desktop.
		if r.Method == http.MethodPost && sub == "resources" {
			return true
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPost || r.Method == http.MethodDelete {
			return sub == "source" || strings.HasPrefix(sub, "source/")
		}
		return read
	default:
		return false
	}
}

type loginForm struct {
	Redirect string
	Error    string
}

// writeLoginPage renders the login form. It is one self-contained
// document with no script and no external asset, so it works before the
// browser has been allowed to load anything else from the daemon.
func (s *Server) writeLoginPage(w http.ResponseWriter, r *http.Request, status int, form loginForm) {
	if form.Redirect == "" {
		form.Redirect = "/"
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	// base-uri has no fallback to default-src, so it has to be named.
	h.Set("Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	// same-origin, not no-referrer: a document with the no-referrer
	// policy makes the browser send `Origin: null` on the form it posts
	// (Fetch, "append the Origin header"), which is indistinguishable
	// from the cross-site post the check below exists to refuse. This
	// policy keeps the real origin on our own POST and still nulls it on
	// anyone else's.
	h.Set("Referrer-Policy", "same-origin")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	loginPage.Execute(w, struct {
		loginForm
		Host string
	}{form, s.tailnetHost})
}

// loginPage is parsed once at startup; a template that fails to parse is
// a programming error, not a runtime condition.
var loginPage = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>md-notes</title>
<style>
:root { --bg:#fbfbfa; --fg:#1f1f1f; --muted:#6b6b6b; --line:#e2e2df; --accent:#2a6db0; --pane:#fff; --error:#b3261e; --scroll-thumb:#7f7f7c; --scroll-track:#efefed; color-scheme: light dark; }
@media (prefers-color-scheme: dark) { :root { --bg:#1b1b1b; --fg:#e6e6e3; --muted:#9a9a96; --line:#333331; --accent:#7fb0e6; --pane:#202020; --error:#f2b8b5; --scroll-thumb:#7a7a78; --scroll-track:#2a2a29; } }
/* The same thin, palette-coloured bars the application draws (M8-R10): this
   page has its own copy of the palette, so it needs its own copy of these.
   On the universal selector for the reason ui/src/style.css gives — Chromium
   inherits scrollbar-color and does not inherit scrollbar-width. */
* { box-sizing: border-box; scrollbar-width: thin; scrollbar-color: var(--scroll-thumb) var(--scroll-track); }
body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center; background:var(--bg); color:var(--fg); font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif; }
main { width:100%; max-width:22rem; padding:1.5rem; margin:1rem; background:var(--pane); border:1px solid var(--line); border-radius:8px; }
h1 { margin:0 0 .25rem; font-size:1.1rem; }
p { margin:0 0 1rem; color:var(--muted); font-size:.9rem; word-break:break-word; }
label { display:block; margin-bottom:.35rem; font-size:.9rem; }
input { width:100%; padding:.55rem .6rem; font:inherit; color:var(--fg); background:var(--bg); border:1px solid var(--line); border-radius:6px; }
input:focus { outline:2px solid var(--accent); outline-offset:1px; }
button { width:100%; margin-top:.9rem; padding:.55rem; font:inherit; color:var(--pane); background:var(--accent); border:0; border-radius:6px; cursor:pointer; }
.error { margin:0 0 1rem; padding:.55rem .6rem; color:var(--error); border:1px solid var(--error); border-radius:6px; font-size:.9rem; }
code { font-family:ui-monospace,"JetBrains Mono",Menlo,monospace; font-size:.9em; }
</style>
<main>
<h1>md-notes</h1>
<p>{{if .Host}}{{.Host}} asks for{{else}}This daemon asks for{{end}} the daemon&rsquo;s token. Run <code>mdn token</code> on the machine serving your notes.</p>
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
<form method="post" action="/login">
<input type="hidden" name="redirect" value="{{.Redirect}}">
<label for="token">Token</label>
<input id="token" name="token" type="password" autocomplete="current-password" autocapitalize="off" autocorrect="off" spellcheck="false" autofocus required>
<button type="submit">Sign in</button>
</form>
</main>
</html>
`))
