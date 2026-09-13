package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/davison/md-notes/internal/roots"
)

func fixture(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "notes")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(dir, "note.md"), "---\r\ntitle: 'Unchanged'\r\n---\r\n# café\r\n\r\n", 0o640)
	reg, err := roots.New(dir, filepath.Join(t.TempDir(), "state.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(reg), dir
}

func put(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, s *Store, path string) Note {
	t.Helper()
	note, err := s.Read("notes", path)
	if err != nil {
		t.Fatal(err)
	}
	return note
}

func assertDisk(t *testing.T, dir, text string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "note.md"))
	if err != nil || string(data) != text {
		t.Fatalf("disk = %q, %v; want %q", data, err, text)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".mdn-save-") {
			t.Errorf("left staging file %s", e.Name())
		}
	}
}

func TestRoundTripAndVersions(t *testing.T) {
	s, dir := fixture(t)
	before := get(t, s, "note.md")
	if before.Source != "---\r\ntitle: 'Unchanged'\r\n---\r\n# café\r\n\r\n" || before.Revision == "" {
		t.Fatalf("read = %+v", before)
	}
	if again := get(t, s, "note.md"); again != before {
		t.Fatalf("unchanged read = %+v", again)
	}
	text := before.Source + "no final newline\x00"
	saved, err := s.Save("notes", "note.md", before.Revision, text)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source != text || saved.Revision == before.Revision || saved.Revision == "" {
		t.Fatalf("save = %+v", saved)
	}
	if again := get(t, s, "note.md"); again != saved {
		t.Fatalf("read after save = %+v; want %+v", again, saved)
	}
	info, err := os.Stat(filepath.Join(dir, "note.md"))
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, %v", info, err)
	}
	assertDisk(t, dir, text)
	if _, err := s.Save("notes", "note.md", before.Revision, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale = %v", err)
	}
	empty, err := s.Save("notes", "note.md", saved.Revision, "")
	if err != nil || empty.Source != "" {
		t.Fatalf("empty save = %+v, %v", empty, err)
	}
	assertDisk(t, dir, "")
}

