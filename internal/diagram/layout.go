package diagram

import (
	"context"
	"fmt"
	"math"
	"sort"
)

// The layout is a layered (Sugiyama) drawing, done in four passes over a
// working graph in which every edge spans at least two ranks:
//
//  1. ranks: cycles are broken by reversing DFS back edges, ranks are
//     assigned by longest path and then pulled tight;
//  2. every edge is split into a chain through one dummy node per rank it
//     crosses, one of which carries its label, and each subgraph gets an
//     invisible placeholder in any rank it spans but has nothing in;
//  3. order: barycentre sweeps, with each subgraph's contents kept
//     contiguous in every rank and sibling subgraphs kept in one order
//     across all ranks, so that a subgraph's box can be a rectangle that
//     nothing else enters;
//  4. position: every node's and every subgraph border's coordinate across
//     the ranks is a variable in a system of separation constraints, solved
//     for a packed start and then relaxed towards straight edges without
//     ever leaving the feasible region — so no two boxes can overlap,
//     whatever the input.
//
// All of it runs in "TB space", ranks down the page, and is turned into
// the diagram's own direction at the end.

const (
	nodeSep     = 24.0 // between two nodes in a rank
	dummySep    = 12.0 // between an edge's dummy and anything else
	clusterSep  = 16.0 // between a subgraph's border and its neighbour
	clusterPad  = 12.0 // between a subgraph's border and its contents
	rankGap     = 26.0 // half the gap between the ranks of two linked nodes
	margin      = 8.0
	loopReach   = 22.0 // how far a self-loop stands off its node
	loopStep    = 12.0 // and each further one
	relaxRounds = 40
	orderRounds = 24
)

type lkind int

const (
	kReal lkind = iota
	kDummy
	kLabel
	kPlaceholder
	kAnchor // stands in for an empty subgraph
)

type lnode struct {
	kind    lkind
	ref     int     // model node for kReal, edge index for kDummy/kLabel, cluster otherwise
	ol, or  float64 // extent left and right of centre across the rank
	rh      float64 // extent along the rank axis
	rank    int
	pos     int
	x, y    float64
	cluster int // innermost cluster, or -1
	up      []int
	down    []int
	upW     []float64
	downW   []float64
}

type chain struct {
	edge     int   // model edge
	nodes    []int // lnodes from tail to head in layout direction
	reversed bool
	label    int // lnode carrying the label, or -1
}

type cluster struct {
	parent   int
	depth    int
	children []int
	first    int // first rank spanned
	last     int
	titleW   float64
	titleH   float64
	lVar     int
	rVar     int
	// Box in TB space.
	x0, x1, y0, y1 float64
}

type engine struct {
	ctx      context.Context
	f        *Flowchart
	lim      Limits
	dir      Direction
	nodes    []*lnode
	chains   []chain
	loops    [][]int // self-loop edges by model node
	clusters []*cluster
	layers   [][]int
	path     [][]int // cluster path per lnode, outermost first
	nodeOf   []int   // lnode of each model node
	anchorOf []int   // anchor lnode of each cluster, or -1
	finalW   []float64
	finalH   []float64
	lines    [][]string // wrapped label per model node
}

// check reports the context's error, if it has one, naming the phase of
// the layout it stopped: rank, order or position. Every round of each of
// those phases calls it, and TestLayoutChecksContext holds each phase to
// that by where the layout stops.
func (e *engine) check(phase string) error {
	if err := e.ctx.Err(); err != nil {
		return fmt.Errorf("layout stopped in %s: %w", phase, err)
	}
	return nil
}

func (e *engine) add(n *lnode) int {
	e.nodes = append(e.nodes, n)
	return len(e.nodes) - 1
}

// layout lays the flowchart out and returns the drawing in final
// coordinates.
func layout(ctx context.Context, f *Flowchart, lim Limits) (*drawing, error) {
	e := &engine{ctx: ctx, f: f, lim: lim, dir: f.Direction}
	e.buildNodes()
	if err := e.buildEdges(); err != nil {
		return nil, err
	}
	if err := e.rank(); err != nil {
		return nil, err
	}
	if err := e.normalise(); err != nil {
		return nil, err
	}
	if err := e.order(); err != nil {
		return nil, err
	}
	if err := e.positionX(); err != nil {
		return nil, err
	}
	e.positionY()
	return e.draw(), nil
}

// extents turns a final-space width and height into the TB-space extent
// across the rank (split left and right) and along it.
func (e *engine) extents(w, h float64) (ol, or, rh float64) {
	if e.dir.horizontal() {
		return h / 2, h / 2, w
	}
	return w / 2, w / 2, h
}

