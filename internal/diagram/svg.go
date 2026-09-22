package diagram

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// The SVG writer. It builds every element and attribute itself from a
// fixed set (allowedElements and allowedAttrs, which the tests hold the
// output to), and source text reaches it only as label lines, each written
// as escaped character data inside a <tspan>. Nothing it writes refers to
// anything: no href, no url(), no id, no class, no style, no <script>, no
// <foreignObject>, no <marker> — arrowheads are drawn as polygons rather
// than referenced — so there is nothing for a hostile label to point at or
// break out of.

// Theme is the palette a diagram is drawn in. Every colour is a six-digit
// lower-case hex value; Render refuses a theme with anything else, so a
// theme cannot carry text into the SVG either.
type Theme struct {
	Background    string // the whole image; "" leaves it transparent
	NodeFill      string
	NodeStroke    string
	Text          string // node text and edge labels
	Edge          string // edge lines and heads
	LabelFill     string // behind an edge label
	ClusterFill   string
	ClusterStroke string
	ClusterText   string
	StrokeWidth   float64 // node and edge outlines; thick edges are twice this
}

// The three palettes the reading view needs, from the app's own colours
// (ui/src/style.css). EInk is black on white with heavier lines: a panel
// with no backlight loses a grey that a screen shows.
var (
	Light = Theme{
		Background: "#fbfbfa", NodeFill: "#ffffff", NodeStroke: "#6b6b6b", Text: "#1f1f1f",
		Edge: "#4f4f4d", LabelFill: "#fbfbfa",
		ClusterFill: "#f1f1ee", ClusterStroke: "#8a8a86", ClusterText: "#1f1f1f",
		StrokeWidth: 1.25,
	}
	Dark = Theme{
		Background: "#1b1b1b", NodeFill: "#262626", NodeStroke: "#9a9a96", Text: "#e6e6e3",
		Edge: "#b4b4b0", LabelFill: "#1b1b1b",
		ClusterFill: "#222221", ClusterStroke: "#7a7a76", ClusterText: "#e6e6e3",
		StrokeWidth: 1.25,
	}
	EInk = Theme{
		Background: "#ffffff", NodeFill: "#ffffff", NodeStroke: "#000000", Text: "#000000",
		Edge: "#000000", LabelFill: "#ffffff",
		ClusterFill: "#ffffff", ClusterStroke: "#000000", ClusterText: "#000000",
		StrokeWidth: 2,
	}
)

var hexColour = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func (t Theme) valid() error {
	for _, c := range []string{t.NodeFill, t.NodeStroke, t.Text, t.Edge, t.LabelFill, t.ClusterFill, t.ClusterStroke, t.ClusterText} {
		if !hexColour.MatchString(c) {
			return fmt.Errorf("diagram: theme colour %q is not #rrggbb", c)
		}
	}
	if t.Background != "" && !hexColour.MatchString(t.Background) {
		return fmt.Errorf("diagram: theme background %q is not #rrggbb", t.Background)
	}
	if !(t.StrokeWidth >= 0.5 && t.StrokeWidth <= 4) {
		return fmt.Errorf("diagram: theme stroke width %v is outside 0.5 to 4", t.StrokeWidth)
	}
	return nil
}

// fontFamily names Noto Sans first because the label widths are measured
// against it (width.go); every face after it sets narrower.
const fontFamily = "'Noto Sans', Roboto, Arial, Helvetica, sans-serif"

type svgWriter struct {
	b bytes.Buffer
	t Theme
}

