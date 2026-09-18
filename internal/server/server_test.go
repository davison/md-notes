package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/search"
	"github.com/davison/md-notes/internal/tags"
	"github.com/davison/md-notes/internal/token"
	"github.com/davison/md-notes/internal/tree"
	"github.com/davison/md-notes/internal/watch"
	"github.com/davison/md-notes/ui"
)

const port = 7337

// servers maps a test server to the Server behind it.
var servers = map[*httptest.Server]*Server{}

func serverOf(t *testing.T, ts *httptest.Server) *Server {
	t.Helper()
	s, ok := servers[ts]
	if !ok {
		t.Fatal("unknown test server")
	}
	return s
}

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	return newTestServerWith(t)
}

// newTestServerWith is newTestServer with extra options, for the tailnet
// tests; the base directory it returns holds the notes root and the token.
func newTestServerWith(t *testing.T, opts ...Option) (*httptest.Server, string) {
	t.Helper()
	base := t.TempDir()
	notes := filepath.Join(base, "notes")
	os.MkdirAll(filepath.Join(notes, "sub"), 0o755)
	os.MkdirAll(filepath.Join(notes, "empty"), 0o755)
	os.WriteFile(filepath.Join(notes, "hello.md"), []byte("# hi\n"), 0o644)
	os.WriteFile(filepath.Join(notes, "sub", "linked.md"), []byte("---\ntitle: Linked\ntags: [demo]\n---\n[back](../hello.md) ![p](pic.png) #inline\n"), 0o644)
	os.WriteFile(filepath.Join(notes, "sub", "pic.png"), []byte("PNG"), 0o644)
	os.WriteFile(filepath.Join(base, "secret"), []byte("s"), 0o644)
	os.Symlink(filepath.Join(base, "secret"), filepath.Join(notes, "escape"))

	reg, err := roots.New(notes, filepath.Join(t.TempDir(), "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	// The four shapes the asset path has to tell apart: a hashed asset the
	// build precompressed both ways, one it could only shrink with gzip, one
	// with no compressed sibling at all, and a file outside the hashed
	// directory. Nothing in the daemon decodes brotli, so a stand-in for the
	// brotli stream is enough to prove the right bytes are chosen.
	ui := fstest.MapFS{
		"index.html":            {Data: []byte("<html>app</html>")},
		"assets/app.js":         {Data: []byte("console.log(1)")},
		"assets/main.js":        {Data: []byte(uiMainJS)},
		"assets/main.js.br":     {Data: []byte(uiMainBrotli)},
		"assets/main.js.gz":     {Data: gzipBytes(uiMainJS)},
		"assets/only-gz.css":    {Data: []byte(uiOnlyGzCSS)},
		"assets/only-gz.css.gz": {Data: gzipBytes(uiOnlyGzCSS)},
		"favicon.svg":           {Data: []byte("<svg/>")},
		// The installable app's unhashed files: none is under the hashed
		// directory, so all of them live or die by the no-cache rule, and
		// all of them are behind the tailnet guard
		// (TestTailnetGuardsTheInstallableAppsFiles).
		"manifest.webmanifest":  {Data: []byte(`{"name":"MD Notes"}`)},
		"sw.js":                 {Data: []byte("self.addEventListener('fetch', () => {})")},
		"icon.svg":              {Data: []byte("<svg/>")},
		"icon-192.png":          {Data: []byte("PNG")},
		"icon-512.png":          {Data: []byte("PNG")},
		"icon-maskable-512.png": {Data: []byte("PNG")},
		// One of each kind the build can emit, so the Content-Type table is
		// answering rather than the host's mime.types.
		"assets/main.js.map":  {Data: []byte(`{"version":3}`)},
		"assets/logo-x.png":   {Data: []byte("PNG")},
		"assets/font-x.woff2": {Data: []byte("wOF2")},
		"assets/data-x.json":  {Data: []byte("{}")},
		"assets/blob-x.bin":   {Data: []byte("bin")},
	}
	store, _, err := token.Open(filepath.Join(base, "token"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg, port, ui, log.New(io.Discard, "", 0), append([]Option{WithToken(store)}, opts...)...)
	s.keepalive = 100 * time.Millisecond
	t.Cleanup(s.Close)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	servers[ts] = s
	return ts, base
}

// do sends a request with the Host header a browser at localhost would send.
func do(t *testing.T, ts *httptest.Server, method, path string, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:7337"
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, _ := io.ReadAll(r)
	return string(b)
}

// daemonToken is the bearer token the test daemon holds.
func daemonToken(t *testing.T, base string) string {
	t.Helper()
	tok, _, err := token.Load(filepath.Join(base, "token"))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// bearerHeader presents tok the way the extension does.
func bearerHeader(tok string, extra ...string) map[string]string {
	h := map[string]string{"Authorization": "Bearer " + tok}
	for i := 0; i+1 < len(extra); i += 2 {
		h[extra[i]] = extra[i+1]
	}
	return h
}

func TestGuardHost(t *testing.T) {
	ts, _ := newTestServer(t)
	for host, want := range map[string]int{
		"localhost:7337":        200,
		"127.0.0.1:7337":        200,
		"LOCALHOST:7337":        200,
		"evil.example:7337":     403,
		"localhost:9999":        403,
		"localhost":             403,
		"127.0.0.1.nip.io:7337": 403,
	} {
		resp := do(t, ts, "GET", "/api/roots", "", map[string]string{"Host": host})
		if resp.StatusCode != want {
			t.Errorf("Host %q: status %d, want %d", host, resp.StatusCode, want)
		}
	}
}

func TestGuardOrigin(t *testing.T) {
	ts, _ := newTestServer(t)
	for origin, want := range map[string]int{
		"":                           200,
		"http://localhost:7337":      200,
		"http://127.0.0.1:7337":      200,
		"https://localhost:7337":     403,
		"http://localhost:8000":      403,
		"http://evil.example":        403,
		"null":                       403,
		"http://localhost:7337.evil": 403,
	} {
		hdr := map[string]string{}
		if origin != "" {
			hdr["Origin"] = origin
		}
		resp := do(t, ts, "GET", "/api/roots", "", hdr)
		if resp.StatusCode != want {
			t.Errorf("Origin %q: status %d, want %d", origin, resp.StatusCode, want)
		}
	}
}

// The token is the claim the same-origin rule stands in for, so a request
// carrying it is served from any origin — which is how the extension writes
// from its own chrome-extension origin.
func TestGuardTokenPassesAnyOrigin(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	cases := []struct {
		name string
		hdr  map[string]string
		want int
	}{
		{"extension origin with the token", bearerHeader(tok, "Origin", "chrome-extension://abcdefghijklmnop"), 200},
		{"extension origin without it", map[string]string{"Origin": "chrome-extension://abcdefghijklmnop"}, 403},
		{"foreign web origin with the token", bearerHeader(tok, "Origin", "https://evil.example"), 200},
		{"no origin with the token", bearerHeader(tok), 200},
		{"wrong token", bearerHeader(tok + "x"), 401},
		{"wrong token from the daemon's own origin", bearerHeader("nonsense", "Origin", "http://localhost:7337"), 401},
		{"empty bearer", map[string]string{"Authorization": "Bearer "}, 401},
		{"another scheme", map[string]string{"Authorization": "Basic " + tok}, 401},
		{"the header alone", map[string]string{"Authorization": tok}, 401},
		{"lower-case scheme", map[string]string{"Authorization": "bearer " + tok}, 200},
	}
	for _, c := range cases {
		resp := do(t, ts, "GET", "/api/roots", "", c.hdr)
		if resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.want)
		}
	}
	// The Host check is not waived by the token: DNS rebinding is a
	// separate attack, and a rebound page could hold no token anyway.
	resp := do(t, ts, "GET", "/api/roots", "", bearerHeader(tok, "Host", "evil.example:7337"))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("bad Host with the token: status %d, want 403", resp.StatusCode)
	}
}

// Every refusal the guard makes carries a code, so a client can tell "no
// token pasted in yet" from "the token was refused" — which is what the
// extension's popup has to report differently.
func TestGuardRefusalsAreDistinguishable(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	cases := []struct {
		name   string
		hdr    map[string]string
		status int
		code   string
	}{
		{"a wrong token", bearerHeader(tok + "x"), http.StatusUnauthorized, "unauthorized"},
		{"no token from another origin", map[string]string{"Origin": "chrome-extension://abc"}, http.StatusForbidden, "cross_origin"},
		{"an unexpected Host", map[string]string{"Host": "evil.example:7337"}, http.StatusForbidden, "bad_host"},
	}
	for _, c := range cases {
		resp := do(t, ts, "GET", "/api/roots", "", c.hdr)
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.status)
			continue
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control %q, want no-store", c.name, got)
		}
		var body struct{ Code, Error string }
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Code != c.code || body.Error == "" {
			t.Errorf("%s: body = %+v, want code %s and a message", c.name, body, c.code)
		}
	}
	// The 401 carries the challenge a generic HTTP client expects.
	resp := do(t, ts, "GET", "/api/roots", "", bearerHeader("nonsense"))
	if got := resp.Header.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
	}
}

