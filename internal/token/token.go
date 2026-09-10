// Package token holds the daemon's bearer token: one secret per
// installation, generated on first start and stored beside the state file
// at mode 0600, so that a client which is not the loopback web UI — the
// browser extension, or a browser reaching the daemon over the tailnet —
// can prove it is the user.
package token

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileMode is the permission the token file is created with and repaired
// to. The single-user premise recorded on the first milestone keeps the
// notes private to the user's own account; the token is kept the same way.
const FileMode fs.FileMode = 0o600

// maxBytes bounds what Load will read, so that a file that is not a token
// is reported rather than held in memory.
const maxBytes = 4096

// ErrMalformed is returned when the token file holds something that is not
// a single line of token text.
var ErrMalformed = errors.New("token file does not hold a single token")

// Load returns the token stored at path, generating and storing one when
// the file is absent or empty. The second result says whether this call
// created it, which the daemon logs once on first start.
func Load(path string) (string, bool, error) {
	value, err := read(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}
	if value != "" {
		return value, false, nil
	}
	value, err = generate(path)
	return value, err == nil, err
}

// Rotate replaces the token at path with a new one and returns it. A
// running daemon picks the new value up on the next request that presents
// it; see Store.Valid.
func Rotate(path string) (string, error) {
	return generate(path)
}

func generate(path string) (string, error) {
	value := rand.Text()
	if err := write(path, value); err != nil {
		return "", err
	}
	return value, nil
}

// read returns the token text at path, or the empty string when the file
// holds only whitespace.
func read(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxBytes {
		return "", fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	value := strings.TrimSpace(string(data))
	if strings.ContainsAny(value, "\n\r") {
		return "", fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	// A secret found readable by anyone else is tightened rather than
	// trusted to stay private; a filesystem that refuses is not fatal.
	if info.Mode().Perm()&^FileMode != 0 {
		os.Chmod(path, FileMode)
	}
	return value, nil
}

// write stores value at path with mode 0600, through a temporary file in
// the same directory so that a reader never sees a half-written token.
func write(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FileMode)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err := f.WriteString(value + "\n"); err != nil {
		f.Close()
		return err
	}
	// The umask does not apply to Chmod, so a strict mode is certain even
	// where the create was widened.
	if err := f.Chmod(FileMode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Store answers whether a presented token is the daemon's own.
type Store struct {
	path string

	mu    sync.Mutex
	value string
}

// NewStore holds value as the current token and path as where a rotated
// one will appear.
func NewStore(path, value string) *Store {
	return &Store{path: path, value: value}
}

// Valid reports whether presented is the current token, comparing in
// constant time. A value that does not match the one held is checked again
// against the file, so that `mdn token --rotate` takes effect on the next
// request that presents the new token without restarting the daemon; a
// matching request does no I/O at all.
func (s *Store) Valid(presented string) bool {
	if presented == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if equal(presented, s.value) {
		return true
	}
	latest, err := read(s.path)
	if err != nil || latest == "" || latest == s.value {
		return false
	}
	s.value = latest
	return equal(presented, latest)
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
