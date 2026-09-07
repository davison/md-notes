package render

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var r = New()

func render(t *testing.T, notePath, src string) Note {
	t.Helper()
	n, err := r.Render("notes", notePath, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func wantContains(t *testing.T, html string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(html, s) {
			t.Errorf("output lacks %q:\n%s", s, html)
		}
	}
}

func wantMissing(t *testing.T, html string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(html, s) {
			t.Errorf("output must not contain %q:\n%s", s, html)
		}
	}
}

func TestGFMFeatures(t *testing.T) {
	n := render(t, "a.md", `# Title

| Left | Right |
|:-----|------:|
| a    | b     |

- [x] done
- [ ] todo

~~gone~~ and https://example.com/x

Footnote here[^1].

[^1]: The note.

`+"```go\nfunc main() {}\n```\n")
	if n.Title != "Title" {
		t.Errorf("title = %q", n.Title)
	}
	wantMissing(t, n.HTML, "<h1")
	wantContains(t, n.HTML,
		`<table>`, `align="left"`, `align="right"`,
		`type="checkbox"`, `checked=""`, `disabled=""`,
		`<del>gone</del>`,
		`<a href="https://example.com/x"`, `target="_blank"`, `rel="nofollow noopener"`,
		`class="footnote-ref"`, `id="fn:1"`, `class="footnote-backref"`,
		`<pre class="chroma">`, `<span class="kd">func</span>`,
	)
}

func TestFrontmatter(t *testing.T) {
	n := render(t, "docs/x.md", "---\ntitle: My Note\ntags: [a, b]\n---\n\nBody **here**.\n")
	if n.Title != "My Note" {
		t.Errorf("title = %q", n.Title)
	}
	if tags, ok := n.Frontmatter["tags"].([]any); !ok || len(tags) != 2 {
		t.Errorf("frontmatter = %#v", n.Frontmatter)
	}
	wantContains(t, n.HTML, "<strong>here</strong>")
	wantMissing(t, n.HTML, "title:", "<hr")
}

func TestFrontmatterCRLFAndDots(t *testing.T) {
	n := render(t, "x.md", "---\r\ntitle: T\r\n...\r\nBody\r\n")
	if n.Title != "T" || !strings.Contains(n.HTML, "Body") {
		t.Errorf("note = %+v", n)
	}
}

func TestEmptyFrontmatter(t *testing.T) {
	n := render(t, "x.md", "---\n---\nBody\n")
	if n.Frontmatter == nil || len(n.Frontmatter) != 0 {
		t.Errorf("frontmatter = %#v, want empty map", n.Frontmatter)
	}
	wantContains(t, n.HTML, "<p>Body</p>")
	wantMissing(t, n.HTML, "<hr")
}

func TestNonMappingFrontmatterIsBody(t *testing.T) {
	for _, src := range []string{"---\n- a\n- b\n---\nBody\n", "---\nscalar\n---\nBody\n"} {
		n := render(t, "x.md", src)
		if n.Frontmatter != nil {
			t.Errorf("%q: frontmatter = %#v, want nil", src, n.Frontmatter)
		}
	}
}

func TestMalformedFrontmatterIsBody(t *testing.T) {
	n := render(t, "x.md", "---\nnot: [valid\n---\nBody\n")
	if n.Frontmatter != nil {
		t.Errorf("frontmatter = %#v, want nil", n.Frontmatter)
	}
	// An unclosed block is body too.
	n = render(t, "x.md", "---\ntitle: T\nBody\n")
	if n.Frontmatter != nil || n.Title != "x" {
		t.Errorf("unclosed: %+v", n)
	}
}

func TestTitleSources(t *testing.T) {
	if n := render(t, "sub/file-name.md", "no heading\n"); n.Title != "file-name" {
		t.Errorf("filename title = %q", n.Title)
	}
	n := render(t, "x.md", "## Two\n\n# The *real* `title`\n\n# Second\n")
	if n.Title != "The real title" {
		t.Errorf("h1 title = %q", n.Title)
	}
	// The heading that supplied the title is removed; a later H1 stays.
	wantMissing(t, n.HTML, "real")
	wantContains(t, n.HTML, "<h2 id=\"two\">Two</h2>", "<h1 id=\"second\">Second</h1>")
	if n = render(t, "x.md", "---\ntitle: FM\n---\n# Kept\n"); !strings.Contains(n.HTML, "Kept") {
		t.Errorf("frontmatter title must not remove the H1: %s", n.HTML)
	}
	// Nested headings are neither titles nor removed.
	n = render(t, "quoted.md", "> # Quoted\n\n- # Listed\n")
	if n.Title != "quoted" {
		t.Errorf("nested h1 title = %q, want file name", n.Title)
	}
	wantContains(t, n.HTML, "Quoted", "Listed")
	if n := render(t, "x.md", "---\ntitle: ''\n---\n# From H1\n"); n.Title != "From H1" {
		t.Errorf("empty fm title = %q", n.Title)
	}
}