func (e *engine) buildNodes() {
	f := e.f
	for i, s := range f.Subgraphs {
		c := &cluster{parent: s.Parent}
		if s.Parent >= 0 {
			c.depth = e.clusters[s.Parent].depth + 1
			e.clusters[s.Parent].children = append(e.clusters[s.Parent].children, i)
		}
		title := wrap(s.Title)
		if len(title) == 1 && title[0] == "" {
			title = nil
		}
		f.Subgraphs[i].Title = title
		c.titleW, c.titleH = textBox(title)
		if len(title) > 0 {
			c.titleH += 6
		}
		e.clusters = append(e.clusters, c)
	}
	e.loops = make([][]int, len(f.Nodes))
	for i, ed := range f.Edges {
		if ed.From.Node >= 0 && ed.From.Node == ed.To.Node {
			e.loops[ed.From.Node] = append(e.loops[ed.From.Node], i)
		}
	}
	e.lines = make([][]string, len(f.Nodes))
	e.finalW = make([]float64, len(f.Nodes))
	e.finalH = make([]float64, len(f.Nodes))
	e.nodeOf = make([]int, len(f.Nodes))
	for i, n := range f.Nodes {
		e.lines[i] = wrap(n.Label)
		tw, th := textBox(e.lines[i])
		w, h := nodeSize(n.Shape, tw, th)
		e.finalW[i], e.finalH[i] = w, h
		ol, or, rh := e.extents(w, h)
		or += e.loopExtent(i)
		rh = math.Max(rh, e.loopLabelSpan(i))
		e.nodeOf[i] = e.add(&lnode{kind: kReal, ref: i, ol: ol, or: or, rh: rh, cluster: n.Subgraph})
	}
	// An empty subgraph still draws a box: an invisible anchor gives it
	// somewhere to be and something for an edge to it to aim at.
	e.anchorOf = make([]int, len(e.clusters))
	has := make([]bool, len(e.clusters))
	for _, n := range f.Nodes {
		for c := n.Subgraph; c >= 0; c = e.clusters[c].parent {
			has[c] = true
		}
	}
	for c := range e.clusters {
		e.anchorOf[c] = -1
		if !has[c] {
			empty := true
			for _, k := range e.clusters[c].children {
				if has[k] {
					empty = false
				}
			}
			if empty {
				ol, or, rh := e.extents(60, 24)
				e.anchorOf[c] = e.add(&lnode{kind: kAnchor, ref: c, ol: ol, or: or, rh: rh, cluster: c})
				for p := c; p >= 0; p = e.clusters[p].parent {
					has[p] = true
				}
			}
		}
	}
}

// loopExtent is the room a node's self-loops and their labels need beside
// it, on the TB-space right: the right of the node across the page, or
// below it when ranks run across.
func (e *engine) loopExtent(n int) float64 {
	if len(e.loops[n]) == 0 {
		return 0
	}
	ext := loopReach + loopStep*float64(len(e.loops[n])-1)
	lab := 0.0
	for _, i := range e.loops[n] {
		w, h := textBox(e.f.Edges[i].Label)
		if e.dir.horizontal() {
			w = h
		}
		lab = math.Max(lab, w)
	}
	if lab > 0 {
		ext += lab + 8
	}
	return ext
}

// loopLabelSpan is how much of the rank axis a node's self-loop labels
// need: a loop's label is centred on the node along that axis, beside the
// loop (below it when ranks run across), and may be longer than the node.
func (e *engine) loopLabelSpan(n int) float64 {
	span := 0.0
	for _, i := range e.loops[n] {
		w, h := textBox(e.f.Edges[i].Label)
		if e.dir.horizontal() {
			span = math.Max(span, w)
		} else {
			span = math.Max(span, h)
		}
	}
	return span
}

func nodeSize(s Shape, tw, th float64) (w, h float64) {
	const px, py = 16.0, 10.0
	switch s {
	case ShapeSubroutine:
		return tw + 2*px + 16, th + 2*py
	case ShapeStadium:
		h = th + 2*py
		return tw + h, h
	case ShapeCylinder:
		w = tw + 2*px
		ry := math.Min(10, math.Max(4, w/12))
		return w, th + 2*py + 2*ry
	case ShapeCircle:
		d := math.Hypot(tw, th) + 16
		return d, d
	case ShapeDoubleCircle:
		d := math.Hypot(tw, th) + 26
		return d, d
	case ShapeRhombus:
		iw, ih := tw+12, th+8
		return 1.5 * iw, 3 * ih
	case ShapeHexagon:
		h = th + 2*py
		return tw + 2*px + h/2, h
	case ShapeParallelogram, ShapeParallelogramAlt, ShapeTrapezoid, ShapeTrapezoidAlt:
		h = th + 2*py
		return tw + 2*px + h, h
	case ShapeAsymmetric:
		h = th + 2*py
		return tw + 2*px + h/2, h
	}
	return tw + 2*px, th + 2*py
}

