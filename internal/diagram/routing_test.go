package diagram

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
)

// Routing quality (davison/md-notes#188): measured on the drawn curves, not
// on golden bytes, so that a change in how an edge is drawn is judged by
// what a reader sees.

// backClear is how far a back edge keeps from every box it does not
// connect to.
const backClear = 16.0

// titleClear is how far every edge keeps from a subgraph title's text.
const titleClear = 2.0

func layoutFixture(t *testing.T, name, dir string) (*Flowchart, *drawing) {
	t.Helper()
	src, err := os.ReadFile("testdata/corpus/" + name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(src, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if dir != "" {
		f.Direction = directions[dir]
	}
	d, err := layout(context.Background(), f, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkDrawing(f, d); err != nil {
		t.Fatal(err)
	}
	return f, d
}

func nodeIndex(t *testing.T, f *Flowchart, id string) int {
	t.Helper()
	for i, n := range f.Nodes {
		if n.ID == id {
			return i
		}
	}
	t.Fatalf("no node %s", id)
	return -1
}

func nodeRect(n dnode) rect { return rect{n.x - n.w/2, n.y - n.h/2, n.x + n.w/2, n.y + n.h/2} }

// distance is how far p is from r: 0 inside it.
func (a rect) distance(p point) float64 {
	dx := math.Max(0, math.Max(a.x0-p.x, p.x-a.x1))
	dy := math.Max(0, math.Max(a.y0-p.y, p.y-a.y1))
	return math.Hypot(dx, dy)
}

// clearance is the nearest the drawn edge comes to r.
func clearance(e dedge, r rect) (float64, point) {
	best, at := math.Inf(1), point{}
	for _, p := range sample(e.pieces()) {
		if d := r.distance(p); d < best {
			best, at = d, p
		}
	}
	return best, at
}

// The operator's shape (#183, #184): a chain A to H, a side input R into
// C, and two back edges, G to D and H to B. business-plan.mmd is it with
// long wrapped labels, chain-back-edges.mmd with short ones and labelled
// back edges.
var chainFixtures = []string{"business-plan.mmd", "chain-back-edges.mmd"}

// Every back edge keeps clear of every box it passes, in every direction.
func TestBackEdgesClearOfNodes(t *testing.T) {
	for _, name := range chainFixtures {
		for _, dir := range []string{"LR", "RL", "TB", "BT"} {
			t.Run(name+"/"+dir, func(t *testing.T) {
				f, d := layoutFixture(t, name, dir)
				back := map[[2]int]bool{
					{nodeIndex(t, f, "G"), nodeIndex(t, f, "D")}: true,
					{nodeIndex(t, f, "H"), nodeIndex(t, f, "B")}: true,
				}
				found := 0
				for _, e := range d.edges {
					ed := f.Edges[e.edge]
					if !back[[2]int{ed.From.Node, ed.To.Node}] {
						continue
					}
					found++
					for i, n := range d.nodes {
						if i == ed.From.Node || i == ed.To.Node {
							continue
						}
						if c, at := clearance(e, nodeRect(n)); c < backClear {
							t.Errorf("back edge %s->%s comes %.1f px from %s (at %.0f,%.0f); want at least %v",
								f.Nodes[ed.From.Node].ID, f.Nodes[ed.To.Node].ID, c, f.Nodes[i].ID, at.x, at.y, backClear)
						}
					}
				}
				if found != 2 {
					t.Fatalf("found %d back edges, want 2", found)
				}
			})
		}
	}
}

// The chain A to H is drawn straight: every node of it on one centre line
// across the ranks, however the back edges and the side input pull.
func TestMainChainStraight(t *testing.T) {
	for _, name := range chainFixtures {
		for _, dir := range []string{"LR", "RL", "TB", "BT"} {
			t.Run(name+"/"+dir, func(t *testing.T) {
				f, d := layoutFixture(t, name, dir)
				across := func(n dnode) float64 {
					if directions[dir].horizontal() {
						return n.y
					}
					return n.x
				}
				line := across(d.nodes[nodeIndex(t, f, "A")])
				for _, id := range []string{"B", "C", "D", "E", "F", "G", "H"} {
					if got := across(d.nodes[nodeIndex(t, f, id)]); math.Abs(got-line) > 0.5 {
						t.Errorf("%s is centred at %.1f across the ranks, A at %.1f", id, got, line)
					}
				}
			})
		}
	}
}

// No edge is drawn through a subgraph's title (#174), in any fixture of
// the corpus and in any direction.
func TestEdgesClearOfTitles(t *testing.T) {
	for name := range corpus(t) {
		for _, dir := range []string{"TB", "BT", "LR", "RL"} {
			t.Run(name+"/"+dir, func(t *testing.T) {
				_, d := layoutFixture(t, name, dir)
				if err := titlesClear(d); err != nil {
					t.Error(err)
				}
			})
		}
	}
}

func titlesClear(d *drawing) error {
	for ci, c := range d.clusters {
		if len(c.title) == 0 {
			continue
		}
		x0, y0, x1, y1 := c.titleBox()
		tb := rect{x0, y0, x1, y1}
		if !(rect{c.x, c.y, c.x + c.w, c.y + c.h}).contains(tb) {
			return fmt.Errorf("subgraph %d's title %v is outside its box", ci, tb)
		}
		for _, e := range d.edges {
			if cl, at := clearance(e, tb); cl < titleClear {
				return fmt.Errorf("edge %d comes %.1f px from subgraph %d's title %q %v (at %.0f,%.0f)",
					e.edge, cl, ci, c.title, tb, at.x, at.y)
			}
		}
	}
	return nil
}
