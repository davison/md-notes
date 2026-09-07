// Package roots keeps the registry of folders the daemon serves and is the
// single place a request-supplied path is turned into a filesystem path.
package roots

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Kind distinguishes the permanent notes root from folders added later.
type Kind string

const (
	KindNotes  Kind = "notes"
	KindRecent Kind = "recent"
)

// Root is a served folder. Path is absolute and Real is Path with symlinks
// evaluated, which is what confinement compares against.
type Root struct {
	Slug string `json:"slug"`
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
	real string
}

// ErrOutside is returned when a path resolves outside its root.
var ErrOutside = errors.New("path is outside the root")

// ErrNotDir is returned when a registered path is not a directory.
var ErrNotDir = errors.New("not a directory")

// Registry holds the roots. It is safe for concurrent use.
type Registry struct {
	mu        sync.Mutex
	statePath string
	roots     []Root
}

type state struct {
	Recent []persisted `json:"recent"`
}

// persisted is one recent root on disk. The slug is stored so that URLs
// survive a restart; without it a later root could inherit an earlier one's
// slug and silently point at a different folder.
type persisted struct {
	Slug string `json:"slug"`
	Path string `json:"path"`
}

// New builds a registry with notesRoot as its permanent root and reloads
// any recent roots persisted at statePath. The state file is a cache: a
// problem reading or rewriting it is reported through warnf (which may be
// nil) and never stops the daemon serving the notes root. Persisted
// folders that no longer exist are dropped.
func New(notesRoot, statePath string, warnf func(format string, args ...any)) (*Registry, error) {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	r := &Registry{statePath: statePath}
	notes, err := newRoot(notesRoot, KindNotes)
	if err != nil {
		return nil, fmt.Errorf("notes root: %w", err)
	}
	r.roots = append(r.roots, r.withSlug(notes))

	data, err := os.ReadFile(statePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		warnf("ignoring recent roots: %v", err)
		return r, nil
	}
	if len(data) > 0 {
		var st state
		if err := json.Unmarshal(data, &st); err != nil {
			warnf("ignoring recent roots: %s: %v", statePath, err)
			return r, nil
		}
		dropped := false
		for _, p := range st.Recent {
			root, err := newRoot(p.Path, KindRecent)
			if err != nil {
				dropped = true
				continue
			}
			if _, ok := r.byPath(root); ok {
				dropped = true
				continue
			}
			root.Slug = p.Slug
			if root.Slug == "" || r.hasSlug(root.Slug) {
				root = r.withSlug(root)
			}
			r.roots = append(r.roots, root)
		}
		if dropped {
			if err := r.save(); err != nil {
				warnf("could not rewrite recent roots: %v", err)
			}
		}
	}
	return r, nil
}

func newRoot(path string, kind Kind) (Root, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Root{}, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Root{}, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return Root{}, err
	}
	if !info.IsDir() {
		return Root{}, fmt.Errorf("%s: %w", abs, ErrNotDir)
	}
	return Root{Path: abs, Kind: kind, real: real}, nil
}

var unsafeSlug = regexp.MustCompile(`[^a-z0-9._-]+`)

// slugify derives a URL-safe slug from the folder's basename.
func slugify(path string) string {
	s := strings.ToLower(filepath.Base(path))
	s = unsafeSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return "root"
	}
	return s
}

// withSlug assigns the first free slug for root. Caller holds mu or is
// still constructing.
func (r *Registry) withSlug(root Root) Root {
	base := slugify(root.Path)
	slug := base
	for n := 2; r.hasSlug(slug); n++ {
		slug = fmt.Sprintf("%s-%d", base, n)
	}
	root.Slug = slug
	return root
}

func (r *Registry) hasSlug(slug string) bool {
	for _, x := range r.roots {
		if x.Slug == slug {
			return true
		}
	}
	return false
}

// byPath finds a registered root for the same folder as candidate, comparing
// real paths so a symlink alias of a registered folder is not registered
// twice.
func (r *Registry) byPath(candidate Root) (Root, bool) {
	for _, x := range r.roots {
		if x.real == candidate.real {
			return x, true
		}
	}
	return Root{}, false
}

// List returns the roots, the notes root first, then recents in the order
// they were added.
func (r *Registry) List() []Root {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Root, len(r.roots))
	copy(out, r.roots)
	return out
}

// Get looks a root up by slug.
func (r *Registry) Get(slug string) (Root, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.roots {
		if x.Slug == slug {
			return x, true
		}
	}
	return Root{}, false
}

// Add registers path as a recent root and persists the registry. Adding a
// path that is already registered returns the existing root unchanged.
func (r *Registry) Add(path string) (Root, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	root, err := newRoot(path, KindRecent)
	if err != nil {
		return Root{}, err
	}
	if existing, ok := r.byPath(root); ok {
		return existing, nil
	}
	root = r.withSlug(root)
	r.roots = append(r.roots, root)
	if err := r.save(); err != nil {
		r.roots = r.roots[:len(r.roots)-1]
		return Root{}, err
	}
	return root, nil
}

// save writes the recent roots atomically. Caller holds mu.
func (r *Registry) save() error {
	var st state
	for _, x := range r.roots {
		if x.Kind == KindRecent {
			st.Recent = append(st.Recent, persisted{Slug: x.Slug, Path: x.Path})
		}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.statePath), 0o700); err != nil {
		return err
	}
	tmp := r.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.statePath)
}

// Resolve turns a request path relative to the root named by slug into a
// real filesystem path, or returns ErrOutside, os.ErrNotExist, or an error
// for an unknown slug.
func (r *Registry) Resolve(slug, rel string) (string, error) {
	root, ok := r.Get(slug)
	if !ok {
		return "", fmt.Errorf("unknown root %q", slug)
	}
	return root.Resolve(rel)
}

// Resolve confines rel to the root. The path is first cleaned lexically so
// that ".." cannot climb above the root, then symlinks are evaluated and
// the real path must still lie within the root's real path. The result is
// the real path, suitable for opening. A path that does not exist returns
// an error wrapping os.ErrNotExist.
func (root Root) Resolve(rel string) (string, error) {
	sep := string(filepath.Separator)
	cleaned := filepath.Clean(strings.TrimLeft(filepath.FromSlash(rel), sep))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+sep) {
		return "", ErrOutside
	}
	lexical := filepath.Join(root.Path, cleaned)
	if !within(root.Path, lexical) {
		return "", ErrOutside
	}
	real, err := filepath.EvalSymlinks(lexical)
	if err != nil {
		return "", err
	}
	if !within(root.real, real) {
		return "", ErrOutside
	}
	return real, nil
}

// within reports whether path is root or lies beneath it. Both must be
// clean absolute paths. The filesystem root is a special case because
// appending a separator to it would give "//".
func within(root, path string) bool {
	sep := string(filepath.Separator)
	if root == sep {
		return strings.HasPrefix(path, sep)
	}
	return path == root || strings.HasPrefix(path, root+sep)
}
