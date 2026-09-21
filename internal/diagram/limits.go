package diagram

import "time"

// Limits bound what one block may cost the daemon. A block over any of
// them is refused (Kind Limit) and shown as its code block.
type Limits struct {
	MaxBytes       int           // the block's source
	MaxNodes       int           // distinct nodes
	MaxEdges       int           // edges, counting each one an & expands to
	MaxSubgraphs   int           // subgraphs
	MaxDepth       int           // subgraphs nested in subgraphs
	MaxLength      int           // an edge's length (---> is 2)
	MaxLabel       int           // characters in one label
	MaxLines       int           // lines in one label
	MaxLayoutNodes int           // nodes, edge dummies and placeholders the layout works on
	Timeout        time.Duration // parse, layout and writing, together
}

// DefaultLimits are the bounds Render applies. The reasons for each number
// are on davison/md-notes#170, with the timings they were chosen from.
var DefaultLimits = Limits{
	MaxBytes:       32 << 10,
	MaxNodes:       200,
	MaxEdges:       400,
	MaxSubgraphs:   50,
	MaxDepth:       8,
	MaxLength:      8,
	MaxLabel:       500,
	MaxLines:       20,
	MaxLayoutNodes: 3000,
	Timeout:        2 * time.Second,
}
