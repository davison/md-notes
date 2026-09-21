package diagram

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Flowchart {
	t.Helper()
	f, err := Parse([]byte(src), DefaultLimits)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return f
}

func TestParseDirections(t *testing.T) {
	for src, want := range map[string]Direction{
		"flowchart":    TopBottom,
		"flowchart TB": TopBottom,
		"flowchart TD": TopBottom,
		"graph BT":     BottomTop,
		"graph LR":     LeftRight,
		"flowchart RL": RightLeft,
		"graph lr":     LeftRight,
	} {
		if got := mustParse(t, src+"\nA").Direction; got != want {
			t.Errorf("%q: direction %v, want %v", src, got, want)
		}
	}
}

func TestParseShapes(t *testing.T) {
	cases := map[string]struct {
		shape Shape
		label string
	}{
		"A":                   {ShapeRect, "A"},
		"A[rect]":             {ShapeRect, "rect"},
		"A(round)":            {ShapeRound, "round"},
		"A([stadium])":        {ShapeStadium, "stadium"},
		"A[[sub]]":            {ShapeSubroutine, "sub"},
		"A[(db)]":             {ShapeCylinder, "db"},
		"A((c))":              {ShapeCircle, "c"},
		"A(((d)))":            {ShapeDoubleCircle, "d"},
		"A>flag]":             {ShapeAsymmetric, "flag"},
		"A{choice}":           {ShapeRhombus, "choice"},
		"A{{hex}}":            {ShapeHexagon, "hex"},
		"A[/lean/]":           {ShapeParallelogram, "lean"},
		`A[\lean\]`:           {ShapeParallelogramAlt, "lean"},
		`A[/trap\]`:           {ShapeTrapezoid, "trap"},
		`A[\trap/]`:           {ShapeTrapezoidAlt, "trap"},
		`A["quoted (x) [y]"]`: {ShapeRect, "quoted (x) [y]"},
		`A("q")`:              {ShapeRound, "q"},
		`A[ spaced ]`:         {ShapeRect, "spaced"},
		"A[rect]:::important": {ShapeRect, "rect"},
		"my-node.v2[dotted]":  {ShapeRect, "dotted"},
	}
	for src, want := range cases {
		f := mustParse(t, "flowchart\n"+src)
		if len(f.Nodes) != 1 {
			t.Fatalf("%q: %d nodes", src, len(f.Nodes))
		}
		n := f.Nodes[0]
		if n.Shape != want.shape || !reflect.DeepEqual(n.Label, []string{want.label}) {
			t.Errorf("%q: shape %v label %q, want %v %q", src, n.Shape, n.Label, want.shape, want.label)
		}
	}
}

