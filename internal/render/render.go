// Package render turns a markdown note into sanitised HTML with its
// frontmatter separated, links rewritten for the app, and code highlighted.
package render

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
	"gopkg.in/yaml.v3"

	"github.com/davison/md-notes/internal/tree"
)

// Note is a rendered markdown file.
type Note struct {
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	HTML        string         `json:"html"`
	// Diagrams are the note's fenced mermaid flowcharts that the diagram
	// package will draw, in document order. They are listed beside the HTML
	// rather than marked in it: see Diagram.
	Diagrams []Diagram `json:"diagrams,omitempty"`
}

// Renderer is safe for concurrent use.
type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
	code   *lexerCache
}

// New builds a Renderer with GitHub-flavoured markdown, footnotes, heading
// IDs, chroma highlighting emitting classes, and the app's link rewriting.
func New() *Renderer {
	// The highlighter draws fenced code, but only once Render has settled
	// each block's lexer and handed it over: see codeBlock.
	highlighter := highlighting.NewHTMLRenderer(
		highlighting.WithFormatOptions(
			chromahtml.WithClasses(true),
			chromahtml.ClassPrefix(ClassPrefix),
		),
	)
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
			extension.Footnote,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(&linkRewriter{}, 100),
				util.Prioritized(&lineMarker{}, 200),
				util.Prioritized(diagramFinder{}, 300),
			),
		),
		goldmark.WithRendererOptions(
			// A note's own HTML is written through, less the markers the
			// application reads off rendered markup: see rawHTMLRenderer.
			html.WithUnsafe(),
			renderer.WithNodeRenderers(
				util.Prioritized(lineAnchorRenderer{}, 500),
				util.Prioritized(rawHTMLRenderer{}, 500),
				// Where goldmark-highlighting's own Extend puts it, so it
				// gets the same options from goldmark.
				util.Prioritized(highlighter, 200),
				util.Prioritized(codeBlockRenderer{highlight: funcOf(highlighter, ast.KindFencedCodeBlock)}, 500),
			),
		),
	)
	return &Renderer{md: md, policy: newPolicy(), code: &lexerCache{names: map[string]string{}}}
}

// Render renders src, the content of the note at notePath inside the root
// named slug.
func (r *Renderer) Render(slug, notePath string, src []byte) (Note, error) {
	fm, body, doc, ctx := r.parse(slug, notePath, src)

	// The app shows the title above the note, so a heading that supplied it
	// is removed from the body rather than shown twice.
	title := ""
	if t, ok := fm["title"].(string); ok && strings.TrimSpace(t) != "" {
		title = strings.TrimSpace(t)
	} else if h, node := firstH1(doc, body); h != "" {
		title = h
		node.Parent().RemoveChild(node.Parent(), node)
	} else {
		title = strings.TrimSuffix(path.Base(notePath), path.Ext(notePath))
	}

	if r.code != nil {
		// Without it, which only the tests arrange, goldmark-highlighting
		// draws every fenced block itself.
		body = r.code.resolveCode(doc, body)
	}
	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, body, doc); err != nil {
		return Note{}, fmt.Errorf("render %s: %w", notePath, err)
	}

	return Note{
		Path:        notePath,
		Title:       title,
		Frontmatter: fm,
		HTML:        r.policy.Sanitize(buf.String()),
		Diagrams:    diagramsOf(ctx),
	}, nil
}

// Diagrams lists the flowcharts in a note's source exactly as Render lists
// them, without rendering the rest: the diagram route finds a block again
// with it, so what a hash names is decided in one place.
func (r *Renderer) Diagrams(src []byte) []Diagram {
	_, _, _, ctx := r.parse("", ".", src)
	return diagramsOf(ctx)
}

// parse splits off the frontmatter and parses the body, running the
// transformers: the link rewriting, the line markers and the diagram list.
func (r *Renderer) parse(slug, notePath string, src []byte) (map[string]any, []byte, ast.Node, parser.Context) {
	fm, body := splitFrontmatter(src)
	ctx := parser.NewContext()
	ctx.Set(linkContextKey, linkContext{slug: slug, noteDir: path.Dir(notePath)})
	// Lines removed with the frontmatter, so markers count from the file's
	// first line as ripgrep does.
	ctx.Set(lineOffsetKey, bytes.Count(src[:len(src)-len(body)], []byte{'\n'}))
	doc := r.md.Parser().Parse(text.NewReader(body), parser.WithContext(ctx))
	return fm, body, doc, ctx
}