// endNode picks the node an edge to or from a subgraph is laid out
// against: for an edge in, the first node of the subgraph that nothing
// inside it points at; for an edge out, the last that points at nothing
// inside it. The drawn edge stops at the subgraph's border.
func (e *engine) endNode(end End, in bool) int {
	if end.Node >= 0 {
		return e.nodeOf[end.Node]
	}
	s := end.Subgraph
	var members []int
	for i, n := range e.f.Nodes {
		if n.Subgraph >= 0 && e.f.ancestor(s, n.Subgraph) {
			members = append(members, i)
		}
	}
	if len(members) == 0 {
		for c := range e.clusters {
			if e.anchorOf[c] >= 0 && e.f.ancestor(s, c) {
				return e.anchorOf[c]
			}
		}
	}
	inside := func(n int) bool { return e.f.Nodes[n].Subgraph >= 0 && e.f.ancestor(s, e.f.Nodes[n].Subgraph) }
	pick := func(n int) bool {
		for _, ed := range e.f.Edges {
			if in && ed.To.Node == n && ed.From.Node >= 0 && inside(ed.From.Node) {
				return false
			}
			if !in && ed.From.Node == n && ed.To.Node >= 0 && inside(ed.To.Node) {
				return false
			}
		}
		return true
	}
	if in {
		for _, n := range members {
			if pick(n) {
				return e.nodeOf[n]
			}
		}
		return e.nodeOf[members[0]]
	}
	for k := len(members) - 1; k >= 0; k-- {
		if pick(members[k]) {
			return e.nodeOf[members[k]]
		}
	}
	return e.nodeOf[members[len(members)-1]]
}

// buildEdges makes a chain for every edge that is not a self-loop, with
// just its two ends for now; normalise adds the dummies once ranks are
// known.
func (e *engine) buildEdges() error {
	for i, ed := range e.f.Edges {
		if ed.From.Node >= 0 && ed.From.Node == ed.To.Node {
			continue
		}
		u, v := e.endNode(ed.From, false), e.endNode(ed.To, true)
		if u == v {
			// Two subgraph ends resolving to one node cannot happen for
			// disjoint subgraphs, which is all the parser lets through.
			continue
		}
		e.chains = append(e.chains, chain{edge: i, nodes: []int{u, v}, label: -1})
	}
	return nil
}

