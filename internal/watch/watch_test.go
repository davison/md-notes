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
	"syscall"
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

// The tests come in two kinds, and only the second one waits.
//
// A test of the debounce itself — what a burst collapses to, when the quiet
// window ends one, what maxWait caps — runs on an injected fake clock and
// feeds its own events, so it states a property of the debounce and finishes
// in microseconds. Nothing in that kind is timing-dependent.
//
// A test of what the kernel reports, or of what the watch set becomes after a
// relisting, has to wait for something outside the process. Those use the
// bounds below. Each is one-sided: it bounds how long the test is prepared to
// wait for something it expects, so a slow machine makes the test slower and
// never wrong.
//
// No test of the debounce concludes an absence from the wall clock: those
// assertions are made through the fake clock, which says when every batch it
// owes has been delivered. Three tests of the relisting still do — the drain
// loops that take 100ms of silence for "ripgrep has finished", before proving
// that a directory is live. They wait on ripgrep rather than on the debounce,
// they are unchanged here, and their 100ms is a raw literal rather than a
// named bound like the two below. They are the honest exception to the
// paragraph above.
const (
	// batchWait is how long a test waits for a batch the kernel owes it,
	// against a 50ms quiet window: twenty times the window, which is the
	// scheduling delay that would have to land to fail the test wrongly.
	batchWait = time.Second
	// settleWait bounds a relisting, which shells out to ripgrep and can
	// therefore be arbitrarily slow on a loaded machine. Polled, not slept:
	// the cost is paid only when the machine is slow.
	settleWait = 3 * time.Second
)

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
	// No settling wait: New places every watch with inotify_add_watch before
	// it returns, and fsnotify's event channel is unbuffered, so an event
	// that arrives before the loop starts waits for it rather than being
	// lost. The 20ms sleep this replaces bought nothing.
	return w, root
}

// quiet and maxWait are what the deterministic tests configure. Their values
// no longer matter to anything but the arithmetic in the tests themselves:
// the fake clock reaches them instantly.
const (
	quiet   = 50 * time.Millisecond
	maxWait = 300 * time.Millisecond
)

// newFakeClockWatcher starts a watcher over an empty root whose debounce runs
// on a clock the test moves itself. Nothing is written to the root, so every
// event the watcher sees is one the test sent.
func newFakeClockWatcher(t *testing.T) (*Watcher, *fakeClock, string) {
	t.Helper()
	root := t.TempDir()
	c := newFakeClock()
	w, err := New(root, nil, t.Logf, WithDebounce(quiet, maxWait), withClock(c))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w, c, root
}

// send delivers one event to the watcher's loop as the kernel would.
// fsnotify's event channel is unbuffered, so the send completes only once the
// loop has taken the event.
func send(w *Watcher, root, rel string, op fsnotify.Op) {
	w.fsw.Events <- fsnotify.Event{Name: filepath.Join(root, filepath.FromSlash(rel)), Op: op}
}

// handled returns once the loop has finished handling every event sent before
// it. The loop takes events one at a time, so its receipt of this ignored
// Chmod is proof that the event before it was handled to completion — its
// path recorded and its timer armed. Call it after sending and before moving
// the clock; it says nothing about a flush, which competes with the event
// channel in the same select.
func handled(w *Watcher, root string) {
	send(w, root, "settle", fsnotify.Chmod)
}

func next(t *testing.T, w *Watcher) Batch {
	t.Helper()
	select {
	case b, ok := <-w.Events():
		if !ok {
			t.Fatal("events closed")
		}
		return b
	case <-time.After(batchWait):
		t.Fatalf("no batch within %v", batchWait)
	}
	return Batch{}
}

// noBatchYet asserts that nothing has been emitted. It does not wait on the
// wall clock and it does not need to: Advance returns only once the loop has
// taken every fire it caused, and handled returns only once the loop is back
// at its select, so a batch the watcher had decided to send has been sent by
// the time the receive below runs. A batch that is not there is not there.
func noBatchYet(t *testing.T, w *Watcher, root string) {
	t.Helper()
	handled(w, root)
	select {
	case b := <-w.Events():
		t.Fatalf("unexpected batch %v", b.Paths)
	default:
	}
}

// waitFor polls until cond holds or settleWait runs out, and reports whether
// it held. For the relistings that run out of band after a flush.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(settleWait)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
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

