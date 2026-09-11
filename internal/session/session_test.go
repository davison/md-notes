package session

import (
	"sync"
	"testing"
	"time"
)

func TestCreateAndValidate(t *testing.T) {
	s := New(time.Hour)
	id := s.Create(1)
	if id == "" {
		t.Fatal("empty session id")
	}
	if !s.Valid(id, 1) {
		t.Error("a fresh session is not valid")
	}
	if s.Valid(id+"x", 1) {
		t.Error("an unknown id is valid")
	}
	if s.Valid("", 1) {
		t.Error("the empty id is valid")
	}
}

// Two logins must not produce the same id, and one must not end the other.
func TestSessionsAreIndependent(t *testing.T) {
	s := New(time.Hour)
	a, b := s.Create(1), s.Create(1)
	if a == b {
		t.Fatal("two logins produced the same session id")
	}
	if !s.Valid(a, 1) || !s.Valid(b, 1) {
		t.Error("one login invalidated another")
	}
}

// The whole point of tying a session to the token generation: rotating
// the token logs every browser out, with nothing to clean up.
func TestRotationInvalidatesEverySession(t *testing.T) {
	s := New(time.Hour)
	a, b := s.Create(7), s.Create(7)
	if !s.Valid(a, 7) {
		t.Fatal("session invalid before the rotation")
	}
	if s.Valid(a, 8) || s.Valid(b, 8) {
		t.Error("a session survived its token generation")
	}
	// And it is forgotten rather than left to be refused again.
	if s.Len() != 0 {
		t.Errorf("%d sessions left after the rotation, want 0", s.Len())
	}
	// A session minted after the rotation is good under the new
	// generation and not under the old one.
	c := s.Create(8)
	if !s.Valid(c, 8) {
		t.Error("a session minted after the rotation is not valid")
	}
	if s.Valid(c, 7) {
		t.Error("a session is valid under a generation that preceded it")
	}
}

func TestSessionsExpire(t *testing.T) {
	s := New(time.Hour)
	now := time.Now()
	s.now = func() time.Time { return now }
	id := s.Create(1)
	now = now.Add(time.Hour - time.Second)
	if !s.Valid(id, 1) {
		t.Error("session expired early")
	}
	now = now.Add(2 * time.Second)
	if s.Valid(id, 1) {
		t.Error("session outlived its ttl")
	}
	if s.Len() != 0 {
		t.Errorf("%d expired sessions left behind, want 0", s.Len())
	}
}

// Expired sessions must not accumulate even if nothing ever presents them
// again, and the cap must hold whatever a client does.
func TestCreatePrunesAndCaps(t *testing.T) {
	s := New(time.Hour)
	now := time.Now()
	s.now = func() time.Time { return now }
	for range 10 {
		s.Create(1)
	}
	now = now.Add(2 * time.Hour)
	s.Create(1)
	if s.Len() != 1 {
		t.Errorf("%d sessions after the expired ones lapsed, want 1", s.Len())
	}
	for range maxSessions + 20 {
		s.Create(1)
	}
	if s.Len() > maxSessions {
		t.Errorf("%d sessions held, want at most %d", s.Len(), maxSessions)
	}
	// The newest login is the one that survives the cap.
	last := s.Create(1)
	if !s.Valid(last, 1) {
		t.Error("the newest session was evicted by the cap")
	}
}

func TestDefaultTTL(t *testing.T) {
	for _, given := range []time.Duration{0, -time.Hour} {
		if got := New(given).TTL(); got != DefaultTTL {
			t.Errorf("New(%v).TTL() = %v, want %v", given, got, DefaultTTL)
		}
	}
	if got := New(time.Minute).TTL(); got != time.Minute {
		t.Errorf("TTL = %v, want a minute", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	s := New(time.Hour)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := s.Create(1)
			for range 8 {
				s.Valid(id, 1)
				s.Valid("nonsense", 1)
			}
		}()
	}
	wg.Wait()
}

func TestDelete(t *testing.T) {
	s := New(time.Hour)
	a, b := s.Create(1), s.Create(1)
	s.Delete(a)
	if s.Valid(a, 1) {
		t.Error("a deleted session is still valid")
	}
	if !s.Valid(b, 1) {
		t.Error("deleting one session ended another")
	}
	if s.Len() != 1 {
		t.Errorf("%d sessions held, want 1", s.Len())
	}
	// Deleting something that was never a session is not an error and
	// does not disturb what is there.
	s.Delete("never-issued")
	s.Delete("")
	if s.Len() != 1 || !s.Valid(b, 1) {
		t.Errorf("deleting an unknown id disturbed the store: %d sessions", s.Len())
	}
}
