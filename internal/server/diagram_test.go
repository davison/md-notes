package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davison/md-notes/internal/diagram"
	"github.com/davison/md-notes/internal/render"
)

const testFlow = "flowchart LR\n  A[Start] --> B{ok?}\n"

// layoutRefused parses — so the note lists it — and is refused only when
// laid out: its back edges need more than the layout-node bound.
func layoutRefused() string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for i := range 149 {
		fmt.Fprintf(&b, "A%d --------> A%d\n", i, i+1)
	}
	for i := range 10 {
		fmt.Fprintf(&b, "A%d --------> A%d\n", 149-i, i)
	}
	return b.String()
}

func fence(src string) string { return "```mermaid\n" + src + "```\n" }

func writeNote(t *testing.T, base, rel, text string) {
	t.Helper()
	p := filepath.Join(base, "notes", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func diagramURL(rel, src, theme string) string {
	return "/api/r/notes/diagram/" + rel + "?h=" + render.HashSource([]byte(src)) + "&theme=" + theme
}

// countDraws wraps the server's draw function and reports how many times
// it ran.
func countDraws(s *Server) *atomic.Int32 {
	var n atomic.Int32
	inner := s.draw
	s.draw = func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error) {
		n.Add(1)
		return inner(ctx, src, theme)
	}
	return &n
}

// The route serves the SVG the renderer draws, in the theme asked for,
// with the headers that keep it an image: typed as SVG, never sniffed as
// anything else, and inert under a sandboxing policy when opened as a
// document (the #170 comment's delivery half).
func TestDiagramServesTheSVG(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", "# D\n\n"+fence(testFlow))
	bodies := map[string]string{}
	for name, theme := range map[string]diagram.Theme{"light": diagram.Light, "dark": diagram.Dark, "eink": diagram.EInk} {
		resp := do(t, ts, "GET", diagramURL("d.md", testFlow, name), "", nil)
		body := readAll(t, resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d: %s", name, resp.StatusCode, body)
		}
		want, err := diagram.Render(context.Background(), []byte(testFlow), theme)
		if err != nil {
			t.Fatal(err)
		}
		if body != string(want) {
			t.Errorf("%s: body is not the renderer's SVG for that theme", name)
		}
		for k, v := range map[string]string{
			"Content-Type":                 "image/svg+xml",
			"X-Content-Type-Options":       "nosniff",
			"Content-Security-Policy":      "default-src 'none'; sandbox",
			"Cross-Origin-Resource-Policy": "same-origin",
			"Cache-Control":                "no-cache",
		} {
			if got := resp.Header.Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", name, k, got, v)
			}
		}
		if etag := resp.Header.Get("ETag"); !strings.HasPrefix(etag, `"`) || len(etag) < 10 {
			t.Errorf("%s: ETag = %q, want a strong validator", name, etag)
		}
		bodies[name] = body
	}
	if bodies["light"] == bodies["dark"] || bodies["light"] == bodies["eink"] || bodies["dark"] == bodies["eink"] {
		t.Error("the three themes drew the same SVG")
	}
}

// A revalidation of an unchanged drawing is a 304 with no body.
func TestDiagramRevalidates(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	first := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", nil)
	etag := first.Header.Get("ETag")
	again := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", map[string]string{"If-None-Match": etag})
	if again.StatusCode != http.StatusNotModified {
		t.Fatalf("status %d, want 304", again.StatusCode)
	}
	if b := readAll(t, again.Body); b != "" {
		t.Errorf("a 304 carried a body: %q", b)
	}
	// The 304 stands in for the image, so it carries what the image does.
	for k, v := range map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      diagramCSP,
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cache-Control":                "no-cache",
		"ETag":                         etag,
	} {
		if got := again.Header.Get(k); got != v {
			t.Errorf("304: %s = %q, want %q", k, got, v)
		}
	}
}