func TestParseLinks(t *testing.T) {
	cases := map[string]Edge{
		"A --> B":           {Line: LineSolid, Head: HeadArrow, Length: 1},
		"A-->B":             {Line: LineSolid, Head: HeadArrow, Length: 1},
		"A --- B":           {Line: LineSolid, Length: 1},
		"A ---> B":          {Line: LineSolid, Head: HeadArrow, Length: 2},
		"A ---- B":          {Line: LineSolid, Length: 2},
		"A -.- B":           {Line: LineDotted, Length: 1},
		"A -.-> B":          {Line: LineDotted, Head: HeadArrow, Length: 1},
		"A -..-> B":         {Line: LineDotted, Head: HeadArrow, Length: 2},
		"A === B":           {Line: LineThick, Length: 1},
		"A ==> B":           {Line: LineThick, Head: HeadArrow, Length: 1},
		"A ~~~ B":           {Line: LineInvisible, Length: 1},
		"A --o B":           {Line: LineSolid, Head: HeadCircle, Length: 1},
		"A --x B":           {Line: LineSolid, Head: HeadCross, Length: 1},
		"A--oB":             {Line: LineSolid, Head: HeadCircle, Length: 1},
		"A ---xB":           {Line: LineSolid, Head: HeadCross, Length: 2},
		"A <--> B":          {Line: LineSolid, Head: HeadArrow, Tail: HeadArrow, Length: 1},
		"A o--o B":          {Line: LineSolid, Head: HeadCircle, Tail: HeadCircle, Length: 1},
		"A x--x B":          {Line: LineSolid, Head: HeadCross, Tail: HeadCross, Length: 1},
		"A <-.-> B":         {Line: LineDotted, Head: HeadArrow, Tail: HeadArrow, Length: 1},
		"A <==> B":          {Line: LineThick, Head: HeadArrow, Tail: HeadArrow, Length: 1},
		"A -->|yes| B":      {Line: LineSolid, Head: HeadArrow, Length: 1, Label: []string{"yes"}},
		"A-->|yes|B":        {Line: LineSolid, Head: HeadArrow, Length: 1, Label: []string{"yes"}},
		`A -->|"a|b"| B`:    {Line: LineSolid, Head: HeadArrow, Length: 1, Label: []string{`"a`}},
		"A -- yes --> B":    {Line: LineSolid, Head: HeadArrow, Length: 1, Label: []string{"yes"}},
		"A -- re-try --> B": {Line: LineSolid, Head: HeadArrow, Length: 1, Label: []string{"re-try"}},
		"A -- no --- B":     {Line: LineSolid, Length: 1, Label: []string{"no"}},
		"A -. maybe .-> B":  {Line: LineDotted, Head: HeadArrow, Length: 1, Label: []string{"maybe"}},
		"A -. e.g. .-> B":   {Line: LineDotted, Head: HeadArrow, Length: 1, Label: []string{"e.g."}},
		"A == sure ==> B":   {Line: LineThick, Head: HeadArrow, Length: 1, Label: []string{"sure"}},
		`A -- "q" --> B`:    {Line: LineSolid, Head: HeadArrow, Length: 1, Label: []string{"q"}},
		"A ---|plain| B":    {Line: LineSolid, Length: 1, Label: []string{"plain"}},
		"A -->|| B":         {Line: LineSolid, Head: HeadArrow, Length: 1},
	}
	for src, want := range cases {
		if strings.Contains(src, `"a|b"`) {
			// A pipe label ends at the first pipe, quotes or not, as in
			// mermaid; that leaves an unclosed quote, which is refused.
			if _, err := Parse([]byte("flowchart\n"+src), DefaultLimits); err == nil {
				t.Errorf("%q: parsed, want refused", src)
			}
			continue
		}
		f := mustParse(t, "flowchart\n"+src)
		if len(f.Edges) != 1 {
			t.Fatalf("%q: %d edges", src, len(f.Edges))
		}
		got := f.Edges[0]
		want.From, want.To = End{0, -1}, End{1, -1}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q:\n got %+v\nwant %+v", src, got, want)
		}
	}
}

func TestParseChainsAndGroups(t *testing.T) {
	f := mustParse(t, "graph TD; A --> B --> C; A & B --> C & D")
	var got []string
	for _, e := range f.Edges {
		got = append(got, f.Nodes[e.From.Node].ID+">"+f.Nodes[e.To.Node].ID)
	}
	want := []string{"A>B", "B>C", "A>C", "A>D", "B>C", "B>D"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("edges %v, want %v", got, want)
	}
	if len(f.Nodes) != 4 {
		t.Errorf("%d nodes, want 4", len(f.Nodes))
	}
}

func TestParseLaterShapeWins(t *testing.T) {
	f := mustParse(t, "flowchart\nA --> B\nA{Decide}\nB --> A")
	if f.Nodes[0].Shape != ShapeRhombus || f.Nodes[0].Label[0] != "Decide" {
		t.Errorf("node A is %v %q", f.Nodes[0].Shape, f.Nodes[0].Label)
	}
}

