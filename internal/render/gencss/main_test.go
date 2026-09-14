package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/davison/md-notes/internal/render"
)

// appStylesheetPath and generatedPath are the files the generator reads
// and writes, from the package directory.
const (
	appStylesheetPath = "../../../ui/src/style.css"
	generatedPath     = "../../../ui/src/chroma.css"
)

// brokenTokens are the classes the dark scheme left in light-theme
// colours (davison/md-notes#54). They are named here so the regression
// is checked by name and not only by the set comparison, which would
// still pass if both schemes lost a class together.
var brokenTokens = []string{
	"nn", "gi", "vg", "no", "nc", "o", "vi", "nd", "bp", "ne", "nf", "go",
	"gp", "nv", "ni", "gd", "gr", "ge", "ow", "vc", "w", "nl", "gt",
}

// renderClassPrefix is the prefix the highlighter emits, named through
// the renderer so these tests cannot drift from the classes it writes.
func renderClassPrefix() string { return render.ClassPrefix }

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestNamedStyleRefusesASubstitute covers the silent fallback that made
// the dark block a style nobody chose: chroma's styles.Get answers a name
// it does not know with its fallback rather than with nothing.
func TestNamedStyleRefusesASubstitute(t *testing.T) {
	// chroma does ship a fallback for this name; which style it is does not
	// matter here, and naming it would tie the test to a chroma release.
	missing := "github-dark"
	if got := styles.Get(missing); got == nil || got.Name == missing {
		t.Fatalf("chroma now has a style called %q; this test has lost its subject", missing)
	}
	if _, err := namedStyle(missing); err == nil {
		t.Fatal("namedStyle accepted a style chroma does not ship")
	} else if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error should name the style asked for, got %v", err)
	}
	s, err := namedStyle(baseStyle)
	if err != nil {
		t.Fatalf("namedStyle(%q): %v", baseStyle, err)
	}
	if s.Name != baseStyle {
		t.Errorf("got style %q, want %q", s.Name, baseStyle)
	}
}

func TestContrastFollowsWCAG(t *testing.T) {
	white := chroma.MustParseColour("#ffffff")
	black := chroma.MustParseColour("#000000")
	for _, c := range []struct {
		a, b chroma.Colour
		want float64
	}{
		{white, black, 21},
		{white, white, 1},
		{chroma.MustParseColour("#767676"), white, 4.54},
		{chroma.MustParseColour("#1b1b1b"), white, 17.22},
		{chroma.MustParseColour("#445588"), chroma.MustParseColour("#1b1b1b"), 2.38},
	} {
		if got := contrast(c.a, c.b); math.Abs(got-c.want) > 0.01 {
			t.Errorf("contrast(%s, %s) = %.2f, want %.2f", c.a, c.b, got, c.want)
		}
	}
}

// TestReadableReachesTheTargetOrTheExtreme checks the property the
// stylesheet rests on: a colour comes back at or above the target ratio,
// on the far side of the background, with its hue intact — and untouched
// when it already clears the target, so the light scheme only changes
// where the source palette is below AA.
func TestReadableReachesTheTargetOrTheExtreme(t *testing.T) {
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	for _, name := range []string{
		"#445588", "#990000", "#008080", "#000000", "#999988", "#bbbbbb",
		"#dd1144", "#ffffff", "#0086b3", "#800080",
	} {
		c := chroma.MustParseColour(name)
		for _, bg := range []chroma.Colour{light, dark} {
			got := readable(c, bg, minContrast)
			if r := contrast(got, bg); r < minContrast-1e-9 {
				// Only an unreachable target may fall short, and then
				// only at the extreme the search clamps to.
				if got != chroma.MustParseColour("#ffffff") && got != chroma.MustParseColour("#000000") {
					t.Errorf("readable(%s, %s) = %s at %.2f:1, short of AA and not clamped", c, bg, got, r)
				}
				continue
			}
			wantH, wantS, _ := toHSL(c)
			gotH, gotS, _ := toHSL(got)
			if wantS > 0.02 && math.Abs(gotH-wantH) > 0.02 {
				t.Errorf("readable(%s, %s) = %s moved the hue from %.3f to %.3f", c, bg, got, wantH, gotH)
			}
			if wantS > 0.02 && gotS < 0.02 {
				t.Errorf("readable(%s, %s) = %s drained the colour", c, bg, got)
			}
		}
		// Moving a colour that has already been moved leaves it alone.
		for _, bg := range []chroma.Colour{light, dark} {
			once := readable(c, bg, minContrast)
			if twice := readable(once, bg, minContrast); twice != once {
				t.Errorf("readable(%s, %s) is not idempotent: %s then %s", c, bg, once, twice)
			}
		}
	}
}