// A burst of changes to one file is one batch, and nothing follows it.
//
// The version of this test that wrote the file twenty times and then waited
// asserted a property of the machine, not of the debounce: any pause longer
// than the quiet window splits the burst into two batches, both of them
// correct, and the second one failed the "nothing follows" assertion. That is
// what failed CI four times, three of them on branches carrying no Go at all
// (#46). Here the events are delivered directly and the quiet window is moved
// by the test, so the burst is a burst by construction.
func TestBurstIsOneBatch(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	for i := 0; i < 20; i++ {
		send(w, root, "n.md", fsnotify.Write)
	}
	handled(w, root)
	noBatchYet(t, w, root) // no quiet window has elapsed, so nothing is out
	c.Advance(quiet)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"n.md"}) {
		t.Fatalf("batch = %v", b.Paths)
	}
	// However long the stream is left alone for, there is no second batch.
	c.Advance(time.Hour)
	noBatchYet(t, w, root)
}

// The batch is held for the whole quiet window and released the moment it
// ends. A wall-clock test can only bracket this; the fake clock states it.
func TestBatchWaitsForTheWholeQuietWindow(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	send(w, root, "n.md", fsnotify.Write)
	handled(w, root)
	c.Advance(quiet - time.Millisecond)
	noBatchYet(t, w, root)
	c.Advance(time.Millisecond)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"n.md"}) {
		t.Fatalf("batch = %v", b.Paths)
	}
}

// Every change restarts the quiet window, which is what makes a burst one
// batch however long the burst runs for.
//
// Three changes, not two, and each one lands after the window the change
// before it opened would have ended. A watcher that armed the window only for
// the change that starts a batch would let the first two go at 50ms and carry
// the third alone, so the first batch is wrong in its contents and not merely
// early: the assertion holds whether or not the absence checks see anything.
func TestEachChangeRestartsTheQuietWindow(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	send(w, root, "a.md", fsnotify.Create)
	handled(w, root)
	c.Advance(quiet - 10*time.Millisecond)
	noBatchYet(t, w, root)

	// The window a.md opened would have ended at 50ms; b.md at 40ms moves it
	// to 90ms.
	send(w, root, "b.md", fsnotify.Create)
	handled(w, root)
	c.Advance(10 * time.Millisecond)
	noBatchYet(t, w, root)

	// And c.md at 50ms moves it to 100ms.
	send(w, root, "c.md", fsnotify.Create)
	handled(w, root)
	c.Advance(quiet - 10*time.Millisecond)
	noBatchYet(t, w, root)

	c.Advance(10 * time.Millisecond)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"a.md", "b.md", "c.md"}) {
		t.Fatalf("batch = %v, want all three changes in one batch", b.Paths)
	}
	noBatchYet(t, w, root)
}

