package session

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"
)

const host = "laptop.example.ts.net"

var (
	t0  = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	key = bytes.Repeat([]byte{7}, 32)
)

func TestIssueAndCheck(t *testing.T) {
	v := Issue(key, host, t0)
	if v == "" {
		t.Fatal("empty session value")
	}
	ok, due := Check(key, host, v, t0)
	if !ok || due {
		t.Fatalf("a fresh session: (%v, %v), want (true, false)", ok, due)
	}
	// The value is safe as a cookie value without quoting.
	if strings.ContainsAny(v, " \t\",;\\=") {
		t.Errorf("value %q needs quoting in a cookie", v)
	}
}

// Two logins in the same second must not produce the same value.
func TestSessionsAreDistinct(t *testing.T) {
	if Issue(key, host, t0) == Issue(key, host, t0) {
		t.Fatal("two logins produced the same session value")
	}
}

// Whatever it is keyed by, a session is bound to it: another token's key
// is another daemon, or this one after a rotation.
func TestAnotherKeyIsRefused(t *testing.T) {
	v := Issue(key, host, t0)
	other := bytes.Repeat([]byte{8}, 32)
	if ok, _ := Check(other, host, v, t0); ok {
		t.Error("a session checked under another key was accepted")
	}
	if ok, _ := Check(nil, host, v, t0); ok {
		t.Error("a session checked under no key was accepted")
	}
}

func TestAnotherHostIsRefused(t *testing.T) {
	v := Issue(key, host, t0)
	for _, h := range []string{"desktop.example.ts.net", "", host + ":443", "LAPTOP.example.ts.net"} {
		if ok, _ := Check(key, h, v, t0); ok {
			t.Errorf("a session issued for %s was accepted for %q", host, h)
		}
	}
}

// resign builds a value the way Issue does, with fields of the test's
// choosing, so a test can show that only the MAC stands between a caller
// and the payload it would like.
func resign(k []byte, h, version, issued, id string) string {
	m := hmac.New(sha256.New, k)
	m.Write([]byte(version + "\x00" + h + "\x00" + issued + "\x00" + id))
	return version + "." + issued + "." + id + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Every field is under the MAC: changing any one of them, without the
// key, is a value that is not a session.
func TestTamperingIsRefused(t *testing.T) {
	v := Issue(key, host, t0)
	parts := strings.Split(v, ".")
	if len(parts) != 4 {
		t.Fatalf("value %q: %d fields, want 4", v, len(parts))
	}
	later := time.Now().Add(365 * 24 * time.Hour).Unix()
	cases := map[string]string{
		"issued-at moved":      strings.Join([]string{parts[0], strconv.FormatInt(later, 10), parts[2], parts[3]}, "."),
		"id changed":           strings.Join([]string{parts[0], parts[1], "AAAAAAAAAAAAAAAAAAAAAA", parts[3]}, "."),
		"version changed":      strings.Join([]string{"v2", parts[1], parts[2], parts[3]}, "."),
		"MAC truncated":        strings.Join([]string{parts[0], parts[1], parts[2], parts[3][:len(parts[3])-4]}, "."),
		"MAC halved":           strings.Join([]string{parts[0], parts[1], parts[2], parts[3][:len(parts[3])/2]}, "."),
		"MAC extended":         v + "AAAA",
		"MAC absent":           strings.Join(parts[:3], "."),
		"MAC empty":            strings.Join([]string{parts[0], parts[1], parts[2], ""}, "."),
		"MAC not base64":       strings.Join([]string{parts[0], parts[1], parts[2], strings.Repeat("*", len(parts[3]))}, "."),
		"MAC one bit flipped":  strings.Join([]string{parts[0], parts[1], parts[2], flip(parts[3])}, "."),
		"extra field":          v + ".x",
		"empty":                "",
		"a v0.3.0 session id":  "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"issued-at signed +":   resign(key, host, "v1", "+"+parts[1], parts[2]),
		"issued-at not number": resign(key, host, "v1", "soon", parts[2]),
	}
	for name, c := range cases {
		if ok, _ := Check(key, host, c, t0); ok {
			t.Errorf("%s: %q was accepted", name, c)
		}
	}
	// And the control: resign with the real key reproduces a valid value,
	// so the cases above fail for the field they change and not for a
	// mistake in how this test builds values.
	if ok, _ := Check(key, host, resign(key, host, parts[0], parts[1], parts[2]), t0); !ok {
		t.Error("the control, re-signed with the real key, was refused")
	}
}

func flip(s string) string {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	b[0] ^= 1
	return base64.RawURLEncoding.EncodeToString(b)
}

// M12-R3: a session is refused once it has gone unused for the idle limit,
// and not a moment before.
func TestIdleLimit(t *testing.T) {
	v := Issue(key, host, t0)
	if ok, _ := Check(key, host, v, t0.Add(Idle-time.Second)); !ok {
		t.Error("refused a second inside the idle limit")
	}
	if ok, _ := Check(key, host, v, t0.Add(Idle)); ok {
		t.Error("accepted at the idle limit")
	}
}

// Reissue at most once a day: due after RefreshAfter, not before.
func TestRefreshIsDueAfterADay(t *testing.T) {
	v := Issue(key, host, t0)
	if ok, due := Check(key, host, v, t0.Add(RefreshAfter-time.Second)); !ok || due {
		t.Errorf("just under a day: (%v, %v), want (true, false)", ok, due)
	}
	if ok, due := Check(key, host, v, t0.Add(RefreshAfter)); !ok || !due {
		t.Errorf("a day old: (%v, %v), want (true, true)", ok, due)
	}
}

// A value from the future was not issued by this daemon's clock: only a
// clock that went backwards, or a key that leaked, makes one. A few
// minutes of skew are tolerated so a clock correction does not log a
// device out.
func TestIssuedInTheFutureIsRefused(t *testing.T) {
	v := Issue(key, host, t0.Add(time.Minute))
	if ok, _ := Check(key, host, v, t0); !ok {
		t.Error("a minute of clock skew logged the session out")
	}
	v = Issue(key, host, t0.Add(time.Hour))
	if ok, _ := Check(key, host, v, t0); ok {
		t.Error("a session issued an hour from now was accepted")
	}
}

// The cookie carries nothing from which the key can be read back.
func TestTheValueDoesNotCarryTheKey(t *testing.T) {
	v := Issue(key, host, t0)
	for _, enc := range []string{string(key), base64.RawURLEncoding.EncodeToString(key), base64.StdEncoding.EncodeToString(key)} {
		if strings.Contains(v, enc) {
			t.Errorf("the value %q carries the key", v)
		}
	}
}
