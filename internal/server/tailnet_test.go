package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davison/md-notes/internal/session"
	"github.com/davison/md-notes/internal/source"
	"github.com/davison/md-notes/internal/token"
)

// tailnetName is the extra Host name `tailscale serve` would forward.
const tailnetName = "laptop.example.ts.net"

// tailnetOrigin is the daemon's own origin on that name. The proxy
// terminates TLS, so it is https and the daemon never sees the handshake.
const tailnetOrigin = "https://" + tailnetName

// noRedirect is the browser-less client these tests use: a 303 from the
// login form has to be inspected, not followed — following it would send
// the httptest server's own Host and be refused.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

func newTailnetServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	return newTestServerWith(t, WithTailnetHost(tailnetName))
}

// tdo sends a request the way `tailscale serve` forwards one: the tailnet
// Host, and X-Forwarded-Proto saying TLS was terminated in front.
func tdo(t *testing.T, ts *httptest.Server, method, p, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+p, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = tailnetName
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "100.64.0.9")
	for k, v := range hdr {
		switch k {
		case "Host":
			req.Host = v
		case "":
			req.Header.Del(v)
		default:
			req.Header.Set(k, v)
		}
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// navigation is what a browser sends when a person types the name in.
func navigation(extra ...string) map[string]string {
	h := map[string]string{
		"Sec-Fetch-Mode": "navigate",
		"Accept":         "text/html,application/xhtml+xml",
	}
	for i := 0; i+1 < len(extra); i += 2 {
		h[extra[i]] = extra[i+1]
	}
	return h
}

func cookieHeader(t *testing.T, resp *http.Response) string {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatalf("no %s cookie on the response", sessionCookie)
	return ""
}

// login exchanges the token for a session the way the login page does,
// and returns the Cookie header a browser would then send.
func login(t *testing.T, ts *httptest.Server, base string) string {
	t.Helper()
	resp := loginPost(t, ts, daemonToken(t, base), "/")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	return cookieHeader(t, resp)
}

func loginPost(t *testing.T, ts *httptest.Server, tok, redirect string, extra ...string) *http.Response {
	t.Helper()
	form := "token=" + tok + "&redirect=" + redirect
	hdr := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Origin":       tailnetOrigin,
	}
	for i := 0; i+1 < len(extra); i += 2 {
		hdr[extra[i]] = extra[i+1]
	}
	return tdo(t, ts, "POST", loginPath, form, hdr)
}

func guardCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct{ Code, Error string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("body is not the {code, error} envelope: %v", err)
	}
	if body.Error == "" {
		t.Errorf("code %q carries no message", body.Code)
	}
	return body.Code
}

// The extra name is accepted where it is configured, and nowhere else —
// the guard's allow-list grows by that one name, as #10 asked.
func TestTailnetHostIsAcceptedOnlyWhenConfigured(t *testing.T) {
	plain, _ := newTestServer(t)
	if resp := tdo(t, plain, "GET", "/api/roots", "", nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("unconfigured daemon: status %d, want 403", resp.StatusCode)
	} else if code := guardCode(t, resp); code != "bad_host" {
		t.Errorf("unconfigured daemon: code %q, want bad_host", code)
	}

	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)
	for host, want := range map[string]int{
		tailnetName:                  http.StatusOK,
		strings.ToUpper(tailnetName): http.StatusOK,
		"laptop.example.ts.net:443":  http.StatusForbidden,
		"other.example.ts.net":       http.StatusForbidden,
		"evil.example":               http.StatusForbidden,
		"laptop.example.ts.net.evil": http.StatusForbidden,
		"x.laptop.example.ts.net":    http.StatusForbidden,
	} {
		resp := tdo(t, ts, "GET", "/api/roots", "", bearerHeader(tok, "Host", host))
		if resp.StatusCode != want {
			t.Errorf("Host %q: status %d, want %d", host, resp.StatusCode, want)
		}
	}
}

