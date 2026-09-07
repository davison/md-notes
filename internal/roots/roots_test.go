package roots

import (
	"errors"
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
	r, err := New(notes, statePath)
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
	if _, err := New(f, filepath.Join(t.TempDir(), "s.json")); !errors.Is(err, ErrNotDir) {
		t.Fatalf("err = %v, want ErrNotDir", err)
	}
	if _, err := New(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "s.json")); err == nil {
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

	r2, err := New(notes, statePath)
	if err != nil {
		t.Fatal(err)
	}
	got := r2.List()
	if len(got) != 2 || got[0].Kind != KindNotes || got[1].Path != kept || got[1].Slug != "kept" {
		t.Fatalf("after reload List() = %+v", got)
	}
	data, _ := os.ReadFile(statePath)
	if strings.Contains(string(data), gone) {
		t.Fatalf("dropped root still persisted: %s", data)
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

	r2, err := New(notes, statePath)
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
	r, err := New("/", filepath.Join(t.TempDir(), "s.json"))
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
	r, err := New(notes, statePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.List(); len(got) != 1 {
		t.Fatalf("List() = %+v, want notes root only", got)
	}
}

func TestNewRejectsCorruptState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "roots.json")
	os.WriteFile(statePath, []byte(`{`), 0o644)
	if _, err := New(t.TempDir(), statePath); err == nil {
		t.Fatal("want error for corrupt state file")
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

	r, err := New(root, filepath.Join(t.TempDir(), "s.json"))
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
