package render

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Fenced code ------------------------------------------------------------
//
// goldmark-highlighting draws every fenced block, but it asks chroma for the
// block's lexer with lexers.Get, once per block and without remembering the
// answer. For a tag that is neither a lexer's name nor one of its aliases,
// Get globs every lexer's filename patterns, each again with every backup
// suffix, twice: about 3 ms. That covers the tags chroma has no lexer for
// (`mermaid`, `log`, `none`) and some that it does have one for
// (`yml`, `h`, `patch`), and it made a note of 20,000 `mermaid` blocks take
// 78 s to render (davison/md-notes#178).
//
// So Render settles each block's lexer before the highlighter sees it:
//
//   - a tag that is a lexer's name or alias is found in the same four map
//     lookups Get starts with, and the block goes to the highlighter as it is;
//   - any other tag is looked up with Get once and the answer kept on the
//     Renderer. A block chroma has no lexer for is written here as plain
//     code, in the highlighter's own markup for one. A block it does have a
//     lexer for goes to the highlighter as a stand-in whose tag is a name
//     that finds the same lexer quickly;
//   - a note gets at most maxSlowTags distinct tags of the second kind. A
//     block tagged with any other renders as plain code. The limit counts
//     tags whether or not they are remembered, so what a note renders as
//     does not depend on what other notes have been read. It is what bounds
//     a note whose every block has a different tag, which no cache helps.

const (
	// maxSlowTags is how many distinct tags outside chroma's names and
	// aliases one note may have looked up: about 3 ms each, the first time.
	maxSlowTags = 16
	// maxCachedTags bounds the lookups a Renderer remembers. Past it, a new
	// tag is still looked up but not kept.
	maxCachedTags = 1024
)

// quickLexers is chroma's name and alias tables as its registry builds them
// (LexerRegistry.Register), which Get consults before anything slow.
var quickLexers = sync.OnceValue(func() (t struct{ byName, byAlias map[string]chroma.Lexer }) {
	t.byName, t.byAlias = map[string]chroma.Lexer{}, map[string]chroma.Lexer{}
	for _, l := range lexers.GlobalLexerRegistry.Lexers {
		c := l.Config()
		t.byName[c.Name] = l
		t.byName[strings.ToLower(c.Name)] = l
		for _, a := range c.Aliases {
			t.byAlias[a] = l
			t.byAlias[strings.ToLower(a)] = l
		}
	}
	return t
})

// quickLexer is the lexer lexers.Get would find for name through its name
// and alias tables, or nil if Get would have to search further.
func quickLexer(name string) chroma.Lexer {
	t := quickLexers()
	if l := t.byName[name]; l != nil {
		return l
	}
	if l := t.byAlias[name]; l != nil {
		return l
	}
	lower := strings.ToLower(name)
	if l := t.byName[lower]; l != nil {
		return l
	}
	return t.byAlias[lower]
}

// quickNameFor is a name quickLexer resolves to l, or "" if it has none.
//
// chroma registers Common Lisp, EmacsLisp and Go HTML Template twice, the
// plain lexer and then a wrapper of it, and the wrapper takes the names.
// Searching for `el` still finds the plain EmacsLisp, which no name
// reaches, so that one is given the name of the lexer registered under its
// name: `el` highlights as `elisp` does.
func quickNameFor(l chroma.Lexer) string {
	c := l.Config()
	names := append([]string{c.Name}, c.Aliases...)
	for _, n := range names {
		if quickLexer(n) == l {
			return n
		}
	}
	for _, n := range names {
		if q := quickLexer(n); q != nil && q.Config().Name == c.Name {
			return n
		}
	}
	return ""
}

// lexerCache remembers, for tags outside chroma's name and alias tables,
// the quick name of the lexer Get found, or "" for none.
type lexerCache struct {
	mu    sync.Mutex
	names map[string]string
	// lookups counts the calls to lexers.Get, for the tests.
	lookups atomic.Int64
}

func (c *lexerCache) quickName(tag string) string {
	c.mu.Lock()
	name, ok := c.names[tag]
	c.mu.Unlock()
	if ok {
		return name
	}
	c.lookups.Add(1)
	if l := lexers.Get(tag); l != nil {
		name = quickNameFor(l)
	}
	c.mu.Lock()
	if len(c.names) < maxCachedTags {
		c.names[tag] = name
	}
	c.mu.Unlock()
	return name
}

// resolveCode replaces every fenced block in doc with a codeBlock that
// records how it is to be drawn. It returns the source to render doc from:
// src itself, or src with the stand-ins' info strings after it.
func (c *lexerCache) resolveCode(doc ast.Node, src []byte) []byte {
	var fences []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if f, ok := n.(*ast.FencedCodeBlock); ok && entering {
			fences = append(fences, f)
		}
		return ast.WalkContinue, nil
	})
	out := slices.Clip(src)
	slow := map[string]bool{}
	for _, f := range fences {
		b := &codeBlock{fence: f}
		if lang := f.Language(src); lang != nil {
			tag := string(lang)
			switch {
			case quickLexer(tag) != nil:
				b.highlight = f
			case slow[tag] || len(slow) < maxSlowTags:
				slow[tag] = true
				if name := c.quickName(tag); name != "" {
					// The same info string with the tag swapped for the
					// quick name, so any attributes after it still apply.
					// Appending copies src the first time.
					start := len(out)
					out = append(out, name...)
					out = append(out, f.Info.Segment.Value(src)[len(lang):]...)
					stand := ast.NewFencedCodeBlock(ast.NewTextSegment(text.NewSegment(start, len(out))))
					stand.SetLines(f.Lines())
					b.highlight = stand
				}
			}
		}
		f.Parent().ReplaceChild(f.Parent(), f, b)
	}
	return out
}

// codeBlock is a fenced block whose lexer is settled. highlight is the node
// to hand the highlighter, or nil to write the block as plain code.
type codeBlock struct {
	ast.BaseBlock
	fence     *ast.FencedCodeBlock
	highlight *ast.FencedCodeBlock
}

var kindCodeBlock = ast.NewNodeKind("CodeBlockResolved")

func (n *codeBlock) Kind() ast.NodeKind { return kindCodeBlock }

func (n *codeBlock) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// codeBlockRenderer draws a codeBlock: through the highlighter's own
// function, or as plain code in the markup the highlighter uses for a
// block it has no lexer for.
type codeBlockRenderer struct {
	highlight renderer.NodeRendererFunc
}

func (r codeBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindCodeBlock, r.render)
}

func (r codeBlockRenderer) render(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	b := node.(*codeBlock)
	if b.highlight != nil {
		return r.highlight(w, source, b.highlight, entering)
	}
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("<pre><code")
	if lang := b.fence.Language(source); lang != nil {
		_, _ = w.WriteString(` class="language-`)
		html.DefaultWriter.Write(w, lang)
		_ = w.WriteByte('"')
	}
	_ = w.WriteByte('>')
	lines := b.fence.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		html.DefaultWriter.RawWrite(w, line.Value(source))
	}
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkContinue, nil
}

// funcOf captures the function a node renderer registers for kind.
func funcOf(nr renderer.NodeRenderer, kind ast.NodeKind) renderer.NodeRendererFunc {
	var cap funcCapture
	nr.RegisterFuncs(&cap)
	return cap.fns[kind]
}

type funcCapture struct {
	fns map[ast.NodeKind]renderer.NodeRendererFunc
}

func (c *funcCapture) Register(k ast.NodeKind, f renderer.NodeRendererFunc) {
	if c.fns == nil {
		c.fns = map[ast.NodeKind]renderer.NodeRendererFunc{}
	}
	c.fns[k] = f
}