// rank breaks cycles and assigns ranks.
func (e *engine) rank() error {
	n := len(e.nodes)
	out := make([][]int, n) // chain indexes by tail
	for ci, c := range e.chains {
		out[c.nodes[0]] = append(out[c.nodes[0]], ci)
	}
	// Reverse DFS back edges, visiting nodes and edges in source order so
	// the result is the same every time.
	state := make([]int8, n)
	for s := 0; s < n; s++ {
		if state[s] != 0 {
			continue
		}
		type frame struct{ v, i int }
		stack := []frame{{s, 0}}
		state[s] = 1
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.i >= len(out[top.v]) {
				state[top.v] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			ci := out[top.v][top.i]
			top.i++
			w := e.chains[ci].nodes[1]
			switch state[w] {
			case 1:
				e.chains[ci].reversed = true
			case 0:
				state[w] = 1
				stack = append(stack, frame{w, 0})
			}
		}
	}
	for ci := range e.chains {
		c := &e.chains[ci]
		if c.reversed {
			c.nodes[0], c.nodes[1] = c.nodes[1], c.nodes[0]
		}
	}
	// Longest path, in a topological order.
	type arc struct{ to, min int }
	succ := make([][]arc, n)
	pred := make([][]arc, n)
	indeg := make([]int, n)
	for _, c := range e.chains {
		m := 2 * e.f.Edges[c.edge].Length
		succ[c.nodes[0]] = append(succ[c.nodes[0]], arc{c.nodes[1], m})
		pred[c.nodes[1]] = append(pred[c.nodes[1]], arc{c.nodes[0], m})
		indeg[c.nodes[1]]++
	}
	var topo []int
	for v := 0; v < n; v++ {
		if indeg[v] == 0 {
			topo = append(topo, v)
		}
	}
	for k := 0; k < len(topo); k++ {
		for _, a := range succ[topo[k]] {
			if indeg[a.to]--; indeg[a.to] == 0 {
				topo = append(topo, a.to)
			}
		}
	}
	if len(topo) != n {
		return refuse(Unsupported, 0, "the layout could not break the diagram's cycles")
	}
	rank := make([]int, n)
	for _, v := range topo {
		for _, a := range pred[v] {
			rank[v] = max(rank[v], rank[a.to]+a.min)
		}
	}
	// Pull tight: move each node, within the ranks its edges allow, to the
	// median of where its edges would like it.
	for pass := 0; pass < 8; pass++ {
		if err := e.check("rank"); err != nil {
			return err
		}
		moved := false
		for k := range topo {
			v := topo[k]
			if pass%2 == 1 {
				v = topo[len(topo)-1-k]
			}
			lo, hi := math.MinInt32, math.MaxInt32
			var want []int
			for _, a := range pred[v] {
				lo = max(lo, rank[a.to]+a.min)
				want = append(want, rank[a.to]+a.min)
			}
			for _, a := range succ[v] {
				hi = min(hi, rank[a.to]-a.min)
				want = append(want, rank[a.to]-a.min)
			}
			if len(want) == 0 {
				continue
			}
			sort.Ints(want)
			r := want[len(want)/2]
			if len(want)%2 == 0 {
				r = want[len(want)/2-1]
			}
			r = min(max(r, lo), hi)
			if r != rank[v] {
				rank[v], moved = r, true
			}
		}
		if !moved {
			break
		}
	}
	least := math.MaxInt32
	for _, r := range rank {
		least = min(least, r)
	}
	for v, r := range rank {
		e.nodes[v].rank = r - least
	}
	return nil
}

// lca is the innermost cluster enclosing both a and b, or -1.
func (e *engine) lca(a, b int) int {
	for x := a; x >= 0; x = e.clusters[x].parent {
		for y := b; y >= 0; y = e.clusters[y].parent {
			if x == y {
				return x
			}
		}
	}
	return -1
}

// normalise splits every chain into one segment per rank, adds the
// subgraph placeholders, and builds the layers.
func (e *engine) normalise() error {
	for ci := range e.chains {
		c := &e.chains[ci]
		u, v := c.nodes[0], c.nodes[1]
		ed := e.f.Edges[c.edge]
		cl := e.lca(e.nodes[u].cluster, e.nodes[v].cluster)
		ru, rv := e.nodes[u].rank, e.nodes[v].rank
		labelRank := -1
		if len(ed.Label) > 0 && ed.Line != LineInvisible {
			labelRank = ru + (rv-ru)/2
		}
		nodes := []int{u}
		for r := ru + 1; r < rv; r++ {
			d := &lnode{kind: kDummy, ref: c.edge, rank: r, cluster: cl}
			if r == labelRank {
				d.kind = kLabel
				w, h := textBox(ed.Label)
				d.ol, d.or, d.rh = e.extents(w+8, h+4)
			}
			nodes = append(nodes, e.add(d))
			if r == labelRank {
				c.label = nodes[len(nodes)-1]
			}
			if len(e.nodes) > e.lim.MaxLayoutNodes {
				return refuse(Limit, 0, "the layout needs more than %d nodes", e.lim.MaxLayoutNodes)
			}
		}
		c.nodes = append(nodes, v)
		for k := 0; k+1 < len(c.nodes); k++ {
			a, b := e.nodes[c.nodes[k]], e.nodes[c.nodes[k+1]]
			w := 1.0
			switch {
			case a.kind != kReal && b.kind != kReal:
				w = 8
			case a.kind != kReal || b.kind != kReal:
				w = 2
			}
			a.down = append(a.down, c.nodes[k+1])
			a.downW = append(a.downW, w)
			b.up = append(b.up, c.nodes[k])
			b.upW = append(b.upW, w)
		}
	}
	// Placeholders: a subgraph that spans a rank has something in it in
	// that rank, so its contents are one contiguous run in every rank it
	// spans. Innermost first, so an outer subgraph sees its children's.
	for c := range e.clusters {
		e.clusters[c].first, e.clusters[c].last = math.MaxInt32, -1
	}
	for _, n := range e.nodes {
		for c := n.cluster; c >= 0; c = e.clusters[c].parent {
			e.clusters[c].first = min(e.clusters[c].first, n.rank)
			e.clusters[c].last = max(e.clusters[c].last, n.rank)
		}
	}
	byDepth := make([]int, len(e.clusters))
	for i := range byDepth {
		byDepth[i] = i
	}
	sort.SliceStable(byDepth, func(a, b int) bool { return e.clusters[byDepth[a]].depth > e.clusters[byDepth[b]].depth })
	for _, c := range byDepth {
		cl := e.clusters[c]
		present := map[int]bool{}
		for _, n := range e.nodes {
			if n.cluster >= 0 && e.f.ancestor(c, n.cluster) {
				present[n.rank] = true
			}
		}
		for r := cl.first; r <= cl.last; r++ {
			if !present[r] {
				e.add(&lnode{kind: kPlaceholder, ref: c, rank: r, cluster: c})
				if len(e.nodes) > e.lim.MaxLayoutNodes {
					return refuse(Limit, 0, "the layout needs more than %d nodes", e.lim.MaxLayoutNodes)
				}
			}
		}
	}
	maxRank := 0
	for _, n := range e.nodes {
		maxRank = max(maxRank, n.rank)
	}
	e.layers = make([][]int, maxRank+1)
	e.path = make([][]int, len(e.nodes))
	for v, n := range e.nodes {
		for c := n.cluster; c >= 0; c = e.clusters[c].parent {
			e.path[v] = append([]int{c}, e.path[v]...)
		}
	}
	return nil
}

