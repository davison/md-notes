package watch

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a clock whose time only moves when a test moves it. It is
// what lets a test of the debounce say "the quiet window elapsed" instead of
// sleeping for it and hoping the machine kept up.
//
// Advance is called from the test goroutine while the watcher's loop calls
// Now, NewTimer, Stop and Reset from its own, so everything is under one
// mutex. A timer's channel holds one value and is drained by Stop and Reset,
// which is what time.Timer guarantees from Go 1.23 on: after either call no
// value armed before it is ever received.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func newFakeClock() *fakeClock {
	// An arbitrary fixed instant: nothing may depend on the real time.
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, ch: make(chan time.Time, 1), deadline: c.now.Add(d), armed: true}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves the clock on, fires every timer the move reached, and returns
// once each fire has been taken off its channel.
//
// Waiting for that matters: without it a test could only say "nothing has been
// emitted" where no timer fired at all, because a timer that fired a moment
// ago might still be on its way through the watcher's loop. Waiting makes the
// return point "the loop has the fire", and a handled call after it makes the
// point "the loop has finished flushing", which is what an assertion of
// absence needs. A fire is only ever waited for on an armed timer, which the
// loop is by construction selecting on, so this waits for progress the loop is
// already committed to making.
func (c *fakeClock) Advance(d time.Duration) {
	// Not under the lock: taking the fire leads the loop to Stop and Reset,
	// which want it.
	for _, t := range c.advance(d) {
		if !waitFor(func() bool { return len(t.ch) == 0 }) {
			panic("fake clock: a fired timer was never taken")
		}
	}
}

// advance moves the clock and fires without waiting for anything, for the two
// tests below: they are about what reaches a timer's channel, so they are the
// one caller with no loop on the other end to take it.
func (c *fakeClock) advance(d time.Duration) []*fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	var fired []*fakeTimer
	for _, t := range c.timers {
		if t.armed && !t.deadline.After(c.now) {
			t.armed = false
			t.ch <- c.now
			fired = append(fired, t)
		}
	}
	return fired
}

type fakeTimer struct {
	c        *fakeClock
	ch       chan time.Time
	deadline time.Time
	armed    bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.armed
	t.armed = false
	t.drain()
	return was
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.armed
	t.armed, t.deadline = true, t.c.now.Add(d)
	t.drain()
	return was
}

// drain discards a fire that has not been received, so a reset timer never
// hands the loop a deadline that has been moved. Called under the lock.
func (t *fakeTimer) drain() {
	select {
	case <-t.ch:
	default:
	}
}

func TestFakeClockFiresOnlyWhenTheDeadlineIsReached(t *testing.T) {
	c := newFakeClock()
	tm := c.NewTimer(50 * time.Millisecond)
	c.advance(49 * time.Millisecond)
	select {
	case <-tm.C():
		t.Fatal("fired before its deadline")
	default:
	}
	c.advance(time.Millisecond)
	select {
	case <-tm.C():
	default:
		t.Fatal("did not fire at its deadline")
	}
}

func TestFakeClockResetDiscardsAnUnreadFire(t *testing.T) {
	c := newFakeClock()
	tm := c.NewTimer(10 * time.Millisecond)
	c.advance(10 * time.Millisecond) // fires, nobody reads it
	tm.Reset(10 * time.Millisecond)
	select {
	case <-tm.C():
		t.Fatal("a fire from before the reset was still delivered")
	default:
	}
	c.advance(10 * time.Millisecond)
	if got := <-tm.C(); !got.Equal(c.Now()) {
		t.Fatalf("fired at %v, want %v", got, c.Now())
	}
}