// TestReadableClampsAnUnreachableTarget covers a saturated red, which
// cannot reach AA on a dark background without being lightened all the
// way through pink.
func TestReadableClampsAnUnreachableTarget(t *testing.T) {
	dark := chroma.MustParseColour("#1b1b1b")
	got := readable(chroma.MustParseColour("#990000"), dark, 21)
	if got != chroma.MustParseColour("#ffffff") {
		t.Errorf("an unreachable target gave %s, want white", got)
	}
	if got := readable(chroma.MustParseColour("#008080"), chroma.MustParseColour("#fbfbfa"), 21); got != chroma.MustParseColour("#000000") {
		t.Errorf("an unreachable target on a light background gave %s, want black", got)
	}
}

// TestToLuminanceBrackets checks the bisection returns a colour on the
// side of want it promises, for both directions.
func TestToLuminanceBrackets(t *testing.T) {
	c := chroma.MustParseColour("#445588")
	for _, want := range []float64{0.01, 0.1, 0.3, 0.6, 0.95} {
		up := toLuminance(c, want, true)
		if luminance(up) < want-1e-6 {
			t.Errorf("toLuminance(%s, %.2f, up) = %s at %.4f", c, want, up, luminance(up))
		}
		down := toLuminance(c, want, false)
		if luminance(down) > want+1e-6 {
			t.Errorf("toLuminance(%s, %.2f, down) = %s at %.4f", c, want, down, luminance(down))
		}
	}
}

// TestTintStaysVisible is the fix for the invisible tints. A tint carries
// its separation from the page across schemes, but never falls below the
// perceptibility floor — which is where matching the ratio alone left the
// inserted-line green, and the line highlight before it.
func TestTintStaysVisible(t *testing.T) {
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	for _, name := range []string{"#e5e5e5", "#ffdddd", "#ddffdd", "#e3d2d2"} {
		c := chroma.MustParseColour(name)
		if got := tint(c, light, light); got != c {
			t.Errorf("tint(%s) within one scheme returned %s; the light palette is above the floor", c, got)
		}
		got := tint(c, light, dark)
		if delta := luminance(got) - luminance(dark); delta < minTintDelta-1e-9 {
			t.Errorf("tint(%s) = %s, %.4f of luminance off the dark page, want %.4f", c, got, delta, minTintDelta)
		}
		// Above the floor the tint still carries the separation it had.
		want := contrast(c, light)
		if r := contrast(got, dark); r < want-0.05 {
			t.Errorf("tint(%s) = %s at %.3f:1, less separated than the %.3f:1 it had", c, got, r, want)
		}
		h, _, _ := toHSL(c)
		gh, _, _ := toHSL(got)
		if s := func() float64 { _, x, _ := toHSL(c); return x }(); s > 0.02 && math.Abs(h-gh) > 0.02 {
			t.Errorf("tint(%s) = %s moved the hue from %.3f to %.3f", c, got, h, gh)
		}
	}
	// The inserted-line tint is the one that matching the ratio left
	// invisible: #ddffdd stands 0.0365 off a near-white page and the same
	// ratio bought 0.0029 on the dark one.
	gi := tint(chroma.MustParseColour("#ddffdd"), light, dark)
	if d := luminance(gi) - luminance(dark); d < minTintDelta {
		t.Errorf("the dark inserted-line tint %s is %.4f off the page", gi, d)
	}
	// A tint already below the floor within its own scheme is lifted too.
	if got := floorTint(chroma.MustParseColour("#191919"), dark); luminance(got)-luminance(dark) < minTintDelta {
		t.Errorf("floorTint left %s indistinguishable from the page", got)
	}
}

