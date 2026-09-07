// Package render turns a markdown note into sanitised HTML with its
// frontmatter separated, links rewritten for the app, and code highlighted.
package render

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"regexp"
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
			parser.WithASTTransformers(util.Prioritized(&linkRewriter{}, 100)),
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

	doc := r.md.Parser().Parse(text.NewReader(body), parser.WithContext(ctx))
	var buf bytes.Buffer
	if err := r.md.Renderer().Render(&buf, body, doc); err != nil {
		return Note{}, fmt.Errorf("render %s: %w", notePath, err)
	}

	title := ""
	if t, ok := fm["title"].(string); ok && strings.TrimSpace(t) != "" {
		title = strings.TrimSpace(t)
	} else if h := firstH1(doc, body); h != "" {
		title = h
	} else {
		title = strings.TrimSuffix(path.Base(notePath), path.Ext(notePath))
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
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil || fm == nil {
		return nil, src
	}
	if after > len(rest) {
		after = len(rest)
	}
	return fm, rest[after:]
}

// firstH1 returns the plain text of the first level-one heading.
func firstH1(doc ast.Node, src []byte) string {
	var out string
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			out = strings.TrimSpace(plainText(h, src))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return out
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
			l.Destination = []byte(dest)
			if raw {
				l.SetAttributeString("target", []byte("_blank"))
			}
		case *ast.Image:
			dest, _ := rewrite(lc, string(l.Destination), false)
			l.Destination = []byte(dest)
		}
		return ast.WalkContinue, nil
	})
}

// rewrite maps a link destination written in a note to the URL the app
// serves it at. Markdown targets become in-app routes; other relative
// files are served raw. The second result reports a raw-file link, which
// opens in a new tab so the client-side router leaves it alone. Absolute
// URLs, fragments, and targets that escape the root are returned as they
// are.
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
	if resolved == ".." || strings.HasPrefix(resolved, "../") || resolved == "." {
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

// Sanitisation -----------------------------------------------------------

var (
	idPattern    = regexp.MustCompile(`^[\pL\pN_:.\-]+$`)
	classPattern = regexp.MustCompile(`^[a-zA-Z0-9 _\-]+$`)
)

// newPolicy is bluemonday's user-generated-content policy extended with
// what goldmark's task lists, heading IDs, footnotes, and chroma emit.
func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("id").Matching(idPattern).OnElements("h1", "h2", "h3", "h4", "h5", "h6", "sup", "li", "div", "section")
	p.AllowAttrs("class").Matching(classPattern).OnElements("pre", "code", "span", "a", "div", "sup", "ol", "ul", "li", "input", "section")
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