// The Origin exemption is for a caller CORS does not govern — an MV3
// service worker holding host_permissions. A page-context fetch would
// preflight first, and the preflight is refused like any other
// cross-origin request: no CORS headers are sent, deliberately.
func TestGuardRefusesThePreflight(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "OPTIONS", "/api/clip", "", map[string]string{
		"Origin":                         "chrome-extension://abc",
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "authorization,content-type",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("preflight: status %d, want 403", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want no CORS headers at all", got)
	}
}

// mdn token --rotate must reach a running daemon; nothing restarts it.
func TestGuardSeesARotatedToken(t *testing.T) {
	ts, base := newTestServer(t)
	old := daemonToken(t, base)
	fresh, err := token.Rotate(filepath.Join(base, "token"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := do(t, ts, "GET", "/api/roots", "", bearerHeader(fresh)); resp.StatusCode != 200 {
		t.Errorf("rotated token: status %d, want 200", resp.StatusCode)
	}
	if resp := do(t, ts, "GET", "/api/roots", "", bearerHeader(old)); resp.StatusCode != 401 {
		t.Errorf("replaced token: status %d, want 401", resp.StatusCode)
	}
}

// A daemon built without a token accepts none, rather than treating the
// Authorization header as decoration.
func TestGuardWithoutATokenRefusesEveryBearer(t *testing.T) {
	reg, err := roots.New(t.TempDir(), filepath.Join(t.TempDir(), "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg, port, fstest.MapFS{}, log.New(io.Discard, "", 0))
	t.Cleanup(s.Close)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	servers[ts] = s
	if resp := do(t, ts, "GET", "/api/roots", "", bearerHeader("anything")); resp.StatusCode != 401 {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
	if resp := do(t, ts, "GET", "/api/roots", "", nil); resp.StatusCode != 200 {
		t.Errorf("unauthenticated loopback request: status %d, want 200", resp.StatusCode)
	}
}

func TestRootsListAndAdd(t *testing.T) {
	ts, base := newTestServer(t)
	resp := do(t, ts, "GET", "/api/roots", "", nil)
	var list struct{ Roots []roots.Root }
	json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Roots) != 1 || list.Roots[0].Slug != "notes" || list.Roots[0].Kind != roots.KindNotes {
		t.Fatalf("roots = %+v", list.Roots)
	}

	proj := filepath.Join(base, "proj")
	os.Mkdir(proj, 0o755)
	resp = do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, map[string]string{"Content-Type": "application/json"})
	if resp.StatusCode != 200 {
		t.Fatalf("add: status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var added roots.Root
	json.NewDecoder(resp.Body).Decode(&added)
	if added.Slug != "proj" || added.Kind != roots.KindRecent || added.Path != proj {
		t.Fatalf("added = %+v", added)
	}

	resp = do(t, ts, "GET", "/api/roots", "", nil)
	list.Roots = nil
	json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Roots) != 2 {
		t.Fatalf("after add roots = %+v", list.Roots)
	}
}

func TestAddRootErrors(t *testing.T) {
	ts, base := newTestServer(t)
	cases := []struct {
		body string
		want int
	}{
		{`not json`, 400},
		{`{}`, 400},
		{`{"path":"` + filepath.Join(base, "missing") + `"}`, 400},
		{`{"path":"` + filepath.Join(base, "secret") + `"}`, 400},
		{`{"path":"relative/dir"}`, 400},
		{`{"path":"."}`, 400},
	}
	for _, c := range cases {
		resp := do(t, ts, "POST", "/api/roots", c.body, nil)
		if resp.StatusCode != c.want {
			t.Errorf("body %s: status %d, want %d", c.body, resp.StatusCode, c.want)
		}
	}
}

func TestRawFile(t *testing.T) {
	ts, _ := newTestServer(t)
	cases := []struct {
		path string
		want int
		body string
	}{
		{"/api/r/notes/raw/hello.md", 200, "# hi\n"},
		{"/api/r/notes/raw/sub/pic.png", 200, "PNG"},
		{"/api/r/notes/raw/sub", 404, ""},
		{"/api/r/notes/raw/nope.md", 404, ""},
		{"/api/r/nope/raw/hello.md", 404, ""},
		{"/api/r/notes/raw/escape", 403, ""},
		{"/api/r/notes/raw/..%2Fsecret", 403, ""},
		{"/api/r/notes/raw/%2e%2e/secret", 403, ""},
		{"/api/r/notes/raw/sub/..%2F..%2Fsecret", 403, ""},
	}
	for _, c := range cases {
		resp := do(t, ts, "GET", c.path, "", nil)
		body := readAll(t, resp.Body)
		if resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.path, resp.StatusCode, c.want, body)
			continue
		}
		if c.body != "" && body != c.body {
			t.Errorf("%s: body %q, want %q", c.path, body, c.body)
		}
	}
	// A plain ../ in the URL is collapsed by the client before sending, and
	// by the mux if it arrives; either way it must never reach the secret.
	resp := do(t, ts, "GET", "/api/r/notes/raw/../../../secret", "", nil)
	if b := readAll(t, resp.Body); strings.Contains(b, "s") && resp.StatusCode == 200 && b == "s" {
		t.Errorf("dot-dot traversal served the secret")
	}
}

func TestRawFileHeadersSandboxContent(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/api/r/notes/raw/hello.md", "", nil)
	if got := resp.Header.Get("Content-Security-Policy"); got != "sandbox" {
		t.Errorf("CSP = %q, want sandbox", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestTree(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/api/r/notes/tree", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var root tree.Node
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	// The fixture has hello.md at the top, sub/linked.md, and sub/pic.png,
	// which is not markdown and must not appear.
	if !root.Dir || len(root.Children) != 2 || root.Children[0].Path != "sub" || root.Children[1].Path != "hello.md" {
		t.Fatalf("tree = %+v", root)
	}
	sub := root.Children[0]
	if len(sub.Children) != 1 || sub.Children[0].Path != "sub/linked.md" {
		t.Fatalf("sub = %+v", sub)
	}

	resp = do(t, ts, "GET", "/api/r/nope/tree", "", nil)
	if resp.StatusCode != 404 {
		t.Errorf("unknown root: status %d, want 404", resp.StatusCode)
	}
}

func TestNote(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/api/r/notes/note/sub/linked.md", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var note struct {
		Path, Title, HTML string
		Frontmatter       map[string]any
	}
	json.NewDecoder(resp.Body).Decode(&note)
	if note.Path != "sub/linked.md" || note.Title != "Linked" || note.Frontmatter["title"] != "Linked" {
		t.Errorf("note = %+v", note)
	}
	for _, want := range []string{`href="/r/notes/hello.md"`, `src="/api/r/notes/raw/sub/pic.png"`} {
		if !strings.Contains(note.HTML, want) {
			t.Errorf("html lacks %s: %s", want, note.HTML)
		}
	}

	for path, want := range map[string]int{
		"/api/r/notes/note/hello.md":       200,
		"/api/r/notes/note/sub/pic.png":    404,
		"/api/r/notes/note/missing.md":     404,
		"/api/r/nope/note/hello.md":        404,
		"/api/r/notes/note/escape":         404,
		"/api/r/notes/note/..%2Fsecret.md": 403,
	} {
		resp := do(t, ts, "GET", path, "", nil)
		if resp.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, resp.StatusCode, want)
		}
	}
}

// sseReader turns a stream into a function that waits for a line
// satisfying want, sharing one scanner across calls.
func sseReader(t *testing.T, body io.Reader) func(want func(string) bool) string {
	t.Helper()
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	return func(want func(string) bool) string {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatal("stream ended")
				}
				if want(l) {
					return l
				}
			case <-deadline:
				t.Fatal("wanted line did not arrive")
			}
		}
	}
}