// The hash is a key among the blocks of the note on disk, never a request
// to draw something: a hash the note no longer holds is a 404, whether the
// note changed, the block lives in another note, or the diagram was never
// anywhere.
func TestDiagramHashMustBeInTheNote(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	writeNote(t, base, "other.md", fence("graph TD; X-->Y\n"))
	if resp := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d before the edit", resp.StatusCode)
	}

	changed := "flowchart LR\n  A[Start] --> C[changed]\n"
	writeNote(t, base, "d.md", fence(changed))
	for name, url := range map[string]string{
		"the note changed":            diagramURL("d.md", testFlow, "light"),
		"the block is in another one": diagramURL("d.md", "graph TD; X-->Y\n", "light"),
		"never anywhere":              diagramURL("d.md", "graph TD; Q-->R\n", "light"),
	} {
		resp := do(t, ts, "GET", url, "", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Security-Policy"); got != diagramCSP {
			t.Errorf("%s: a refusal carries CSP %q, want %q", name, got, diagramCSP)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control %q, want no-store", name, got)
		}
	}
	if resp := do(t, ts, "GET", diagramURL("d.md", changed, "light"), "", nil); resp.StatusCode != http.StatusOK {
		t.Errorf("the new block: status %d, want 200", resp.StatusCode)
	}
}

// A block the note render never lists is not drawn here either: the
// route finds blocks with the same code.
func TestDiagramRefusesWhatTheNoteDoesNotList(t *testing.T) {
	ts, base := newTestServer(t)
	seq := "sequenceDiagram\nA->>B: hi\n"
	writeNote(t, base, "d.md", fence(seq)+"\n```go\n"+testFlow+"```\n")
	for _, src := range []string{seq, testFlow} {
		if resp := do(t, ts, "GET", diagramURL("d.md", src, "light"), "", nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%q: status %d, want 404", src, resp.StatusCode)
		}
	}
}

func TestDiagramRequestRefusals(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	// A note outside the root through a link inside it.
	if err := os.WriteFile(filepath.Join(base, "outside.md"), []byte(fence(testFlow)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside.md"), filepath.Join(base, "notes", "link.md")); err != nil {
		t.Fatal(err)
	}
	hash := render.HashSource([]byte(testFlow))
	for _, c := range []struct {
		name, url string
		status    int
	}{
		{"no theme", "/api/r/notes/diagram/d.md?h=" + hash, 400},
		{"unknown theme", "/api/r/notes/diagram/d.md?h=" + hash + "&theme=sepia", 400},
		{"no hash", "/api/r/notes/diagram/d.md?theme=light", 400},
		{"short hash", "/api/r/notes/diagram/d.md?h=" + hash[:31] + "&theme=light", 400},
		{"upper-case hash", "/api/r/notes/diagram/d.md?h=" + strings.ToUpper(hash) + "&theme=light", 400},
		{"missing note", diagramURL("gone.md", testFlow, "light"), 404},
		{"unknown root", "/api/r/nope/diagram/d.md?h=" + hash + "&theme=light", 404},
		{"not markdown", diagramURL("sub/pic.png", testFlow, "light"), 404},
		{"outside the root", diagramURL("link.md", testFlow, "light"), 403},
		// A write reaches no handler, as on every other read-only route.
		{"a write", "", 404},
	} {
		method := "GET"
		url := c.url
		if c.name == "a write" {
			method, url = "POST", diagramURL("d.md", testFlow, "light")
		}
		resp := do(t, ts, method, url, "", nil)
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d: %s", c.name, resp.StatusCode, c.status, readAll(t, resp.Body))
		}
		if c.name != "a write" && resp.Header.Get("Content-Security-Policy") != diagramCSP {
			t.Errorf("%s: no CSP on the refusal", c.name)
		}
		if c.name != "a write" && resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff on the refusal", c.name)
		}
	}
}

// A block the parser accepts and the layout refuses is a 422 with the
// renderer's reason, which the page shows as the code block; the refusal
// is cached, so a hostile block costs its layout once.
func TestDiagramDrawTimeRefusal(t *testing.T) {
	ts, base := newTestServer(t)
	src := layoutRefused()
	writeNote(t, base, "d.md", fence(src))
	draws := countDraws(serverOf(t, ts))
	for range 3 {
		resp := do(t, ts, "GET", diagramURL("d.md", src, "light"), "", nil)
		body := readAll(t, resp.Body)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status %d, want 422: %s", resp.StatusCode, body)
		}
		if !strings.Contains(body, "layout needs more than") {
			t.Errorf("body %q does not give the renderer's reason", body)
		}
		if ct := resp.Header.Get("Content-Type"); ct == "image/svg+xml" {
			t.Error("a refusal is typed as an image")
		}
	}
	if n := draws.Load(); n != 1 {
		t.Errorf("drew %d times for three requests, want 1", n)
	}
}

