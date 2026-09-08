package tree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	got := Build([]string{
		"zeta.md",
		"docs/b.md",
		"docs/A.md",
		"docs/deep/x/y.md",
		"Alpha.md",
		"code/README.md",
	})
	want := &Node{Dir: true, Children: []*Node{
		{Name: "code", Path: "code", Dir: true, Children: []*Node{
			{Name: "README.md", Path: "code/README.md"},
		}},
		{Name: "docs", Path: "docs", Dir: true, Children: []*Node{
			{Name: "deep", Path: "docs/deep", Dir: true, Children: []*Node{
				{Name: "x", Path: "docs/deep/x", Dir: true, Children: []*Node{
					{Name: "y.md", Path: "docs/deep/x/y.md"},
				}},
			}},
			{Name: "A.md", Path: "docs/A.md"},
			{Name: "b.md", Path: "docs/b.md"},
		}},
		{Name: "Alpha.md", Path: "Alpha.md"},
		{Name: "zeta.md", Path: "zeta.md"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() =\n%s\nwant\n%s", dump(got, 0), dump(want, 0))
	}
}

func TestBuildEmpty(t *testing.T) {
	got := Build(nil)
	if !got.Dir || len(got.Children) != 0 || got.Path != "" {
		t.Fatalf("Build(nil) = %+v", got)
	}
}

func dump(n *Node, depth int) string {
	s := ""
	for _, c := range n.Children {
		pad := ""
		for i := 0; i < depth; i++ {
			pad += "  "
		}
		mark := ""
		if c.Dir {
			mark = "/"
		}
		s += pad + c.Name + mark + "  (" + c.Path + ")\n"
		if c.Dir {
			s += dump(c, depth+1)
		}
	}
	return s
}

func requireRg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
}

func write(t *testing.T, p string, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListFiltersAndHonoursIgnores(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "top.md"), "")
	write(t, filepath.Join(root, "notes", "a.markdown"), "")
	write(t, filepath.Join(root, "notes", "img.png"), "")
	write(t, filepath.Join(root, "empty", "data.json"), "")
	write(t, filepath.Join(root, ".hidden", "secret.md"), "")
	write(t, filepath.Join(root, ".dotfile.md"), "")
	write(t, filepath.Join(root, "vendor", "lib.md"), "")
	write(t, filepath.Join(root, ".ignore"), "vendor/\n")
	write(t, filepath.Join(root, "UPPER.MD"), "")
	write(t, filepath.Join(root, "back\\slash.md"), "")

	got, err := List(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"UPPER.MD", "back\\slash.md", "notes/a.markdown", "top.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
}

func TestListHonoursGitignoreInRepo(t *testing.T) {
	requireRg(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// Directory patterns and file patterns both: an rg include glob would
	// override the file patterns, which is the bug this test pins.
	write(t, filepath.Join(root, ".gitignore"), "node_modules/\nbuild/\nsecret.md\n*.gen.md\nprivate/*.md\n")
	write(t, filepath.Join(root, "README.md"), "")
	write(t, filepath.Join(root, "node_modules", "dep", "README.md"), "")
	write(t, filepath.Join(root, "build", "out.md"), "")
	write(t, filepath.Join(root, "docs", "guide.md"), "")
	write(t, filepath.Join(root, "secret.md"), "")
	write(t, filepath.Join(root, "api.gen.md"), "")
	write(t, filepath.Join(root, "private", "p.md"), "")

	got, err := List(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "docs/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
}

func TestListEmptyRoot(t *testing.T) {
	requireRg(t)
	got, err := List(context.Background(), t.TempDir(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("List(empty) = %v, %v", got, err)
	}
}

func TestListNewlineInName(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "line\nbreak.md"), "")
	got, err := List(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"line\nbreak.md"}) {
		t.Fatalf("List() = %q", got)
	}
}

