package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
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
	if _, err := namedStyle("github-dark"); err == nil {
		t.Fatal("namedStyle accepted a style chroma does not ship")
	} else if !strings.Contains(err.Error(), "swapoff") {
		t.Errorf("the error should name the substitute chroma returned, got %v", err)
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

// TestTintKeepsItsSeparationFromThePage is the fix for the invisible
// line highlight: a tint one step off the light page background comes
// back one step off the dark one, not as the near-black that shares it.
func TestTintKeepsItsSeparationFromThePage(t *testing.T) {
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	for _, name := range []string{"#e5e5e5", "#ffdddd", "#ddffdd", "#e3d2d2"} {
		c := chroma.MustParseColour(name)
		if got := tint(c, light, light); got != c {
			t.Errorf("tint(%s) within one scheme returned %s", c, got)
		}
		got := tint(c, light, dark)
		if luminance(got) <= luminance(dark) {
			t.Errorf("tint(%s) for the dark scheme gave %s, no lighter than the page", c, got)
		}
		want := contrast(c, light)
		if r := contrast(got, dark); math.Abs(r-want) > 0.05 {
			t.Errorf("tint(%s) = %s at %.3f:1, want the %.3f:1 it had", c, got, r, want)
		}
	}
	// The old dark line highlight was #191919 on #1b1b1b — indistinguishable.
	if got := tint(chroma.MustParseColour("#e5e5e5"), light, dark); contrast(got, dark) < 1.1 {
		t.Errorf("the dark line highlight %s is no more visible than the one it replaces", got)
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
// rejects the two shapes it exists to catch, so a future palette change
// cannot reintroduce them unnoticed.
func TestVerifyRefusesAStylesheetThatFails(t *testing.T) {
	light := chroma.MustParseColour("#fbfbfa")
	dark := chroma.MustParseColour("#1b1b1b")
	good := "/* K */ .mdn-chroma .mdn-k { color: #000000 }\n" +
		"@media (prefers-color-scheme: dark) {\n/* K */ .mdn-chroma .mdn-k { color: #ffffff }\n}\n"
	if err := verify([]byte(good), light, dark); err != nil {
		t.Fatalf("verify rejected a sound stylesheet: %v", err)
	}
	gap := "/* K */ .mdn-chroma .mdn-k { color: #000000 }\n" +
		"/* C */ .mdn-chroma .mdn-nc { color: #445588 }\n" +
		"@media (prefers-color-scheme: dark) {\n/* K */ .mdn-chroma .mdn-k { color: #ffffff }\n}\n"
	if err := verify([]byte(gap), light, dark); err == nil {
		t.Error("verify accepted a class the dark scheme does not style")
	} else if !strings.Contains(err.Error(), "mdn-nc") {
		t.Errorf("the error should name the class, got %v", err)
	}
	dim := "/* K */ .mdn-chroma .mdn-k { color: #000000 }\n" +
		"@media (prefers-color-scheme: dark) {\n/* K */ .mdn-chroma .mdn-k { color: #445588 }\n}\n"
	if err := verify([]byte(dim), light, dark); err == nil {
		t.Error("verify accepted dark blue on the dark code background")
	} else if !strings.Contains(err.Error(), "dark scheme") {
		t.Errorf("the error should name the scheme, got %v", err)
	}
	onOwnBackground := "/* E */ .mdn-chroma .mdn-err { color: #a61717; background-color: #e3d2d2 }\n" +
		"@media (prefers-color-scheme: dark) {\n" +
		"/* E */ .mdn-chroma .mdn-err { color: #a61717; background-color: #423131 }\n}\n"
	if err := verify([]byte(onOwnBackground), light, dark); err == nil {
		t.Error("verify ignored a token's own background")
	}
	// The dark line highlight the generator replaces: #191919 on #1b1b1b,
	// a separation of 1.02 where the light highlight has 1.19.
	invisible := "/* H */ .mdn-chroma .mdn-hl { background-color: #e5e5e5 }\n" +
		"@media (prefers-color-scheme: dark) {\n" +
		"/* H */ .mdn-chroma .mdn-hl { background-color: #191919 }\n}\n"
	if err := verify([]byte(invisible), light, dark); err == nil {
		t.Error("verify accepted a line highlight indistinguishable from the page")
	} else if !strings.Contains(err.Error(), "mdn-hl") {
		t.Errorf("the error should name the class, got %v", err)
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
	// The light scheme is the source palette with only its sub-AA
	// colours moved, so the tokens that already cleared AA are untouched.
	for _, tt := range []chroma.TokenType{chroma.Keyword, chroma.LiteralString, chroma.NameClass} {
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
	if err := verify(readFile(t, generatedPath), lightBG, darkBG); err != nil {
		t.Error(err)
	}
}