func TestEventsStream(t *testing.T) {
	ts, base := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/notes/events", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != 200 || ct != "text/event-stream" {
		t.Fatalf("status %d, content-type %q", resp.StatusCode, ct)
	}
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })
	// The coverage of a small root is complete and says so.
	next(func(l string) bool { return l == "event: status" })
	status := next(func(l string) bool { return strings.HasPrefix(l, "data: ") })
	var cov watch.Coverage
	json.Unmarshal([]byte(strings.TrimPrefix(status, "data: ")), &cov)
	if cov.Limited || cov.Unwatched != 0 || cov.Watched == 0 {
		t.Fatalf("coverage = %+v, want a complete one", cov)
	}
	next(func(l string) bool { return l == ": keepalive" })

	os.WriteFile(filepath.Join(base, "notes", "new.md"), []byte("# new"), 0o644)
	next(func(l string) bool { return l == "event: change" })
	data := next(func(l string) bool { return strings.HasPrefix(l, "data: ") })
	var b struct{ Paths []string }
	json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &b)
	if len(b.Paths) != 1 || b.Paths[0] != "new.md" {
		t.Fatalf("paths = %v", b.Paths)
	}

	// A directory that was empty at startup is watched too.
	os.WriteFile(filepath.Join(base, "notes", "sub", "later.md"), []byte("x"), 0o644)
	next(func(l string) bool { return strings.Contains(l, `"sub/later.md"`) })
	os.WriteFile(filepath.Join(base, "notes", "empty", "first.md"), []byte("x"), 0o644)
	next(func(l string) bool { return strings.Contains(l, `"empty/first.md"`) })

	resp2 := do(t, ts, "GET", "/api/r/nope/events", "", nil)
	if resp2.StatusCode != 404 {
		t.Errorf("unknown root: status %d", resp2.StatusCode)
	}
}