// Every request under the extra name must prove it holds the token. A
// person gets the login page; a program gets the 401 it can act on.
func TestTailnetRequiresAuthentication(t *testing.T) {
	ts, _ := newTailnetServer(t)

	page := tdo(t, ts, "GET", "/r/notes/hello.md", "", navigation())
	if page.StatusCode != http.StatusUnauthorized {
		t.Errorf("navigation: status %d, want 401", page.StatusCode)
	}
	if ct := page.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("navigation: Content-Type %q, want the login page", ct)
	}
	body := readAll(t, page.Body)
	for _, want := range []string{`action="/login"`, `name="token"`, `name="redirect" value="/r/notes/hello.md"`} {
		if !strings.Contains(body, want) {
			t.Errorf("login page lacks %q:\n%s", want, body)
		}
	}
	// The page is self-contained: nothing to fetch before you are allowed
	// to fetch anything.
	if strings.Contains(body, "<script") || strings.Contains(body, "src=") {
		t.Errorf("the login page loads something:\n%s", body)
	}
	if len(page.Cookies()) != 0 {
		t.Errorf("the login page set a cookie before anyone logged in: %v", page.Cookies())
	}

	// An API call gets JSON, not a login page it cannot read.
	api := tdo(t, ts, "GET", "/api/roots", "", nil)
	if api.StatusCode != http.StatusUnauthorized {
		t.Fatalf("api: status %d, want 401", api.StatusCode)
	}
	if got := api.Header.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
	}
	if code := guardCode(t, api); code != "unauthorized" {
		t.Errorf("api: code %q, want unauthorized", code)
	}
	// So does a fetch for something outside /api/ — a login page arriving
	// where the UI expected JSON or a script helps nobody.
	sub := tdo(t, ts, "GET", "/assets/app.js", "", map[string]string{"Sec-Fetch-Mode": "no-cors"})
	if sub.StatusCode != http.StatusUnauthorized {
		t.Errorf("subresource: status %d, want 401", sub.StatusCode)
	}
	if ct := sub.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("subresource: Content-Type %q, want JSON", ct)
	}
	// A wrong token is a wrong token, not an invitation to log in.
	bad := tdo(t, ts, "GET", "/", "", bearerHeader("nonsense", "Sec-Fetch-Mode", "navigate"))
	if bad.StatusCode != http.StatusUnauthorized || guardCode(t, bad) != "unauthorized" {
		t.Errorf("wrong token on a navigation: status %d", bad.StatusCode)
	}
}

// The login page must not carry Referrer-Policy: no-referrer. A document
// with that policy makes the browser send `Origin: null` on the form it
// posts — Fetch, "append the Origin header" — which the cross-origin
// check above then refuses, so the login form refuses its own submission
// in a real browser while passing every test that sets Origin by hand.
// Caught in headless Chromium, pinned here.
func TestLoginPageKeepsItsOriginOnTheFormPost(t *testing.T) {
	ts, _ := newTailnetServer(t)
	resp := tdo(t, ts, "GET", loginPath, "", navigation())
	got := resp.Header.Get("Referrer-Policy")
	if got == "no-referrer" {
		t.Fatal("Referrer-Policy: no-referrer nulls the Origin of the form this page posts")
	}
	if got != "same-origin" {
		t.Errorf("Referrer-Policy = %q, want same-origin: it keeps the origin on our own POST and nulls it on anyone else's", got)
	}
}

