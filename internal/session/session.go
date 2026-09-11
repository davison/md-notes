// Package session holds the browser sessions the daemon issues on the
// tailnet host. A browser cannot put the bearer token in a header on every
// request, so it presents it once to a login page and is given a session
// id instead; this is where those ids live.
//
// Sessions are kept in memory and nowhere else. They are worth no more
// than the token they were minted from and rather less — each is tied to
// the token's generation, so `mdn token --rotate` ends every one of them,
// and a daemon restart ends them too. Nothing about a session is written
// to disk, which is deliberate: a second secret at rest would be a second
// secret to protect, and logging in again costs one paste.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"
)

// DefaultTTL is how long a session lasts before the token must be
// presented again. Long, because the daemon serves one person's own notes
// from their own devices and a login that expires over a weekend is a
// login they will stop bothering with; bounded, because a session left
// behind on a borrowed browser should not outlive the reason for it.
const DefaultTTL = 30 * 24 * time.Hour

// maxSessions caps how many live sessions are remembered at once. One
// person with a handful of devices needs a fraction of this; the cap is
// here so that repeated logins cannot grow the map without limit, and it
// evicts the session closest to expiry rather than refusing the new one.
const maxSessions = 64

// Store issues and validates session ids.
type Store struct {
	ttl time.Duration
	// now is the clock, replaced in tests.
	now func() time.Time

	mu   sync.Mutex
	live map[[32]byte]entry
}

type entry struct {
	// generation is the token generation the session was minted from. A
	// session whose generation has moved on was authorised by a secret
	// that no longer exists.
	generation uint64
	expires    time.Time
}

// New returns an empty store whose sessions last ttl. A ttl of zero or
// less asks for DefaultTTL.
func New(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{ttl: ttl, now: time.Now, live: map[[32]byte]entry{}}
}

// TTL is how long a new session lasts, which is also the cookie's Max-Age.
func (s *Store) TTL() time.Duration { return s.ttl }

// Create mints a session for the given token generation and returns the
// id the browser will present. The id is the only copy: the store keeps a
// hash of it, so that neither a memory dump nor a timing difference in the
// lookup hands back anything a client could present.
func (s *Store) Create(generation uint64) string {
	id := rand.Text()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if len(s.live) >= maxSessions {
		s.evictSoonest()
	}
	s.live[key(id)] = entry{generation: generation, expires: s.now().Add(s.ttl)}
	return id
}

// Valid reports whether id names a live session minted from the token
// generation currently in force. A session that fails for any reason is
// forgotten on the spot, so a rotation does not leave dead entries behind.
func (s *Store) Valid(id string, generation uint64) bool {
	if id == "" {
		return false
	}
	k := key(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.live[k]
	if !ok {
		return false
	}
	if e.generation != generation || !s.now().Before(e.expires) {
		delete(s.live, k)
		return false
	}
	return true
}

// Len is how many sessions the store is holding, expired ones included
// until something prunes them. For tests and for nothing else.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.live)
}

// prune drops expired sessions. Called on Create, which is the only
// moment the map can grow; there is no sweeper goroutine to stop.
func (s *Store) prune() {
	now := s.now()
	for k, e := range s.live {
		if !now.Before(e.expires) {
			delete(s.live, k)
		}
	}
}

// evictSoonest removes the session closest to expiry, so that reaching
// the cap logs out the oldest device rather than refusing the newest.
func (s *Store) evictSoonest() {
	var oldest [32]byte
	var at time.Time
	for k, e := range s.live {
		if at.IsZero() || e.expires.Before(at) {
			oldest, at = k, e.expires
		}
	}
	if !at.IsZero() {
		delete(s.live, oldest)
	}
}

func key(id string) [32]byte { return sha256.Sum256([]byte(id)) }