// A drawing is cached by source and theme: asking again draws nothing,
// another theme is drawn once, and an edit elsewhere in the note does not
// redraw a block that did not change.
func TestDiagramIsCached(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	draws := countDraws(serverOf(t, ts))
	get := func(theme string) {
		t.Helper()
		if resp := do(t, ts, "GET", diagramURL("d.md", testFlow, theme), "", nil); resp.StatusCode != 200 {
			t.Fatalf("status %d", resp.StatusCode)
		}
	}
	get("light")
	get("light")
	writeNote(t, base, "d.md", "A paragraph added above.\n\n"+fence(testFlow))
	get("light")
	if n := draws.Load(); n != 1 {
		t.Errorf("drew %d times, want 1", n)
	}
	get("dark")
	if n := draws.Load(); n != 2 {
		t.Errorf("drew %d times after a second theme, want 2", n)
	}
}

// The context ending is not a refusal and is not remembered as one.
func TestDiagramCancelledDrawIsNotCached(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	s := serverOf(t, ts)
	s.draw = func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error) {
		return nil, context.Canceled
	}
	if resp := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", resp.StatusCode)
	}
	if n := s.svgs.len(); n != 0 {
		t.Errorf("cache holds %d entries after a cancelled draw", n)
	}
}

// No more than drawSlots diagrams are drawn at once, whatever the number
// of requests.
func TestDiagramDrawsAreBounded(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.drawSlots = make(chan struct{}, 2)
	var text strings.Builder
	var srcs []string
	for i := range 6 {
		src := fmt.Sprintf("graph TD; A%d-->B\n", i)
		srcs = append(srcs, src)
		text.WriteString(fence(src) + "\n")
	}
	writeNote(t, base, "d.md", text.String())
	var running, peak atomic.Int32
	inner := s.draw
	s.draw = func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		return inner(ctx, src, theme)
	}
	var wg sync.WaitGroup
	for _, src := range srcs {
		wg.Go(func() {
			if resp := do(t, ts, "GET", diagramURL("d.md", src, "light"), "", nil); resp.StatusCode != 200 {
				t.Errorf("status %d", resp.StatusCode)
			}
		})
	}
	wg.Wait()
	if p := peak.Load(); p != 2 {
		t.Errorf("peak concurrent draws %d, want 2", p)
	}
}

// The cache is bounded by entries and by bytes, and evicts the least
// recently used first.
func TestSVGCacheIsBounded(t *testing.T) {
	d := func(n int) drawn { return drawn{svg: make([]byte, n)} }
	c := newSVGCache(1000, 3)
	c.put("a", d(10))
	c.put("b", d(10))
	c.put("c", d(10))
	c.get("a")
	c.put("d", d(10))
	if _, ok := c.get("b"); ok {
		t.Error("b, the least recently used, survived the entry bound")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.get(k); !ok {
			t.Errorf("%s was evicted", k)
		}
	}
	c.put("big", d(900))
	if c.size > 1000 {
		t.Errorf("size %d over the byte bound", c.size)
	}
	if _, ok := c.get("big"); !ok {
		t.Error("the newest entry was evicted")
	}
	c.put("huge", d(2000))
	if _, ok := c.get("huge"); ok {
		t.Error("an entry larger than the whole cache was kept")
	}
	c.put("big", d(5))
	if want := itemSize("big", d(5)); c.size > want+3*itemSize("x", d(10)) {
		t.Errorf("size %d after replacing an entry", c.size)
	}
}

