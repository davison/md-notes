package watch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/davison/md-notes/internal/tree"
)

// group builds a directory set of one priority group, in the order given.
func group(g tree.Group, names ...string) []tree.Dir {
	out := make([]tree.Dir, 0, len(names))
	for _, n := range names {
		out = append(out, tree.Dir{Path: n, Group: g})
	}
	return out
}

func newTestWatcher(t *testing.T, dirs []string) (*Watcher, string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	w, err := New(root, group(tree.GroupNotes, dirs...), t.Logf, WithDebounce(50*time.Millisecond, 300*time.Millisecond))
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

func TestOverflowAsksForFullRefresh(t *testing.T) {
	root := t.TempDir()
	w, err := New(root, nil, nil, WithDebounce(10*time.Millisecond, 50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)
	w.fsw.Errors <- fsnotify.ErrEventOverflow
	select {
	case b := <-w.Events():
		if len(b.Paths) != 0 {
			t.Fatalf("after overflow got %v, want an empty full-refresh batch", b.Paths)
		}
	case <-time.After(time.Second):
		t.Fatal("no batch after an event overflow")
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
	w, err := New(root, group(tree.GroupNotes, "nope"), func(string, ...any) { warned = true })
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

func TestBudgetWatchesThePriorityPrefix(t *testing.T) {
	root := t.TempDir()
	dirs := []string{"notes", "placeholder", "assets", "more"}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	var logged []string
	w, err := New(root, group(tree.GroupNotes, dirs...), func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) },
		WithDebounce(50*time.Millisecond, 300*time.Millisecond), WithBudget(3))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	cov := w.Coverage()
	want := Coverage{Watched: 3, Unwatched: 2, Budget: 3, OverBudget: true, Limited: true}
	if cov != want {
		t.Fatalf("Coverage() = %+v, want %+v", cov, want)
	}
	// The budget bought the root and the two directories offered first.
	var got []string
	for _, p := range w.fsw.WatchList() {
		r, _ := filepath.Rel(root, p)
		got = append(got, r)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{".", "notes", "placeholder"}) {
		t.Fatalf("watched = %v", got)
	}
	// A change in a watched directory is still reported.
	os.WriteFile(filepath.Join(root, "notes", "a.md"), nil, 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"notes/a.md"}) {
		t.Fatalf("watched directory: %v", b.Paths)
	}
	// One line says the coverage is limited, why, and what it costs.
	if len(logged) != 1 || !strings.Contains(logged[0], "2 unwatched") ||
		!strings.Contains(logged[0], "budget of 3") || !strings.Contains(logged[0], "max_watches") {
		t.Fatalf("log = %q", logged)
	}
}

func TestNoBudgetCoversEverything(t *testing.T) {
	root := t.TempDir()
	dirs := []string{"a", "b"}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	var logged int
	w, err := New(root, group(tree.GroupNotes, dirs...), func(string, ...any) { logged++ }, WithBudget(0))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	cov := w.Coverage()
	if cov.Limited || cov.Watched != 3 || cov.Unwatched != 0 || cov.Budget != 0 {
		t.Fatalf("Coverage() = %+v, want complete coverage of three directories", cov)
	}
	if logged != 0 {
		t.Fatalf("%d log lines for a fully covered root", logged)
	}
}

func TestBudgetStillCoversTheRoot(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	w, err := New(root, group(tree.GroupNotes, "sub"), nil, WithDebounce(50*time.Millisecond, 300*time.Millisecond), WithBudget(1))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)
	if cov := w.Coverage(); cov.Watched != 1 || cov.Unwatched != 1 {
		t.Fatalf("Coverage() = %+v, want the root watched and sub not", cov)
	}
	os.WriteFile(filepath.Join(root, "top.md"), nil, 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"top.md"}) {
		t.Fatalf("root batch = %v", b.Paths)
	}
}

// The first note in a directory whose only file is a hidden placeholder is
// reported live, from the same listing the daemon watches with.
func TestFirstNoteInHiddenOnlyDirectory(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "inbox"), 0o755)
	os.WriteFile(filepath.Join(root, "inbox", ".gitkeep"), nil, 0o644)
	dirs, err := tree.Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(root, dirs, t.Logf, WithDebounce(50*time.Millisecond, 300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)

	os.WriteFile(filepath.Join(root, "inbox", "first.md"), []byte("# first"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"inbox/first.md"}) {
		t.Fatalf("first note in a hidden-only directory: %v", b.Paths)
	}
}

