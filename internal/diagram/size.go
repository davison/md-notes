package diagram

import (
	"bytes"
	"encoding/xml"
	"strconv"
)

// Size reads the natural width and height of an SVG this package wrote,
// from its root element. The layout does not depend on the theme, so a
// drawing's size is the same in every palette; the reading view reserves
// the image's box with it before the drawing loads (davison/md-notes#177).
func Size(svg []byte) (w, h float64, ok bool) {
	d := xml.NewDecoder(bytes.NewReader(svg))
	for {
		tok, err := d.Token()
		if err != nil {
			return 0, 0, false
		}
		el, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}
		if el.Name.Local != "svg" {
			return 0, 0, false
		}
		for _, a := range el.Attr {
			v, err := strconv.ParseFloat(a.Value, 64)
			if err != nil || v <= 0 {
				continue
			}
			switch a.Name.Local {
			case "width":
				w = v
			case "height":
				h = v
			}
		}
		return w, h, w > 0 && h > 0
	}
}