// The route is behind the same guard as the note API: a cross-origin page
// on loopback is refused, and under the tailnet name nothing is served
// without a session or the token.
func TestDiagramIsGuarded(t *testing.T) {
	ts, base := newTailnetServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	url := diagramURL("d.md", testFlow, "light")
	if resp := do(t, ts, "GET", url, "", map[string]string{"Origin": "http://evil.example"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin on loopback: status %d, want 403", resp.StatusCode)
	}
	if resp := do(t, ts, "GET", url, "", map[string]string{"Host": "evil.example:7337"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Host: status %d, want 403", resp.StatusCode)
	}
	if resp := tdo(t, ts, "GET", url, "", map[string]string{"Sec-Fetch-Dest": "image", "Sec-Fetch-Mode": "no-cors"}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("tailnet without a session: status %d, want 401", resp.StatusCode)
	}
	cookie := login(t, ts, base)
	resp := tdo(t, ts, "GET", url, "", map[string]string{"Cookie": cookie, "Sec-Fetch-Dest": "image"})
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/svg+xml" {
		t.Errorf("tailnet with a session: status %d, type %q, want the SVG", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp := tdo(t, ts, "POST", url, "", map[string]string{"Cookie": cookie, "Origin": tailnetOrigin}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a write under the tailnet name: status %d, want 403", resp.StatusCode)
	}
}

// The note's JSON carries the list the reading view decorates from.
func TestNoteListsItsDiagrams(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", "# D\n\n"+fence(testFlow)+"\n"+fence("sequenceDiagram\nA->>B: x\n"))
	resp := do(t, ts, "GET", "/api/r/notes/note/d.md?sizes=1", "", nil)
	body := readAll(t, resp.Body)
	want := `"diagrams":[{"line":3,"hash":"` + render.HashSource([]byte(testFlow)) + `","width":185,"height":94.6}]`
	if !strings.Contains(body, want) {
		t.Errorf("note JSON lacks %s:\n%s", want, body)
	}
}

// countLists wraps the server's listing step and reports how often it ran.
func countLists(s *Server) *atomic.Int32 {
	var n atomic.Int32
	inner := s.buildList
	s.buildList = func(real string) ([]render.Diagram, error) {
		n.Add(1)
		return inner(real)
	}
	return &n
}

// A page asks for each of its diagrams separately. Listing the note is a
// full parse of it, so doing that per request makes one view of a note
// with N diagrams cost N parses of the whole note — a burst of cached
// requests took eleven of twelve cores in the round-one review of PR #175
// (B1). The list is kept per note, as a stat sees it, and built again only
// when the note changes.
func TestDiagramListsANoteOncePerVersion(t *testing.T) {
	ts, base := newTestServer(t)
	var text strings.Builder
	var srcs []string
	for i := range 5 {
		src := fmt.Sprintf("graph TD; A%d-->B\n", i)
		srcs = append(srcs, src)
		text.WriteString(fence(src) + "\n")
	}
	writeNote(t, base, "d.md", text.String())
	lists := countLists(serverOf(t, ts))
	for _, theme := range []string{"light", "dark", "eink"} {
		for _, src := range srcs {
			if resp := do(t, ts, "GET", diagramURL("d.md", src, theme), "", nil); resp.StatusCode != 200 {
				t.Fatalf("status %d", resp.StatusCode)
			}
		}
	}
	if n := lists.Load(); n != 1 {
		t.Errorf("listed the note %d times for 15 requests, want 1", n)
	}
	// An edit is a new version of the note, and is listed again; the old
	// hash is gone from it.
	writeNote(t, base, "d.md", fence("graph TD; Z-->Y\n"))
	if resp := do(t, ts, "GET", diagramURL("d.md", srcs[0], "light"), "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a hash the edited note no longer holds: status %d, want 404", resp.StatusCode)
	}
	if n := lists.Load(); n != 2 {
		t.Errorf("listed %d times after an edit, want 2", n)
	}
}

// Building a list — reading and parsing a note — takes a draw slot, so a
// burst over many notes queues rather than parsing them all at once.
func TestDiagramListingsAreBounded(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.drawSlots = make(chan struct{}, 2)
	for i := range 6 {
		writeNote(t, base, fmt.Sprintf("n%d.md", i), fence(testFlow))
	}
	var running, peak atomic.Int32
	inner := s.buildList
	s.buildList = func(real string) ([]render.Diagram, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		return inner(real)
	}
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			if resp := do(t, ts, "GET", diagramURL(fmt.Sprintf("n%d.md", i), testFlow, "light"), "", nil); resp.StatusCode != 200 {
				t.Errorf("status %d", resp.StatusCode)
			}
		})
	}
	wg.Wait()
	if p := peak.Load(); p != 2 {
		t.Errorf("peak concurrent listings %d, want 2", p)
	}
}
func TestListCacheIsBounded(t *testing.T) {
	l := func(n int) []render.Diagram { return []render.Diagram{{Hash: "h", Source: make([]byte, n)}} }
	k := func(p string) noteKey { return noteKey{path: p} }
	c := newListCache(1000, 2)
	c.put(k("a"), l(10))
	c.put(k("b"), l(10))
	c.get(k("a"))
	c.put(k("c"), l(10))
	if _, ok := c.get(k("b")); ok {
		t.Error("b, the least recently used, survived the entry bound")
	}
	c.put(k("big"), l(900))
	if c.size > 1000 {
		t.Errorf("size %d over the byte bound", c.size)
	}
	c.put(k("huge"), l(2000))
	if _, ok := c.get(k("huge")); ok {
		t.Error("a list larger than the whole cache was kept")
	}
}