// splitFrontmatter separates a leading YAML block delimited by "---" lines.
// Anything that is not a well-formed mapping is left in the body.
func splitFrontmatter(src []byte) (map[string]any, []byte) {
	s := src
	if !bytes.HasPrefix(s, []byte("---\n")) && !bytes.HasPrefix(s, []byte("---\r\n")) {
		return nil, src
	}
	nl := bytes.IndexByte(s, '\n')
	rest := s[nl+1:]
	// Find the closing delimiter on its own line.
	var end, after int = -1, 0
	for pos := 0; pos <= len(rest); {
		lineEnd := bytes.IndexByte(rest[pos:], '\n')
		var line []byte
		if lineEnd < 0 {
			line = rest[pos:]
			lineEnd = len(rest) - pos
		} else {
			line = rest[pos : pos+lineEnd]
		}
		trimmed := bytes.TrimRight(line, "\r")
		if bytes.Equal(trimmed, []byte("---")) || bytes.Equal(trimmed, []byte("...")) {
			end = pos
			after = pos + lineEnd + 1
			break
		}
		pos += lineEnd + 1
	}
	if end < 0 {
		return nil, src
	}
	var fm map[string]any
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return nil, src
	}
	if fm == nil {
		// An empty block is still frontmatter, with nothing in it.
		fm = map[string]any{}
	}
	if after > len(rest) {
		after = len(rest)
	}
	return fm, rest[after:]
}

// firstH1 returns the plain text of the first top-level level-one heading
// and the heading node itself, or "" and nil. Headings nested in quotes or
// lists are not titles and are left alone.
func firstH1(doc ast.Node, src []byte) (string, ast.Node) {
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			if t := strings.TrimSpace(plainText(h, src)); t != "" {
				return t, h
			}
			return "", nil
		}
	}
	return "", nil
}

func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
		case *ast.String:
			b.Write(t.Value)
		default:
			b.WriteString(plainText(c, src))
		}
	}
	return b.String()
}

// Link rewriting ---------------------------------------------------------

type linkContext struct {
	slug    string
	noteDir string
}

var linkContextKey = parser.NewContextKey()

type linkRewriter struct{}

func (linkRewriter) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	lc, _ := pc.Get(linkContextKey).(linkContext)
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch l := n.(type) {
		case *ast.Link:
			dest, raw := rewrite(lc, string(l.Destination), true)
			if dest == "" && len(l.Destination) > 0 {
				l.Title = []byte("Link target is outside this root")
				l.SetAttributeString("class", []byte("outside-root"))
			}
			l.Destination = []byte(dest)
			if raw {
				l.SetAttributeString("target", []byte("_blank"))
			}
		case *ast.Image:
			dest, _ := rewrite(lc, string(l.Destination), false)
			if dest == "" && len(l.Destination) > 0 {
				l.Title = []byte("Image is outside this root")
				l.SetAttributeString("class", []byte("outside-root"))
			}
			l.Destination = []byte(dest)
		}
		return ast.WalkContinue, nil
	})
}

// rewrite maps a link destination written in a note to the URL the app
// serves it at. Markdown targets become in-app routes; other relative
// files are served raw. The second result reports a raw-file link, which
// opens in a new tab so the client-side router leaves it alone. Absolute
// URLs and fragments are returned as they are; a target that escapes the
// root becomes an empty destination.
func rewrite(lc linkContext, dest string, isLink bool) (string, bool) {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "//") {
		return dest, false
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return dest, false
	}
	target := u.Path
	if target == "" {
		return dest, false
	}
	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = path.Clean(target)[1:]
	} else {
		resolved = path.Clean(path.Join(lc.noteDir, target))
	}
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		// Left relative, the browser would resolve this against the app's
		// route and land on a dead page. An empty destination keeps the
		// text and the title says why.
		return "", false
	}
	if resolved == "." {
		return dest, false
	}
	suffix := ""
	if u.RawQuery != "" {
		suffix += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		suffix += "#" + u.EscapedFragment()
	}
	if isLink && tree.IsMarkdown(resolved) {
		return "/r/" + url.PathEscape(lc.slug) + "/" + encodePath(resolved) + suffix, false
	}
	return "/api/r/" + url.PathEscape(lc.slug) + "/raw/" + encodePath(resolved) + suffix, isLink
}

func encodePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// Source line markers ----------------------------------------------------

var lineOffsetKey = parser.NewContextKey()

// lineMarker sets data-line on every block element to the 1-based source
// line it starts at, so a search hit can be scrolled to in the rendered
// note. Blocks without their own source lines, such as lists, take the
// line of their first descendant that has one.
type lineMarker struct{}

func (lineMarker) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	offset, _ := pc.Get(lineOffsetKey).(int)
	// Byte offset of the start of each line, for a binary search.
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	lineOf := func(pos int) int {
		return sort.Search(len(starts), func(i int) bool { return starts[i] > pos }) + offset
	}
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Type() != ast.TypeBlock || n == doc {
			return ast.WalkContinue, nil
		}
		if _, isAnchor := n.(*lineAnchor); isAnchor {
			return ast.WalkSkipChildren, nil
		}
		pos, ok := firstSegment(n)
		if !ok {
			if _, hr := n.(*ast.ThematicBreak); !hr {
				return ast.WalkContinue, nil
			}
			// A thematic break has no segment of its own; place it after
			// the previous block's last line.
			pos = -1
			if prev := n.PreviousSibling(); prev != nil {
				if last, ok := lastSegmentEnd(prev); ok {
					pos = last
				}
			}
			if pos < 0 {
				return ast.WalkContinue, nil
			}
		}
		line := lineOf(pos)
		switch n.(type) {
		case *ast.FencedCodeBlock:
			// The opening fence is the line before the content; the
			// highlighter drops node attributes, so an anchor precedes it.
			anchorBefore(n, line-1)
		case *ast.CodeBlock, *ast.HTMLBlock:
			anchorBefore(n, line)
		case *ast.ThematicBreak:
			anchorBefore(n, line+1)
		default:
			n.SetAttributeString("data-line", []byte(strconv.Itoa(line)))
		}
		return ast.WalkContinue, nil
	})
}

// lastSegmentEnd returns the byte offset of the end of n's last source
// segment, searching its descendants.
func lastSegmentEnd(n ast.Node) (int, bool) {
	if lines := n.Lines(); lines != nil && lines.Len() > 0 {
		return lines.At(lines.Len() - 1).Stop, true
	}
	for c := n.LastChild(); c != nil; c = c.PreviousSibling() {
		if end, ok := lastSegmentEnd(c); ok {
			return end, true
		}
	}
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Stop, true
	}
	return 0, false
}

func anchorBefore(n ast.Node, line int) {
	if p := n.Parent(); p != nil {
		p.InsertBefore(p, n, &lineAnchor{line: line})
	}
}

// lineAnchor is an empty block that carries a data-line marker for a
// following block whose renderer would discard attributes.
type lineAnchor struct {
	ast.BaseBlock
	line int
}

var kindLineAnchor = ast.NewNodeKind("LineAnchor")

func (n *lineAnchor) Kind() ast.NodeKind { return kindLineAnchor }

func (n *lineAnchor) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type lineAnchorRenderer struct{}

func (r lineAnchorRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindLineAnchor, r.render)
}

func (lineAnchorRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, "<div class=\"line-anchor\" data-line=\"%d\"></div>\n", n.(*lineAnchor).line)
	}
	return ast.WalkContinue, nil
}

// Raw HTML from the note -------------------------------------------------

// rawHTMLRenderer writes the HTML a note wrote itself. goldmark runs with
// WithUnsafe, so it would otherwise reach the output verbatim, and its
// KindHTMLBlock and KindRawHTML nodes are the only way it can: no
// attribute syntax is enabled in the parser, so nothing else a note writes
// carries attributes of its choosing.
type rawHTMLRenderer struct{}

func (r rawHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, r.renderBlock)
	reg.Register(ast.KindRawHTML, r.renderInline)
}