// The cookie is exactly what M3-R6 asks for: scoped to the host that set
// it, unreadable by script, and sent only on this site's own requests.
func TestTailnetLoginSetsAHostScopedCookie(t *testing.T) {
	ts, base := newTailnetServer(t)
	resp := loginPost(t, ts, daemonToken(t, base), "/r/notes/hello.md")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if got := resp.Header.Get("Location"); got != "/r/notes/hello.md" {
		t.Errorf("Location = %q, want the page that was asked for", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var c *http.Cookie
	for _, got := range resp.Cookies() {
		if got.Name == sessionCookie {
			c = got
		}
	}
	if c == nil {
		t.Fatalf("no session cookie: %v", resp.Header.Values("Set-Cookie"))
	}
	if !strings.HasPrefix(c.Name, "__Host-") {
		t.Errorf("cookie name %q, want the __Host- prefix that binds it to this host", c.Name)
	}
	if c.Value == "" || c.Value == daemonToken(t, base) {
		t.Errorf("cookie value %q must be a session id, never the token itself", c.Value)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Strict, Path=/ and no Domain", c)
	}
	if c.MaxAge <= 0 {
		t.Errorf("cookie Max-Age = %d, want the session lifetime", c.MaxAge)
	}
}

func TestTailnetLoginRefusals(t *testing.T) {
	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)

	wrong := loginPost(t, ts, tok+"x", "/")
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: status %d, want 401", wrong.StatusCode)
	}
	if len(wrong.Cookies()) != 0 {
		t.Errorf("a wrong token was given a session: %v", wrong.Cookies())
	}
	if body := readAll(t, wrong.Body); !strings.Contains(body, "not the current token") {
		t.Errorf("wrong token: page does not say so:\n%s", body)
	}

	// A Secure cookie would be dropped by a browser that reached the
	// daemon over plain http, so say that rather than loop.
	plain := loginPost(t, ts, tok, "/", "X-Forwarded-Proto", "http")
	if plain.StatusCode != http.StatusBadRequest {
		t.Errorf("http login: status %d, want 400", plain.StatusCode)
	}
	if len(plain.Cookies()) != 0 {
		t.Errorf("a cookie was set for a request that did not arrive over https: %v", plain.Cookies())
	}
	if body := readAll(t, plain.Body); !strings.Contains(body, "https") {
		t.Errorf("http login: page does not explain:\n%s", body)
	}

	// Login CSRF: a form posted by another site must not be able to seat
	// this browser in a session somebody else chose.
	foreign := loginPost(t, ts, tok, "/", "Origin", "https://evil.example")
	if foreign.StatusCode != http.StatusForbidden || guardCode(t, foreign) != "cross_origin" {
		t.Errorf("cross-origin login: status %d, want 403 cross_origin", foreign.StatusCode)
	}

	// A form whose document carried no referrer policy at all posts
	// `Origin: null`, which is what a sandboxed or cross-origin-redirected
	// document sends too. It is not this daemon's own origin.
	opaque := loginPost(t, ts, tok, "/", "Origin", "null")
	if opaque.StatusCode != http.StatusForbidden {
		t.Errorf("null origin: status %d, want 403", opaque.StatusCode)
	}

	// The one endpoint that runs before anything is proved reads a form,
	// not a megabyte.
	big := loginPost(t, ts, strings.Repeat("x", 32<<10), "/")
	if big.StatusCode != http.StatusBadRequest {
		t.Errorf("oversize form: status %d, want 400", big.StatusCode)
	}
	if len(big.Cookies()) != 0 {
		t.Errorf("oversize form was given a session: %v", big.Cookies())
	}

	// The form is posted, not fetched with a method of the caller's choice.
	if resp := tdo(t, ts, "DELETE", loginPath, "", nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /login: status %d, want 405", resp.StatusCode)
	} else if code := guardCode(t, resp); code != "method_not_allowed" {
		t.Errorf("DELETE /login: code %q, want method_not_allowed", code)
	}
	// And it is reachable without credentials, which is its whole point.
	if resp := tdo(t, ts, "GET", loginPath, "", navigation()); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /login: status %d, want 200", resp.StatusCode)
	}
}

// The redirect the form carries is a path on this daemon or it is the
// home page: a login form is exactly the place an open redirect would be
// worth having.
func TestTailnetLoginRedirectIsConfined(t *testing.T) {
	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)
	for given, want := range map[string]string{
		"/r/notes/hello.md":          "/r/notes/hello.md",
		"/":                          "/",
		"":                           "/",
		"//evil.example/":            "/",
		"/%5Cevil.example":           "/",
		"https%3A%2F%2Fevil.example": "/",
		"/login":                     "/",
		"/r/notes/../../etc":         "/etc",
	} {
		resp := loginPost(t, ts, tok, given)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("redirect %q: status %d, want 303", given, resp.StatusCode)
			continue
		}
		if got := resp.Header.Get("Location"); got != want {
			t.Errorf("redirect %q became %q, want %q", given, got, want)
		}
	}
	// A query string survives, since the UI's routes use none but a
	// future one might.
	resp := loginPost(t, ts, tok, "/r/notes/hello.md%3Fq%3Done")
	if got := resp.Header.Get("Location"); got != "/r/notes/hello.md?q=one" {
		t.Errorf("Location = %q, want the query kept", got)
	}
}