// TestCarryableCapsTheWashOut covers the second half of the toning rule:
// a saturated hue may not be pushed so far that it stops being that hue.
// NameTag #000080 used to reach for the 16.9:1 it holds on white and land
// at #f2f2ff, which flattened YAML, HTML and XML fences to one near-white.
func TestCarryableCapsTheWashOut(t *testing.T) {
	dark := chroma.MustParseColour("#1b1b1b")
	light := chroma.MustParseColour("#fbfbfa")
	navy := chroma.MustParseColour("#000080")
	cap := carryable(navy, dark)
	if cap > 8 {
		t.Errorf("carryable(%s) = %.2f:1, too much room to keep a hue", navy, cap)
	}
	got := readable(navy, dark, math.Max(math.Min(contrast(navy, light), cap), minContrast))
	if contrast(got, dark) < minContrast {
		t.Errorf("the capped %s is %.2f:1, below AA", got, contrast(got, dark))
	}
	_, s, _ := toHSL(got)
	if s < 0.5 {
		t.Errorf("the capped %s has drained to saturation %.2f", got, s)
	}
	if chromaOf(got) < chromaOf(navy)-0.01 {
		t.Errorf("the capped %s carries less colour (%.2f) than the source (%.2f)", got, chromaOf(got), chromaOf(navy))
	}
	// A grey has no hue to lose, so black keywords still reach white.
	if c := carryable(chroma.MustParseColour("#000000"), dark); !math.IsInf(c, 1) {
		t.Errorf("carryable capped a grey at %.2f", c)
	}
}

// chromaOf is the sRGB colourfulness of a colour: the spread between its
// brightest and dimmest channel.
func chromaOf(c chroma.Colour) float64 {
	r, g, b := float64(c.Red()), float64(c.Green()), float64(c.Blue())
	return (math.Max(r, math.Max(g, b)) - math.Min(r, math.Min(g, b))) / 255
}

// TestSeparateTellsCollapsedColoursApart covers the distinctness pass on
// the pair it exists for: the AA floor pushes #009999 numbers up onto
// #008080 variables, one unit apart.
func TestSeparateTellsCollapsedColoursApart(t *testing.T) {
	base, err := namedStyle(baseStyle)
	if err != nil {
		t.Fatal(err)
	}
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	for _, bg := range []chroma.Colour{light, dark} {
		toned, err := tone(base, light, bg)
		if err != nil {
			t.Fatal(err)
		}
		number := toned.Get(chroma.LiteralNumber).Colour
		variable := toned.Get(chroma.NameVariable).Colour
		want := required(chroma.MustParseColour("#009999"), chroma.MustParseColour("#008080"))
		if got := distance(number, variable); got < want {
			t.Errorf("against %s: numbers %s and variables %s are %.3f apart, want %.3f",
				bg, number, variable, got, want)
		}
		for _, tt := range []chroma.TokenType{chroma.LiteralNumber, chroma.NameVariable} {
			if r := contrast(toned.Get(tt).Colour, bg); r < minContrast {
				t.Errorf("against %s: %s came out at %.2f:1 after separating", bg, tt, r)
			}
		}
	}
}

// TestRequiredAsksForNothingWhereTheSourceAgrees keeps the rule honest at
// its edges: identical sources never have to be told apart, and a pair
// the source barely separates is not held to more than it had.
func TestRequiredAsksForNothingWhereTheSourceAgrees(t *testing.T) {
	teal := chroma.MustParseColour("#008080")
	if got := required(teal, teal); got != 0 {
		t.Errorf("required for one colour with itself is %.3f", got)
	}
	near := chroma.MustParseColour("#008081")
	if got, src := required(teal, near), distance(teal, near); got > src {
		t.Errorf("required %.4f exceeds the %.4f the source had", got, src)
	}
	far := required(chroma.MustParseColour("#000000"), chroma.MustParseColour("#ffffff"))
	if math.Abs(far-distinctFloor) > 1e-9 {
		t.Errorf("required for black and white is %.3f, want the floor %.3f", far, distinctFloor)
	}
}

