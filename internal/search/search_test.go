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
	"unicode/utf16"
	"unicode/utf8"
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
	write(t, filepath.Join(root, ".secret.md"), "needle hidden file\n")
	write(t, filepath.Join(root, ".hid", "h.md"), "needle hidden dir\n")
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
	res, err := Search(context.Background(), root, "needle", nil)
	if err != nil {
		t.Fatal(err)
	}
	hits := res.Hits
	paths := map[string]bool{}
	for _, h := range hits {
		paths[h.Path] = true
	}
	for _, want := range []string{"a.md", "docs/b.markdown", "regex.md"} {
		if !paths[want] {
			t.Errorf("missing hits in %s; got %v", want, paths)
		}
	}
	for _, no := range []string{"c.txt", "vendor/v.md", "notes/x.mdx", ".secret.md", ".hid/h.md"} {
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
	res, err := Search(context.Background(), root, "needle", nil)
	if err != nil || len(res.Hits) != 1 {
		t.Fatalf("hits = %v, %v", res.Hits, err)
	}
	// "café " is 5 units, the emoji is 2, then a space: needle starts at 8.
	if res.Hits[0].Matches[0] != [2]int{8, 14} {
		t.Fatalf("matches = %v, want [8 14]", res.Hits[0].Matches)
	}
}

func TestSearchIsLiteral(t *testing.T) {
	requireRg(t)
	root := fixture(t)
	for q, want := range map[string]int{"a.b (c)": 1, "[d]": 1, "needle*": 1, "a.b": 1, "aXb": 0} {
		res, err := Search(context.Background(), root, q, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(find(res.Hits, "regex.md")); got != want {
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
	res, err := Search(context.Background(), root, "cap", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != MaxHits || !res.Truncated {
		t.Fatalf("got %d hits, truncated %v; want cap %d and truncated", len(res.Hits), res.Truncated, MaxHits)
	}
	if n := len(find(res.Hits, "f00.md")); n != MaxHitsPerFile {
		t.Fatalf("per-file cap: %d, want %d", n, MaxHitsPerFile)
	}
}

func TestSearchExactlyAtCapIsNotTruncated(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	var b strings.Builder
	for i := 0; i < MaxHitsPerFile; i++ {
		b.WriteString("cap line\n")
	}
	for i := 0; i < MaxHits/MaxHitsPerFile; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("f%02d.md", i)), b.String())
	}
	res, err := Search(context.Background(), root, "cap", nil)
	if err != nil || len(res.Hits) != MaxHits || res.Truncated {
		t.Fatalf("hits %d, truncated %v, err %v; want exactly %d and not truncated", len(res.Hits), res.Truncated, err, MaxHits)
	}
}

func TestSearchHugeLineDoesNotBreakOthers(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "a.md"), "needle early\n")
	write(t, filepath.Join(root, "big.md"), "needle "+strings.Repeat("x", 5*1024*1024)+"\n")
	write(t, filepath.Join(root, "z.md"), "needle late\n")
	res, err := Search(context.Background(), root, "needle", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 3 || res.Hits[2].Path != "z.md" {
		t.Fatalf("hits = %d (%v)", len(res.Hits), res.Hits)
	}
}

func TestLongLinesAreWindowed(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	long := strings.Repeat("a", 2000) + " needle " + strings.Repeat("b", 2000)
	write(t, filepath.Join(root, "w.md"), strings.Repeat("c", 1000)+"\n"+long+"\n"+strings.Repeat("d", 1000)+"\n")
	res, err := Search(context.Background(), root, "needle", nil)
	if err != nil || len(res.Hits) != 1 {
		t.Fatalf("hits = %v, %v", res.Hits, err)
	}
	h := res.Hits[0]
	if n := utf8.RuneCountInString(h.Text); n > MaxText+2 {
		t.Fatalf("text is %d runes, want at most %d plus marks", n, MaxText+2)
	}
	if !strings.HasPrefix(h.Text, "…") || !strings.HasSuffix(h.Text, "…") {
		t.Fatalf("text = %q, want both ends marked", h.Text)
	}
	if len(h.Matches) != 1 || sliceUTF16(h.Text, h.Matches[0]) != "needle" {
		t.Fatalf("matches %v do not point at the needle in %q", h.Matches, h.Text)
	}
	if utf8.RuneCountInString(h.Before) != MaxContext+1 || utf8.RuneCountInString(h.After) != MaxContext+1 {
		t.Fatalf("context not clipped: %d / %d", len(h.Before), len(h.After))
	}
}

func TestWindowKeepsOffsetsWithNonASCII(t *testing.T) {
	text := strings.Repeat("é", 400) + "needle" + strings.Repeat("😀", 400)
	h := window(Hit{Text: text, Matches: [][2]int{{400, 406}}})
	if got := sliceUTF16(h.Text, h.Matches[0]); got != "needle" {
		t.Fatalf("window offsets point at %q", got)
	}
}

// sliceUTF16 slices s by UTF-16 unit offsets, as JavaScript would.
func sliceUTF16(s string, m [2]int) string {
	u := utf16.Encode([]rune(s))
	return string(utf16.Decode(u[m[0]:m[1]]))
}

func TestSearchNonUTF8Line(t *testing.T) {
	requireRg(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "bin.md"), "needle \xff\xfe here\n")
	res, err := Search(context.Background(), root, "needle", nil)
	if err != nil || len(res.Hits) != 1 || !strings.HasPrefix(res.Hits[0].Text, "needle ") {
		t.Fatalf("hits = %+v, %v", res.Hits, err)
	}
}

func TestSearchBadQuery(t *testing.T) {
	for _, q := range []string{"", "  ", "a\nb", "a\x00b", "\t", "\x1b[0m"} {
		if _, err := Search(context.Background(), t.TempDir(), q, nil); !errors.Is(err, ErrBadQuery) {
			t.Errorf("query %q: err = %v, want ErrBadQuery", q, err)
		}
	}
}

func TestSearchNoMatchesAndEmptyRoot(t *testing.T) {
	requireRg(t)
	res, err := Search(context.Background(), fixture(t), "zzzz-nothing", nil)
	if err != nil || len(res.Hits) != 0 || res.Hits == nil {
		t.Fatalf("no matches: %v, %v", res, err)
	}
	res, err = Search(context.Background(), t.TempDir(), "x", nil)
	if err != nil || len(res.Hits) != 0 {
		t.Fatalf("empty root: %v, %v", res, err)
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