func TestListUnreadableSubdirIsPartial(t *testing.T) {
	requireRg(t)
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "ok.md"), "")
	locked := filepath.Join(root, "locked")
	write(t, filepath.Join(locked, "hidden.md"), "")
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	var warnings []string
	got, err := List(context.Background(), root, func(f string, a ...any) { warnings = append(warnings, fmt.Sprintf(f, a...)) })
	if err != nil {
		t.Fatalf("unreadable subdir must not fail the listing: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"ok.md"}) {
		t.Fatalf("List() = %v", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "partial") {
		t.Fatalf("warnings = %q, want one partial-listing warning", warnings)
	}
}

func TestListCancelled(t *testing.T) {
	requireRg(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := List(ctx, t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestIsMarkdown(t *testing.T) {
	for name, want := range map[string]bool{"a.md": true, "A.MD": true, "b.markdown": true, "c.mdx": false, "md": false, "dir.md/x": false} {
		if got := IsMarkdown(name); got != want {
			t.Errorf("IsMarkdown(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestListManyFilesStreams(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	for i := 0; i < 2000; i++ {
		name := filepath.Join(root, "d", fmt.Sprintf("f%04d.txt", i))
		if i%100 == 0 {
			name = strings.TrimSuffix(name, ".txt") + ".md"
		}
		write(t, name, "")
	}
	got, err := List(context.Background(), root, nil)
	if err != nil || len(got) != 20 {
		t.Fatalf("List() = %d files, %v", len(got), err)
	}
}

func TestDirs(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "b", "note.md"), "")
	write(t, filepath.Join(root, "images", "pic.png"), "")
	os.MkdirAll(filepath.Join(root, "empty", "nested", "deeper"), 0o755)
	os.MkdirAll(filepath.Join(root, "a", "emptychild"), 0o755)
	os.MkdirAll(filepath.Join(root, "onlyhidden", ".cache"), 0o755)
	write(t, filepath.Join(root, "onlyhidden", ".cache", "x"), "")
	write(t, filepath.Join(root, "placeholder", ".gitkeep"), "")
	write(t, filepath.Join(root, "nest", ".gitkeep"), "")
	write(t, filepath.Join(root, "nest", "deep", ".gitkeep"), "")
	os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755)
	write(t, filepath.Join(root, ".ignore"), "vendor/\n")
	write(t, filepath.Join(root, "vendor", "lib", "x.md"), "")

	// vendor is ignored and holds files, so it is not watched. onlyhidden
	// has nothing but a hidden cache, which the navigator would skip, so a
	// note created there would be listed and it is watched; placeholder and
	// the nest under it hold nothing but .gitkeep files and are watched for
	// the same reason. Empty nests are watched throughout.
	want := []string{
		"", "a", "a/b", "a/emptychild", "empty", "empty/nested", "empty/nested/deeper",
		"images", "nest", "nest/deep", "onlyhidden", "placeholder",
	}
	for i := 0; i < 50; i++ {
		got, err := Dirs(context.Background(), root, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: Dirs() = %v, want %v", i, got, want)
		}
	}
}

// A directory whose only files are ignored stays unwatched: reading ignored
// files as absent would make node_modules look empty and pull whole ignored
// trees into the watch set. The asymmetry with hidden files is deliberate.
func TestDirsLeavesIgnoredOnlyDirectoriesUnwatched(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "note.md"), "")
	write(t, filepath.Join(root, ".ignore"), "build/\n*.log\n")
	write(t, filepath.Join(root, "build", "out.md"), "")
	write(t, filepath.Join(root, "logs", "run.log"), "")

	got, err := Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if strings.HasPrefix(d, "build") || strings.HasPrefix(d, "logs") {
			t.Fatalf("Dirs() = %v, want no ignored-only directory", got)
		}
	}
}

func TestListMissingRipgrep(t *testing.T) {
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = orig })
	if _, err := List(context.Background(), t.TempDir(), nil); !errors.Is(err, ErrNoRipgrep) {
		t.Fatalf("err = %v, want ErrNoRipgrep", err)
	}
}
