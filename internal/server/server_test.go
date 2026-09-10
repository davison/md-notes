package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	ui := fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	store, _, err := token.Open(filepath.Join(base, "token"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(reg, port, ui, log.New(io.Discard, "", 0), WithToken(store))
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