func TestParseSubgraphs(t *testing.T) {
	f := mustParse(t, `flowchart TB
    c1-->a2
    subgraph one
      a1-->a2
    end
    subgraph two [Second group]
      direction LR
      b1
      subgraph inner["Inner (nested)"]
        i1
      end
    end
    subgraph "Quoted title"
      q1
    end
    subgraph A title with spaces
      s1
    end
    one --> two`)
	type sg struct {
		id, title string
		parent    int
	}
	var got []sg
	for _, s := range f.Subgraphs {
		got = append(got, sg{s.ID, strings.Join(s.Title, "|"), s.Parent})
	}
	want := []sg{{"one", "one", -1}, {"two", "Second group", -1}, {"inner", "Inner (nested)", 1}, {"", "Quoted title", -1}, {"", "A title with spaces", -1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subgraphs\n got %+v\nwant %+v", got, want)
	}
	member := map[string]int{}
	for _, n := range f.Nodes {
		member[n.ID] = n.Subgraph
	}
	// c1 is mentioned only at the top level; a2 first at the top level and
	// then inside one, which claims it.
	wantMember := map[string]int{"c1": -1, "a2": 0, "a1": 0, "b1": 1, "i1": 2, "q1": 3, "s1": 4}
	if !reflect.DeepEqual(member, wantMember) {
		t.Errorf("membership %v, want %v", member, wantMember)
	}
	last := f.Edges[len(f.Edges)-1]
	if last.From != (End{-1, 0}) || last.To != (End{-1, 1}) {
		t.Errorf("one --> two is %+v", last)
	}
}

func TestParseLabels(t *testing.T) {
	cases := map[string][]string{
		`A["one<br>two<br/>three<BR />four"]`: {"one", "two", "three", "four"},
		`A["#quot;quoted#quot; #35; #9829;"]`: {`"quoted" # ♥`},
		`A["#unknown; stays"]`:                {"#unknown; stays"},
		`A["a < b & c > d"]`:                  {"a < b & c > d"},
		"A[tab\there]":                        {"tab here"},
		`A[""]`:                               {""},
		`A["x"] %% a comment "with quotes"`:   {"x"},
		`A["100%% sure"]`:                     {"100%% sure"},
	}
	for src, want := range cases {
		f := mustParse(t, "flowchart\n"+src)
		if !reflect.DeepEqual(f.Nodes[0].Label, want) {
			t.Errorf("%q: label %q, want %q", src, f.Nodes[0].Label, want)
		}
	}
}

func TestParseCommentsAndIgnored(t *testing.T) {
	f := mustParse(t, `%% leading comment
flowchart LR
  %% a comment line
  A --> B %% trailing
  style A fill:#f9f,stroke:#333,stroke-width:4px
  classDef green fill:#9f6
  class A,B green
  B:::green --> C
  linkStyle 0 stroke:#ff3
  click A "https://example.com" "tooltip"
  click B call callback()
`)
	if len(f.Nodes) != 3 || len(f.Edges) != 2 {
		t.Errorf("%d nodes and %d edges, want 3 and 2", len(f.Nodes), len(f.Edges))
	}
}

func TestParseRefusals(t *testing.T) {
	cases := []struct {
		src    string
		kind   Kind
		reason string
	}{
		{"", Syntax, "empty"},
		{"%% only a comment", Syntax, "empty"},
		{"sequenceDiagram\nA->>B: hi", Unsupported, "only flowchart and graph"},
		{"flowchart-elk TD\nA", Unsupported, "only flowchart and graph"},
		{"graph XY\nA", Syntax, "unknown direction"},
		{"%%{init: {'theme':'dark'}}%%\ngraph TD\nA", Unsupported, "directives"},
		{"---\ntitle: x\n---\ngraph TD\nA", Unsupported, "front matter"},
		{"graph TD\naccTitle: hi\nA", Unsupported, "accTitle"},
		{"graph TD\naccDescr { x }", Unsupported, "accDescr"},
		{"graph TD\ntitle x", Unsupported, "title"},
		{"graph TD\ndirection LR", Unsupported, "direction outside a subgraph"},
		{"graph TD\nA@{ shape: rect }", Unsupported, "@{"},
		{"graph TD\nA[\"`**bold**`\"]", Unsupported, "markdown"},
		{"graph TD\nA[\"<b>x</b>\"]", Unsupported, "HTML"},
		{"graph TD\nA[\"fa:fa-car Car\"]", Unsupported, "icons"},
		{"graph TD\nA[x (y)]", Syntax, "double quotes"},
		{"graph TD\nA[say \"hi\"]", Syntax, "quote"},
		{"graph TD\nA[unclosed", Syntax, "not closed"},
		{"graph TD\nA[\"unclosed]", Syntax, "not closed"},
		{"graph TD\nA -->", Syntax, "expected a node"},
		{"graph TD\nA --> B C", Syntax, "expected a link"},
		{"graph TD\nA -- text", Syntax, "not closed"},
		{"graph TD\nA -> B", Syntax, "malformed link"},
		{"graph TD\nA -- B", Syntax, "not closed"},
		{"graph TD\nA <--- B", Syntax, "head at its start"},
		{"graph TD\nA ~~ B", Syntax, "invisible"},
		{"graph TD\nA -->|a| B -->|b|", Syntax, "expected a node"},
		{"graph TD\nA -- x -->|y| B", Syntax, "two labels"},
		{"graph TD\nA e1@--> B", Syntax, "expected a link"},
		{"graph TD\nsubgraph\nend", Syntax, "needs a name"},
		{"graph TD\nsubgraph s\nA", Syntax, "never closed"},
		{"graph TD\nend", Syntax, "without a subgraph"},
		{"graph TD\nsubgraph s\nend\nsubgraph s\nend", Unsupported, "defined twice"},
		{"graph TD\nsubgraph s\nend\ns[box]", Unsupported, "both a subgraph and a node"},
		{"graph TD\nsubgraph s\nA\nend\ns --> A", Unsupported, "subgraph and something inside it"},
		{"graph TD\nsubgraph s\nsubgraph t\nA\nend\nend\ns --> t", Unsupported, "subgraph and something inside it"},
		{"graph TD\nA[\"x\u0007\"]", Syntax, "control character"},
		{"graph TD\n\xff", Syntax, "UTF-8"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.src), DefaultLimits)
		var r *Refusal
		if !errors.As(err, &r) {
			t.Errorf("%q: got %v, want a refusal", c.src, err)
			continue
		}
		if r.Kind != c.kind || !strings.Contains(r.Reason, c.reason) {
			t.Errorf("%q: refused %s %q, want %s containing %q", c.src, r.Kind, r.Reason, c.kind, c.reason)
		}
	}
}

