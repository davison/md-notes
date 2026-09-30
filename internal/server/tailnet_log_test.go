package server

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davison/md-notes/internal/session"
	"github.com/davison/md-notes/internal/token"
)

// M13-R2: every tailnet request sent to the login page or refused is logged
// with its reason, and a login names the browser family — with no cookie
// value, token or anything derived from them in the log
// (davison/md-notes#240).

// syncBuffer is a log sink the handler goroutines and the test can share.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// logged points the daemon's log at a buffer the test reads, and its
// clock at one the test moves.
func logged(t *testing.T, s *Server) (*syncBuffer, *time.Time) {
	t.Helper()
	buf := &syncBuffer{}
	s.log = log.New(buf, "", 0)
	clock := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	return buf, &clock
}

// since is what the log gained after mark.
func since(buf *syncBuffer, mark int) string { return buf.String()[mark:] }

const android = "Mozilla/5.0 (Linux; Android 14; Pixel 10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36"

func TestTailnetRefusalsAreLoggedWithTheirReason(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	buf, clock := logged(t, s)
	t0 := *clock

	good := login(t, ts, base)
	// Another daemon's session: a well-formed value under another token.
	otherTS, otherBase := newTailnetServer(t)
	foreign := login(t, otherTS, otherBase)

	type step struct {
		name   string
		addr   string
		do     func() *http.Response
		status int
		want   string
	}
	steps := []step{
		{"a navigation with no cookie", "100.64.0.21", func() *http.Response {
			return tdo(t, ts, "GET", "/r/notes/hello.md", "", navigation("X-Forwarded-For", "100.64.0.21"))
		}, 401, "tailnet: 100.64.0.21 GET /r/notes/hello.md sent to the login page: no session cookie"},
		{"an API call with no cookie", "100.64.0.22", func() *http.Response {
			return tdo(t, ts, "GET", "/api/roots?q=secret", "", map[string]string{"X-Forwarded-For": "100.64.0.22"})
		}, 401, "tailnet: 100.64.0.22 GET /api/roots refused (401 unauthorized): no session cookie"},
		{"a v0.3.0 session id", "100.64.0.23", func() *http.Response {
			return tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.0.23", "Cookie": sessionCookie + "=ABCDEFGHIJKLMNOPQRSTUVWXYZ"})
		}, 401, "refused (401 unauthorized): not a session value"},
		{"another token's session", "100.64.0.24", func() *http.Response {
			return tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.0.24", "Cookie": foreign})
		}, 401, "refused (401 unauthorized): bad signature (another token, a token replaced while the daemon was stopped, another tailnet_host, or altered)"},
		{"a wrong bearer token", "100.64.0.25", func() *http.Response {
			return tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.0.25", "Authorization": "Bearer not-the-token"})
		}, 401, "refused (401 unauthorized): bearer token is not the current token"},
		{"a foreign Origin", "100.64.0.26", func() *http.Response {
			return tdo(t, ts, "PUT", "/api/r/notes/source/hello.md", "{}", map[string]string{"X-Forwarded-For": "100.64.0.26", "Cookie": good, "Origin": "https://evil.example"})
		}, 403, "tailnet: 100.64.0.26 PUT /api/r/notes/source/hello.md refused (403 cross_origin): Origin \"https://evil.example\" is not " + tailnetOrigin},
		{"a loopback-only endpoint", "100.64.0.27", func() *http.Response {
			return tdo(t, ts, "POST", "/api/roots", "{}", map[string]string{"X-Forwarded-For": "100.64.0.27", "Cookie": good, "Origin": tailnetOrigin})
		}, 403, "tailnet: 100.64.0.27 POST /api/roots refused (403 loopback_only): served on loopback only"},
		{"a Host the daemon does not answer to", "100.64.0.28", func() *http.Response {
			return tdo(t, ts, "GET", "/", "", map[string]string{"X-Forwarded-For": "100.64.0.28", "Host": "laptop.example.ts.net:8443"})
		}, 403, "tailnet: 100.64.0.28 GET / refused (403 bad_host): Host \"laptop.example.ts.net:8443\" is not " + tailnetName},
	}
	for _, st := range steps {
		mark := len(buf.String())
		resp := st.do()
		if resp.StatusCode != st.status {
			t.Errorf("%s: status %d, want %d", st.name, resp.StatusCode, st.status)
		}
		if got := since(buf, mark); !strings.Contains(got, st.want) {
			t.Errorf("%s: log %q, want a line containing %q", st.name, got, st.want)
		}
	}

	// Expired: the good session, unused past the idle limit.
	mark := len(buf.String())
	*clock = t0.Add(session.Idle)
	tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.0.29", "Cookie": good})
	if got := since(buf, mark); !strings.Contains(got, "refused (401 unauthorized): expired: unused for the idle limit") {
		t.Errorf("an expired session: log %q", got)
	}
	*clock = t0

	// Rotated while running: the daemon still holds the token it replaced,
	// so it can say so.
	if _, err := token.Rotate(filepath.Join(base, "token")); err != nil {
		t.Fatal(err)
	}
	mark = len(buf.String())
	tdo(t, ts, "GET", "/", "", navigation("X-Forwarded-For", "100.64.0.30", "Cookie", good))
	if got := since(buf, mark); !strings.Contains(got, "sent to the login page: signed with the token before the last rotation") {
		t.Errorf("a rotated-away session: log %q", got)
	}

	// None of it carries a secret.
	secrets := map[string]string{
		"the session cookie": strings.TrimPrefix(good, sessionCookie+"="),
		"the other cookie":   strings.TrimPrefix(foreign, sessionCookie+"="),
		"the new token":      daemonToken(t, base),
		"the other token":    daemonToken(t, otherBase),
		"the query":          "q=secret",
	}
	for _, k := range [][]byte{s.token.Derive(session.KeyPurpose), s.token.DerivePrevious(session.KeyPurpose)} {
		secrets[fmt.Sprintf("key %x", k[:4])] = hex.EncodeToString(k)
		secrets[fmt.Sprintf("key b64 %x", k[:4])] = base64.RawURLEncoding.EncodeToString(k)
	}
	for _, part := range strings.Split(strings.TrimPrefix(good, sessionCookie+"="), ".")[2:] {
		secrets["a cookie field "+part[:4]] = part
	}
	out := buf.String()
	for name, v := range secrets {
		if v != "" && strings.Contains(out, v) {
			t.Errorf("the log carries %s", name)
		}
	}
	if strings.Contains(out, "Mozilla/") {
		t.Error("the log carries a User-Agent string")
	}
}

