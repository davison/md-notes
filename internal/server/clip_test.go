package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clipBody is the JSON the extension posts.
func clipBody(title, markdown string) string {
	b, _ := json.Marshal(map[string]string{
		"url":      "https://example.com/article",
		"title":    title,
		"markdown": markdown,
		"kind":     "page",
	})
	return string(b)
}

func TestClipCreatesANote(t *testing.T) {
	ts, base := newTestServer(t)
	// The extension writes from its own origin, which only the token makes
	// acceptable.
	resp := do(t, ts, "POST", "/api/clip", clipBody("The Cost of Abstraction", "# Heading\n\nBody text.\n"),
		bearerHeader(daemonToken(t, base),
			"Content-Type", "application/json",
			"Origin", "chrome-extension://abcdefghijklmnop"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var created struct{ Root, Path string }
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	want := time.Now().Format("2006-01-02") + "-the-cost-of-abstraction.md"
	if created.Root != "notes" || created.Path != "clips/"+want {
		t.Fatalf("created = %+v, want the notes root and clips/%s", created, want)
	}
	data, err := os.ReadFile(filepath.Join(base, "notes", filepath.FromSlash(created.Path)))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, fragment := range []string{
		"---\ntitle: The Cost of Abstraction\n",
		"source: https://example.com/article\n",
		"tags: [clip]\n",
		"---\n\n# Heading\n\nBody text.\n",
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("note lacks %q:\n%s", fragment, body)
		}
	}
	// The created note is reachable through the API that serves it.
	note := do(t, ts, "GET", "/api/r/notes/note/"+created.Path, "", nil)
	if note.StatusCode != 200 {
		t.Errorf("rendering the new note: status %d", note.StatusCode)
	}
}

func TestClipRequiresTheToken(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	json := map[string]string{"Content-Type": "application/json"}
	cases := []struct {
		name string
		hdr  map[string]string
		want int
	}{
		{"no credential at all, same origin", json, http.StatusUnauthorized},
		{"wrong token", bearerHeader(tok+"x", "Content-Type", "application/json"), http.StatusUnauthorized},
		{"the token", bearerHeader(tok, "Content-Type", "application/json"), http.StatusCreated},
	}
	for _, c := range cases {
		resp := do(t, ts, "POST", "/api/clip", clipBody("Auth "+c.name, "x\n"), c.hdr)
		if resp.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.want)
		}
		if c.want == http.StatusUnauthorized {
			var body struct{ Code, Error string }
			decodeJSON(t, resp, &body)
			if body.Code != "unauthorized" || body.Error == "" {
				t.Errorf("%s: body = %+v", c.name, body)
			}
		}
	}
	// Nothing was written by the refused attempts.
	entries, err := os.ReadDir(filepath.Join(base, "notes", "clips"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("%d notes in the clips directory, want the one accepted clip", len(entries))
	}
}