// Some answers under the diagram path are written before its handler runs:
// the guard's refusals, and the mux's own 404 and redirect for a path it
// will not route as written. They carry the same two headers, so nothing
// answered at a diagram URL is sniffed or runs, whoever answers it
// (round-one review of PR #175, N2).
func TestDiagramPathHeadersBeforeTheHandler(t *testing.T) {
	ts, base := newTailnetServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	q := "?h=" + render.HashSource([]byte(testFlow)) + "&theme=light"
	for _, c := range []struct {
		name   string
		resp   *http.Response
		status int
	}{
		{"cross-origin on loopback", do(t, ts, "GET", "/api/r/notes/diagram/d.md"+q, "", map[string]string{"Origin": "http://evil.example"}), 403},
		{"tailnet without a session", tdo(t, ts, "GET", "/api/r/notes/diagram/d.md"+q, "", nil), 401},
		// Whoever answers this one — the mux or the handler — it is refused.
		{"a dot-dot segment", do(t, ts, "GET", "/api/r/notes/diagram/%2e%2e/d.md"+q, "", nil), 0},
		{"a doubled slash", doNoRedirect(t, ts, "/api/r/notes/diagram//d.md"+q), 307},
	} {
		if c.status == 0 && c.resp.StatusCode < 400 {
			t.Errorf("%s: status %d, want a refusal", c.name, c.resp.StatusCode)
		}
		if c.status != 0 && c.resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, c.resp.StatusCode, c.status)
		}
		if got := c.resp.Header.Get("Content-Security-Policy"); got != diagramCSP {
			t.Errorf("%s: CSP %q, want %q", c.name, got, diagramCSP)
		}
		if got := c.resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options %q, want nosniff", c.name, got)
		}
	}
	// Elsewhere the guard adds nothing.
	if got := do(t, ts, "GET", "/api/r/notes/note/d.md", "", nil).Header.Get("Content-Security-Policy"); got != "" {
		t.Errorf("the note API gained a CSP: %q", got)
	}
}

func doNoRedirect(t *testing.T, ts *httptest.Server, p string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", ts.URL+p, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:7337"
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// A panic while listing a note costs that request and nothing more: the
// draw slot it held comes back, and the next request is served. Without
// that, as many panics as there are slots would stop every diagram on the
// daemon (round-two review of PR #175, B2).
func TestDiagramListingPanicReleasesItsSlot(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.drawSlots = make(chan struct{}, 1)
	writeNote(t, base, "bad.md", fence(testFlow))
	writeNote(t, base, "good.md", fence(testFlow)+"\n")
	inner := s.buildList
	s.buildList = func(real string) ([]render.Diagram, error) {
		if filepath.Base(real) == "bad.md" {
			panic("boom")
		}
		return inner(real)
	}
	get := func(rel string) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		t.Cleanup(cancel)
		req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+diagramURL(rel, testFlow, "light"), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "localhost:7337"
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			t.Cleanup(func() { resp.Body.Close() })
		}
		return resp, err
	}
	for range 2 {
		resp, err := get("bad.md")
		if err != nil {
			t.Errorf("the panicking listing dropped the connection: %v", err)
		} else if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("the panicking listing: status %d, want 500", resp.StatusCode)
		} else if resp.Header.Get("Content-Security-Policy") != diagramCSP {
			t.Error("the 500 lacks the diagram CSP")
		}
	}
	start := time.Now()
	resp, err := get("good.md")
	if err != nil {
		t.Fatalf("another note after a listing panic: %v after %v; the slot was kept", err, time.Since(start))
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("another note after a listing panic: status %d, want 200", resp.StatusCode)
	}
}

