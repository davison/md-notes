package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// created decodes the 201 body of a create.
type createdBody struct {
	Root     string `json:"root"`
	Path     string `json:"path"`
	Source   string `json:"source"`
	Revision string `json:"revision"`
}

func jsonHeader() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

// listing is every path under dir, relative to it, so a test can say that
// a refusal changed nothing rather than only that the one file it looked
// at is still there.
func listing(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func TestCreateNote(t *testing.T) {
	ts, base := newTestServer(t)
	resp := do(t, ts, "POST", "/api/r/notes/source/new.md", `{"source":"# new\n"}`, jsonHeader())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if loc := resp.Header.Get("Location"); loc != "/api/r/notes/source/new.md" {
		t.Errorf("Location = %q", loc)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing no-store")
	}
	var body createdBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Root != "notes" || body.Path != "new.md" || body.Source != "# new\n" || body.Revision == "" {
		t.Fatalf("body = %+v", body)
	}
	data, err := os.ReadFile(filepath.Join(base, "notes", "new.md"))
	if err != nil || string(data) != "# new\n" {
		t.Fatalf("disk = %q, %v", data, err)
	}

	// The revision the create returned is the one a first save is checked
	// against, so the editor can open the new note and save it without
	// reading it again.
	save := do(t, ts, "PUT", "/api/r/notes/source/new.md", sourceBody(t, "# saved\n", body.Revision), jsonHeader())
	if save.StatusCode != http.StatusOK {
		t.Fatalf("save after create = %d: %s", save.StatusCode, readAll(t, save.Body))
	}
}

// A note created without a body is an empty note: the create control asks
// for a name, not for content.
func TestCreateNoteWithoutABody(t *testing.T) {
	ts, base := newTestServer(t)
	for _, c := range []struct{ name, path, body string }{
		{"no body at all", "/api/r/notes/source/blank.md", ""},
		{"an object with no source", "/api/r/notes/source/blank2.md", "{}"},
		{"a null source", "/api/r/notes/source/blank3.md", `{"source":null}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			hdr := map[string]string{}
			if c.body != "" {
				hdr = jsonHeader()
			}
			resp := do(t, ts, "POST", c.path, c.body, hdr)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("create = %d: %s", resp.StatusCode, readAll(t, resp.Body))
			}
			name := strings.TrimPrefix(c.path, "/api/r/notes/source/")
			data, err := os.ReadFile(filepath.Join(base, "notes", name))
			if err != nil || len(data) != 0 {
				t.Fatalf("disk = %q, %v", data, err)
			}
		})
	}
}

// Missing parent directories are created, and the path the response names
// is the cleaned one.
func TestCreateNoteMakesParentDirectories(t *testing.T) {
	ts, base := newTestServer(t)
	resp := do(t, ts, "POST", "/api/r/notes/source/a/b/c/deep.md", `{"source":"x"}`, jsonHeader())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if _, err := os.Stat(filepath.Join(base, "notes", "a", "b", "c", "deep.md")); err != nil {
		t.Fatal(err)
	}
	// A path that walks back inside the root is cleaned, and the client is
	// told the name the note actually has.
	resp = do(t, ts, "POST", "/api/r/notes/source/sub/../tidy.md", `{"source":"x"}`, jsonHeader())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var body createdBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Path != "tidy.md" {
		t.Fatalf("path = %q, want tidy.md", body.Path)
	}
	if _, err := os.Stat(filepath.Join(base, "notes", "tidy.md")); err != nil {
		t.Fatal(err)
	}
}

// The refusal set M5-R2 names, and what each refusal answers. Nothing in
// the root changes for any of them.
func TestCreateNoteRefusals(t *testing.T) {
	ts, base := newTestServer(t)
	notes := filepath.Join(base, "notes")
	if err := os.WriteFile(filepath.Join(base, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "secret.md"), filepath.Join(notes, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(notes, "out")); err != nil {
		t.Fatal(err)
	}
	before := listing(t, base)

	for _, c := range []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"an existing file", "/api/r/notes/source/hello.md", `{"source":"x"}`, 409, "exists"},
		{"an existing directory", "/api/r/notes/source/sub", `{"source":"x"}`, 404, "not_markdown"},
		{"an empty name", "/api/r/notes/source/", `{"source":"x"}`, 400, "invalid_path"},
		{"a blank name", "/api/r/notes/source/%20", `{"source":"x"}`, 400, "invalid_path"},
		{"a hidden name", "/api/r/notes/source/.hidden.md", `{"source":"x"}`, 400, "invalid_path"},
		{"a name that is only an extension", "/api/r/notes/source/.md", `{"source":"x"}`, 400, "invalid_path"},
		{"a hidden folder", "/api/r/notes/source/.git/x.md", `{"source":"x"}`, 400, "invalid_path"},
		{"a control character", "/api/r/notes/source/bad%00name.md", `{"source":"x"}`, 400, "invalid_path"},
		{"a non-markdown name", "/api/r/notes/source/notes.txt", `{"source":"x"}`, 404, "not_markdown"},
		{"no extension at all", "/api/r/notes/source/notes", `{"source":"x"}`, 404, "not_markdown"},
		{"an escape by path", "/api/r/notes/source/%2e%2e%2fsecret.md", `{"source":"x"}`, 403, "outside_root"},
		{"an escape by symlink", "/api/r/notes/source/escape.md", `{"source":"x"}`, 403, "outside_root"},
		{"an escape through a linked folder", "/api/r/notes/source/out/new.md", `{"source":"x"}`, 403, "outside_root"},
		{"an escape through a folder not made yet", "/api/r/notes/source/out/deeper/new.md", `{"source":"x"}`, 403, "outside_root"},
		{"an unknown root", "/api/r/missing/source/new.md", `{"source":"x"}`, 404, "not_found"},
		{"a body that is not JSON", "/api/r/notes/source/new.md", `{`, 400, "invalid_body"},
		{"a source that is not a string", "/api/r/notes/source/new.md", `{"source":12}`, 400, "invalid_body"},
		{"invalid UTF-8", "/api/r/notes/source/new.md", "{\"source\":\"\xff\"}", 400, "invalid_body"},
		{"an unpaired surrogate", "/api/r/notes/source/new.md", `{"source":"\ud800"}`, 400, "invalid_body"},
	} {
		t.Run(c.name, func(t *testing.T) {
			resp := do(t, ts, "POST", c.path, c.body, jsonHeader())
			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("body is not the {code, error} envelope: %v", err)
			}
			if resp.StatusCode != c.status || body["code"] != c.code {
				t.Fatalf("got %d %v; want %d %s", resp.StatusCode, body, c.status, c.code)
			}
			if body["error"] == "" {
				t.Errorf("code %q carries no message", body["code"])
			}
		})
	}
	// A body with no declared type is refused before the path is touched.
	if resp := do(t, ts, "POST", "/api/r/notes/source/new.md", `{"source":"x"}`, nil); resp.StatusCode != 415 {
		t.Errorf("untyped body = %d, want 415", resp.StatusCode)
	}
	if got := listing(t, base); !equal(got, before) {
		t.Fatalf("a refusal changed the root:\n got %v\nwant %v", got, before)
	}
	if data, err := os.ReadFile(filepath.Join(base, "secret.md")); err != nil || string(data) != "secret" {
		t.Fatalf("escaped the root: %q %v", data, err)
	}
}

// A create is refused where a save is refused: the guard is the same one.
func TestCreateNoteIsGuarded(t *testing.T) {
	ts, base := newTestServer(t)
	before := listing(t, base)
	for _, hdr := range []map[string]string{
		{"Host": "evil.example:7337"},
		{"Origin": "http://evil.example"},
		{"Origin": "null"},
	} {
		hdr["Content-Type"] = "application/json"
		for _, method := range []string{"POST", "DELETE"} {
			resp := do(t, ts, method, "/api/r/notes/source/hello.md", `{"source":"x"}`, hdr)
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s %v = %d, want 403", method, hdr, resp.StatusCode)
			}
		}
	}
	if got := listing(t, base); !equal(got, before) {
		t.Fatalf("the guard let a write through:\n got %v\nwant %v", got, before)
	}
}

func TestDeleteNote(t *testing.T) {
	ts, base := newTestServer(t)
	before := listing(t, base)
	resp := do(t, ts, "DELETE", "/api/r/notes/source/hello.md", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing no-store")
	}
	if body := readAll(t, resp.Body); body != "" {
		t.Errorf("204 carried a body: %q", body)
	}
	if _, err := os.Stat(filepath.Join(base, "notes", "hello.md")); !os.IsNotExist(err) {
		t.Fatalf("file survived: %v", err)
	}
	if got := do(t, ts, "GET", "/api/r/notes/source/hello.md", "", nil); got.StatusCode != 404 {
		t.Errorf("read after delete = %d, want 404", got.StatusCode)
	}
	// Exactly one file went.
	want := make([]string, 0, len(before))
	for _, p := range before {
		if p != filepath.Join("notes", "hello.md") {
			want = append(want, p)
		}
	}
	if got := listing(t, base); !equal(got, want) {
		t.Fatalf("delete removed more than the named note:\n got %v\nwant %v", got, want)
	}
	// A second delete of the same note is a 404, not a silent success.
	if again := do(t, ts, "DELETE", "/api/r/notes/source/hello.md", "", nil); again.StatusCode != 404 {
		t.Errorf("second delete = %d, want 404", again.StatusCode)
	}
	// A note in a subdirectory goes without its directory.
	if resp := do(t, ts, "DELETE", "/api/r/notes/source/sub/linked.md", "", nil); resp.StatusCode != 204 {
		t.Fatalf("delete in a subdirectory = %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(base, "notes", "sub")); err != nil {
		t.Fatalf("the directory went with the note: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "notes", "sub", "pic.png")); err != nil {
		t.Fatalf("a sibling went with the note: %v", err)
	}
}

// The refusal set M5-R3 names. The gate is that nothing but the one named
// markdown file can go, so every case checks the whole tree afterwards.
func TestDeleteNoteRefusals(t *testing.T) {
	ts, base := newTestServer(t)
	notes := filepath.Join(base, "notes")
	if err := os.WriteFile(filepath.Join(base, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "secret.md"), filepath.Join(notes, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(notes, "hello.md"), filepath.Join(notes, "inside.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, filepath.Join(notes, "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(notes, "folder.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "readonly.md"), []byte("ro"), 0o444); err != nil {
		t.Fatal(err)
	}
	before := listing(t, base)

	for _, c := range []struct {
		name, path string
		status     int
		code       string
	}{
		{"a directory named like a note", "/api/r/notes/source/folder.md", 422, "unsupported_source"},
		{"a directory", "/api/r/notes/source/sub", 404, "not_markdown"},
		{"a read-only file", "/api/r/notes/source/readonly.md", 403, "permission_denied"},
		{"an empty name", "/api/r/notes/source/", 400, "invalid_path"},
		{"a hidden name", "/api/r/notes/source/.hidden.md", 400, "invalid_path"},
		{"a control character", "/api/r/notes/source/bad%00name.md", 400, "invalid_path"},
		{"a non-markdown file", "/api/r/notes/source/sub/pic.png", 404, "not_markdown"},
		{"an escape by path", "/api/r/notes/source/%2e%2e%2fsecret.md", 403, "outside_root"},
		{"an escape by symlink", "/api/r/notes/source/escape.md", 403, "outside_root"},
		{"an escape through a linked folder", "/api/r/notes/source/out/secret.md", 403, "outside_root"},
		{"a symlink inside the root", "/api/r/notes/source/inside.md", 422, "unsupported_source"},
		{"an absent note", "/api/r/notes/source/absent.md", 404, "not_found"},
		{"an unknown root", "/api/r/missing/source/hello.md", 404, "not_found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			resp := do(t, ts, "DELETE", c.path, "", nil)
			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("body is not the {code, error} envelope: %v", err)
			}
			if resp.StatusCode != c.status || body["code"] != c.code {
				t.Fatalf("got %d %v; want %d %s", resp.StatusCode, body, c.status, c.code)
			}
			if body["error"] == "" {
				t.Errorf("code %q carries no message", body["code"])
			}
			if got := listing(t, base); !equal(got, before) {
				t.Fatalf("a refusal removed something:\n got %v\nwant %v", got, before)
			}
		})
	}
	if data, err := os.ReadFile(filepath.Join(base, "secret.md")); err != nil || string(data) != "secret" {
		t.Fatalf("reached outside the root: %q %v", data, err)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The navigator needs no special handling for either verb: the watcher
// already reports a file appearing and a file going, so both reach the
// page on the events stream it is already listening to.
func TestEventsStreamCarriesCreateAndDelete(t *testing.T) {
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events = %d", resp.StatusCode)
	}
	next := sseReader(t, resp.Body)
	next(func(l string) bool { return l == ": connected" })

	if got := do(t, ts, "POST", "/api/r/notes/source/sub/fresh.md", `{"source":"# fresh\n"}`, jsonHeader()); got.StatusCode != 201 {
		t.Fatalf("create = %d: %s", got.StatusCode, readAll(t, got.Body))
	}
	next(func(l string) bool { return strings.Contains(l, `"sub/fresh.md"`) })

	if got := do(t, ts, "DELETE", "/api/r/notes/source/sub/fresh.md", "", nil); got.StatusCode != 204 {
		t.Fatalf("delete = %d: %s", got.StatusCode, readAll(t, got.Body))
	}
	next(func(l string) bool { return strings.Contains(l, `"sub/fresh.md"`) })
	if _, err := os.Stat(filepath.Join(base, "notes", "sub", "fresh.md")); !os.IsNotExist(err) {
		t.Fatalf("file survived: %v", err)
	}
}

// A root registered at runtime with `mdn open` takes create and delete on
// the same terms as the notes root — the terms the source save already
// takes. Recorded as a Decision on #76.
func TestCreateAndDeleteInARootOpenedAtRuntime(t *testing.T) {
	ts, _ := newTestServer(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "there.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := do(t, ts, "POST", "/api/roots", `{"path":"`+other+`"}`, jsonHeader())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add root = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	var root struct{ Slug string }
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, ts, "POST", "/api/r/"+root.Slug+"/source/made.md", `{"source":"x"}`, jsonHeader()); resp.StatusCode != 201 {
		t.Fatalf("create in a recent root = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if _, err := os.Stat(filepath.Join(other, "made.md")); err != nil {
		t.Fatal(err)
	}
	if resp := do(t, ts, "DELETE", "/api/r/"+root.Slug+"/source/there.md", "", nil); resp.StatusCode != 204 {
		t.Fatalf("delete in a recent root = %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if _, err := os.Stat(filepath.Join(other, "there.md")); !os.IsNotExist(err) {
		t.Fatalf("file survived: %v", err)
	}
	// The root itself is not reachable as a note, in either verb.
	for _, method := range []string{"POST", "DELETE"} {
		if resp := do(t, ts, method, "/api/r/"+root.Slug+"/source/", "", nil); resp.StatusCode != 400 {
			t.Errorf("%s at the root = %d, want 400", method, resp.StatusCode)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("the root itself went: %v", err)
	}
}
