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

// A self-loop and a back edge at the same node are drawn apart: neither
// runs along or into the other (review of PR #200, finding 2).
func TestLoopsApartFromBackEdges(t *testing.T) {
	const apart = 8.0
	for _, dir := range []string{"LR", "RL", "TB", "BT"} {
		t.Run(dir, func(t *testing.T) {
			f, d := layoutFixture(t, "loops-back-edges.mmd", dir)
			var loops, back []dedge
			for _, e := range d.edges {
				ed := f.Edges[e.edge]
				switch {
				case e.loop:
					loops = append(loops, e)
				case ed.From.Node >= 0 && ed.To.Node >= 0 &&
					nodeIndex(t, f, f.Nodes[ed.From.Node].ID) > nodeIndex(t, f, f.Nodes[ed.To.Node].ID):
					back = append(back, e)
				}
			}
			if len(loops) != 3 || len(back) != 2 {
				t.Fatalf("%d loops and %d back edges, want 3 and 2", len(loops), len(back))
			}
			for _, l := range loops {
				lp := sample(l.pieces())
				for _, b := range back {
					if dist, at := curveDistance(sample(b.pieces()), lp); dist < apart {
						n := f.Nodes[f.Edges[l.edge].From.Node].ID
						bed := f.Edges[b.edge]
						t.Errorf("back edge %s->%s comes %.1f px from %s's self-loop (at %.0f,%.0f)",
							f.Nodes[bed.From.Node].ID, f.Nodes[bed.To.Node].ID, dist, n, at.x, at.y)
					}
				}
			}
		})
	}
}

// curveDistance is the least distance between two polylines, 0 where they
// cross, and a point of the first where it is reached.
func curveDistance(a, b []point) (float64, point) {
	best, at := math.Inf(1), point{}
	for i := 0; i+1 < len(a); i++ {
		for j := 0; j+1 < len(b); j++ {
			if d := segmentDistance(a[i], a[i+1], b[j], b[j+1]); d < best {
				best, at = d, a[i]
			}
		}
	}
	return best, at
}

func segmentDistance(p, q, r, s point) float64 {
	cross := func(o, a, b point) float64 { return (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x) }
	d1, d2 := cross(r, s, p), cross(r, s, q)
	d3, d4 := cross(p, q, r), cross(p, q, s)
	if (d1 > 0) != (d2 > 0) && (d3 > 0) != (d4 > 0) && d1 != 0 && d2 != 0 && d3 != 0 && d4 != 0 {
		return 0
	}
	toSeg := func(x, a, b point) float64 {
		dx, dy := b.x-a.x, b.y-a.y
		l := dx*dx + dy*dy
		t := 0.0
		if l > 0 {
			t = math.Max(0, math.Min(1, ((x.x-a.x)*dx+(x.y-a.y)*dy)/l))
		}
		return math.Hypot(x.x-a.x-t*dx, x.y-a.y-t*dy)
	}
	return math.Min(math.Min(toSeg(p, r, s), toSeg(q, r, s)), math.Min(toSeg(r, p, q), toSeg(s, p, q)))
}

// Back edges whose spans interleave, and two parallel ones, each still
// run in one straight lane (review of PR #200, finding 5): every control
// point of a lane between its two ends sits on one line along the ranks.
func TestLanesStraight(t *testing.T) {
	for _, dir := range []string{"LR", "RL", "TB", "BT"} {
		t.Run(dir, func(t *testing.T) {
			f, d := layoutFixture(t, "interleaved-back-edges.mmd", dir)
			lanes := 0
			for _, e := range d.edges {
				ed := f.Edges[e.edge]
				if e.loop || nodeIndex(t, f, f.Nodes[ed.From.Node].ID) < nodeIndex(t, f, f.Nodes[ed.To.Node].ID) {
					continue
				}
				lanes++
				across := func(p point) float64 {
					if directions[dir].horizontal() {
						return p.y
					}
					return p.x
				}
				// The ends and the corners level with them are off the lane.
				mid := e.pts[2 : len(e.pts)-2]
				for _, p := range mid {
					if math.Abs(across(p)-across(mid[0])) > 0.5 {
						t.Errorf("back edge %s->%s leaves its lane: %.1f against %.1f",
							f.Nodes[ed.From.Node].ID, f.Nodes[ed.To.Node].ID, across(p), across(mid[0]))
						break
					}
				}
			}
			if lanes != 5 {
				t.Fatalf("%d back edges, want 5", lanes)
			}
		})
	}
}