// order fixes each node's position in its rank.
func (e *engine) order() error {
	// Start from a depth-first walk in source order, which keeps what the
	// author wrote together together.
	seen := make([]bool, len(e.nodes))
	var visit func(v int)
	visit = func(v int) {
		stack := []int{v}
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[x] {
				continue
			}
			seen[x] = true
			n := e.nodes[x]
			e.layers[n.rank] = append(e.layers[n.rank], x)
			for k := len(n.down) - 1; k >= 0; k-- {
				if !seen[n.down[k]] {
					stack = append(stack, n.down[k])
				}
			}
		}
	}
	for v := range e.nodes {
		if !seen[v] && len(e.nodes[v].up) == 0 {
			visit(v)
		}
	}
	for v := range e.nodes {
		if !seen[v] {
			visit(v)
		}
	}
	e.setPositions()
	sib := e.siblingOrder()
	for r := range e.layers {
		e.arrange(r, sib, e.currentKeys(r))
	}
	best := e.snapshot()
	bestCross := e.crossings()
	stale := 0
	for it := 0; it < orderRounds && bestCross > 0; it++ {
		if err := e.check("order"); err != nil {
			return err
		}
		sib = e.siblingOrder()
		for r := range e.layers {
			e.arrange(r, sib, e.currentKeys(r))
		}
		if it%2 == 0 {
			for r := 1; r < len(e.layers); r++ {
				e.arrange(r, sib, e.barycentres(r, true))
			}
		} else {
			for r := len(e.layers) - 2; r >= 0; r-- {
				e.arrange(r, sib, e.barycentres(r, false))
			}
		}
		if c := e.crossings(); c < bestCross {
			best, bestCross, stale = e.snapshot(), c, 0
		} else if stale++; stale >= 4 {
			break
		}
	}
	e.layers = best
	e.setPositions()
	return nil
}

func (e *engine) snapshot() [][]int {
	out := make([][]int, len(e.layers))
	for r, l := range e.layers {
		out[r] = append([]int(nil), l...)
	}
	return out
}

func (e *engine) setPositions() {
	for _, l := range e.layers {
		for i, v := range l {
			e.nodes[v].pos = i
		}
	}
}

func (e *engine) currentKeys(r int) map[int]float64 {
	k := make(map[int]float64, len(e.layers[r]))
	for i, v := range e.layers[r] {
		k[v] = float64(i)
	}
	return k
}

// barycentres keys each node of rank r by the mean position of its
// neighbours in the rank above (down sweep) or below. A node with none
// keeps its place, scaled to the other rank's width.
func (e *engine) barycentres(r int, down bool) map[int]float64 {
	k := make(map[int]float64, len(e.layers[r]))
	other := r + 1
	if down {
		other = r - 1
	}
	scale := float64(len(e.layers[other])) / math.Max(1, float64(len(e.layers[r])))
	for i, v := range e.layers[r] {
		nb := e.nodes[v].down
		if down {
			nb = e.nodes[v].up
		}
		if len(nb) == 0 {
			k[v] = float64(i) * scale
			continue
		}
		s := 0.0
		for _, u := range nb {
			s += float64(e.nodes[u].pos)
		}
		k[v] = s / float64(len(nb))
	}
	return k
}

