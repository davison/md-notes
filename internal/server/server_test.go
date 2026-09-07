package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/davison/md-notes/internal/roots"
)

const port = 7337

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	base := t.TempDir()
	notes := filepath.Join(base, "notes")
	os.MkdirAll(filepath.Join(notes, "sub"), 0o755)
	os.WriteFile(filepath.Join(notes, "hello.md"), []byte("# hi\n"), 0o644)
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
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
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