// watchedRel lists the watched directories relative to the root, sorted.
func watchedRel(t *testing.T, w *Watcher, root string) []string {
	t.Helper()
	var got []string
	for _, p := range w.fsw.WatchList() {
		r, _ := filepath.Rel(root, p)
		got = append(got, r)
	}
	sort.Strings(got)
	return got
}

func TestPlaceReclaimsTheLowestPriorityWatch(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "b", "c", "d"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	dirs := []tree.Dir{
		{Path: "a", Group: tree.GroupNotes},
		{Path: "b", Group: tree.GroupEmpty},
		{Path: "c", Group: tree.GroupOther},
	}
	w, err := New(root, dirs, nil, WithBudget(3))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "b"}) {
		t.Fatalf("watched = %v, want the root and the two best directories", got)
	}

	// Recomputing an unchanged set changes nothing: equal priorities never
	// displace each other, so a stable root does not churn its watches.
	w.place(dirs)
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "b"}) {
		t.Fatalf("after replacing the same set, watched = %v", got)
	}

	// A directory that holds a note takes the watch off the least valuable
	// one, not off the root and not off the other notes directory.
	w.place([]tree.Dir{
		{Path: "a", Group: tree.GroupNotes},
		{Path: "d", Group: tree.GroupNotes},
		{Path: "b", Group: tree.GroupEmpty},
		{Path: "c", Group: tree.GroupOther},
	})
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "d"}) {
		t.Fatalf("watched = %v, want d to have taken b's watch", got)
	}
	cov := w.Coverage()
	if cov.Watched != 3 || cov.Unwatched != 2 || !cov.OverBudget {
		t.Fatalf("Coverage() = %+v, want three watched and two given up", cov)
	}
}

// The priority order must survive startup: on a root whose budget is spent,
// a directory that gains notes later has to displace a directory holding
// files but no note. Reported in the model review of #26, whose scenario
// this is.
func TestNotesDirectoryCreatedLaterTakesAWatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "a"), 0o755)
	os.MkdirAll(filepath.Join(root, "b"), 0o755)
	os.MkdirAll(filepath.Join(root, "c"), 0o755)
	os.WriteFile(filepath.Join(root, "a", "a.md"), []byte("# a"), 0o644)
	os.WriteFile(filepath.Join(root, "b", "b.txt"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(root, "c", "c.txt"), []byte("c"), 0o644)

	dirs, err := tree.Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(root, dirs, t.Logf, WithDebounce(50*time.Millisecond, 300*time.Millisecond), WithBudget(3))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "b"}) {
		t.Fatalf("watched = %v, want the root, the notes directory and one other", got)
	}

	// A new directory with notes in it, as the review's scenario had it.
	os.MkdirAll(filepath.Join(root, "d"), 0o755)
	os.WriteFile(filepath.Join(root, "d", "n.md"), []byte("# dn"), 0o644)
	next(t, w)
	// The relisting runs at flush; give it a moment to settle.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if reflect.DeepEqual(watchedRel(t, w, root), []string{".", "a", "d"}) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "d"}) {
		t.Fatalf("watched = %v, want d to hold a watch and b to have given one up", got)
	}
	// Drain whatever the creations produced, then prove d is live.
	for {
		select {
		case <-w.Events():
			continue
		case <-time.After(100 * time.Millisecond):
		}
		break
	}
	os.WriteFile(filepath.Join(root, "d", "second.md"), []byte("# d2"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"d/second.md"}) {
		t.Fatalf("second note in the new directory: %v", b.Paths)
	}
}

