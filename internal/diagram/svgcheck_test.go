package diagram

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
)

// The allowlist the writer is held to. An element or attribute outside it,
// anywhere in the output, fails every test that renders.
var (
	allowedElements = map[string]bool{
		"svg": true, "rect": true, "path": true, "polygon": true, "ellipse": true,
		"circle": true, "line": true, "text": true, "tspan": true,
	}
	allowedAttrs = map[string]bool{
		"xmlns": true, "width": true, "height": true, "viewBox": true,
		"font-family": true, "font-size": true, "text-anchor": true,
		"x": true, "y": true, "rx": true, "ry": true, "cx": true, "cy": true, "r": true,
		"x1": true, "y1": true, "x2": true, "y2": true, "d": true, "points": true,
		"fill": true, "stroke": true, "stroke-width": true, "stroke-dasharray": true,
	}
	// Attribute values are numbers, path data, colours and keywords: no
	// value can hold a quote, a bracket, a colon or a semicolon, so none
	// can be markup, CSS, a URL or a reference.
	plainValue = regexp.MustCompile(`^[-0-9A-Za-z#., ]*$`)
	// Geometry is held to numbers: a NaN or an infinity from a layout bug
	// would otherwise pass as a plain value.
	number      = `-?[0-9]+(\.[0-9]+)?`
	numberValue = regexp.MustCompile(`^` + number + `$`)
	numberList  = regexp.MustCompile(`^` + number + `([ ,]` + number + `)*$`)
	pathValue   = regexp.MustCompile(`^[MLCAVZ]` + `([ ,]?(` + number + `|[MLCAVZ]))*$`)
	numeric     = map[string]*regexp.Regexp{
		"width": numberValue, "height": numberValue, "x": numberValue, "y": numberValue,
		"rx": numberValue, "ry": numberValue, "cx": numberValue, "cy": numberValue, "r": numberValue,
		"x1": numberValue, "y1": numberValue, "x2": numberValue, "y2": numberValue,
		"stroke-width": numberValue, "font-size": numberValue,
		"viewBox": numberList, "points": numberList, "stroke-dasharray": numberList,
		"d": pathValue,
	}
	fixedValue = map[string]string{
		"xmlns":       "http://www.w3.org/2000/svg",
		"font-family": fontFamily,
	}
)

// checkSVG parses an SVG document and reports the first thing in it the
// writer should never have produced. It returns the document's text: the
// character data of its <tspan>s, one per line.
func checkSVG(svg []byte) ([]string, error) {
	if bytes.Contains(svg, []byte("<!")) || bytes.Contains(svg, []byte("<?")) {
		return nil, fmt.Errorf("a comment, CDATA section, doctype or processing instruction")
	}
	dec := xml.NewDecoder(bytes.NewReader(svg))
	dec.Strict = true
	var stack []string
	var texts []string
	var cur strings.Builder
	roots := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("not well-formed XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" {
				return nil, fmt.Errorf("element %s in namespace %q", name, t.Name.Space)
			}
			if !allowedElements[name] {
				return nil, fmt.Errorf("element <%s> is not allowed", name)
			}
			if len(stack) == 0 {
				roots++
				if name != "svg" || roots > 1 {
					return nil, fmt.Errorf("root element <%s>", name)
				}
			}
			if name == "tspan" && (len(stack) == 0 || stack[len(stack)-1] != "text") {
				return nil, fmt.Errorf("<tspan> outside <text>")
			}
			for _, a := range t.Attr {
				an := a.Name.Local
				if a.Name.Space != "" && !(a.Name.Space == "xmlns" || an == "xmlns") {
					return nil, fmt.Errorf("namespaced attribute %s:%s on <%s>", a.Name.Space, an, name)
				}
				if !allowedAttrs[an] {
					return nil, fmt.Errorf("attribute %s on <%s> is not allowed", an, name)
				}
				if want, ok := fixedValue[an]; ok {
					if a.Value != want {
						return nil, fmt.Errorf("attribute %s=%q", an, a.Value)
					}
				} else if re, ok := numeric[an]; ok && !re.MatchString(a.Value) {
					return nil, fmt.Errorf("attribute %s=%q on <%s> is not numeric", an, a.Value, name)
				} else if !plainValue.MatchString(a.Value) {
					return nil, fmt.Errorf("attribute %s=%q on <%s> is not a plain value", an, a.Value, name)
				}
			}
			stack = append(stack, name)
			cur.Reset()
		case xml.EndElement:
			if stack[len(stack)-1] == "tspan" {
				texts = append(texts, cur.String())
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == "tspan" {
				cur.Write(t)
			} else if len(bytes.TrimSpace(t)) > 0 {
				return nil, fmt.Errorf("text %q outside a <tspan>", string(t))
			}
		default:
			return nil, fmt.Errorf("unexpected XML token %T", tok)
		}
	}
	if roots != 1 {
		return nil, fmt.Errorf("no root element")
	}
	return texts, nil
}

// The validator itself rejects what the writer must never produce.
func TestCheckSVGRejects(t *testing.T) {
	ok := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><path d="M0,0 L1.5,-2 C1,2 3,4 5,6 Z" fill="#ffffff"/><text x="1" y="2"><tspan x="1" y="2">a &lt;b&gt;</tspan></text></svg>`
	if _, err := checkSVG([]byte(ok)); err != nil {
		t.Fatalf("a valid document fails: %v", err)
	}
	for _, bad := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" width="NaN" height="10"></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect x="-Inf" y="0"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><path d="M0,NaN L1,1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><polygon points="0,0 +Inf,1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><script>alert(1)</script></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" onload="alert(1)"></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect fill="url(#x)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">loose text</svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><!-- c --></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="10"`,
	} {
		if _, err := checkSVG([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