// The whole UI over the cookie, end to end: the bundle, the navigator's
// data, a read, a save, search and tags.
func TestTailnetSessionServesTheUI(t *testing.T) {
	ts, base := newTailnetServer(t)
	cookie := login(t, ts, base)
	hdr := func(extra ...string) map[string]string {
		h := map[string]string{"Cookie": cookie}
		for i := 0; i+1 < len(extra); i += 2 {
			h[extra[i]] = extra[i+1]
		}
		return h
	}

	index := tdo(t, ts, "GET", "/r/notes/hello.md", "", hdr("Sec-Fetch-Mode", "navigate"))
	if index.StatusCode != http.StatusOK || !strings.Contains(readAll(t, index.Body), "app") {
		t.Fatalf("UI: status %d", index.StatusCode)
	}
	for _, p := range []string{
		"/api/roots",
		"/api/r/notes/tree",
		"/api/r/notes/note/hello.md",
		"/api/r/notes/source/hello.md",
		"/api/r/notes/raw/sub/pic.png",
		"/api/r/notes/search?q=hi",
		"/api/r/notes/tags",
		"/assets/app.js",
	} {
		if resp := tdo(t, ts, "GET", p, "", hdr()); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s over the session: status %d", p, resp.StatusCode)
		}
	}

	// A save, which is the one write the UI makes.
	read := tdo(t, ts, "GET", sourceURL, "", hdr())
	var note source.Note
	if err := json.NewDecoder(read.Body).Decode(&note); err != nil {
		t.Fatal(err)
	}
	saved := tdo(t, ts, "PUT", sourceURL, sourceBody(t, "# hi\n\nedited on the tailnet\n", note.Revision),
		hdr("Content-Type", "application/json", "Origin", tailnetOrigin))
	if saved.StatusCode != http.StatusOK {
		t.Fatalf("save: status %d: %s", saved.StatusCode, readAll(t, saved.Body))
	}
	data, err := os.ReadFile(filepath.Join(base, "notes", "hello.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "edited on the tailnet") {
		t.Errorf("the save did not land: %q", data)
	}
}

// Live update has to work over the proxy, or the remote UI is a stale
// snapshot. The cookie rides the EventSource request like any other.
func TestTailnetSessionStreamsEvents(t *testing.T) {
	ts, base := newTailnetServer(t)
	cookie := login(t, ts, base)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/notes/events", nil)
	req.Host = tailnetName
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Cookie", cookie)
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != 200 || ct != "text/event-stream" {
		t.Fatalf("status %d, content-type %q", resp.StatusCode, ct)
	}
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })
	os.WriteFile(filepath.Join(base, "notes", "over-the-tailnet.md"), []byte("# new"), 0o644)
	next(func(l string) bool { return strings.Contains(l, `"over-the-tailnet.md"`) })

	// Without the cookie the stream is refused rather than opened and
	// left hanging.
	if plain := tdo(t, ts, "GET", "/api/r/notes/events", "", nil); plain.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated stream: status %d, want 401", plain.StatusCode)
	}
}

// An API client presents the header instead, exactly as the extension
// does on loopback.
func TestTailnetAcceptsTheBearerToken(t *testing.T) {
	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)
	for _, c := range []struct {
		name string
		hdr  map[string]string
		want int
	}{
		{"the token", bearerHeader(tok), http.StatusOK},
		{"the token from anywhere", bearerHeader(tok, "Origin", "https://evil.example"), http.StatusOK},
		{"a wrong token", bearerHeader(tok + "x"), http.StatusUnauthorized},
		{"another scheme", map[string]string{"Authorization": "Basic " + tok}, http.StatusUnauthorized},
	} {
		if resp := tdo(t, ts, "GET", "/api/roots", "", c.hdr); resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}
}

