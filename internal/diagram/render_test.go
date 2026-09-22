package diagram

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

func luminance(hex string) float64 {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	ch := func(c uint64) float64 {
		s := float64(c) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(v>>16&0xff) + 0.7152*ch(v>>8&0xff) + 0.0722*ch(v&0xff)
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Every text colour clears WCAG AA (4.5:1) against each fill it is drawn
// on, and every line clears the non-text 3:1 (WCAG 1.4.11) against the
// fills either side of it — so a diagram is legible in each theme,
// including on an e-ink panel that has no backlight.
func TestThemeContrast(t *testing.T) {
	for name, th := range map[string]Theme{"Light": Light, "Dark": Dark, "EInk": EInk} {
		checks := []struct {
			what   string
			fg, bg string
			min    float64
		}{
			{"node text on node fill", th.Text, th.NodeFill, 4.5},
			{"edge label on label fill", th.Text, th.LabelFill, 4.5},
			{"subgraph title on subgraph fill", th.ClusterText, th.ClusterFill, 4.5},
			{"node outline on node fill", th.NodeStroke, th.NodeFill, 3},
			{"node outline on background", th.NodeStroke, th.Background, 3},
			{"node outline on subgraph fill", th.NodeStroke, th.ClusterFill, 3},
			{"edge on background", th.Edge, th.Background, 3},
			{"edge on subgraph fill", th.Edge, th.ClusterFill, 3},
			{"subgraph outline on background", th.ClusterStroke, th.Background, 3},
		}
		for _, c := range checks {
			if r := contrast(c.fg, c.bg); r < c.min {
				t.Errorf("%s: %s is %.2f:1, want %.1f:1", name, c.what, r, c.min)
			}
		}
	}
}

func TestThemeValidation(t *testing.T) {
	bad := []func(*Theme){
		func(t *Theme) { t.Text = "red" },
		func(t *Theme) { t.NodeFill = "#FFFFFF" },
		func(t *Theme) { t.Edge = "#fff" },
		func(t *Theme) { t.ClusterFill = `#ffffff" onload="x` },
		func(t *Theme) { t.Background = "url(#x)" },
		func(t *Theme) { t.StrokeWidth = 0 },
		func(t *Theme) { t.StrokeWidth = math.NaN() },
	}
	for i, mod := range bad {
		th := Light
		mod(&th)
		_, err := Render(context.Background(), []byte("graph\nA-->B"), th)
		var r *Refusal
		if err == nil || errors.As(err, &r) {
			t.Errorf("case %d: got %v, want a theme error that is not a refusal", i, err)
		}
	}
	th := Light
	th.Background = ""
	svg, err := Render(context.Background(), []byte("graph\nA--oB"), th)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(svg), `width="100%"`) || strings.Count(string(svg), "<rect") != 2 {
		t.Errorf("a transparent theme drew a background:\n%s", svg)
	}
}

func TestRenderTheme(t *testing.T) {
	for name, th := range map[string]Theme{"Light": Light, "Dark": Dark, "EInk": EInk} {
		svg, err := Render(context.Background(), []byte("graph LR\nA[one] --> B[two]"), th)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []string{th.Background, th.NodeFill, th.NodeStroke, th.Text, th.Edge} {
			if !strings.Contains(string(svg), `"`+c+`"`) {
				t.Errorf("%s: colour %s is not used", name, c)
			}
		}
		if !strings.Contains(string(svg), `stroke-width="`+num(th.StrokeWidth)+`"`) {
			t.Errorf("%s: stroke width %v is not used", name, th.StrokeWidth)
		}
	}
}

func TestRenderRefusalIsRefusal(t *testing.T) {
	_, err := Render(context.Background(), []byte("pie\n\"a\": 1"), Light)
	var r *Refusal
	if !errors.As(err, &r) || r.Kind != Unsupported {
		t.Fatalf("got %v, want an unsupported refusal", err)
	}
}

// The caller's own context ending is not a refusal: it says nothing about
// the block.
func TestRenderCallerCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Render(ctx, randomSource(60, 90, 1), Light)
	var r *Refusal
	if err == nil || errors.As(err, &r) || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled and no refusal", err)
	}
}

