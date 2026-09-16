package source

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/roots"
)

var (
	// ErrExists is returned when the name a new note was asked for is
	// already taken — by a file, a directory or a link. Creating never
	// replaces anything, so this is the answer for all three.
	ErrExists = errors.New("a file of that name already exists")
	// ErrName is returned for a name no note can have: empty, hidden, or
	// carrying a control character.
	ErrName = errors.New("invalid note name")
)

// Created is a new note together with the path it was actually given,
// relative to the root and slash-separated. That is the requested path
// cleaned — "sub/../a.md" becomes "a.md" — so a client has the name to
// read, save and route to without repeating the cleaning rule.
type Created struct {
	Note
	Path string `json:"path"`
}

// Create writes a new note at rel under slug and returns it with the
// revision a first save will be checked against. It never replaces an
// existing file: the name is opened O_CREATE|O_EXCL through a handle on
// the root, so a file, directory or link already at that name is refused
// rather than overwritten. Missing parent directories are created.
func (s *Store) Create(slug, rel, text string) (Created, error) {
	if len(text) > MaxBytes {
		return Created{}, ErrTooLarge
	}
	if !utf8.ValidString(text) {
		return Created{}, ErrEncoding
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ok := s.reg.Get(slug)
	if !ok {
		return Created{}, os.ErrNotExist
	}
	name, err := checkedName(root, rel)
	if err != nil {
		return Created{}, err
	}
	handle, err := root.Open()
	if err != nil {
		return Created{}, err
	}
	defer handle.Close()
	if dir := filepath.Dir(name); dir != "." {
		if err := root.EnsureDir(handle, dir); err != nil {
			return Created{}, err
		}
	}
	// Lstat first so that a name taken by a link — the one case where
	// O_EXCL's answer would be true but uninformative — is reported as
	// the taken name it is, and an escaping link is reported as an
	// escape. O_EXCL below is what actually decides it.
	if _, err := handle.Lstat(name); err == nil {
		if _, err := root.Resolve(name); errors.Is(err, roots.ErrOutside) {
			return Created{}, roots.ErrOutside
		}
		return Created{}, ErrExists
	}
	f, err := handle.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return Created{}, ErrExists
	}
	if err != nil {
		return Created{}, err
	}
	if err := writeAll(f, []byte(text)); err != nil {
		handle.Remove(name)
		return Created{}, err
	}
	// Read the note back through the path a save will take, so the
	// revision returned here is the one that save will compare against.
	dir, base, canonical, err := s.reg.OpenParent(slug, name)
	if err != nil {
		return Created{}, err
	}
	defer dir.Close()
	data, snap, err := read(dir, base)
	if err != nil {
		return Created{}, err
	}
	return Created{
		Note: Note{Source: string(data), Revision: s.remember(canonical, snap)},
		Path: filepath.ToSlash(name),
	}, nil
}

// Delete removes one markdown file inside the root, and nothing else.
// The name is resolved through a handle opened on the root, so no
// component can leave it; Lstat rather than Stat means a symlink is
// refused rather than followed, so the file that goes is always the file
// the caller named. The only call here that changes the filesystem is
// Remove, on that one name — never RemoveAll, and never a directory.
func (s *Store) Delete(slug, rel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ok := s.reg.Get(slug)
	if !ok {
		return os.ErrNotExist
	}
	name, err := checkedName(root, rel)
	if err != nil {
		return err
	}
	// Resolving the parent first is diagnosis, not enforcement — the
	// handle below is the enforcement — but it is what lets a folder that
	// is a link out of the root be answered as the escape it is rather
	// than as an unexplained I/O error.
	if dir := filepath.Dir(name); dir != "." {
		if _, err := root.Resolve(dir); err != nil {
			return err
		}
	}
	handle, err := root.Open()
	if err != nil {
		return err
	}
	defer handle.Close()
	info, err := handle.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		// A directory, a device, or a link. A link that leaves the root
		// is reported as an escape, the way the read and save paths
		// report one; a link inside the root is refused as well, because
		// the name the caller confirmed and the file that would go are
		// not the same file.
		if _, err := root.Resolve(name); errors.Is(err, roots.ErrOutside) {
			return roots.ErrOutside
		}
		return ErrNotRegular
	}
	// Removing needs write permission on the directory, not on the file,
	// so a read-only note would otherwise go without the save path's
	// refusal ever applying to it.
	if info.Mode().Perm()&0o222 == 0 {
		return os.ErrPermission
	}
	// An uncoordinated local writer could still swap the name for
	// something else between the check above and this call, exactly as it
	// could between the save path's verify and its rename; nothing here
	// widens what such a writer can already do to the root by hand.
	return handle.Remove(name)
}

// checkedName confines rel to the root lexically and refuses the names a
// note cannot have. It is the same rule for creating and for deleting, so
// that a name the UI may create is a name it may remove.
func checkedName(root roots.Root, rel string) (string, error) {
	name, err := root.Relative(rel)
	if err != nil {
		return "", err
	}
	if name == "." || name == string(filepath.Separator) {
		return "", ErrName
	}
	// A hidden file or folder is not listed by the navigator — ripgrep
	// skips dotfiles — so a note created under one would be invisible in
	// the app that created it, and a write into a directory like .git
	// would be a surprise. Every component is checked, not only the last;
	// this also refuses a file called ".md".
	for _, part := range strings.Split(name, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return "", ErrName
		}
	}
	if strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", ErrName
	}
	return name, nil
}

// writeAll writes the note and makes its bytes durable before reporting
// success, the way the save path stages its replacement: a 201 the caller
// acts on should not name a note that a power cut leaves empty.
func writeAll(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
