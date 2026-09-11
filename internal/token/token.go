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
	"time"
)

// FileMode is the permission the token file is created with and repaired
// to. The single-user premise recorded on the first milestone keeps the
// notes private to the user's own account; the token is kept the same way.
const FileMode fs.FileMode = 0o600

// maxBytes bounds what Load will read, so that a file that is not a token
// is reported rather than held in memory.
const maxBytes = 4096

// ErrMalformed is returned when the token file holds something that is
// not a single line of token text.
var ErrMalformed = errors.New("token file does not hold a single token")

// ErrNotRegular is returned when the token file is a symlink, a
// directory, or anything else that is not a regular file. A symlink is
// refused rather than followed because reading the token repairs the
// file's permissions, and that must not reach a file the user did not
// nominate as the token. `mdn token --rotate` replaces such a path with
// a regular file.
var ErrNotRegular = errors.New("the token file must be a regular file; --token-file names another one")

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
	return &Store{path: path, value: value, info: info, gen: 1}, created, nil
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
		return "", nil, fmt.Errorf("%s is %s: %w", path, describe(link.Mode()), ErrNotRegular)
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
		return "", nil, fmt.Errorf("%s changed while it was being read: %w", path, ErrNotRegular)
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
	// The descriptor, not the path: between the SameFile check above and a
	// chmod by name the path could become a symlink, and the repair would
	// follow it to a file the user never nominated.
	tighten(f, info)
	return value, info, nil
}

// describe names what was found where the token file should be, so the
// message says what to fix rather than only that something is wrong.
func describe(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return "a symlink"
	case mode.IsDir():
		return "a directory"
	default:
		return "not a regular file"
	}
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
	sweep(path, tmp)
	return info, nil
}

// staleStaging is how long a staging file must have been lying about
// before a write clears it. A write takes microseconds, so anything this
// old was left by a crash between CreateTemp and Rename — and a unique
// name, unlike the fixed one it replaced, is not reclaimed by the next
// write. Well clear of any rotation in flight, since deleting a live
// staging file would break the rename it is waiting for.
const staleStaging = time.Hour

// sweep removes staging files a crash left behind, so that the state
// directory does not accumulate 0600 files holding valid-looking secrets
// that are not the live token. Best effort: this is tidying, and a
// failure to tidy is not a failure to write.
func sweep(path, keep string) {
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), filepath.Base(path)+".*.tmp"))
	if err != nil {
		return
	}
	for _, m := range matches {
		if m == keep {
			continue
		}
		info, err := os.Lstat(m)
		if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) < staleStaging {
			continue
		}
		os.Remove(m)
	}
}

// tighten repairs a token file left readable by anyone else, rather than
// trusting it to stay private. It takes the open file rather than its
// name so that the repair lands on the descriptor already proved to be
// the token — a name can be replaced between the proof and the chmod. A
// filesystem that refuses is not fatal.
func tighten(f *os.File, info os.FileInfo) {
	if info.Mode().Perm()&^FileMode != 0 {
		f.Chmod(FileMode)
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
	// gen counts the distinct token values this Store has held. It starts
	// at 1 and moves only when a re-read finds a different secret, so
	// anything derived from the token — a login session, in the daemon's
	// case — can be tied to the generation that authorised it and fall
	// with `mdn token --rotate`.
	gen uint64
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

// Generation identifies the token currently held: it changes when, and
// only when, the secret does. A caller holding a credential minted from an
// earlier generation can see that the token it rests on has been rotated
// away without ever seeing the token itself. Like Valid, it notices a
// rotation on the spot, at the cost of the same one stat.
func (s *Store) Generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh()
	return s.gen
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
		if info.Mode().Perm()&^FileMode != 0 {
			// A chmod changes no field sameFile compares, so the repair
			// cannot wait for the next re-read. Going through read gets
			// the lstat, the SameFile check and the descriptor-based
			// repair rather than a chmod by name.
			read(s.path)
		}
		return
	}
	value, from, err := read(s.path)
	if err != nil || value == "" {
		return
	}
	if value != s.value {
		s.gen++
	}
	s.value, s.info = value, from
}

func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.ModTime().Equal(b.ModTime()) && a.Size() == b.Size()
}

func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
