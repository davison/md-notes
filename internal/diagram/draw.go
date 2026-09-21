package diagram

import "math"

// A drawing is the laid-out diagram in final coordinates: what the SVG
// writer draws, and all it can draw.

type point struct{ x, y float64 }

type dnode struct {
	shape      Shape
	x, y, w, h float64 // centre and size
	lines      []string
}

type dedge struct {
	line       Line
	head, tail Head
	pts        []point // the curve's control polyline, ends already clipped
	loop       bool    // a self-loop: pts are one cubic Bézier
	label      []string
	lx, ly     float64 // label centre
	lw, lh     float64
}

type dcluster struct {
	x, y, w, h float64
	title      []string
}

type drawing struct {
	w, h     float64
	nodes    []dnode
	edges    []dedge
	clusters []dcluster
}

func (e *engine) draw() *drawing {
	// TB-space bounds.
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	grow := func(x0, y0, x1, y1 float64) {
		minX, minY = math.Min(minX, x0), math.Min(minY, y0)
		maxX, maxY = math.Max(maxX, x1), math.Max(maxY, y1)
	}
	for _, nd := range e.nodes {
		grow(nd.x-nd.ol, nd.y-nd.rh/2, nd.x+nd.or, nd.y+nd.rh/2)
	}
	for _, cl := range e.clusters {
		grow(cl.x0, cl.y0, cl.x1, cl.y1)
	}
	if math.IsInf(minX, 1) {
		minX, minY, maxX, maxY = 0, 0, 0, 0
	}
	spanX, spanY := maxX-minX, maxY-minY
	tf := func(x, y float64) point {
		x, y = x-minX, y-minY
		switch e.dir {
		case BottomTop:
			return point{x + margin, spanY - y + margin}
		case LeftRight:
			return point{y + margin, x + margin}
		case RightLeft:
			return point{spanY - y + margin, x + margin}
		}
		return point{x + margin, y + margin}
	}
	d := &drawing{w: spanX + 2*margin, h: spanY + 2*margin}
	if e.dir.horizontal() {
		d.w, d.h = spanY+2*margin, spanX+2*margin
	}
	for c, cl := range e.clusters {
		a, b := tf(cl.x0, cl.y0), tf(cl.x1, cl.y1)
		d.clusters = append(d.clusters, dcluster{
			x: math.Min(a.x, b.x), y: math.Min(a.y, b.y),
			w: math.Abs(b.x - a.x), h: math.Abs(b.y - a.y),
			title: e.f.Subgraphs[c].Title,
		})
	}
	// Outer boxes first, so inner ones are drawn over them.
	sortByDepth(d.clusters, e.clusters)
	for i, n := range e.f.Nodes {
		nd := e.nodes[e.nodeOf[i]]
		p := tf(nd.x, nd.y)
		d.nodes = append(d.nodes, dnode{shape: n.Shape, x: p.x, y: p.y, w: e.finalW[i], h: e.finalH[i], lines: e.lines[i]})
	}
	box := func(c int) (x0, y0, x1, y1 float64) {
		cl := e.clusters[c]
		a, b := tf(cl.x0, cl.y0), tf(cl.x1, cl.y1)
		return math.Min(a.x, b.x), math.Min(a.y, b.y), math.Max(a.x, b.x), math.Max(a.y, b.y)
	}
	for _, c := range e.chains {
		ed := e.f.Edges[c.edge]
		if ed.Line == LineInvisible {
			continue
		}
		pts := make([]point, len(c.nodes))
		for k, v := range c.nodes {
			pts[k] = tf(e.nodes[v].x, e.nodes[v].y)
		}
		if c.reversed {
			for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
				pts[i], pts[j] = pts[j], pts[i]
			}
		}
		// Clip each end to the outline of what it touches: the node's
		// shape, or the subgraph's box.
		if ed.From.Subgraph >= 0 {
			x0, y0, x1, y1 := box(ed.From.Subgraph)
			pts = clipToBox(pts, x0, y0, x1, y1)
		} else {
			pts[0] = clipShape(d.nodes[ed.From.Node], pts[1])
		}
		if ed.To.Subgraph >= 0 {
			rev := reverse(pts)
			x0, y0, x1, y1 := box(ed.To.Subgraph)
			rev = clipToBox(rev, x0, y0, x1, y1)
			pts = reverse(rev)
		} else {
			pts[len(pts)-1] = clipShape(d.nodes[ed.To.Node], pts[len(pts)-2])
		}
		de := dedge{line: ed.Line, head: ed.Head, tail: ed.Tail, pts: pts}
		if c.label >= 0 {
			nd := e.nodes[c.label]
			p := tf(nd.x, nd.y)
			w, h := textBox(ed.Label)
			de.label, de.lx, de.ly, de.lw, de.lh = ed.Label, p.x, p.y, w, h
		}
		d.edges = append(d.edges, de)
	}
	for n, loops := range e.loops {
		nd := d.nodes[n]
		for k, i := range loops {
			ed := e.f.Edges[i]
			if ed.Line == LineInvisible {
				continue
			}
			reach := loopReach + loopStep*float64(k)
			de := dedge{line: ed.Line, head: ed.Head, tail: ed.Tail, loop: true, label: ed.Label}
			if e.dir.horizontal() {
				y := nd.y + nd.h/2
				de.pts = []point{{nd.x - nd.w/4, y}, {nd.x - nd.w/4 - 6, y + reach}, {nd.x + nd.w/4 + 6, y + reach}, {nd.x + nd.w/4, y}}
				if len(ed.Label) > 0 {
					w, h := textBox(ed.Label)
					de.lx, de.ly, de.lw, de.lh = nd.x, y+reach+4+h/2, w, h
				}
			} else {
				x := nd.x + nd.w/2
				de.pts = []point{{x, nd.y - nd.h/4}, {x + reach, nd.y - nd.h/4 - 6}, {x + reach, nd.y + nd.h/4 + 6}, {x, nd.y + nd.h/4}}
				if len(ed.Label) > 0 {
					w, h := textBox(ed.Label)
					de.lx, de.ly, de.lw, de.lh = x+reach+4+w/2, nd.y, w, h
				}
			}
			if len(ed.Label) == 0 {
				de.label = nil
			}
			d.edges = append(d.edges, de)
		}
	}
	return d
}

