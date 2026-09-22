package render

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"
)

// fences writes n one-line fenced blocks, each tagged with tag(i).
func fences(n int, tag func(int) string) []byte {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "```%s\nx := %d\n```\n\n", tag(i), i)
	}
	return []byte(b.String())
}

func sameTag(tag string) func(int) string { return func(int) string { return tag } }

func eachTagDifferent(i int) string { return fmt.Sprintf("lang%d", i) }

// A block whose tag chroma has no lexer for is shown as plain code, so it
// must cost no more than a block chroma highlights; and a tag chroma finds
// only by searching must cost about what the lexer's own name does. Both
// used to cost about 3.5 ms a block, 40 to 60 times a highlighted `go`
// block, because chroma's lookup of such a tag globs every lexer's
// filename patterns and nothing remembered the answer
// (davison/md-notes#178). The note whose every block has a different tag
// is the case remembering alone would not bound.
//
// Each bound is a ratio against blocks rendered in the same run rather
// than a wall-clock threshold, so a loaded machine slows both sides.
func TestTaggedBlocksCostNoMoreThanHighlightedOnes(t *testing.T) {
	const n = 4000
	// The quickest of a few renders, each by a fresh Renderer so that
	// nothing a previous render remembered is reused. A render already ten
	// times over its bound is not repeated.
	quickest := func(src []byte, bound time.Duration) time.Duration {
		best := time.Duration(1<<63 - 1)
		for range 3 {
			fresh := New()
			start := time.Now()
			if _, err := fresh.Render("notes", "n.md", src); err != nil {
				t.Fatal(err)
			}
			best = min(best, time.Since(start))
			if bound > 0 && best > 10*bound {
				break
			}
		}
		return best
	}
	highlighted := quickest(fences(n, sameTag("go")), 0)
	yaml := quickest(fences(n, sameTag("yaml")), 0)
	for _, c := range []struct {
		name     string
		src      []byte
		baseline string
		bound    time.Duration
	}{
		{"`mermaid`, which chroma has no lexer for", fences(n, sameTag("mermaid")), "`go`", highlighted},
		{"a different unknown tag on every block", fences(n, eachTagDifferent), "`go`", highlighted},
		// Twice, because these two are the same work and only noise
		// separates them.
		{"`yml`, which chroma finds only by searching", fences(n, sameTag("yml")), "`yaml`", 2 * yaml},
	} {
		got := quickest(c.src, c.bound)
		t.Logf("%s: %v for %d blocks, bound %v from %s blocks (%.2fx)",
			c.name, got, n, c.bound, c.baseline, float64(got)/float64(c.bound))
		if got > c.bound {
			t.Errorf("%s: %d blocks took %v, over the %v bound from the same number of %s blocks",
				c.name, n, got, c.bound, c.baseline)
		}
	}
}

// The same bound as a count of chroma's slow lookups, which no machine's
// load can move.
func TestSlowLexerLookupsAreBounded(t *testing.T) {
	fresh := New()
	lookups := func(src []byte) int64 {
		t.Helper()
		before := fresh.code.lookups.Load()
		if _, err := fresh.Render("notes", "n.md", src); err != nil {
			t.Fatal(err)
		}
		return fresh.code.lookups.Load() - before
	}
	if got := lookups(fences(2000, sameTag("mermaid"))); got != 1 {
		t.Errorf("2000 `mermaid` blocks: %d slow lookups, want 1", got)
	}
	if got := lookups(fences(2000, sameTag("mermaid"))); got != 0 {
		t.Errorf("the same note again: %d slow lookups, want 0", got)
	}
	if got := lookups(fences(2000, sameTag("go"))); got != 0 {
		t.Errorf("2000 `go` blocks: %d slow lookups, want 0", got)
	}
	if got := lookups(fences(2000, eachTagDifferent)); got != maxSlowTags {
		t.Errorf("2000 blocks with different tags: %d slow lookups, want %d", got, maxSlowTags)
	}
}

// Past maxSlowTags distinct searched-for tags, a block renders as plain
// code, and does so whatever the Renderer remembers from other notes.
func TestTagsPastTheLimitRenderPlain(t *testing.T) {
	var b strings.Builder
	for i := range maxSlowTags {
		fmt.Fprintf(&b, "```lang%d\nx\n```\n\n", i)
	}
	b.WriteString("```yml\nkey: value\n```\n")
	src := []byte(b.String())

	fresh := New()
	// yml is remembered from an earlier note, and is still over the limit.
	render := func(src []byte) string {
		t.Helper()
		n, err := fresh.Render("notes", "n.md", src)
		if err != nil {
			t.Fatal(err)
		}
		return n.HTML
	}
	wantContains(t, render([]byte("```yml\nkey: value\n```\n")), `class="`+ClassPrefix+`chroma"`)
	got := render(src)
	wantContains(t, got, `<pre><code class="language-yml">key: value`)
	wantMissing(t, got, ClassPrefix+"chroma")
}

