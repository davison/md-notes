package render

import (
	"io/fs"
	"os"
	"path/filepath"
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
		`<table data-line="3">`, `align="left"`, `align="right"`,
		`type="checkbox"`, `checked=""`, `disabled=""`,
		`<del>gone</del>`,
		`<a href="https://example.com/x"`, `target="_blank"`, `rel="nofollow noopener"`,
		`class="footnote-ref"`, `id="fn:1"`, `class="footnote-backref"`,
		`<pre class="mdn-chroma">`, `<span class="mdn-kd">func</span>`,
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
	wantContains(t, n.HTML, `<p data-line="3">Body</p>`)
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
	wantContains(t, n.HTML, `<h2 id="two" data-line="1">Two</h2>`, `<h1 id="second" data-line="5">Second</h1>`)
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

<span class="mdn-kd">w</span>

<a class="footnote-ref" href="#fn:1">f</a>
`)
	wantContains(t, n.HTML, `<h2 id="app">x</h2>`, `<span class="mdn-kd">w</span>`, `class="footnote-ref"`)
	wantMissing(t, n.HTML, `javascript:alert`, `class="side pane note-title"`)
}

// TestNoteCannotBorrowAppClasses renders a note that tries to dress
// itself in the application's own classes, short ones included, next to a
// code block that must still come out highlighted.
func TestNoteCannotBorrowAppClasses(t *testing.T) {
	n := render(t, "x.md", `<div class="shell">

<p class="error">boom</p>

<span class="nav">a</span> <span class="hit">b</span> <span class="ln">c</span>
<span class="tag">d</span> <span class="ctx">e</span> <code class="cm-editor">f</code>

</div>

`+"```go\nfunc main() {}\n```\n")
	wantMissing(t, n.HTML,
		`class="shell"`, `class="error"`, `class="nav"`, `class="hit"`,
		`class="ln"`, `class="tag"`, `class="ctx"`, `class="cm-editor"`)
	wantContains(t, n.HTML, "boom", `<pre class="mdn-chroma">`, `<span class="mdn-kd">func</span>`)
}

// generatedStylesheet is the one stylesheet under ui/src that the app
// does not write: gencss emits it, in the reserved namespace, so it is
// the single exception to the naming rule the test below enforces.
const generatedStylesheet = "chroma.css"

// TestAppClassesAreUnreachable reads every stylesheet the application
// writes and checks that each class they style is stripped from note
// content, on every element the policy allows a class on, and that none
// of them takes a name in the reserved namespace. The note's own classes
// are the deliberate exception; anything else the app adds later —
// including the editor's CodeMirror classes, which share the document
// with a rendered note — fails this test the moment it becomes
// reachable. Stylesheets are found rather than named, so adding one puts
// it under the same rule instead of quietly outside it.
func TestAppClassesAreUnreachable(t *testing.T) {
	sheets := appStylesheets(t)
	if len(sheets) == 0 {
		t.Fatal("no application stylesheets found under ui/src")
	}
	classes := map[string]string{}
	for _, path := range sheets {
		css, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for c := range cssClasses(string(css)) {
			classes[c] = path
		}
	}
	// The stylesheets are the input to the check, so a scan that found
	// nothing must not pass silently.
	for _, want := range []string{"shell", "nav", "hit", "ln", "ctx", "tag", "error", "cm-editor", "markdown"} {
		if _, ok := classes[want]; !ok {
			t.Fatalf("class %q not found in the app stylesheets; the selector scan is broken", want)
		}
	}
	elements := append(append([]string{}, codeClassElements...), noteClassElements...)
	for c, path := range classes {
		if noteClassPattern.MatchString(c) {
			continue
		}
		if strings.HasPrefix(c, ClassPrefix) {
			t.Errorf("%s: app class %q uses the prefix reserved for note content (%q)", path, c, ClassPrefix)
			continue
		}
		for _, el := range elements {
			out := r.policy.Sanitize(`<` + el + ` class="` + c + `">x</` + el + `>`)
			if strings.Contains(out, "class=") {
				t.Errorf("%s: app class %q survives on <%s>: %s", path, c, el, out)
			}
		}
	}
}

// appStylesheets returns every stylesheet under ui/src that the
// application writes itself, generated ones excepted.
func appStylesheets(t *testing.T) []string {
	t.Helper()
	var out []string
	root := filepath.Join("..", "..", "ui", "src")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".css" || d.Name() == generatedStylesheet {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// cssClasses returns the class names named in the selectors of a
// stylesheet. Comments are dropped, and the text between a brace and the
// next opening brace is a selector, so rules nested in an at-rule are
// scanned and declarations are not.
func cssClasses(css string) map[string]bool {
	for {
		i := strings.Index(css, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(css[i+2:], "*/")
		if j < 0 {
			css = css[:i]
			break
		}
		css = css[:i] + " " + css[i+2+j+2:]
	}
	name := regexp.MustCompile(`\.(-?[A-Za-z_][A-Za-z0-9_-]*)`)
	out := map[string]bool{}
	start := 0
	for i, ch := range css {
		switch ch {
		case '{':
			for _, m := range name.FindAllStringSubmatch(css[start:i], -1) {
				out[m[1]] = true
			}
			start = i + 1
		case '}':
			start = i + 1
		}
	}
	return out
}

// TestChromaClassesPassSanitiser reads the generated stylesheet and checks
// every token class it styles survives the class allow-list, so the two
// cannot drift apart.
func TestChromaClassesPassSanitiser(t *testing.T) {
	css, err := os.ReadFile("../../ui/src/chroma.css")
	if err != nil {
		t.Fatal(err)
	}
	prefix := regexp.QuoteMeta(ClassPrefix)
	sel := regexp.MustCompile(`\.` + prefix + `chroma \.(` + prefix + `[A-Za-z0-9]+)`)
	classes := sel.FindAllStringSubmatch(string(css), -1)
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
	for _, c := range []string{ClassPrefix + "c1", ClassPrefix + "s1", ClassPrefix + "s2"} {
		if !seen[c] {
			t.Errorf("stylesheet lacks %q; the check is weaker than intended", c)
		}
	}
}

func TestLineMarkers(t *testing.T) {
	n := render(t, "x.md", `---
title: T
tags: [a]
---
# Heading

Paragraph one
continues.

- item one
- item two

> quoted

| a | b |
|---|---|
| 1 | 2 |

`+"```go\nx := 1\n```\n")
	// Frontmatter occupies lines 1-4, so the body starts at file line 5.
	wantContains(t, n.HTML,
		`<h1 id="heading" data-line="5">`,
		`<p data-line="7">`,
		`<ul data-line="10">`,
		`<li data-line="10">`,
		`<li data-line="11">`,
		`<blockquote data-line="13">`,
		`<table data-line="15">`,
		`<div class="line-anchor" data-line="19"></div>`,
	)
}

func TestLineAnchorsForAttributelessBlocks(t *testing.T) {
	n := render(t, "x.md", "para\n\n---\n\n<div>raw</div>\n\n    indented code\n\n```\nfence\n```\n")
	// A rule has no source segment; its anchor sits just after the block
	// before it, which is close enough for scrolling.
	wantContains(t, n.HTML,
		`<p data-line="1">`,
		`<div class="line-anchor" data-line="2"></div>`,
		`<div class="line-anchor" data-line="5"></div>`,
		`<div class="line-anchor" data-line="7"></div>`,
		`<div class="line-anchor" data-line="9"></div>`,
	)
}

func TestLineMarkersWithoutFrontmatter(t *testing.T) {
	n := render(t, "x.md", "para\n\n## Two\n")
	wantContains(t, n.HTML, `<p data-line="1">`, `<h2 id="two" data-line="3">`)
}

func TestLineMarkerSanitised(t *testing.T) {
	out := r.policy.Sanitize(`<p data-line="12">a</p><p data-line="x">b</p><span data-line="3">c</span>`)
	if out != `<p data-line="12">a</p><p>b</p><span>c</span>` {
		t.Fatalf("sanitised = %s", out)
	}
}

func TestRawHTMLAllowedSubset(t *testing.T) {
	n := render(t, "x.md", "<details><summary>More</summary>\n\nHidden\n\n</details>\n\n<kbd>Ctrl</kbd>\n")
	wantContains(t, n.HTML, "<kbd>Ctrl</kbd>")
}
