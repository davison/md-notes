package server

import (
	"fmt"
	"testing"
	"time"
)

func TestThrottleTiers(t *testing.T) {
	th := newThrottle()
	now := time.Now()
	th.now = func() time.Time { return now }
	for i := 1; i <= loginFree; i++ {
		if wait, refuse := th.failed("a"); wait != 0 || refuse {
			t.Errorf("failure %d: wait %v refuse %v, want a plain answer", i, wait, refuse)
		}
	}
	for i := loginFree + 1; i <= loginMax; i++ {
		if wait, refuse := th.failed("a"); wait != loginDelay || refuse {
			t.Errorf("failure %d: wait %v refuse %v, want the delay", i, wait, refuse)
		}
	}
	if _, refuse := th.failed("a"); !refuse {
		t.Errorf("failure %d: not refused", loginMax+1)
	}
	// A second caller has its own count, but by now the daemon has seen
	// more than free failures in total, so the floor applies to it.
	if wait, refuse := th.failed("b"); wait != loginDelay || refuse {
		t.Errorf("a second caller: wait %v refuse %v, want the floor", wait, refuse)
	}
	// The window rolls, for the total as well as for the key.
	now = now.Add(loginWindow)
	if wait, refuse := th.failed("a"); wait != 0 || refuse {
		t.Errorf("after the window: wait %v refuse %v, want a plain answer", wait, refuse)
	}
	// A success clears the slate rather than leaving the next mistype
	// throttled.
	for range loginMax {
		th.failed("a")
	}
	th.succeeded("a")
	// The key's own count is cleared; the total is not, because whoever
	// is generating failures still is.
	if wait, refuse := th.failed("a"); wait != loginDelay || refuse {
		t.Errorf("after a success: wait %v refuse %v, want the floor to stand", wait, refuse)
	}
	now = now.Add(loginWindow)
	if wait, refuse := th.failed("a"); wait != 0 || refuse {
		t.Errorf("a fresh window after a success: wait %v refuse %v, want a plain answer", wait, refuse)
	}
}

// The key comes from X-Forwarded-For, which a caller can vary. Varying it
// escapes the per-key count — that is unavoidable, since under a proxy the
// peer address is always the proxy — but it must not escape the delay, or
// the limit bounds nothing at all.
func TestVaryingTheKeyDoesNotEscapeTheDelay(t *testing.T) {
	th := newThrottle()
	now := time.Now()
	th.now = func() time.Time { return now }
	var delayed, plain int
	for i := range 40 {
		wait, refuse := th.failed(fmt.Sprintf("10.0.0.%d", i))
		if refuse {
			t.Fatalf("attempt %d was refused; a total counted across callers must never lock anyone out", i)
		}
		if wait > 0 {
			delayed++
		} else {
			plain++
		}
	}
	if plain != loginFree {
		t.Errorf("%d attempts answered at once, want %d — the floor did not engage", plain, loginFree)
	}
	if delayed != 40-loginFree {
		t.Errorf("%d attempts delayed, want %d", delayed, 40-loginFree)
	}
	// No key ever reached its own limit, which is the bypass.
	for addr, w := range th.seen {
		if w.failures > th.free {
			t.Fatalf("%s reached %d failures; the keys were supposed to vary", addr, w.failures)
		}
	}
}

// Failures from varying addresses must not grow the table without bound.
func TestThrottleTableIsBounded(t *testing.T) {
	th := newThrottle()
	th.limit = 8
	for i := range 100 {
		th.failed(string(rune('a' + i%64)))
	}
	if len(th.seen) > th.limit {
		t.Errorf("%d entries held, want at most %d", len(th.seen), th.limit)
	}
	// And clearing the table does not stop it counting afterwards.
	for range th.max + 1 {
		th.failed("z")
	}
	if _, refuse := th.failed("z"); !refuse {
		t.Error("counting stopped after the table was cleared")
	}
}
