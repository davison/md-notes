package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireRg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
}

func write(t *testing.T, p, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "a.md"), "first line\nThe Needle is here\nlast line\n")
	write(t, filepath.Join(root, "docs", "b.markdown"), "needle at start\nmiddle\nend needle\n")
	write(t, filepath.Join(root, "c.txt"), "needle in a text file\n")
	write(t, filepath.Join(root, "regex.md"), "a.b (c) [d] needle*\n")
	write(t, filepath.Join(root, ".ignore"), "vendor/\n")
	write(t, filepath.Join(root, "vendor", "v.md"), "needle in vendor\n")
	write(t, filepath.Join(root, "notes", "x.mdx"), "needle in mdx\n")
	return root
}

func find(hits []Hit, p string) []Hit {
	var out []Hit
	for _, h := range hits {
		if h.Path == p {
			out = append(out, h)
		}
	}
	return out
}

func TestSearchBasics(t *testing.T) {
	requireRg(t)
	root := fixture(t)
	hits, err := Search(context.Background(), root, "needle", nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, h := range hits {
		paths[h.Path] = true
	}
	for _, want := range []string{"a.md", "docs/b.markdown", "regex.md"} {
		if !paths[want] {
			t.Errorf("missing hits in %s; got %v", want, paths)
		}
	}
	for _, no := range []string{"c.txt", "vendor/v.md", "notes/x.mdx"} {
		if paths[no] {
			t.Errorf("unexpected hits in %s", no)
		}
	}

	a := find(hits, "a.md")
	if len(a) != 1 || a[0].Line != 2 || a[0].Text != "The Needle is here" {
		t.Fatalf("a.md hits = %+v", a)
	}
	if a[0].Before != "first line" || a[0].After != "last line" {
		t.Errorf("context = %q / %q", a[0].Before, a[0].After)
	}
	if len(a[0].Matches) != 1 || a[0].Matches[0] != [2]int{4, 10} {
		t.Errorf("matches = %v", a[0].Matches)
	}

	b := find(hits, "docs/b.markdown")
	if len(b) != 2 {
		t.Fatalf("b hits = %+v", b)
	}
	if b[0].Before != "" || b[0].After != "middle" {
		t.Errorf("file-start context = %q / %q", b[0].Before, b[0].After)
	}
	if b[1].Before != "middle" || b[1].After != "" {
		t.Errorf("file-end context = %q / %q", b[1].Before, b[1].After)
	}
	// Path order.
	if hits[0].Path != "a.md" || hits[len(hits)-1].Path != "regex.md" {
		t.Errorf("order: %s ... %s", hits[0].Path, hits[len(hits)-1].Path)
	}
}

func TestMatchOffsetsAreUTF16(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "u.md"), "café 😀 needle\n")
	hits, err := Search(context.Background(), root, "needle", nil)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits = %v, %v", hits, err)
	}
	// "café " is 5 units, the emoji is 2, then a space: needle starts at 8.
	if hits[0].Matches[0] != [2]int{8, 14} {
		t.Fatalf("matches = %v, want [8 14]", hits[0].Matches)
	}
}

func TestSearchIsLiteral(t *testing.T) {
	requireRg(t)
	root := fixture(t)
	for q, want := range map[string]int{"a.b (c)": 1, "[d]": 1, "needle*": 1, "a.b": 1, "aXb": 0} {
		hits, err := Search(context.Background(), root, q, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(find(hits, "regex.md")); got != want {
			t.Errorf("query %q: %d hits in regex.md, want %d", q, got, want)
		}
	}
}

func TestSearchCaps(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	var big strings.Builder
	for i := 0; i < 50; i++ {
		big.WriteString("cap here\n")
	}
	for i := 0; i < 15; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("f%02d.md", i)), big.String())
	}
	hits, err := Search(context.Background(), root, "cap", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != MaxHits {
		t.Fatalf("got %d hits, want cap %d", len(hits), MaxHits)
	}
	if n := len(find(hits, "f00.md")); n != MaxHitsPerFile {
		t.Fatalf("per-file cap: %d, want %d", n, MaxHitsPerFile)
	}
}

func TestSearchNoMatchesAndEmptyRoot(t *testing.T) {
	requireRg(t)
	hits, err := Search(context.Background(), fixture(t), "zzzz-nothing", nil)
	if err != nil || len(hits) != 0 {
		t.Fatalf("no matches: %v, %v", hits, err)
	}
	hits, err = Search(context.Background(), t.TempDir(), "x", nil)
	if err != nil || len(hits) != 0 {
		t.Fatalf("empty root: %v, %v", hits, err)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	if _, err := Search(context.Background(), t.TempDir(), "  ", nil); err == nil {
		t.Fatal("want error for empty query")
	}
}

func TestSearchCancelled(t *testing.T) {
	requireRg(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(ctx, fixture(t), "needle", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
}

func TestSearchMissingRipgrep(t *testing.T) {
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = orig })
	if _, err := Search(context.Background(), t.TempDir(), "x", nil); !errors.Is(err, ErrNoRipgrep) {
		t.Fatalf("err = %v", err)
	}
}
