// Package source reads and conditionally replaces existing markdown source.
package source

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"sync"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/roots"
)

const MaxBytes = 8 << 20

var (
	ErrConflict   = errors.New("note changed; reload before saving")
	ErrTooLarge   = errors.New("source exceeds 8 MiB")
	ErrEncoding   = errors.New("source must be valid UTF-8")
	ErrNotRegular = errors.New("not a regular file")
)

type Note struct {
	Source   string `json:"source"`
	Revision string `json:"revision"`
}

type snapshot struct {
	info   os.FileInfo
	digest [sha256.Size]byte
}

func (a snapshot) same(b snapshot) bool {
	return os.SameFile(a.info, b.info) && a.info.ModTime() == b.info.ModTime() &&
		a.info.Mode() == b.info.Mode() && a.info.Size() == b.info.Size() && a.digest == b.digest
}

type version struct {
	snapshot
	token string
}

// Store serializes source operations, including saves through root/path aliases.
// Versions are session-local: a daemon restart safely rejects old revisions.
type Store struct {
	mu       sync.Mutex
	reg      *roots.Registry
	versions map[string]version
}

func New(reg *roots.Registry) *Store {
	return &Store{reg: reg, versions: make(map[string]version)}
}

func (s *Store) remember(path string, snap snapshot) string {
	v, ok := s.versions[path]
	if !ok || !v.snapshot.same(snap) {
		v = version{snapshot: snap, token: rand.Text()}
		s.versions[path] = v
	}
	return v.token
}

func (s *Store) Read(slug, path string) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, name, canonical, err := s.reg.OpenParent(slug, path)
	if err != nil {
		return Note{}, err
	}
	defer dir.Close()
	data, snap, err := read(dir, name)
	if err != nil {
		return Note{}, err
	}
	return Note{Source: string(data), Revision: s.remember(canonical, snap)}, nil
}

func (s *Store) Save(slug, path, revision, text string) (Note, error) {
	if len(text) > MaxBytes {
		return Note{}, ErrTooLarge
	}
	if !utf8.ValidString(text) {
		return Note{}, ErrEncoding
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, name, canonical, err := s.reg.OpenParent(slug, path)
	if err != nil {
		return Note{}, err
	}
	defer dir.Close()
	_, before, err := read(dir, name)
	if err != nil {
		return Note{}, err
	}
	if revision == "" || s.remember(canonical, before) != revision {
		return Note{}, ErrConflict
	}
	// Rename needs directory permission, but must not bypass a read-only note.
	if before.info.Mode().Perm()&0o222 == 0 {
		return Note{}, os.ErrPermission
	}
	writable, err := dir.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		return Note{}, err
	}
	err = writable.Close()
	if err != nil {
		return Note{}, err
	}

	after, err := replace(dir, name, []byte(text), before.info.Mode().Perm(), func() error {
		// Re-resolve the original URL as well: a renamed parent or retargeted
		// alias must not make this save silently update an abandoned file.
		currentDir, currentName, currentPath, err := s.reg.OpenParent(slug, path)
		if err != nil {
			return err
		}
		defer currentDir.Close()
		if currentPath != canonical {
			return ErrConflict
		}
		_, current, err := read(currentDir, currentName)
		if err != nil {
			return err
		}
		if !before.same(current) {
			return ErrConflict
		}
		return nil
	})
	if err != nil {
		return Note{}, err
	}
	return Note{Source: text, Revision: s.remember(canonical, after)}, nil
}

func read(dir *os.Root, name string) ([]byte, snapshot, error) {
	info, err := dir.Stat(name)
	if err != nil {
		return nil, snapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, snapshot{}, ErrNotRegular
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, snapshot{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, snapshot{}, err
	}
	if !before.Mode().IsRegular() {
		return nil, snapshot{}, ErrNotRegular
	}
	if !os.SameFile(info, before) {
		return nil, snapshot{}, ErrConflict
	}
	if before.Size() > MaxBytes {
		return nil, snapshot{}, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return nil, snapshot{}, err
	}
	if len(data) > MaxBytes {
		return nil, snapshot{}, ErrTooLarge
	}
	if !utf8.Valid(data) {
		return nil, snapshot{}, ErrEncoding
	}
	after, err := f.Stat()
	if err != nil {
		return nil, snapshot{}, err
	}
	current, err := dir.Stat(name)
	if err != nil {
		return nil, snapshot{}, err
	}
	digest := sha256.Sum256(data)
	snap := snapshot{before, digest}
	if !snap.same(snapshot{after, digest}) || !snap.same(snapshot{current, digest}) {
		return nil, snapshot{}, ErrConflict
	}
	return data, snap, nil
}

// replace stages durable bytes, then checks the current revision immediately
// before rename. Uncoordinated external writers can still race after verify;
// ordinary filesystems offer no conditional rename against such writers.
func replace(dir *os.Root, name string, data []byte, mode os.FileMode, verify func() error) (snapshot, error) {
	tmp := ".mdn-save-" + rand.Text()
	f, err := dir.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return snapshot{}, err
	}
	defer dir.Remove(tmp)
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return snapshot{}, err
	}
	if err := f.Chmod(mode); err != nil {
		return snapshot{}, err
	}
	if err := f.Sync(); err != nil {
		return snapshot{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return snapshot{}, err
	}
	if err := f.Close(); err != nil {
		return snapshot{}, err
	}
	if err := verify(); err != nil {
		return snapshot{}, err
	}
	if err := dir.Rename(tmp, name); err != nil {
		return snapshot{}, err
	}
	return snapshot{info: info, digest: sha256.Sum256(data)}, nil
}
