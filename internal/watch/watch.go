// Package watch reports file changes under a root as debounced batches of
// relative paths, watching the directories a note can appear in, within a
// budget so that one large ad-hoc root cannot exhaust the kernel's watches.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/davison/md-notes/internal/tree"
)

// Batch is one debounced set of changed paths, relative to the root with
// forward slashes, sorted and unique. A renamed directory appears as the
// directory path, so consumers refresh anything beneath it. An empty
// Paths means "anything may have changed": a batch was lost and the
// consumer should refresh everything.
type Batch struct {
	Paths []string `json:"paths"`
}

// Watcher watches one root.
type Watcher struct {
	root     string
	fsw      *fsnotify.Watcher
	out      chan Batch
	warnf    func(string, ...any)
	debounce time.Duration
	maxWait  time.Duration
	budget   int
	done     chan struct{}
	once     sync.Once
	lost     bool

	cmu sync.Mutex
	cov Coverage
}

// Coverage says how much of a root the watcher covers. An unwatched
// directory is not invisible: its changes are still seen when a watched
// directory above it reports them, and when the daemon restarts. It simply
// does not report changes of its own.
type Coverage struct {
	// Watched is the number of directories carrying a watch.
	Watched int `json:"watched"`
	// Unwatched is how many directories of the root's set carry none.
	Unwatched int `json:"unwatched"`
	// Budget is the per-root maximum, or zero when there is none.
	Budget int `json:"budget"`
	// OverBudget reports that the budget, rather than an error, is what
	// left directories unwatched.
	OverBudget bool `json:"overBudget"`
	// Failed counts directories the kernel refused, which on Linux means
	// fs.inotify.max_user_watches is exhausted.
	Failed int `json:"failed"`
	// Limited is Unwatched > 0: live update covers part of the root.
	Limited bool `json:"limited"`
}

// Option adjusts a Watcher.
type Option func(*Watcher)

// WithDebounce sets the quiet period before a batch is sent and the
// longest a busy stream may delay one.
func WithDebounce(quiet, maxWait time.Duration) Option {
	return func(w *Watcher) { w.debounce, w.maxWait = quiet, maxWait }
}

// WithBudget caps the directories watched for this root. Watches are placed
// in the order dirs arrives in, so a budget too small for the root covers
// its most valuable directories; zero or less removes the cap.
func WithBudget(n int) Option {
	return func(w *Watcher) { w.budget = n }
}

// New watches root and the relative directories in dirs, most valuable
// first (the root itself is always watched, whatever the budget).
// Directories created later inside a watched one are added as they appear.
// A spent budget and a kernel out of inotify watches are both reported
// through warnf and through Coverage, leaving the rest unwatched rather
// than failing.
func New(root string, dirs []tree.Dir, warnf func(string, ...any), opts ...Option) (*Watcher, error) {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		root:     root,
		fsw:      fsw,
		out:      make(chan Batch, 16),
		warnf:    warnf,
		debounce: 150 * time.Millisecond,
		maxWait:  time.Second,
		done:     make(chan struct{}),
	}
	for _, o := range opts {
		o(w)
	}
	w.place(dirs)
	go w.loop()
	return w, nil
}

