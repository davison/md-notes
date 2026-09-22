// Package roots keeps the registry of folders the daemon serves and is the
// single place a request-supplied path is turned into a filesystem path.
package roots

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Kind distinguishes the configured roots from folders added later.
type Kind string

const (
	// KindNotes is the first configured root: the notes root the clipper
	// writes to and clips_dir resolves against. There is exactly one.
	KindNotes Kind = "notes"
	// KindPermanent is every configured root after the first — another
	// `--root`, or another entry in notes_root's list. Like the notes root
	// it is the daemon's configuration: served from every start, never
	// written to the state file, and not removable.
	KindPermanent Kind = "permanent"
	// KindRecent is a folder registered at runtime with `mdn open` (or
	// the extension), remembered in the state file and removable.
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

// ErrNoNote is returned by AddFor when the file a registration was to be
// verified against is not there to find: missing, not a regular file, or
// resolving outside the directory being registered. One error for the
// three, because the answer to all of them is the same and the caller is
// on loopback asking about a path it already holds — telling the three
// apart would say more about what is outside the folder than it says
// about the folder.
var ErrNoNote = errors.New("no such file inside the directory")

// ErrNotesRoot is returned by Remove for the configured notes root. That
// root is the daemon's configuration — `--root`, or `notes_root` in the
// config file — rather than a registration to undo, and the next start
// would put it straight back.
var ErrNotesRoot = errors.New("the notes root is configured, not registered")

// ErrPermanentRoot is ErrNotesRoot for the configured roots after the
// first: configuration as well, which a removal could not make stick.
var ErrPermanentRoot = errors.New("a permanent root is configured, not registered")

// ErrDuplicateRoot is returned by NewConfigured when two configured paths
// name the same folder, symlinks evaluated. A list someone wrote with a
// folder in it twice is a mistake in that list, and serving it once
// without a word would be the silent half-acceptance a repeated --root
// once got (#162).
var ErrDuplicateRoot = errors.New("names the same folder as an earlier root")

// ErrTooManyLinks is returned by the lexical walk below when a chain of
// symlinks is longer than it will follow. It is deliberately not silence:
// a walk that gave up and answered "does not leave the root" would hand
// the caller the wrong refusal without anyone being able to tell.
var ErrTooManyLinks = errors.New("too many levels of symbolic links")

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

// New builds a registry with notesRoot as its only configured root; see
// NewConfigured.
func New(notesRoot, statePath string, warnf func(format string, args ...any)) (*Registry, error) {
	return NewConfigured([]string{notesRoot}, statePath, warnf)
}

// NewConfigured builds a registry from the configured roots — the first is
// the notes root, the rest permanent roots, in the order given — and
// reloads any recent roots persisted at statePath. Two configured paths
// naming the same folder are ErrDuplicateRoot; one folder inside another
// is allowed, as it is for a folder opened with `mdn open`.
//
// The state file is a cache: a problem reading or rewriting it is reported
// through warnf (which may be nil) and never stops the daemon serving the
// configured roots. Persisted folders that no longer exist are dropped, and
// so is one that is now configured: it is served as configuration, once.
func NewConfigured(configured []string, statePath string, warnf func(format string, args ...any)) (*Registry, error) {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	if len(configured) == 0 {
		return nil, errors.New("no notes root")
	}
	r := &Registry{statePath: statePath}
	for i, path := range configured {
		kind, what := KindPermanent, "root "+path
		if i == 0 {
			kind, what = KindNotes, "notes root"
		}
		root, err := newRoot(path, kind)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		if earlier, ok := r.byPath(root); ok {
			return nil, fmt.Errorf("%s %w (%s)", path, ErrDuplicateRoot, earlier.Path)
		}
		r.roots = append(r.roots, r.withSlug(root))
	}

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

// List returns the roots: the notes root first, then the permanent roots in
// the order they were configured, then recents in the order they were
// added.
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

// Notes returns the permanent notes root. The second result is false only
// for a registry that was never constructed by New.
func (r *Registry) Notes() (Root, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.roots {
		if x.Kind == KindNotes {
			return x, true
		}
	}
	return Root{}, false
}

// Add registers path as a recent root and persists the registry. Adding a
// path that is already registered returns the existing root unchanged.
func (r *Registry) Add(path string) (Root, error) {
	return r.AddFor(path, "")
}

// AddFor is Add with something to confirm first: file names a path inside
// dir — relative to it, or absolute and under it — and the folder is
// registered only if that path resolves inside the folder and is a regular
// file that exists. Otherwise nothing is appended to the registry and
// nothing is written to the state file, and the error is ErrNoNote.
//
// Verification comes before the registry is even consulted, so a folder
// that is already a root is not a way past it, and before the first write,
// so a refusal leaves no trace to undo. That is the difference from
// registering and rolling back:
// [#50](https://github.com/davison/md-notes/issues/50) is a root that
// outlived the file it was registered for, and a rollback has a window in
// which the same thing happens again.
//
// Whether the name is one the daemon serves as a note is not decided here:
// the `.md`/`.markdown` rule is the server's, beside every other place
// that asks that question of a request path.
//
// An empty file is Add: a caller with no particular file in mind, which is
// what `mdn open` is.
func (r *Registry) AddFor(dir, file string) (Root, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	root, err := newRoot(dir, KindRecent)
	if err != nil {
		return Root{}, err
	}
	if file != "" {
		if err := root.hasFile(file); err != nil {
			return Root{}, err
		}
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

// hasFile reports whether file is a regular file inside root, through the
// same confinement every request path goes through: cleaned lexically,
// then resolved with symlinks evaluated and checked against the root's
// real path, so a link out of the folder is not a file inside it.
//
// An absolute file is made relative to the root's own path *lexically*,
// before any symlink is evaluated, so it must be spelled under the path
// this registration named — through an alias of that folder it is refused,
// even where it names a file that is really inside. That is the price of
// one confinement funnel: the relative path goes on to Resolve, which is
// the same function every request path is confined by, rather than this
// growing a second way of deciding what is inside a root. The extension
// sends a base name, and a caller that holds an absolute path holds the
// folder's spelling with it, so nothing in the daemon reaches the refused
// shape. Reported in review of #121.
func (root Root) hasFile(file string) error {
	if filepath.IsAbs(file) {
		rel, err := filepath.Rel(root.Path, filepath.Clean(file))
		if err != nil {
			return ErrNoNote
		}
		file = rel
	}
	real, err := root.Resolve(file)
	if err != nil {
		return ErrNoNote
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return ErrNoNote
	}
	return nil
}

// Remove unregisters the recent root named by slug and rewrites the state
// file without it. It removes nothing from disk: the folder and every file
// in it are left exactly as they are, and a root removed by mistake is
// registered again with `mdn open`.
//
// An unknown slug is os.ErrNotExist, the notes root is ErrNotesRoot and a
// permanent root is ErrPermanentRoot; none changes the registry or the
// state file.
func (r *Registry) Remove(slug string) (Root, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.roots {
		if x.Slug != slug {
			continue
		}
		switch x.Kind {
		case KindNotes:
			return Root{}, ErrNotesRoot
		case KindPermanent:
			return Root{}, ErrPermanentRoot
		}
		kept := make([]Root, 0, len(r.roots)-1)
		kept = append(kept, r.roots[:i]...)
		kept = append(kept, r.roots[i+1:]...)
		was := r.roots
		r.roots = kept
		if err := r.save(); err != nil {
			r.roots = was
			return Root{}, err
		}
		return x, nil
	}
	return Root{}, fmt.Errorf("unknown root %q: %w", slug, os.ErrNotExist)
}

// save writes the recent roots atomically. Caller holds mu. With none left
// the file says so as an empty list, `{"recent": []}`, rather than as null
// (#125).
func (r *Registry) save() error {
	st := state{Recent: []persisted{}}
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

// OpenParent resolves an existing path and opens its parent through a confined
// directory handle. Callers own the returned handle; subsequent operations must
// use it, rather than reopen the absolute path (which can race with symlinks).
// Canonical is suitable for grouping aliases for concurrency control.
func (r *Registry) OpenParent(slug, rel string) (parent *os.Root, name, canonical string, err error) {
	root, ok := r.Get(slug)
	if !ok {
		return nil, "", "", os.ErrNotExist
	}
	canonical, err = root.Resolve(rel)
	if err != nil {
		return nil, "", "", err
	}
	base, err := os.OpenRoot(root.real)
	if err != nil {
		return nil, "", "", err
	}
	defer base.Close()
	relative, err := filepath.Rel(root.real, filepath.Dir(canonical))
	if err != nil {
		return nil, "", "", err
	}
	parent, err = base.OpenRoot(relative)
	return parent, filepath.Base(canonical), canonical, err
}

// Open returns a directory handle confined to the root, through which a
// writer creates files without a path it composes itself ever escaping —
// including through a symlink swapped in mid-operation.
func (root Root) Open() (*os.Root, error) {
	return os.OpenRoot(root.real)
}

// Resolve confines rel to the root. The path is first cleaned lexically so
// that ".." cannot climb above the root, then symlinks are evaluated and
// the real path must still lie within the root's real path. The result is
// the real path, suitable for opening. A path that does not exist returns
// an error wrapping os.ErrNotExist.
func (root Root) Resolve(rel string) (string, error) {
	cleaned, err := root.Relative(rel)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(root.Path, cleaned))
	if err != nil {
		// EvalSymlinks gives up on a chain longer than its own budget
		// with an error it makes itself: no errno to match on and no
		// exported sentinel, so it is recognised here and given a name a
		// caller can test for. What makes that safe to do by text is the
		// shape of the error and not the text: every other error
		// EvalSymlinks returns is an *fs.PathError quoting the path, and
		// a path is a name a reader chose — a note called "too many
		// links.md" would otherwise answer for a chain it is not, and so
		// would every absent name under a root whose own path says it.
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) && strings.Contains(err.Error(), "too many links") {
			return "", ErrTooManyLinks
		}
		return "", err
	}
	if !within(root.real, real) {
		return "", ErrOutside
	}
	return real, nil
}

// Relative is the lexical half of Resolve: it cleans rel and confines it
// to the root without touching the filesystem, returning the path relative
// to the root. It is what a target that does not exist yet can be checked
// with — a note about to be created has no real path to evaluate — and it
// is not confinement on its own: the caller must go on to open the result
// through a handle on the root, so that a symlink cannot make the same
// name mean somewhere else.
func (root Root) Relative(rel string) (string, error) {
	sep := string(filepath.Separator)
	cleaned := filepath.Clean(strings.TrimLeft(filepath.FromSlash(rel), sep))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+sep) {
		return "", ErrOutside
	}
	if !within(root.Path, filepath.Join(root.Path, cleaned)) {
		return "", ErrOutside
	}
	return cleaned, nil
}

// OpenDir opens a handle on the directory rel names inside the root, and
// returns its real path as well. Symlinks are evaluated first, exactly as
// the read and save paths evaluate them, and the handle is then opened on
// the resolved path — so a link to a directory inside the root is
// followed there too, rather than refused by a handle that may not
// traverse an absolute link at all. A component that exists but is not a
// directory is reported as ErrNotDir rather than as a bare ENOTDIR.
func (root Root) OpenDir(rel string) (*os.Root, string, error) {
	canonical, err := root.Resolve(rel)
	if err != nil {
		// A component that is a symlink with no target has no real path
		// for Resolve to call anything but missing. Where its chain
		// leaves the root, the honest answer is the escape it is.
		if errors.Is(err, os.ErrNotExist) {
			escaped, chainErr := root.DanglingEscape(rel)
			if chainErr != nil {
				return nil, "", chainErr
			}
			if escaped {
				return nil, "", ErrOutside
			}
		}
		return nil, "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("%s: %w", rel, ErrNotDir)
	}
	base, err := os.OpenRoot(root.real)
	if err != nil {
		return nil, "", err
	}
	defer base.Close()
	relative, err := filepath.Rel(root.real, canonical)
	if err != nil {
		return nil, "", err
	}
	handle, err := base.OpenRoot(relative)
	if err != nil {
		return nil, "", err
	}
	return handle, canonical, nil
}

// EnsureDir makes dir inside the root, and reports a dir that resolves out
// of it as ErrOutside rather than as whatever a handle happens to say. The
// missing components are made through a handle on the deepest ancestor
// that does exist, so that a link to a directory inside the root is
// followed here as it is everywhere else; that resolved ancestor is the
// enforcement, and the walk up to it is the diagnosis.
func (root Root) EnsureDir(dir string) error {
	ancestor := dir
	for {
		_, err := root.Resolve(ancestor)
		if err == nil {
			break
		}
		// Only leaving the root is decided here. Every other reason a
		// component will not resolve — missing, or a file where a
		// directory was expected — is a question for the ancestor above
		// it, and OpenDir and MkdirAll below report what they find.
		if errors.Is(err, ErrOutside) {
			return err
		}
		// A component that is a symlink with no target is not a directory
		// to make: where its chain leaves the root, the walk must stop
		// here rather than climb past it and hand the name to MkdirAll.
		if errors.Is(err, os.ErrNotExist) {
			escaped, chainErr := root.DanglingEscape(ancestor)
			if chainErr != nil {
				return chainErr
			}
			if escaped {
				return ErrOutside
			}
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			ancestor = "."
			break
		}
		ancestor = parent
	}
	if ancestor == dir {
		return nil
	}
	handle, _, err := root.OpenDir(ancestor)
	if err != nil {
		return err
	}
	defer handle.Close()
	rest, err := filepath.Rel(ancestor, dir)
	if err != nil {
		return err
	}
	if err := handle.MkdirAll(rest, 0o755); err != nil {
		return err
	}
	// A symlink raced in under the new directory is still outside.
	_, err = root.Resolve(dir)
	return err
}

// Escapes reports whether a symlink read out of realDir, with the target
// it names, points out of the root. It answers lexically, because the
// link a caller asks about may be dangling and so have no real path to
// evaluate at all. It is diagnosis for an operation being refused either
// way, never the decision to refuse one.
func (root Root) Escapes(realDir, target string) bool {
	if !filepath.IsAbs(target) {
		target = filepath.Join(realDir, target)
	}
	return !within(root.real, filepath.Clean(target))
}

// maxLinkHops is as many links as the resolver behind Resolve will itself
// follow. filepath.EvalSymlinks walks a chain hop by hop in user space,
// with a budget of 255, so a *dangling* chain of up to 255 links is
// reported as missing — which is what sends a caller to the walk below —
// and a chain one link longer is refused by EvalSymlinks before the walk
// is ever consulted. The kernel's own budget of 40 (ELOOP at the 41st)
// never comes into a dangling chain at all, because EvalSymlinks never
// asks it to resolve more than one hop at a time; measured on this
// machine, a dangling chain of 255 links answers ENOENT and one of 256
// answers "too many links". Following the same 255 means every chain that
// can reach the walk is answered by it, and a circular one still ends.
const maxLinkHops = 255

// EscapesChain reports whether the symlink named base inside realDir leads
// out of the root. Escapes answers for the one target a link names;
// this follows the chain of them, because a first hop that stays inside
// the root can still name a second that does not. Each hop is read
// through a handle opened here on the root, rather than through the
// caller's handle on the note's own parent directory: a chain's second
// hop may name anything anywhere in the root, which a handle on one
// directory cannot read. That handle confines the walk exactly as the
// caller's does — (*os.Root).Readlink refuses a name that leaves the root
// — and the walk only ever picks which refusal an already-refused
// operation is given, so a link swapped in between the two handles costs
// the caller a different error code and nothing else. Like Escapes it is
// lexical: the links it is asked about are the ones with no target for
// Resolve to evaluate. ErrTooManyLinks says the chain is longer than the
// walk will follow, so that giving up is never mistaken for an answer.
func (root Root) EscapesChain(realDir, base string) (bool, error) {
	handle, err := os.OpenRoot(root.real)
	if err != nil {
		return false, err
	}
	defer handle.Close()
	_, escaped, err := root.followLinks(handle, filepath.Join(realDir, base))
	return escaped, err
}

// DanglingEscape asks the same question of a whole path, component by
// component: the link with no target may be a directory along the way, not
// the last name. It answers false for every path that does not contain
// such a link — including one that is merely missing — so a caller reaches
// for it only once Resolve has already refused the path.
func (root Root) DanglingEscape(rel string) (bool, error) {
	cleaned, err := root.Relative(rel)
	if err != nil {
		return false, nil
	}
	handle, err := os.OpenRoot(root.real)
	if err != nil {
		return false, err
	}
	defer handle.Close()
	at := root.real
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		next := filepath.Join(at, part)
		if !within(root.real, next) {
			return false, nil
		}
		end, escaped, err := root.followLinks(handle, next)
		if escaped || err != nil {
			return escaped, err
		}
		at = end
	}
	return false, nil
}

// followLinks walks the chain of symlinks starting at abs, an absolute
// path inside the root, and returns the path it ends at — abs itself when
// it is not a link, or not one a handle on the root will read. escaped is
// true as soon as a hop points out of the root, and the walk stops there.
// A chain longer than maxLinkHops ends the walk with ErrTooManyLinks: no
// path that reaches here can be that long, because the resolver that sent
// the caller here follows the same number and refuses anything longer, but
// a bound that is reached in silence is a wrong answer waiting to happen,
// and it is what ends a circular chain.
func (root Root) followLinks(handle *os.Root, abs string) (end string, escaped bool, err error) {
	for hop := 0; hop < maxLinkHops; hop++ {
		name, err := filepath.Rel(root.real, abs)
		if err != nil {
			return abs, false, nil
		}
		target, err := handle.Readlink(name)
		if err != nil {
			return abs, false, nil
		}
		if root.Escapes(filepath.Dir(abs), target) {
			return abs, true, nil
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(abs), target)
		}
		abs = filepath.Clean(target)
	}
	return abs, false, ErrTooManyLinks
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
