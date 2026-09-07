// Package render turns a markdown note into sanitised HTML with its
// frontmatter separated, links rewritten for the app, and code highlighted.
package render

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"regexp"
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
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"

	"github.com/davison/md-notes/internal/tree"
)

// Note is a rendered markdown file.
type Note struct {
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	HTML        string         `json:"html"`
}

// Renderer is safe for concurrent use.
type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

// New builds a Renderer with GitHub-flavoured markdown, footnotes, heading
// IDs, chroma highlighting emitting classes, and the app's link rewriting.
func New() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(&linkRewriter{}, 100),
				util.Prioritized(&lineMarker{}, 200),
			),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	return &Renderer{md: md, policy: newPolicy()}
}

// Render renders src, the content of the note at notePath inside the root
// named slug.
func (r *Renderer) Render(slug, notePath string, src []byte) (Note, error) {
	fm, body := splitFrontmatter(src)
	ctx := parser.NewContext()
	ctx.Set(linkContextKey, linkContext{slug: slug, noteDir: path.Dir(notePath)})
	// Lines removed with the frontmatter, so markers count from the file's
	// first line as ripgrep does.
	ctx.Set(lineOffsetKey, bytes.Count(src[:len(src)-len(body)], []byte{'\n'}))

	doc := r.md.Parser().Parse(text.NewReader(body), parser.WithContext(ctx))

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

	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, body, doc); err != nil {
		return Note{}, fmt.Errorf("render %s: %w", notePath, err)
	}

	return Note{
		Path:        notePath,
		Title:       title,
		Frontmatter: fm,
		HTML:        r.policy.Sanitize(buf.String()),
	}, nil
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
		if pos, ok := firstSegment(n); ok {
			line := lineOf(pos)
			if _, fenced := n.(*ast.FencedCodeBlock); fenced {
				line-- // the opening fence is the line before the content
			}
			n.SetAttributeString("data-line", []byte(strconv.Itoa(line)))
		}
		return ast.WalkContinue, nil
	})
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

var (
	// Heading IDs from goldmark, plus the footnote IDs it generates.
	idPattern = regexp.MustCompile(`^(fn|fnref):\d+$|^[\pL\pN_\-]+$`)
	// Only the classes goldmark and chroma emit, so a note cannot borrow
	// the app's own layout classes.
	// Chroma token classes are one to three letters, some with a digit
	// (c1, s1, s2). TestChromaClassesPassSanitiser ties this to the
	// generated stylesheet.
	codeClassPattern = regexp.MustCompile(`^(chroma|line|cl|hl|ln|lnt|lntd|lntable|language-[\w+#.\-]+|[a-z]{1,3}[0-9]?)( (chroma|line|cl|hl|ln|lnt|lntd|lntable|[a-z]{1,3}[0-9]?))*$`)
	noteClassPattern = regexp.MustCompile(`^(footnotes|footnote-ref|footnote-backref|outside-root)$`)
)

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
	p.AllowAttrs("data-line").Matching(regexp.MustCompile(`^[0-9]{1,9}$`)).OnElements(
		"p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "table", "pre", "hr", "div")
	p.AllowAttrs("class").Matching(codeClassPattern).OnElements("pre", "code", "span")
	p.AllowAttrs("class").Matching(noteClassPattern).OnElements("a", "img", "div", "section")
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