func TestRenderDeadline(t *testing.T) {
	lim := DefaultLimits
	lim.Timeout = time.Millisecond
	_, err := RenderLimits(context.Background(), slowest(), Light, lim)
	var r *Refusal
	if !errors.As(err, &r) || r.Kind != Limit || !strings.Contains(r.Reason, "took longer") {
		t.Fatalf("got %v, want a deadline refusal", err)
	}
	// And the same block inside the default deadline draws.
	if _, err := Render(context.Background(), slowest(), Light); err != nil {
		t.Fatalf("default deadline: %v", err)
	}
}

// countdown is a context whose deadline passes after a set number of
// checks, so a test can say which check stops the layout without timing
// anything.
type countdown struct {
	context.Context
	left, calls int
}

func (c *countdown) Err() error {
	c.calls++
	if c.left <= 0 {
		return context.DeadlineExceeded
	}
	c.left--
	return nil
}

// The layout stops at whichever of its checks first sees the deadline —
// the first, the last, and every one between — and each of its three
// costly phases (rank, order, position) has checks of its own, so none
// runs on past a deadline. The phase a stop happened in is read from the
// error, so a phase whose check is removed is missing from the stops and
// the test fails; no timing is involved, so it cannot flake.
func TestLayoutChecksContext(t *testing.T) {
	// A graph with subgraphs, cycles and crossings, so every phase has
	// work to do.
	src := append(randomSource(40, 70, 3), []byte("subgraph s\nn1\nn2\nend\nsubgraph t\nn3\nend\n")...)
	f, err := Parse(src, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	full := &countdown{Context: context.Background(), left: 1 << 30}
	if _, err := layout(full, f, DefaultLimits); err != nil {
		t.Fatal(err)
	}
	// Title placement checks too, and when it makes room for a title the
	// position and title phases run again.
	phases := []string{"rank", "order", "position", "title"}
	stops := map[string]int{}
	var sequence []string
	for n := 0; n < full.calls; n++ {
		c := &countdown{Context: context.Background(), left: n}
		d, err := layout(c, f, DefaultLimits)
		if !errors.Is(err, context.DeadlineExceeded) || d != nil {
			t.Fatalf("deadline at check %d of %d: got %v, want the layout to stop", n+1, full.calls, err)
		}
		if c.calls != n+1 {
			t.Fatalf("deadline at check %d: the layout checked %d times", n+1, c.calls)
		}
		phase := ""
		for _, p := range phases {
			if strings.Contains(err.Error(), "layout stopped in "+p+":") {
				phase = p
			}
		}
		if phase == "" {
			t.Fatalf("deadline at check %d: %v names no phase", n+1, err)
		}
		stops[phase]++
		if len(sequence) == 0 || sequence[len(sequence)-1] != phase {
			sequence = append(sequence, phase)
		}
	}
	for _, p := range phases {
		if stops[p] == 0 {
			t.Errorf("the %s phase never checked its context (checks by phase: %v)", p, stops)
		}
	}
	ok := len(sequence) >= len(phases) && strings.Join(sequence[:len(phases)], ",") == strings.Join(phases, ",")
	for i := len(phases); ok && i < len(sequence); i++ {
		ok = sequence[i] == "position" || sequence[i] == "title"
	}
	if !ok {
		t.Errorf("the phases checked in the order %v, want %v, then only position and title", sequence, phases)
	}
}

func TestLayoutNodeLimit(t *testing.T) {
	lim := DefaultLimits
	src := []byte("graph\nA ---------> B\nB ---------> C")
	lim.MaxLayoutNodes = 3 + 2*15
	if _, err := RenderLimits(context.Background(), src, Light, lim); err != nil {
		t.Fatalf("at the bound: %v", err)
	}
	lim.MaxLayoutNodes--
	_, err := RenderLimits(context.Background(), src, Light, lim)
	var r *Refusal
	if !errors.As(err, &r) || r.Kind != Limit || !strings.Contains(r.Reason, "layout needs more than") {
		t.Fatalf("over the bound: got %v", err)
	}
}
