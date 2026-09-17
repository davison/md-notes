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
	"time"
)

func TestBuild(t *testing.T) {
	got := Build(listed(
		"zeta.md",
		"docs/b.md",
		"docs/A.md",
		"docs/deep/x/y.md",
		"Alpha.md",
		"code/README.md",
	))
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

// listed is a listing a case cares about the shape of rather than the
// times in.
func listed(names ...string) []File {
	out := make([]File, len(names))
	for i, n := range names {
		out[i] = File{Path: n}
	}
	return out
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

// paths flattens a directory set for comparison; the group is asserted
// separately where it matters.
func paths(dirs []Dir) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, d.Path)
	}
	return out
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
	// note created there would be listed and it is watched; placeholder
	// holds nothing but a .gitkeep, which ripgrep lists once hidden files
	// are asked for, so it is watched too — and so is the nest, at both
	// levels, because a placeholder counts wherever it lies. Empty nests,
	// which hold no files at any depth, are watched throughout. The order
	// is markdown-holding directories, then the empty ones, then images,
	// which holds a file but no note.
	want := []string{
		"", "a", "a/b",
		"a/emptychild", "empty", "empty/nested", "empty/nested/deeper", "nest", "nest/deep",
		"onlyhidden", "placeholder",
		"images",
	}
	for i := 0; i < 50; i++ {
		got, err := Dirs(context.Background(), root, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(paths(got), want) {
			t.Fatalf("run %d: Dirs() = %v, want %v", i, paths(got), want)
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
	for _, d := range paths(got) {
		if strings.HasPrefix(d, "build") || strings.HasPrefix(d, "logs") {
			t.Fatalf("Dirs() = %v, want no ignored-only directory", paths(got))
		}
	}
}

// The order is the watch budget's priority: a truncated set keeps the
// directories where notes live and where a first note can appear unseen.
func TestDirsOrdersByPriority(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "zz-notes", "n.md"), "")
	write(t, filepath.Join(root, "aa-assets", "pic.png"), "")
	write(t, filepath.Join(root, "mm-placeholder", ".gitkeep"), "")

	got, err := Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Dir{
		{Path: "", Group: GroupNotes},
		{Path: "zz-notes", Group: GroupNotes},
		{Path: "mm-placeholder", Group: GroupEmpty},
		{Path: "aa-assets", Group: GroupOther},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Dirs() = %v, want %v", got, want)
	}
}

// A gitignored subtree whose files all happen to be hidden must stay
// unwatched: exempting dotfiles at every depth read it as empty and put its
// whole directory tree in the watch set, ahead of the root's own content
// directories. Reported in the model review of #26 with these figures — 202
// watched directories where main watched one.
func TestDirsLeavesAnIgnoredHiddenFileTreeUnwatched(t *testing.T) {
	requireRg(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	write(t, filepath.Join(root, ".gitignore"), "cache/\n")
	write(t, filepath.Join(root, "note.md"), "")
	for i := 1; i <= 200; i++ {
		write(t, filepath.Join(root, "cache", fmt.Sprintf("d%d", i), ".lock"), "")
	}

	got, err := Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths(got), []string{""}) {
		t.Fatalf("Dirs() covered %d directories, want just the root: %v", len(got), paths(got))
	}
}

// A placeholder makes its directory watchable wherever it lies, because the
// listing that finds it applies the root's ignore rules. The layouts are the
// ones the model review of #26 measured, with the answers M2-R4 asks for.
func TestDirsWatchesPlaceholderTreesAtEveryLevel(t *testing.T) {
	requireRg(t)
	cases := []struct {
		name   string
		layout map[string]bool // path -> is a file
		want   []string
	}{
		{"a placeholder alone", map[string]bool{"A/.gitkeep": true}, []string{"", "A"}},
		{"a placeholder one level down", map[string]bool{"B/deep/.gitkeep": true}, []string{"", "B", "B/deep"}},
		{"placeholders at two levels", map[string]bool{"C/.gitkeep": true, "C/sub/.gitkeep": true}, []string{"", "C", "C/sub"}},
		{"a placeholder beside an empty directory", map[string]bool{"D/.gitkeep": true, "D/emptysub": false}, []string{"", "D", "D/emptysub"}},
		{"an entirely empty nest", map[string]bool{"E/x/y": false}, []string{"", "E", "E/x", "E/x/y"}},
	}
	for _, c := range cases {
		root := t.TempDir()
		for p, isFile := range c.layout {
			if isFile {
				write(t, filepath.Join(root, filepath.FromSlash(p)), "")
			} else {
				os.MkdirAll(filepath.Join(root, filepath.FromSlash(p)), 0o755)
			}
		}
		got, err := Dirs(context.Background(), root, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !reflect.DeepEqual(paths(got), c.want) {
			t.Errorf("%s: Dirs() = %v, want %v", c.name, paths(got), c.want)
		}
		for _, d := range got {
			if d.Path != "" && d.Group != GroupEmpty {
				t.Errorf("%s: %q is in group %d, want the group whose watches exist for a first note", c.name, d.Path, d.Group)
			}
		}
	}
}

// A hidden directory is never watched, whatever it holds: the navigator
// does not list it, and the listing that finds placeholders excludes it.
func TestDirsNeverWatchesHiddenDirectories(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "note.md"), "")
	write(t, filepath.Join(root, ".config", "keep.md"), "")
	write(t, filepath.Join(root, ".config", "nested", ".gitkeep"), "")
	os.MkdirAll(filepath.Join(root, ".empty"), 0o755)
	write(t, filepath.Join(root, "visible", ".gitkeep"), "")

	got, err := Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths(got), []string{"", "visible"}) {
		t.Fatalf("Dirs() = %v, want the root and the one non-hidden placeholder directory", paths(got))
	}
}

