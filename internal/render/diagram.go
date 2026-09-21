package render

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/davison/md-notes/internal/diagram"
)

// Diagram is a fenced ```mermaid block that the diagram package will draw
// (davison/md-notes#171).
//
// It travels beside the note's HTML rather than in it. The HTML keeps the
// code block exactly as it was, which is the fallback for everything that
// can go wrong later, and the reading view puts the image in front of it,
// finding it by the line anchor the renderer emits before every fenced
// block. That anchor is the renderer's alone — scrubRaw takes the class
// and the attribute off anything the note wrote — and this list is built
// from the markdown's fenced blocks, never from HTML, so nothing a note
// writes can put an image anywhere or name a block of its own choosing.
// The sanitiser admits nothing new for it.
type Diagram struct {
	// Line is the data-line of the anchor directly before the block's <pre>.
	Line int `json:"line"`
	// Hash names the block's source: the first 128 bits of its SHA-256, in
	// hex. The diagram route looks the block up by it in the note it
	// re-reads, so the hash is a key and never a claim.
	Hash string `json:"hash"`
	// Width and Height are the drawing's natural size, which the reading
	// view reserves before the image loads. The renderer does not know
	// them — they come from the layout — so the caller that draws fills
	// them in; zero means not measured.
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	// Source is the block's content, the text its code block shows.
	Source []byte `json:"-"`
}

// HashSource is how a Diagram's Hash is made from its source.
func HashSource(src []byte) string {
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:16])
}

var diagramsKey = parser.NewContextKey()

func diagramsOf(pc parser.Context) []Diagram {
	d, _ := pc.Get(diagramsKey).([]Diagram)
	return d
}

// diagramFinder runs after lineMarker, so every fenced block already has
// its anchor in front of it, and it takes the line from that anchor rather
// than working it out again: the list and the markup cannot disagree.
//
// A block is listed only when diagram.Parse accepts it. Parsing is linear
// and bounded by the input limit, so a refusal that can be known now —
// another diagram type, a directive, a syntax error, a count over a bound
// — is known now, and no image is ever asked for; a refusal only the
// layout can find (its size, its deadline) is left to the route.
type diagramFinder struct{}

func (diagramFinder) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var found []Diagram
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		anchor, ok := n.(*lineAnchor)
		if !ok {
			return ast.WalkContinue, nil
		}
		block, ok := anchor.NextSibling().(*ast.FencedCodeBlock)
		if !ok || string(block.Language(src)) != "mermaid" {
			return ast.WalkContinue, nil
		}
		var body []byte
		lines := block.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			body = append(body, seg.Value(src)...)
		}
		if _, err := diagram.Parse(body, diagram.DefaultLimits); err != nil {
			return ast.WalkContinue, nil
		}
		found = append(found, Diagram{Line: anchor.line, Hash: HashSource(body), Source: body})
		return ast.WalkContinue, nil
	})
	pc.Set(diagramsKey, found)
}