// SameSite=Strict should not be the only thing between a foreign page and
// a write, so a cookie-authenticated request gets the Origin check too.
func TestTailnetCookieIsCheckedAgainstTheOrigin(t *testing.T) {
	ts, base := newTailnetServer(t)
	cookie := login(t, ts, base)
	for origin, want := range map[string]int{
		"":                                   http.StatusOK,
		tailnetOrigin:                        http.StatusOK,
		"http://" + tailnetName:              http.StatusForbidden,
		"https://evil.example":               http.StatusForbidden,
		"null":                               http.StatusForbidden,
		"https://laptop.example.ts.net.evil": http.StatusForbidden,
	} {
		hdr := map[string]string{"Cookie": cookie}
		if origin != "" {
			hdr["Origin"] = origin
		}
		resp := tdo(t, ts, "GET", "/api/roots", "", hdr)
		if resp.StatusCode != want {
			t.Errorf("Origin %q: status %d, want %d", origin, resp.StatusCode, want)
		}
	}
	// And no CORS header is sent under this name either, so the boundary
	// PR #42 drew still holds: the preflight is refused.
	pre := tdo(t, ts, "OPTIONS", "/api/roots", "", map[string]string{
		"Origin":                        "https://evil.example",
		"Access-Control-Request-Method": "GET",
	})
	if pre.StatusCode != http.StatusUnauthorized {
		t.Errorf("preflight: status %d, want 401", pre.StatusCode)
	}
	if got := pre.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want no CORS headers at all", got)
	}
}

// The session is worth no more than the token behind it: `mdn token
// --rotate` logs every browser out, with no restart and nothing to clean.
func TestTailnetRotationEndsTheSession(t *testing.T) {
	ts, base := newTailnetServer(t)
	cookie := login(t, ts, base)
	if resp := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie}); resp.StatusCode != 200 {
		t.Fatalf("session not usable before the rotation: %d", resp.StatusCode)
	}
	if _, err := token.Rotate(filepath.Join(base, "token")); err != nil {
		t.Fatal(err)
	}
	resp := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session survived the rotation: status %d", resp.StatusCode)
	}
	if code := guardCode(t, resp); code != "unauthorized" {
		t.Errorf("code %q, want unauthorized", code)
	}
	// The browser is sent back to the login page, and the new token works.
	page := tdo(t, ts, "GET", "/", "", navigation("Cookie", cookie))
	if page.StatusCode != http.StatusUnauthorized || !strings.Contains(readAll(t, page.Body), `name="token"`) {
		t.Errorf("navigation after the rotation: status %d, want the login page", page.StatusCode)
	}
	fresh := login(t, ts, base)
	if resp := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": fresh}); resp.StatusCode != 200 {
		t.Errorf("a session minted from the new token: status %d", resp.StatusCode)
	}
}

// The narrowing this task owns: over the network the credential reaches
// the UI's own API and nothing else. Registering a root is the step from
// "read my notes" to "read any file on the machine", and nothing on the
// tailnet clips.
func TestTailnetNarrowsWhatTheCredentialReaches(t *testing.T) {
	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)
	cookie := login(t, ts, base)
	elsewhere := t.TempDir()

	for _, c := range []struct {
		name, method, path, body string
		hdr                      map[string]string
	}{
		{"register a root with the token", "POST", "/api/roots", `{"path":"` + elsewhere + `"}`,
			bearerHeader(tok, "Content-Type", "application/json")},
		{"register a root with the session", "POST", "/api/roots", `{"path":"` + elsewhere + `"}`,
			map[string]string{"Cookie": cookie, "Content-Type": "application/json", "Origin": tailnetOrigin}},
		{"clip with the token", "POST", "/api/clip", clipBody("Remote", "# x\n"),
			bearerHeader(tok, "Content-Type", "application/json")},
		{"clip with the session", "POST", "/api/clip", clipBody("Remote", "# x\n"),
			map[string]string{"Cookie": cookie, "Content-Type": "application/json", "Origin": tailnetOrigin}},
	} {
		resp := tdo(t, ts, c.method, c.path, c.body, c.hdr)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", c.name, resp.StatusCode)
			continue
		}
		if code := guardCode(t, resp); code != "loopback_only" {
			t.Errorf("%s: code %q, want loopback_only", c.name, code)
		}
	}

	// An endpoint that does not exist yet is refused too: the rule is an
	// allow-list, so nothing new is reachable off the machine by default.
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/whatever"},
		{"DELETE", "/api/r/notes/source/hello.md"},
		{"PUT", "/api/r/notes/tree"},
		{"POST", "/api/r/notes/search"},
		{"POST", "/api/roots/"},
		{"POST", "/api/r/notes/source/../../../roots"},
	} {
		resp := tdo(t, ts, c.method, c.path, "", bearerHeader(tok))
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: status %d, want 403", c.method, c.path, resp.StatusCode)
		}
	}

	// The same calls still work on loopback, which is where they belong.
	if resp := do(t, ts, "POST", "/api/roots", `{"path":"`+elsewhere+`"}`,
		bearerHeader(tok, "Content-Type", "application/json")); resp.StatusCode != http.StatusOK {
		t.Errorf("register a root on loopback: status %d, want 200", resp.StatusCode)
	}
	if resp := do(t, ts, "POST", "/api/clip", clipBody("Local", "# x\n"),
		bearerHeader(tok, "Content-Type", "application/json")); resp.StatusCode != http.StatusCreated {
		t.Errorf("clip on loopback: status %d, want 201", resp.StatusCode)
	}
}

