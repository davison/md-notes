package diagram

import (
	"math"
	"sort"
)

// A drawing is the laid-out diagram in final coordinates: what the SVG
// writer draws, and all it can draw.

type point struct{ x, y float64 }

type dnode struct {
	shape      Shape
	x, y, w, h float64 // centre and size
	lines      []string
}

type dedge struct {
	edge       int // the model edge drawn
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
	tx         float64 // the title's centre across the box
	ref        int     // the subgraph
}

// titleBox is the rectangle a subgraph's title is drawn in.
func (c dcluster) titleBox() (x0, y0, x1, y1 float64) {
	w, h := textBox(c.title)
	y := c.y + clusterPad/2 + h/2 + 2
	return c.tx - w/2, y - h/2, c.tx + w/2, y + h/2
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
			tx:    (a.x + b.x) / 2,
			ref:   c,
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
	// Each chain's polyline, ends not yet clipped.
	type pending struct {
		edge  int
		ed    Edge
		pts   []point
		label int
		side  [2]*sideEnd // a back edge's From and To ends, where they use a side port
	}
	var ps []pending
	var sides []*sideEnd
	for ci, c := range e.chains {
		ed := e.f.Edges[c.edge]
		if ed.Line == LineInvisible {
			continue
		}
		p := pending{edge: c.edge, ed: ed, label: c.label}
		if c.reversed {
			// In TB space a back edge's chain runs from its head, in the
			// lower rank, along its lane to its tail.
			ns := c.nodes
			p.side[1] = e.sideEnd(ci, ns[0], ns[1], ed.To.Node >= 0)
			p.side[0] = e.sideEnd(ci, ns[len(ns)-1], ns[len(ns)-2], ed.From.Node >= 0)
			for _, se := range p.side {
				if se != nil {
					sides = append(sides, se)
				}
			}
		}
		ps = append(ps, p)
	}
	e.spreadSides(sides)
	for i := range ps {
		p := &ps[i]
		var c chain
		for _, ch := range e.chains {
			if ch.edge == p.edge {
				c = ch
				break
			}
		}
		var tb []point
		for k, v := range c.nodes {
			nd := e.nodes[v]
			pt := point{nd.x, nd.y}
			if c.reversed {
				// The side ends: aimed from a point inside the node, and
				// through a corner level with it where the way to the lane
				// is clear.
				var se *sideEnd
				switch k {
				case 0:
					se = p.side[1]
				case len(c.nodes) - 1:
					se = p.side[0]
				}
				if se != nil {
					se.anchor = tf(nd.x, nd.y+se.off)
					if se.corner {
						corner := point{e.nodes[se.lane].x, nd.y + se.off}
						if k == 0 {
							tb = append(tb, pt, corner)
						} else {
							tb = append(tb, corner, pt)
						}
						continue
					}
				}
			}
			tb = append(tb, pt)
		}
		p.pts = make([]point, len(tb))
		for k, q := range tb {
			p.pts[k] = tf(q.x, q.y)
		}
		if c.reversed {
			p.pts = reverse(p.pts)
		}
	}
	// Where more than one edge meets a node on the same side, their ends
	// are spread across that side in the order they arrive from, rather
	// than all aimed at the centre, so their heads stay apart.
	anchors := e.ports(d, len(ps), func(i int) (Edge, []point, [2]bool) {
		return ps[i].ed, ps[i].pts, [2]bool{ps[i].side[0] != nil, ps[i].side[1] != nil}
	})
	for i, p := range ps {
		for which, se := range p.side {
			if se != nil {
				anchors[i][which] = se.anchor
			}
		}
		ed, pts := p.ed, p.pts
		// Clip each end to the outline of what it touches: the node's
		// shape, or the subgraph's box.
		if ed.From.Subgraph >= 0 {
			x0, y0, x1, y1 := box(ed.From.Subgraph)
			pts = clipToBox(pts, x0, y0, x1, y1)
		} else {
			pts[0] = clipFrom(d.nodes[ed.From.Node], anchors[i][0], pts[1])
		}
		if ed.To.Subgraph >= 0 {
			rev := reverse(pts)
			x0, y0, x1, y1 := box(ed.To.Subgraph)
			rev = clipToBox(rev, x0, y0, x1, y1)
			pts = reverse(rev)
		} else {
			pts[len(pts)-1] = clipFrom(d.nodes[ed.To.Node], anchors[i][1], pts[len(pts)-2])
		}
		de := dedge{edge: p.edge, line: ed.Line, head: ed.Head, tail: ed.Tail, pts: pts}
		if p.label >= 0 {
			nd := e.nodes[p.label]
			lp := tf(nd.x, nd.y)
			w, h := textBox(ed.Label)
			de.label, de.lx, de.ly, de.lw, de.lh = ed.Label, lp.x, lp.y, w, h
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
			de := dedge{edge: i, line: ed.Line, head: ed.Head, tail: ed.Tail, loop: true, label: ed.Label}
			// Each further loop leaves and returns further apart as well as
			// reaching further out, so loops on one node nest rather than
			// cross.
			spread := func(size float64) float64 { return math.Min(size/2-2, size/6+5*float64(k)) }
			if e.dir.horizontal() {
				y := nd.y + nd.h/2
				a := spread(nd.w)
				de.pts = []point{{nd.x - a, y}, {nd.x - a - 6, y + reach}, {nd.x + a + 6, y + reach}, {nd.x + a, y}}
				if len(ed.Label) > 0 {
					w, h := textBox(ed.Label)
					de.lx, de.ly, de.lw, de.lh = nd.x, y+reach+4+h/2, w, h
				}
			} else {
				x := nd.x + nd.w/2
				a := spread(nd.h)
				de.pts = []point{{x, nd.y - a}, {x + reach, nd.y - a - 6}, {x + reach, nd.y + a + 6}, {x, nd.y + a}}
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

// titleGap is how far a subgraph's title keeps from any edge.
const titleGap = 3.0

// placeTitles puts each subgraph's title in its band, at the place nearest
// the centre that no edge crosses (davison/md-notes#174). Where there is
// no such place the title stays centred, and the subgraph is returned with
// the room its title needs, to be kept clear of its contents on the side
// of the box the edges leave freer: negative on the left, positive on the
// right.
func placeTitles(d *drawing) map[int]float64 {
	var curves [][]point
	for _, e := range d.edges {
		curves = append(curves, sample(e.pieces()))
	}
	blocked := map[int]float64{}
	for i := range d.clusters {
		c := &d.clusters[i]
		if len(c.title) == 0 {
			continue
		}
		tw, _ := textBox(c.title)
		_, ty0, _, ty1 := c.titleBox()
		ty0, ty1 = ty0-titleGap, ty1+titleGap
		type span struct{ a, b float64 }
		var busy []span
		for _, pts := range curves {
			for k := 0; k+1 < len(pts); k++ {
				p, q := pts[k], pts[k+1]
				if math.Max(p.y, q.y) < ty0 || math.Min(p.y, q.y) > ty1 {
					continue
				}
				// The part of the segment inside the band.
				xa, xb := p.x, q.x
				if p.y != q.y {
					at := func(y float64) float64 { return p.x + (q.x-p.x)*(y-p.y)/(q.y-p.y) }
					ya, yb := math.Max(math.Min(p.y, q.y), ty0), math.Min(math.Max(p.y, q.y), ty1)
					xa, xb = at(ya), at(yb)
				}
				busy = append(busy, span{math.Min(xa, xb) - titleGap, math.Max(xa, xb) + titleGap})
			}
		}
		mid := c.x + c.w/2
		lo, hi := c.x+clusterPad/2+tw/2, c.x+c.w-clusterPad/2-tw/2
		if lo > hi {
			lo, hi = mid, mid
		}
		free := func(x float64) bool {
			for _, s := range busy {
				if x-tw/2 < s.b && s.a < x+tw/2 {
					return false
				}
			}
			return true
		}
		cands := []float64{mid}
		for _, s := range busy {
			cands = append(cands, s.a-tw/2, s.b+tw/2)
		}
		best, found := mid, false
		for _, x := range cands {
			x = math.Min(math.Max(x, lo), hi)
			if free(x) && (!found || math.Abs(x-mid) < math.Abs(best-mid)) {
				best, found = x, true
			}
		}
		c.tx = best
		if !found {
			left, right := c.x+clusterPad/2, c.x+c.w-clusterPad/2
			first, last := right, left
			for _, s := range busy {
				first, last = math.Min(first, s.a), math.Max(last, s.b)
			}
			blocked[c.ref] = tw + 2*titleGap
			if first-left >= right-last {
				blocked[c.ref] = -blocked[c.ref]
			}
		}
	}
	return blocked
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

// inside reports whether p is inside a node's outline.
func inside(n dnode, p point) bool {
	dx, dy := math.Abs(p.x-n.x)/(n.w/2), math.Abs(p.y-n.y)/(n.h/2)
	switch n.shape {
	case ShapeRhombus:
		return dx+dy <= 1
	case ShapeCircle, ShapeDoubleCircle:
		return dx*dx+dy*dy <= 1
	}
	return dx <= 1 && dy <= 1
}

// clipFrom is where the line from a point inside a node towards p leaves
// the node's outline, found by bisection so that any outline will do. A p
// inside the node is returned as it is.
func clipFrom(n dnode, from, p point) point {
	if inside(n, p) || !inside(n, from) {
		return p
	}
	lo, hi := 0.0, 1.0
	for i := 0; i < 40; i++ {
		mid := (lo + hi) / 2
		if inside(n, point{from.x + (p.x-from.x)*mid, from.y + (p.y-from.y)*mid}) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return point{from.x + (p.x-from.x)*lo, from.y + (p.y-from.y)*lo}
}

// A sideEnd is one end of a back edge at a node: it leaves the node by the
// side that faces the edge's lane (TB-space right), rather than by the
// node's top or bottom, where it would crowd the main flow's ends.
type sideEnd struct {
	node   int     // lnode
	lane   int     // the lane's dummy next to the node
	up     bool    // the lane runs from here to lower ranks
	corner bool    // nothing stands between the node and the lane in its rank
	off    float64 // along the ranks from the node's centre
	anchor point   // in final space
}

// sideEnd describes the end of back edge chain ci at lnode v, whose
// neighbour in the chain is lane; nil where the end is a subgraph's.
func (e *engine) sideEnd(ci, v, lane int, atNode bool) *sideEnd {
	nd := e.nodes[v]
	if !atNode || nd.kind != kReal || e.nodes[lane].back == 0 {
		return nil
	}
	se := &sideEnd{node: v, lane: lane, up: e.nodes[lane].rank < nd.rank}
	_, or, _ := e.extents(e.finalW[nd.ref], e.finalH[nd.ref])
	lx := e.nodes[lane].x
	se.corner = lx > nd.x+or
	for _, u := range e.layers[nd.rank][nd.pos+1:] {
		if o := e.nodes[u]; o.x-o.ol < lx && o.kind != kDummy {
			se.corner = false
		}
	}
	return se
}

// spreadSides spreads the side ends that share a node along its side, in
// the order that keeps their lanes from crossing: those whose lanes run
// to lower ranks first, inner lane nearest the top, then the others,
// outer lane nearest the top.
func (e *engine) spreadSides(ends []*sideEnd) {
	by := map[int][]*sideEnd{}
	var nodes []int
	for _, se := range ends {
		if _, ok := by[se.node]; !ok {
			nodes = append(nodes, se.node)
		}
		by[se.node] = append(by[se.node], se)
	}
	for _, v := range nodes {
		g := by[v]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(a, b int) bool {
			if g[a].up != g[b].up {
				return g[a].up
			}
			la, lb := e.nodes[g[a].lane].x, e.nodes[g[b].lane].x
			if g[a].up {
				return la < lb
			}
			return la > lb
		})
		nd := e.nodes[v]
		_, _, rh := e.extents(e.finalW[nd.ref], e.finalH[nd.ref])
		step := math.Min(12, rh*0.6/float64(len(g)-1))
		for j, se := range g {
			se.off = (float64(j) - float64(len(g)-1)/2) * step
		}
	}
}

// ports picks, for each edge's two ends, the point inside its node that the
// end is aimed from. Ends meeting a node on the same side of it (before or
// after it along the ranks) are spread across that side, in the order of
// where they come from; a lone end is aimed from the centre.
func (e *engine) ports(d *drawing, n int, edge func(int) (Edge, []point, [2]bool)) [][2]point {
	out := make([][2]point, n)
	type end struct {
		edge, which int
		cross       float64 // the far point's position across the ranks
	}
	groups := map[[2]int][]end{}
	var keys [][2]int
	for i := 0; i < n; i++ {
		ed, pts, fixed := edge(i)
		for which, nodeEnd := range []End{ed.From, ed.To} {
			if nodeEnd.Node < 0 || fixed[which] {
				continue
			}
			nd := d.nodes[nodeEnd.Node]
			far := pts[1]
			if which == 1 {
				far = pts[len(pts)-2]
			}
			out[i][which] = point{nd.x, nd.y}
			along, cross := far.y-nd.y, far.x
			if e.dir.horizontal() {
				along, cross = far.x-nd.x, far.y
			}
			side := 0
			if along > 0 {
				side = 1
			}
			k := [2]int{nodeEnd.Node, side}
			if _, ok := groups[k]; !ok {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], end{i, which, cross})
		}
	}
	for _, k := range keys {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(a, b int) bool { return g[a].cross < g[b].cross })
		nd := d.nodes[k[0]]
		width := nd.w
		if e.dir.horizontal() {
			width = nd.h
		}
		step := math.Min(18, width*0.8/float64(len(g)-1))
		for j, en := range g {
			off := (float64(j) - float64(len(g)-1)/2) * step
			if e.dir.horizontal() {
				out[en.edge][en.which] = point{nd.x, nd.y + off}
			} else {
				out[en.edge][en.which] = point{nd.x + off, nd.y}
			}
		}
	}
	return out
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

// A piece of a drawn edge: a straight line from a to d, or a cubic Bézier
// from a to d through the control points b and c.
type piece struct {
	cubic      bool
	a, b, c, d point
}

// pieces is the curve an edge's control polyline is drawn as: a uniform
// cubic B-spline from its first point to its last — d3's curveBasis, which
// is how mermaid draws its edges — or, for a self-loop, its one Bézier.
func (e dedge) pieces() []piece {
	p := e.pts
	if e.loop && len(p) == 4 {
		return []piece{{true, p[0], p[1], p[2], p[3]}}
	}
	return basisPieces(p)
}

func basisPieces(p []point) []piece {
	if len(p) < 2 {
		return nil
	}
	if len(p) == 2 {
		return []piece{{a: p[0], d: p[1]}}
	}
	out := []piece{{a: p[0], d: point{(5*p[0].x + p[1].x) / 6, (5*p[0].y + p[1].y) / 6}}}
	bez := func(p0, p1, p2 point) {
		from := out[len(out)-1].d
		out = append(out, piece{true, from,
			point{(2*p0.x + p1.x) / 3, (2*p0.y + p1.y) / 3},
			point{(p0.x + 2*p1.x) / 3, (p0.y + 2*p1.y) / 3},
			point{(p0.x + 4*p1.x + p2.x) / 6, (p0.y + 4*p1.y + p2.y) / 6}})
	}
	for i := 2; i < len(p); i++ {
		bez(p[i-2], p[i-1], p[i])
	}
	n := len(p)
	bez(p[n-2], p[n-1], p[n-1])
	return append(out, piece{a: out[len(out)-1].d, d: p[n-1]})
}

// sample is the drawn curve as a polyline, close enough to measure
// clearances against.
func sample(ps []piece) []point {
	var out []point
	for i, pc := range ps {
		if i == 0 {
			out = append(out, pc.a)
		}
		if !pc.cubic {
			out = append(out, pc.d)
			continue
		}
		const steps = 16
		for k := 1; k <= steps; k++ {
			t := float64(k) / steps
			u := 1 - t
			out = append(out, point{
				u*u*u*pc.a.x + 3*u*u*t*pc.b.x + 3*u*t*t*pc.c.x + t*t*t*pc.d.x,
				u*u*u*pc.a.y + 3*u*u*t*pc.b.y + 3*u*t*t*pc.c.y + t*t*t*pc.d.y,
			})
		}
	}
	return out
}
