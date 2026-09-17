package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/token"
	"github.com/davison/md-notes/internal/tree"
)

// The registry's two new answers — refusing a registration whose stated
// file is missing, and unregistering a recent root — are about what ends
// up on disk as much as what comes back over HTTP, so every case here
// reads the state file afterwards. M7-R1 and M7-R2, adopting
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

// Removing a recent root takes it out of the registry and the state file,
// and takes nothing off disk. M7-R2.
func TestDeleteRoot(t *testing.T) {
	ts, base, statePath := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(proj, "todo.md")
	if err := os.WriteFile(note, []byte("# todo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, nil)
	if rec := persisted(t, statePath); len(rec) != 1 {
		t.Fatalf("before the delete the state file holds %v", rec)
	}

	resp := do(t, ts, "DELETE", "/api/roots/proj", "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if body := readAll(t, resp.Body); body != "" {
		t.Errorf("204 carried a body: %q", body)
	}
	if slugs := registeredSlugs(t, ts); len(slugs) != 1 || slugs[0] != "notes" {
		t.Errorf("roots = %v, want the notes root alone", slugs)
	}
	if rec := persisted(t, statePath); len(rec) != 0 {
		t.Errorf("state file still holds %v", rec)
	}
	// Unregistering is not deleting: the folder and its notes are there.
	if _, err := os.Stat(note); err != nil {
		t.Errorf("the note went with the root: %v", err)
	}
	// Its routes are gone with it.
	if resp := do(t, ts, "GET", "/api/r/proj/tree", "", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("tree of a removed root: status %d, want 404", resp.StatusCode)
	}
	// And it stays gone across a restart, which is the state file's job.
	reg, err := roots.New(filepath.Join(base, "notes"), statePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if list := reg.List(); len(list) != 1 || list[0].Kind != roots.KindNotes {
		t.Errorf("after a restart roots = %+v", list)
	}
}

// The refusals, each leaving the registry and the state file as they were.
func TestDeleteRootRefusals(t *testing.T) {
	ts, base, statePath := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, nil)

	for _, c := range []struct {
		name, slug, code string
		status           int
	}{
		// The notes root is the daemon's configuration — `--root`, or the
		// config file — and not a registration to undo. Removing it would
		// leave a daemon serving nothing, until the next restart put it
		// straight back.
		{"the notes root", "notes", "notes_root", http.StatusForbidden},
		{"an unknown slug", "nope", "not_found", http.StatusNotFound},
		{"a slug that is nearly one", "proj-2", "not_found", http.StatusNotFound},
	} {
		resp := do(t, ts, "DELETE", "/api/roots/"+c.slug, "", nil)
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.status)
			continue
		}
		if code, msg := refusalOf(t, resp); code != c.code {
			t.Errorf("%s: code %q (%s), want %q", c.name, code, msg, c.code)
		}
	}
	if slugs := registeredSlugs(t, ts); len(slugs) != 2 {
		t.Errorf("roots = %v, want both still registered", slugs)
	}
	if rec := persisted(t, statePath); len(rec) != 1 || rec[0].Slug != "proj" {
		t.Errorf("state file holds %v, want the one recent root", rec)
	}

	// Removing the same root twice: the second is an unknown slug, not a
	// second removal.
	if resp := do(t, ts, "DELETE", "/api/roots/proj", "", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first delete: status %d", resp.StatusCode)
	}
	resp := do(t, ts, "DELETE", "/api/roots/proj", "", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("second delete: status %d, want 404", resp.StatusCode)
	}
	if code, _ := refusalOf(t, resp); code != "not_found" {
		t.Errorf("second delete: code %q, want not_found", code)
	}
}

// A tab open on a root that has just been removed has to find out. The
// recorded route is the event stream: the daemon ends it when the root
// goes, and the reconnect the browser makes meets the 404 every route
// under a slug now gives, which is what sends the page home.
func TestDeletedRootEndsItsEventStream(t *testing.T) {
	ts, base, _ := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "todo.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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

	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(resp.Body)
		done <- err
	}()
	if r := do(t, ts, "DELETE", "/api/roots/proj", "", nil); r.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", r.StatusCode)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reading the stream after the delete: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream was still open five seconds after its root was removed")
	}
	// The reconnect a browser makes next meets the 404.
	if r := do(t, ts, "GET", "/api/r/proj/events", "", nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("events after the delete: status %d, want 404", r.StatusCode)
	}
}

// Unregistering is refused under the tailnet name exactly as registering
// is, and by the same rule: the allow-list admits `GET /api/roots` and
// nothing else under that path, so `DELETE /api/roots/{slug}` is refused
// by the default rather than by anything added for it. M7-R2's "never
// widens what a tailnet credential can do".
func TestDeleteRootIsLoopbackOnly(t *testing.T) {
	ts, base, statePath := newRootsServer(t, WithTailnetHost(tailnetName))
	tok := daemonToken(t, base)
	cookie := login(t, ts, base)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`"}`, nil)

	for _, c := range []struct {
		name string
		hdr  map[string]string
	}{
		{"with the token", bearerHeader(tok)},
		{"with a browser session", map[string]string{"Cookie": cookie, "Origin": tailnetOrigin}},
	} {
		resp := tdo(t, ts, "DELETE", "/api/roots/proj", "", c.hdr)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", c.name, resp.StatusCode)
			continue
		}
		if code := guardCode(t, resp); code != "loopback_only" {
			t.Errorf("%s: code %q, want loopback_only", c.name, code)
		}
	}
	// Nothing moved, on either side of the name.
	if slugs := registeredSlugs(t, ts); len(slugs) != 2 {
		t.Errorf("roots = %v, want both still registered", slugs)
	}
	if rec := persisted(t, statePath); len(rec) != 1 || rec[0].Slug != "proj" {
		t.Errorf("state file holds %v, want the root untouched", rec)
	}
	// The same call on loopback, which is where it belongs, works.
	if resp := do(t, ts, "DELETE", "/api/roots/proj", "", nil); resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete on loopback: status %d, want 204", resp.StatusCode)
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