// A login names the browser and the platform, coarsely, and never the
// User-Agent itself.
func TestTailnetLoginNamesTheBrowserFamily(t *testing.T) {
	ts, base := newTailnetServer(t)
	buf, _ := logged(t, serverOf(t, ts))
	resp := loginPost(t, ts, daemonToken(t, base), "/", "User-Agent", android)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status %d", resp.StatusCode)
	}
	if got := buf.String(); !strings.Contains(got, "tailnet login from 100.64.0.9 (Chrome on Android)") {
		t.Errorf("log %q, want the login with its browser family", got)
	}
	if strings.Contains(buf.String(), "Mozilla/") {
		t.Error("the log carries the User-Agent string")
	}
}

// One caller repeating one refusal is one line a minute, and every caller
// together is at most refusalBurst lines a minute; the first line after a
// capped minute says how many were left out.
func TestTailnetRefusalLogIsBounded(t *testing.T) {
	ts, _ := newTailnetServer(t)
	s := serverOf(t, ts)
	buf, clock := logged(t, s)
	t0 := *clock
	count := func() int { return strings.Count(buf.String(), "\n") }

	for i := 0; i < 5; i++ {
		tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.0.40"})
	}
	if n := count(); n != 1 {
		t.Fatalf("one caller, one reason, five times: %d lines, want 1\n%s", n, buf.String())
	}
	*clock = t0.Add(time.Minute)
	tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.0.40"})
	if n := count(); n != 2 {
		t.Fatalf("the same refusal a minute later: %d lines, want 2", n)
	}

	// A caller varying its forwarded address meets the total cap.
	*clock = t0.Add(2 * time.Minute)
	before := count()
	for i := 0; i < refusalBurst+20; i++ {
		tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": fmt.Sprintf("100.64.1.%d", i)})
	}
	if n := count() - before; n != refusalBurst {
		t.Fatalf("%d varying callers in a minute: %d lines, want the cap, %d", refusalBurst+20, n, refusalBurst)
	}
	*clock = t0.Add(3 * time.Minute)
	mark := len(buf.String())
	tdo(t, ts, "GET", "/api/roots", "", map[string]string{"X-Forwarded-For": "100.64.2.1"})
	if got := since(buf, mark); !strings.Contains(got, "20 tailnet refusals not logged in the minute from 09:02:00") {
		t.Errorf("after a capped minute: log %q, want the count left out", got)
	}
}

