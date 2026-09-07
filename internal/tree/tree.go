// Package tree lists the markdown files under a root with ripgrep and
// arranges them as a directory tree for the navigator.
package tree

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
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
		if name == "" || !IsMarkdown(name) {
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
