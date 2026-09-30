package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davison/md-notes/internal/session"
	"github.com/davison/md-notes/internal/token"
)

// These are M12's: a tailnet login lasts until the token is rotated, not
// until the daemon next restarts (davison/md-notes#231). A "restart" here
// is a second Server over the same base directory — nothing carried over
// but the notes and the token file, which is all a restarted process has.

// M12-R1: the same cookie, presented to the daemon after a restart, is a
// session, with no token presented again.
func TestTailnetSessionSurvivesARestart(t *testing.T) {
	before, base := newTailnetServer(t)
	cookie := login(t, before, base)
	before.Close()

	after := serveBase(t, base, WithTailnetHost(tailnetName))
	if got := sessionStatus(t, after, cookie); got != http.StatusOK {
		t.Fatalf("the cookie after a restart: status %d, want 200", got)
	}
	page := tdo(t, after, "GET", "/", "", navigation("Cookie", cookie))
	if page.StatusCode != http.StatusOK || strings.Contains(readAll(t, page.Body), `name="token"`) {
		t.Errorf("navigation after a restart: status %d, want the app and not the login page", page.StatusCode)
	}
}

// M12-R2: a token file replaced while the daemon was stopped ends every
// session, on the first request to the daemon that starts over it.
func TestTailnetRotationWhileStoppedEndsTheSession(t *testing.T) {
	before, base := newTailnetServer(t)
	cookie := login(t, before, base)
	before.Close()
	if _, err := token.Rotate(filepath.Join(base, "token")); err != nil {
		t.Fatal(err)
	}

	after := serveBase(t, base, WithTailnetHost(tailnetName))
	if got := sessionStatus(t, after, cookie); got != http.StatusUnauthorized {
		t.Fatalf("a cookie from before the rotation: status %d, want 401", got)
	}
	fresh := login(t, after, base)
	if got := sessionStatus(t, after, fresh); got != http.StatusOK {
		t.Errorf("a login with the new token: status %d, want 200", got)
	}
}

// M12-R2: a rotation while running ends the session whatever else has
// happened, a restart in between included — and a restart after the
// rotation does not bring the old cookie back.
func TestTailnetRotationEndsTheSessionAcrossARestart(t *testing.T) {
	before, base := newTailnetServer(t)
	cookie := login(t, before, base)
	if _, err := token.Rotate(filepath.Join(base, "token")); err != nil {
		t.Fatal(err)
	}
	if got := sessionStatus(t, before, cookie); got != http.StatusUnauthorized {
		t.Fatalf("after a rotation while running: status %d, want 401", got)
	}
	before.Close()
	after := serveBase(t, base, WithTailnetHost(tailnetName))
	if got := sessionStatus(t, after, cookie); got != http.StatusUnauthorized {
		t.Errorf("the rotated-away cookie after a restart: status %d, want 401", got)
	}
}

// M12-R2: a cookie is worth nothing under a token it was not issued with
// — another daemon's, whose file is laid out exactly like this one's.
func TestTailnetCookieFromAnotherTokenIsRefused(t *testing.T) {
	a, baseA := newTailnetServer(t)
	b, _ := newTailnetServer(t)
	cookie := login(t, a, baseA)
	if got := sessionStatus(t, b, cookie); got != http.StatusUnauthorized {
		t.Errorf("a cookie from another daemon's token: status %d, want 401", got)
	}
}

// A cookie is bound to the tailnet name it was issued under, even by a
// daemon holding the same token. The browser already keeps a __Host-
// cookie to that name; this is the daemon not relying on it.
func TestTailnetCookieIsBoundToItsHostName(t *testing.T) {
	before, base := newTailnetServer(t)
	cookie := login(t, before, base)
	const other = "desktop.example.ts.net"
	after := serveBase(t, base, WithTailnetHost(other))
	resp := tdo(t, after, "GET", "/api/roots", "", map[string]string{"Host": other, "Cookie": cookie})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a cookie issued under %s, presented under %s: status %d, want 401", tailnetName, other, resp.StatusCode)
	}
}

