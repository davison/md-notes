package roots

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestRegistry(t *testing.T) (*Registry, string, string) {
	t.Helper()
	notes := filepath.Join(t.TempDir(), "My Notes")
	if err := os.Mkdir(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state", "roots.json")
	r, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r, notes, statePath
}

func TestNewRegistersNotesRootFirst(t *testing.T) {
	r, notes, _ := newTestRegistry(t)
	got := r.List()
	if len(got) != 1 || got[0].Path != notes || got[0].Kind != KindNotes || got[0].Slug != "my-notes" {
		t.Fatalf("List() = %+v", got)
	}
}

func TestNewRejectsFileOrMissingNotesRoot(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	os.WriteFile(f, nil, 0o644)
	if _, err := New(f, filepath.Join(t.TempDir(), "s.json"), nil); !errors.Is(err, ErrNotDir) {
		t.Fatalf("err = %v, want ErrNotDir", err)
	}
	if _, err := New(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "s.json"), nil); err == nil {
		t.Fatal("want error for missing notes root")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"/a/My Notes":   "my-notes",
		"/a/md-notes":   "md-notes",
		"/a/.hidden":    "hidden",
		"/a/Ünïcode!":   "n-code",
		"/a/---":        "root",
		"/a/proj_v2.1":  "proj_v2.1",
		"/a/spaces  x ": "spaces-x",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddDeduplicatesSlugsAndPaths(t *testing.T) {
	r, _, _ := newTestRegistry(t)
	a := filepath.Join(t.TempDir(), "docs")
	b := filepath.Join(t.TempDir(), "docs")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)

	ra, err := r.Add(a)
	if err != nil || ra.Slug != "docs" || ra.Kind != KindRecent {
		t.Fatalf("Add(a) = %+v, %v", ra, err)
	}
	rb, err := r.Add(b)
	if err != nil || rb.Slug != "docs-2" {
		t.Fatalf("Add(b) = %+v, %v", rb, err)
	}
	again, err := r.Add(a)
	if err != nil || again != ra {
		t.Fatalf("Add(a) again = %+v, %v; want %+v", again, err, ra)
	}
	if got := r.List(); len(got) != 3 {
		t.Fatalf("List() has %d roots, want 3", len(got))
	}
}

func TestAddRejectsFile(t *testing.T) {
	r, _, _ := newTestRegistry(t)
	f := filepath.Join(t.TempDir(), "f.md")
	os.WriteFile(f, nil, 0o644)
	if _, err := r.Add(f); !errors.Is(err, ErrNotDir) {
		t.Fatalf("err = %v, want ErrNotDir", err)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	r, notes, statePath := newTestRegistry(t)
	kept := filepath.Join(t.TempDir(), "kept")
	gone := filepath.Join(t.TempDir(), "gone")
	os.Mkdir(kept, 0o755)
	os.Mkdir(gone, 0o755)
	r.Add(kept)
	r.Add(gone)
	os.RemoveAll(gone)

	r2, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := r2.List()
	if len(got) != 2 || got[0].Kind != KindNotes || got[1].Path != kept || got[1].Slug != "kept" {
		t.Fatalf("after reload List() = %+v", got)
	}
	// Loading does not write: the gone folder leaves the file only when the
	// daemon, up and serving, settles it.
	before, _ := os.ReadFile(statePath)
	if !strings.Contains(string(before), gone) {
		t.Fatalf("loading rewrote the state file: %s", before)
	}
	if err := r2.SettleState(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(statePath)
	if strings.Contains(string(data), gone) || !strings.Contains(string(data), kept) {
		t.Fatalf("after SettleState the state file is %s", data)
	}
}

func TestPersistenceKeepsSlugs(t *testing.T) {
	r, notes, statePath := newTestRegistry(t)
	a := filepath.Join(t.TempDir(), "docs")
	b := filepath.Join(t.TempDir(), "docs")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)
	r.Add(a)
	r.Add(b)
	os.RemoveAll(a)

	r2, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r2.Get("docs-2")
	if !ok || got.Path != b {
		t.Fatalf("docs-2 after reload = %+v, %v; want %s", got, ok, b)
	}
	if _, ok := r2.Get("docs"); ok {
		t.Fatal("removed root's slug must not point at another folder")
	}
}

func TestAddDeduplicatesSymlinkAlias(t *testing.T) {
	r, notes, _ := newTestRegistry(t)
	alias := filepath.Join(t.TempDir(), "alias")
	os.Symlink(notes, alias)
	got, err := r.Add(alias)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindNotes || len(r.List()) != 1 {
		t.Fatalf("Add(alias) = %+v, roots = %+v", got, r.List())
	}
}

func TestWithin(t *testing.T) {
	cases := []struct {
		root, path string
		want       bool
	}{
		{"/", "/", true},
		{"/", "/etc", true},
		{"/a", "/a", true},
		{"/a", "/a/b", true},
		{"/a", "/ab", false},
		{"/a", "/", false},
	}
	for _, c := range cases {
		if got := within(c.root, c.path); got != c.want {
			t.Errorf("within(%q, %q) = %v, want %v", c.root, c.path, got, c.want)
		}
	}
}

func TestFilesystemRoot(t *testing.T) {
	r, err := New("/", filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root := r.List()[0]
	if root.Slug != "root" {
		t.Fatalf("slug = %q", root.Slug)
	}
	if got, err := root.Resolve("etc"); err != nil || got != "/etc" {
		t.Fatalf("Resolve(etc) = %q, %v", got, err)
	}
	if _, err := root.Resolve("../etc"); !errors.Is(err, ErrOutside) {
		t.Fatalf("Resolve(../etc) err = %v", err)
	}
}

func TestNewSkipsPersistedNotesRoot(t *testing.T) {
	notes := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "roots.json")
	os.WriteFile(statePath, []byte(`{"recent":[{"slug":"x","path":"`+notes+`"}]}`), 0o644)
	r, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.List(); len(got) != 1 {
		t.Fatalf("List() = %+v, want notes root only", got)
	}
}

func TestNewWarnsAndContinuesOnStateProblems(t *testing.T) {
	notes := t.TempDir()
	var warnings []string
	warnf := func(f string, a ...any) { warnings = append(warnings, fmt.Sprintf(f, a...)) }

	// Corrupt JSON: the notes root still serves.
	statePath := filepath.Join(t.TempDir(), "roots.json")
	os.WriteFile(statePath, []byte(`{`), 0o644)
	r, err := New(notes, statePath, warnf)
	if err != nil || len(r.List()) != 1 || len(warnings) != 1 {
		t.Fatalf("corrupt state: err %v, roots %+v, warnings %q", err, r.List(), warnings)
	}

	// Previous on-disk format: unreadable entries are ignored, not fatal.
	warnings = nil
	os.WriteFile(statePath, []byte(`{"recent":["/tmp"]}`), 0o644)
	r, err = New(notes, statePath, warnf)
	if err != nil || len(r.List()) != 1 || len(warnings) != 1 {
		t.Fatalf("old format: err %v, roots %+v, warnings %q", err, r.List(), warnings)
	}

	// A stale recent in an unwritable state directory: the prune cannot be
	// saved, which is a warning and not a startup failure.
	warnings = nil
	dir := filepath.Join(t.TempDir(), "ro")
	os.Mkdir(dir, 0o755)
	statePath = filepath.Join(dir, "roots.json")
	os.WriteFile(statePath, []byte(`{"recent":[{"slug":"gone","path":"`+filepath.Join(dir, "gone")+`"}]}`), 0o644)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	r, err = New(notes, statePath, warnf)
	if err != nil || len(r.List()) != 1 || len(warnings) != 0 {
		t.Fatalf("unwritable state: err %v, roots %+v, warnings %q", err, r.List(), warnings)
	}
	if err := r.SettleState(); err == nil {
		t.Fatal("SettleState into an unwritable directory reported no error")
	}
}

func TestStateFileIsPrivate(t *testing.T) {
	r, _, statePath := newTestRegistry(t)
	r.Add(t.TempDir())
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file mode = %o, want 600", perm)
	}
}

func TestResolveConfinement(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(root, "sub", "b.md"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(base, "secret"), []byte("s"), 0o644)
	os.Symlink(filepath.Join(base, "secret"), filepath.Join(root, "escape"))
	os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "inside"))
	os.Symlink("..", filepath.Join(root, "up"))

	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	slug := r.List()[0].Slug

	ok := []struct{ rel, want string }{
		{"a.md", "a.md"},
		{"/a.md", "a.md"},
		{"sub/b.md", "sub/b.md"},
		{"./sub/../a.md", "a.md"},
		{"inside/b.md", "sub/b.md"},
		{"", ""},
		{"/", ""},
		{"sub/..", ""},
	}
	for _, c := range ok {
		got, err := r.Resolve(slug, c.rel)
		if err != nil {
			t.Errorf("Resolve(%q) error %v", c.rel, err)
			continue
		}
		if want := filepath.Join(root, c.want); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", c.rel, got, want)
		}
	}

	outside := []string{"../secret", "escape", "up/secret", "sub/../../secret", "/../secret"}
	for _, rel := range outside {
		if _, err := r.Resolve(slug, rel); !errors.Is(err, ErrOutside) {
			t.Errorf("Resolve(%q) err = %v, want ErrOutside", rel, err)
		}
	}

	if _, err := r.Resolve(slug, "nope.md"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file err = %v, want ErrNotExist", err)
	}
	if _, err := r.Resolve("unknown", "a.md"); err == nil {
		t.Error("unknown slug should error")
	}
}