func (rawHTMLRenderer) renderBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.HTMLBlock)
	var raw []byte
	if entering {
		// Scrubbed as one block rather than line by line: a tag may be
		// written across several lines of it.
		lines := n.Lines()
		for i := range lines.Len() {
			line := lines.At(i)
			raw = append(raw, line.Value(source)...)
		}
	} else {
		if !n.HasClosure() {
			return ast.WalkContinue, nil
		}
		raw = n.ClosureLine.Value(source)
	}
	html.DefaultWriter.SecureWrite(w, scrubRaw(raw))
	return ast.WalkContinue, nil
}

func (rawHTMLRenderer) renderInline(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.RawHTML)
	var raw []byte
	for i := range n.Segments.Len() {
		seg := n.Segments.At(i)
		raw = append(raw, seg.Value(source)...)
	}
	_, _ = w.Write(scrubRaw(raw))
	return ast.WalkSkipChildren, nil
}

// scrubRaw takes off HTML the note wrote the two things the application
// reads back off rendered markup: the data-line attribute the note view
// scrolls a search hit by, and the structural class names the renderer
// emits. Without it a note can plant a decoy scroll target, because one
// sanitiser pass runs over the whole rendered document and no pattern in
// it can tell the renderer's markup from the note's (davison/md-notes#31).
// This runs before that pass, where the two are still distinguishable.
//
// What it does not touch it does not rewrite: a tag is re-emitted only
// when something was actually removed from it, and every other token is
// copied byte for byte, so unbalanced tags spanning a block, comments,
// entities and text come through as they were written.
//
// Every fragment goes through the tokeniser. A cheaper first look that
// scanned for the literal names could not be made to agree with it: the
// tokeniser decodes entities in an attribute value, and the sanitiser
// decodes them again afterwards, so `class="line&#45;anchor"` carries
// none of the bytes such a scan looks for and reaches the page as the
// class the stylesheet selects on. One decision procedure, which is the
// one that reads the value the browser will (davison/md-notes#144).
func scrubRaw(src []byte) []byte {
	z := xhtml.NewTokenizer(bytes.NewReader(src))
	var out bytes.Buffer
	for {
		tt := z.Next()
		if tt != xhtml.StartTagToken && tt != xhtml.SelfClosingTagToken {
			out.Write(z.Raw())
			if tt == xhtml.ErrorToken {
				return out.Bytes()
			}
			continue
		}
		// Raw is only valid until the token is parsed, and it is what an
		// untouched tag is written from.
		raw := append([]byte(nil), z.Raw()...)
		tok := z.Token()
		kept, stripped := tok.Attr[:0], false
		for _, a := range tok.Attr {
			switch a.Key {
			case lineAttr:
				stripped = true
				continue
			case "class":
				if v := withoutStructural(a.Val); v != a.Val {
					stripped = true
					if v == "" {
						continue
					}
					a.Val = v
				}
			}
			kept = append(kept, a)
		}
		if !stripped {
			out.Write(raw)
			continue
		}
		tok.Attr = kept
		out.WriteString(tok.String())
	}
}

