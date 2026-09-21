package diagram

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Seeds: the corpus, every advisory payload, and a handful of fragments
// that sit on the parser's edges.
func seed(f *testing.F) {
	for _, src := range corpus(f) {
		f.Add(src)
	}
	for _, a := range advisories() {
		f.Add([]byte(a.src))
	}
	for _, s := range []string{
		"graph", "graph LR;A", "graph\nA-->A", "graph\nA<-->B", "graph\nA --o B --x C",
		"graph\nsubgraph s\nend\ns-->s2\nsubgraph s2\nend",
		"graph\nA -- -- --> B", "graph\nA -. .- B", "graph\nA ==  == B",
		"graph\nA[\"#35;#quot;#60;\"]", "graph\nA;;;B;;", "graph\nA -->|x|B-->|y|C",
		"graph BT\nsubgraph a\nsubgraph b\nx-->y\nend\nend\ny-->a",
	} {
		f.Add([]byte(s))
	}
}

// FuzzParse: the parser never panics, and what it accepts is a model that
// holds together — every index in range, every label within its bounds.
func FuzzParse(f *testing.F) {
	seed(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		fc, err := Parse(src, DefaultLimits)
		if err != nil {
			var r *Refusal
			if !errors.As(err, &r) {
				t.Fatalf("error is not a refusal: %v", err)
			}
			return
		}
		lim := DefaultLimits
		if len(fc.Nodes) > lim.MaxNodes || len(fc.Edges) > lim.MaxEdges || len(fc.Subgraphs) > lim.MaxSubgraphs {
			t.Fatalf("over a count: %d nodes, %d edges, %d subgraphs", len(fc.Nodes), len(fc.Edges), len(fc.Subgraphs))
		}
		for _, n := range fc.Nodes {
			if n.Subgraph < -1 || n.Subgraph >= len(fc.Subgraphs) || len(n.Label) == 0 || len(n.Label) > lim.MaxLines {
				t.Fatalf("bad node %+v", n)
			}
		}
		for i, s := range fc.Subgraphs {
			if s.Parent < -1 || s.Parent >= i {
				t.Fatalf("bad subgraph %+v", s)
			}
		}
		for _, e := range fc.Edges {
			for _, end := range []End{e.From, e.To} {
				if (end.Node < 0) == (end.Subgraph < 0) || end.Node >= len(fc.Nodes) || end.Subgraph >= len(fc.Subgraphs) {
					t.Fatalf("bad end %+v", end)
				}
			}
			if e.Length < 1 || e.Length > lim.MaxLength {
				t.Fatalf("bad length %d", e.Length)
			}
		}
	})
}

// FuzzRender: the whole pipeline, with no recover in the way — the layout
// is called directly — never panics; a block it draws holds the layout's
// invariants, and its SVG passes the allowlist.
func FuzzRender(f *testing.F) {
	seed(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		lim := DefaultLimits
		fc, err := Parse(src, lim)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		d, err := layout(ctx, fc, lim)
		if err != nil {
			var r *Refusal
			if !errors.As(err, &r) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("layout error is neither a refusal nor the deadline: %v", err)
			}
			return
		}
		if err := checkDrawing(fc, d); err != nil {
			t.Fatalf("%v\nsource: %q", err, src)
		}
		for _, th := range []Theme{Light, Dark, EInk} {
			if _, err := checkSVG(writeSVG(d, th)); err != nil {
				t.Fatalf("%v\nsource: %q", err, src)
			}
		}
	})
}