func TestOpenParentConfinement(t *testing.T) {
	base := t.TempDir()
	notes := filepath.Join(base, "notes")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{notes, outside, filepath.Join(notes, "sub")} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(notes, "sub", "note.md"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "note.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := New(notes, filepath.Join(base, "state.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(notes, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside/note.md", "escape/note.md"} {
		if parent, _, _, err := reg.OpenParent("notes", name); !errors.Is(err, ErrOutside) {
			if parent != nil {
				parent.Close()
			}
			t.Errorf("OpenParent(%q) = %v", name, err)
		}
	}
	parent, name, canonical, err := reg.OpenParent("notes", "sub/note.md")
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if name != "note.md" || canonical != filepath.Join(notes, "sub", "note.md") {
		t.Fatalf("name=%s canonical=%s", name, canonical)
	}
	// Swapping the parent pathname after opening cannot redirect writes outside.
	if err := os.Rename(filepath.Join(notes, "sub"), filepath.Join(notes, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(notes, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := parent.WriteFile(name, []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outside, "note.md"))
	if err != nil || string(data) != "secret" {
		t.Fatalf("outside changed: %q %v", data, err)
	}
	// A final-component symlink introduced after resolving is confined too.
	if err := parent.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := parent.Symlink(filepath.Join(outside, "note.md"), name); err != nil {
		t.Fatal(err)
	}
	if err := parent.WriteFile(name, []byte("escape"), 0o644); err == nil {
		t.Fatal("followed outside symlink")
	}
}

func TestNotesReturnsThePermanentRoot(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes")
	other := filepath.Join(dir, "other")
	for _, d := range []string{notes, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r, err := New(notes, filepath.Join(dir, "state.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(other); err != nil {
		t.Fatal(err)
	}
	root, ok := r.Notes()
	if !ok {
		t.Fatal("no notes root")
	}
	if root.Kind != KindNotes || root.Path != notes {
		t.Errorf("notes root = %+v, want %s", root, notes)
	}
}

func TestOpenConfinesToTheRoot(t *testing.T) {
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := New(notes, filepath.Join(dir, "state.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := r.Notes()
	handle, err := root.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if _, err := handle.Create("inside.md"); err != nil {
		t.Errorf("create inside the root: %v", err)
	}
	if _, err := handle.Create("../outside.md"); err == nil {
		t.Error("created a file above the root")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.md")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a file appeared above the root")
	}
}

// Relative is the check a path that does not exist yet can still be put
// through: the same lexical confinement Resolve applies, with nothing
// asked of the filesystem.
func TestRelativeConfinesPathsThatDoNotExist(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]

	for rel, want := range map[string]string{
		"new.md":               "new.md",
		"/new.md":              "new.md",
		"a/b/new.md":           filepath.Join("a", "b", "new.md"),
		"./a/../new.md":        "new.md",
		"a/b/../../new.md":     "new.md",
		"":                     ".",
		"/":                    ".",
		"missing/nowhere.md":   filepath.Join("missing", "nowhere.md"),
		"sub/./deep/note.md":   filepath.Join("sub", "deep", "note.md"),
		"trailing/slash.md/":   filepath.Join("trailing", "slash.md"),
		"double//slash/a.md":   filepath.Join("double", "slash", "a.md"),
		"unicode/café note.md": filepath.Join("unicode", "café note.md"),
	} {
		got, err := only.Relative(rel)
		if err != nil {
			t.Errorf("Relative(%q) error %v", rel, err)
			continue
		}
		if got != want {
			t.Errorf("Relative(%q) = %q, want %q", rel, got, want)
		}
	}

	for _, rel := range []string{"..", "../secret.md", "a/../../secret.md", "/../secret.md", "../"} {
		if _, err := only.Relative(rel); !errors.Is(err, ErrOutside) {
			t.Errorf("Relative(%q) err = %v, want ErrOutside", rel, err)
		}
	}

	// It says nothing about symlinks, by design: a link is only knowable
	// once the path exists, and the caller must still open the result
	// through a handle on the root.
	if err := os.Symlink(base, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	if got, err := only.Relative("out/secret.md"); err != nil || got != filepath.Join("out", "secret.md") {
		t.Errorf("Relative through a link = %q, %v", got, err)
	}
	if _, err := only.Resolve("out/secret.md"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Resolve should be the one to look: %v", err)
	}
}

func TestEnsureDirMakesParentsAndRefusesAnEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "elsewhere"), filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]

	if err := only.EnsureDir(filepath.Join("a", "b", "c")); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "a", "b", "c")); err != nil || !info.IsDir() {
		t.Fatalf("directory not made: %v", err)
	}
	// Again, on a directory that now exists.
	if err := only.EnsureDir(filepath.Join("a", "b", "c")); err != nil {
		t.Fatalf("EnsureDir on an existing directory: %v", err)
	}
	// Through a link to a directory inside the root, named by its
	// absolute path — which a handle on the root may not traverse at all,
	// so the missing components are made through the resolved ancestor.
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := only.EnsureDir(filepath.Join("linkdir", "made")); err != nil {
		t.Fatalf("EnsureDir through an absolute link inside the root: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "a", "made")); err != nil || !info.IsDir() {
		t.Fatalf("directory not made through the link: %v", err)
	}
	// A component that exists but is not a directory.
	if err := os.WriteFile(filepath.Join(root, "file.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := only.EnsureDir(filepath.Join("file.md", "under")); !errors.Is(err, ErrNotDir) {
		t.Errorf("EnsureDir under a file err = %v, want ErrNotDir", err)
	}
	for _, dir := range []string{"out", filepath.Join("out", "deeper"), filepath.Join("..", "elsewhere")} {
		if err := only.EnsureDir(dir); !errors.Is(err, ErrOutside) {
			t.Errorf("EnsureDir(%q) err = %v, want ErrOutside", dir, err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(base, "elsewhere")); err != nil || len(entries) != 0 {
		t.Fatalf("made a directory outside the root: %v %v", entries, err)
	}
}

// OpenDir follows a link to a directory inside the root the way the read
// and save paths follow one, including a link named by its absolute path,
// which a handle on the root may not traverse itself.
func TestOpenDirFollowsLinksInsideTheRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real", "note.md"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.md"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "abs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "rel")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]

	for _, dir := range []string{"real", "abs", "rel", "."} {
		handle, real, err := only.OpenDir(dir)
		if err != nil {
			t.Errorf("OpenDir(%q): %v", dir, err)
			continue
		}
		want := filepath.Join(root, "real")
		if dir == "." {
			want = root
		}
		if real != want {
			t.Errorf("OpenDir(%q) real = %q, want %q", dir, real, want)
		}
		if _, err := handle.Lstat("note.md"); dir != "." && err != nil {
			t.Errorf("OpenDir(%q) cannot see the note: %v", dir, err)
		}
		handle.Close()
	}

	if _, _, err := only.OpenDir("file.md"); !errors.Is(err, ErrNotDir) {
		t.Errorf("OpenDir on a file err = %v, want ErrNotDir", err)
	}
	if _, _, err := only.OpenDir("out"); !errors.Is(err, ErrOutside) {
		t.Errorf("OpenDir on a link out err = %v, want ErrOutside", err)
	}
	if _, _, err := only.OpenDir("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("OpenDir on a missing directory err = %v, want ErrNotExist", err)
	}
}

// A link with no target may be one hop of a chain, and a chain that starts
// inside the root can still end outside it. EscapesChain follows it as far
// as the resolver itself would; a chain longer than that, and a circular
// one, end the walk with ErrTooManyLinks rather than with a silent "no".
func TestEscapesChainFollowsDanglingHops(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	links := [][2]string{
		{filepath.Join(base, "gone.md"), "out.md"},    // one hop, out
		{"hop.md", "two.md"},                          // two hops, the second out
		{filepath.Join(base, "gone.md"), "hop.md"},    //
		{"sub/deeper.md", "deep.md"},                  // three hops, the last out
		{"../../gone.md", "sub/deeper.md"},            //
		{"gone.md", "inside.md"},                      // dangling, but inside
		{"loop-b.md", "loop-a.md"},                    // circular
		{"loop-a.md", "loop-b.md"},                    //
		{"self.md", "self.md"},                        //
		{filepath.Join(root, "sub"), "abs-inside.md"}, // absolute, inside
	}
	for _, l := range links {
		if err := os.Symlink(l[0], filepath.Join(root, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	// As long a chain as the resolver behind Resolve will follow, and one
	// hop longer than the walk will.
	chain(t, root, "long", filepath.Join(base, "gone.md"), maxLinkHops)
	chain(t, root, "longer", filepath.Join(base, "gone.md"), maxLinkHops+1)
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]

	for _, c := range []struct {
		dir, base string
		want      bool
		wantErr   error
	}{
		{root, "out.md", true, nil},
		{root, "two.md", true, nil},
		{root, "deep.md", true, nil},
		{root, "inside.md", false, nil},
		{root, "self.md", false, ErrTooManyLinks},
		{root, "loop-a.md", false, ErrTooManyLinks},
		{root, "abs-inside.md", false, nil},
		{root, "sub", false, nil},       // a real directory, not a link at all
		{root, "absent.md", false, nil}, // nothing of that name
		{filepath.Join(root, "sub"), "deeper.md", true, nil},
		{root, "long-1.md", true, nil},
		{root, "longer-1.md", false, ErrTooManyLinks},
	} {
		got, err := only.EscapesChain(c.dir, c.base)
		if got != c.want || !errors.Is(err, c.wantErr) {
			t.Errorf("EscapesChain(%q, %q) = %v, %v; want %v, %v", c.dir, c.base, got, err, c.want, c.wantErr)
		}
	}
}

// chain makes n links under dir, each naming the next, the last naming a
// target outside the root. It is what a walk that stops short of n gets
// wrong.
func chain(t *testing.T, dir, prefix, outside string, n int) {
	t.Helper()
	for i := 1; i < n; i++ {
		link := filepath.Join(dir, fmt.Sprintf("%s-%d.md", prefix, i))
		if err := os.Symlink(fmt.Sprintf("%s-%d.md", prefix, i+1), link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(dir, fmt.Sprintf("%s-%d.md", prefix, n))); err != nil {
		t.Fatal(err)
	}
}

// DanglingEscape is the same question asked of a whole path: the link that
// leaves the root may be a directory component, and the components before
// it may themselves be links that stay inside.
func TestDanglingEscapeWalksTheComponents(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	links := [][2]string{
		{filepath.Join(base, "gone-dir"), "out-dir"}, // a directory link with no target, out
		{"sub", "live"},                            // a live link, inside
		{"gone-dir", "inside-dir"},                 // a directory link with no target, inside
		{"hop.md", "two.md"},                       // a two-hop chain, out
		{filepath.Join(base, "gone.md"), "hop.md"}, //
	}
	for _, l := range links {
		if err := os.Symlink(l[0], filepath.Join(root, l[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(base, "gone.md"), filepath.Join(root, "sub", "out.md")); err != nil {
		t.Fatal(err)
	}
	chain(t, root, "long", filepath.Join(base, "gone.md"), maxLinkHops)
	chain(t, root, "longer", filepath.Join(base, "gone.md"), maxLinkHops+1)
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]

	for _, c := range []struct {
		rel     string
		want    bool
		wantErr error
	}{
		{"out-dir/new.md", true, nil},
		{"out-dir/deeper/new.md", true, nil},
		{"out-dir", true, nil},
		{"two.md", true, nil},
		{"live/out.md", true, nil},
		{"inside-dir/new.md", false, nil},
		{"live/absent.md", false, nil},
		{"sub/absent.md", false, nil},
		{"absent.md", false, nil},
		{"sub", false, nil},
		{"long-1.md", true, nil},
		{"longer-1.md", false, ErrTooManyLinks},
		{"longer-1.md/child.md", false, ErrTooManyLinks},
	} {
		got, err := only.DanglingEscape(c.rel)
		if got != c.want || !errors.Is(err, c.wantErr) {
			t.Errorf("DanglingEscape(%q) = %v, %v; want %v, %v", c.rel, got, err, c.want, c.wantErr)
		}
	}
}

// The resolver's own give-up carries no path; every other error it returns
// quotes one, and a path is a name someone chose. A root whose own path
// reads like that message must not make every absent name under it answer
// for a chain of links, and neither must a note called that.
func TestResolveTellsTheGiveUpFromANameThatReadsLikeIt(t *testing.T) {
	base := filepath.Join(t.TempDir(), "too many links")
	root := filepath.Join(base, "notes")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "too many links.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	chain(t, root, "longer", filepath.Join(base, "gone.md"), maxLinkHops+1)
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]

	for _, c := range []struct {
		rel     string
		wantErr error
	}{
		{"absent.md", os.ErrNotExist},
		{"sub/absent.md", os.ErrNotExist},
		{"too many links.md", nil},
		{"too many links/absent.md", os.ErrNotExist},
		// The real thing still answers for what it is: a chain longer
		// than this resolver will follow.
		{"longer-1.md", ErrTooManyLinks},
	} {
		_, err := only.Resolve(c.rel)
		if !errors.Is(err, c.wantErr) {
			t.Errorf("Resolve(%q) = %v, want %v", c.rel, err, c.wantErr)
		}
	}
}

// Escapes answers for a link Resolve cannot follow, which is the only
// reason it exists: a dangling one has no real path.
func TestEscapesIsLexical(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := New(root, filepath.Join(t.TempDir(), "s.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	only := r.List()[0]
	sub := filepath.Join(root, "sub")

	for _, c := range []struct {
		realDir, target string
		want            bool
	}{
		{root, "gone.md", false},
		{root, "sub/gone.md", false},
		{sub, "../gone.md", false},
		{sub, "gone.md", false},
		{root, "../gone.md", true},
		{sub, "../../gone.md", true},
		{root, filepath.Join(base, "gone.md"), true},
		{root, filepath.Join(root, "gone.md"), false},
		{root, "/etc/passwd", true},
	} {
		if got := only.Escapes(c.realDir, c.target); got != c.want {
			t.Errorf("Escapes(%q, %q) = %v, want %v", c.realDir, c.target, got, c.want)
		}
	}
}

// AddFor is Add with something to check first: the folder is registered
// only when the file named inside it is there. Nothing is appended to the
// registry and nothing is written to the state file otherwise, which is
// what makes a refusal leave no trace — the defect in
// [#50](https://github.com/davison/md-notes/issues/50) was a root that
// outlived the file it was registered for. Whether the name is one the
// daemon serves as a note is the server's question, not this one's.
func TestAddForVerifiesTheFileFirst(t *testing.T) {
	r, _, statePath := newTestRegistry(t)
	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"todo.md", filepath.Join("sub", "deep.md")} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "todo.md"), filepath.Join(dir, "inside.md")); err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{
		"gone.md",
		"sub/gone.md",
		"../elsewhere/secret.md",
		filepath.Join(outside, "secret.md"),
		"escape.md",
		"sub",
		".",
	} {
		if _, err := r.AddFor(dir, file); !errors.Is(err, ErrNoNote) {
			t.Errorf("AddFor(dir, %q) err = %v, want ErrNoNote", file, err)
		}
		if got := r.List(); len(got) != 1 {
			t.Fatalf("AddFor(dir, %q) registered %+v", file, got)
		}
		if _, err := os.Stat(statePath); !os.IsNotExist(err) {
			t.Fatalf("AddFor(dir, %q) wrote the state file", file)
		}
	}

	// A file that is there, named four ways, registers the folder once.
	for _, file := range []string{"todo.md", "./todo.md", "sub/deep.md", "inside.md"} {
		got, err := r.AddFor(dir, file)
		if err != nil || got.Path != dir || got.Kind != KindRecent {
			t.Fatalf("AddFor(dir, %q) = %+v, %v", file, got, err)
		}
	}
	if got := r.List(); len(got) != 2 {
		t.Fatalf("List() = %+v, want the notes root and one more", got)
	}
	// And an empty file is Add's own behaviour: nothing to check.
	other := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AddFor(other, ""); err != nil {
		t.Fatalf("AddFor(empty dir, \"\") = %v", err)
	}
}

// Remove takes a recent root out of the registry and the state file, and
// refuses the two roots there is no sense in removing. M7-R2.
func TestRemove(t *testing.T) {
	r, notes, statePath := newTestRegistry(t)
	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(dir, "todo.md")
	if err := os.WriteFile(note, []byte("# todo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := r.Add(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.Remove("nope"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Remove(unknown) err = %v, want os.ErrNotExist", err)
	}
	if _, err := r.Remove("my-notes"); !errors.Is(err, ErrNotesRoot) {
		t.Errorf("Remove(notes root) err = %v, want ErrNotesRoot", err)
	}
	if got := r.List(); len(got) != 2 {
		t.Fatalf("a refused Remove changed the registry: %+v", got)
	}

	gone, err := r.Remove(added.Slug)
	if err != nil {
		t.Fatalf("Remove(%q) = %v", added.Slug, err)
	}
	if gone.Path != dir {
		t.Errorf("Remove returned %+v, want the root it removed", gone)
	}
	if got := r.List(); len(got) != 1 || got[0].Kind != KindNotes {
		t.Fatalf("List() = %+v, want the notes root alone", got)
	}
	if _, ok := r.Get(added.Slug); ok {
		t.Error("the removed root is still resolvable by slug")
	}
	// Unregistering is not deleting.
	if _, err := os.Stat(note); err != nil {
		t.Errorf("the note went with the root: %v", err)
	}
	// The state file is rewritten, so the root stays gone across a restart.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), dir) {
		t.Errorf("the removed root is still in the state file: %s", data)
	}
	again, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.List(); len(got) != 1 {
		t.Fatalf("after a reload List() = %+v", got)
	}
	// Removing it twice is an unknown slug the second time.
	if _, err := r.Remove(added.Slug); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("second Remove err = %v, want os.ErrNotExist", err)
	}
}

// A slug freed by a removal is available again, and the root that takes it
// is the one the state file names afterwards.
func TestRemoveFreesTheSlug(t *testing.T) {
	r, _, _ := newTestRegistry(t)
	a := filepath.Join(t.TempDir(), "docs")
	b := filepath.Join(t.TempDir(), "docs")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)
	if _, err := r.Add(a); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Remove("docs"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Add(b)
	if err != nil || got.Slug != "docs" {
		t.Fatalf("Add after Remove = %+v, %v; want the freed slug", got, err)
	}
}

// Removing the last recent root leaves a state file that says there are
// none, as an empty list and not as null (#125).
func TestRemoveTheLastRecentWritesAnEmptyList(t *testing.T) {
	r, _, statePath := newTestRegistry(t)
	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Remove("proj"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "{\n  \"recent\": []\n}"; got != want {
		t.Errorf("state file = %q, want %q", got, want)
	}
}

// Several configured roots (M10-R5, #162): the first is the notes root, the
// rest are permanent, in the order given, and none of them is a
// registration Remove can undo or the state file records.
func TestNewConfiguredServesEveryConfiguredRoot(t *testing.T) {
	base := t.TempDir()
	notes, projects, work := filepath.Join(base, "notes"), filepath.Join(base, "projects"), filepath.Join(base, "work")
	for _, d := range []string{notes, projects, work} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	statePath := filepath.Join(t.TempDir(), "roots.json")
	r, err := NewConfigured([]string{notes, projects, work}, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := r.List()
	want := []struct {
		slug string
		kind Kind
	}{{"notes", KindNotes}, {"projects", KindPermanent}, {"work", KindPermanent}}
	if len(got) != len(want) {
		t.Fatalf("List() = %+v", got)
	}
	for i, w := range want {
		if got[i].Slug != w.slug || got[i].Kind != w.kind {
			t.Errorf("root %d = %+v, want %s (%s)", i, got[i], w.slug, w.kind)
		}
	}
	if n, _ := r.Notes(); n.Path != notes {
		t.Errorf("Notes() = %+v, want the first configured root", n)
	}
	if _, err := r.Remove("projects"); !errors.Is(err, ErrPermanentRoot) {
		t.Errorf("Remove(permanent root) err = %v, want ErrPermanentRoot", err)
	}
	if _, err := r.Remove("notes"); !errors.Is(err, ErrNotesRoot) {
		t.Errorf("Remove(notes root) err = %v, want ErrNotesRoot", err)
	}
	if len(r.List()) != 3 {
		t.Errorf("a refused Remove changed the registry: %+v", r.List())
	}
	// Opening a permanent root is the root that is already there, not a
	// recent one, and nothing is written for it.
	if again, err := r.Add(projects); err != nil || again.Kind != KindPermanent || again.Slug != "projects" {
		t.Errorf("Add(permanent root) = %+v, %v", again, err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a registry with no recent roots wrote a state file: %v", err)
	}
}

// The same folder twice in the configured list is refused, however it is
// spelled; one folder inside another is served as two roots.
func TestNewConfiguredDuplicatesAndNesting(t *testing.T) {
	base := t.TempDir()
	notes := filepath.Join(base, "notes")
	inner := filepath.Join(notes, "projects")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(notes, alias); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "roots.json")
	for _, c := range []struct {
		name  string
		paths []string
	}{
		{"the same path twice", []string{notes, inner, notes}},
		{"a symlink alias", []string{notes, alias}},
		{"a trailing slash", []string{inner + "/", notes, inner}},
	} {
		_, err := NewConfigured(c.paths, state, nil)
		if !errors.Is(err, ErrDuplicateRoot) {
			t.Errorf("%s: err = %v, want ErrDuplicateRoot", c.name, err)
		}
	}

	r, err := NewConfigured([]string{notes, inner}, state, nil)
	if err != nil {
		t.Fatalf("a nested root: %v", err)
	}
	if got := r.List(); len(got) != 2 || got[1].Kind != KindPermanent || got[1].Path != inner {
		t.Errorf("List() = %+v, want the inner folder as a permanent root", got)
	}
	// And the other way round: the notes root inside a permanent root.
	if _, err := NewConfigured([]string{inner, notes}, state, nil); err != nil {
		t.Errorf("the notes root inside a permanent root: %v", err)
	}
}

// A folder once opened with `mdn open` and since configured is served once,
// as configuration, with a warning naming it; its state-file entry is kept,
// through saves as well, so a start without the configuration serves it as
// a recent root again, under the same slug.
func TestNewConfiguredShadowsAPersistedRecentRoot(t *testing.T) {
	base := t.TempDir()
	notes, projects, other := filepath.Join(base, "notes"), filepath.Join(base, "projects"), filepath.Join(base, "other")
	for _, d := range []string{notes, projects, other} {
		os.Mkdir(d, 0o755)
	}
	statePath := filepath.Join(t.TempDir(), "roots.json")
	original := []byte(`{"recent":[{"slug":"proj","path":"` + projects + `"}]}`)
	os.WriteFile(statePath, original, 0o600)
	var warnings []string
	warnf := func(f string, a ...any) { warnings = append(warnings, fmt.Sprintf(f, a...)) }

	r, err := NewConfigured([]string{notes, projects}, statePath, warnf)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.List(); len(got) != 2 || got[1].Kind != KindPermanent || got[1].Slug != "projects" {
		t.Fatalf("List() = %+v, want projects once, as permanent", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], projects) {
		t.Errorf("warnings = %q, want one naming %s", warnings, projects)
	}
	if err := r.SettleState(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(statePath); string(data) != string(original) {
		t.Errorf("state file changed to %s", data)
	}
	// A save for another reason keeps the entry.
	if _, err := r.Add(other); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(statePath); !strings.Contains(string(data), projects) {
		t.Errorf("a save dropped the shadowed entry: %s", data)
	}

	// Without the configuration it is a recent root again.
	again, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := again.List()
	if len(got) != 3 || got[1].Slug != "other" || got[2].Slug != "proj" || got[2].Kind != KindRecent {
		t.Errorf("after a start without it List() = %+v", got)
	}
}

func TestNewConfiguredNamesTheRootThatFails(t *testing.T) {
	notes := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := NewConfigured([]string{notes, missing}, filepath.Join(t.TempDir(), "s.json"), nil)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("err = %v, want it to name %s", err, missing)
	}
	if _, err := NewConfigured(nil, filepath.Join(t.TempDir(), "s.json"), nil); err == nil {
		t.Fatal("want an error for no roots at all")
	}
}

// A shadowed entry's slug stays reserved while it is hidden, so a folder
// opened meanwhile cannot take it, and the entry comes back under its own
// slug on a start without the configuration. The reviewer's sequence on
// PR #198 (round two, finding 1).
func TestShadowedSlugIsReserved(t *testing.T) {
	base := t.TempDir()
	notes, projects := filepath.Join(base, "notes"), filepath.Join(base, "projects")
	other, another := filepath.Join(base, "other"), filepath.Join(base, "c", "projects")
	for _, d := range []string{notes, projects, other, another} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	statePath := filepath.Join(t.TempDir(), "roots.json")
	os.WriteFile(statePath, []byte(`{"recent":[{"slug":"projects-2","path":"`+projects+
		`"},{"slug":"other","path":"`+other+`"}]}`), 0o600)

	r, err := NewConfigured([]string{notes, projects}, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := r.Get("projects"); !ok || p.Path != projects || p.Kind != KindPermanent {
		t.Fatalf("projects = %+v, %v", p, ok)
	}
	opened, err := r.Add(another)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Slug == "projects-2" || opened.Slug == "projects" {
		t.Errorf("a folder opened while projects-2 is shadowed took %q", opened.Slug)
	}
	var st state
	data, _ := os.ReadFile(statePath)
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, p := range st.Recent {
		if prev, ok := seen[p.Slug]; ok {
			t.Errorf("slug %q is in the state file twice: %s and %s", p.Slug, prev, p.Path)
		}
		seen[p.Slug] = p.Path
	}

	// Without the configuration, every entry comes back under its own slug.
	again, err := New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	for slug, path := range map[string]string{"projects-2": projects, opened.Slug: another, "other": other} {
		if got, ok := again.Get(slug); !ok || got.Path != path || got.Kind != KindRecent {
			t.Errorf("after a start without the configuration %q = %+v, %v; want %s", slug, got, ok, path)
		}
	}
}