// withoutStructural drops the renderer's own class names from a class
// attribute a note wrote, leaving the rest of it in place.
func withoutStructural(class string) string {
	kept := make([]string, 0, 4)
	for _, f := range strings.Fields(class) {
		if !slices.Contains(structuralClasses, f) {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(strings.Fields(class)) {
		return class
	}
	return strings.Join(kept, " ")
}

// firstSegment returns the byte offset of the first source segment of n
// or of its first descendant that has one.
func firstSegment(n ast.Node) (int, bool) {
	if lines := n.Lines(); lines != nil && lines.Len() > 0 {
		return lines.At(0).Start, true
	}
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start, true
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if pos, ok := firstSegment(c); ok {
			return pos, true
		}
	}
	return 0, false
}

// Sanitisation -----------------------------------------------------------

// ClassPrefix is the namespace reserved for classes that rendered note
// content may carry. Chroma emits its token classes under it, and the
// sanitiser admits nothing else on a note's code elements, so what a note
// can apply to itself does not depend on what the application calls its
// own classes. The one rule the app's stylesheets must keep is never to
// use the prefix themselves; TestAppClassesAreUnreachable enforces it.
const ClassPrefix = "mdn-"

// lineAttr is the attribute the note view scrolls a search hit by. The
// renderer puts it on the blocks it emits; scrubRaw takes it off anything
// the note wrote, so what the application reads is the renderer's alone.
const lineAttr = "data-line"

var (
	// Heading IDs from goldmark, plus the footnote IDs it generates.
	idPattern = regexp.MustCompile(`^(fn|fnref):\d+$|^[\pL\pN_\-]+$`)
	// Chroma's token classes, emitted under ClassPrefix, and goldmark's
	// language- class for a fenced block it could not tokenise — which
	// nothing styles, but which is the markup convention for one.
	// TestChromaClassesPassSanitiser ties this to the generated stylesheet.
	codeClassPattern = classList(regexp.QuoteMeta(ClassPrefix)+`[a-z0-9]+`, `language-[\w+#.\-]+`)
	// The classes the note's own structure carries: goldmark's footnotes,
	// and this package's own two. They are named in full, and the app
	// styles them only inside the rendered note. The sanitiser admits them
	// and scrubRaw takes them off anything the note wrote itself, so the
	// two are built from one list rather than from two that could drift.
	structuralClasses = []string{"footnotes", "footnote-ref", "footnote-backref", "outside-root", "line-anchor"}
	noteClassPattern  = regexp.MustCompile(`^(` + strings.Join(structuralClasses, "|") + `)$`)

	// The only elements the policy allows a class attribute on. Named
	// here rather than at the call site so that the sanitiser and
	// TestAppClassesAreUnreachable, which proves no app class survives on
	// any of them, cannot come to disagree about which those are.
	codeClassElements = []string{"pre", "code", "span"}
	noteClassElements = []string{"a", "img", "div", "section"}
)

// classList matches a class attribute whose every entry is one of alts.
func classList(alts ...string) *regexp.Regexp {
	one := `(?:` + strings.Join(alts, "|") + `)`
	return regexp.MustCompile(`^` + one + `( ` + one + `)*$`)
}

// newPolicy is bluemonday's user-generated-content element set, without
// its global id allowance (whose pattern is unanchored), extended with
// what goldmark's task lists, heading IDs, footnotes, and chroma emit.
func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowStandardURLs()
	p.AllowAttrs("title").Matching(bluemonday.Paragraph).Globally()
	p.AllowAttrs("dir").Matching(bluemonday.Direction).Globally()
	p.AllowAttrs("lang").Matching(regexp.MustCompile(`^[a-zA-Z]{2,20}$`)).Globally()
	p.AllowElements("article", "aside", "section", "figure", "figcaption", "details", "summary",
		"h1", "h2", "h3", "h4", "h5", "h6", "hgroup",
		"br", "div", "hr", "p", "span", "wbr",
		"abbr", "acronym", "cite", "code", "dfn", "em", "s", "strong", "sub", "sup", "var",
		"b", "i", "pre", "small", "strike", "tt", "u", "rp", "rt", "ruby", "q", "time", "bdi", "bdo")
	p.AllowAttrs("open").Matching(regexp.MustCompile(`(?i)^(|open)$`)).OnElements("details")
	p.AllowAttrs("cite").OnElements("blockquote", "q")
	p.AllowAttrs("cite").Matching(bluemonday.Paragraph).OnElements("del", "ins")
	p.AllowAttrs("datetime").Matching(bluemonday.ISO8601).OnElements("time", "del", "ins")
	p.AllowAttrs("href").OnElements("a")
	p.AllowLists()
	p.AllowTables()
	p.AllowImages()
	p.AllowAttrs("id").Matching(idPattern).OnElements("h1", "h2", "h3", "h4", "h5", "h6", "sup", "li")
	p.AllowAttrs(lineAttr).Matching(regexp.MustCompile(`^[0-9]{1,9}$`)).OnElements(
		"p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "table", "pre", "hr", "div")
	p.AllowAttrs("class").Matching(codeClassPattern).OnElements(codeClassElements...)
	p.AllowAttrs("class").Matching(noteClassPattern).OnElements(noteClassElements...)
	p.AllowAttrs("role").Matching(regexp.MustCompile(`^doc-(noteref|endnotes|backlink)$`)).OnElements("a", "div", "section")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	p.AllowAttrs("align").Matching(regexp.MustCompile(`^(left|center|right)$`)).OnElements("td", "th")
	p.AllowElements("kbd", "samp", "mark")
	p.AllowRelativeURLs(true)
	// nofollow is only meaningful on links leaving the app.
	p.RequireNoFollowOnLinks(false)
	p.RequireNoFollowOnFullyQualifiedLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}
