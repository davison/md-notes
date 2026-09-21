package diagram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The attack vectors published against mermaid-js, one case per advisory,
// adapted to flowchart syntax where the original targeted another diagram
// type (davison/md-notes#170, the acceptance addition in
// https://github.com/davison/md-notes/issues/170#issuecomment-5765984674).
//
// Each case is either refused — and the reason is asserted, so a later
// change cannot quietly turn a refusal into a partial drawing — or drawn,
// and then the SVG must parse, hold only the writer's allowlisted elements
// and attributes (checkSVG), and carry the payload only as the character
// data of a <tspan>: present as text, absent as markup.
//
// CVE-2024-45801 has no case. It is a bypass of the DOMPurify that mermaid
// bundles to clean the HTML it generates; this package generates no HTML
// and bundles no sanitiser. Its SVG is built from a fixed element set,
// with source text escaped into character data (svg.go), so there is no
// sanitiser here to bypass.

type advisory struct {
	name string
	src  string
	// refused: the Kind and a fragment of the Reason. Empty when drawn.
	kind   Kind
	reason string
	// drawn: text that must appear in the SVG's text, and substrings that
	// must appear nowhere in its bytes.
	text   []string
	absent []string
}

const img = `<img src=x onerror=alert(1)>`

func advisories() []advisory {
	// markup is what must not be in the bytes of a drawing, whatever its
	// text: tags and the attribute forms of links and CSS.
	markup := []string{"<img", "<script", "<div", "<style", "<svg onload", "<a ", "<b>", "<i>", "<u>", "href=", "url(", "onerror=\"", "onload"}
	return []advisory{
		// CVE-2025-54881 (critical): KaTeX $$…$$ delimiters in a label
		// reached innerHTML.
		{name: "CVE-2025-54881/node label", src: "flowchart TD\nA[\"$$" + img + "$$\"]", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54881/edge label", src: "flowchart TD\nA -->|\"$$" + img + "$$\"| B", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54881/entity-encoded, drawn as text", src: "flowchart TD\nA[\"$$#lt;img src=x onerror=alert(1)#gt;$$\"]",
			text: []string{"$$" + img + "$$"}, absent: markup},
		{name: "CVE-2025-54881/KaTeX href, drawn as text", src: "flowchart TD\nA[\"$$\\href{javascript:alert(1)}{x}$$\"]",
			text: []string{`$$\href{javascript:alert(1)}{x}$$`}, absent: []string{"href=", "<a"}},

		// CVE-2025-54880 (critical): icon text passed to d3 .html(). Markup
		// in every text position the grammar has.
		{name: "CVE-2025-54880/quoted node label", src: "flowchart TD\nA[\"" + img + "\"]", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54880/unquoted node label", src: "flowchart TD\nA[" + img + "]", kind: Syntax, reason: "brackets or quotes in double quotes"},
		{name: "CVE-2025-54880/unquoted node label, no brackets", src: "flowchart TD\nA[<img src=x onerror=alert.call>]", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54880/pipe edge label", src: "flowchart TD\nA -->|" + img + "| B", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54880/text edge label", src: "flowchart TD\nA -- " + img + " --> B", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54880/subgraph title", src: "flowchart TD\nsubgraph s [\"" + img + "\"]\nA\nend", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54880/bare subgraph title", src: "flowchart TD\nsubgraph " + img + "\nA\nend", kind: Syntax, reason: "brackets in double quotes"},
		{name: "CVE-2025-54880/bare subgraph title, no brackets", src: "flowchart TD\nsubgraph <img src=x onerror=alert.call>\nA\nend", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2025-54880/node id", src: "flowchart TD\n" + img + " --> B", kind: Syntax, reason: "expected a node"},
		{name: "CVE-2025-54880/quoted node id", src: "flowchart TD\n\"" + img + "\" --> B", kind: Syntax, reason: "expected a node"},
		{name: "CVE-2025-54880/icon metadata", src: "flowchart TD\nA@{ icon: \"" + img + "\", label: \"x\" }", kind: Unsupported, reason: "@{ shape"},
		{name: "CVE-2025-54880/entity-encoded in every position, drawn as text",
			src:  "flowchart TD\nsubgraph s [\"#lt;b#gt;title#lt;/b#gt;\"]\nA[\"#lt;img src=x onerror=alert(1)#gt;\"] -->|#lt;i#gt;edge#lt;/i#gt;| B\nB -- #lt;u#gt;text#lt;/u#gt; --> C\nend",
			text: []string{img, "<b>title</b>", "<i>edge</i>", "<u>text</u>"}, absent: markup},

		// CVE-2021-43861 (high): sanitiser bypass letting a diagram run
		// script.
		{name: "CVE-2021-43861/script", src: "flowchart TD\nA[\"<script>alert(1)</script>\"]", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2021-43861/svg onload", src: "flowchart TD\nA[\"<svg onload=alert(1)>\"]", kind: Unsupported, reason: "HTML in labels"},
		{name: "CVE-2021-43861/javascript URL, drawn as text", src: "flowchart TD\nA[\"javascript:alert(1)\"]",
			text: []string{"javascript:alert(1)"}, absent: []string{"href", "<a"}},
		{name: "CVE-2021-43861/HTML entities, drawn as text", src: "flowchart TD\nA[\"&lt;script&gt;alert(1)&lt;/script&gt;\"] --> B[\"&#x3C;script&#x3E;\"]",
			text: []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&#x3C;script&#x3E;"}, absent: []string{"<script"}},
		{name: "CVE-2021-43861/mermaid entities, drawn as text", src: "flowchart TD\nA[\"#60;script#62;alert(1)#60;/script#62;\"]",
			text: []string{"<script>alert(1)</script>"}, absent: []string{"<script"}},

		// CVE-2026-41149: a classDef value escaping <style>.
		{name: "CVE-2026-41149/classDef breakout", src: "flowchart TD\nA --> B\nclassDef evil fill:#f00</style></svg><div onmouseover=alert(1)>\nclass A evil",
			text: []string{"A", "B"}, absent: append(markup, "onmouseover", "evil", "#f00", "</style", "<div")},
		{name: "CVE-2026-41149/breakout in a label", src: "flowchart TD\nA[\"</style></svg><div onmouseover=alert(1)>\"]", kind: Unsupported, reason: "HTML in labels"},

		// CVE-2026-41148, CVE-2022-31108: classDef and style CSS closing
		// the selector and adding page-wide rules.
		{name: "CVE-2026-41148/classDef selector escape", src: "flowchart TD\nA --> B\nclassDef x }*{background:url(http://x)}\nclass A x",
			text: []string{"A", "B"}, absent: append(markup, "background", "*{")},
		{name: "CVE-2022-31108/style selector escape", src: "flowchart TD\nA --> B\nstyle A fill:red;}*{background:url(http://x)}", kind: Syntax, reason: "expected a node"},
		{name: "CVE-2022-31108/style without a semicolon", src: "flowchart TD\nA --> B\nstyle A fill:red,}*{background:url(http://x)}",
			text: []string{"A", "B"}, absent: append(markup, "background", "red")},

		// CVE-2026-41159: CSS through %%{init}%%.
		{name: "CVE-2026-41159/fontFamily", src: "%%{init: {\"fontFamily\": \"x;}</style><script>alert(1)</script>\"}}%%\nflowchart TD\nA --> B", kind: Unsupported, reason: "directives are not supported"},
		{name: "CVE-2026-41159/themeCSS", src: "flowchart TD\n%%{init: {\"themeCSS\": \"* { background: url(http://x) }\"}}%%\nA --> B", kind: Unsupported, reason: "directives are not supported"},
		{name: "CVE-2026-41159/front matter", src: "---\nconfig:\n  themeCSS: \"* { background: url(http://x) }\"\n---\nflowchart TD\nA --> B", kind: Unsupported, reason: "front matter"},

		// CVE-2026-50159: sibling combinators escaping the diagram's scope,
		// through every directive that takes CSS.
		{name: "CVE-2026-50159/classDef", src: "flowchart TD\nA --> B\nclassDef x fill:red} & ~ * {display:none\nclass A x",
			text: []string{"A", "B"}, absent: append(markup, "display", "~")},
		{name: "CVE-2026-50159/style", src: "flowchart TD\nA --> B\nstyle A fill:red} & ~ * {display:none",
			text: []string{"A", "B"}, absent: append(markup, "display", "~")},
		{name: "CVE-2026-50159/linkStyle", src: "flowchart TD\nA --> B\nlinkStyle 0 stroke:red} & ~ * {display:none",
			text: []string{"A", "B"}, absent: append(markup, "display", "~")},
		{name: "CVE-2026-50159/class shorthand", src: "flowchart TD\nA:::x ~ * {display:none} --> B", kind: Syntax, reason: "invisible link"},
		{name: "CVE-2026-50159/init themeCSS", src: "%%{init: {\"themeCSS\": \"& ~ * {display:none}\"}}%%\nflowchart TD\nA --> B", kind: Unsupported, reason: "directives are not supported"},

		// CVE-2026-71437, CVE-2026-71438: prototype pollution through ids.
		// Here they are ordinary names.
		{name: "CVE-2026-71437/node ids", src: "flowchart TD\n__proto__ --> constructor --> prototype",
			text: []string{"__proto__", "constructor", "prototype"}},
		{name: "CVE-2026-71438/subgraph ids and &", src: "flowchart TD\nsubgraph __proto__\nconstructor\nend\nsubgraph prototype\nA\nend\nA & __proto__ --> B",
			text: []string{"__proto__", "prototype", "constructor", "A", "B"}},

		// click: no single CVE; it must produce no link or handler.
		{name: "click/javascript URL", src: "flowchart TD\nA --> B\nclick A \"javascript:alert(1)\" \"tip\"",
			text: []string{"A", "B"}, absent: append(markup, "javascript", "tip")},
		{name: "click/callback", src: "flowchart TD\nA --> B\nclick A call alert(1)",
			text: []string{"A", "B"}, absent: append(markup, "alert", "call")},
		{name: "click/href", src: "flowchart TD\nA --> B\nclick A href \"javascript:alert(1)\" _blank",
			text: []string{"A", "B"}, absent: append(markup, "_blank", "javascript")},
	}
}

func TestAdvisories(t *testing.T) {
	for _, a := range advisories() {
		t.Run(a.name, func(t *testing.T) {
			svg, err := Render(context.Background(), []byte(a.src), Light)
			if a.reason != "" {
				var r *Refusal
				if !errors.As(err, &r) {
					t.Fatalf("drawn, want a %s refusal (%q); err=%v", a.kind, a.reason, err)
				}
				if r.Kind != a.kind || !strings.Contains(r.Reason, a.reason) {
					t.Fatalf("refused %s %q, want %s containing %q", r.Kind, r.Reason, a.kind, a.reason)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused, want it drawn: %v", err)
			}
			assertDrawn(t, svg, a.text, a.absent)
		})
	}
}

func assertDrawn(t *testing.T, svg []byte, text, absent []string) {
	t.Helper()
	lines, err := checkSVG(svg)
	if err != nil {
		t.Fatalf("SVG fails the allowlist: %v\n%s", err, svg)
	}
	// Long lines are wrapped at spaces, so the text is read back as one
	// line: the payloads are about content, not where it breaks.
	all := strings.Join(lines, " ")
	for _, want := range text {
		if !strings.Contains(all, want) {
			t.Errorf("text %q is not in the drawing's text %q", want, lines)
		}
	}
	for _, bad := range absent {
		if strings.Contains(string(svg), bad) {
			t.Errorf("%q appears in the SVG's bytes", bad)
		}
	}
}

// The denial-of-service family (CVE-2026-41150, CVE-2026-71436,
// CVE-2026-71439): unbounded loops and counts. Each input is refused by its
// bound, and quickly: the counts are checked while parsing, before any
// layout work.
func TestAdvisoriesDoS(t *testing.T) {
	rep := func(s string, n int, sep string) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = strings.ReplaceAll(s, "%d", itoa(i))
		}
		return strings.Join(parts, sep)
	}
	// Each of these is refused in about a millisecond, while parsing or
	// before any layout work; the bound on the time is wide so a loaded
	// runner cannot fail it, and still far short of what the unbounded
	// work would take.
	const bounded = 500 * time.Millisecond
	cases := []struct {
		name, src, reason string
		within            time.Duration
	}{
		{"CVE-2026-41150/input size", "flowchart TD\nA --> B\n" + strings.Repeat("%% padding\n", 3100), "bytes; the limit is", bounded},
		{"CVE-2026-41150/node count", "flowchart TD\n" + rep("n%d", 201, "\n"), "more than 200 nodes", bounded},
		{"CVE-2026-71436/edge count", "flowchart TD\n" + strings.Repeat("a --> b\n", 401), "more than 400 edges", bounded},
		{"CVE-2026-71436/& product", "flowchart TD\n" + rep("a%d", 25, " & ") + " --> " + rep("b%d", 25, " & "), "more than 400 edges", bounded},
		{"CVE-2026-71436/long chain", "flowchart TD\n" + rep("c%d", 199, " --> ") + "\n" + rep("c%d", 199, " --> ") + "\n" + rep("c%d", 199, " --> "), "more than 400 edges", bounded},
		{"CVE-2026-71439/deep nesting", "flowchart TD\n" + rep("subgraph s%d", 9, "\n") + "\nA\n" + rep("end", 9, "\n"), "nested more than 8 deep", bounded},
		{"CVE-2026-71439/label length", "flowchart TD\nA[\"" + strings.Repeat("x", 501) + "\"]", "more than 500 characters", bounded},
		{"CVE-2026-71439/label lines", "flowchart TD\nA[\"" + strings.Repeat("x<br>", 21) + "\"]", "more than 20 lines", bounded},
		{"CVE-2026-71439/link length", "flowchart TD\nA " + strings.Repeat("-", 11) + "> B", "longer than 8", bounded},
		// Inside every count, but 25 links of length 8 in a chain put its ends
		// 400 ranks apart, and ten edges between them need a dummy node in
		// every rank between.
		{"CVE-2026-71439/layout size", "flowchart TD\n" + longChain(25) + strings.Repeat("c0 --> c25\n", 10), "layout needs more than", bounded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			_, err := Render(context.Background(), []byte(c.src), Light)
			took := time.Since(start)
			var r *Refusal
			if !errors.As(err, &r) || r.Kind != Limit || !strings.Contains(r.Reason, c.reason) {
				t.Fatalf("got %v, want a limit refusal containing %q", err, c.reason)
			}
			if took > c.within {
				t.Errorf("refused after %v, want within %v", took, c.within)
			}
		})
	}
	// A layout that hits the render deadline: a dense graph inside every
	// count, given a deadline shorter than it takes. The time allowed past
	// the deadline is generous, so a loaded runner cannot fail it;
	// TestLayoutChecksContext is the exact test that every phase of the
	// layout stops at its deadline.
	t.Run("CVE-2026-41150/render deadline", func(t *testing.T) {
		lim := DefaultLimits
		lim.Timeout = 20 * time.Millisecond
		start := time.Now()
		_, err := RenderLimits(context.Background(), slowest(), Light, lim)
		took := time.Since(start)
		var r *Refusal
		if !errors.As(err, &r) || r.Kind != Limit || !strings.Contains(r.Reason, "took longer than 20ms") {
			t.Fatalf("got %v, want a deadline refusal", err)
		}
		if took > lim.Timeout+time.Second {
			t.Errorf("refused after %v, want soon after the %v deadline", took, lim.Timeout)
		}
	})
}

// longChain links c0 to cn with links of the greatest length allowed.
func longChain(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("c" + itoa(i) + " ---------> c" + itoa(i+1) + "\n")
	}
	return b.String()
}
