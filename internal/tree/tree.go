// Package tree lists the markdown files under a root with ripgrep and
// arranges them as a directory tree for the navigator.
package tree

import (
	"bytes"
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

// Globs are the file patterns that count as markdown.
var Globs = []string{"*.md", "*.markdown"}

// rgPath is looked up once per call so tests can point it elsewhere.
var lookPath = exec.LookPath

// List returns the markdown files under root, relative to it, sorted by
// path. ripgrep's defaults apply: gitignore inside repositories, .ignore
// and .rgignore anywhere, and hidden files and directories skipped.
func List(root string) ([]string, error) {
	rg, err := lookPath("rg")
	if err != nil {
		return nil, ErrNoRipgrep
	}
	args := []string{"--files", "--sort", "path"}
	for _, g := range Globs {
		args = append(args, "-g", g)
	}
	// An explicit include glob makes rg list hidden files that match it,
	// so hidden entries are excluded again. The last matching glob wins,
	// which is why this one comes after the includes.
	args = append(args, "-g", "!.*")
	cmd := exec.Command(rg, args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		// rg exits 1 when nothing matched, which is an empty root, not a failure.
		if errors.As(err, &exit) && exit.ExitCode() == 1 && stderr.Len() == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("rg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var files []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if line == "" {
			continue
		}
		files = append(files, path.Clean(strings.TrimPrefix(strings.ReplaceAll(line, "\\", "/"), "./")))
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
