package diagram

// The typed model a flowchart parses into. Nothing in it carries source
// text through to the drawing except labels, and a label is a list of
// plain-text lines: the writer escapes each one and places it in a <text>
// element it builds itself. Identifiers never reach the SVG at all.

// Direction is the flow of a diagram's ranks.
type Direction int

const (
	TopBottom Direction = iota // TB and TD
	BottomTop                  // BT
	LeftRight                  // LR
	RightLeft                  // RL
)

func (d Direction) String() string {
	return [...]string{"TB", "BT", "LR", "RL"}[d]
}

// horizontal reports whether ranks run across the page.
func (d Direction) horizontal() bool { return d == LeftRight || d == RightLeft }

// Shape is a node's outline.
type Shape int

const (
	ShapeRect             Shape = iota // A or A[text]
	ShapeRound                         // A(text)
	ShapeStadium                       // A([text])
	ShapeSubroutine                    // A[[text]]
	ShapeCylinder                      // A[(text)]
	ShapeCircle                        // A((text))
	ShapeDoubleCircle                  // A(((text)))
	ShapeAsymmetric                    // A>text]
	ShapeRhombus                       // A{text}
	ShapeHexagon                       // A{{text}}
	ShapeParallelogram                 // A[/text/]
	ShapeParallelogramAlt              // A[\text\]
	ShapeTrapezoid                     // A[/text\]
	ShapeTrapezoidAlt                  // A[\text/]
)

// Line is how an edge is stroked.
type Line int

const (
	LineSolid     Line = iota // --- -->
	LineDotted                // -.- -.->
	LineThick                 // === ==>
	LineInvisible             // ~~~
)

// Head is what an edge ends in.
type Head int

const (
	HeadNone   Head = iota
	HeadArrow       // >
	HeadCircle      // o
	HeadCross       // x
)

// Node is one vertex of the diagram.
type Node struct {
	ID       string
	Label    []string // lines of plain text
	Shape    Shape
	Subgraph int // index into Flowchart.Subgraphs, or -1 at the top level
}

// End is one end of an edge: a node, or — mermaid allows it — a whole
// subgraph. Exactly one of the two is non-negative.
type End struct {
	Node     int
	Subgraph int
}

// Edge is one link between two ends.
type Edge struct {
	From, To End
	Line     Line
	Head     Head // at To
	Tail     Head // at From, for <--> and o--o
	Length   int  // 1 for -->, 2 for --->, and so on
	Label    []string
}

// Subgraph is a titled box around a set of nodes.
type Subgraph struct {
	ID     string
	Title  []string
	Parent int // enclosing subgraph, or -1
}

// Flowchart is a parsed diagram.
type Flowchart struct {
	Direction Direction
	Nodes     []Node
	Edges     []Edge
	Subgraphs []Subgraph
}

// ancestor reports whether subgraph a encloses subgraph b, or is it.
func (f *Flowchart) ancestor(a, b int) bool {
	for ; b >= 0; b = f.Subgraphs[b].Parent {
		if a == b {
			return true
		}
	}
	return false
}