func TestClipErrors(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	auth := func(extra ...string) map[string]string { return bearerHeader(tok, extra...) }
	cases := []struct {
		name   string
		body   string
		hdr    map[string]string
		status int
		code   string
	}{
		{"no content type", clipBody("x", "y"), auth(), 415, "invalid_body"},
		{"wrong content type", clipBody("x", "y"), auth("Content-Type", "text/plain"), 415, "invalid_body"},
		{"not JSON", "{", auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"no url", `{"markdown":"x","kind":"page"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"relative url", `{"url":"/a","markdown":"x","kind":"page"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"no markdown", `{"url":"https://e.com/","kind":"page"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"empty markdown", `{"url":"https://e.com/","markdown":"","kind":"page"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"no kind", `{"url":"https://e.com/","markdown":"x"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"unknown kind", `{"url":"https://e.com/","markdown":"x","kind":"whole-site"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
		{"lone surrogate", `{"url":"https://e.com/","markdown":"\ud800","kind":"page"}`, auth("Content-Type", "application/json"), 400, "invalid_body"},
	}
	for _, c := range cases {
		resp := do(t, ts, "POST", "/api/clip", c.body, c.hdr)
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d (%s)", c.name, resp.StatusCode, c.status, readAll(t, resp.Body))
			continue
		}
		var body struct{ Code, Error string }
		decodeJSON(t, resp, &body)
		if body.Code != c.code || body.Error == "" {
			t.Errorf("%s: body = %+v, want code %s and a message", c.name, body, c.code)
		}
	}
}

func TestClipAcceptsASelection(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	body := `{"url":"https://example.com/a","title":"Sel","markdown":"> quoted\n","kind":"selection"}`
	resp := do(t, ts, "POST", "/api/clip", body, bearerHeader(tok, "Content-Type", "application/json"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
}

func TestClipRefusesAnOversizeBody(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	// Above the markdown limit but inside the request limit: the field is
	// what is too large, and the envelope says so.
	big := clipBody("Big", strings.Repeat("m", 8<<20+1))
	resp := do(t, ts, "POST", "/api/clip", big, bearerHeader(tok, "Content-Type", "application/json"))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize markdown: status %d, want 413", resp.StatusCode)
	}
	var body struct{ Code string }
	decodeJSON(t, resp, &body)
	if body.Code != "too_large" {
		t.Errorf("code = %q, want too_large", body.Code)
	}
	// Above the request limit: refused before the JSON is looked at.
	huge := strings.Repeat("m", 6*(8<<20)+70<<10)
	resp = do(t, ts, "POST", "/api/clip", `{"url":"https://e.com/","kind":"page","markdown":"`+huge+`"}`,
		bearerHeader(tok, "Content-Type", "application/json"))
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize request: status %d, want 413", resp.StatusCode)
	}
}

func TestClipDeduplicatesNames(t *testing.T) {
	ts, base := newTestServer(t)
	tok := daemonToken(t, base)
	day := time.Now().Format("2006-01-02")
	for i, want := range []string{
		"clips/" + day + "-repeat.md",
		"clips/" + day + "-repeat-2.md",
		"clips/" + day + "-repeat-3.md",
	} {
		resp := do(t, ts, "POST", "/api/clip", clipBody("Repeat", "x\n"),
			bearerHeader(tok, "Content-Type", "application/json"))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("clip %d: status %d", i+1, resp.StatusCode)
		}
		var created struct{ Path string }
		decodeJSON(t, resp, &created)
		if created.Path != want {
			t.Errorf("clip %d: path %q, want %q", i+1, created.Path, want)
		}
	}
}

func TestClipRefusesAClipsDirOutsideTheRoot(t *testing.T) {
	ts, base := newTestServer(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "notes", "clips")); err != nil {
		t.Fatal(err)
	}
	resp := do(t, ts, "POST", "/api/clip", clipBody("Escaping", "x\n"),
		bearerHeader(daemonToken(t, base), "Content-Type", "application/json"))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	var body struct{ Code string }
	decodeJSON(t, resp, &body)
	if body.Code != "outside_root" {
		t.Errorf("code = %q, want outside_root", body.Code)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote %d entries outside the notes root", len(entries))
	}
}

func TestClipHonoursTheConfiguredDirectory(t *testing.T) {
	ts, base := newTestServer(t)
	serverOf(t, ts).clipsDir = filepath.Join("inbox", "web")
	resp := do(t, ts, "POST", "/api/clip", clipBody("Configured", "x\n"),
		bearerHeader(daemonToken(t, base), "Content-Type", "application/json"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var created struct{ Path string }
	decodeJSON(t, resp, &created)
	if !strings.HasPrefix(created.Path, "inbox/web/") {
		t.Errorf("path = %q, want it under the configured directory", created.Path)
	}
}

// The whole point of writing through the daemon is that the page sees the
// note without a refresh.
func TestClipAppearsOnTheEventStream(t *testing.T) {
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
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })

	clip := func(title string) string {
		t.Helper()
		resp := do(t, ts, "POST", "/api/clip", clipBody(title, "x\n"),
			bearerHeader(daemonToken(t, base), "Content-Type", "application/json"))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
		}
		var created struct{ Path string }
		decodeJSON(t, resp, &created)
		return created.Path
	}

	// The first clip of an installation creates the clips directory, which
	// the root's own watch reports; the page refetches the tree on any
	// batch, so the note is in the navigator either way.
	first := clip("Live Update")
	next(func(l string) bool {
		return strings.Contains(l, `"clips"`) || strings.Contains(l, fmt.Sprintf("%q", first))
	})
	// Once the directory exists it is watched, and the next clip is named.
	second := clip("Named In The Batch")
	next(func(l string) bool { return strings.Contains(l, fmt.Sprintf("%q", second)) })
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
}