// A root too large for its watch budget says so on its event stream, so the
// limit is visible in the browser and not only in the daemon's log.
func TestEventsStreamReportsLimitedCoverage(t *testing.T) {
	notes := t.TempDir()
	for _, d := range []string{"a", "b", "c"} {
		os.MkdirAll(filepath.Join(notes, d), 0o755)
		os.WriteFile(filepath.Join(notes, d, "n.md"), []byte("x"), 0o644)
	}
	reg, err := roots.New(notes, filepath.Join(t.TempDir(), "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg, port, fstest.MapFS{}, log.New(io.Discard, "", 0), WithWatchBudget(2))
	s.keepalive = 100 * time.Millisecond
	t.Cleanup(s.Close)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/"+reg.List()[0].Slug+"/events", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == "event: status" })
	data := next(func(l string) bool { return strings.HasPrefix(l, "data: ") })
	var cov watch.Coverage
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &cov); err != nil {
		t.Fatal(err)
	}
	want := watch.Coverage{Watched: 2, Unwatched: 2, Budget: 2, OverBudget: true, Limited: true}
	if cov != want {
		t.Fatalf("coverage = %+v, want %+v", cov, want)
	}
}

// A directory the filesystem refuses reaches the browser as that cause, not
// as the kernel limit: the page can then offer a remedy that works (M8-R7).
func TestEventsStreamNamesARefusedDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory mode, so nothing is refused")
	}
	notes := t.TempDir()
	os.WriteFile(filepath.Join(notes, "top.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(notes, "ok"), 0o755)
	os.WriteFile(filepath.Join(notes, "ok", "n.md"), []byte("x"), 0o644)
	locked := filepath.Join(notes, "locked")
	os.MkdirAll(locked, 0o755)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}

	reg, err := roots.New(notes, filepath.Join(t.TempDir(), "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg, port, fstest.MapFS{}, log.New(io.Discard, "", 0))
	s.keepalive = 100 * time.Millisecond
	t.Cleanup(s.Close)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/"+reg.List()[0].Slug+"/events", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == "event: status" })
	data := next(func(l string) bool { return strings.HasPrefix(l, "data: ") })
	var cov watch.Coverage
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &cov); err != nil {
		t.Fatal(err)
	}
	if cov.Refused != 1 || cov.Reason != "permission denied" || cov.Failed != 0 || !cov.Limited {
		t.Fatalf("coverage = %+v, want one directory refused by the filesystem and none by the kernel", cov)
	}
	if !strings.Contains(data, `"refused":1`) || !strings.Contains(data, `"reason":"permission denied"`) {
		t.Fatalf("status event = %s, want the cause on the wire", data)
	}
}

// M2-R4 over the wire: the first note in a directory whose only file is a
// hidden placeholder reaches the browser, at every level of a nest of them.
func TestEventsStreamReportsFirstNoteInPlaceholderDirectories(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	notes := t.TempDir()
	os.WriteFile(filepath.Join(notes, "hello.md"), []byte("# hi"), 0o644)
	os.MkdirAll(filepath.Join(notes, "nest", "deep"), 0o755)
	os.WriteFile(filepath.Join(notes, "nest", ".gitkeep"), nil, 0o644)
	os.WriteFile(filepath.Join(notes, "nest", "deep", ".gitkeep"), nil, 0o644)

	reg, err := roots.New(notes, filepath.Join(t.TempDir(), "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg, port, fstest.MapFS{}, log.New(io.Discard, "", 0))
	s.keepalive = 100 * time.Millisecond
	t.Cleanup(s.Close)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/"+reg.List()[0].Slug+"/events", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })
	status := next(func(l string) bool { return strings.HasPrefix(l, "data: ") })
	var cov watch.Coverage
	json.Unmarshal([]byte(strings.TrimPrefix(status, "data: ")), &cov)
	if cov.Watched != 3 || cov.Limited {
		t.Fatalf("coverage = %+v, want the root and both placeholder directories", cov)
	}

	os.WriteFile(filepath.Join(notes, "nest", "first.md"), []byte("# f"), 0o644)
	next(func(l string) bool { return strings.Contains(l, `"nest/first.md"`) })
	os.WriteFile(filepath.Join(notes, "nest", "deep", "first.md"), []byte("# d"), 0o644)
	next(func(l string) bool { return strings.Contains(l, `"nest/deep/first.md"`) })
}

func TestAddedRootIsWatched(t *testing.T) {
	ts, base := newTestServer(t)
	proj := filepath.Join(base, "proj")
	os.Mkdir(proj, 0o755)
	do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/proj/events", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })
	os.WriteFile(filepath.Join(proj, "readme.md"), []byte("x"), 0o644)
	next(func(l string) bool { return strings.Contains(l, `"readme.md"`) })
}

func TestEventsUnavailableWithoutWatcher(t *testing.T) {
	ts, _ := newTestServer(t)
	// Simulate a root whose watcher failed: registered, no hub, not starting.
	s := serverOf(t, ts)
	s.hub(context.Background(), "notes")
	s.wmu.Lock()
	delete(s.hubs, "notes")
	s.wmu.Unlock()
	resp := do(t, ts, "GET", "/api/r/notes/events", "", nil)
	if resp.StatusCode != 503 {
		t.Fatalf("status %d, want 503 when live update is unavailable", resp.StatusCode)
	}
}

func TestCloseEndsOpenStreamsQuickly(t *testing.T) {
	ts, _ := newTestServer(t)
	s := serverOf(t, ts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/notes/events", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })
	start := time.Now()
	s.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("stream took %v to end after Close", d)
	}
}

func TestSearch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/api/r/notes/search?q=hi", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var body search.Result
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Hits) != 1 || body.Hits[0].Path != "hello.md" || body.Hits[0].Line != 1 || body.Truncated {
		t.Fatalf("hits = %+v", body)
	}
	for path, want := range map[string]int{
		"/api/r/notes/search":            400,
		"/api/r/notes/search?q=%20":      400,
		"/api/r/notes/search?q=a%0Ab":    400,
		"/api/r/notes/search?q=a%00b":    400,
		"/api/r/nope/search?q=x":         404,
		"/api/r/notes/search?q=zzz-none": 200,
	} {
		resp := do(t, ts, "GET", path, "", nil)
		if resp.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, resp.StatusCode, want)
		}
	}
	resp = do(t, ts, "GET", "/api/r/notes/search?q=zzz-none", "", nil)
	if b := readAll(t, resp.Body); !strings.Contains(b, `"hits":[]`) {
		t.Errorf("no-match body = %s, want an empty array", b)
	}
}