// siblingOrder puts each set of sibling subgraphs in one order, by the mean
// relative position of everything inside each, to be kept in every rank.
func (e *engine) siblingOrder() []int {
	sum := make([]float64, len(e.clusters))
	cnt := make([]float64, len(e.clusters))
	for _, l := range e.layers {
		for i, v := range l {
			rel := (float64(i) + 0.5) / float64(len(l))
			for _, c := range e.path[v] {
				sum[c] += rel
				cnt[c]++
			}
		}
	}
	key := make([]float64, len(e.clusters))
	for c := range key {
		if cnt[c] > 0 {
			key[c] = sum[c] / cnt[c]
		}
	}
	rank := make([]int, len(e.clusters))
	idx := make([]int, len(e.clusters))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return key[idx[a]] < key[idx[b]] })
	for r, c := range idx {
		rank[c] = r
	}
	return rank
}

// arrange sorts rank r by key, keeping each subgraph's contents together
// and sibling subgraphs in their shared order.
func (e *engine) arrange(r int, sib []int, key map[int]float64) {
	var build func(depth int, items []int) []int
	build = func(depth int, items []int) []int {
		var plain []int
		groups := map[int][]int{}
		var kids []int
		for _, v := range items {
			if len(e.path[v]) > depth {
				c := e.path[v][depth]
				if _, ok := groups[c]; !ok {
					kids = append(kids, c)
				}
				groups[c] = append(groups[c], v)
			} else {
				plain = append(plain, v)
			}
		}
		sort.SliceStable(plain, func(a, b int) bool { return key[plain[a]] < key[plain[b]] })
		sort.SliceStable(kids, func(a, b int) bool { return sib[kids[a]] < sib[kids[b]] })
		kidKey := make([]float64, len(kids))
		for i, c := range kids {
			s := 0.0
			for _, v := range groups[c] {
				s += key[v]
			}
			kidKey[i] = s / float64(len(groups[c]))
			if i > 0 && kidKey[i] < kidKey[i-1] {
				kidKey[i] = kidKey[i-1]
			}
		}
		out := make([]int, 0, len(items))
		p := 0
		for i, c := range kids {
			for p < len(plain) && key[plain[p]] < kidKey[i] {
				out = append(out, plain[p])
				p++
			}
			out = append(out, build(depth+1, groups[c])...)
		}
		return append(out, plain[p:]...)
	}
	e.layers[r] = build(0, e.layers[r])
	for i, v := range e.layers[r] {
		e.nodes[v].pos = i
	}
}

// crossings counts edge crossings between every pair of adjacent ranks
// (Barth and Mutzel's accumulator tree).
func (e *engine) crossings() int {
	total := 0
	for r := 0; r+1 < len(e.layers); r++ {
		type pr struct{ a, b int }
		var ps []pr
		for _, v := range e.layers[r] {
			for _, w := range e.nodes[v].down {
				ps = append(ps, pr{e.nodes[v].pos, e.nodes[w].pos})
			}
		}
		sort.Slice(ps, func(i, j int) bool {
			if ps[i].a != ps[j].a {
				return ps[i].a < ps[j].a
			}
			return ps[i].b < ps[j].b
		})
		size := 1
		for size < len(e.layers[r+1]) {
			size <<= 1
		}
		tree := make([]int, 2*size)
		for _, p := range ps {
			i := p.b + size
			tree[i]++
			for i > 1 {
				if i%2 == 0 {
					total += tree[i+1]
				}
				i /= 2
				tree[i]++
			}
		}
	}
	return total
}

// constraint: x[to] - x[from] >= gap.
type constraint struct {
	from, to int
	gap      float64
}

// padding is the room between a subgraph's border and its contents on
// each TB-space side; the title band goes on whichever side is the top of
// the drawn diagram.
func (e *engine) padding(c int) (left, right, top, bottom float64) {
	left, right, top, bottom = clusterPad, clusterPad, clusterPad, clusterPad
	t := e.clusters[c].titleH
	switch e.dir {
	case TopBottom:
		top += t
	case BottomTop:
		bottom += t
	default:
		left += t
	}
	return
}

func (e *engine) sep(a, b int) float64 {
	if e.nodes[a].kind == kReal && e.nodes[b].kind == kReal {
		return nodeSep
	}
	return dummySep
}

