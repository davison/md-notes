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
	// answered — past loginFree from one caller, or past loginFree in
	// total. Small enough not to look broken, large enough that a caller
	// cannot run through attempts as fast as it can open sockets.
	loginDelay = 500 * time.Millisecond
)

// throttle counts recent failed logins, per caller and in total.
//
// The address it keys on comes from X-Forwarded-For and is therefore only
// as trustworthy as the proxy that set it: a caller that varies the value
// reaches no key's own limit. That is what the total is for. Once the
// daemon has seen more than free failures in a window across every key,
// each further failure waits, whatever key it claims — so the delay is a
// property of the daemon rather than of a header the caller controls. The
// total never refuses, only delays, because a refusal counted across all
// callers would let anyone the ACL admits lock the operator out.
type throttle struct {
	window time.Duration
	free   int
	max    int
	delay  time.Duration

	// now is the clock, replaced in tests.
	now func() time.Time

	mu   sync.Mutex
	seen map[string]window
	// all counts failures across every key, so that varying the key
	// escapes the count but not the delay.
	all   window
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
		window: loginWindow, free: loginFree, max: loginMax, delay: loginDelay,
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
	t.all = t.all.record(now, t.window)
	w := t.seen[addr].record(now, t.window)
	t.seen[addr] = w
	if w.failures > t.max {
		return 0, true
	}
	// The per-key tier separates one caller's mistypes from another's;
	// the total is the floor underneath it, which no choice of key can
	// get out from under.
	if w.failures > t.free || t.all.failures > t.free {
		return t.delay, false
	}
	return 0, false
}

// record adds one failure to a counting window, starting a fresh one when
// the old has run out.
func (w window) record(now time.Time, length time.Duration) window {
	if w.since.IsZero() || now.Sub(w.since) >= length {
		w = window{since: now}
	}
	w.failures++
	return w
}

// succeeded forgets a caller's failures, so that logging in correctly
// clears the slate rather than leaving the next mistype throttled. The
// total is deliberately left standing: whoever is generating failures is
// still generating them, and a successful login elsewhere is no reason to
// stop delaying them.
func (t *throttle) succeeded(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.seen, addr)
}