func TestTags(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	ts, _ := newTestServer(t)
	resp := do(t, ts, "GET", "/api/r/notes/tags", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var body struct{ Tags []tags.Tag }
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Tags) != 2 || body.Tags[0].Name != "demo" || body.Tags[1].Name != "inline" || body.Tags[0].Notes[0] != "sub/linked.md" {
		t.Fatalf("tags = %+v", body.Tags)
	}
	if resp := do(t, ts, "GET", "/api/r/nope/tags", "", nil); resp.StatusCode != 404 {
		t.Errorf("unknown root: %d", resp.StatusCode)
	}
}

func TestCloseBoundsWaitForStuckSetup(t *testing.T) {
	ts, _ := newTestServer(t)
	s := serverOf(t, ts)
	orig := setupWait
	setupWait = 50 * time.Millisecond
	t.Cleanup(func() { setupWait = orig })
	stuck := make(chan struct{})
	s.wmu.Lock()
	s.starting["stuck"] = stuck
	s.wmu.Unlock()
	t.Cleanup(func() { close(stuck) }) // so the server's own cleanup does not wait again
	start := time.Now()
	s.Close()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Close waited %v for a stuck setup", d)
	}
}

func TestUIFallback(t *testing.T) {
	ts, _ := newTestServer(t)
	for path, want := range map[string]string{
		"/":                  "<html>app</html>",
		"/r/notes/":          "<html>app</html>",
		"/r/notes/some/note": "<html>app</html>",
		"/assets/app.js":     "console.log(1)",
		"/index.html":        "<html>app</html>",
	} {
		resp := do(t, ts, "GET", path, "", nil)
		if body := readAll(t, resp.Body); resp.StatusCode != 200 || body != want {
			t.Errorf("%s: %d %q, want %q", path, resp.StatusCode, body, want)
		}
	}
	resp := do(t, ts, "GET", "/api/nope", "", nil)
	if resp.StatusCode != 404 {
		t.Errorf("/api/nope: status %d, want 404", resp.StatusCode)
	}
}

// The UI fixtures. uiMainBrotli stands in for a brotli stream: the daemon
// serves the file's bytes without reading them, so what matters is that
// they are distinct from every other representation.
const (
	uiMainJS     = "export const main = 1;\n"
	uiMainBrotli = "\x1b\x15\x00brotli(main.js)"
	uiOnlyGzCSS  = "body { color: red }\n"
)

func gzipBytes(s string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(s))
	zw.Close()
	return buf.Bytes()
}