// A stream that never goes quiet still yields a batch: maxWait caps how long
// a busy directory may hold one back. Production behaviour with no test until
// the clock could be injected, because testing it meant sleeping for it.
func TestBusyStreamFlushesAtMaxWait(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	var want []string
	// A change every 40ms against a 50ms window: the quiet window never
	// elapses, so only the 300ms cap can end the batch.
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("n%d.md", i)
		want = append(want, name)
		send(w, root, name, fsnotify.Write)
		handled(w, root)
		c.Advance(40 * time.Millisecond)
		if i < 7 {
			noBatchYet(t, w, root) // 280ms of stream, and no 50ms of quiet
		}
	}
	// The eighth change lands at 280ms, so its window is cut to the 20ms the
	// cap has left.
	if b := next(t, w); !reflect.DeepEqual(b.Paths, want) {
		t.Fatalf("batch = %v, want the whole stream capped at maxWait: %v", b.Paths, want)
	}
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
	// The relisting runs out of band, after a flush; wait for its result
	// rather than for a fixed number of milliseconds.
	if !waitFor(func() bool {
		for _, p := range w.fsw.WatchList() {
			if strings.HasSuffix(p, filepath.Join("src", "notes")) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("src/notes never watched; watching %v", w.fsw.WatchList())
	}
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

// A consumer that stops reading loses a batch, and is told to refresh
// everything rather than left believing the batches it did get were all of
// them. The old version of this test paced forty writes with sleeps and hoped
// one batch would be dropped; here the seventeenth batch is dropped because
// the buffer holds sixteen, which is arithmetic rather than timing.
func TestLostBatchAsksForFullRefresh(t *testing.T) {
	root := t.TempDir()
	c := newFakeClock()
	warned := make(chan string, 8)
	w, err := New(root, nil, func(f string, a ...any) {
		select {
		case warned <- fmt.Sprintf(f, a...):
		default:
		}
	}, WithDebounce(quiet, maxWait), withClock(c))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })

	buffered := cap(w.out)
	// One batch per change and nothing read: the buffer takes the first
	// cap(out) of them and the next has nowhere to go. Which batch is
	// dropped is arithmetic here, where the old test paced forty writes with
	// sleeps and hoped.
	for i := 0; i <= buffered; i++ {
		send(w, root, fmt.Sprintf("f%d.md", i), fsnotify.Create)
		handled(w, root)
		c.Advance(quiet)
		if i < buffered {
			// The batch reaching the buffer is the flush completing, so the
			// next change is sent after it and never joins this batch.
			if !waitFor(func() bool { return len(w.out) > i }) {
				t.Fatalf("%d batches buffered after %d changes", len(w.out), i+1)
			}
		}
	}
	// The warning is the watcher saying it could not deliver, and reading it
	// is what proves the drop happened before the buffer was drained.
	select {
	case msg := <-warned:
		if !strings.Contains(msg, "consumer not reading") {
			t.Fatalf("warning = %q, want the undelivered batch", msg)
		}
	case <-time.After(batchWait):
		t.Fatal("no warning that a batch could not be delivered")
	}
	for i := 0; i < buffered; i++ {
		if b := next(t, w); len(b.Paths) != 1 {
			t.Fatalf("batch %d = %v, want the one change it was made of", i, b.Paths)
		}
	}
	// handled returns once the loop is back at its select, so the rearming
	// the drop did has happened before the clock is moved again.
	handled(w, root)
	c.Advance(quiet)
	if b := next(t, w); len(b.Paths) != 0 {
		t.Fatalf("after a dropped batch got %v, want an empty full-refresh batch", b.Paths)
	}
}

// The kernel dropping events means the consumer cannot trust what it has, so
// it is asked to refresh everything.
func TestOverflowAsksForFullRefresh(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	w.fsw.Errors <- fsnotify.ErrEventOverflow
	// The error and the events share one select, so an event taken after it
	// proves the overflow was handled and its timer armed.
	handled(w, root)
	noBatchYet(t, w, root)
	c.Advance(quiet)
	if b := next(t, w); len(b.Paths) != 0 {
		t.Fatalf("after overflow got %v, want an empty full-refresh batch", b.Paths)
	}
}

// A change to a hidden file or inside a hidden directory is not reported. The
// sentinel is what makes the absence provable: a batch for a hidden path
// would have to arrive before it, so the assertion fails on what was sent
// rather than on how long the test was willing to wait.
func TestHiddenIgnored(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	for _, rel := range []string{".swp", ".git", ".git/HEAD", "sub/.hidden.md"} {
		send(w, root, rel, fsnotify.Create)
	}
	send(w, root, "visible.md", fsnotify.Create)
	handled(w, root)
	c.Advance(quiet)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"visible.md"}) {
		t.Fatalf("batch = %v, want the hidden paths dropped", b.Paths)
	}
	noBatchYet(t, w, root)
}

// A permission change is not a content change, so it is not reported.
func TestChmodIgnored(t *testing.T) {
	w, c, root := newFakeClockWatcher(t)
	send(w, root, "a.md", fsnotify.Chmod)
	send(w, root, "b.md", fsnotify.Write)
	handled(w, root)
	c.Advance(quiet)
	if b := next(t, w); !reflect.DeepEqual(b.Paths, []string{"b.md"}) {
		t.Fatalf("batch = %v, want the chmod dropped", b.Paths)
	}
	noBatchYet(t, w, root)
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

// A directory the filesystem refuses is not the kernel running out of
// watches, and must not be reported as it: raising
// fs.inotify.max_user_watches will never make a directory at mode 000
// readable. The reader is told the real reason instead (M8-R7, #33).
func TestUnreadableDirectoryReportsItsOwnCause(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory mode, so nothing is refused")
	}
	root := t.TempDir()
	dirs := []string{"ok", "locked"}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	locked := filepath.Join(root, "locked")
	// Restored before t.TempDir's own cleanup, which runs after this one and
	// cannot remove a directory it may not read.
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}

	var logged []string
	w, err := New(root, group(tree.GroupNotes, dirs...), func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	cov := w.Coverage()
	want := Coverage{Watched: 2, Unwatched: 1, Refused: 1, Reason: "permission denied", Limited: true}
	if cov != want {
		t.Fatalf("Coverage() = %+v, want %+v", cov, want)
	}
	if len(logged) != 1 {
		t.Fatalf("log = %q, want one line", logged)
	}
	line := logged[0]
	for _, want := range []string{"1 unwatched", "permission denied", "not the kernel limit", "needs access to that directory"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q does not say %q", line, want)
		}
	}
	if strings.Contains(line, "max_user_watches") || strings.Contains(line, "max_watches") {
		t.Errorf("log line %q offers a limit to raise for a refusal no limit answers", line)
	}
}

