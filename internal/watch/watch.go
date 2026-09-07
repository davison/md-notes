// Package watch reports file changes under a root as debounced batches of
// relative paths, watching only the directories the navigator would list.
package watch

import (
	"context"
	"errors"
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
	done     chan struct{}
	once     sync.Once
	lost     bool
}

// Option adjusts a Watcher.
type Option func(*Watcher)

// WithDebounce sets the quiet period before a batch is sent and the
// longest a busy stream may delay one.
func WithDebounce(quiet, maxWait time.Duration) Option {
	return func(w *Watcher) { w.debounce, w.maxWait = quiet, maxWait }
}

// New watches root and the relative directories in dirs (the root itself
// is always watched). Directories created later inside a watched one are
// added as they appear. Running out of inotify watches is reported through
// warnf and leaves the rest unwatched rather than failing.
func New(root string, dirs []string, warnf func(string, ...any), opts ...Option) (*Watcher, error) {
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
	failed := w.addAll(dirs)
	failed.report(w, "")
	go w.loop()
	return w, nil
}

// failures aggregates watch errors so a root with hundreds of unwatchable
// directories logs one line, not hundreds.
type failures struct {
	n     int
	first error
}

func (f *failures) note(err error) {
	if f.first == nil {
		f.first = err
	}
	f.n++
}

func (f failures) report(w *Watcher, under string) {
	if f.n == 0 {
		return
	}
	where := w.root
	if under != "" {
		where = filepath.Join(w.root, filepath.FromSlash(under))
	}
	w.warnf("watch %s: %d director%s could not be watched (first error: %v); changes there will not be seen",
		where, f.n, map[bool]string{true: "y", false: "ies"}[f.n == 1], f.first)
}

// addAll watches the root and each relative directory in dirs.
func (w *Watcher) addAll(dirs []string) failures {
	var f failures
	if err := w.add(""); err != nil {
		f.note(err)
	}
	for _, d := range dirs {
		if d == "" || d == "." {
			continue
		}
		if err := w.add(d); err != nil {
			f.note(err)
		}
	}
	return f
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

// add watches one relative directory. A directory that vanished before
// the watch was placed is not an error.
func (w *Watcher) add(rel string) error {
	abs := filepath.Join(w.root, filepath.FromSlash(rel))
	if err := w.fsw.Add(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// addMissing recomputes the directories the root's listing would cover
// and watches any that are not watched yet. Runs when a batch shows a
// directory appeared or moved. Listing the whole root is what the
// navigator does for the same batch, so the cost is already being paid;
// listing only the new directory would not work, because ripgrep never
// applies ignore rules to a directory it is pointed at, so an ignored tree
// created at runtime would be watched in full.
func (w *Watcher) addMissing() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dirs, err := tree.Dirs(ctx, w.root, nil)
	if err != nil {
		dirs = Walk(w.root)
	}
	watched := map[string]struct{}{}
	for _, p := range w.fsw.WatchList() {
		watched[p] = struct{}{}
	}
	var f failures
	for _, d := range dirs {
		abs := filepath.Join(w.root, filepath.FromSlash(d))
		if _, ok := watched[abs]; ok {
			continue
		}
		if err := w.add(d); err != nil {
			f.note(err)
		}
	}
	f.report(w, "")
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
	for p := range paths {
		abs := filepath.Join(w.root, filepath.FromSlash(p))
		info, err := os.Lstat(abs)
		if err != nil {
			for _, watched := range w.fsw.WatchList() {
				if watched == abs || strings.HasPrefix(watched, abs+string(filepath.Separator)) {
					w.fsw.Remove(watched)
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

// Walk returns every non-hidden directory under root, relative to it,
// for use when no ignore-aware listing is available.
func Walk(root string) []string {
	var dirs []string
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
		dirs = append(dirs, filepath.ToSlash(r))
		return nil
	})
	return dirs
}
