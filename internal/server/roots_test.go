package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/token"
)

// Refusing a registration whose stated file is missing is about what ends
// up on disk as much as about what comes back over HTTP, so every case
// here reads the state file afterwards. M7-R1, adopting
// [#50](https://github.com/davison/md-notes/issues/50).

// newRootsServer is a daemon whose state file path the test knows, which
// the shared newTestServer deliberately keeps to itself.
func newRootsServer(t *testing.T, opts ...Option) (ts *httptest.Server, base, statePath string) {
	t.Helper()
	base = t.TempDir()
	notes := filepath.Join(base, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "hello.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statePath = filepath.Join(base, "state", "roots.json")
	reg, err := roots.New(notes, statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := token.Open(filepath.Join(base, "token"))
	if err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}}
	s := New(reg, port, ui, log.New(io.Discard, "", 0), append([]Option{WithToken(store)}, opts...)...)
	s.keepalive = 100 * time.Millisecond
	t.Cleanup(s.Close)
	ts = httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	servers[ts] = s
	return ts, base, statePath
}

// persisted is the recent roots the state file holds, in its order. A
// state file that was never written is no recents at all, which is what a
// refusal that wrote nothing leaves behind.
func persisted(t *testing.T, statePath string) []struct{ Slug, Path string } {
	t.Helper()
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Recent []struct{ Slug, Path string } `json:"recent"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("state file %s is not JSON: %v\n%s", statePath, err, data)
	}
	return state.Recent
}

// registeredSlugs is what GET /api/roots lists.
func registeredSlugs(t *testing.T, ts *httptest.Server) []string {
	t.Helper()
	resp := do(t, ts, "GET", "/api/roots", "", nil)
	var list struct{ Roots []roots.Root }
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(list.Roots))
	for _, r := range list.Roots {
		out = append(out, r.Slug)
	}
	return out
}

func refusalOf(t *testing.T, resp *http.Response) (code, message string) {
	t.Helper()
	var body struct{ Code, Error string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("refusal body is not JSON: %v", err)
	}
	return body.Code, body.Error
}

// A registration naming a file the daemon cannot find is refused whole:
// 404 not_found, nothing in the registry, nothing in the state file. This
// is the defect #50 reported — `file:///etc/no-such-note.md` registering
// /etc for ever — and the one case that has to hold for every shape of
// "cannot find it".
func TestAddRootRefusesAMissingFile(t *testing.T) {
	ts, base, statePath := newRootsServer(t)

	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(filepath.Join(proj, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"todo.md", filepath.Join("sub", "deep.md"), "notes.txt"} {
		if err := os.WriteFile(filepath.Join(proj, name), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory outside the one being registered, with a note in it, and
	// a symlink inside pointing at that note: neither is under this root,
	// however the caller spells the way there.
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(proj, "escape.md")); err != nil {
		t.Fatal(err)
	}
	// A directory is not a note either, however markdown its name is.
	if err := os.MkdirAll(filepath.Join(proj, "folder.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ name, file string }{
		{"missing", "no-such-note.md"},
		{"missing in a subdirectory", "sub/no-such-note.md"},
		{"not markdown", "notes.txt"},
		{"outside the directory, lexically", "../outside/secret.md"},
		{"outside the directory, absolutely", filepath.Join(outside, "secret.md")},
		{"outside the directory, through a symlink", "escape.md"},
		{"a directory, not a file", "folder.md"},
		{"the directory itself", "."},
		{"empty after cleaning", "./"},
	} {
		body := `{"path":"` + proj + `","file":"` + c.file + `"}`
		resp := do(t, ts, "POST", "/api/roots", body, map[string]string{"Content-Type": "application/json"})
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", c.name, resp.StatusCode)
			continue
		}
		if code, _ := refusalOf(t, resp); code != "not_found" {
			t.Errorf("%s: code %q, want not_found", c.name, code)
		}
		if slugs := registeredSlugs(t, ts); len(slugs) != 1 || slugs[0] != "notes" {
			t.Fatalf("%s: roots = %v, want the notes root alone", c.name, slugs)
		}
		if rec := persisted(t, statePath); len(rec) != 0 {
			t.Fatalf("%s: state file holds %v, want nothing", c.name, rec)
		}
	}
}

// The other half of the same rule: a file that is there registers the
// folder exactly as a registration with no file at all does.
func TestAddRootWithAFileThatIsThere(t *testing.T) {
	ts, base, statePath := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(filepath.Join(proj, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"todo.md", filepath.Join("sub", "deep.markdown")} {
		if err := os.WriteFile(filepath.Join(proj, name), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, file := range []string{
		"todo.md",
		"./todo.md",
		"sub/deep.markdown",
		filepath.Join(proj, "todo.md"),
	} {
		body := `{"path":"` + proj + `","file":"` + file + `"}`
		resp := do(t, ts, "POST", "/api/roots", body, map[string]string{"Content-Type": "application/json"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("file %q: status %d: %s", file, resp.StatusCode, readAll(t, resp.Body))
		}
		var added roots.Root
		if err := json.NewDecoder(resp.Body).Decode(&added); err != nil {
			t.Fatal(err)
		}
		if added.Slug != "proj" || added.Path != proj || added.Kind != roots.KindRecent {
			t.Fatalf("file %q: added = %+v", file, added)
		}
		// Registering the same folder again returns the same root, so the
		// list and the state file hold one entry however many times the
		// intercept has run.
		rec := persisted(t, statePath)
		if len(rec) != 1 || rec[0].Slug != "proj" || rec[0].Path != proj {
			t.Fatalf("file %q: state file holds %v", file, rec)
		}
	}
}

// A registration with no file is what `mdn open` sends, and it is
// unchanged: the daemon registers the folder without being told a note.
func TestAddRootWithoutAFileIsUnchanged(t *testing.T) {
	ts, base, statePath := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	// An empty folder, so nothing in it could be what admitted it.
	resp := do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, map[string]string{"Content-Type": "application/json"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if rec := persisted(t, statePath); len(rec) != 1 || rec[0].Path != proj {
		t.Fatalf("state file holds %v", rec)
	}
	// An empty file field is the same as none: a client that always sends
	// the field must not be refused for having nothing to put in it.
	other := filepath.Join(base, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	resp = do(t, ts, "POST", "/api/roots", `{"path":"`+other+`","file":""}`, map[string]string{"Content-Type": "application/json"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf(`"file":"": status %d: %s`, resp.StatusCode, readAll(t, resp.Body))
	}
}

// A folder that is already a root is not a way round the check: the file
// is verified before the registry is consulted, so a missing note is
// refused whether or not its folder is already served. (The extension
// reaches this through a symlinked alias of a registered folder, which is
// the one path that registers something already registered.)
func TestAddRootVerifiesBeforeTheRegistry(t *testing.T) {
	ts, base, statePath := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "todo.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, nil)

	resp := do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`","file":"gone.md"}`, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
	// And the root that was already there is untouched.
	if rec := persisted(t, statePath); len(rec) != 1 || rec[0].Slug != "proj" {
		t.Fatalf("state file holds %v, want the root that was already registered", rec)
	}
}

// Registering with a file is loopback-only too: the field changes what the
// daemon checks before it registers, not who may ask it to.
func TestAddRootWithAFileIsLoopbackOnly(t *testing.T) {
	ts, base, _ := newRootsServer(t, WithTailnetHost(tailnetName))
	tok := daemonToken(t, base)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "todo.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := tdo(t, ts, "POST", "/api/roots", `{"path":"`+proj+`","file":"todo.md"}`,
		bearerHeader(tok, "Content-Type", "application/json"))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	if code := guardCode(t, resp); code != "loopback_only" {
		t.Errorf("code %q, want loopback_only", code)
	}
	if slugs := registeredSlugs(t, ts); len(slugs) != 1 {
		t.Errorf("roots = %v, want the notes root alone", slugs)
	}
}
