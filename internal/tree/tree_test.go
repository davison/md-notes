package tree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"notes/a.markdown", "top.md"}
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
	write(t, filepath.Join(root, ".gitignore"), "node_modules/\nbuild/\n")
	write(t, filepath.Join(root, "README.md"), "")
	write(t, filepath.Join(root, "node_modules", "dep", "README.md"), "")
	write(t, filepath.Join(root, "build", "out.md"), "")
	write(t, filepath.Join(root, "docs", "guide.md"), "")

	got, err := List(root)
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
	got, err := List(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Fatalf("List(empty) = %v, %v", got, err)
	}
}

func TestListMissingRipgrep(t *testing.T) {
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = orig })
	if _, err := List(t.TempDir()); !errors.Is(err, ErrNoRipgrep) {
		t.Fatalf("err = %v, want ErrNoRipgrep", err)
	}
}
