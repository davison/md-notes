package render

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
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
	wantContains(t, n.HTML, `<h2 id="app">x</h2>`, `<span class="mdn-kd">w</span>`, `<a href="#fn:1">f</a>`)
	// The structural classes are the renderer's to emit, and scrubRaw
	// takes them off anything the note wrote itself, so the note's own
	// footnote-ref is gone while a real footnote keeps it — see
	// TestFootnotes (davison/md-notes#31).
	wantMissing(t, n.HTML, `javascript:alert`, `class="side pane note-title"`, `class="footnote-ref"`)
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

// lineTargetOf applies the rule the note view uses to choose the element a
// `?l=` hit scrolls to (`ui/src/note-view.tsx:156`): among the elements
// carrying a data-line at or below the requested line, the last one in
// document order with the highest value. It is reproduced here so that what
// the renderer emits is checked against the choice it feeds rather than
// against the markup alone.
func lineTargetOf(t *testing.T, doc string, want int) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	var best *html.Node
	bestLine := -1
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key != "data-line" {
					continue
				}
				if v, err := strconv.Atoi(a.Val); err == nil && v <= want && v >= bestLine {
					best, bestLine = n, v
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return best
}

// nodeText is the visible text of a parsed element.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// forgeryNote places two decoys after the block a search hit for line 5
// would really point at: one wearing the renderer's own line-anchor class,
// and one a bare paragraph wearing nothing but the attribute. Under the
// selection rule both tie with the real block and beat it, being later in
// document order.
const forgeryNote = "first\n" +
	"\n" +
	"## Two\n" +
	"\n" +
	"real target\n" +
	"\n" +
	`<div class="line-anchor" data-line="5"></div>` + "\n" +
	"\n" +
	`<p data-line="5">decoy without a class</p>` + "\n"

// A note cannot plant a scroll target: what the app's own JavaScript reads
// off the rendered markup has to come from the renderer, not from raw HTML
// the note wrote (#31, M8-R6). The class is not the vector on its own — the
// bare paragraph forges the same target — so the attribute goes with it.
func TestNoteContentCannotForgeAScrollTarget(t *testing.T) {
	n := render(t, "x.md", forgeryNote)
	target := lineTargetOf(t, n.HTML, 5)
	if target == nil {
		t.Fatalf("no scroll target at all:\n%s", n.HTML)
	}
	if got := nodeText(target); got != "real target" {
		t.Errorf("a hit on line 5 scrolls to %q, want %q — note content redirected it:\n%s",
			got, "real target", n.HTML)
	}
	// The decoys keep their text and lose only what the app reads off them.
	wantContains(t, n.HTML, "decoy without a class", "<div></div>")
	wantMissing(t, n.HTML, `data-line="5">decoy`, `class="line-anchor" data-line="5"`)
}

// scrubRaw is the only thing standing between note-written HTML and the
// attributes the application reads, so what it leaves alone matters as
// much as what it takes: everything that is not a marker comes through
// byte for byte, including near-misses.
func TestScrubRawLeavesEverythingElseAlone(t *testing.T) {
	for _, src := range []string{
		`<div class="wrap">`,
		`<p title="a note about data-line">hi</p>`,
		`<!-- data-line="5" is discussed here -->`,
		// A class that merely contains a structural name is not one.
		`<div class="note-line-anchor-list outside-rootish">`,
		`<p>the data-line attribute, written as text</p>`,
		// An unbalanced tag spanning a block keeps its shape: nothing is
		// closed or reordered on the way through.
		"<div class=\"panel\">\n",
		// Case and spacing are the author's when nothing is removed.
		`<DIV  CLASS = "wrap" >`,
		`<img src="x.png" alt="data-line">`,
	} {
		if got := string(scrubRaw([]byte(src))); got != src {
			t.Errorf("scrubRaw(%q) = %q, want it unchanged", src, got)
		}
	}
}

