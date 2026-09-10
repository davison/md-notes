package clip

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davison/md-notes/internal/roots"
	"gopkg.in/yaml.v3"
)

var when = time.Date(2026, 9, 10, 14, 5, 0, 0, time.FixedZone("BST", 3600))

func notesRoot(t *testing.T) roots.Root {
	t.Helper()
	base := t.TempDir()
	notes := filepath.Join(base, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := roots.New(notes, filepath.Join(base, "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := reg.Notes()
	if !ok {
		t.Fatal("no notes root")
	}
	return root
}

func page(title string) Clip {
	return Clip{URL: "https://example.com/a", Title: title, Markdown: "# Body\n", Kind: KindPage}
}

func TestSlug(t *testing.T) {
	for title, want := range map[string]string{
		"Hello, World!":             "hello-world",
		"  spaced   out  ":          "spaced-out",
		"Ünicode Café — naïve":      "unicode-cafe-naive",
		"Trailing punctuation...":   "trailing-punctuation",
		"C++ vs. Rust: which?":      "c-vs-rust-which",
		"2026/09/10 report":         "2026-09-10-report",
		"":                          FallbackSlug,
		"    ":                      FallbackSlug,
		"日本語のページ":                   FallbackSlug,
		"---":                       FallbackSlug,
		"ÆØÅ":                       "aeoa",
		"Straße":                    "strasse",
		"under_score and-hyphen":    "under-score-and-hyphen",
		"MiXeD CaSe":                "mixed-case",
		"emoji 🎉 party":             "emoji-party",
		"a/../../etc/passwd":        "a-etc-passwd",
		".hidden":                   "hidden",
		strings.Repeat("long ", 40): "long-long-long-long-long-long-long-long-long-long-long-long",
		strings.Repeat("x", 100):    strings.Repeat("x", 64),
	} {
		if got := Slug(title); got != want {
			t.Errorf("Slug(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestSlugIsAlwaysASafeFilenamePart(t *testing.T) {
	for _, title := range []string{"../escape", "a/b/c", "nul\x00byte", ". .", "~/x", "$(rm -rf)", "\\windows\\path"} {
		got := Slug(title)
		if got == "" || got == "." || got == ".." {
			t.Errorf("Slug(%q) = %q", title, got)
		}
		if strings.ContainsAny(got, "/\\\x00. ") {
			t.Errorf("Slug(%q) = %q, contains a path or control character", title, got)
		}
	}
}

func TestWriteCreatesTheClipsDirectoryAndNote(t *testing.T) {
	root := notesRoot(t)
	rel, err := Write(root, "clips", page("A Page About Things"), when)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "clips/2026-09-10-a-page-about-things.md" {
		t.Fatalf("path = %q", rel)
	}
	data, err := os.ReadFile(filepath.Join(root.Path, rel))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: A Page About Things\nsource: https://example.com/a\nclipped: \"2026-09-10T14:05:00+01:00\"\ntags: [clip]\n---\n\n# Body\n"
	if string(data) != want {
		t.Errorf("note =\n%q\nwant\n%q", data, want)
	}
}

func TestWriteFrontmatterParsesBack(t *testing.T) {
	root := notesRoot(t)
	c := page(`Quotes: "he said" - and: a colon`)
	c.URL = "https://example.com/x?a=1&b=2#frag"
	rel, err := Write(root, "clips", c, when)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root.Path, rel))
	if err != nil {
		t.Fatal(err)
	}
	front, body, ok := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "---\n")
	if !ok {
		t.Fatalf("no frontmatter in %q", data)
	}
	var got struct {
		Title   string
		Source  string
		Clipped string
		Tags    []string
	}
	if err := yaml.Unmarshal([]byte(front), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != c.Title {
		t.Errorf("title = %q, want %q", got.Title, c.Title)
	}
	if got.Source != c.URL {
		t.Errorf("source = %q, want %q", got.Source, c.URL)
	}
	if _, err := time.Parse(time.RFC3339, got.Clipped); err != nil {
		t.Errorf("clipped %q is not RFC 3339: %v", got.Clipped, err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "clip" {
		t.Errorf("tags = %v, want [clip]", got.Tags)
	}
	if body != "\n"+c.Markdown {
		t.Errorf("body = %q, want a blank line then the markdown", body)
	}
}

func TestWriteKeepsTheMarkdownVerbatim(t *testing.T) {
	root := notesRoot(t)
	c := page("Verbatim")
	c.Markdown = "---\nnot: frontmatter\n---\n\ttab\r\ncrlf\n\n\nno trailing newline"
	rel, err := Write(root, "clips", c, when)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root.Path, rel))
	if err != nil {
		t.Fatal(err)
	}
	if _, body, _ := strings.Cut(string(data), "---\n\n"); body != c.Markdown {
		t.Errorf("body = %q, want it byte for byte", body)
	}
}

func TestWriteSuffixesOnCollision(t *testing.T) {
	root := notesRoot(t)
	var got []string
	for range 3 {
		rel, err := Write(root, "clips", page("Same Title"), when)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, rel)
	}
	want := []string{
		"clips/2026-09-10-same-title.md",
		"clips/2026-09-10-same-title-2.md",
		"clips/2026-09-10-same-title-3.md",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("clip %d = %q, want %q", i+1, got[i], want[i])
		}
	}
	// A clip the next day starts again at the unsuffixed name.
	rel, err := Write(root, "clips", page("Same Title"), when.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if rel != "clips/2026-09-11-same-title.md" {
		t.Errorf("next day = %q", rel)
	}
}

func TestWriteNeverOverwritesAnExistingNote(t *testing.T) {
	root := notesRoot(t)
	if err := os.MkdirAll(filepath.Join(root.Path, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(root.Path, "clips", "2026-09-10-taken.md")
	if err := os.WriteFile(existing, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := Write(root, "clips", page("Taken"), when)
	if err != nil {
		t.Fatal(err)
	}
	if rel == "clips/2026-09-10-taken.md" {
		t.Fatal("the clip took the existing note's name")
	}
	data, err := os.ReadFile(existing)
	if err != nil || string(data) != "mine\n" {
		t.Errorf("existing note = %q, %v", data, err)
	}
}

func TestWriteNestedAndRootClipsDir(t *testing.T) {
	root := notesRoot(t)
	rel, err := Write(root, "inbox/web", page("Nested"), when)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "inbox/web/2026-09-10-nested.md" {
		t.Fatalf("path = %q", rel)
	}
	if _, err := os.Stat(filepath.Join(root.Path, rel)); err != nil {
		t.Fatal(err)
	}
	rel, err = Write(root, ".", page("At The Top"), when)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "2026-09-10-at-the-top.md" {
		t.Errorf("path = %q, want the note in the root itself", rel)
	}
}

func TestWriteRefusesAClipsDirThatLeavesTheRoot(t *testing.T) {
	root := notesRoot(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root.Path, "clips")); err != nil {
		t.Fatal(err)
	}
	_, err := Write(root, "clips", page("Escaping"), when)
	if !errors.Is(err, roots.ErrOutside) {
		t.Fatalf("error = %v, want ErrOutside", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("wrote %d entries outside the root", len(entries))
	}
}

func TestWriteFollowsASymlinkThatStaysInsideTheRoot(t *testing.T) {
	root := notesRoot(t)
	if err := os.MkdirAll(filepath.Join(root.Path, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root.Path, "clips")); err != nil {
		t.Fatal(err)
	}
	rel, err := Write(root, "clips", page("Inside"), when)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root.Path, "real", filepath.Base(rel))); err != nil {
		t.Errorf("note not written through the alias: %v", err)
	}
}

func TestWriteRefusesAnUnknownKind(t *testing.T) {
	root := notesRoot(t)
	for _, kind := range []string{"", "PAGE", "article", "selection "} {
		c := page("Kind")
		c.Kind = kind
		if _, err := Write(root, "clips", c, when); !errors.Is(err, ErrKind) {
			t.Errorf("kind %q: error = %v, want ErrKind", kind, err)
		}
	}
	c := page("Selected")
	c.Kind = KindSelection
	if _, err := Write(root, "clips", c, when); err != nil {
		t.Errorf("selection: %v", err)
	}
}

func TestTitleIsOneBoundedLine(t *testing.T) {
	if got := Title("  a\n\tlong\r\n  title  "); got != "a long title" {
		t.Errorf("Title = %q", got)
	}
	if got := Title(strings.Repeat("x", 400)); len(got) != maxTitle {
		t.Errorf("length %d, want %d", len(got), maxTitle)
	}
	if got := Title(""); got != "" {
		t.Errorf("Title(\"\") = %q, want it left empty", got)
	}
}

func TestWriteWithAnEmptyTitle(t *testing.T) {
	root := notesRoot(t)
	rel, err := Write(root, "clips", page(""), when)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "clips/2026-09-10-"+FallbackSlug+".md" {
		t.Fatalf("path = %q", rel)
	}
	data, err := os.ReadFile(filepath.Join(root.Path, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `title: ""`) {
		t.Errorf("frontmatter did not record the empty title: %q", data)
	}
}