// Loopback is what it was before the extra name existed, with the extra
// name configured: same statuses, same codes, no login page, no cookie.
func TestLoopbackIsUnchangedByATailnetHost(t *testing.T) {
	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)
	for _, c := range []struct {
		name   string
		method string
		path   string
		hdr    map[string]string
		want   int
	}{
		{"the UI", "GET", "/", nil, http.StatusOK},
		{"a client-side route", "GET", "/r/notes/hello.md", nil, http.StatusOK},
		{"/login is an ordinary route", "GET", "/login", navigation(), http.StatusOK},
		{"the API unauthenticated", "GET", "/api/roots", nil, http.StatusOK},
		{"the events stream", "GET", "/api/r/notes/tags", nil, http.StatusOK},
		{"the daemon's own origin", "GET", "/api/roots", map[string]string{"Origin": "http://localhost:7337"}, http.StatusOK},
		{"a foreign origin", "GET", "/api/roots", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"the tailnet origin", "GET", "/api/roots", map[string]string{"Origin": tailnetOrigin}, http.StatusForbidden},
		{"a wrong token", "GET", "/api/roots", bearerHeader("nonsense"), http.StatusUnauthorized},
		{"the token from another origin", "GET", "/api/roots", bearerHeader(tok, "Origin", "chrome-extension://abc"), http.StatusOK},
		{"an unexpected Host", "GET", "/api/roots", map[string]string{"Host": "evil.example:7337"}, http.StatusForbidden},
	} {
		resp := do(t, ts, c.method, c.path, "", c.hdr)
		if resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.want)
			continue
		}
		if len(resp.Cookies()) != 0 {
			t.Errorf("%s: loopback was given a cookie: %v", c.name, resp.Cookies())
		}
		if c.want == http.StatusOK && strings.Contains(readAll(t, resp.Body), `name="token"`) {
			t.Errorf("%s: loopback was shown the login page", c.name)
		}
	}
	// The login form does not exist on loopback: posting to it lands on
	// the UI's catch-all, which refuses the method as it always did.
	resp := do(t, ts, "POST", loginPath, "token="+tok,
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if resp.StatusCode != http.StatusMethodNotAllowed || len(resp.Cookies()) != 0 {
		t.Errorf("POST /login on loopback: status %d, cookies %v", resp.StatusCode, resp.Cookies())
	}
	// A session cookie is worth nothing on loopback, because loopback
	// never asked for one — but it must not break the request either.
	cookie := login(t, ts, base)
	if resp := do(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie}); resp.StatusCode != http.StatusOK {
		t.Errorf("a session cookie on loopback: status %d, want the usual 200", resp.StatusCode)
	}
}

