package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func hashOf(src string) string {
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:16])
}

const flow = "flowchart LR\n  A[Start] --> B{ok?}\n"

// A fenced flowchart is listed with the line of the anchor the renderer
// puts before it, and the hash of exactly the text the code block shows,
// so the reading view can find the block and the route can find the
// source again. The HTML is what it was: the code block is the fallback.
func TestDiagramsListsAFlowchart(t *testing.T) {
	n := render(t, "x.md", "---\ntitle: T\n---\npara\n\n```mermaid\n"+flow+"```\n")
	want := []Diagram{{Line: 6, Hash: hashOf(flow), Source: []byte(flow)}}
	if !reflect.DeepEqual(n.Diagrams, want) {
		t.Fatalf("diagrams = %+v, want %+v", n.Diagrams, want)
	}
	wantContains(t, n.HTML,
		`<div class="line-anchor" data-line="6"></div>`+"\n"+`<pre><code class="language-mermaid">flowchart LR`)
	if !strings.Contains(string(mustJSON(t, n)), `"diagrams":[{"line":6,"hash":"`+hashOf(flow)+`"}]`) {
		t.Errorf("JSON = %s", mustJSON(t, n))
	}
}

// Every diagram entry must point at an anchor that is directly followed by
// the fenced block's <pre>, whatever the block is nested in; that is the
// whole contract the reading view relies on.
func TestDiagramsPointAtTheirCodeBlock(t *testing.T) {
	src := "# T\n\n```mermaid\n" + flow + "```\n\n- item\n\n  ```mermaid\n  graph TD\n  X-->Y\n  ```\n\n> ```mermaid\n> graph LR; P-->Q\n> ```\n\n```mermaid\n" + flow + "```\n"
	n := render(t, "x.md", src)
	if len(n.Diagrams) != 4 {
		t.Fatalf("diagrams = %+v, want 4", n.Diagrams)
	}
	doc, err := html.Parse(strings.NewReader(n.HTML))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range n.Diagrams {
		anchor := lineTargetOfNode(doc, d.Line)
		if anchor == nil || attr(anchor, "class") != "line-anchor" {
			t.Fatalf("no line anchor at %d:\n%s", d.Line, n.HTML)
		}
		pre := nextElement(anchor)
		if pre == nil || pre.Data != "pre" {
			t.Fatalf("anchor %d is not followed by a <pre>:\n%s", d.Line, n.HTML)
		}
		if got := nodeText(pre); got != string(d.Source) {
			t.Errorf("block at %d shows %q, the entry carries %q", d.Line, got, d.Source)
		}
		if d.Hash != hashOf(string(d.Source)) {
			t.Errorf("hash %s is not the hash of the source %q", d.Hash, d.Source)
		}
	}
	// Two identical blocks carry one hash; the reading view tells them apart
	// by position.
	if n.Diagrams[0].Hash != n.Diagrams[3].Hash {
		t.Errorf("identical sources hashed differently: %+v", n.Diagrams)
	}
}

// Only a block the renderer will draw is listed. A refusal known now — a
// block outside the subset, another diagram type, a parse failure, a bound
// the parser checks — never becomes an image, so it never flashes one.
func TestDiagramsSkipsWhatWillNotDraw(t *testing.T) {
	for name, block := range map[string]string{
		"another language": "```go\nflowchart LR\nA-->B\n```\n",
		"no language":      "```\nflowchart LR\nA-->B\n```\n",
		"sequence":         "```mermaid\nsequenceDiagram\nA->>B: hi\n```\n",
		"init directive":   "```mermaid\n%%{init: {\"theme\": \"dark\"}}%%\nflowchart LR\nA-->B\n```\n",
		"syntax":           "```mermaid\nflowchart LR\nA-->\n```\n",
		"empty":            "```mermaid\n```\n",
		"too many nodes":   "```mermaid\nflowchart LR\n" + manyNodes(250) + "```\n",
		"indented code":    "    flowchart LR\n    A-->B\n",
		"mermaid in a tilde fence with other words": "~~~ mermaidx\nflowchart LR\nA-->B\n~~~\n",
	} {
		t.Run(name, func(t *testing.T) {
			n := render(t, "x.md", "para\n\n"+block)
			if len(n.Diagrams) != 0 {
				t.Errorf("diagrams = %+v, want none", n.Diagrams)
			}
		})
	}
}

func manyNodes(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("N" + strconv.Itoa(i) + "\n")
	}
	return b.String()
}

// What a note writes as raw HTML is never a diagram, and cannot borrow
// anything the reading view uses to find one: the diagram list comes from
// the fenced blocks in the markdown alone, the line anchor it is matched
// against loses a forged class and data-line, and no diagram class or data-
// attribute survives the sanitiser. An <img> at the route's URL is an
// ordinary image, as any raw-file image is, and gains nothing.
func TestNoteCannotForgeADiagram(t *testing.T) {
	src := "para\n\n" +
		`<div class="line-anchor" data-line="10"></div><pre data-diagram="x" data-hash="` + hashOf(flow) + `"><code class="language-mermaid">` + "\n" +
		"flowchart LR\nA-->B\n</code></pre>\n\n" +
		`<p><img class="diagram" data-diagram="1" data-hash="h" src="/api/r/notes/diagram/x.md?h=` + hashOf(flow) + `&amp;theme=light"></p>` + "\n\n" +
		"```mermaid\n" + flow + "```\n"
	n := render(t, "x.md", src)
	want := []Diagram{{Line: 10, Hash: hashOf(flow), Source: []byte(flow)}}
	if !reflect.DeepEqual(n.Diagrams, want) {
		t.Fatalf("diagrams = %+v, want %+v", n.Diagrams, want)
	}
	wantMissing(t, n.HTML, `class="diagram"`, "data-diagram", "data-hash", `class="line-anchor" data-line="10"></div><pre data`)
	if got := strings.Count(n.HTML, `data-line="10"`); got != 1 {
		t.Errorf("%d elements carry data-line=10, want the renderer's one:\n%s", got, n.HTML)
	}
	doc, err := html.Parse(strings.NewReader(n.HTML))
	if err != nil {
		t.Fatal(err)
	}
	if pre := nextElement(lineTargetOfNode(doc, 10)); pre == nil || nodeText(pre) != flow {
		t.Errorf("the anchor at line 10 does not lead to the fenced block:\n%s", n.HTML)
	}
}

// The route finds a block again from the note on disk, with the same code
// the note render used, so the two cannot disagree about what a hash means.
func TestDiagramsAgreesWithRender(t *testing.T) {
	src := "---\ntags: [a]\n---\n# T\n\n```mermaid\n" + flow + "```\n\n```mermaid\ngraph TD; X-->Y\n```\n"
	n := render(t, "x.md", src)
	if got := r.Diagrams([]byte(src)); !reflect.DeepEqual(got, n.Diagrams) {
		t.Fatalf("Diagrams = %+v, Render listed %+v", got, n.Diagrams)
	}
	if len(n.Diagrams) != 2 {
		t.Fatalf("diagrams = %+v, want 2", n.Diagrams)
	}
}

func lineTargetOfNode(doc *html.Node, line int) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && attr(n, "data-line") == strconv.Itoa(line) && attr(n, "class") == "line-anchor" {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func nextElement(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode {
			return s
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