// The note says how big each drawing is, so the reading view can reserve
// its box before the image loads and a scroll to a line below it lands
// (davison/md-notes#177). The size is the drawing's own, and the drawing
// made to measure it is the one the image route then serves.
func TestNoteSizesItsDiagrams(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", "# D\n\n"+fence(testFlow)+"\n"+fence(layoutRefused()))
	draws := countDraws(serverOf(t, ts))
	resp := do(t, ts, "GET", "/api/r/notes/note/d.md?sizes=1", "", nil)
	var note struct {
		Diagrams []struct {
			Line          int
			Hash          string
			Width, Height float64
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		t.Fatal(err)
	}
	// The layout refuses the second block, so it is not listed: it stays
	// code and no image is ever asked for.
	if len(note.Diagrams) != 1 || note.Diagrams[0].Hash != render.HashSource([]byte(testFlow)) {
		t.Fatalf("diagrams = %+v, want the drawable one alone", note.Diagrams)
	}
	for _, theme := range []string{"light", "dark", "eink"} {
		img := do(t, ts, "GET", diagramURL("d.md", testFlow, theme), "", nil)
		w, h, ok := diagram.Size([]byte(readAll(t, img.Body)))
		if !ok || w != note.Diagrams[0].Width || h != note.Diagrams[0].Height {
			t.Errorf("%s: drawing is %vx%v, the note said %vx%v", theme, w, h, note.Diagrams[0].Width, note.Diagrams[0].Height)
		}
	}
	// Two measured at note time (one drawn, one refused), then dark and
	// e-ink; light came from the cache.
	if n := draws.Load(); n != 4 {
		t.Errorf("drew %d times, want 4", n)
	}
}

// measureNote opens a note the way the reading view does, asking for its
// diagrams' sizes, and reports how long the answer took.
func measureNote(t *testing.T, ts *httptest.Server, rel string) (sized int, took time.Duration) {
	t.Helper()
	start := time.Now()
	resp := do(t, ts, "GET", "/api/r/notes/note/"+rel+"?sizes=1", "", nil)
	var note struct {
		Diagrams []struct{ Width float64 }
	}
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		t.Fatal(err)
	}
	took = time.Since(start)
	for _, d := range note.Diagrams {
		if d.Width > 0 {
			sized++
		}
	}
	return sized, took
}

func distinctFlows(prefix string, n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString(fence(fmt.Sprintf("graph TD; %s%d-->B\n", prefix, i)) + "\n")
	}
	return b.String()
}

// Sizes are kept by source hash, apart from the drawings, so opening an
// unchanged note again lays nothing out even once its drawings have been
// evicted — which a note with more diagrams than the drawing cache holds
// does to itself on every open (review of PR #181, B1).
func TestNoteSizesAreKeptApartFromTheDrawings(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.svgs = newSVGCache(1<<20, 4)
	writeNote(t, base, "d.md", distinctFlows("A", 10))
	draws := countDraws(s)
	if sized, _ := measureNote(t, ts, "d.md"); sized != 10 {
		t.Fatalf("first open sized %d of 10", sized)
	}
	first := draws.Load()
	if sized, _ := measureNote(t, ts, "d.md"); sized != 10 {
		t.Fatalf("second open sized %d of 10", sized)
	}
	if n := draws.Load() - first; n != 0 {
		t.Errorf("a warm open of an unchanged note laid out %d diagrams, want 0", n)
	}
	// An image request's draw records its size too.
	writeNote(t, base, "e.md", fence("graph TD; Q-->R\n"))
	do(t, ts, "GET", diagramURL("e.md", "graph TD; Q-->R\n", "dark"), "", nil)
	before := draws.Load()
	if sized, _ := measureNote(t, ts, "e.md"); sized != 1 {
		t.Errorf("a size the image route drew was not used")
	}
	if n := draws.Load() - before; n != 0 {
		t.Errorf("drew %d to size a block the route had drawn, want 0", n)
	}
}

// slowDraws makes every draw of a source containing marker take d.
func slowDraws(s *Server, marker string, d time.Duration) {
	inner := s.draw
	s.draw = func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error) {
		if strings.Contains(string(src), marker) {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return inner(ctx, src, theme)
	}
}

// The note's text is never held for its diagrams beyond the measuring
// budget: the blocks it does not reach go out unmeasured, and the page
// keeps its place for them itself.
func TestNoteMeasuringIsBudgeted(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.drawSlots = make(chan struct{}, 6)
	slowDraws(s, "Slow", 100*time.Millisecond)
	writeNote(t, base, "slow.md", distinctFlows("Slow", 40))
	// The same shape with nothing slow in it, for what rendering the note
	// costs anyway; the budget is on top of that.
	writeNote(t, base, "plain.md", distinctFlows("Plain", 40))
	_, plain := measureNote(t, ts, "plain.md")
	sized, took := measureNote(t, ts, "slow.md")
	if limit := plain + measureBudget + 100*time.Millisecond; took > limit {
		t.Errorf("the note took %v with 40 slow diagrams (%v without), want under %v", took, plain, limit)
	}
	if sized == 0 || sized == 40 {
		t.Errorf("sized %d of 40; the budget should reach some and not all", sized)
	}
}

