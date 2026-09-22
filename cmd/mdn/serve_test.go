package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serveFixture is a scratch home for `mdn serve`: the folders it may serve,
// and flags pointing its config, state and token somewhere disposable.
func serveFixture(t *testing.T, dirs ...string) (base string, scratch []string) {
	t.Helper()
	base = t.TempDir()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return base, []string{
		"--config", filepath.Join(base, "config.yml"),
		"--state", filepath.Join(base, "state", "roots.json"),
		"--token-file", filepath.Join(base, "state", "token"),
	}
}

type listedRoot struct{ Slug, Path, Kind string }

// listRoots asks a prepared daemon what it serves, the way the home page does.
func listRoots(t *testing.T, args []string) ([]listedRoot, string) {
	t.Helper()
	var stderr bytes.Buffer
	srv, code := prepareServe(args, &stderr)
	if srv == nil {
		t.Fatalf("mdn serve refused (exit %d): %s", code, stderr.String())
	}
	t.Cleanup(srv.Close)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	req, _ := http.NewRequest("GET", ts.URL+"/api/roots", nil)
	req.Host = "localhost:7337"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct{ Roots []listedRoot }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Roots, stderr.String()
}

// The operator's own invocation (#162): `mdn serve --root ~/notes --root
// ~/projects` served projects alone, as the notes root, because --root
// kept its last value. Both are served now, the first as the notes root.
// A config file naming only the first changes nothing: flags replace it.
func TestServeRepeatedRootServesEveryOne(t *testing.T) {
	base, scratch := serveFixture(t, "notes", "projects")
	notes, projects := filepath.Join(base, "notes"), filepath.Join(base, "projects")
	os.WriteFile(filepath.Join(base, "config.yml"), []byte("notes_root: "+notes+"\n"), 0o644)

	got, _ := listRoots(t, append(scratch, "--root", notes, "--root", projects))
	want := []listedRoot{{"notes", notes, "notes"}, {"projects", projects, "permanent"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("GET /api/roots = %+v, want %+v", got, want)
	}
}

// The list form of notes_root does the same from the file alone.
func TestServeConfigListServesEveryRoot(t *testing.T) {
	base, scratch := serveFixture(t, "notes", "projects")
	notes, projects := filepath.Join(base, "notes"), filepath.Join(base, "projects")
	os.WriteFile(filepath.Join(base, "config.yml"),
		[]byte("notes_root:\n  - "+notes+"\n  - "+projects+"\n"), 0o644)

	got, _ := listRoots(t, scratch)
	if len(got) != 2 || got[0].Kind != "notes" || got[1].Kind != "permanent" || got[1].Path != projects {
		t.Fatalf("GET /api/roots = %+v", got)
	}
}

// A repeated --root naming a folder already named is refused at the start,
// never served once in silence.
func TestServeRefusesADuplicateRoot(t *testing.T) {
	base, scratch := serveFixture(t, "notes")
	notes := filepath.Join(base, "notes")
	var stderr bytes.Buffer
	srv, code := prepareServe(append(scratch, "--root", notes, "--root", notes+"/"), &stderr)
	if srv != nil {
		srv.Close()
		t.Fatal("a duplicate --root was accepted")
	}
	if code != 1 || !strings.Contains(stderr.String(), "same folder") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

// The first-start hint names the token file in use when --token-file was
// given, since a bare `mdn token` reads a different one (#155).
func TestServeFirstStartTokenHint(t *testing.T) {
	base, scratch := serveFixture(t, "notes")
	_, stderr := listRoots(t, append(scratch, "--root", filepath.Join(base, "notes")))
	want := "print it with `mdn token --token-file " + filepath.Join(base, "state", "token") + "`"
	if !strings.Contains(stderr, want) {
		t.Errorf("first start logged %q, want it to contain %q", stderr, want)
	}
	// A second start creates nothing and says nothing about it.
	_, stderr = listRoots(t, append(scratch, "--root", filepath.Join(base, "notes")))
	if strings.Contains(stderr, "generated a bearer token") {
		t.Errorf("second start logged %q", stderr)
	}
}

func TestTokenCreatedLine(t *testing.T) {
	for _, c := range []struct {
		path  string
		given bool
		want  string
	}{
		{"/home/you/.local/state/mdn/token", false, "print it with `mdn token`"},
		{"/tmp/mdn-dev/token", true, "print it with `mdn token --token-file /tmp/mdn-dev/token`"},
		{"/tmp/my dev/token", true, "print it with `mdn token --token-file '/tmp/my dev/token'`"},
		{"/tmp/it's/token", true, `print it with ` + "`" + `mdn token --token-file '/tmp/it'\''s/token'` + "`"},
	} {
		got := tokenCreatedLine(c.path, c.given)
		if !strings.HasSuffix(got, c.want) || !strings.Contains(got, "in "+c.path+";") {
			t.Errorf("tokenCreatedLine(%q, %v) = %q, want it to end %q", c.path, c.given, got, c.want)
		}
	}
}
