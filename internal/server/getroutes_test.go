package server

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// auditedReads are the routes the daemon answers on GET, each read on
// davison/md-notes#240 and found to change nothing on disk and nothing a
// user can see — the in-memory caches and the source revision record
// aside. Under SameSite=Lax a page on another site can send the user's
// browser to any of them with the session attached, as a top-level
// navigation it cannot read; that is safe only while every one of them is
// a read (M13-R1).
var auditedReads = []string{
	"GET /api/r/{slug}/diagram/{path...}",
	"GET /api/r/{slug}/events",
	"GET /api/r/{slug}/note/{path...}",
	"GET /api/r/{slug}/raw/{path...}",
	"GET /api/r/{slug}/search",
	"GET /api/r/{slug}/source/{path...}",
	"GET /api/r/{slug}/tags",
	"GET /api/r/{slug}/tree",
	"GET /api/roots",
}

// registeredPatterns is every string-literal pattern passed to a Handle
// or HandleFunc call in the package's non-test sources, however the call
// is spelt or laid out, and in whichever file. A pattern that is not a
// literal is reported as an error, since it could not be audited.
func registeredPatterns(t *testing.T) []string {
	t.Helper()
	fset := gotoken.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != gotoken.STRING {
				t.Errorf("%s: a route pattern that is not a string literal cannot be audited", fset.Position(call.Pos()))
				return true
			}
			p, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, p)
			return true
		})
	}
	return out
}

// A GET route added later fails here until somebody has read it under
// this heading and added it to auditedReads. Method-less patterns ("/",
// "/api/") are the UI bundle and the API's 404, which write nothing.
// Review nit 3 on PR #241: the patterns come from parsing every source
// file in the package, not from grepping one.
func TestEveryGETRouteHasBeenAuditedAsARead(t *testing.T) {
	patterns := registeredPatterns(t)
	if len(patterns) < len(auditedReads) {
		t.Fatalf("found only %d route patterns: %q", len(patterns), patterns)
	}
	var found []string
	for _, p := range patterns {
		method, _, hasMethod := strings.Cut(p, " ")
		switch {
		case !hasMethod:
			if p != "/" && p != "/api/" {
				t.Errorf("route %q answers every method; name the method so a write cannot hide behind GET", p)
			}
		case method == "GET" || method == "HEAD":
			found = append(found, p)
		}
	}
	sort.Strings(found)
	want := append([]string(nil), auditedReads...)
	sort.Strings(want)
	if strings.Join(found, "\n") != strings.Join(want, "\n") {
		t.Errorf("GET routes registered:\n%s\n\naudited as reads:\n%s", strings.Join(found, "\n"), strings.Join(want, "\n"))
	}
}