// positionX places every node across its rank.
func (e *engine) positionX() error {
	n := len(e.nodes)
	for c, cl := range e.clusters {
		cl.lVar, cl.rVar = n+2*c, n+2*c+1
	}
	vars := n + 2*len(e.clusters)
	gaps := map[[2]int]float64{}
	add := func(a, b int, g float64) {
		k := [2]int{a, b}
		if old, ok := gaps[k]; !ok || g > old {
			gaps[k] = g
		}
	}
	for _, l := range e.layers {
		for i, v := range l {
			pv := e.path[v]
			var prev []int
			if i > 0 {
				prev = e.path[l[i-1]]
			}
			common := 0
			for common < len(prev) && common < len(pv) && prev[common] == pv[common] {
				common++
			}
			from, off, gap := -1, 0.0, 0.0
			if i > 0 {
				from, off = l[i-1], e.nodes[l[i-1]].or
				gap = e.sep(l[i-1], v)
				for k := len(prev) - 1; k >= common; k-- {
					_, right, _, _ := e.padding(prev[k])
					add(from, e.clusters[prev[k]].rVar, off+right)
					from, off, gap = e.clusters[prev[k]].rVar, 0, clusterSep
				}
			}
			for k := common; k < len(pv); k++ {
				if from >= 0 {
					if k == common && i > 0 && len(prev) == common {
						gap = clusterSep
					}
					add(from, e.clusters[pv[k]].lVar, off+gap)
				}
				left, _, _, _ := e.padding(pv[k])
				from, off, gap = e.clusters[pv[k]].lVar, 0, left
			}
			if from >= 0 {
				add(from, v, off+gap+e.nodes[v].ol)
			}
			if i == len(l)-1 {
				from, off = v, e.nodes[v].or
				for k := len(pv) - 1; k >= 0; k-- {
					_, right, _, _ := e.padding(pv[k])
					add(from, e.clusters[pv[k]].rVar, off+right)
					from, off = e.clusters[pv[k]].rVar, 0
				}
			}
		}
	}
	for _, cl := range e.clusters {
		if !e.dir.horizontal() {
			add(cl.lVar, cl.rVar, cl.titleW+2*clusterPad)
		} else {
			add(cl.lVar, cl.rVar, 2*clusterPad)
		}
	}
	in := make([][]constraint, vars)
	out := make([][]constraint, vars)
	indeg := make([]int, vars)
	for k, g := range gaps {
		c := constraint{k[0], k[1], g}
		out[k[0]] = append(out[k[0]], c)
		in[k[1]] = append(in[k[1]], c)
		indeg[k[1]]++
	}
	for v := range out {
		sort.Slice(out[v], func(a, b int) bool { return out[v][a].to < out[v][b].to })
		sort.Slice(in[v], func(a, b int) bool { return in[v][a].from < in[v][b].from })
	}
	var topo []int
	for v := 0; v < vars; v++ {
		if indeg[v] == 0 {
			topo = append(topo, v)
		}
	}
	for k := 0; k < len(topo); k++ {
		for _, c := range out[topo[k]] {
			if indeg[c.to]--; indeg[c.to] == 0 {
				topo = append(topo, c.to)
			}
		}
	}
	if len(topo) != vars {
		return refuse(Unsupported, 0, "the layout could not order the subgraphs")
	}
	x := make([]float64, vars)
	for _, v := range topo {
		for _, c := range in[v] {
			x[v] = math.Max(x[v], x[c.from]+c.gap)
		}
	}
	lo := func(v int) float64 {
		m := math.Inf(-1)
		for _, c := range in[v] {
			m = math.Max(m, x[c.from]+c.gap)
		}
		return m
	}
	hi := func(v int) float64 {
		m := math.Inf(1)
		for _, c := range out[v] {
			m = math.Min(m, x[c.to]-c.gap)
		}
		return m
	}
	isL := func(v int) bool { return v >= n && (v-n)%2 == 0 }
	isR := func(v int) bool { return v >= n && (v-n)%2 == 1 }
	hug := func() {
		for k := len(topo) - 1; k >= 0; k-- {
			if v := topo[k]; isL(v) {
				if h := hi(v); !math.IsInf(h, 1) {
					x[v] = h
				}
			}
		}
		for _, v := range topo {
			if isR(v) {
				if l := lo(v); !math.IsInf(l, -1) {
					x[v] = l
				}
			}
		}
	}
	type pair struct{ x, w float64 }
	for round := 0; round < relaxRounds; round++ {
		if err := e.check("position"); err != nil {
			return err
		}
		// Loosen every subgraph border as far as its neighbours allow, so
		// its contents can move, then move each node towards the weighted
		// median of its neighbours, then pull the borders back in.
		for _, v := range topo {
			if isL(v) {
				x[v] = lo(v)
			}
		}
		for k := len(topo) - 1; k >= 0; k-- {
			if v := topo[k]; isR(v) {
				x[v] = hi(v)
			}
		}
		centre := make([]float64, len(e.clusters))
		count := make([]float64, len(e.clusters))
		for v, nd := range e.nodes {
			if nd.kind != kPlaceholder {
				for _, c := range e.path[v] {
					centre[c] += x[v]
					count[c]++
				}
			}
		}
		moved := 0.0
		for pass := 0; pass < 2; pass++ {
			for li := range e.layers {
				r := li
				if pass == 1 {
					r = len(e.layers) - 1 - li
				}
				l := e.layers[r]
				for dirn := 0; dirn < 2; dirn++ {
					for k := range l {
						v := l[k]
						if dirn == 1 {
							v = l[len(l)-1-k]
						}
						nd := e.nodes[v]
						var want float64
						if nd.kind == kPlaceholder {
							if count[nd.ref] == 0 {
								continue
							}
							want = centre[nd.ref] / count[nd.ref]
						} else {
							var ps []pair
							for i, u := range nd.up {
								ps = append(ps, pair{x[u], nd.upW[i]})
							}
							for i, u := range nd.down {
								ps = append(ps, pair{x[u], nd.downW[i]})
							}
							if len(ps) == 0 {
								continue
							}
							sort.Slice(ps, func(a, b int) bool { return ps[a].x < ps[b].x })
							total := 0.0
							for _, p := range ps {
								total += p.w
							}
							acc := 0.0
							for i, p := range ps {
								acc += p.w
								if acc*2 >= total {
									want = p.x
									if acc*2 == total && i+1 < len(ps) {
										want = (p.x + ps[i+1].x) / 2
									}
									break
								}
							}
						}
						nx := math.Min(math.Max(want, lo(v)), hi(v))
						if !math.IsNaN(nx) && !math.IsInf(nx, 0) {
							moved += math.Abs(nx - x[v])
							x[v] = nx
						}
					}
				}
			}
		}
		hug()
		if moved < 0.5 {
			break
		}
	}
	hug()
	for v, nd := range e.nodes {
		nd.x = x[v]
	}
	for _, cl := range e.clusters {
		cl.x0, cl.x1 = x[cl.lVar], x[cl.rVar]
	}
	return nil
}