// place brings the watch set in line with dirs, the root's whole directory
// set ordered most valuable first, and records the coverage that results.
// The root is watched first whether or not dirs names it, and is never
// given up.
//
// Within the budget it simply watches what is not watched yet. Once the
// budget is spent, a directory that outranks a watched one takes its watch:
// the lowest-priority watch is released to make room. Without that the
// order would hold at startup only, and every notes directory created
// afterwards on a full root would stay dark until the daemon restarted,
// losing to directories the policy ranks below it.
func (w *Watcher) place(dirs []tree.Dir) {
	all := append([]tree.Dir{{Path: "", Group: tree.GroupNotes}}, dirs...)
	watched := map[string]struct{}{}
	for _, p := range w.fsw.WatchList() {
		watched[p] = struct{}{}
	}
	root := filepath.Clean(w.root)

	// Rank every directory of the set, so a watch already in place can be
	// compared with one asking for room. A watched directory the set no
	// longer names — one that became ignored, say — ranks below them all.
	rank := make(map[string]tree.Group, len(all))
	for _, d := range all {
		rank[filepath.Join(w.root, filepath.FromSlash(d.Path))] = d.Group
	}
	givable := make([]string, 0, len(watched))
	for p := range watched {
		if p != root {
			givable = append(givable, p)
		}
	}
	// Least valuable first, and deterministic, so the same watch is the
	// first to go every time the set is recomputed: a directory the set no
	// longer names, then the lowest group, then the last path within it.
	sort.Slice(givable, func(i, j int) bool {
		gi, oki := rank[givable[i]]
		gj, okj := rank[givable[j]]
		if oki != okj {
			return okj
		}
		if gi != gj {
			return gi > gj
		}
		return givable[i] > givable[j]
	})

	// One error stands for all of them: a root with hundreds of
	// unwatchable directories logs one line, not hundreds.
	var first error
	cov := Coverage{Budget: max(w.budget, 0)}
	for _, d := range all {
		rel := d.Path
		if rel == "." {
			rel = ""
		}
		abs := filepath.Join(w.root, filepath.FromSlash(rel))
		if _, ok := watched[abs]; ok {
			continue
		}
		if w.budget > 0 && len(watched) >= w.budget {
			// Reclaim a watch from a directory this one outranks. Equal
			// groups never displace each other, so a stable set never
			// churns its watches between recomputations.
			victim := ""
			for len(givable) > 0 {
				cand := givable[0]
				if _, still := watched[cand]; !still {
					givable = givable[1:]
					continue
				}
				if g, ok := rank[cand]; ok && g <= d.Group {
					// The least valuable watch left is worth as much as
					// this directory, so nothing here gives way.
					break
				}
				givable = givable[1:]
				victim = cand
				break
			}
			if victim == "" {
				cov.OverBudget = true
				cov.Unwatched++
				continue
			}
			w.fsw.Remove(victim)
			delete(watched, victim)
		}
		placed, err := w.add(abs)
		if err != nil {
			if first == nil {
				first = err
			}
			cov.Failed++
			cov.Unwatched++
			continue
		}
		if placed {
			watched[abs] = struct{}{}
		}
	}
	cov.Watched = len(watched)
	cov.Limited = cov.Unwatched > 0

	w.cmu.Lock()
	changed := cov != w.cov
	w.cov = cov
	w.cmu.Unlock()
	if changed {
		w.report(cov, first)
	}
}

// Coverage reports how much of the root is watched.
func (w *Watcher) Coverage() Coverage {
	w.cmu.Lock()
	defer w.cmu.Unlock()
	return w.cov
}