func TestScrubRawTakesTheMarkersOff(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`<p data-line="5">x</p>`, `<p>x</p>`},
		{`<div class="line-anchor" data-line="5"></div>`, `<div></div>`},
		{`<a class="outside-root" href="x.md">a</a>`, `<a href="x.md">a</a>`},
		// Only the structural names go; the note keeps its own classes.
		{`<span class="keep line-anchor mdn-kd">s</span>`, `<span class="keep mdn-kd">s</span>`},
		// Attribute names are not case-sensitive in HTML, and neither is
		// the strip.
		{`<p DATA-LINE="5">x</p>`, `<p>x</p>`},
		// An unbalanced tag is rewritten, not closed.
		{"<div class=\"line-anchor\">\n", "<div>\n"},
		{`<p title="keep" data-line="5">x</p>`, `<p title="keep">x</p>`},
		// A marker inside text or a comment is text, not markup.
		{`<p>data-line="5"</p>`, `<p>data-line="5"</p>`},
	} {
		if got := string(scrubRaw([]byte(c.src))); got != c.want {
			t.Errorf("scrubRaw(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

// Inline raw HTML is the other way a note writes its own markup, and a
// fenced block that talks about the attribute is displaying source rather
// than writing markup: it keeps its text.
func TestScrubReachesInlineHTMLAndNotCodeBlocks(t *testing.T) {
	n := render(t, "x.md", "para <span class=\"line-anchor\" data-line=\"3\">inline</span> on\n\n"+
		"```\n<p data-line=\"9\">shown, not applied</p>\n```\n")
	wantContains(t, n.HTML, `<span>inline</span>`, `&lt;p data-line=&#34;9&#34;&gt;shown, not applied`)
	wantMissing(t, n.HTML, `<span class="line-anchor"`, `<p data-line="9">shown`)
}

// An entity in an attribute value is decoded by the tokeniser, and by the
// sanitiser after it, so anything that decides what to scrub by scanning
// for literal bytes can be walked straight past: `class="line&#45;anchor"`
// reaches the page as the class the app styles. Found in review of
// davison/md-notes#144, finding 1.
func TestScrubRawSeesThroughEntities(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`<div class="line&#45;anchor">x</div>`, `<div>x</div>`},
		{`<div class="line&#x2D;anchor">x</div>`, `<div>x</div>`},
		{`<div class="&#x6c;ine-anchor">x</div>`, `<div>x</div>`},
		{`<div class="foot&#110;otes">x</div>`, `<div>x</div>`},
		{`<a class="outside&#45;root" href="x.md">a</a>`, `<a href="x.md">a</a>`},
		// A named entity that decodes to whitespace is a class separator,
		// so it hides a name in the middle of an attribute as well as at
		// its edges. The note keeps the class either side of it.
		{`<div class="keep&Tab;line-anchor">x</div>`, `<div class="keep">x</div>`},
		{`<div class="line-anchor&NewLine;keep">x</div>`, `<div class="keep">x</div>`},
		// An attribute name is not entity-decoded, so this one is not the
		// marker and is left where it is; the sanitiser drops it as an
		// attribute nothing allows.
		{`<p data&#45;line="5">x</p>`, `<p data&#45;line="5">x</p>`},
		// What is written back out is re-escaped, so a value the note
		// wrote as an entity is still one after a sibling is removed.
		{`<p title="a &amp; b" data-line="5">x</p>`, `<p title="a &amp; b">x</p>`},
	} {
		if got := string(scrubRaw([]byte(c.src))); got != c.want {
			t.Errorf("scrubRaw(%q) = %q, want %q", c.src, got, c.want)
		}
	}
}

// End to end, because the sanitiser decodes the entity back into the class
// the stylesheet selects on (ui/src/style.css:1180, :1190): a note must not
// reach the page wearing one.
func TestEntityEncodedClassesDoNotReachThePage(t *testing.T) {
	n := render(t, "x.md", "para\n\n"+
		`<a class="outside&#45;root" href="x.md">entity borrowed</a>`+"\n\n"+
		`<div class="foot&#110;otes">also borrowed</div>`+"\n")
	wantContains(t, n.HTML, "entity borrowed", "also borrowed")
	wantMissing(t, n.HTML, `class="outside-root"`, `class="footnotes"`, "&#45;", "&#110;")
}
