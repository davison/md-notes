// Package watch reports file changes under a root as debounced batches of
// relative paths, watching only the directories the navigator would list.
package watch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Batch is one debounced set of changed paths, relative to the root with
// forward slashes, sorted and unique. A renamed directory appears as the
// directory path, so consumers refresh anything beneath it.
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
	w.add("")
	for _, d := range dirs {
		if d != "" && d != "." {
			w.add(d)
		}
	}
	go w.loop()
	return w, nil
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

func (w *Watcher) add(rel string) {
	abs := filepath.Join(w.root, filepath.FromSlash(rel))
	if err := w.fsw.Add(abs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		w.warnf("watch %s: %v", abs, err)
	}
}

// addTree watches rel and every non-hidden directory beneath it.
func (w *Watcher) addTree(rel string) {
	base := filepath.Join(w.root, filepath.FromSlash(rel))
	filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != base && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		r, _ := filepath.Rel(w.root, p)
		w.add(filepath.ToSlash(r))
		return nil
	})
}

func (w *Watcher) loop() {
	defer close(w.done)
	defer close(w.out)
	pending := map[string]struct{}{}
	var timer *time.Timer
	var timerC <-chan time.Time
	var first time.Time

	flush := func() {
		if len(pending) == 0 {
			return
		}
		b := Batch{Paths: make([]string, 0, len(pending))}
		for p := range pending {
			b.Paths = append(b.Paths, p)
		}
		sort.Strings(b.Paths)
		pending = map[string]struct{}{}
		timerC = nil
		select {
		case w.out <- b:
		default:
			// A consumer that is not reading loses this batch; the next one
			// will bring it up to date.
			w.warnf("watch %s: dropping change batch, consumer not reading", w.root)
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
			if ev.Has(fsnotify.Create) {
				if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
					w.addTree(rel)
				}
			}
			if len(pending) == 0 {
				first = time.Now()
			}
			pending[rel] = struct{}{}
			wait := w.debounce
			if remaining := w.maxWait - time.Since(first); remaining < wait {
				wait = max(remaining, 0)
			}
			if timer == nil {
				timer = time.NewTimer(wait)
			} else {
				timer.Stop()
				timer.Reset(wait)
			}
			timerC = timer.C
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
