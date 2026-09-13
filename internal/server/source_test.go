package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davison/md-notes/internal/source"
)

const sourceURL = "/api/r/notes/source/hello.md"

func sourceRead(t *testing.T, ts *httptest.Server) source.Note {
	t.Helper()
	resp := do(t, ts, "GET", sourceURL, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("read status=%d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing no-store")
	}
	var note source.Note
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		t.Fatal(err)
	}
	return note
}

func sourceBody(t *testing.T, text, revision string) string {
	t.Helper()
	data, err := json.Marshal(source.Note{Source: text, Revision: revision})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSourceAPI(t *testing.T) {
	ts, base := newTestServer(t)
	before := sourceRead(t, ts)
	text := "---\r\ntitle: 'preserve me'\r\n---\r\n\r\n## 📝 café\r\nno final newline"
	body := sourceBody(t, text, before.Revision)
	hdr := map[string]string{"Content-Type": "application/json", "Origin": "http://localhost:7337"}
	resp := do(t, ts, "PUT", sourceURL, body, hdr)
	if resp.StatusCode != 200 {
		t.Fatalf("save = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var saved source.Note
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.Source != text || saved.Revision == "" || saved.Revision == before.Revision {
		t.Fatalf("save = %+v", saved)
	}
	if got := sourceRead(t, ts); got != saved {
		t.Fatalf("read %+v; want %+v", got, saved)
	}
	data, err := os.ReadFile(filepath.Join(base, "notes", "hello.md"))
	if err != nil || string(data) != text {
		t.Fatalf("disk %q, %v", data, err)
	}
	resp = do(t, ts, "PUT", sourceURL, body, hdr)
	if resp.StatusCode != 409 {
		t.Fatalf("stale save = %d", resp.StatusCode)
	}
	if err := os.Remove(filepath.Join(base, "notes", "hello.md")); err != nil {
		t.Fatal(err)
	}
	resp = do(t, ts, "PUT", sourceURL, sourceBody(t, "draft", saved.Revision), hdr)
	if resp.StatusCode != 404 {
		t.Fatalf("deleted save = %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(base, "notes", "hello.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted file recreated: %v", err)
	}
}

func TestSaveSourceRequestValidation(t *testing.T) {
	ts, base := newTestServer(t)
	before := sourceRead(t, ts)
	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"empty", "", 400, "invalid_body"},
		{"malformed", "{", 400, "invalid_body"},
		{"missing-source", `{"revision":"x"}`, 400, "invalid_body"},
		{"null-source", `{"source":null,"revision":"x"}`, 400, "invalid_body"},
		{"numeric-source", `{"source":12,"revision":"x"}`, 400, "invalid_body"},
		{"missing-revision", `{"source":"draft"}`, 428, "revision_required"},
		{"null-revision", `{"source":"draft","revision":null}`, 428, "revision_required"},
		{"empty-revision", `{"source":"draft","revision":""}`, 428, "revision_required"},
		{"wrong-revision-type", `{"source":"draft","revision":12}`, 400, "invalid_body"},
		{"trailing-json", `{"source":"draft","revision":"x"}{}`, 400, "invalid_body"},
		{"invalid-utf8", "{\"source\":\"\xff\",\"revision\":\"x\"}", 400, "invalid_body"},
		{"unpaired-surrogate", `{"source":"\ud800","revision":"x"}`, 400, "invalid_body"},
		{"unpaired-low-surrogate", `{"source":"\udfff","revision":"x"}`, 400, "invalid_body"},
		{"stale", sourceBody(t, "draft", "obsolete"), 409, "conflict"},
		{"too-large-source", sourceBody(t, strings.Repeat("x", source.MaxBytes+1), before.Revision), 413, "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, ts, "PUT", sourceURL, tc.body, map[string]string{"Content-Type": "application/json"})
			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status || body["code"] != tc.code {
				t.Fatalf("got %d %v; want %d %s", resp.StatusCode, body, tc.status, tc.code)
			}
		})
	}
	resp := do(t, ts, "PUT", sourceURL, sourceBody(t, "draft", before.Revision), nil)
	if resp.StatusCode != 415 {
		t.Fatalf("missing content type=%d", resp.StatusCode)
	}
	data, err := os.ReadFile(filepath.Join(base, "notes", "hello.md"))
	if err != nil || string(data) != before.Source {
		t.Fatalf("invalid request changed file: %q %v", data, err)
	}
}

func TestSaveSourceBodyLimit(t *testing.T) {
	ts, _ := newTestServer(t)
	s := serverOf(t, ts)
	// Exercise the encoded-body bound without allocating a huge client string.
	// The target is a path, not an absolute URL: absolute form is what a
	// client sends to a forward proxy, and the guard refuses it.
	req := httptest.NewRequest("PUT", sourceURL, io.LimitReader(repeatByte(' '), 6*source.MaxBytes+1025))
	req.Host = "localhost:7337"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 413 {
		t.Fatalf("oversized body=%d: %s", rec.Code, rec.Body.String())
	}
}

type repeatByte byte

func (b repeatByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func TestSourceConfinementAndGuard(t *testing.T) {
	ts, base := newTestServer(t)
	before := sourceRead(t, ts)
	if err := os.WriteFile(filepath.Join(base, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "secret.md"), filepath.Join(base, "notes", "escape.md")); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PUT"} {
		for _, tc := range []struct {
			path   string
			status int
		}{
			{"/api/r/notes/source/%2e%2e%2fsecret.md", 403},
			{"/api/r/notes/source/escape.md", 403},
			{"/api/r/missing/source/hello.md", 404},
			{"/api/r/notes/source/absent.md", 404},
			{"/api/r/notes/source/sub/pic.png", 404},
		} {
			resp := do(t, ts, method, tc.path, sourceBody(t, "draft", before.Revision), map[string]string{"Content-Type": "application/json"})
			if resp.StatusCode != tc.status {
				t.Errorf("%s %s = %d; want %d", method, tc.path, resp.StatusCode, tc.status)
			}
		}
	}
	for _, hdr := range []map[string]string{
		{"Host": "evil.example:7337"},
		{"Origin": "http://evil.example"},
		{"Origin": "null"},
	} {
		hdr["Content-Type"] = "application/json"
		resp := do(t, ts, "PUT", sourceURL, sourceBody(t, "draft", before.Revision), hdr)
		if resp.StatusCode != 403 {
			t.Errorf("guard %v = %d", hdr, resp.StatusCode)
		}
	}
	data, err := os.ReadFile(filepath.Join(base, "secret.md"))
	if err != nil || string(data) != "secret" {
		t.Fatalf("escaped root: %q %v", data, err)
	}
	if got := sourceRead(t, ts); got != before {
		t.Fatalf("guard changed note: %+v", got)
	}
}

func TestSourceErrorResponses(t *testing.T) {
	for _, kind := range []string{"directory", "encoding", "permission"} {
		t.Run(kind, func(t *testing.T) {
			ts, base := newTestServer(t)
			file := filepath.Join(base, "notes", "hello.md")
			before := sourceRead(t, ts)
			status, code := 422, "unsupported_source"
			switch kind {
			case "directory":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(file, 0o755); err != nil {
					t.Fatal(err)
				}
			case "encoding":
				if err := os.WriteFile(file, []byte{0xff}, 0o644); err != nil {
					t.Fatal(err)
				}
			case "permission":
				if err := os.Chmod(file, 0o444); err != nil {
					t.Fatal(err)
				}
				before = sourceRead(t, ts)
				status, code = 403, "permission_denied"
			}
			resp := do(t, ts, "PUT", sourceURL, sourceBody(t, "draft", before.Revision), map[string]string{"Content-Type": "application/json"})
			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != status || body["code"] != code {
				t.Fatalf("got %d %v; want %d %s", resp.StatusCode, body, status, code)
			}
		})
	}
}

func TestSaveSourceUnicodeEscapes(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, tc := range []struct{ encoded, want string }{
		{`"\ud83d\udcdd"`, "📝"},
		{`"\\ud800"`, `\ud800`},
		{`"\u0000\r\n"`, "\x00\r\n"},
	} {
		before := sourceRead(t, ts)
		body := `{"source":` + tc.encoded + `,"revision":"` + before.Revision + `"}`
		resp := do(t, ts, "PUT", sourceURL, body, map[string]string{"Content-Type": "application/json"})
		if resp.StatusCode != 200 {
			t.Fatalf("unicode save = %d: %s", resp.StatusCode, readAll(t, resp.Body))
		}
		if note := sourceRead(t, ts); note.Source != tc.want {
			t.Fatalf("source = %q; want %q", note.Source, tc.want)
		}
	}
}