// uiGet fetches a UI path sending exactly the Accept-Encoding given — the
// empty string meaning none at all. The default client fills that header in
// with gzip and then transparently decodes the reply, which would hide both
// the negotiation and the headers under test.
func uiGet(t *testing.T, ts *httptest.Server, method, path, accept string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:7337"
	if accept != "" {
		req.Header.Set("Accept-Encoding", accept)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestUICaching(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, tc := range []struct {
		path, cache, ctype string
	}{
		{"/assets/app.js", uiImmutable, "javascript"},
		{"/assets/only-gz.css", uiImmutable, "css"},
		{"/index.html", "no-cache", "html"},
		{"/favicon.svg", "no-cache", "svg"},
		{"/r/notes/some/note.md", "no-cache", "html"}, // the client-side route falls back to index.html
		{"/", "no-cache", "html"},
	} {
		resp := uiGet(t, ts, "GET", tc.path, "identity", nil)
		h := resp.Header
		switch {
		case resp.StatusCode != 200:
			t.Errorf("%s: status %d, want 200", tc.path, resp.StatusCode)
		case h.Get("Cache-Control") != tc.cache:
			t.Errorf("%s: Cache-Control %q, want %q", tc.path, h.Get("Cache-Control"), tc.cache)
		case h.Get("Vary") != "Accept-Encoding":
			t.Errorf("%s: Vary %q, want Accept-Encoding", tc.path, h.Get("Vary"))
		case h.Get("ETag") == "":
			t.Errorf("%s: no ETag", tc.path)
		case h.Get("Content-Encoding") != "":
			t.Errorf("%s: Content-Encoding %q under identity, want none", tc.path, h.Get("Content-Encoding"))
		case !strings.Contains(h.Get("Content-Type"), tc.ctype):
			t.Errorf("%s: Content-Type %q, want it to mention %q", tc.path, h.Get("Content-Type"), tc.ctype)
		}
	}
}

func TestUIConditionalRequest(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, tc := range []struct{ path, accept string }{
		{"/assets/app.js", "identity"},
		{"/assets/main.js", "br, gzip"}, // the brotli representation
		{"/assets/main.js", "gzip"},     // and the gzip one, under its own tag
		{"/index.html", "identity"},
		{"/r/notes/some/note.md", "identity"},
	} {
		first := uiGet(t, ts, "GET", tc.path, tc.accept, nil)
		tag := first.Header.Get("ETag")
		body := readAll(t, first.Body)
		if first.StatusCode != 200 || tag == "" || body == "" {
			t.Fatalf("%s (%s): status %d tag %q body %q", tc.path, tc.accept, first.StatusCode, tag, body)
		}
		again := uiGet(t, ts, "GET", tc.path, tc.accept, map[string]string{"If-None-Match": tag})
		if again.StatusCode != http.StatusNotModified {
			t.Errorf("%s (%s): status %d, want 304", tc.path, tc.accept, again.StatusCode)
		}
		if b := readAll(t, again.Body); b != "" {
			t.Errorf("%s (%s): 304 carried %q", tc.path, tc.accept, b)
		}
		stale := uiGet(t, ts, "GET", tc.path, tc.accept, map[string]string{"If-None-Match": `"stale"`})
		if stale.StatusCode != 200 || readAll(t, stale.Body) != body {
			t.Errorf("%s (%s): a stale validator got %d, want the body again", tc.path, tc.accept, stale.StatusCode)
		}
	}
}

func TestUIEncodingNegotiation(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, tc := range []struct{ name, accept, coding, body string }{
		{"brotli preferred", "gzip, deflate, br, zstd", "br", uiMainBrotli},
		{"gzip when brotli is not offered", "gzip, deflate", "gzip", string(gzipBytes(uiMainJS))},
		{"brotli refused by name", "br;q=0, gzip", "gzip", string(gzipBytes(uiMainJS))},
		{"weights honoured over preference order", "br;q=0.2, gzip;q=0.9", "gzip", string(gzipBytes(uiMainJS))},
		{"the wildcard accepts brotli", "*", "br", uiMainBrotli},
		{"identity only", "identity", "", uiMainJS},
		{"everything refused but identity", "gzip;q=0, br;q=0", "", uiMainJS},
		{"no Accept-Encoding at all", "", "", uiMainJS},
	} {
		resp := uiGet(t, ts, "GET", "/assets/main.js", tc.accept, nil)
		h := resp.Header
		switch {
		case resp.StatusCode != 200:
			t.Errorf("%s: status %d, want 200", tc.name, resp.StatusCode)
		case h.Get("Content-Encoding") != tc.coding:
			t.Errorf("%s: Content-Encoding %q, want %q", tc.name, h.Get("Content-Encoding"), tc.coding)
		case readAll(t, resp.Body) != tc.body:
			t.Errorf("%s: wrong body for %q", tc.name, tc.coding)
		case !strings.Contains(h.Get("Content-Type"), "javascript"):
			t.Errorf("%s: Content-Type %q, want the source type not the sibling's", tc.name, h.Get("Content-Type"))
		case h.Get("Vary") != "Accept-Encoding":
			t.Errorf("%s: Vary %q", tc.name, h.Get("Vary"))
		case h.Get("Cache-Control") != uiImmutable:
			t.Errorf("%s: Cache-Control %q", tc.name, h.Get("Cache-Control"))
		}
	}

	// The gzip representation must arrive intact, not merely be labelled.
	resp := uiGet(t, ts, "GET", "/assets/main.js", "gzip", nil)
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	if got := readAll(t, zr); got != uiMainJS {
		t.Errorf("gzip body decoded to %q, want %q", got, uiMainJS)
	}

	// One tag per representation: a cache keyed on Vary must never be able
	// to answer a gzip request from the brotli entry.
	tags := map[string]string{}
	for _, accept := range []string{"br", "gzip", "identity"} {
		tag := uiGet(t, ts, "GET", "/assets/main.js", accept, nil).Header.Get("ETag")
		if tag == "" {
			t.Fatalf("%s: no ETag", accept)
		}
		if prev, ok := tags[tag]; ok {
			t.Errorf("%s and %s share the ETag %s", accept, prev, tag)
		}
		tags[tag] = accept
	}
}

func TestUIEncodingFallsBackWhenNotPrecompressed(t *testing.T) {
	ts, _ := newTestServer(t)
	// No sibling at all: the identity bytes answer rather than a 404.
	resp := uiGet(t, ts, "GET", "/assets/app.js", "br, gzip", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "" || readAll(t, resp.Body) != "console.log(1)" {
		t.Errorf("uncompressed asset: %d %q", resp.StatusCode, resp.Header.Get("Content-Encoding"))
	}
	if resp.Header.Get("Vary") != "Accept-Encoding" {
		t.Errorf("uncompressed asset: Vary %q", resp.Header.Get("Vary"))
	}
	// Only gzip was written: brotli is preferred but must not be invented.
	resp = uiGet(t, ts, "GET", "/assets/only-gz.css", "br, gzip", nil)
	if resp.Header.Get("Content-Encoding") != "gzip" || readAll(t, resp.Body) != string(gzipBytes(uiOnlyGzCSS)) {
		t.Errorf("gzip-only asset: Content-Encoding %q", resp.Header.Get("Content-Encoding"))
	}
	// index.html is below the build's compression threshold in practice, so
	// the same fallback carries the page itself.
	resp = uiGet(t, ts, "GET", "/index.html", "br, gzip", nil)
	if resp.Header.Get("Content-Encoding") != "" || readAll(t, resp.Body) != "<html>app</html>" {
		t.Errorf("index.html: Content-Encoding %q", resp.Header.Get("Content-Encoding"))
	}
}

func TestUIHead(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := uiGet(t, ts, "HEAD", "/assets/main.js", "br", nil)
	switch {
	case resp.StatusCode != 200:
		t.Errorf("status %d, want 200", resp.StatusCode)
	case resp.Header.Get("Content-Encoding") != "br":
		t.Errorf("Content-Encoding %q, want br", resp.Header.Get("Content-Encoding"))
	case resp.ContentLength != int64(len(uiMainBrotli)):
		t.Errorf("Content-Length %d, want %d", resp.ContentLength, len(uiMainBrotli))
	case readAll(t, resp.Body) != "":
		t.Error("HEAD carried a body")
	}
}

func TestEncodingQuality(t *testing.T) {
	for _, tc := range []struct {
		header, coding string
		want           float64
	}{
		{"gzip, deflate, br, zstd", "br", 1},
		{"gzip, deflate", "br", 0},
		{"", "gzip", 0},
		{"identity", "gzip", 0},
		{"*", "br", 1},
		{"*;q=0", "br", 0},
		{"*, br;q=0", "br", 0}, // the named coding beats the wildcard
		{"br;q=0.5", "br", 0.5},
		{" BR ;Q=0.25 ", "br", 0.25},  // case and spacing are not significant
		{"gzip;q=abc", "gzip", 1},     // an unparseable weight is not a refusal
		{"gzip;foo=1;q=0", "gzip", 0}, // other parameters are skipped, not misread
	} {
		if got := encodingQuality(tc.header, tc.coding); got != tc.want {
			t.Errorf("encodingQuality(%q, %q) = %v, want %v", tc.header, tc.coding, got, tc.want)
		}
	}
}

func TestUIContentTypes(t *testing.T) {
	ts, _ := newTestServer(t)
	// The types are the binary's own, not the build host's /etc/mime.types.
	for path, want := range map[string]string{
		"/assets/app.js":        "text/javascript; charset=utf-8",
		"/assets/only-gz.css":   "text/css; charset=utf-8",
		"/index.html":           "text/html; charset=utf-8",
		"/favicon.svg":          "image/svg+xml",
		"/assets/main.js.map":   "application/json",
		"/assets/logo-x.png":    "image/png",
		"/assets/font-x.woff2":  "font/woff2",
		"/assets/data-x.json":   "application/json",
		"/assets/blob-x.bin":    "application/octet-stream",
		"/manifest.webmanifest": "application/manifest+json",
		"/sw.js":                "text/javascript; charset=utf-8",
		"/r/notes/some/note.md": "text/html; charset=utf-8",
	} {
		if got := uiGet(t, ts, "GET", path, "identity", nil).Header.Get("Content-Type"); got != want {
			t.Errorf("%s: Content-Type %q, want %q", path, got, want)
		}
	}
}

// TestUIInstallableAssetsAreNotCachedBlind covers the two files the
// installable app adds outside the hashed directory. Neither carries a
// content hash, so neither may be given the immutable year: a browser that
// kept the old worker would keep the old shell with it, and the M4 rule that
// index.html always revalidates would hold for one file out of three.
func TestUIInstallableAssetsAreNotCachedBlind(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, path := range []string{"/manifest.webmanifest", "/sw.js"} {
		resp := uiGet(t, ts, "GET", path, "identity", nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: %d, want 200", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control %q, want no-cache", path, got)
		}
		if resp.Header.Get("ETag") == "" {
			t.Errorf("%s: no ETag, so revalidating costs the whole body", path)
		}
	}
}

func TestUIPrecompressedSiblingsAreNotAddressable(t *testing.T) {
	ts, _ := newTestServer(t)
	// A .br or .gz URL would hand out bytes no client asked to decode. They
	// are representations of another URL, not resources, so nothing under
	// the assets directory answers for them.
	for _, path := range []string{"/assets/main.js.br", "/assets/main.js.gz", "/assets/only-gz.css.gz"} {
		resp := uiGet(t, ts, "GET", path, "identity", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, resp.StatusCode)
		}
	}
	// The representation is still reachable the only way it should be.
	resp := uiGet(t, ts, "GET", "/assets/main.js", "br", nil)
	if resp.Header.Get("Content-Encoding") != "br" || readAll(t, resp.Body) != uiMainBrotli {
		t.Errorf("negotiated brotli: Content-Encoding %q", resp.Header.Get("Content-Encoding"))
	}
}

func TestUIRangeOverACompressedRepresentation(t *testing.T) {
	ts, _ := newTestServer(t)
	// The hand-written Content-Length holds only because ServeContent
	// overwrites it for a range; a regression here would declare the whole
	// compressed file for a partial body.
	full := uiGet(t, ts, "GET", "/assets/main.js", "br", nil)
	if got, want := full.Header.Get("Content-Length"), strconv.Itoa(len(uiMainBrotli)); got != want {
		t.Errorf("full body: Content-Length %q, want %q", got, want)
	}
	part := uiGet(t, ts, "GET", "/assets/main.js", "br", map[string]string{"Range": "bytes=0-9"})
	switch {
	case part.StatusCode != http.StatusPartialContent:
		t.Errorf("range: status %d, want 206", part.StatusCode)
	case part.Header.Get("Content-Length") != "10":
		t.Errorf("range: Content-Length %q, want 10", part.Header.Get("Content-Length"))
	case part.Header.Get("Content-Range") != fmt.Sprintf("bytes 0-9/%d", len(uiMainBrotli)):
		t.Errorf("range: Content-Range %q", part.Header.Get("Content-Range"))
	case part.Header.Get("Content-Encoding") != "br":
		t.Errorf("range: Content-Encoding %q, want br", part.Header.Get("Content-Encoding"))
	case readAll(t, part.Body) != uiMainBrotli[:10]:
		t.Error("range: wrong ten bytes")
	}
	// Past the end of the compressed body, not of the file it decodes to.
	over := uiGet(t, ts, "GET", "/assets/main.js", "br", map[string]string{"Range": "bytes=900-999"})
	if over.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("unsatisfiable range: status %d, want 416", over.StatusCode)
	}
	if over.Header.Get("Content-Encoding") != "" || over.Header.Get("Cache-Control") != "" {
		t.Errorf("unsatisfiable range kept Content-Encoding %q and Cache-Control %q",
			over.Header.Get("Content-Encoding"), over.Header.Get("Cache-Control"))
	}
}

