package server

import (
	"sync"
	"time"
)

// The login page is the one endpoint on the network side that runs before
// anything has been proved. The token is 130 bits from crypto/rand, so
// guessing it is hopeless and the limit here is not what stands between an
// attacker and the notes; what it bounds is how fast one caller can make
// the daemon do work and write log lines.
const (
	// loginWindow is how long failures are remembered for.
	loginWindow = time.Minute
	// loginFree is how many failures in a window are answered at once — a
	// person mistyping a token, or pasting the one they rotated away, is
	// not being throttled.
	loginFree = 3
	// loginMax is how many failures in a window are answered at all.
	loginMax = 12
	// loginDelay is how long a failure past loginFree waits before it is
	// answered. Small enough not to look broken, large enough that a
	// caller cannot run through attempts as fast as it can open sockets.
	loginDelay = 500 * time.Millisecond
)

// throttle counts recent failed logins per caller. The address it keys on
// comes from X-Forwarded-For and is therefore only as trustworthy as the
// proxy that set it; that is acceptable here, because the limit exists to
// bound noise and work rather than to defend a secret, and the delay below
// applies whatever the key says.
type throttle struct {
	window time.Duration
	free   int
	max    int

	// now is the clock, replaced in tests.
	now func() time.Time

	mu    sync.Mutex
	seen  map[string]window
	limit int
}

type window struct {
	failures int
	since    time.Time
}

// maxThrottled caps how many callers are remembered, so that a stream of
// failures from varying addresses cannot grow the map without bound. Past
// it the table is cleared rather than pruned one entry at a time: it holds
// no state worth preserving, and a caller that has to start counting again
// still meets the delay.
const maxThrottled = 1024

func newThrottle() *throttle {
	return &throttle{
		window: loginWindow, free: loginFree, max: loginMax,
		now: time.Now, seen: map[string]window{}, limit: maxThrottled,
	}
}

// failed records a failed login from addr and says what to do about it:
// how long to wait before answering, and whether to answer at all.
func (t *throttle) failed(addr string) (wait time.Duration, refuse bool) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.seen) >= t.limit {
		t.seen = map[string]window{}
	}
	w := t.seen[addr]
	if w.since.IsZero() || now.Sub(w.since) >= t.window {
		w = window{since: now}
	}
	w.failures++
	t.seen[addr] = w
	switch {
	case w.failures > t.max:
		return 0, true
	case w.failures > t.free:
		return loginDelay, false
	default:
		return 0, false
	}
}

// succeeded forgets a caller's failures, so that logging in correctly
// clears the slate rather than leaving the next mistype throttled.
func (t *throttle) succeeded(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.seen, addr)
}
