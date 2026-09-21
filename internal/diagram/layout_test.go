package diagram

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type rect struct{ x0, y0, x1, y1 float64 }

func (a rect) overlaps(b rect) bool {
	const eps = 0.5
	return a.x0 < b.x1-eps && b.x0 < a.x1-eps && a.y0 < b.y1-eps && b.y0 < a.y1-eps
}

func (a rect) contains(b rect) bool {
	const eps = 0.5
	return b.x0 >= a.x0-eps && b.y0 >= a.y0-eps && b.x1 <= a.x1+eps && b.y1 <= a.y1+eps
}

func (a rect) String() string {
	return fmt.Sprintf("[%.1f,%.1f %.1f,%.1f]", a.x0, a.y0, a.x1, a.y1)
}

// checkDrawing holds a laid-out diagram to what the layout promises for
// any input: nothing overlaps, every node sits inside its own subgraphs'
// boxes and outside every other, subgraph boxes nest or stay apart, edge
// labels keep off the nodes, and it all fits the image.
func checkDrawing(f *Flowchart, d *drawing) error {
	for _, v := range []float64{d.w, d.h} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return fmt.Errorf("image size %v x %v", d.w, d.h)
		}
	}
	image := rect{0, 0, d.w, d.h}
	nodes := make([]rect, len(d.nodes))
	for i, n := range d.nodes {
		nodes[i] = rect{n.x - n.w/2, n.y - n.h/2, n.x + n.w/2, n.y + n.h/2}
		if !image.contains(nodes[i]) {
			return fmt.Errorf("node %s %v outside the image %v", f.Nodes[i].ID, nodes[i], image)
		}
		for j := 0; j < i; j++ {
			if nodes[i].overlaps(nodes[j]) {
				return fmt.Errorf("nodes %s %v and %s %v overlap", f.Nodes[i].ID, nodes[i], f.Nodes[j].ID, nodes[j])
			}
		}
	}
	// d.clusters is in depth order; map back by matching the model.
	boxes := make([]rect, len(f.Subgraphs))
	order := make([]int, len(f.Subgraphs))
	for i := range order {
		order[i] = i
	}
	depth := func(c int) int {
		k := 0
		for p := f.Subgraphs[c].Parent; p >= 0; p = f.Subgraphs[p].Parent {
			k++
		}
		return k
	}
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && depth(order[j]) < depth(order[j-1]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	for k, c := range order {
		dc := d.clusters[k]
		boxes[c] = rect{dc.x, dc.y, dc.x + dc.w, dc.y + dc.h}
		if !image.contains(boxes[c]) {
			return fmt.Errorf("subgraph %d %v outside the image", c, boxes[c])
		}
	}
	for i, n := range f.Nodes {
		for c := range f.Subgraphs {
			inside := n.Subgraph >= 0 && f.ancestor(c, n.Subgraph)
			if inside && !boxes[c].contains(nodes[i]) {
				return fmt.Errorf("node %s %v is not inside its subgraph %d %v", n.ID, nodes[i], c, boxes[c])
			}
			if !inside && boxes[c].overlaps(nodes[i]) {
				return fmt.Errorf("node %s %v intrudes on subgraph %d %v", n.ID, nodes[i], c, boxes[c])
			}
		}
	}
	for a := range f.Subgraphs {
		for b := 0; b < a; b++ {
			switch {
			case f.ancestor(a, b):
				if !boxes[a].contains(boxes[b]) {
					return fmt.Errorf("subgraph %d %v does not contain its child %d %v", a, boxes[a], b, boxes[b])
				}
			case f.ancestor(b, a):
				if !boxes[b].contains(boxes[a]) {
					return fmt.Errorf("subgraph %d %v does not contain its child %d %v", b, boxes[b], a, boxes[a])
				}
			default:
				if boxes[a].overlaps(boxes[b]) {
					return fmt.Errorf("subgraphs %d %v and %d %v overlap", a, boxes[a], b, boxes[b])
				}
			}
		}
	}
	for _, e := range d.edges {
		if len(e.label) == 0 || e.loop {
			continue
		}
		lb := rect{e.lx - e.lw/2, e.ly - e.lh/2, e.lx + e.lw/2, e.ly + e.lh/2}
		for i := range nodes {
			if lb.overlaps(nodes[i]) {
				return fmt.Errorf("edge label %q %v overlaps node %s %v", e.label, lb, f.Nodes[i].ID, nodes[i])
			}
		}
		for _, p := range e.pts {
			if math.IsNaN(p.x) || math.IsNaN(p.y) {
				return fmt.Errorf("edge point is NaN")
			}
		}
	}
	return nil
}

func corpus(t testing.TB) map[string][]byte {
	files, err := filepath.Glob("testdata/corpus/*.mmd")
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(f)] = b
	}
	return out
}

func TestLayoutInvariantsCorpus(t *testing.T) {
	srcs := corpus(t)
	for _, n := range []int{10, 40, 100} {
		srcs[fmt.Sprintf("random-%d", n)] = randomSource(n, n*3/2, uint64(n))
	}
	for name, src := range srcs {
		for _, dir := range []string{"TB", "BT", "LR", "RL"} {
			t.Run(name+"/"+dir, func(t *testing.T) {
				f, err := Parse(src, DefaultLimits)
				if err != nil {
					t.Fatal(err)
				}
				f.Direction = directions[dir]
				d, err := layout(context.Background(), f, DefaultLimits)
				if err != nil {
					t.Fatal(err)
				}
				if err := checkDrawing(f, d); err != nil {
					t.Fatal(err)
				}
				if _, err := checkSVG(writeSVG(d, Light)); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestRenderDeterministic(t *testing.T) {
	for name, src := range corpus(t) {
		a, err := Render(context.Background(), src, Dark)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for i := 0; i < 5; i++ {
			b, _ := Render(context.Background(), src, Dark)
			if !bytes.Equal(a, b) {
				t.Fatalf("%s: two renders differ", name)
			}
		}
	}
}

// The corpus as drawn: every node's text is in the SVG, once per line,
// and every edge and subgraph is drawn.
func TestRenderCorpusContent(t *testing.T) {
	for name, src := range corpus(t) {
		f, err := Parse(src, DefaultLimits)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		svg, err := Render(context.Background(), src, Light)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		texts, err := checkSVG(svg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		have := map[string]int{}
		for _, s := range texts {
			have[s]++
		}
		for _, n := range f.Nodes {
			for _, l := range wrap(n.Label) {
				if have[l] == 0 {
					t.Errorf("%s: node %s's text %q is not drawn", name, n.ID, l)
				}
			}
		}
		paths := bytes.Count(svg, []byte("<path"))
		visible := 0
		for _, e := range f.Edges {
			if e.Line != LineInvisible {
				visible++
			}
		}
		if paths < visible {
			t.Errorf("%s: %d paths for %d visible edges", name, paths, visible)
		}
		if got := bytes.Count(svg, []byte(`rx="4" fill="`+Light.ClusterFill)); got != len(f.Subgraphs) {
			t.Errorf("%s: %d subgraph boxes, want %d", name, got, len(f.Subgraphs))
		}
	}
}
