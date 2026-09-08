// Package tree lists the markdown files under a root with ripgrep and
// arranges them as a directory tree for the navigator.
package tree

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Node is a directory or a markdown file. Path is relative to the root
// with forward slashes; a directory's Children are directories first, then
// files, each group sorted case-insensitively.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Dir      bool    `json:"dir"`
	Children []*Node `json:"children,omitempty"`
}

// ErrNoRipgrep is returned when the rg binary cannot be found.
var ErrNoRipgrep = errors.New("ripgrep (rg) is not installed or not on PATH")

// lookPath is replaced in tests.
var lookPath = exec.LookPath

// IsMarkdown reports whether a file name has a markdown extension,
// case-insensitively.
func IsMarkdown(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// List returns the markdown files under root, relative to it, sorted by
// path. ripgrep lists every file it would search, so its defaults apply:
// gitignore inside repositories, .ignore and .rgignore anywhere, and hidden
// files and directories skipped. The extension filter is applied here
// rather than with rg's include globs, because an include glob overrides
// ignore rules for the files it matches. Output is streamed, so a large
// root costs memory only for the markdown paths it contains.
//
// If rg reports errors for parts of the tree it could not read, the files
// it did list are returned and the problem goes to warnf (which may be
// nil); only a total failure is an error.
func List(ctx context.Context, root string, warnf func(string, ...any)) ([]string, error) {
	return list(ctx, root, warnf, IsMarkdown)
}

// Dirs returns the directories a watcher should cover, relative to root
// and including the root itself (""), ordered so that the watches worth
// most are placed first when a budget cannot cover them all:
//
//  1. the root, every directory holding a markdown file, and their
//     ancestors — the set the navigator lists, where notes change;
//  2. directories holding nothing the navigator would list — empty ones,
//     and ones holding only hidden files such as a .gitkeep placeholder —
//     where a first note can appear that nothing else would report;
//  3. every other directory holding a file ripgrep lists, which can gain a
//     markdown file later.
//
// Paths are sorted within each group, so the whole result is stable. A
// subtree that has files ripgrep does not list is ignored or hidden and is
// left unwatched.
func Dirs(ctx context.Context, root string, warnf func(string, ...any)) ([]string, error) {
	files, err := list(ctx, root, warnf, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	notes := map[string]struct{}{"": {}}
	others := map[string]struct{}{}
	for _, f := range files {
		set := others
		if IsMarkdown(f) {
			set = notes
		}
		for d := path.Dir(f); d != "." && d != "/" && d != ""; d = path.Dir(d) {
			set[d] = struct{}{}
		}
	}
	for d := range notes {
		delete(others, d)
	}
	empty := map[string]struct{}{}
	for _, d := range sorted(notes, others) {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(d)))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			child := path.Join(d, e.Name())
			if _, ok := notes[child]; ok {
				continue
			}
			if _, ok := others[child]; ok {
				continue
			}
			if _, ok := empty[child]; ok {
				continue
			}
			if dirs, ok := emptySubtree(filepath.Join(root, filepath.FromSlash(child))); ok {
				for _, sub := range dirs {
					empty[path.Join(child, sub)] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(notes)+len(empty)+len(others))
	out = append(out, sorted(notes)...)
	out = append(out, sorted(empty)...)
	out = append(out, sorted(others)...)
	return out, nil
}

// sorted returns the keys of one or more sets as one sorted slice.
func sorted(sets ...map[string]struct{}) []string {
	n := 0
	for _, s := range sets {
		n += len(s)
	}
	out := make([]string, 0, n)
	for _, s := range sets {
		for k := range s {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// emptySubtree walks abs and reports its non-hidden directories (relative
// to abs, "" for abs itself) if the subtree holds nothing the navigator
// would list: no files at all, or only hidden ones such as the .gitkeep
// placeholder that keeps an otherwise empty directory in a git repository.
// Such a directory is as empty as one holding nothing, and the first note
// created in it must be seen.
//
// Ignored files still count as files. Reading them as absent would make
// node_modules look empty and pull whole ignored trees into the watch set,
// so the walk stops at the first non-hidden file and an ignored tree full
// of files costs one directory read.
func emptySubtree(abs string) ([]string, bool) {
	var dirs []string
	empty := true
	filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			empty = false
			return fs.SkipAll
		}
		if p != abs && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(abs, p)
		if rel == "." {
			rel = ""
		}
		dirs = append(dirs, filepath.ToSlash(rel))
		return nil
	})
	return dirs, empty
}

func list(ctx context.Context, root string, warnf func(string, ...any), keep func(string) bool) ([]string, error) {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	rg, err := lookPath("rg")
	if err != nil {
		return nil, ErrNoRipgrep
	}
	cmd := exec.CommandContext(ctx, rg, "--files", "--sort", "path", "--null")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("rg: %w", err)
	}
	var files []string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(splitNUL)
	for sc.Scan() {
		name := sc.Text()
		if name == "" || !keep(name) {
			continue
		}
		files = append(files, path.Clean(strings.TrimPrefix(name, "./")))
	}
	scanErr := sc.Err()
	runErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if scanErr != nil {
		return nil, fmt.Errorf("rg: reading output: %w", scanErr)
	}
	if runErr != nil {
		var exit *exec.ExitError
		partial := errors.As(runErr, &exit) && exit.ExitCode() == 2 && len(files) > 0
		// Exit 1 is "no files", an empty root rather than a failure.
		noFiles := errors.As(runErr, &exit) && exit.ExitCode() == 1 && stderr.Len() == 0
		if !partial && !noFiles {
			return nil, fmt.Errorf("rg: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		if partial {
			warnf("rg: partial listing of %s: %s", root, strings.TrimSpace(stderr.String()))
		}
	}
	return files, nil
}

// splitNUL is a bufio.SplitFunc for NUL-terminated records.
func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Build arranges relative file paths into a tree whose root node has an
// empty name and path. Directories appear only because a file lies
// beneath them.
func Build(files []string) *Node {
	root := &Node{Dir: true}
	index := map[string]*Node{"": root}
	for _, f := range files {
		dir := path.Dir(f)
		if dir == "." {
			dir = ""
		}
		parent := ensureDir(index, dir)
		parent.Children = append(parent.Children, &Node{Name: path.Base(f), Path: f})
	}
	sortTree(root)
	return root
}

func ensureDir(index map[string]*Node, dir string) *Node {
	if n, ok := index[dir]; ok {
		return n
	}
	parentPath := path.Dir(dir)
	if parentPath == "." {
		parentPath = ""
	}
	parent := ensureDir(index, parentPath)
	n := &Node{Name: path.Base(dir), Path: dir, Dir: true}
	parent.Children = append(parent.Children, n)
	index[dir] = n
	return n
}

func sortTree(n *Node) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		if c.Dir {
			sortTree(c)
		}
	}
}
