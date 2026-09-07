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
	"github.com/davison/md-notes/internal/tree"
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
	os.WriteFile(filepath.Join(notes, "sub", "linked.md"), []byte("---\ntitle: Linked\n---\n[back](../hello.md) ![p](pic.png)\n"), 0o644)
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
	s := New(reg, port, ui, log.New(io.Discard, "", 0))
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
	var body struct {
		Hits      []search.Hit
		Truncated bool
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Hits) != 1 || body.Hits[0].Path != "hello.md" || body.Hits[0].Line != 1 || body.Truncated {
		t.Fatalf("hits = %+v", body)
	}
	for path, want := range map[string]int{
		"/api/r/notes/search":            400,
		"/api/r/notes/search?q=%20":      400,
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