func TestParseRefusalLine(t *testing.T) {
	_, err := Parse([]byte("graph TD\nA --> B\n\nB --> C[x (y)]"), DefaultLimits)
	var r *Refusal
	if !errors.As(err, &r) || r.Line != 4 {
		t.Fatalf("got %v, want a refusal on line 4", err)
	}
	if !strings.Contains(r.Error(), "line 4") {
		t.Errorf("Error() = %q", r.Error())
	}
}

// Every bound refuses at one past its value and accepts at it.
func TestParseLimits(t *testing.T) {
	lim := Limits{MaxBytes: 1000, MaxNodes: 3, MaxEdges: 2, MaxSubgraphs: 2, MaxDepth: 1, MaxLength: 2, MaxLabel: 5, MaxLines: 2, MaxLayoutNodes: 100}
	cases := []struct {
		ok, over, reason string
	}{
		{"graph\nA\nB\nC", "graph\nA\nB\nC\nD", "more than 3 nodes"},
		{"graph\nA --> B --> C", "graph\nA --> B --> C --> A", "more than 2 edges"},
		{"graph\nA & B --> C", "graph\nA & B --> C & A", "more than 2 edges"},
		{"graph\nsubgraph a\nend\nsubgraph b\nend", "graph\nsubgraph a\nend\nsubgraph b\nend\nsubgraph c\nend", "more than 2 subgraphs"},
		{"graph\nsubgraph a\nend", "graph\nsubgraph a\nsubgraph b\nend\nend", "nested more than 1 deep"},
		{"graph\nA ---> B", "graph\nA ----> B", "longer than 2"},
		{"graph\nA[abcde]", "graph\nA[abcdef]", "more than 5 characters"},
		{"graph\nA[\"ab<br>cd\"]", "graph\nA[\"a<br>b<br>c\"]", "more than 2 lines"},
		{"graph\nA\n" + strings.Repeat(" ", 1000-8), "graph\nA\n" + strings.Repeat(" ", 1000-7), "bytes; the limit is 1000"},
	}
	for _, c := range cases {
		if _, err := Parse([]byte(c.ok), lim); err != nil {
			t.Errorf("%q at the bound: %v", c.ok, err)
		}
		_, err := Parse([]byte(c.over), lim)
		var r *Refusal
		if !errors.As(err, &r) || r.Kind != Limit || !strings.Contains(r.Reason, c.reason) {
			t.Errorf("%q over the bound: got %v, want a limit refusal containing %q", c.over, err, c.reason)
		}
	}
}