// One note's measuring takes at most a couple of slots, and only for its
// budget, so another note opened meanwhile is not held behind it.
func TestNoteMeasuringDoesNotStarveOthers(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.drawSlots = make(chan struct{}, 6)
	slowDraws(s, "Slow", 100*time.Millisecond)
	writeNote(t, base, "slow.md", distinctFlows("Slow", 120))
	writeNote(t, base, "other.md", fence("graph TD; Other-->One\n"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		measureNote(t, ts, "slow.md")
	}()
	// Once the slow note's measuring holds the slots.
	for deadline := time.Now().Add(2 * time.Second); len(s.drawSlots) == 0; {
		if time.Now().After(deadline) {
			t.Fatal("the slow note never started measuring")
		}
		time.Sleep(time.Millisecond)
	}
	sized, took := measureNote(t, ts, "other.md")
	<-done
	if took > 100*time.Millisecond || sized != 1 {
		t.Errorf("another note took %v and sized %d of 1 while a slow one measured; want under 100ms, 1", took, sized)
	}
}

// A request that goes away stops its measuring: no draw starts after it.
func TestNoteMeasuringStopsWithTheRequest(t *testing.T) {
	ts, base := newTestServer(t)
	s := serverOf(t, ts)
	s.drawSlots = make(chan struct{}, 6)
	var started atomic.Int64
	var mu sync.Mutex
	var starts []time.Time
	inner := s.draw
	s.draw = func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error) {
		started.Add(1)
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		select {
		case <-time.After(40 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return inner(ctx, src, theme)
	}
	writeNote(t, base, "slow.md", distinctFlows("Slow", 40))
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/notes/note/slow.md?sizes=1", nil)
	req.Host = "localhost:7337"
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}
	gone := time.Now()
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	late := 0
	for _, at := range starts {
		if at.After(gone.Add(20 * time.Millisecond)) {
			late++
		}
	}
	if late != 0 {
		t.Errorf("%d draws started after the request went away", late)
	}
}

// Only the reading view asks for sizes. Other callers of the note
// endpoint — the editor's title, for one — lay nothing out.
func TestNoteWithoutSizesDrawsNothing(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", distinctFlows("A", 3))
	draws := countDraws(serverOf(t, ts))
	body := readAll(t, do(t, ts, "GET", "/api/r/notes/note/d.md", "", nil).Body)
	if n := draws.Load(); n != 0 {
		t.Errorf("a plain note fetch drew %d diagrams, want 0", n)
	}
	if strings.Contains(body, `"width"`) {
		t.Errorf("a plain note fetch carries sizes: %s", body)
	}
	if !strings.Contains(body, `"diagrams":[{`) {
		t.Errorf("a plain note fetch lost its diagram list: %s", body)
	}
}

// slowThenFast is a draw that the render deadline refuses until fast is
// set, and a clock the test moves by hand (davison/md-notes#182).
type slowThenFast struct {
	fast atomic.Bool
	// hang makes a draw run until its context ends, as a slow layout
	// under the note endpoint's budget does.
	hang  atomic.Bool
	draws atomic.Int32
	now   time.Time
	mu    sync.Mutex
}

func (f *slowThenFast) install(s *Server) {
	inner := s.draw
	s.draw = func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error) {
		f.draws.Add(1)
		if f.hang.Load() {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if !f.fast.Load() {
			return nil, &diagram.Refusal{Kind: diagram.Limit, Reason: "the layout took longer than 2s", Deadline: true}
		}
		return inner(ctx, src, theme)
	}
	f.now = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.now
	}
}