// Review finding 1 on PR #241: a caller must not be able to write lines of
// its own into the log. The path is percent-decoded before it is logged,
// so %0a and %0d would be line breaks and %1b an escape sequence.
func TestTailnetLogCannotBeForged(t *testing.T) {
	ts, _ := newTailnetServer(t)
	buf, _ := logged(t, serverOf(t, ts))
	forged := "/api/x%0a2026/09/30%2000:00:00%20tailnet%20login%20from%20100.64.0.1%20(Chrome%20on%20Android)%1b[31mRED%1b[0m"
	tdo(t, ts, "GET", forged, "", map[string]string{"X-Forwarded-For": "100.64.0.50"})
	tdo(t, ts, "GET", "/r/a%0d%0aFORGED%20LINE%c2%85%e2%80%a8", "", navigation("X-Forwarded-For", "100.64.0.51"))
	out := buf.String()
	if n := strings.Count(out, "\n"); n != 2 {
		t.Errorf("two refusals wrote %d lines:\n%s", n, out)
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "tailnet: 100.64.0.5") {
			t.Errorf("a line the daemon did not write: %q", line)
		}
	}
	for _, r := range out {
		if (r < 0x20 && r != '\n') || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 {
			t.Errorf("the log carries control character %U:\n%q", r, out)
		}
	}
	if !strings.Contains(out, `/api/x\n2026/09/30`) || !strings.Contains(out, `\x1b[31mRED`) {
		t.Errorf("the path is not logged in escaped form:\n%s", out)
	}
}

// Review finding 2 on PR #241: every tailnet refusal is logged with its
// reason, the login form's and the Host guard's included.
func TestEveryTailnetRefusalIsLogged(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	buf, _ := logged(t, s)
	s.logins.delay = 0
	form := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Origin": tailnetOrigin}
	with := func(h map[string]string, kv ...string) map[string]string {
		out := map[string]string{}
		for k, v := range h {
			out[k] = v
		}
		for i := 0; i+1 < len(kv); i += 2 {
			out[kv[i]] = kv[i+1]
		}
		return out
	}
	for _, c := range []struct {
		name   string
		do     func() int
		status int
		want   string
	}{
		{"a login that is not a POST", func() int {
			return tdo(t, ts, "PUT", loginPath, "", with(form, "X-Forwarded-For", "100.64.3.1")).StatusCode
		}, 405, "tailnet: 100.64.3.1 PUT /login refused (405 method_not_allowed): the login form is posted"},
		{"a login from another origin", func() int {
			return tdo(t, ts, "POST", loginPath, "token=x", with(form, "X-Forwarded-For", "100.64.3.2", "Origin", "https://evil.example")).StatusCode
		}, 403, `tailnet: 100.64.3.2 POST /login refused (403 cross_origin): Origin "https://evil.example" is not ` + tailnetOrigin},
		{"a login form that cannot be read", func() int {
			return tdo(t, ts, "POST", loginPath, "token=%zz", with(form, "X-Forwarded-For", "100.64.3.3")).StatusCode
		}, 400, "tailnet: 100.64.3.3 POST /login refused (400): the login form could not be read"},
		{"a login not over https", func() int {
			return tdo(t, ts, "POST", loginPath, "token=x", with(form, "X-Forwarded-For", "100.64.3.4", "", "X-Forwarded-Proto")).StatusCode
		}, 400, "tailnet: 100.64.3.4 POST /login refused (400): not over https: no X-Forwarded-Proto: https from the proxy, so the Secure cookie would be discarded"},
		{"a wrong token", func() int {
			return tdo(t, ts, "POST", loginPath, "token=wrong", with(form, "X-Forwarded-For", "100.64.3.5")).StatusCode
		}, 401, "tailnet login refused from 100.64.3.5: not the current token"},
		{"too many failed logins", func() int {
			st := 0
			for i := 0; i < 20 && st != 429; i++ {
				st = tdo(t, ts, "POST", loginPath, "token=wrong", with(form, "X-Forwarded-For", "100.64.3.6")).StatusCode
			}
			return st
		}, 429, "tailnet: 100.64.3.6 POST /login refused (429 too_many_attempts): too many failed logins"},
		{"a forwarded request carrying the loopback Host", func() int {
			return tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Host": "127.0.0.1:7337", "X-Forwarded-For": "100.64.3.7"}).StatusCode
		}, 403, `tailnet: 100.64.3.7 GET /api/roots refused (403 bad_host): a forwarded request carries the loopback Host "127.0.0.1:7337"; the proxy must pass the original Host through`},
		{"a request target in absolute form", func() int {
			line := raw(t, ts, "GET http://"+tailnetName+"/api/roots HTTP/1.1\r\nHost: "+tailnetName+"\r\nX-Forwarded-For: 100.64.3.8\r\nConnection: close\r\n\r\n")
			if strings.Contains(line, " 403 ") {
				return 403
			}
			return 0
		}, 403, "tailnet: 100.64.3.8 GET /api/roots refused (403 bad_host): the request target is in absolute form"},
	} {
		mark := len(buf.String())
		if got := c.do(); got != c.status {
			t.Errorf("%s: status %d, want %d", c.name, got, c.status)
		}
		if got := since(buf, mark); !strings.Contains(got, c.want) {
			t.Errorf("%s: log %q, want a line containing %q", c.name, got, c.want)
		}
	}
	if strings.Contains(buf.String(), daemonToken(t, base)) {
		t.Error("the log carries the token")
	}
}
