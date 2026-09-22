package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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

// Every start names what it serves, and says when --root set the config
// file's roots aside: the replace rule is the right one, and silent it
// would be the trap #162 was (review of #198, finding 1).
func TestServeLogsTheRootsItServes(t *testing.T) {
	base, scratch := serveFixture(t, "notes", "projects", "other")
	notes, projects, other := filepath.Join(base, "notes"), filepath.Join(base, "projects"), filepath.Join(base, "other")
	configPath := filepath.Join(base, "config.yml")
	os.WriteFile(configPath, []byte("notes_root: ["+notes+", "+projects+"]\n"), 0o644)

	_, stderr := listRoots(t, scratch)
	for _, want := range []string{"serving notes (notes): " + notes, "serving projects (permanent): " + projects} {
		if !strings.Contains(stderr, want) {
			t.Errorf("start from the file logged %q, want %q", stderr, want)
		}
	}
	if strings.Contains(stderr, "replaced") {
		t.Errorf("start from the file logged %q, want no replacement", stderr)
	}

	_, stderr = listRoots(t, append(scratch, "--root", other))
	for _, want := range []string{
		"--root replaced notes_root from " + configPath + " (" + notes + ", " + projects + ")",
		"serving other (notes): " + other,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("start with --root logged %q, want %q", stderr, want)
		}
	}
	if strings.Contains(stderr, "serving notes") {
		t.Errorf("start with --root logged %q, which still serves the file's roots", stderr)
	}
}

// stateFixture writes a state file naming a folder that is gone and one
// that is about to be configured, and returns it with its bytes.
func stateFixture(t *testing.T, base string) (string, []byte) {
	t.Helper()
	statePath := filepath.Join(base, "state", "roots.json")
	os.MkdirAll(filepath.Dir(statePath), 0o700)
	data := []byte(`{"recent":[{"slug":"gone","path":"` + filepath.Join(base, "gone") +
		`"},{"slug":"proj","path":"` + filepath.Join(base, "projects") + `"}]}`)
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return statePath, data
}

// A start that cannot bind its port changes nothing in the state file, not
// even the tidying a successful start does (review of #198, finding 2).
func TestServeFailedBindLeavesTheStateFile(t *testing.T) {
	base, scratch := serveFixture(t, "notes", "projects")
	statePath, before := stateFixture(t, base)
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := strconv.Itoa(taken.Addr().(*net.TCPAddr).Port)

	var out, stderr bytes.Buffer
	code := runServe(append(scratch, "--port", port,
		"--root", filepath.Join(base, "notes"), "--root", filepath.Join(base, "projects")), &out, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1 for a port in use; stderr %q", code, stderr.String())
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Errorf("state file changed by a failed start:\nbefore %s\nafter  %s", before, after)
	}
}

// A start that does bind drops the gone folder from the state file and
// keeps the entry for the folder it now serves as configured.
func TestServeSettlesTheStateFileOnceListening(t *testing.T) {
	base, scratch := serveFixture(t, "notes", "projects")
	statePath, before := stateFixture(t, base)
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(free.Addr().(*net.TCPAddr).Port)
	free.Close()

	var stderr bytes.Buffer
	srv, code := prepareServe(append(scratch, "--port", port,
		"--root", filepath.Join(base, "notes"), "--root", filepath.Join(base, "projects")), &stderr)
	if srv == nil {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if after, _ := os.ReadFile(statePath); !bytes.Equal(after, before) {
		t.Fatalf("state file changed before the port was bound: %s", after)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe(ctx) }()
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); <-done }) }
	defer stop()

	deadline := time.Now().Add(5 * time.Second)
	for {
		after, _ := os.ReadFile(statePath)
		if !bytes.Equal(after, before) {
			if strings.Contains(string(after), filepath.Join(base, "gone")) ||
				!strings.Contains(string(after), filepath.Join(base, "projects")) {
				t.Fatalf("settled state file is %s", after)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the state file was never settled")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop() // the log is the daemon's until it has stopped writing to it
	if !strings.Contains(stderr.String(), filepath.Join(base, "projects")+" is configured as a root") {
		t.Errorf("no warning named the shadowed recent root: %q", stderr.String())
	}
}
