package diagram

import "fmt"

// Kind says why a block was not drawn, so the caller can decide what to
// say and whether trying again could help.
type Kind int

const (
	// Unsupported: valid mermaid, perhaps, but outside the subset this
	// package draws — another diagram type, a directive, an HTML label.
	Unsupported Kind = iota
	// Syntax: the text does not parse as a flowchart.
	Syntax
	// Limit: a bound was hit — the input's size, its node, edge or
	// subgraph count, or the time the layout took.
	Limit
)

func (k Kind) String() string {
	return [...]string{"unsupported", "syntax", "limit"}[k]
}

// Refusal is the error Render returns for a block it will not draw. The
// block is then shown as the code block it was; nothing of it is drawn.
type Refusal struct {
	Kind   Kind
	Line   int // 1-based line of the block, or 0 when it is not about one line
	Reason string
}

func (r *Refusal) Error() string {
	if r.Line > 0 {
		return fmt.Sprintf("flowchart %s: line %d: %s", r.Kind, r.Line, r.Reason)
	}
	return fmt.Sprintf("flowchart %s: %s", r.Kind, r.Reason)
}

func refuse(k Kind, line int, format string, a ...any) *Refusal {
	return &Refusal{Kind: k, Line: line, Reason: fmt.Sprintf(format, a...)}
}
