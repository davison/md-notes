package roots

import (
	"errors"
	"os"
	"path/filepath"
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
}

func TestNewSkipsPersistedNotesRoot(t *testing.T) {
	notes := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "roots.json")
	os.WriteFile(statePath, []byte(`{"recent":["`+notes+`"]}`), 0o644)
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
