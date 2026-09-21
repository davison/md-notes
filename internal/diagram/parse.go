package diagram

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Parse reads the text of a mermaid flowchart into the typed model, or
// refuses it. It accepts the subset documented on the package and refuses
// everything else rather than guessing at it; the returned error is always
// a *Refusal.
func Parse(src []byte, lim Limits) (*Flowchart, error) {
	if len(src) > lim.MaxBytes {
		return nil, refuse(Limit, 0, "the diagram is %d bytes; the limit is %d", len(src), lim.MaxBytes)
	}
	if !utf8.Valid(src) {
		return nil, refuse(Syntax, 0, "the diagram is not valid UTF-8")
	}
	p := &parser{
		lim:      lim,
		f:        &Flowchart{},
		mentions: map[string]*mention{},
		subIndex: map[string]int{},
	}
	if err := p.run(string(src)); err != nil {
		return nil, err
	}
	return p.f, nil
}

type mention struct {
	name     string
	label    []string
	shape    Shape
	hasShape bool
	sub      int // the first subgraph whose body mentions it, or -1
	line     int
}

type pendingEdge struct {
	from, to string
	edge     Edge
	line     int
}

type parser struct {
	lim      Limits
	f        *Flowchart
	mentions map[string]*mention
	order    []*mention
	subIndex map[string]int
	stack    []int // open subgraphs, innermost last
	edges    []pendingEdge
	header   bool
}

// ignored are the statements that only style or wire up interaction:
// colours are the theme's business, and an image has nothing to click. They
// are recognised and skipped whole — a statement at a time, up to its
// newline or semicolon — never parsed into anything that is drawn.
var ignored = map[string]bool{
	"style":     true,
	"classDef":  true,
	"class":     true,
	"linkStyle": true,
	"click":     true,
}

func (p *parser) run(src string) error {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	lines := strings.Split(src, "\n")
	for n, raw := range lines {
		line := n + 1
		text, err := stripComment(raw, line)
		if err != nil {
			return err
		}
		if !p.header && strings.TrimSpace(text) == "---" {
			return refuse(Unsupported, line, "front matter (a --- block) is not supported")
		}
		stmts, err := splitStatements(text, line)
		if err != nil {
			return err
		}
		for _, st := range stmts {
			st = strings.TrimSpace(st)
			if st == "" {
				continue
			}
			if err := p.statement(st, line); err != nil {
				return err
			}
		}
	}
	if !p.header {
		return refuse(Syntax, 0, "the diagram is empty")
	}
	if len(p.stack) > 0 {
		return refuse(Syntax, 0, "subgraph %q is never closed with end", p.f.Subgraphs[p.stack[len(p.stack)-1]].ID)
	}
	return p.resolve()
}

// stripComment removes a %% comment from a line. A %%{ is a directive
// (%%{init: …}%% and its kin), which can reconfigure how mermaid draws
// anything; it is refused rather than ignored.
func stripComment(s string, line int) (string, error) {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case !inQuote && strings.HasPrefix(s[i:], "%%"):
			if strings.HasPrefix(s[i:], "%%{") {
				return "", refuse(Unsupported, line, "%%%%{…}%%%% directives are not supported")
			}
			return s[:i], nil
		}
	}
	return s, nil
}

