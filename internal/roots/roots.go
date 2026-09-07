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
	Recent []string `json:"recent"`
}

// New builds a registry with notesRoot as its permanent root and reloads
// any recent roots persisted at statePath. Persisted folders that no longer
// exist are dropped silently.
func New(notesRoot, statePath string) (*Registry, error) {
	r := &Registry{statePath: statePath}
	notes, err := newRoot(notesRoot, KindNotes)
	if err != nil {
		return nil, fmt.Errorf("notes root: %w", err)
	}
	r.roots = append(r.roots, r.withSlug(notes))

	data, err := os.ReadFile(statePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(data) > 0 {
		var st state
		if err := json.Unmarshal(data, &st); err != nil {
			return nil, fmt.Errorf("%s: %w", statePath, err)
		}
		for _, p := range st.Recent {
			if _, ok := r.byPath(p); ok {
				continue
			}
			root, err := newRoot(p, KindRecent)
			if err != nil {
				continue
			}
			r.roots = append(r.roots, r.withSlug(root))
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

func (r *Registry) byPath(path string) (Root, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Root{}, false
	}
	for _, x := range r.roots {
		if x.Path == abs {
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
	if existing, ok := r.byPath(path); ok {
		return existing, nil
	}
	root, err := newRoot(path, KindRecent)
	if err != nil {
		return Root{}, err
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
			st.Recent = append(st.Recent, x.Path)
		}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.statePath), 0o755); err != nil {
		return err
	}
	tmp := r.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
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

func within(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