func TestParseHeadOrName(t *testing.T) {
	f := mustParse(t, "flowchart\nA --- oB")
	if len(f.Nodes) != 2 || f.Nodes[1].ID != "oB" || f.Edges[0].Head != HeadNone {
		t.Errorf("A --- oB parsed as %+v %+v", f.Nodes, f.Edges)
	}
}

// A semicolon after a hex colour in a skipped styling statement ends that
// statement; it is not the end of an entity code (review of PR #173, B1).
func TestParseStylingSemicolon(t *testing.T) {
	for _, src := range []string{
		"graph TD; A-->B; style A fill:#fff; B-->C",
		"graph TD; A-->B; classDef x fill:#abc; B-->C",
		"graph TD; A-->B; style A fill:#123; B-->C",
		"graph TD; A-->B; linkStyle 0 stroke:#f00; B-->C",
	} {
		f := mustParse(t, src)
		if len(f.Nodes) != 3 || len(f.Edges) != 2 {
			t.Errorf("%q: %d nodes and %d edges, want 3 and 2", src, len(f.Nodes), len(f.Edges))
		}
	}
	// In a label, an entity code's semicolon is still part of the text.
	f := mustParse(t, "graph TD; A[\"#35; one\"] --> B[x #quot;y#quot;]; B --> C")
	if f.Nodes[0].Label[0] != "# one" || f.Nodes[1].Label[0] != `x "y"` || len(f.Edges) != 2 {
		t.Errorf("entity codes: %+v %+v", f.Nodes, f.Edges)
	}
}

// Only accTitle and accDescr are refused, not every name that begins with
// acc (review of PR #173, B2).
func TestParseAccNames(t *testing.T) {
	for _, src := range []string{"graph TD\naccount --> B", "graph TD\naccept[Accept] --> B", "graph TD\naccess & accumulator --> B"} {
		mustParse(t, src)
	}
	for _, src := range []string{"graph TD\naccTitle: x\nA", "graph TD\naccDescr: x\nA", "graph TD\naccDescr{ x }\nA", "graph TD\naccTitle x\nA"} {
		_, err := Parse([]byte(src), DefaultLimits)
		var r *Refusal
		if !errors.As(err, &r) || r.Kind != Unsupported {
			t.Errorf("%q: got %v, want unsupported", src, err)
		}
	}
}

// A diagram with nothing to draw is refused rather than drawn as an empty
// image, and a node named after a skipped keyword is refused rather than
// silently dropped (review of PR #173, nit 1).
func TestParseNothingToDraw(t *testing.T) {
	for _, c := range []struct{ src, reason string }{
		{"graph TD", "draws nothing"},
		{"graph TD\n%% just a comment\nstyle A fill:#fff", "draws nothing"},
		{"graph TD\nclass --> B", "keyword"},
		{"graph TD\nA --> B\nstyle --> C", "keyword"},
		{"graph TD\nA --> B\nclick & C", "keyword"},
		{"graph TD\nA\nclass", "keyword"},
	} {
		_, err := Parse([]byte(c.src), DefaultLimits)
		var r *Refusal
		if !errors.As(err, &r) || !strings.Contains(r.Reason, c.reason) {
			t.Errorf("%q: got %v, want a refusal containing %q", c.src, err, c.reason)
		}
	}
	mustParse(t, "graph TD\nsubgraph empty\nend")
	mustParse(t, "graph TD\nclassy --> B")
}