func TestCodeBackgroundsComeFromTheAppStylesheet(t *testing.T) {
	light, dark, err := codeBackgrounds(readFile(t, appStylesheetPath))
	if err != nil {
		t.Fatal(err)
	}
	if luminance(light) <= luminance(dark) {
		t.Fatalf("the light background %s is no lighter than the dark one %s", light, dark)
	}
	if !lightenAgainst(dark) || lightenAgainst(light) {
		t.Errorf("text should be lightened against %s and darkened against %s", dark, light)
	}
	// The body text colour is read the same way, so the assertions about
	// it cannot drift from what the browser paints either.
	lightFG, darkFG, err := schemeColours(readFile(t, appStylesheetPath), "fg")
	if err != nil {
		t.Fatal(err)
	}
	if contrast(lightFG, light) < minContrast || contrast(darkFG, dark) < minContrast {
		t.Errorf("the app's own body text is below AA: %s on %s, %s on %s", lightFG, light, darkFG, dark)
	}
	if _, _, err := schemeColours(readFile(t, appStylesheetPath), "no-such-property"); err == nil {
		t.Error("schemeColours invented a property the stylesheet does not declare")
	}
	for _, bad := range []struct {
		name, css string
	}{
		{"one declaration", ":root { --bg: #ffffff; }\n@media (prefers-color-scheme: dark) { :root { } }"},
		{"no dark query", ":root { --bg: #ffffff; }\n:root { --bg: #000000; }"},
		{"both before the query", ":root { --bg: #ffffff; --bg: #000000; }\n@media (prefers-color-scheme: dark) { }"},
	} {
		if _, _, err := codeBackgrounds([]byte(bad.css)); err == nil {
			t.Errorf("%s: codeBackgrounds accepted it", bad.name)
		}
	}
}