// A hidden file an explicit ! rule un-ignores is a note the navigator
// lists, so its directory belongs with the note-holding ones and not with
// the placeholders. Reported in the model review of #26, whose root this is.
func TestDirsRanksAnUnignoredHiddenNoteAsANoteDirectory(t *testing.T) {
	requireRg(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	write(t, filepath.Join(root, ".gitignore"), "!/hid/.secret.md\n")
	write(t, filepath.Join(root, "hid", ".secret.md"), "# secret")
	write(t, filepath.Join(root, "plain", "f.txt"), "")

	// The premise: a plain listing names the un-ignored dotfile, so the
	// navigator shows it. If ripgrep ever stops doing that, this test has
	// nothing left to pin.
	listed, err := List(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(listed, []string{"hid/.secret.md"}) {
		t.Skipf("ripgrep no longer names an un-ignored hidden file: %v", listed)
	}

	got, err := Dirs(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Dir{
		{Path: "", Group: GroupNotes},
		{Path: "hid", Group: GroupNotes},
		{Path: "plain", Group: GroupOther},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Dirs() = %v, want %v", got, want)
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

// The modification times the navigator's recency order is built on
// (davison/md-notes#116). Stat answers for the files a listing named, Build
// carries the answers onto the file nodes in Unix milliseconds, and a
// directory node carries none.

func TestStatFillsModificationTimes(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	want := map[string]time.Time{
		"a.md":      time.Date(2024, 1, 2, 3, 4, 5, 600*int(time.Millisecond), time.UTC),
		"docs/b.md": time.Date(2026, 9, 17, 8, 30, 0, 250*int(time.Millisecond), time.UTC),
	}
	for rel, when := range want {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(rel)), when, when); err != nil {
			t.Fatal(err)
		}
	}
	got := Stat(root, []string{"a.md", "docs/b.md"}, nil)
	if len(got) != 2 {
		t.Fatalf("Stat returned %d files, want 2: %+v", len(got), got)
	}
	for _, f := range got {
		if !f.Modified.Equal(want[f.Path]) {
			t.Errorf("%s modified = %s, want %s", f.Path, f.Modified, want[f.Path])
		}
	}
}

func TestStatLeavesAVanishedFileWithoutATime(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "here.md"), []byte("x"), 0o644)

	// "gone.md" is what a note deleted between the listing and the stat
	// looks like — a sync landing mid-request is the ordinary way to see
	// it. The listing must survive it, and the file it could not read must
	// still be in the answer, with no time.
	var warnings []string
	got := Stat(root, []string{"gone.md", "here.md"}, func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	if len(got) != 2 || got[0].Path != "gone.md" || got[1].Path != "here.md" {
		t.Fatalf("Stat = %+v, want both paths in order", got)
	}
	if !got[0].Modified.IsZero() {
		t.Errorf("gone.md has a time: %s", got[0].Modified)
	}
	if got[1].Modified.IsZero() {
		t.Error("here.md has no time")
	}
	// A file that is merely absent is not worth a log line; it is the
	// commonest thing that can happen here.
	if len(warnings) != 0 {
		t.Errorf("warned about an absent file: %v", warnings)
	}
}

func TestBuildCarriesModificationTimes(t *testing.T) {
	when := time.Date(2026, 5, 6, 7, 8, 9, 100*int(time.Millisecond), time.UTC)
	got := Build([]File{
		{Path: "docs/b.md", Modified: when},
		{Path: "a.md"},
	})
	if len(got.Children) != 2 {
		t.Fatalf("tree = %s", dump(got, 0))
	}
	docs, a := got.Children[0], got.Children[1]
	if docs.Modified != 0 {
		t.Errorf("the directory node carries a time: %d", docs.Modified)
	}
	if b := docs.Children[0]; b.Modified != when.UnixMilli() {
		t.Errorf("docs/b.md modified = %d, want %d", b.Modified, when.UnixMilli())
	}
	// A file Stat could not answer for keeps a zero, which `omitempty`
	// keeps off the wire and the navigator reads as "oldest".
	if a.Modified != 0 {
		t.Errorf("a.md, with no time from Stat, carries %d", a.Modified)
	}
}

// BenchmarkList and BenchmarkListStat are the two halves of what a tree
// request costs, on the two root sizes davison/md-notes#116 asked for: the
// ripgrep listing the endpoint has always paid for, and the per-file stat
// the modification times add to it. Run them together:
//
//	go test ./internal/tree -run xxx -bench 'List' -benchtime 10x
func BenchmarkList(b *testing.B) {
	for _, n := range []int{300, 5000} {
		root := benchRoot(b, n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := List(context.Background(), root, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkListStat(b *testing.B) {
	for _, n := range []int{300, 5000} {
		root := benchRoot(b, n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				files, err := List(context.Background(), root, nil)
				if err != nil {
					b.Fatal(err)
				}
				Build(Stat(root, files, nil))
			}
		})
	}
}

// BenchmarkStat is the added cost alone, with the listing hoisted out.
func BenchmarkStat(b *testing.B) {
	for _, n := range []int{300, 5000} {
		root := benchRoot(b, n)
		files, err := List(context.Background(), root, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Stat(root, files, nil)
			}
		})
	}
}

// benchRoot writes n notes over 25 directories, which is the shape of a
// notes root rather than one flat directory of files.
func benchRoot(b *testing.B, n int) string {
	b.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		b.Skip("ripgrep not installed")
	}
	root := b.TempDir()
	for i := 0; i < n; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d%02d", i%25))
		os.MkdirAll(dir, 0o755)
		body := fmt.Sprintf("# Note %d\n\nSome body text for note %d.\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("note-%05d.md", i)), []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return root
}
