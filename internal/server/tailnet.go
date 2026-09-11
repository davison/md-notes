package server

import (
	"html/template"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
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
	switch {
	case carried && s.validToken(presented):
		// An API client. The Origin exemption the token buys on loopback
		// carries over: a page cannot set an Authorization header
		// cross-origin without a preflight, and the daemon still answers
		// none.
	case carried:
		writeUnauthorized(w)
		return false
	case s.validSession(r):
		// A browser. Here the Origin check does apply — SameSite=Strict
		// should not be the only thing between a foreign page and a
		// write to the notes.
		if origin := r.Header.Get("Origin"); origin != "" && !s.isTailnetOrigin(origin) {
			writeGuardError(w, http.StatusForbidden, "cross_origin",
				"cross-origin request refused; present the bearer token to write from another origin")
			return false
		}
	default:
		s.challenge(w, r)
		return false
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

// validSession reports whether the request carries a live session cookie.
// The session is tied to the token generation in force now, so a rotation
// ends it on this very request.
func (s *Server) validSession(r *http.Request) bool {
	if s.sessions == nil || s.token == nil {
		return false
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return s.sessions.Valid(c.Value, s.token.Generation())
}

// challenge answers a request under the tailnet name that proved nothing.
// A person typing the daemon's name into a browser gets the login page;
// everything else — a fetch, the events stream, any call under /api/ —
// gets the 401 an API client can act on, because a login page arriving
// where JSON was expected is not an improvement on an error.
func (s *Server) challenge(w http.ResponseWriter, r *http.Request) {
	if !isNavigation(r) {
		writeUnauthorized(w)
		return
	}
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
		writeSourceError(w, http.StatusMethodNotAllowed, "method_not_allowed", "the login form is posted")
		return
	}
	// A cross-site form post must not be able to log this browser in as
	// somebody else's session; browsers always send Origin on a POST.
	if origin := r.Header.Get("Origin"); origin != "" && !s.isTailnetOrigin(origin) {
		writeGuardError(w, http.StatusForbidden, "cross_origin", "cross-origin login refused")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.writeLoginPage(w, r, http.StatusBadRequest, loginForm{Error: "That form could not be read. Try again."})
		return
	}
	redirect := sanitizeRedirect(r.PostFormValue("redirect"))
	// The cookie is Secure, so a browser that reached the daemon over
	// plain http would discard it and come straight back to this page.
	// Saying so is better than the login loop that would otherwise be the
	// only symptom.
	if !forwardedHTTPS(r) {
		s.writeLoginPage(w, r, http.StatusBadRequest, loginForm{
			Redirect: redirect,
			Error:    "This request did not arrive over https, so the session cookie would be discarded. Reach the daemon through `tailscale serve`, which terminates TLS.",
		})
		return
	}
	if s.token == nil || !s.validToken(strings.TrimSpace(r.PostFormValue("token"))) {
		s.log.Printf("tailnet login refused from %s", clientAddr(r))
		s.writeLoginPage(w, r, http.StatusUnauthorized, loginForm{
			Redirect: redirect,
			Error:    "That is not the current token. `mdn token` prints it on the machine running the daemon.",
		})
		return
	}
	id := s.sessions.Create(s.token.Generation())
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   int(s.sessions.TTL().Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	s.log.Printf("tailnet login from %s", clientAddr(r))
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
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	h.Set("Referrer-Policy", "no-referrer")
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
:root { --bg:#fbfbfa; --fg:#1f1f1f; --muted:#6b6b6b; --line:#e2e2df; --accent:#2a6db0; --pane:#fff; --error:#b3261e; color-scheme: light dark; }
@media (prefers-color-scheme: dark) { :root { --bg:#1b1b1b; --fg:#e6e6e3; --muted:#9a9a96; --line:#333331; --accent:#7fb0e6; --pane:#202020; --error:#f2b8b5; } }
* { box-sizing: border-box; }
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