func num(v float64) string {
	v = math.Round(v*10) / 10
	if v == 0 {
		v = 0 // no "-0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// el writes one element with its attributes, which are name/value pairs
// that come only from this file: numbers, theme colours and constants.
func (w *svgWriter) el(name string, selfClose bool, attrs ...string) {
	w.b.WriteString("<" + name)
	for i := 0; i+1 < len(attrs); i += 2 {
		w.b.WriteString(" " + attrs[i] + `="`)
		xml.EscapeText(&w.b, []byte(attrs[i+1]))
		w.b.WriteString(`"`)
	}
	if selfClose {
		w.b.WriteString("/>")
	} else {
		w.b.WriteString(">")
	}
}

// text writes lines of source text centred on (x, y). This is the only
// place source text enters the SVG, and it goes through xml.EscapeText,
// which escapes < > & ' " and the line-ending controls and replaces
// anything XML cannot carry with U+FFFD.
func (w *svgWriter) text(lines []string, x, y float64, fill string) {
	if len(lines) == 0 {
		return
	}
	top := y - float64(len(lines))*lineHeight/2
	w.el("text", false, "x", num(x), "y", num(top), "fill", fill, "text-anchor", "middle")
	for i, l := range lines {
		base := top + (float64(i)+0.5)*lineHeight + capHeight*fontSize/2
		w.el("tspan", false, "x", num(x), "y", num(base))
		xml.EscapeText(&w.b, []byte(l))
		w.b.WriteString("</tspan>")
	}
	w.b.WriteString("</text>")
}

func writeSVG(d *drawing, t Theme) []byte {
	w := &svgWriter{t: t}
	sw := num(t.StrokeWidth)
	w.el("svg", false,
		"xmlns", "http://www.w3.org/2000/svg",
		"width", num(d.w), "height", num(d.h),
		"viewBox", "0 0 "+num(d.w)+" "+num(d.h),
		"font-family", fontFamily, "font-size", num(fontSize))
	if t.Background != "" {
		w.el("rect", true, "x", "0", "y", "0", "width", num(d.w), "height", num(d.h), "fill", t.Background)
	}
	for _, c := range d.clusters {
		w.el("rect", true, "x", num(c.x), "y", num(c.y), "width", num(c.w), "height", num(c.h),
			"rx", "4", "fill", t.ClusterFill, "stroke", t.ClusterStroke, "stroke-width", sw)
		if len(c.title) > 0 {
			x0, y0, x1, y1 := c.titleBox()
			w.text(c.title, (x0+x1)/2, (y0+y1)/2, t.ClusterText)
		}
	}
	for _, e := range d.edges {
		w.edge(e)
	}
	for _, n := range d.nodes {
		w.node(n)
	}
	// Edge labels last, over any node or line they would otherwise hide
	// behind.
	for _, e := range d.edges {
		if len(e.label) == 0 {
			continue
		}
		w.el("rect", true, "x", num(e.lx-e.lw/2-4), "y", num(e.ly-e.lh/2-2), "width", num(e.lw+8), "height", num(e.lh+4),
			"rx", "2", "fill", t.LabelFill)
		w.text(e.label, e.lx, e.ly, t.Text)
	}
	w.b.WriteString("</svg>\n")
	return w.b.Bytes()
}

func (w *svgWriter) outline() []string {
	return []string{"fill", w.t.NodeFill, "stroke", w.t.NodeStroke, "stroke-width", num(w.t.StrokeWidth)}
}

func poly(pts ...float64) string {
	var b bytes.Buffer
	for i := 0; i+1 < len(pts); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(num(pts[i]) + "," + num(pts[i+1]))
	}
	return b.String()
}

func (w *svgWriter) node(n dnode) {
	x0, y0, x1, y1 := n.x-n.w/2, n.y-n.h/2, n.x+n.w/2, n.y+n.h/2
	o := w.outline()
	rect := func(rx float64) {
		w.el("rect", true, append([]string{"x", num(x0), "y", num(y0), "width", num(n.w), "height", num(n.h), "rx", num(rx)}, o...)...)
	}
	polygon := func(pts ...float64) {
		w.el("polygon", true, append([]string{"points", poly(pts...)}, o...)...)
	}
	s := n.h / 2
	switch n.shape {
	case ShapeRect:
		rect(0)
	case ShapeRound:
		rect(8)
	case ShapeStadium:
		rect(n.h / 2)
	case ShapeSubroutine:
		rect(0)
		for _, x := range []float64{x0 + 8, x1 - 8} {
			w.el("line", true, "x1", num(x), "y1", num(y0), "x2", num(x), "y2", num(y1), "stroke", w.t.NodeStroke, "stroke-width", num(w.t.StrokeWidth))
		}
	case ShapeCylinder:
		rx := n.w / 2
		ry := math.Min(10, math.Max(4, n.w/12))
		body := fmt.Sprintf("M%s,%s A%s,%s 0 0 1 %s,%s V%s A%s,%s 0 0 1 %s,%s Z",
			num(x0), num(y0+ry), num(rx), num(ry), num(x1), num(y0+ry), num(y1-ry), num(rx), num(ry), num(x0), num(y1-ry))
		w.el("path", true, append([]string{"d", body}, o...)...)
		lid := fmt.Sprintf("M%s,%s A%s,%s 0 0 0 %s,%s", num(x0), num(y0+ry), num(rx), num(ry), num(x1), num(y0+ry))
		w.el("path", true, "d", lid, "fill", "none", "stroke", w.t.NodeStroke, "stroke-width", num(w.t.StrokeWidth))
	case ShapeCircle:
		w.el("ellipse", true, append([]string{"cx", num(n.x), "cy", num(n.y), "rx", num(n.w / 2), "ry", num(n.h / 2)}, o...)...)
	case ShapeDoubleCircle:
		w.el("ellipse", true, append([]string{"cx", num(n.x), "cy", num(n.y), "rx", num(n.w / 2), "ry", num(n.h / 2)}, o...)...)
		w.el("ellipse", true, append([]string{"cx", num(n.x), "cy", num(n.y), "rx", num(n.w/2 - 5), "ry", num(n.h/2 - 5)}, o...)...)
	case ShapeRhombus:
		polygon(n.x, y0, x1, n.y, n.x, y1, x0, n.y)
	case ShapeHexagon:
		i := n.h / 4
		polygon(x0+i, y0, x1-i, y0, x1, n.y, x1-i, y1, x0+i, y1, x0, n.y)
	case ShapeParallelogram:
		polygon(x0+s, y0, x1, y0, x1-s, y1, x0, y1)
	case ShapeParallelogramAlt:
		polygon(x0, y0, x1-s, y0, x1, y1, x0+s, y1)
	case ShapeTrapezoid:
		polygon(x0+s, y0, x1-s, y0, x1, y1, x0, y1)
	case ShapeTrapezoidAlt:
		polygon(x0, y0, x1, y0, x1-s, y1, x0+s, y1)
	case ShapeAsymmetric:
		polygon(x0, y0, x1, y0, x1, y1, x0, y1, x0+s/2, n.y)
	}
	cx := n.x
	if n.shape == ShapeAsymmetric {
		cx += s / 4
	}
	w.text(n.lines, cx, n.y, w.t.Text)
}

const headLen = 9.0

func (w *svgWriter) edge(e dedge) {
	pts := append([]point(nil), e.pts...)
	if len(pts) < 2 {
		return
	}
	sw := w.t.StrokeWidth
	if e.line == LineThick {
		sw *= 2
	}
	scale := math.Max(1, sw/1.25)
	// Pull the line's ends back so it stops at the base of its heads.
	end, tip := pts[len(pts)-1], pts[len(pts)-2]
	start, next := pts[0], pts[1]
	pts[len(pts)-1] = back(end, tip, w.headRoom(e.head, scale))
	pts[0] = back(start, next, w.headRoom(e.tail, scale))
	var d string
	if e.loop {
		d = fmt.Sprintf("M%s,%s C%s,%s %s,%s %s,%s", num(pts[0].x), num(pts[0].y), num(pts[1].x), num(pts[1].y), num(pts[2].x), num(pts[2].y), num(pts[3].x), num(pts[3].y))
	} else {
		d = basis(pts)
	}
	attrs := []string{"d", d, "fill", "none", "stroke", w.t.Edge, "stroke-width", num(sw)}
	if e.line == LineDotted {
		attrs = append(attrs, "stroke-dasharray", num(3*scale)+" "+num(3*scale))
	}
	w.el("path", true, attrs...)
	w.head(e.head, end, tip, scale)
	w.head(e.tail, start, next, scale)
}

func (w *svgWriter) headRoom(h Head, scale float64) float64 {
	switch h {
	case HeadArrow:
		return headLen * scale
	case HeadCircle:
		return 8 * scale
	}
	return 0
}

// back is the point dist from p towards q.
func back(p, q point, dist float64) point {
	dx, dy := q.x-p.x, q.y-p.y
	l := math.Hypot(dx, dy)
	if l == 0 || dist == 0 {
		return p
	}
	dist = math.Min(dist, l*0.9)
	return point{p.x + dx/l*dist, p.y + dy/l*dist}
}

// head draws an edge's end at p, arriving from the direction of q.
func (w *svgWriter) head(h Head, p, q point, scale float64) {
	dx, dy := p.x-q.x, p.y-q.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	switch h {
	case HeadArrow:
		bx, by := p.x-ux*headLen*scale, p.y-uy*headLen*scale
		hw := 4.5 * scale
		w.el("polygon", true, "points", poly(p.x, p.y, bx-uy*hw, by+ux*hw, bx+uy*hw, by-ux*hw), "fill", w.t.Edge)
	case HeadCircle:
		r := 4 * scale
		w.el("circle", true, "cx", num(p.x-ux*r), "cy", num(p.y-uy*r), "r", num(r),
			"fill", w.t.hollowFill(), "stroke", w.t.Edge, "stroke-width", num(w.t.StrokeWidth))
	case HeadCross:
		c := 5 * scale
		cx, cy := p.x-ux*c, p.y-uy*c
		d := fmt.Sprintf("M%s,%s L%s,%s M%s,%s L%s,%s",
			num(cx-c*0.7), num(cy-c*0.7), num(cx+c*0.7), num(cy+c*0.7),
			num(cx-c*0.7), num(cy+c*0.7), num(cx+c*0.7), num(cy-c*0.7))
		w.el("path", true, "d", d, "fill", "none", "stroke", w.t.Edge, "stroke-width", num(w.t.StrokeWidth*1.2))
	}
}

// hollowFill is the colour a hollow head is filled with: the image's
// background, or the node fill when the image has none.
func (t Theme) hollowFill() string {
	if t.Background != "" {
		return t.Background
	}
	return t.NodeFill
}

// basis writes the path data of a B-spline through the polyline's control
// points (basisPieces).
func basis(p []point) string {
	var b bytes.Buffer
	ps := basisPieces(p)
	b.WriteString("M" + num(p[0].x) + "," + num(p[0].y))
	for _, pc := range ps {
		if pc.cubic {
			fmt.Fprintf(&b, " C%s,%s %s,%s %s,%s", num(pc.b.x), num(pc.b.y), num(pc.c.x), num(pc.c.y), num(pc.d.x), num(pc.d.y))
		} else {
			b.WriteString(" L" + num(pc.d.x) + "," + num(pc.d.y))
		}
	}
	return b.String()
}
