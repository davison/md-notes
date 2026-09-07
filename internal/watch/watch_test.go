package watch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func newTestWatcher(t *testing.T, dirs []string) (*Watcher, string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	w, err := New(root, dirs, t.Logf, WithDebounce(50*time.Millisecond, 300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	// Let the watches settle before the test writes anything.
	time.Sleep(20 * time.Millisecond)
	return w, root
}

func next(t *testing.T, w *Watcher) Batch {
	t.Helper()
	select {
	case b, ok := <-w.Events():
		if !ok {
			t.Fatal("events closed")
		}
		return b
	case <-time.After(time.Second):
		t.Fatal("no batch within a second")
	}
	return Batch{}
}

func noBatch(t *testing.T, w *Watcher) {
	t.Helper()
	select {
	case b := <-w.Events():
		t.Fatalf("unexpected batch %v", b.Paths)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestCreateWriteDeleteRename(t *testing.T) {
	w, root := newTestWatcher(t, []string{"docs"})
	p := filepath.Join(root, "docs", "a.md")

	os.WriteFile(p, []byte("x"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"docs/a.md"}) {
		t.Fatalf("create: %v", b.Paths)
	}
	os.WriteFile(p, []byte("xy"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"docs/a.md"}) {
		t.Fatalf("write: %v", b.Paths)
	}
	os.Rename(p, filepath.Join(root, "docs", "b.md"))
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"docs/a.md", "docs/b.md"}) {
		t.Fatalf("rename: %v", b.Paths)
	}
	os.Remove(filepath.Join(root, "docs", "b.md"))
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"docs/b.md"}) {
		t.Fatalf("delete: %v", b.Paths)
	}
}

func TestBurstIsOneBatch(t *testing.T) {
	w, root := newTestWatcher(t, nil)
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(root, "n.md"), []byte{byte(i)}, 0o644)
	}
	b := next(t, w)
	if !reflect.DeepEqual(b.Paths, []string{"n.md"}) {
		t.Fatalf("batch = %v", b.Paths)
	}
	noBatch(t, w)
}

func TestNewSubdirectoryIsWatched(t *testing.T) {
	w, root := newTestWatcher(t, nil)
	os.MkdirAll(filepath.Join(root, "new", "deep"), 0o755)
	b := next(t, w)
	if len(b.Paths) == 0 || b.Paths[0] != "new" {
		t.Fatalf("mkdir batch = %v", b.Paths)
	}
	os.WriteFile(filepath.Join(root, "new", "deep", "x.md"), []byte("x"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"new/deep/x.md"}) {
		t.Fatalf("file in new dir: %v", b.Paths)
	}
}

func TestDirectoryRenameReportsDirectory(t *testing.T) {
	w, root := newTestWatcher(t, []string{"old", "old/sub"})
	os.WriteFile(filepath.Join(root, "old", "x.md"), nil, 0o644)
	next(t, w)
	os.Rename(filepath.Join(root, "old"), filepath.Join(root, "renamed"))
	b := next(t, w)
	if !reflect.DeepEqual(b.Paths, []string{"old", "renamed"}) {
		t.Fatalf("dir rename: %v", b.Paths)
	}
	// The renamed directory and its subdirectory are watched under the
	// new name, and nothing reports under the old one.
	os.WriteFile(filepath.Join(root, "renamed", "y.md"), nil, 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"renamed/y.md"}) {
		t.Fatalf("after rename: %v", b.Paths)
	}
	os.WriteFile(filepath.Join(root, "renamed", "sub", "z.md"), nil, 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"renamed/sub/z.md"}) {
		t.Fatalf("after rename, nested: %v", b.Paths)
	}
	for _, p := range w.fsw.WatchList() {
		if strings.Contains(p, "old") {
			t.Fatalf("stale watch %s", p)
		}
	}
}

func TestDeletedDirectoryDropsWatches(t *testing.T) {
	w, root := newTestWatcher(t, []string{"gone", "gone/sub"})
	before := len(w.fsw.WatchList())
	os.RemoveAll(filepath.Join(root, "gone"))
	next(t, w)
	if after := len(w.fsw.WatchList()); after != before-2 {
		t.Fatalf("watches before %d, after %d; want two fewer", before, after)
	}
}

func TestNewDirectoryHonoursIgnores(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	w, root := newTestWatcher(t, nil)
	os.WriteFile(filepath.Join(root, ".ignore"), []byte("node_modules/\n"), 0o644)
	// Created after the watcher started, as npm install would.
	os.MkdirAll(filepath.Join(root, "node_modules", "dep", "lib"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "dep", "README.md"), nil, 0o644)
	os.MkdirAll(filepath.Join(root, "src", "notes"), 0o755)
	time.Sleep(400 * time.Millisecond)
	for _, p := range w.fsw.WatchList() {
		if strings.Contains(p, "node_modules") {
			t.Fatalf("watching an ignored tree: %s", p)
		}
	}
	// Drain whatever the creations produced, then prove src/notes is live.
	for {
		select {
		case <-w.Events():
			continue
		case <-time.After(100 * time.Millisecond):
		}
		break
	}
	os.WriteFile(filepath.Join(root, "src", "notes", "a.md"), nil, 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"src/notes/a.md"}) {
		t.Fatalf("new nested dir: %v", b.Paths)
	}
}

func TestLostBatchAsksForFullRefresh(t *testing.T) {
	root := t.TempDir()
	w, err := New(root, nil, nil, WithDebounce(10*time.Millisecond, 50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)
	// Fill the output buffer without reading, so a batch is dropped.
	for i := 0; i < 40; i++ {
		os.WriteFile(filepath.Join(root, fmt.Sprintf("f%d.md", i)), nil, 0o644)
		time.Sleep(25 * time.Millisecond)
	}
	var sawEmpty bool
	for i := 0; i < 20; i++ {
		select {
		case b := <-w.Events():
			if len(b.Paths) == 0 {
				sawEmpty = true
			}
		case <-time.After(300 * time.Millisecond):
		}
	}
	if !sawEmpty {
		t.Fatal("no full-refresh batch after drops")
	}
}

func TestHiddenIgnored(t *testing.T) {
	w, root := newTestWatcher(t, nil)
	os.WriteFile(filepath.Join(root, ".swp"), nil, 0o644)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	noBatch(t, w)
	os.WriteFile(filepath.Join(root, ".git", "HEAD"), nil, 0o644)
	noBatch(t, w)
}

func TestChmodIgnored(t *testing.T) {
	w, root := newTestWatcher(t, nil)
	p := filepath.Join(root, "a.md")
	os.WriteFile(p, nil, 0o644)
	next(t, w)
	os.Chmod(p, 0o600)
	noBatch(t, w)
}

func TestCloseEndsEvents(t *testing.T) {
	w, _ := newTestWatcher(t, nil)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-w.Events(); ok {
		t.Fatal("events still open after Close")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestMissingDirIsSkipped(t *testing.T) {
	root := t.TempDir()
	var warned bool
	w, err := New(root, []string{"nope"}, func(string, ...any) { warned = true })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if warned {
		t.Fatal("a missing directory is not worth a warning")
	}
}

func TestHidden(t *testing.T) {
	for p, want := range map[string]bool{"a/b.md": false, ".a/b.md": true, "a/.b": true, "a/..": false, ".": false} {
		if got := hidden(p); got != want {
			t.Errorf("hidden(%q) = %v", p, got)
		}
	}
}