// sessionCookieFrom returns the session cookie a response set, or nil.
func sessionCookieFrom(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

// M12-R3: a session in use is reissued on use, so it never reaches a fixed
// end — but at most once a day, not a Set-Cookie on every response.
func TestTailnetSessionIsRefreshedOnUse(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	clock := t0
	s.now = func() time.Time { return clock }
	cookie := login(t, ts, base)

	clock = t0.Add(time.Hour)
	resp := tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if c := sessionCookieFrom(resp); c != nil {
		t.Errorf("an hour-old session was reissued: %v", c)
	}

	clock = t0.Add(25 * time.Hour)
	resp = tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	c := sessionCookieFrom(resp)
	if c == nil {
		t.Fatal("a day-old session in use was not reissued")
	}
	if c.Value == strings.TrimPrefix(cookie, sessionCookie+"=") {
		t.Error("the reissued cookie is the one presented")
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("the reissued cookie lost a protection: %+v", c)
	}
	if c.MaxAge != int(session.Idle.Seconds()) {
		t.Errorf("Max-Age %d, want the idle limit, %d", c.MaxAge, int(session.Idle.Seconds()))
	}

	// The reissued cookie carries the session forward past where the
	// first one would have lapsed.
	refreshed := sessionCookie + "=" + c.Value
	clock = t0.Add(session.Idle + time.Hour)
	if got := sessionStatus(t, ts, cookie); got != http.StatusUnauthorized {
		t.Errorf("the original cookie past the idle limit: status %d, want 401", got)
	}
	if got := sessionStatus(t, ts, refreshed); got != http.StatusOK {
		t.Errorf("the reissued cookie, used within the idle limit of its reissue: status %d, want 200", got)
	}
}

// M12-R3: what bounds a session nobody uses is the idle limit, and the
// cookie's Max-Age says the same thing to the browser.
func TestTailnetUnusedSessionLapses(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	clock := t0
	s.now = func() time.Time { return clock }
	resp := loginPost(t, ts, daemonToken(t, base), "/")
	c := sessionCookieFrom(resp)
	if c == nil {
		t.Fatal("no cookie")
	}
	if c.MaxAge != int(session.Idle.Seconds()) {
		t.Errorf("Max-Age %d, want the idle limit, %d", c.MaxAge, int(session.Idle.Seconds()))
	}
	cookie := sessionCookie + "=" + c.Value
	clock = t0.Add(session.Idle - time.Minute)
	if got := sessionStatus(t, ts, cookie); got != http.StatusOK {
		t.Errorf("just inside the idle limit: status %d, want 200", got)
	}
	// The request above reissued it; the original is what an unused
	// device holds.
	clock = t0.Add(session.Idle)
	if got := sessionStatus(t, ts, cookie); got != http.StatusUnauthorized {
		t.Errorf("at the idle limit: status %d, want 401", got)
	}
}

// A reissue rides only on a response the guard serves: a request it
// refuses — a foreign Origin, an endpoint kept to loopback — is answered
// with the refusal and no new cookie.
func TestTailnetRefusedRequestIsNotReissued(t *testing.T) {
	ts, base := newTailnetServer(t)
	s := serverOf(t, ts)
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	clock := t0
	s.now = func() time.Time { return clock }
	cookie := login(t, ts, base)
	clock = t0.Add(48 * time.Hour)
	for name, resp := range map[string]*http.Response{
		"a foreign Origin":      tdo(t, ts, "GET", "/api/roots", "", map[string]string{"Cookie": cookie, "Origin": "https://evil.example"}),
		"a loopback-only write": tdo(t, ts, "POST", "/api/roots", `{"path":"/"}`, map[string]string{"Cookie": cookie, "Origin": tailnetOrigin}),
	} {
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, resp.StatusCode)
		}
		if c := sessionCookieFrom(resp); c != nil {
			t.Errorf("%s: a refused request reissued the session", name)
		}
	}
}