// The three causes are reported as themselves, and only the kernel's own
// limit carries the sysctl remedy.
func TestReportNamesEachCause(t *testing.T) {
	var logged []string
	w, err := New(t.TempDir(), nil, func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	cases := []struct {
		name       string
		overBudget bool
		errs       []error
		wantCov    Coverage
		want       []string
		notWant    []string
	}{{
		name:       "the budget is spent",
		overBudget: true,
		wantCov:    Coverage{Budget: 3, OverBudget: true, Watched: 3, Unwatched: 2, Limited: true},
		want:       []string{"budget of 3 directories is spent", "raise max_watches"},
		notWant:    []string{"could not be watched"},
	}, {
		name:    "the kernel is out of watches",
		errs:    []error{syscall.ENOSPC},
		wantCov: Coverage{Budget: 3, Watched: 3, Unwatched: 2, Failed: 1, Limited: true},
		want:    []string{"1 could not be watched: no space left on device", "raise fs.inotify.max_user_watches"},
		notWant: []string{"not the kernel limit", "budget of 3"},
	}, {
		name:    "the filesystem refuses them",
		errs:    []error{syscall.EACCES, syscall.EACCES},
		wantCov: Coverage{Budget: 3, Watched: 3, Unwatched: 2, Refused: 2, Reason: "permission denied", Limited: true},
		want:    []string{"2 could not be watched: permission denied", "not the kernel limit", "needs access to those directories"},
		notWant: []string{"max_user_watches", "budget of 3"},
	}, {
		name:       "all three at once",
		overBudget: true,
		errs:       []error{syscall.ENOSPC, syscall.EACCES},
		wantCov:    Coverage{Budget: 3, OverBudget: true, Watched: 3, Unwatched: 2, Failed: 1, Refused: 1, Reason: "permission denied", Limited: true},
		want:       []string{"budget of 3", "no space left on device", "raise fs.inotify.max_user_watches", "permission denied", "not the kernel limit"},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logged = nil
			cov := Coverage{Budget: 3, OverBudget: c.overBudget, Watched: 3, Unwatched: 2, Limited: true}
			var causes refusals
			for _, err := range c.errs {
				causes.record(&cov, err)
			}
			if cov != c.wantCov {
				t.Fatalf("coverage = %+v, want %+v", cov, c.wantCov)
			}
			w.report(cov, causes)
			if len(logged) != 1 {
				t.Fatalf("log = %q, want one line", logged)
			}
			for _, want := range c.want {
				if !strings.Contains(logged[0], want) {
					t.Errorf("log line %q does not say %q", logged[0], want)
				}
			}
			for _, not := range c.notWant {
				if strings.Contains(logged[0], not) {
					t.Errorf("log line %q says %q, which is not its cause", logged[0], not)
				}
			}
		})
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
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "b"}) {
		t.Fatalf("watched = %v, want the root, the notes directory and one other", got)
	}

	// A new directory with notes in it, as the review's scenario had it.
	os.MkdirAll(filepath.Join(root, "d"), 0o755)
	os.WriteFile(filepath.Join(root, "d", "n.md"), []byte("# dn"), 0o644)
	next(t, w)
	// The relisting runs out of band, after the flush.
	waitFor(func() bool { return reflect.DeepEqual(watchedRel(t, w, root), []string{".", "a", "d"}) })
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
	if got := watchedRel(t, w, root); !reflect.DeepEqual(got, []string{".", "a", "b"}) {
		t.Fatalf("watched = %v", got)
	}
	if cov := w.Coverage(); cov.Watched != 3 || cov.Unwatched != 1 {
		t.Fatalf("Coverage() = %+v, want c refused", cov)
	}

	os.RemoveAll(filepath.Join(root, "a"))
	next(t, w)
	waitFor(func() bool { return reflect.DeepEqual(watchedRel(t, w, root), []string{".", "b", "c"}) })
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