// A daemon reachable off the machine with no token at all can mint no
// session, and says so rather than letting anybody in.
func TestTailnetWithoutATokenAdmitsNobody(t *testing.T) {
	ts, _ := newTestServerWith(t, WithTailnetHost(tailnetName), WithToken(nil))
	if resp := tdo(t, ts, "GET", "/api/roots", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
	resp := loginPost(t, ts, "anything", "/")
	if resp.StatusCode != http.StatusUnauthorized || len(resp.Cookies()) != 0 {
		t.Errorf("login: status %d, cookies %v", resp.StatusCode, resp.Cookies())
	}
}

// A session id that was never issued, or one from another daemon, is not
// a session.
func TestTailnetRejectsAForgedCookie(t *testing.T) {
	ts, base := newTailnetServer(t)
	real := login(t, ts, base)
	for _, c := range []string{
		sessionCookie + "=nonsense",
		sessionCookie + "=",
		"mdn_session=" + strings.TrimPrefix(real, sessionCookie+"="),
		real + "x",
	} {
		resp := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": c})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("cookie %q: status %d, want 401", c, resp.StatusCode)
		}
	}
}

func TestSanitizeRedirect(t *testing.T) {
	for given, want := range map[string]string{
		"":                      "/",
		"/":                     "/",
		"/r/notes/a.md":         "/r/notes/a.md",
		"/r/notes/a.md?x=1":     "/r/notes/a.md?x=1",
		"//evil.example":        "/",
		"///evil.example":       "/",
		`/\evil.example`:        "/",
		"https://evil.example":  "/",
		"http://evil.example/x": "/",
		"evil.example":          "/",
		"../etc/passwd":         "/",
		"/../../etc/passwd":     "/etc/passwd",
		"/login":                "/",
		"/r/notes/a.md\r\nX: y": "/",
		"/r/n%20o/a.md":         "/r/n%20o/a.md",
	} {
		if got := sanitizeRedirect(given); got != want {
			t.Errorf("sanitizeRedirect(%q) = %q, want %q", given, got, want)
		}
	}
}

// A session does not last forever; the cookie's Max-Age and the store
// agree about when it stops.
func TestTailnetSessionExpires(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	s.sessions = session.New(40 * time.Millisecond)
	resp := loginPost(t, ts, daemonToken(t, base), "/")
	cookie := cookieHeader(t, resp)
	if got := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie}); got.StatusCode != 200 {
		t.Fatalf("status %d", got.StatusCode)
	}
	time.Sleep(60 * time.Millisecond)
	if got := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie}); got.StatusCode != http.StatusUnauthorized {
		t.Errorf("an expired session: status %d, want 401", got.StatusCode)
	}
}

// raw sends a request exactly as written, so a request target in absolute
// form — which no http.Client will send to an origin server — can be put
// on the wire. It returns the status line.
func raw(t *testing.T, ts *httptest.Server, request string) string {
	t.Helper()
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line)
}

// The tailnet boundary is the Host header and nothing else corroborates
// it, so nothing may be able to decide it *other* than the Host header.
// Go fills r.Host from the request target's authority when the target is
// in absolute form, which would let the target pick the rule.
func TestGuardRefusesAnAbsoluteFormTarget(t *testing.T) {
	ts, base := newTailnetServer(t)
	addr := ts.Listener.Addr().String()
	// The demonstration: a loopback authority in the target with the
	// tailnet name in the header, which was served under the loopback
	// rule with no credential at all.
	got := raw(t, ts, "GET http://"+addr+"/api/roots HTTP/1.1\r\nHost: "+tailnetName+"\r\nConnection: close\r\n\r\n")
	if !strings.Contains(got, "403") {
		t.Errorf("absolute-form target with a tailnet Host: %q, want 403", got)
	}
	// And the other way round, so neither direction is decided by the
	// target.
	got = raw(t, ts, "GET https://"+tailnetName+"/api/roots HTTP/1.1\r\nHost: 127.0.0.1:7337\r\nConnection: close\r\n\r\n")
	if !strings.Contains(got, "403") {
		t.Errorf("absolute-form target with a loopback Host: %q, want 403", got)
	}
	// Origin form, the only form anything sends here, is untouched.
	got = raw(t, ts, "GET /api/roots HTTP/1.1\r\nHost: 127.0.0.1:7337\r\nConnection: close\r\n\r\n")
	if !strings.Contains(got, "200") {
		t.Errorf("origin form on loopback: %q, want 200", got)
	}
	got = raw(t, ts, "GET /api/roots HTTP/1.1\r\nHost: "+tailnetName+"\r\nAuthorization: Bearer "+daemonToken(t, base)+"\r\nConnection: close\r\n\r\n")
	if !strings.Contains(got, "200") {
		t.Errorf("origin form under the tailnet name: %q, want 200", got)
	}
}