// openFailsAfter opens its named file a fixed number of times and then
// refuses, so the error paths serveUIFile takes once a file has already
// vouched for itself can be reached at all.
type openFailsAfter struct {
	fs.FS
	name  string
	after int
	opens int
}

func (f *openFailsAfter) Open(name string) (fs.File, error) {
	if name == f.name {
		f.opens++
		if f.opens > f.after {
			return nil, fs.ErrNotExist
		}
	}
	return f.FS.Open(name)
}

// noSeekFS hides the io.Seeker its files implement, for the branch that
// serves an fs.FS whose files cannot seek.
type noSeekFS struct{ fs.FS }

func (f noSeekFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return noSeekFile{file}, nil
}

type noSeekFile struct{ fs.File }

func uiFixture() fstest.MapFS {
	return fstest.MapFS{
		"assets/main.js":    {Data: []byte(uiMainJS)},
		"assets/main.js.br": {Data: []byte(uiMainBrotli)},
	}
}

func TestUIErrorScrubsTheRepresentationHeaders(t *testing.T) {
	// Both error paths: the sibling vanishing between the ETag and the
	// body, and again between the body and the read for an FS that cannot
	// seek, where a Content-Length has been written too. By then the reply
	// is labelled as brotli, with a validator and a year-long immutable
	// cache directive, for a body that is not coming.
	for _, tc := range []struct {
		name  string
		ui    fs.FS
		after int
	}{
		{"seeking", uiFixture(), 2},
		{"read-it-all", noSeekFS{uiFixture()}, 3},
	} {
		s := &Server{ui: &openFailsAfter{FS: tc.ui, name: "assets/main.js.br", after: tc.after}}
		r := httptest.NewRequest("GET", "/assets/main.js", nil)
		r.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()
		s.serveUIFile(w, r, "assets/main.js")
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", tc.name, w.Code)
		}
		for _, k := range []string{"Cache-Control", "Content-Encoding", "Content-Length", "ETag", "Vary"} {
			if v := w.Header().Get(k); v != "" {
				t.Errorf("%s: the 404 kept %s: %q", tc.name, k, v)
			}
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type %q, want the error's own", tc.name, ct)
		}
	}
}

