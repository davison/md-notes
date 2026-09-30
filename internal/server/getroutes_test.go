package server

import (
	"os"
	"regexp"
	"sort"
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

// A GET route added later fails here until somebody has read it under
// this heading and added it to auditedReads. Method-less patterns ("/",
// "/api/") are the UI bundle and the API's 404, which write nothing.
func TestEveryGETRouteHasBeenAuditedAsARead(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, m := range regexp.MustCompile(`HandleFunc\("((?:GET|HEAD) [^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		found = append(found, m[1])
	}
	sort.Strings(found)
	want := append([]string(nil), auditedReads...)
	sort.Strings(want)
	if strings.Join(found, "\n") != strings.Join(want, "\n") {
		t.Errorf("GET routes registered:\n%s\n\naudited as reads:\n%s", strings.Join(found, "\n"), strings.Join(want, "\n"))
	}
	for _, m := range regexp.MustCompile(`HandleFunc\("([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		p := m[1]
		if !strings.Contains(p, " ") && p != "/" && p != "/api/" {
			t.Errorf("route %q answers every method; name the method so a write cannot hide behind GET", p)
		}
	}
}
