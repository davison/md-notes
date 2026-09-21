package diagram

import (
	"context"
	"testing"
)

func TestSizeReadsTheRoot(t *testing.T) {
	svg, err := Render(context.Background(), []byte("flowchart LR\n A[Start] --> B{ok?}\n"), Light)
	if err != nil {
		t.Fatal(err)
	}
	w, h, ok := Size(svg)
	if !ok || w != 185 || h != 94.6 {
		t.Errorf("Size = %v, %v, %v; want 185, 94.6", w, h, ok)
	}
	for _, bad := range []string{"", "<svg>", `<svg width="x" height="1">`, `<rect width="1" height="1"/>`, `<svg width="-1" height="2">`} {
		if _, _, ok := Size([]byte(bad)); ok {
			t.Errorf("Size(%q) ok", bad)
		}
	}
}