func TestUIServesAnFSThatCannotSeek(t *testing.T) {
	s := &Server{ui: noSeekFS{uiFixture()}}
	for _, tc := range []struct{ accept, coding, body string }{
		{"br", "br", uiMainBrotli},
		{"identity", "", uiMainJS},
	} {
		r := httptest.NewRequest("GET", "/assets/main.js", nil)
		r.Header.Set("Accept-Encoding", tc.accept)
		w := httptest.NewRecorder()
		s.serveUIFile(w, r, "assets/main.js")
		switch {
		case w.Code != 200:
			t.Errorf("%s: status %d", tc.accept, w.Code)
		case w.Header().Get("Content-Encoding") != tc.coding:
			t.Errorf("%s: Content-Encoding %q, want %q", tc.accept, w.Header().Get("Content-Encoding"), tc.coding)
		case w.Body.String() != tc.body:
			t.Errorf("%s: wrong body", tc.accept)
		case w.Header().Get("ETag") == "":
			t.Errorf("%s: no ETag", tc.accept)
		}
	}
}

func TestUIUnknownAssetIs404(t *testing.T) {
	ts, _ := newTestServer(t)
	// A stale hashed name is the case this exists for: after an upgrade the
	// page asks for a chunk that is not there, and 200 text/html turns that
	// into a MIME error in the console rather than a status anyone can read.
	for _, path := range []string{"/assets/index-gone.js", "/assets/nested/deeper.css", "/assets/"} {
		resp := uiGet(t, ts, "GET", path, "identity", nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
			t.Errorf("%s: Content-Type %q, want the error envelope", path, ct)
		}
	}
	// Everywhere else the client-side routes still resolve.
	for _, path := range []string{"/r/notes/some/note.md", "/nothing/like/an/asset", "/asset-ish.js"} {
		resp := uiGet(t, ts, "GET", path, "identity", nil)
		if resp.StatusCode != 200 || readAll(t, resp.Body) != "<html>app</html>" {
			t.Errorf("%s: %d, want the app shell", path, resp.StatusCode)
		}
	}
}

// TestUITypesCoverTheBundle holds the build and the daemon to one list. The
// build precompresses by extension and the daemon types by extension; if
// either list gains a kind the other does not know, a real asset goes out as
// application/octet-stream, which for a module script is a load failure.
// Only `make check` populates dist, so run it that way to mean anything.
func TestUITypesCoverTheBundle(t *testing.T) {
	bundle := ui.FS()
	if _, err := fs.Stat(bundle, "index.html"); err != nil {
		t.Skip("no built UI to check; make ui")
	}
	files := 0
	err := fs.WalkDir(bundle, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		// A dotfile the build carries rather than serves, .gitkeep among them.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		files++
		// A compressed sibling is typed from the file it is a copy of.
		source := name
		for _, e := range uiEncodings {
			source = strings.TrimSuffix(source, e.suffix)
		}
		if _, ok := uiTypes[strings.ToLower(path.Ext(source))]; !ok {
			t.Errorf("%s: uiTypes has no entry for %q, so it would be served as application/octet-stream",
				name, path.Ext(source))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("walked the bundle and found nothing")
	}
}

// TestTreeCarriesModificationTimes holds the half of M7-R3 the navigator's
// recency order is built on: every file node in the tree response states
// when the file was last modified, in Unix milliseconds, and no directory
// node does — the folder order is derived in the UI from the notes it can
// actually see, which under a tag filter is not the set the daemon would
// have aggregated over (davison/md-notes#116).
func TestTreeCarriesModificationTimes(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	ts, base := newTestServer(t)
	notes := filepath.Join(base, "notes")

	// Two distinct, known times, far enough apart that no clock skew or
	// filesystem timestamp granularity can put them in the wrong order.
	hello := time.Date(2026, 3, 4, 5, 6, 7, 800*int(time.Millisecond), time.UTC)
	linked := hello.Add(48 * time.Hour)
	for file, when := range map[string]time.Time{
		"hello.md":      hello,
		"sub/linked.md": linked,
	} {
		if err := os.Chtimes(filepath.Join(notes, filepath.FromSlash(file)), when, when); err != nil {
			t.Fatal(err)
		}
	}

	resp := do(t, ts, "GET", "/api/r/notes/tree", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var root map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}

	// The response is walked as plain JSON rather than decoded into
	// tree.Node, because "the directory node has no modified field" is a
	// statement about the wire format that a typed decode cannot tell from
	// "the field is there and zero".
	byPath := map[string]map[string]any{}
	var walk func(n map[string]any)
	walk = func(n map[string]any) {
		byPath[n["path"].(string)] = n
		children, _ := n["children"].([]any)
		for _, c := range children {
			walk(c.(map[string]any))
		}
	}
	walk(root)

	for path, want := range map[string]time.Time{"hello.md": hello, "sub/linked.md": linked} {
		node, ok := byPath[path]
		if !ok {
			t.Fatalf("%s is not in the tree: %v", path, byPath)
		}
		got, ok := node["modified"].(float64)
		if !ok {
			t.Errorf("%s has no modified time: %v", path, node)
			continue
		}
		if int64(got) != want.UnixMilli() {
			t.Errorf("%s modified = %d, want %d (%s)", path, int64(got), want.UnixMilli(), want)
		}
	}
	for _, dir := range []string{"", "sub"} {
		if _, ok := byPath[dir]["modified"]; ok {
			t.Errorf("directory %q carries a modified time: %v", dir, byPath[dir])
		}
	}
}