// A placeholder tree is watched at every level, and the first note in each
// of its directories is reported live. This is M2-R4's sentence for the
// nested shape, end to end from the listing through the watcher.
func TestFirstNoteInANestOfPlaceholderDirectories(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "control.md"), []byte("# c"), 0o644)
	os.MkdirAll(filepath.Join(root, "nest", "deep"), 0o755)
	os.WriteFile(filepath.Join(root, "nest", ".gitkeep"), nil, 0o644)
	os.WriteFile(filepath.Join(root, "nest", "deep", ".gitkeep"), nil, 0o644)

	dirs, err := tree.Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(root, dirs, t.Logf, WithDebounce(50*time.Millisecond, 300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)

	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "nest", "nest/deep"}) {
		t.Fatalf("watched = %v, want the nest at both levels", got)
	}
	if cov := w.Coverage(); cov.Limited || cov.Watched != 3 {
		t.Fatalf("Coverage() = %+v, want three directories covered", cov)
	}
	os.WriteFile(filepath.Join(root, "nest", "first.md"), []byte("# f"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"nest/first.md"}) {
		t.Fatalf("first note in the placeholder directory: %v", b.Paths)
	}
	os.WriteFile(filepath.Join(root, "nest", "deep", "first.md"), []byte("# d"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"nest/deep/first.md"}) {
		t.Fatalf("first note one level down: %v", b.Paths)
	}
}

// The ignored tree the narrowed guard was written for stays out, now
// because ripgrep never lists its dotfiles rather than because the walker
// refuses to look at them.
func TestIgnoredDotfileTreeStillCostsOneWatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("cache/\n"), 0o644)
	os.WriteFile(filepath.Join(root, "note.md"), []byte("# n"), 0o644)
	for i := 1; i <= 200; i++ {
		d := filepath.Join(root, "cache", fmt.Sprintf("d%d", i))
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, ".lock"), nil, 0o644)
	}
	dirs, err := tree.Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(root, dirs, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("watched = %v, want the root alone", got)
	}
}

// A deleted subtree hands its budget back: the directories that were
// refused for want of it get watched, and the reported coverage stops
// describing a tree that is gone.
func TestDeletionReturnsBudgetToRefusedDirectories(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	root := t.TempDir()
	for _, d := range []string{"a", "b", "c"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
		os.WriteFile(filepath.Join(root, d, "n.md"), []byte("# n"), 0o644)
	}
	dirs, err := tree.Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(root, dirs, t.Logf, WithDebounce(50*time.Millisecond, 300*time.Millisecond), WithBudget(3))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(20 * time.Millisecond)
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "b"}) {
		t.Fatalf("watched = %v", got)
	}
	if cov := w.Coverage(); cov.Watched != 3 || cov.Unwatched != 1 {
		t.Fatalf("Coverage() = %+v, want c refused", cov)
	}

	os.RemoveAll(filepath.Join(root, "a"))
	next(t, w)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if reflect.DeepEqual(watchedRel(t, w, root), []string{".", "b", "c"}) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "b", "c"}) {
		t.Fatalf("watched = %v, want c to have taken the budget a gave back", got)
	}
	if cov := w.Coverage(); cov.Limited || cov.Watched != 3 || cov.Unwatched != 0 {
		t.Fatalf("Coverage() = %+v, want a complete coverage of what is left", cov)
	}
	for {
		select {
		case <-w.Events():
			continue
		case <-time.After(100 * time.Millisecond):
		}
		break
	}
	os.WriteFile(filepath.Join(root, "c", "second.md"), []byte("# s"), 0o644)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"c/second.md"}) {
		t.Fatalf("the directory that took the freed budget: %v", b.Paths)
	}
}

// A watch left over from a directory the set no longer names is not part of
// the root's coverage: watched plus unwatched is the size of the set.
func TestCoverageCountsOnlyTheCurrentSet(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "gone"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	w, err := New(root, group(tree.GroupNotes, "a", "gone"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if cov := w.Coverage(); cov.Watched != 3 {
		t.Fatalf("Coverage() = %+v, want three watched", cov)
	}
	// "gone" drops out of the listing — an ignore rule now covers it — but
	// keeps its kernel watch until something needs the room.
	w.place(group(tree.GroupNotes, "a"))
	cov := w.Coverage()
	if cov.Watched != 2 || cov.Unwatched != 0 || cov.Limited {
		t.Fatalf("Coverage() = %+v, want two watched of a two-directory set", cov)
	}
	if held := len(w.fsw.WatchList()); held != 3 {
		t.Fatalf("%d kernel watches, want the stale one still held and reclaimable", held)
	}
}