// positionY places the ranks down the page, leaving room at each rank
// boundary for the borders and titles of the subgraphs that open or close
// there.
func (e *engine) positionY() {
	R := len(e.layers)
	height := make([]float64, R)
	for _, nd := range e.nodes {
		height[nd.rank] = math.Max(height[nd.rank], nd.rh)
	}
	extra := make([]float64, R+1)
	for c, cl := range e.clusters {
		_, _, top, bottom := e.padding(c)
		extra[cl.first] += top + clusterSep/2
		extra[cl.last+1] += bottom + clusterSep/2
	}
	top := make([]float64, R)
	place := func() {
		y := 0.0
		for r := 0; r < R; r++ {
			y += extra[r]
			if r > 0 {
				y += rankGap
			}
			top[r] = y
			y += height[r]
		}
		for _, nd := range e.nodes {
			nd.y = top[nd.rank] + height[nd.rank]/2
		}
		// Boxes, innermost first so each outer one encloses its children.
		order := make([]int, len(e.clusters))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool { return e.clusters[order[a]].depth > e.clusters[order[b]].depth })
		for _, c := range order {
			cl := e.clusters[c]
			y0, y1 := top[cl.first], top[cl.last]+height[cl.last]
			for _, k := range cl.children {
				y0 = math.Min(y0, e.clusters[k].y0)
				y1 = math.Max(y1, e.clusters[k].y1)
			}
			_, _, pt, pb := e.padding(c)
			cl.y0, cl.y1 = y0-pt, y1+pb
		}
	}
	place()
	if !e.dir.horizontal() {
		return
	}
	// Ranks run across the page, so a subgraph's drawn width is its extent
	// down the ranks: widen the gaps around one too narrow for its title.
	for pass := 0; pass < 3; pass++ {
		grew := false
		for _, cl := range e.clusters {
			need := cl.titleW + 2*clusterPad
			if have := cl.y1 - cl.y0; have < need {
				extra[cl.first] += (need - have) / 2
				extra[cl.last+1] += (need - have) / 2
				grew = true
			}
		}
		if !grew {
			return
		}
		place()
	}
}