// report logs one line for a limited coverage, whatever the number of
// directories behind it, saying why and what it costs.
func (w *Watcher) report(cov Coverage, first error) {
	if !cov.Limited {
		return
	}
	var why []string
	if cov.OverBudget {
		why = append(why, fmt.Sprintf("the budget of %d directories is spent (raise max_watches to cover more)", cov.Budget))
	}
	if cov.Failed > 0 {
		why = append(why, fmt.Sprintf("%d could not be watched: %v (raise fs.inotify.max_user_watches)", cov.Failed, first))
	}
	w.warnf("watch %s: live update covers %d director%s, %d unwatched — %s; a change in an unwatched directory is seen when a watched one reports it or the daemon restarts",
		w.root, cov.Watched, plural(cov.Watched), cov.Unwatched, strings.Join(why, "; "))
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// Events delivers batches until Close.
func (w *Watcher) Events() <-chan Batch { return w.out }

// Close stops the watcher and closes Events.
func (w *Watcher) Close() error {
	var err error
	w.once.Do(func() {
		err = w.fsw.Close()
		<-w.done
	})
	return err
}

// add watches one directory and reports whether a watch was placed. A
// directory that vanished before the watch was placed is not an error, and
// costs nothing against the budget.
func (w *Watcher) add(abs string) (bool, error) {
	err := w.fsw.Add(abs)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// addMissing recomputes the directories the root's listing would cover and
// watches any that are not watched yet, within the budget. Runs when a
// batch shows a directory appeared or moved. Listing the whole root is what
// the navigator does for the same batch, so the cost is already being paid;
// listing only the new directory would not work, because ripgrep never
// applies ignore rules to a directory it is pointed at, so an ignored tree
// created at runtime would be watched in full.
func (w *Watcher) addMissing() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dirs, err := tree.Dirs(ctx, w.root, w.warnf)
	if err != nil {
		w.warnf("watch %s: relisting failed (%v); watching every non-hidden directory", w.root, err)
		dirs = Walk(w.root)
	}
	w.place(dirs)
}

func (w *Watcher) loop() {
	defer close(w.done)
	defer close(w.out)
	pending := map[string]struct{}{}
	var timer *time.Timer
	var timerC <-chan time.Time
	var first time.Time

	arm := func(d time.Duration) {
		if timer == nil {
			timer = time.NewTimer(d)
		} else {
			timer.Stop()
			timer.Reset(d)
		}
		timerC = timer.C
	}

	flush := func() {
		timerC = nil
		if len(pending) == 0 && !w.lost {
			return
		}
		w.reconcile(pending)
		b := Batch{Paths: make([]string, 0, len(pending))}
		for p := range pending {
			b.Paths = append(b.Paths, p)
		}
		sort.Strings(b.Paths)
		pending = map[string]struct{}{}
		if w.lost {
			// Something was dropped earlier: tell the consumer to refresh
			// everything rather than trust this batch alone.
			b.Paths = []string{}
		}
		select {
		case w.out <- b:
			w.lost = false
		default:
			if !w.lost {
				w.warnf("watch %s: consumer not reading; it will be asked for a full refresh", w.root)
			}
			w.lost = true
			arm(w.debounce)
		}
	}

	for {
		select {
		case ev, ok := <-w.fsw.Events:
			if !ok {
				flush()
				return
			}
			if ev.Op == fsnotify.Chmod {
				continue
			}
			rel, err := filepath.Rel(w.root, ev.Name)
			if err != nil || hidden(rel) {
				continue
			}
			rel = filepath.ToSlash(rel)
			if len(pending) == 0 {
				first = time.Now()
			}
			pending[rel] = struct{}{}
			wait := w.debounce
			if remaining := w.maxWait - time.Since(first); remaining < wait {
				wait = max(remaining, 0)
			}
			arm(wait)
		case <-timerC:
			flush()
		case err, ok := <-w.fsw.Errors:
			if !ok {
				flush()
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				// The kernel dropped events: whatever they were, the
				// consumer must refresh everything.
				w.lost = true
				arm(w.debounce)
			}
			w.warnf("watch %s: %v", w.root, err)
		}
	}
}

// reconcile brings the watch set in line with the paths of a batch. A path
// that no longer exists loses its watches, and a directory that exists is
// re-added, which resets the path fsnotify reports for a moved directory
// whatever order the rename and create events arrived in.
//
// Removals run before additions. After a rename the old and new names of
// a subdirectory share one inode, and inotify one watch, so removing the
// old name after the new one was added would silence the new one.
func (w *Watcher) reconcile(paths map[string]struct{}) {
	newDir := false
	var watched []string // fetched once, only if something vanished
	for p := range paths {
		abs := filepath.Join(w.root, filepath.FromSlash(p))
		info, err := os.Lstat(abs)
		if err != nil {
			if watched == nil {
				watched = w.fsw.WatchList()
			}
			for _, x := range watched {
				if x == abs || strings.HasPrefix(x, abs+string(filepath.Separator)) {
					w.fsw.Remove(x)
				}
			}
			continue
		}
		if info.IsDir() {
			// Drop any watch under a stale name for this inode before the
			// listing re-adds it under the current one.
			w.fsw.Remove(abs)
			newDir = true
		}
	}
	if newDir {
		w.addMissing()
	}
}

// hidden reports whether any component of a relative path starts with a
// dot, which is what the navigator skips too.
func hidden(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

// Walk returns every non-hidden directory under root, relative to it, for
// use when no ignore-aware listing is available. Without ripgrep there is
// nothing to rank them by, so they all share one group and none displaces
// another; a budget is then spent in the order the walk finds them.
func Walk(root string) []tree.Dir {
	var dirs []tree.Dir
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		r, _ := filepath.Rel(root, p)
		if r == "." {
			r = ""
		}
		dirs = append(dirs, tree.Dir{Path: filepath.ToSlash(r), Group: tree.GroupOther})
		return nil
	})
	return dirs
}