func sortByDepth(out []dcluster, cs []*cluster) {
	idx := make([]int, len(cs))
	for i := range idx {
		idx[i] = i
	}
	// Insertion sort: there are at most MaxSubgraphs of them.
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && cs[idx[j]].depth < cs[idx[j-1]].depth; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	tmp := append([]dcluster(nil), out...)
	for k, i := range idx {
		out[k] = tmp[i]
	}
}

func reverse(p []point) []point {
	out := make([]point, len(p))
	for i := range p {
		out[len(p)-1-i] = p[i]
	}
	return out
}

// clipShape is where the line from a node's centre towards p leaves the
// node's outline.
func clipShape(n dnode, p point) point {
	dx, dy := p.x-n.x, p.y-n.y
	if dx == 0 && dy == 0 {
		return p
	}
	hw, hh := n.w/2, n.h/2
	var t float64
	switch n.shape {
	case ShapeRhombus:
		t = 1 / (math.Abs(dx)/hw + math.Abs(dy)/hh)
	case ShapeCircle, ShapeDoubleCircle:
		t = 1 / math.Sqrt(dx*dx/(hw*hw)+dy*dy/(hh*hh))
	default:
		t = math.Inf(1)
		if dx != 0 {
			t = hw / math.Abs(dx)
		}
		if dy != 0 {
			t = math.Min(t, hh/math.Abs(dy))
		}
	}
	if t >= 1 {
		return p
	}
	return point{n.x + dx*t, n.y + dy*t}
}

// clipToBox cuts a polyline that starts inside a box at the point where it
// last leaves the box.
func clipToBox(pts []point, x0, y0, x1, y1 float64) []point {
	inside := func(p point) bool { return p.x >= x0 && p.x <= x1 && p.y >= y0 && p.y <= y1 }
	last := -1
	for i := 0; i+1 < len(pts); i++ {
		if inside(pts[i]) && !inside(pts[i+1]) {
			last = i
		}
	}
	if last < 0 {
		return pts
	}
	a, b := pts[last], pts[last+1]
	t := 1.0
	edge := func(p0, p1, bound float64) {
		if p1 != p0 {
			if s := (bound - p0) / (p1 - p0); s >= 0 && s <= 1 && s < t {
				t = s
			}
		}
	}
	edge(a.x, b.x, x0)
	edge(a.x, b.x, x1)
	edge(a.y, b.y, y0)
	edge(a.y, b.y, y1)
	cut := point{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t}
	return append([]point{cut}, pts[last+1:]...)
}
