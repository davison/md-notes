// Package tree lists the markdown files under a root with ripgrep and
// arranges them as a directory tree for the navigator.
package tree

import (
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
// ignore rules for the files it matches.
//
// If rg reports errors for parts of the tree it could not read, the files
// it did list are returned; only a total failure is an error.
func List(ctx context.Context, root string) ([]string, error) {
	rg, err := lookPath("rg")
	if err != nil {
		return nil, ErrNoRipgrep
	}
	cmd := exec.CommandContext(ctx, rg, "--files", "--sort", "path", "--null")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil {
		var exit *exec.ExitError
		partial := errors.As(runErr, &exit) && exit.ExitCode() == 2 && stdout.Len() > 0
		// Exit 1 is "no files", an empty root rather than a failure.
		noFiles := errors.As(runErr, &exit) && exit.ExitCode() == 1 && stderr.Len() == 0
		if !partial && !noFiles {
			return nil, fmt.Errorf("rg: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
	}
	var files []string
	for _, name := range bytes.Split(stdout.Bytes(), []byte{0}) {
		if len(name) == 0 || !IsMarkdown(string(name)) {
			continue
		}
		files = append(files, path.Clean(strings.TrimPrefix(string(name), "./")))
	}
	return files, nil
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