func TestLinkRewriting(t *testing.T) {
	cases := []struct {
		name, md, want string
	}{
		{"sibling", "[a](other.md)", `href="/r/notes/docs/other.md">`},
		{"markdown ext case", "[a](Other.MD)", `href="/r/notes/docs/Other.MD"`},
		{"parent", "[a](../top.md)", `href="/r/notes/top.md"`},
		{"root relative", "[a](/index.md)", `href="/r/notes/index.md"`},
		{"fragment kept", "[a](other.md#sec)", `href="/r/notes/docs/other.md#sec"`},
		{"fragment only", "[a](#here)", `href="#here"`},
		{"absolute", "[a](https://x.example/p.md)", `href="https://x.example/p.md"`},
		{"protocol relative", "[a](//x.example/p.md)", `href="//x.example/p.md"`},
		{"mailto", "[a](mailto:x@example.com)", `href="mailto:x@example.com"`},
		{"raw file new tab", "[a](paper.pdf)", `href="/api/r/notes/raw/docs/paper.pdf" target="_blank"`},
		{"image", "![p](img/pic.png)", `src="/api/r/notes/raw/docs/img/pic.png"`},
		{"image of markdown is raw", "![p](other.md)", `src="/api/r/notes/raw/docs/other.md"`},
		{"spaces encoded", "[a](<my note.md>)", `href="/r/notes/docs/my%20note.md"`},
		{"already encoded", "[a](my%20note.md)", `href="/r/notes/docs/my%20note.md"`},
		{"escapes root", "[a](../../etc/passwd)", `<a title="Link target is outside this root" class="outside-root">a</a>`},
		{"escapes root via root-relative", "[a](/../x.md)", `href="/r/notes/x.md"`},
		{"image escapes root", "![alt](../../x.png)", `<img alt="alt" title="Image is outside this root" class="outside-root">`},
	}
	for _, c := range cases {
		n := render(t, "docs/note.md", c.md)
		if !strings.Contains(n.HTML, c.want) {
			t.Errorf("%s: %q lacks %q:\n%s", c.name, c.md, c.want, n.HTML)
		}
	}
}

func TestSanitisation(t *testing.T) {
	n := render(t, "x.md", `<script>alert(1)</script>

<p onclick="alert(1)">para</p>

[js](javascript:alert(1))

<iframe src="https://x.example"></iframe>

<img src="x.png" onerror="alert(1)">

- [ ] keep me
`)
	wantMissing(t, n.HTML, "<script", "onclick", "javascript:", "<iframe", "onerror")
	// Raw HTML images are kept but not rewritten: only markdown image
	// syntax is resolved against the root.
	wantContains(t, n.HTML, "<p>para</p>", `type="checkbox"`, `<img src="x.png">`)
}

func TestIDsAndClassesAreScoped(t *testing.T) {
	n := render(t, "x.md", `<h2 id="app">x</h2>

<h2 id="javascript:alert(1)">y</h2>

<p class="side pane note-title">z</p>

<span class="kd">w</span>

<a class="footnote-ref" href="#fn:1">f</a>
`)
	wantContains(t, n.HTML, `<h2 id="app">x</h2>`, `<span class="kd">w</span>`, `class="footnote-ref"`)
	wantMissing(t, n.HTML, `javascript:alert`, `class="side pane note-title"`)
}

// TestChromaClassesPassSanitiser reads the generated stylesheet and checks
// every token class it styles survives the class allow-list, so the two
// cannot drift apart.
func TestChromaClassesPassSanitiser(t *testing.T) {
	css, err := os.ReadFile("../../ui/src/chroma.css")
	if err != nil {
		t.Fatal(err)
	}
	classes := regexp.MustCompile(`\.chroma \.([A-Za-z0-9]+)`).FindAllStringSubmatch(string(css), -1)
	if len(classes) < 20 {
		t.Fatalf("found only %d chroma classes; is the stylesheet generated?", len(classes))
	}
	seen := map[string]bool{}
	for _, m := range classes {
		c := m[1]
		if seen[c] {
			continue
		}
		seen[c] = true
		out := r.policy.Sanitize(`<span class="` + c + `">x</span>`)
		if !strings.Contains(out, `class="`+c+`"`) {
			t.Errorf("chroma class %q is stripped by the sanitiser", c)
		}
	}
	for _, c := range []string{"c1", "s1", "s2"} {
		if !seen[c] {
			t.Errorf("stylesheet lacks %q; the check is weaker than intended", c)
		}
	}
}

func TestRawHTMLAllowedSubset(t *testing.T) {
	n := render(t, "x.md", "<details><summary>More</summary>\n\nHidden\n\n</details>\n\n<kbd>Ctrl</kbd>\n")
	wantContains(t, n.HTML, "<kbd>Ctrl</kbd>")
}