func (f *slowThenFast) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// A block refused at the deadline on a busy host is drawn again once its
// refusal has expired, rather than staying code until the daemon restarts;
// until then, asking again draws nothing (davison/md-notes#182).
func TestDiagramDeadlineRefusalIsRetried(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	f := &slowThenFast{}
	f.install(serverOf(t, ts))
	status := func() int {
		t.Helper()
		resp := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", nil)
		readAll(t, resp.Body)
		return resp.StatusCode
	}
	if got := status(); got != http.StatusUnprocessableEntity {
		t.Fatalf("slow draw: status %d, want 422", got)
	}
	// The host is idle now, but the refusal is still fresh.
	f.fast.Store(true)
	f.advance(59 * time.Second)
	if got := status(); got != http.StatusUnprocessableEntity {
		t.Fatalf("within the expiry: status %d, want 422", got)
	}
	if sized, _ := measureNote(t, ts, "d.md"); sized != 0 {
		t.Errorf("within the expiry the note sized the block")
	}
	if n := f.draws.Load(); n != 1 {
		t.Fatalf("drew %d times within the expiry, want 1", n)
	}
	f.advance(2 * time.Second)
	if got := status(); got != http.StatusOK {
		t.Fatalf("after the expiry: status %d, want 200", got)
	}
	if sized, _ := measureNote(t, ts, "d.md"); sized != 1 {
		t.Errorf("after the retry the note did not size the block")
	}
}

// The note endpoint lists a block again once its deadline refusal has
// expired, so the page asks for its image; before then it is code from the
// start.
func TestNoteListsADeadlineRefusalAgainAfterItExpires(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	f := &slowThenFast{}
	f.install(serverOf(t, ts))
	listed := func() int {
		t.Helper()
		resp := do(t, ts, "GET", "/api/r/notes/note/d.md?sizes=1", "", nil)
		var note struct{ Diagrams []struct{ Hash string } }
		if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
			t.Fatal(err)
		}
		return len(note.Diagrams)
	}
	if n := listed(); n != 0 {
		t.Fatalf("a deadline-refused block was listed (%d)", n)
	}
	f.fast.Store(true)
	if n := listed(); n != 0 {
		t.Fatalf("listed within the expiry (%d)", n)
	}
	f.advance(time.Minute)
	// Still slow: the budget would cut every measure off.
	f.hang.Store(true)
	before := f.draws.Load()
	for range 5 {
		if n := listed(); n != 1 {
			t.Fatalf("not listed after the expiry (%d)", n)
		}
	}
	// Listed unmeasured, and not measured again on every open: the image
	// route is what retries it, once per expiry (review of PR #204, nit 2).
	if n := f.draws.Load(); n != before {
		t.Errorf("five opens after the expiry drew %d times, want 0", n-before)
	}
	// The image route draws it in full.
	f.hang.Store(false)
	resp := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", nil)
	readAll(t, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("image after the expiry: status %d, want 200", resp.StatusCode)
	}
}

// A block that is always too slow is drawn once per window, and the window
// doubles, up to an hour: it cannot be re-rendered in a loop.
func TestDiagramDeadlineRefusalBacksOff(t *testing.T) {
	ts, base := newTestServer(t)
	writeNote(t, base, "d.md", fence(testFlow))
	f := &slowThenFast{}
	f.install(serverOf(t, ts))
	ask := func() {
		t.Helper()
		resp := do(t, ts, "GET", diagramURL("d.md", testFlow, "light"), "", nil)
		readAll(t, resp.Body)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status %d, want 422", resp.StatusCode)
		}
	}
	ask()
	want := int32(1)
	for _, window := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour} {
		// Asked every second of the window, it is drawn only at the end.
		for range 5 {
			f.advance(window / 6)
			ask()
		}
		if n := f.draws.Load(); n != want {
			t.Fatalf("inside a %v window: drew %d times, want %d", window, n, want)
		}
		f.advance(window - 5*(window/6))
		ask()
		want++
		if n := f.draws.Load(); n != want {
			t.Fatalf("after a %v window: drew %d times, want %d", window, n, want)
		}
	}
}

// A refusal that is a property of the source is never drawn again, however
// much time passes.
func TestDiagramDeterministicRefusalIsNotRetried(t *testing.T) {
	ts, base := newTestServer(t)
	src := layoutRefused()
	writeNote(t, base, "d.md", fence(src))
	s := serverOf(t, ts)
	f := &slowThenFast{}
	f.fast.Store(true)
	f.install(s)
	for range 3 {
		resp := do(t, ts, "GET", diagramURL("d.md", src, "light"), "", nil)
		readAll(t, resp.Body)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status %d, want 422", resp.StatusCode)
		}
		f.advance(24 * time.Hour)
	}
	if n := f.draws.Load(); n != 1 {
		t.Errorf("drew %d times, want 1", n)
	}
}
