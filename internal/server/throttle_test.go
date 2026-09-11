package server

import (
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
	// Another caller is unaffected by the first one's count.
	if wait, refuse := th.failed("b"); wait != 0 || refuse {
		t.Errorf("a second caller: wait %v refuse %v, want a plain answer", wait, refuse)
	}
	// The window rolls.
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
	if wait, refuse := th.failed("a"); wait != 0 || refuse {
		t.Errorf("after a success: wait %v refuse %v, want a plain answer", wait, refuse)
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
