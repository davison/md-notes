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
// a single line of token text, or is not a regular file. A symlink is
// refused rather than followed: reading the token repairs the file's
// permissions, and that must not reach a file the user did not nominate.
var ErrMalformed = errors.New("token file does not hold a single token")

// Load returns the token stored at path, generating and storing one when
// the file is absent or empty. The second result says whether this call
// created it, which the daemon logs once on first start.
func Load(path string) (string, bool, error) {
	value, _, created, err := load(path)
	return value, created, err
}

// Open is Load for the daemon: it returns a Store holding both the token
// and the identity of the file it came from. Reading the value and
// stamping the file must be one operation — a rotation landing between
// them would leave the daemon serving the superseded token and refusing
// the current one, since the file it holds would already look current.
func Open(path string) (*Store, bool, error) {
	value, info, created, err := load(path)
	if err != nil {
		return nil, false, err
	}
	return &Store{path: path, value: value, info: info}, created, nil
}

func load(path string) (string, os.FileInfo, bool, error) {
	value, info, err := read(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", nil, false, err
	}
	if value != "" {
		return value, info, false, nil
	}
	value, info, err = generate(path)
	return value, info, err == nil, err
}

// Rotate replaces the token at path with a new one and returns it. A
// running daemon picks the new value up on its next authenticated
// request; see Store.Valid. A path that is a symlink is replaced by the
// new regular file rather than written through.
func Rotate(path string) (string, error) {
	value, _, err := generate(path)
	return value, err
}

func generate(path string) (string, os.FileInfo, error) {
	value := rand.Text()
	info, err := write(path, value)
	if err != nil {
		return "", nil, err
	}
	return value, info, nil
}

// read returns the token text at path, with the identity of the file it
// came from, or the empty string when the file holds only whitespace.
func read(path string) (string, os.FileInfo, error) {
	// Lstat first: os.Open would follow a symlink, and the permission
	// repair below would then chmod its target — a file the user never
	// nominated as the token.
	link, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !link.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", nil, err
	}
	// And the file opened must be the one that was not a link, in case the
	// two raced.
	if !info.Mode().IsRegular() || !os.SameFile(link, info) {
		return "", nil, fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > maxBytes {
		return "", nil, fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	value := strings.TrimSpace(string(data))
	if strings.ContainsAny(value, "\n\r") {
		return "", nil, fmt.Errorf("%s: %w", path, ErrMalformed)
	}
	tighten(path, info)
	return value, info, nil
}

// write stores value at path with mode 0600, through a temporary file in
// the same directory so that a reader never sees a half-written token,
// and returns the identity of the file now at path. The rename replaces
// whatever was there, a symlink included, rather than writing through it.
// The name is unique so that two rotations at once cannot share a staging
// file.
func write(path, value string) (os.FileInfo, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.WriteString(value + "\n"); err != nil {
		f.Close()
		return nil, err
	}
	// CreateTemp already makes the file 0600, but say so rather than rely
	// on it; the umask does not apply to Chmod either way.
	if err := f.Chmod(FileMode); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	// The rename preserves the inode, so what was stat-ed here identifies
	// the file that ends up at path.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return info, nil
}

// tighten repairs a token file left readable by anyone else, rather than
// trusting it to stay private. A filesystem that refuses is not fatal.
func tighten(path string, info os.FileInfo) {
	if info.Mode().Perm()&^FileMode != 0 {
		os.Chmod(path, FileMode)
	}
}

// Store answers whether a presented token is the daemon's own.
type Store struct {
	path string

	mu    sync.Mutex
	value string
	// info identifies the file the held value was read from, so that a
	// rotation is noticed without reading the file on every request.
	info os.FileInfo
}

// Valid reports whether presented is the current token, comparing in
// constant time.
func (s *Store) Valid(presented string) bool {
	if presented == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return equal(presented, s.value)
}

// refresh re-reads the token when the file is no longer the one the held
// value came from, so `mdn token --rotate` both admits the new token and
// refuses the old one on the very next request, with no restart. Rotation
// replaces the file by rename, so the change is a different inode and
// never a matter of timestamp resolution; an unchanged file costs one
// stat, and the permission repair rides on that stat rather than waiting
// for the next read, since a chmod changes no field the comparison sees.
// A file that cannot be read leaves the held value standing: a state
// directory that went missing is not a revocation, and treating it as one
// would lock the user out of their own daemon.
func (s *Store) refresh() {
	info, err := os.Stat(s.path)
	if err != nil {
		return
	}
	if s.info != nil && sameFile(s.info, info) {
		tighten(s.path, info)
		return
	}
	value, from, err := read(s.path)
	if err != nil || value == "" {
		return
	}
	s.value, s.info = value, from
}

func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.ModTime().Equal(b.ModTime()) && a.Size() == b.Size()
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