// splitStatements splits a line at the semicolons outside double quotes.
func splitStatements(s string, line int) ([]string, error) {
	var out []string
	inQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case ';':
			// The semicolon that ends a mermaid entity code (#quot;, #35;)
			// is part of the text, not the end of a statement — except in
			// a skipped styling statement, where `fill:#fff;` is a colour
			// and its statement's end.
			if !inQuote && (styling(s[start:i]) || !entityBefore(s[:i])) {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if inQuote {
		return nil, refuse(Syntax, line, "a double quote is not closed")
	}
	return append(out, s[start:]), nil
}

// styling reports whether a statement is one of the skipped styling or
// interaction statements.
func styling(st string) bool {
	st = strings.TrimSpace(st)
	if i := strings.IndexAny(st, " \t"); i >= 0 {
		st = st[:i]
	}
	return ignored[st]
}

// entityBefore reports whether s ends in the body of an entity code: a #
// and then letters only or digits only. It looks back at most as far as the
// longest code, so a line of semicolons costs no more than its length.
func entityBefore(s string) bool {
	letters, digits := false, false
	for k := 1; k <= 12 && k <= len(s); k++ {
		c := s[len(s)-k]
		switch {
		case c == '#':
			return k > 1 && letters != digits
		case c >= '0' && c <= '9':
			digits = true
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			letters = true
		default:
			return false
		}
	}
	return false
}

var directions = map[string]Direction{
	"TB": TopBottom, "TD": TopBottom, "BT": BottomTop, "LR": LeftRight, "RL": RightLeft,
}

func (p *parser) statement(st string, line int) error {
	word, rest := st, ""
	if i := strings.IndexAny(st, " \t"); i >= 0 {
		word, rest = st[:i], strings.TrimSpace(st[i+1:])
	}
	if !p.header {
		if word != "flowchart" && word != "graph" {
			return refuse(Unsupported, line, "only flowchart and graph diagrams are drawn")
		}
		p.header = true
		if rest == "" {
			p.f.Direction = TopBottom
			return nil
		}
		d, ok := directions[strings.ToUpper(rest)]
		if !ok {
			return refuse(Syntax, line, "unknown direction %q; use TB, TD, BT, LR or RL", rest)
		}
		p.f.Direction = d
		return nil
	}
	switch {
	case word == "subgraph":
		return p.subgraph(rest, line)
	case word == "end" && rest == "":
		if len(p.stack) == 0 {
			return refuse(Syntax, line, "end without a subgraph to close")
		}
		p.stack = p.stack[:len(p.stack)-1]
		return nil
	case word == "direction":
		if len(p.stack) == 0 {
			return refuse(Unsupported, line, "direction outside a subgraph; the direction goes on the flowchart line")
		}
		if _, ok := directions[strings.ToUpper(rest)]; !ok {
			return refuse(Syntax, line, "unknown direction %q", rest)
		}
		// Accepted and not honoured: a subgraph is drawn in the diagram's
		// own direction, which is also what mermaid does whenever an edge
		// crosses the subgraph's border.
		return nil
	case ignored[word]:
		return nil
	case accessibility(word) != "":
		return refuse(Unsupported, line, "%s is not supported", accessibility(word))
	case word == "title":
		return refuse(Unsupported, line, "title is not supported")
	}
	return p.chain(st, line)
}

// accessibility is the accTitle or accDescr statement a word opens, or "".
func accessibility(word string) string {
	for _, kw := range []string{"accTitle", "accDescr"} {
		if word == kw || strings.HasPrefix(word, kw+":") || strings.HasPrefix(word, kw+"{") {
			return kw
		}
	}
	return ""
}

func (p *parser) subgraph(rest string, line int) error {
	if len(p.stack) >= p.lim.MaxDepth {
		return refuse(Limit, line, "subgraphs are nested more than %d deep", p.lim.MaxDepth)
	}
	if len(p.f.Subgraphs) >= p.lim.MaxSubgraphs {
		return refuse(Limit, line, "more than %d subgraphs", p.lim.MaxSubgraphs)
	}
	if rest == "" {
		return refuse(Syntax, line, "subgraph needs a name")
	}
	var id string
	var title []string
	var err error
	n := idLen(rest, 0)
	b := skipSpace(rest, n)
	switch {
	case strings.HasPrefix(rest, `"`):
		title, err = label(rest, line, p.lim)
	case n > 0 && b < len(rest) && rest[b] == '[':
		id = rest[:n]
		if !strings.HasSuffix(rest, "]") {
			return refuse(Syntax, line, "subgraph title is not closed with ]")
		}
		inner := rest[b+1 : len(rest)-1]
		if t := strings.TrimSpace(inner); !strings.HasPrefix(t, `"`) && strings.ContainsAny(t, "[](){}") {
			return refuse(Syntax, line, "put a subgraph title with brackets in double quotes")
		}
		title, err = label(inner, line, p.lim)
	default:
		if strings.ContainsAny(rest, "[](){}") {
			return refuse(Syntax, line, "put a subgraph title with brackets in double quotes")
		}
		title, err = label(rest, line, p.lim)
		if !strings.ContainsAny(rest, " \t") {
			id = rest
		}
	}
	if err != nil {
		return err
	}
	if id != "" {
		if _, dup := p.subIndex[id]; dup {
			return refuse(Unsupported, line, "subgraph %q is defined twice", id)
		}
		p.subIndex[id] = len(p.f.Subgraphs)
	}
	parent := -1
	if len(p.stack) > 0 {
		parent = p.stack[len(p.stack)-1]
	}
	p.stack = append(p.stack, len(p.f.Subgraphs))
	p.f.Subgraphs = append(p.f.Subgraphs, Subgraph{ID: id, Title: title, Parent: parent})
	return nil
}

// chain parses `group (link group)*`, where a group is `node (& node)*`.
func (p *parser) chain(s string, line int) error {
	i := 0
	prev, i, err := p.group(s, i, line)
	if err != nil {
		return err
	}
	for {
		i = skipSpace(s, i)
		if i >= len(s) {
			return nil
		}
		lk, j, err := parseLink(s, i, line, p.lim)
		if err != nil {
			return err
		}
		next, k, err := p.group(s, skipSpace(s, j), line)
		if err != nil {
			return err
		}
		for _, a := range prev {
			for _, b := range next {
				if len(p.edges) >= p.lim.MaxEdges {
					return refuse(Limit, line, "more than %d edges", p.lim.MaxEdges)
				}
				p.edges = append(p.edges, pendingEdge{from: a, to: b, edge: lk, line: line})
			}
		}
		prev, i = next, k
	}
}

func (p *parser) group(s string, i int, line int) ([]string, int, error) {
	var names []string
	for {
		name, j, err := p.node(s, skipSpace(s, i), line)
		if err != nil {
			return nil, 0, err
		}
		names = append(names, name)
		j = skipSpace(s, j)
		if j < len(s) && s[j] == '&' {
			i = j + 1
			continue
		}
		return names, j, nil
	}
}

// shapes are the node outlines by their opening delimiter, longest first so
// that `((` is not read as `(`. Two openers have two possible closers.
var shapes = []struct {
	open   string
	closes []string
	shapes []Shape
}{
	{"(((", []string{")))"}, []Shape{ShapeDoubleCircle}},
	{"((", []string{"))"}, []Shape{ShapeCircle}},
	{"([", []string{"])"}, []Shape{ShapeStadium}},
	{"[[", []string{"]]"}, []Shape{ShapeSubroutine}},
	{"[(", []string{")]"}, []Shape{ShapeCylinder}},
	{"[/", []string{"/]", `\]`}, []Shape{ShapeParallelogram, ShapeTrapezoid}},
	{`[\`, []string{`\]`, "/]"}, []Shape{ShapeParallelogramAlt, ShapeTrapezoidAlt}},
	{"{{", []string{"}}"}, []Shape{ShapeHexagon}},
	{"(", []string{")"}, []Shape{ShapeRound}},
	{"[", []string{"]"}, []Shape{ShapeRect}},
	{"{", []string{"}"}, []Shape{ShapeRhombus}},
	{">", []string{"]"}, []Shape{ShapeAsymmetric}},
}

// node parses `id shape? (:::class)?` and records the mention.
func (p *parser) node(s string, i int, line int) (string, int, error) {
	n := idLen(s, i)
	if n == 0 {
		if i >= len(s) {
			return "", 0, refuse(Syntax, line, "expected a node at the end of the line")
		}
		return "", 0, refuse(Syntax, line, "expected a node at %q", excerpt(s[i:]))
	}
	name := s[i : i+n]
	i += n
	m := p.mention(name, line)
	if m == nil {
		return "", 0, refuse(Limit, line, "more than %d nodes", p.lim.MaxNodes)
	}
	if strings.HasPrefix(s[i:], "@{") {
		return "", 0, refuse(Unsupported, line, "the @{ shape: … } syntax is not supported")
	}
	for _, sh := range shapes {
		if !strings.HasPrefix(s[i:], sh.open) {
			continue
		}
		body := i + len(sh.open)
		raw, shape, end, err := shapeBody(s, body, sh.closes, sh.shapes, line)
		if err != nil {
			return "", 0, err
		}
		lbl, err := label(raw, line, p.lim)
		if err != nil {
			return "", 0, err
		}
		m.label, m.shape, m.hasShape = lbl, shape, true
		i = end
		break
	}
	if strings.HasPrefix(s[i:], ":::") {
		// A class reference: styling, ignored like classDef.
		i += 3
		i += idLen(s, i)
	}
	return name, i, nil
}

// shapeBody reads a node's text up to its closing delimiter.
func shapeBody(s string, i int, closes []string, kinds []Shape, line int) (string, Shape, int, error) {
	j := skipSpace(s, i)
	if j < len(s) && s[j] == '"' {
		q := strings.IndexByte(s[j+1:], '"')
		if q < 0 {
			return "", 0, 0, refuse(Syntax, line, "a double quote is not closed")
		}
		after := skipSpace(s, j+1+q+1)
		for k, c := range closes {
			if strings.HasPrefix(s[after:], c) {
				return s[j : j+q+2], kinds[k], after + len(c), nil
			}
		}
		return "", 0, 0, refuse(Syntax, line, "expected %s after the quoted text", closes[0])
	}
	best, kind := -1, 0
	for k, c := range closes {
		if at := strings.Index(s[i:], c); at >= 0 && (best < 0 || at < best) {
			best, kind = at, k
		}
	}
	if best < 0 {
		return "", 0, 0, refuse(Syntax, line, "a node's text is not closed with %s", closes[0])
	}
	raw := s[i : i+best]
	if strings.ContainsAny(raw, "[](){}\"") {
		return "", 0, 0, refuse(Syntax, line, "put text containing brackets or quotes in double quotes")
	}
	return raw, kinds[kind], i + best + len(closes[kind]), nil
}

func (p *parser) mention(name string, line int) *mention {
	m := p.mentions[name]
	if m == nil {
		if len(p.order) >= p.lim.MaxNodes {
			return nil
		}
		m = &mention{name: name, sub: -1, line: line}
		p.mentions[name] = m
		p.order = append(p.order, m)
	}
	if m.sub < 0 && len(p.stack) > 0 {
		m.sub = p.stack[len(p.stack)-1]
	}
	return m
}

// resolve turns names into indexes, once the whole diagram is read: a name
// can be used as an edge's end before the subgraph it names is defined.
func (p *parser) resolve() error {
	f := p.f
	nodeIdx := map[string]int{}
	for _, m := range p.order {
		if _, isSub := p.subIndex[m.name]; isSub {
			if m.hasShape {
				return refuse(Unsupported, m.line, "%q is both a subgraph and a node", m.name)
			}
			continue
		}
		lbl := m.label
		if !m.hasShape {
			var err error
			if lbl, err = label(m.name, m.line, p.lim); err != nil {
				return err
			}
		}
		nodeIdx[m.name] = len(f.Nodes)
		f.Nodes = append(f.Nodes, Node{ID: m.name, Label: lbl, Shape: m.shape, Subgraph: m.sub})
	}
	end := func(name string) End {
		if s, ok := p.subIndex[name]; ok {
			return End{Node: -1, Subgraph: s}
		}
		return End{Node: nodeIdx[name], Subgraph: -1}
	}
	for _, pe := range p.edges {
		e := pe.edge
		e.From, e.To = end(pe.from), end(pe.to)
		if e.From.Subgraph >= 0 || e.To.Subgraph >= 0 {
			if p.encloses(e.From, e.To) || p.encloses(e.To, e.From) {
				return refuse(Unsupported, pe.line, "an edge between a subgraph and something inside it is not supported")
			}
		}
		f.Edges = append(f.Edges, e)
	}
	return nil
}

// encloses reports whether end a is a subgraph containing end b.
func (p *parser) encloses(a, b End) bool {
	if a.Subgraph < 0 {
		return false
	}
	inner := b.Subgraph
	if b.Node >= 0 {
		inner = p.f.Nodes[b.Node].Subgraph
	}
	return p.f.ancestor(a.Subgraph, inner)
}

// parseLink reads one link at s[i:]: its stroke, its heads, its length and
// its label in either form (`-- text -->` or `-->|text|`).
func parseLink(s string, i int, line int, lim Limits) (Edge, int, error) {
	e := Edge{Length: 1}
	j := i
	switch {
	case j < len(s) && s[j] == '<':
		e.Tail = HeadArrow
		j++
	case j+1 < len(s) && (s[j] == 'o' || s[j] == 'x') && strings.ContainsRune("-=.", rune(s[j+1])):
		e.Tail = headOf(s[j])
		j++
	}
	if j >= len(s) {
		return e, 0, refuse(Syntax, line, "expected a link at %q", excerpt(s[i:]))
	}
	var textOpen string // the opener of a `-- text -->` label, if this is one
	k := j
	switch s[j] {
	case '-':
		n := run(s, j, '-')
		if n == 1 && j+1 < len(s) && s[j+1] == '.' {
			d := run(s, j+1, '.')
			k = j + 1 + d
			if k < len(s) && s[k] == '-' {
				e.Line, e.Length = LineDotted, d
				k++
				e.Head, k = endHead(s, k)
			} else if d == 1 {
				e.Line, textOpen = LineDotted, "-."
			} else {
				return e, 0, refuse(Syntax, line, "malformed dotted link at %q", excerpt(s[i:]))
			}
		} else {
			e.Line = LineSolid
			k = j + n
			e.Head, k = endHead(s, k)
			if !lineLength(&e, n) {
				if n == 2 && e.Head == HeadNone {
					textOpen = "--"
				} else {
					return e, 0, refuse(Syntax, line, "malformed link at %q", excerpt(s[i:]))
				}
			}
		}
	case '=':
		n := run(s, j, '=')
		e.Line = LineThick
		k = j + n
		e.Head, k = endHead(s, k)
		if !lineLength(&e, n) {
			if n == 2 && e.Head == HeadNone {
				textOpen = "=="
			} else {
				return e, 0, refuse(Syntax, line, "malformed link at %q", excerpt(s[i:]))
			}
		}
	case '~':
		n := run(s, j, '~')
		if n < 3 || e.Tail != HeadNone {
			return e, 0, refuse(Syntax, line, "malformed invisible link at %q", excerpt(s[i:]))
		}
		e.Line, e.Length = LineInvisible, n-2
		k = j + n
	default:
		return e, 0, refuse(Syntax, line, "expected a link at %q", excerpt(s[i:]))
	}
	hasLabel := false
	if textOpen != "" {
		var raw string
		var err error
		raw, k, err = textLabel(s, j+len(textOpen), textOpen, &e, line)
		if err != nil {
			return e, 0, err
		}
		if e.Label, err = label(raw, line, lim); err != nil {
			return e, 0, err
		}
		hasLabel = true
	}
	if e.Tail != HeadNone && e.Head == HeadNone {
		return e, 0, refuse(Syntax, line, "a link with a head at its start needs one at its end")
	}
	if e.Length > lim.MaxLength {
		return e, 0, refuse(Limit, line, "a link longer than %d", lim.MaxLength)
	}
	if k2 := skipSpace(s, k); k2 < len(s) && s[k2] == '|' {
		if hasLabel || e.Line == LineInvisible {
			return e, 0, refuse(Syntax, line, "a link with two labels")
		}
		q := strings.IndexByte(s[k2+1:], '|')
		if q < 0 {
			return e, 0, refuse(Syntax, line, "a |label| is not closed")
		}
		var err error
		if e.Label, err = label(s[k2+1:k2+1+q], line, lim); err != nil {
			return e, 0, err
		}
		if len(e.Label) == 1 && e.Label[0] == "" {
			e.Label = nil
		}
		k = k2 + 1 + q + 1
	}
	return e, k, nil
}

// lineLength sets a solid or thick link's length from its run of n strokes,
// reporting false for a run too short to be a link: `-->` and `---` are
// the shortest.
func lineLength(e *Edge, n int) bool {
	if e.Head != HeadNone {
		e.Length = n - 1
		return n >= 2
	}
	e.Length = n - 2
	return n >= 3
}

// textLabel finds the end of a `-- text -->` label: the first point after
// the opener where the link closes. It sets the link's head and length from
// the closer and returns the label text and the index after the link.
func textLabel(s string, i int, open string, e *Edge, line int) (string, int, error) {
	for m := i; m < len(s); m++ {
		switch open {
		case "--", "==":
			c := open[0]
			if s[m] != c {
				continue
			}
			n := run(s, m, c)
			head, k := endHead(s, m+n)
			probe := Edge{Head: head}
			if n >= 2 && lineLength(&probe, n) {
				e.Head, e.Length = head, probe.Length
				return s[i:m], k, nil
			}
			m += n - 1
		case "-.":
			if s[m] != '.' {
				continue
			}
			d := run(s, m, '.')
			if m+d < len(s) && s[m+d] == '-' {
				e.Head, e.Length = HeadNone, d
				var k int
				e.Head, k = endHead(s, m+d+1)
				return s[i:m], k, nil
			}
			m += d - 1
		}
	}
	return "", 0, refuse(Syntax, line, "a link's text opened with %s is not closed", open)
}

// endHead reads the head at the end of a link's strokes. An o or x right
// after the strokes is a head, as it is to mermaid's lexer: `A--oB` is a
// circle to B, and `A --- oB` (with the space) a plain link to oB.
func endHead(s string, k int) (Head, int) {
	if k >= len(s) {
		return HeadNone, k
	}
	switch s[k] {
	case '>':
		return HeadArrow, k + 1
	case 'o', 'x':
		return headOf(s[k]), k + 1
	}
	return HeadNone, k
}

func headOf(c byte) Head {
	if c == 'o' {
		return HeadCircle
	}
	return HeadCross
}

func run(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

// idLen is the length of the identifier at s[i:]: letters, digits and
// underscores, with a hyphen or dot allowed between two of them, so
// `my-node` is one name and `A-->B` is two.
func idLen(s string, i int) int {
	j := i
	for j < len(s) {
		r, size := utf8.DecodeRuneInString(s[j:])
		switch {
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			j += size
		case (r == '-' || r == '.') && j > i && isIDByte(s, j+1):
			j += size
		default:
			return j - i
		}
	}
	return j - i
}

func isIDByte(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func excerpt(s string) string {
	if utf8.RuneCountInString(s) > 20 {
		r := []rune(s)
		return string(r[:20]) + "…"
	}
	return s
}

var (
	lineBreak = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlTag   = regexp.MustCompile(`<[A-Za-z/!?]`)
	faIcon    = regexp.MustCompile(`\bfa[bklrs]?:fa-`)
	entity    = regexp.MustCompile(`#([a-zA-Z]+|[0-9]+);`)
)

var namedEntities = map[string]string{
	"quot": `"`, "amp": "&", "lt": "<", "gt": ">", "nbsp": " ", "apos": "'",
}

// label turns the raw text of a label into plain-text lines. It is the one
// place source text becomes something that is drawn, and it lets through
// only text: a <br> is a line break, a mermaid entity code (#quot; or
// #35;) is its character, and any other markup — an HTML tag, a markdown
// string, a Font Awesome icon — is refused, never passed on or shown as if
// it were the text.
func label(raw string, line int, lim Limits) ([]string, error) {
	t := strings.TrimSpace(raw)
	if strings.HasPrefix(t, `"`) {
		if len(t) < 2 || !strings.HasSuffix(t, `"`) {
			return nil, refuse(Syntax, line, "a double quote is not closed")
		}
		t = t[1 : len(t)-1]
		if strings.HasPrefix(t, "`") && strings.HasSuffix(t, "`") && len(t) >= 2 {
			return nil, refuse(Unsupported, line, "markdown labels are not supported")
		}
	} else if strings.Contains(t, `"`) {
		return nil, refuse(Syntax, line, "a double quote inside unquoted text")
	}
	if faIcon.MatchString(t) {
		return nil, refuse(Unsupported, line, "icons in labels are not supported")
	}
	parts := lineBreak.Split(t, -1)
	if len(parts) > lim.MaxLines {
		return nil, refuse(Limit, line, "a label of more than %d lines", lim.MaxLines)
	}
	total := 0
	for k, part := range parts {
		if htmlTag.MatchString(part) {
			return nil, refuse(Unsupported, line, "HTML in labels is not supported (only <br>)")
		}
		part = entity.ReplaceAllStringFunc(part, decodeEntity)
		var b strings.Builder
		for _, r := range part {
			switch {
			case r == '\t':
				b.WriteByte(' ')
			case unicode.IsControl(r):
				return nil, refuse(Syntax, line, "a control character in a label")
			default:
				b.WriteRune(r)
			}
		}
		parts[k] = strings.TrimSpace(b.String())
		total += utf8.RuneCountInString(parts[k])
	}
	if total > lim.MaxLabel {
		return nil, refuse(Limit, line, "a label of more than %d characters", lim.MaxLabel)
	}
	return parts, nil
}

func decodeEntity(m string) string {
	name := m[1 : len(m)-1]
	if v, ok := namedEntities[name]; ok {
		return v
	}
	if n, err := strconv.Atoi(name); err == nil && n > 0 && n <= unicode.MaxRune && !unicode.IsControl(rune(n)) && utf8.ValidRune(rune(n)) {
		return string(rune(n))
	}
	return m
}