// A proxy that rewrote the Host to the upstream — nginx's `proxy_pass`
// does by default — would present a loopback Host carrying the forwarding
// headers it added on the way. Serving that under the loopback rule hands
// the whole API, unauthenticated, to everything the proxy admits. It fails
// closed instead, and the refusal says what to fix.
func TestGuardRefusesAForwardedLoopbackHost(t *testing.T) {
	ts, _ := newTailnetServer(t)
	for _, header := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host"} {
		for _, host := range []string{"127.0.0.1:7337", "localhost:7337"} {
			resp := do(t, ts, "GET", "/api/roots", "", map[string]string{"Host": host, header: "x"})
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s with Host %s: status %d, want 403", header, host, resp.StatusCode)
				continue
			}
			if code := guardCode(t, resp); code != "bad_host" {
				t.Errorf("%s with Host %s: code %q, want bad_host", header, host, code)
			}
		}
	}
	// The write the reviewer reached this way is refused with it.
	resp := do(t, ts, "POST", "/api/roots", `{"path":"/etc"}`, map[string]string{
		"Host": "127.0.0.1:7337", "X-Forwarded-Proto": "https", "X-Forwarded-For": "100.64.0.9",
		"Content-Type": "application/json",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forwarded root registration: status %d, want 403", resp.StatusCode)
	}
	// An ordinary loopback request, which sets none of those, is served.
	if resp := do(t, ts, "GET", "/api/roots", "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("plain loopback: status %d, want 200", resp.StatusCode)
	}
}

// With no tailnet host configured there is no boundary to protect and
// nothing in front, so a forwarding header is just a header.
func TestForwardedHeadersAreOrdinaryWithoutATailnetHost(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-For": "100.64.0.9"})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d, want 200 — loopback without the extra name is unchanged", resp.StatusCode)
	}
}

// The docs tell the user that `tailscale status --json` prints the name
// with its root label, so the fully qualified form of the Host has to
// match the configured name, which is stored without it.
func TestTailnetHostMatchesItsFullyQualifiedForm(t *testing.T) {
	ts, base := newTailnetServer(t)
	tok := daemonToken(t, base)
	for host, want := range map[string]int{
		tailnetName:        http.StatusOK,
		tailnetName + ".":  http.StatusOK,
		tailnetName + "..": http.StatusForbidden,
	} {
		resp := do(t, ts, "GET", "/api/roots", "", bearerHeader(tok, "Host", host))
		if resp.StatusCode != want {
			t.Errorf("Host %q: status %d, want %d", host, resp.StatusCode, want)
		}
	}
	// Loopback's own names are compared as they always were.
	if resp := do(t, ts, "GET", "/api/roots", "", map[string]string{"Host": "localhost.:7337"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("localhost. : status %d, want the 403 it has always had", resp.StatusCode)
	}
}

// The one pre-authentication endpoint on the network side does not answer
// guesses indefinitely.
func TestLoginAttemptsAreBounded(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	// free == max so the refusal is reached without walking through the
	// delayed tier, which has its own test.
	s.logins.free, s.logins.max = 2, 2
	for i := range 2 {
		if resp := loginPost(t, ts, "WRONG", "/"); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i+1, resp.StatusCode)
		}
	}
	resp := loginPost(t, ts, "WRONG", "/")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("the attempt past the limit: status %d, want 429", resp.StatusCode)
	}
	if code := guardCode(t, resp); code != "too_many_attempts" {
		t.Errorf("code %q, want too_many_attempts", code)
	}
	// Being throttled is not being locked out of your own daemon: the
	// right token still works, and clears the count.
	good := loginPost(t, ts, daemonToken(t, base), "/")
	if good.StatusCode != http.StatusSeeOther {
		t.Fatalf("the correct token while throttled: status %d, want 303", good.StatusCode)
	}
	if resp := loginPost(t, ts, "WRONG", "/"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after a success: status %d, want the count cleared", resp.StatusCode)
	}
}