// TestGeneratedStylesheetIsCurrent keeps `go generate` honest: the
// committed stylesheet is what the generator produces from the committed
// application stylesheet, byte for byte.
func TestGeneratedStylesheetIsCurrent(t *testing.T) {
	got, err := generate(readFile(t, appStylesheetPath))
	if err != nil {
		t.Fatal(err)
	}
	if want := readFile(t, generatedPath); string(got) != string(want) {
		t.Errorf("%s is stale: run `go generate ./internal/render/...`", generatedPath)
	}
	again, err := generate(readFile(t, appStylesheetPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(got) {
		t.Error("two runs of the generator disagree")
	}
}

// TestBothSchemesStyleTheSameClasses is M4-R3: no token class may be
// styled in one scheme and left to the other scheme's colour in the
// other.
func TestBothSchemesStyleTheSameClasses(t *testing.T) {
	light, dark := blocks(readFile(t, generatedPath))
	if len(light) < 60 {
		t.Fatalf("parsed only %d light rules; is the stylesheet generated?", len(light))
	}
	for class := range light {
		if _, ok := dark[class]; !ok {
			t.Errorf("%s is styled in the light scheme only", class)
		}
	}
	for class := range dark {
		if _, ok := light[class]; !ok {
			t.Errorf("%s is styled in the dark scheme only", class)
		}
	}
	for _, token := range brokenTokens {
		class := "mdn-" + token
		if _, ok := light[class]; !ok {
			t.Errorf("%s is no longer styled in the light scheme; the check has lost its point", class)
		}
		if r, ok := dark[class]; !ok {
			t.Errorf("%s has no dark-scheme rule", class)
		} else if !r.colour.IsSet() && !r.background.IsSet() {
			t.Errorf("%s has a dark-scheme rule with no colour of its own", class)
		}
	}
}

// TestEveryColourClearsAA is the other half of M4-R3: nothing is drawn
// below the WCAG AA ratio against the background it lands on, in either
// scheme.
func TestEveryColourClearsAA(t *testing.T) {
	lightBG, darkBG, err := codeBackgrounds(readFile(t, appStylesheetPath))
	if err != nil {
		t.Fatal(err)
	}
	light, dark := blocks(readFile(t, generatedPath))
	for _, scheme := range []struct {
		name  string
		bg    chroma.Colour
		rules map[string]rule
	}{{"light", lightBG, light}, {"dark", darkBG, dark}} {
		for class, r := range scheme.rules {
			bg := scheme.bg
			if r.background.IsSet() {
				bg = r.background
			}
			if !r.colour.IsSet() {
				continue
			}
			if got := contrast(r.colour, bg); got < minContrast {
				t.Errorf("%s scheme: %s is %s on %s, %.2f:1", scheme.name, class, r.colour, bg, got)
			}
		}
	}
}

// TestVerifyRefusesAStylesheetThatFails checks the generator's own gate
// rejects every shape it exists to catch, so a future palette change
// cannot reintroduce one unnoticed.
func TestVerifyRefusesAStylesheetThatFails(t *testing.T) {
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	none := map[string]chroma.Colour{}
	both := func(light, dark string) []byte {
		return []byte(light + "\n@media (prefers-color-scheme: dark) {\n" + dark + "\n}\n")
	}
	if err := verify(both("/* K */ .mdn-chroma .mdn-k { color: #000000 }",
		"/* K */ .mdn-chroma .mdn-k { color: #ffffff }"), light, dark, none); err != nil {
		t.Fatalf("verify rejected a sound stylesheet: %v", err)
	}
	if err := verify(both("/* K */ .mdn-chroma .mdn-k { color: #000000 }\n"+
		"/* C */ .mdn-chroma .mdn-nc { color: #445588 }",
		"/* K */ .mdn-chroma .mdn-k { color: #ffffff }"), light, dark, none); err == nil {
		t.Error("verify accepted a class the dark scheme does not style")
	} else if !strings.Contains(err.Error(), "mdn-nc") {
		t.Errorf("the error should name the class, got %v", err)
	}
	if err := verify(both("/* K */ .mdn-chroma .mdn-k { color: #000000 }",
		"/* K */ .mdn-chroma .mdn-k { color: #445588 }"), light, dark, none); err == nil {
		t.Error("verify accepted dark blue on the dark code background")
	} else if !strings.Contains(err.Error(), "dark scheme") {
		t.Errorf("the error should name the scheme, got %v", err)
	}
	if err := verify(both("/* E */ .mdn-chroma .mdn-err { color: #a61717; background-color: #e3d2d2 }",
		"/* E */ .mdn-chroma .mdn-err { color: #a61717; background-color: #423131 }"), light, dark, none); err == nil {
		t.Error("verify ignored a token's own background")
	}
	// The two tints the generator replaces: the line highlight #191919 and
	// the inserted-line green #1a211a, both on #1b1b1b.
	for _, tint := range []struct{ class, colour string }{{"hl", "#191919"}, {"gi", "#1a211a"}} {
		css := both("/* T */ .mdn-chroma .mdn-"+tint.class+" { background-color: #e5e5e5 }",
			"/* T */ .mdn-chroma .mdn-"+tint.class+" { background-color: "+tint.colour+" }")
		if err := verify(css, light, dark, none); err == nil {
			t.Errorf("verify accepted %s, indistinguishable from the page", tint.colour)
		} else if !strings.Contains(err.Error(), "mdn-"+tint.class) {
			t.Errorf("the error should name the class, got %v", err)
		}
	}
	// Two classes the source palette tells apart, landing on one colour.
	source := map[string]chroma.Colour{
		"mdn-m":  chroma.MustParseColour("#009999"),
		"mdn-nv": chroma.MustParseColour("#008080"),
	}
	collapsed := both("/* M */ .mdn-chroma .mdn-m { color: #008181 }\n"+
		"/* V */ .mdn-chroma .mdn-nv { color: #008080 }",
		"/* M */ .mdn-chroma .mdn-m { color: #009999 }\n"+
			"/* V */ .mdn-chroma .mdn-nv { color: #009494 }")
	if err := verify(collapsed, light, dark, source); err == nil {
		t.Error("verify accepted numbers and variables on the same colour")
	} else if !strings.Contains(err.Error(), "mdn-m") || !strings.Contains(err.Error(), "mdn-nv") {
		t.Errorf("the error should name both classes, got %v", err)
	} else if strings.Count(err.Error(), "are ") != 2 {
		t.Errorf("both schemes should be reported, got %v", err)
	}
	apart := both("/* M */ .mdn-chroma .mdn-m { color: #008181 }\n"+
		"/* V */ .mdn-chroma .mdn-nv { color: #006b6b }",
		"/* M */ .mdn-chroma .mdn-m { color: #009999 }\n"+
			"/* V */ .mdn-chroma .mdn-nv { color: #00b9b9 }")
	if err := verify(apart, light, dark, source); err != nil {
		t.Errorf("verify rejected a separated pair: %v", err)
	}
}

// TestSourceColoursCoversTheSyntaxTokensOnly checks what the distinctness
// rule is asked about: the tokens that sit beside another token on a line,
// not the line-level furniture.
func TestSourceColoursCoversTheSyntaxTokensOnly(t *testing.T) {
	base, err := namedStyle(baseStyle)
	if err != nil {
		t.Fatal(err)
	}
	source := sourceColours(base)
	for _, class := range []string{"mdn-k", "mdn-nc", "mdn-nv", "mdn-m", "mdn-s", "mdn-c", "mdn-o"} {
		if _, ok := source[class]; !ok {
			t.Errorf("%s should be held to the distinctness rule", class)
		}
	}
	for _, class := range []string{"mdn-w", "mdn-ln", "mdn-lnt", "mdn-hl", "mdn-go", "mdn-gu", "mdn-gh", "mdn-err"} {
		if _, ok := source[class]; ok {
			t.Errorf("%s is line-level furniture and should be exempt", class)
		}
	}
}

// TestToneGivesBothSchemesTheSameTokens checks the coverage guarantee at
// its source rather than in the text it produces.
func TestToneGivesBothSchemesTheSameTokens(t *testing.T) {
	base, err := namedStyle(baseStyle)
	if err != nil {
		t.Fatal(err)
	}
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	a, err := tone(base, light, light)
	if err != nil {
		t.Fatal(err)
	}
	b, err := tone(base, light, dark)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Types()) != len(b.Types()) {
		t.Fatalf("light styles %d token types, dark %d", len(a.Types()), len(b.Types()))
	}
	for _, tt := range a.Types() {
		if !b.Has(tt) {
			t.Errorf("%s is styled in the light scheme only", tt)
		}
	}
	// The light scheme is the source palette with only its sub-AA colours
	// moved, and the one pair the AA floor brought together, so everything
	// else is untouched.
	for _, tt := range []chroma.TokenType{
		chroma.Keyword, chroma.LiteralString, chroma.NameClass, chroma.NameNamespace,
		chroma.NameDecorator, chroma.NameTag, chroma.NameException, chroma.GenericPrompt,
	} {
		if got, want := a.Get(tt).Colour, base.Get(tt).Colour; got != want {
			t.Errorf("%s changed in the light scheme: %s, want %s", tt, got, want)
		}
	}
}

// TestStyledTypesMatchesTheFormatter guards the assumption that the set
// tone() rebuilds is the set chroma's CSS writer emits a rule for.
func TestStyledTypesMatchesTheFormatter(t *testing.T) {
	base, err := namedStyle(baseStyle)
	if err != nil {
		t.Fatal(err)
	}
	types := styledTypes(base)
	if len(types) < 60 {
		t.Fatalf("styledTypes found %d types", len(types))
	}
	seen := map[string]bool{}
	for _, tt := range types {
		seen[fmt.Sprint(tt)] = true
	}
	for _, want := range []chroma.TokenType{
		chroma.NameClass, chroma.NameFunction, chroma.Operator, chroma.NameConstant,
		chroma.NameVariable, chroma.LineHighlight, chroma.LineNumbers, chroma.Error,
	} {
		if !seen[fmt.Sprint(want)] {
			t.Errorf("styledTypes missed %s", want)
		}
	}
}

// TestTheCommittedStylesheetPassesTheGeneratorsChecks runs the gate the
// generator applies before writing against the file that is checked in,
// so a hand edit is caught by the same rule as a regeneration.
func TestTheCommittedStylesheetPassesTheGeneratorsChecks(t *testing.T) {
	lightBG, darkBG, err := codeBackgrounds(readFile(t, appStylesheetPath))
	if err != nil {
		t.Fatal(err)
	}
	base, err := namedStyle(baseStyle)
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(readFile(t, generatedPath), lightBG, darkBG, sourceColours(base)); err != nil {
		t.Error(err)
	}
}

// TestTheDarkSchemeKeepsItsColours names the three regressions the review
// of this work turned up, so the committed stylesheet is checked for them
// by name and not only by the rules that catch their shape.
func TestTheDarkSchemeKeepsItsColours(t *testing.T) {
	_, darkBG, err := codeBackgrounds(readFile(t, appStylesheetPath))
	if err != nil {
		t.Fatal(err)
	}
	light, dark := blocks(readFile(t, generatedPath))

	// The inserted-line tint has to be as visible as the deleted-line one;
	// matching the light ratio left it 0.003 of luminance off the page.
	for _, class := range []string{"mdn-gi", "mdn-gd", "mdn-hl"} {
		r, ok := dark[class]
		if !ok || !r.background.IsSet() {
			t.Fatalf("%s has no dark background", class)
		}
		if d := luminance(r.background) - luminance(darkBG); d < minTintDelta {
			t.Errorf("the dark %s tint %s is %.4f of luminance off the page, want %.4f",
				class, r.background, d, minTintDelta)
		}
	}

	// NameTag carries YAML keys, HTML and XML tags. Reaching for the
	// contrast navy holds on white washed it out to #f2f2ff.
	tag := dark["mdn-nt"].colour
	if !tag.IsSet() {
		t.Fatal("mdn-nt has no dark colour")
	}
	if chromaOf(tag) < chromaOf(light["mdn-nt"].colour)-0.01 {
		t.Errorf("the dark tag colour %s carries less colour than the light %s",
			tag, light["mdn-nt"].colour)
	}
	_, body, err := schemeColours(readFile(t, appStylesheetPath), "fg")
	if err != nil {
		t.Fatal(err)
	}
	if distance(tag, body) < distinctFloor {
		t.Errorf("the dark tag colour %s is %.3f from the body text %s; a YAML fence would read as one colour",
			tag, distance(tag, body), body)
	}

	// Numbers and variables, the pair the AA floor brought together.
	for name, rules := range map[string]map[string]rule{"light": light, "dark": dark} {
		want := required(chroma.MustParseColour("#009999"), chroma.MustParseColour("#008080"))
		if got := distance(rules["mdn-m"].colour, rules["mdn-nv"].colour); got < want {
			t.Errorf("%s scheme: numbers %s and variables %s are %.3f apart, want %.3f",
				name, rules["mdn-m"].colour, rules["mdn-nv"].colour, got, want)
		}
	}
}

// TestTheDarkBlockFollowsTheLightOverride is the half of M4-R5 that lives
// here: a reader who overrides a dark device to light gets the light code
// colours with it. The dark rules are scoped so they stop applying, and
// the light rules are not, so they are what is left.
func TestTheDarkBlockFollowsTheLightOverride(t *testing.T) {
	css := string(readFile(t, generatedPath))
	cut := strings.Index(css, "@media")
	if cut < 0 {
		t.Fatal("no media query in the generated stylesheet")
	}
	light, dark := 0, 0
	for _, line := range strings.Split(css[:cut], "\n") {
		if !strings.Contains(line, "chroma .") {
			continue
		}
		light++
		if strings.Contains(line, darkScope) {
			t.Errorf("a light rule is scoped to the override: %s", line)
		}
	}
	for _, line := range strings.Split(css[cut:], "\n") {
		if !strings.Contains(line, "chroma .") {
			continue
		}
		dark++
		if !strings.Contains(line, darkScope+" ."+renderClassPrefix()+"chroma ") {
			t.Errorf("a dark rule escapes the override: %s", line)
		}
	}
	if light == 0 || light != dark {
		t.Fatalf("parsed %d light and %d dark rules", light, dark)
	}
}

// TestTheDarkScopeMatchesTheAppStylesheet holds the two halves of the
// override together: the colours the generator writes and the palette the
// application stylesheet declares have to stop applying at the same
// moment, or the override leaves light paper under dark text.
func TestTheDarkScopeMatchesTheAppStylesheet(t *testing.T) {
	app := string(readFile(t, appStylesheetPath))
	at := darkPattern.FindStringIndex(app)
	if at == nil {
		t.Fatal("no dark colour scheme media query in the application stylesheet")
	}
	end := strings.Index(app[at[1]:], "--bg:")
	if end < 0 {
		t.Fatal("no --bg inside the dark media query")
	}
	if selector := app[at[1] : at[1]+end]; !strings.Contains(selector, darkScope) {
		t.Errorf("the app's dark palette is declared on %q, which does not carry %s",
			strings.TrimSpace(selector), darkScope)
	}
}

// TestScopedRefusesALineItCannotScope covers the refusal: a dark rule that
// slipped past the prefix would keep its dark colour under the override,
// and silence is how that would reach a reader.
func TestScopedRefusesALineItCannotScope(t *testing.T) {
	in := []byte("/* Keyword */ ." + renderClassPrefix() + "chroma ." + renderClassPrefix() + "k { color: #ffffff }\n")
	out, err := scoped(in, darkScope)
	if err != nil {
		t.Fatalf("scoped: %v", err)
	}
	if want := "/* Keyword */ " + darkScope + " ."; !strings.HasPrefix(string(out), want) {
		t.Errorf("got %q, want it to start %q", out, want)
	}
	if _, err := scoped([]byte("html { color: red }\n"), darkScope); err == nil {
		t.Error("scoped accepted a rule it could not scope")
	}
}