func TestConcurrentSavesThroughAliases(t *testing.T) {
	s, dir := fixture(t)
	if err := os.Symlink("note.md", filepath.Join(dir, "alias.md")); err != nil {
		t.Fatal(err)
	}
	// The same file is reachable through a second registered root too.
	parent, err := s.reg.Add(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	before := get(t, s, "note.md")
	alias := get(t, s, "alias.md")
	if alias.Revision != before.Revision {
		t.Fatal("aliases do not share revision")
	}
	const n = 20
	start := make(chan struct{})
	result := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			slug, path := "notes", "note.md"
			if i%3 == 1 {
				path = "alias.md"
			}
			if i%3 == 2 {
				slug, path = parent.Slug, "notes/note.md"
			}
			_, err := s.Save(slug, path, before.Revision, "winner")
			result <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(result)
	success, conflict := 0, 0
	for err := range result {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != n-1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	assertDisk(t, dir, "winner")
	if info, err := os.Lstat(filepath.Join(dir, "alias.md")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("alias replaced: %v %v", info, err)
	}
}

func TestExternalChanges(t *testing.T) {
	for _, kind := range []string{"edit", "same-content-replacement", "metadata", "delete", "retarget"} {
		t.Run(kind, func(t *testing.T) {
			s, dir := fixture(t)
			path := filepath.Join(dir, "note.md")
			if err := os.Symlink("note.md", filepath.Join(dir, "alias.md")); err != nil {
				t.Fatal(err)
			}
			before := get(t, s, "alias.md")
			expected := before.Source
			want := ErrConflict
			switch kind {
			case "edit":
				expected = "external"
				put(t, path, expected, 0o640)
			case "same-content-replacement":
				info, _ := os.Stat(path)
				put(t, filepath.Join(dir, "replacement"), before.Source, 0o640)
				if err := os.Chtimes(filepath.Join(dir, "replacement"), info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(dir, "replacement"), path); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				want = os.ErrNotExist
			case "retarget":
				put(t, filepath.Join(dir, "other.md"), "other", 0o640)
				if err := os.Remove(filepath.Join(dir, "alias.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("other.md", filepath.Join(dir, "alias.md")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Save("notes", "alias.md", before.Revision, "draft"); !errors.Is(err, want) {
				t.Fatalf("save = %v; want %v", err, want)
			}
			if kind == "delete" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("deleted file recreated: %v", err)
				}
			} else {
				assertDisk(t, dir, expected)
			}
		})
	}
}

func TestRestartRejectsOldRevision(t *testing.T) {
	s, dir := fixture(t)
	before := get(t, s, "note.md")
	restarted := New(s.reg)
	if _, err := restarted.Save("notes", "note.md", before.Revision, "draft"); !errors.Is(err, ErrConflict) {
		t.Fatalf("restart save = %v", err)
	}
	assertDisk(t, dir, before.Source)
}

func TestPermissionsAndUnsupportedSource(t *testing.T) {
	for _, kind := range []string{"read-only-file", "read-only-directory", "unreadable-file", "directory", "encoding", "size"} {
		t.Run(kind, func(t *testing.T) {
			s, dir := fixture(t)
			path := filepath.Join(dir, "note.md")
			before := get(t, s, "note.md")
			want := os.ErrPermission
			switch kind {
			case "read-only-file":
				if err := os.Chmod(path, 0o440); err != nil {
					t.Fatal(err)
				}
				before = get(t, s, "note.md")
			case "read-only-directory":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses directory permissions")
				}
				if err := os.Chmod(dir, 0o550); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(dir, 0o755)
			case "unreadable-file":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses read permissions")
				}
				if err := os.Chmod(path, 0o200); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				want = ErrNotRegular
			case "encoding":
				put(t, path, "\xff", 0o640)
				want = ErrEncoding
			case "size":
				if err := os.Truncate(path, MaxBytes+1); err != nil {
					t.Fatal(err)
				}
				want = ErrTooLarge
			}
			if _, err := s.Save("notes", "note.md", before.Revision, "draft"); !errors.Is(err, want) {
				t.Fatalf("save = %v, want %v", err, want)
			}
			if kind == "read-only-file" || kind == "read-only-directory" {
				assertDisk(t, dir, before.Source)
			}
		})
	}
	s, dir := fixture(t)
	before := get(t, s, "note.md")
	for _, text := range []string{"\xff", strings.Repeat("x", MaxBytes+1)} {
		_, err := s.Save("notes", "note.md", before.Revision, text)
		if !errors.Is(err, ErrEncoding) && !errors.Is(err, ErrTooLarge) {
			t.Fatalf("invalid source save = %v", err)
		}
	}
	assertDisk(t, dir, before.Source)
}

func TestReplaceFinalCheckAndCleanup(t *testing.T) {
	for _, kind := range []string{"edit", "delete", "rename-failure"} {
		t.Run(kind, func(t *testing.T) {
			_, dir := fixture(t)
			parent, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			_, before, err := read(parent, "note.md")
			if err != nil {
				t.Fatal(err)
			}
			_, err = replace(parent, "note.md", []byte("draft"), 0o640, func() error {
				if kind == "edit" {
					put(t, filepath.Join(dir, "note.md"), "external", 0o640)
				} else {
					if err := parent.Remove("note.md"); err != nil {
						t.Fatal(err)
					}
					if kind == "rename-failure" {
						if err := parent.Mkdir("note.md", 0o755); err != nil {
							t.Fatal(err)
						}
						return nil
					}
				}
				_, current, err := read(parent, "note.md")
				if err != nil {
					return err
				}
				if !before.same(current) {
					return ErrConflict
				}
				return nil
			})
			if err == nil {
				t.Fatal("expected save failure")
			}
			if kind == "edit" {
				assertDisk(t, dir, "external")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".mdn-save-") {
					t.Errorf("staging file survived: %s", e.Name())
				}
			}
		})
	}
}