// The daemon's own words for the two new refusals, which the extension and
// the home page both branch on rather than reading the sentence.
func TestRootRefusalsCarryACode(t *testing.T) {
	ts, base, _ := newRootsServer(t)
	proj := filepath.Join(base, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	resp := do(t, ts, "POST", "/api/roots", `{"path":"`+proj+`","file":"gone.md"}`, nil)
	code, message := refusalOf(t, resp)
	if code != "not_found" || !strings.Contains(message, "gone.md") {
		t.Errorf("missing file: code %q, message %q — the message should name the file", code, message)
	}
	resp = do(t, ts, "DELETE", "/api/roots/notes", "", nil)
	code, message = refusalOf(t, resp)
	if code != "notes_root" || message == "" {
		t.Errorf("the notes root: code %q, message %q", code, message)
	}
}

// withDirs replaces the directory listing a watcher setup starts with, so
// a test can hold one setup open while the registry changes under it.
func withDirs(f func(context.Context, string, func(string, ...any)) ([]tree.Dir, error)) Option {
	return func(s *Server) { s.listDirs = f }
}

// A slug freed by a removal can come back for a different folder while the
// first folder's watcher is still being set up, and the watcher that
// finishes must not be installed under the slug that now means somewhere
// else. Before M7-R2 no slug could be freed while the daemon ran, so the
// slug was a safe proxy for the root; unregistering is what ends that.
//
// The sequence, with the first setup held inside its directory listing:
// register /a/docs (slug "docs"), remove it, register /b/docs (slug "docs"
// again — its own setup returns at once, because the first is still
// marked as starting), then let the first setup finish. What comes out has
// to be a watcher on /b/docs: with the guard asking only whether *something*
// holds the slug, it is a watcher on /a/docs, and the stream for "docs"
// then reports a folder nobody is looking at.
func TestWatcherSetupTargetsTheRootItStartedFor(t *testing.T) {
	base := t.TempDir()
	first := filepath.Join(base, "a", "docs")
	second := filepath.Join(base, "b", "docs")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	held := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	ts, _, _ := newRootsServer(t, withDirs(func(ctx context.Context, root string, warnf func(string, ...any)) ([]tree.Dir, error) {
		if root == first {
			once.Do(func() { close(held) })
			<-release
		}
		return tree.Dirs(ctx, root, warnf)
	}))

	if resp := do(t, ts, "POST", "/api/roots", `{"path":"`+first+`"}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("register the first folder: status %d", resp.StatusCode)
	}
	<-held
	if resp := do(t, ts, "DELETE", "/api/roots/docs", "", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove it: status %d", resp.StatusCode)
	}
	var added roots.Root
	resp := do(t, ts, "POST", "/api/roots", `{"path":"`+second+`"}`, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register the second folder: status %d", resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(&added)
	if added.Slug != "docs" || added.Path != second {
		t.Fatalf("the second folder did not take the freed slug: %+v", added)
	}
	close(release)

	// The stream under the slug is the folder the slug now names. A change
	// in it arrives; the wait is what a watcher on the other folder fails.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/r/docs/events", nil)
	req.Host = "localhost:7337"
	stream, err := waitForStream(t, req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	next := sseReader(t, stream.Body)
	next(func(l string) bool { return l == ": connected" })
	os.WriteFile(filepath.Join(second, "new.md"), []byte("# new\n"), 0o644)
	// A write in the folder the slug no longer names must not answer for
	// it. Written second, so a stream that reports it rather than the one
	// above is reporting the wrong folder and not merely racing.
	os.WriteFile(filepath.Join(first, "wrong.md"), []byte("# wrong\n"), 0o644)
	// The status event carries coverage, which is 1 directory either way;
	// the change is what names the folder.
	next(func(l string) bool { return l == "event: change" })
	line := next(func(l string) bool { return strings.HasPrefix(l, "data: ") })
	if !strings.Contains(line, "new.md") || strings.Contains(line, "wrong.md") {
		t.Fatalf("the stream under slug \"docs\" reported %q; it is watching the wrong folder", line)
	}
}

// waitForStream opens the event stream once the root has a watcher. The
// setup runs in the background, so a request that arrives before it has
// finished is answered 503 rather than made to wait.
func waitForStream(t *testing.T, req *http.Request) (*http.Response, error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.DefaultClient.Do(req.Clone(req.Context()))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		resp.Body.Close()
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the events endpoint answered %d for ten seconds; the root never got a watcher", resp.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
