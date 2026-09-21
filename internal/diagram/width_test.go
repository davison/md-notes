package diagram

import "testing"

// The width table is Noto Sans's. The spot values are FreeType's advances
// for NotoSans-Regular.ttf 2.015 at 1000 units, read independently of
// genmetrics (Pillow's ImageFont.getlength).
func TestTextMetrics(t *testing.T) {
	for r, want := range map[rune]int{' ': 260, 'A': 639, 'i': 258, 'W': 930, 'm': 935, 'é': 564, 'Ж': 905} {
		if got := advance(r); got != want {
			t.Errorf("advance(%q) = %d, want %d", r, got, want)
		}
	}
	if advance('中') != 1000 || advance('😀') != 1000 {
		t.Error("wide characters are not an em")
	}
	if advance('\u0301') != 0 {
		t.Error("a combining mark has width")
	}
	if got := wrap([]string{"Club documents and correspondence for the whole season"}); len(got) < 2 {
		t.Errorf("a long label is not wrapped: %q", got)
	}
	for _, l := range wrap([]string{"a b c d e f g h i j k l m n o p q r s t u v w x y z a b c d e f g h"}) {
		if textWidth(l) > wrapWidth {
			t.Errorf("wrapped line %q is %.0fpx", l, textWidth(l))
		}
	}
}