// quickLexer is chroma's own name and alias tables, so it must agree with
// lexers.Get wherever it answers.
func TestQuickLexerAgreesWithChroma(t *testing.T) {
	for _, name := range lexers.Names(true) {
		for _, tag := range []string{name, strings.ToUpper(name), strings.ToLower(name)} {
			if l := quickLexer(tag); l != nil && l != lexers.Get(tag) {
				t.Errorf("quickLexer(%q) is %s, lexers.Get says %s", tag, l.Config().Name, lexers.Get(tag).Config().Name)
			}
		}
		if quickLexer(name) == nil {
			t.Errorf("quickLexer(%q) finds nothing for one of chroma's own names", name)
		}
	}
	// Every lexer is reachable by a quick name, so a tag chroma finds by
	// searching can always be handed to the highlighter as one.
	for _, l := range lexers.GlobalLexerRegistry.Lexers {
		if quickNameFor(l) == "" {
			t.Errorf("lexer %s has no name or alias that finds it", l.Config().Name)
		}
	}
}

// Settling the lexers first changes how long a note takes, not what it
// renders as: every note under the limit renders byte for byte as
// goldmark-highlighting renders it alone, attributes in the info string
// included.
func TestCodeRendersAsTheHighlighterAloneRendersIt(t *testing.T) {
	alone := New()
	alone.code = nil

	var tags []string
	for _, name := range lexers.Names(true) {
		if strings.EqualFold(name, "jungle") {
			// chroma v2.2.0's Jungle lexer never finishes on the sample
			// below, with or without this package's change.
			continue
		}
		tags = append(tags, name, strings.ToUpper(name))
	}
	// Found only by searching, among them all the slow ones measured on
	// davison/md-notes#190 that chroma has a lexer for, bar `el`: see
	// TestElHighlightsAsElisp.
	searched := []string{"yml", "h", "hpp", "cs", "patch", "txt", "kt", "env", "ml", "erl", "fs", "gradle", "YML", "Dockerfile", "go.mod", "cl", "lisp"}
	unknown := []string{"mermaid", "math", "csv", "log", "none", "json5", "graphviz", "a<b>&\"c"}

	notes := map[string]string{}
	var all strings.Builder
	for _, tag := range tags {
		fmt.Fprintf(&all, "```%s\nfunc main() { x := \"<a & b>\" } // 1\n# heading\n```\n\n", tag)
	}
	notes["every name and alias"] = all.String()
	for _, tag := range append(searched, unknown...) {
		notes[tag] = fmt.Sprintf("# T\n\ntext\n\n```%s\nkey: \"<v & w>\"\n- item\n```\n\n- in a list\n\n  ```%[1]s\n  nested\n  ```\n\n> ```%[1]s\n> quoted\n> ```\n", tag)
	}
	notes["untagged and indented"] = "```\nplain <b>\n```\n\n    indented\n\n~~~\ntilde\n~~~\n"
	notes["info-string attributes"] = "```go {linenos=true, hl_lines=[2], linenostart=5}\na := 1\nb := 2\n```\n\n" +
		"```yml {linenos=table}\nk: v\nj: w\n```\n\n```go {nohl}\nx := 1\n```\n\n```go extra words\ny := 2\n```\n\n" +
		"```yml {hl_lines=[1]}\nk: v\n```\n\n```mermaid {linenos=true}\ngraph TD\n```\n"
	notes["with frontmatter"] = "---\ntitle: T\n---\n```yml\nk: v\n```\n"
	notes["a mermaid flowchart"] = "```mermaid\nflowchart TD\n  A --> B\n```\n"

	for name, src := range notes {
		want, err := alone.Render("notes", "n.md", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		got, err := New().Render("notes", "n.md", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if got.HTML != want.HTML {
			t.Errorf("%s: renders differently from goldmark-highlighting alone\n got: %s\nwant: %s", name, got.HTML, want.HTML)
		}
	}
}

// `el` is the one tag whose rendering changes. Searching for it finds a
// plain EmacsLisp lexer that chroma registers and then covers with a
// wrapper under the same names, so no name reaches it; it is highlighted
// with the wrapper, as `elisp` and `emacs-lisp` always were.
func TestElHighlightsAsElisp(t *testing.T) {
	block := func(tag string) string {
		return render(t, "n.md", "```"+tag+"\n(defun f (x) (if x (message \"%s\" x) nil))\n```\n").HTML
	}
	if got, want := block("el"), block("elisp"); got != want {
		t.Errorf("`el` renders differently from `elisp`\n got: %s\nwant: %s", got, want)
	}
	wantContains(t, block("el"), ClassPrefix+"chroma")
}
