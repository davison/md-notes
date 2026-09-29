package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davison/md-notes/internal/source"
	"github.com/davison/md-notes/internal/tree"
)

func testPNG(t *testing.T, seed uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	img.Set(1, 1, color.RGBA{seed, seed, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

type uploadedBody struct {
	Root    string `json:"root"`
	Path    string `json:"path"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

func decodeUpload(t *testing.T, resp *http.Response) uploadedBody {
	t.Helper()
	var body uploadedBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func resourcesListing(t *testing.T, base string) []string {
	t.Helper()
	dir := filepath.Join(base, "notes", "_resources")
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil
	}
	return listing(t, dir)
}

func TestUploadImage(t *testing.T) {
	ts, base := newTestServer(t)
	img := testPNG(t, 1)
	resp := do(t, ts, "POST", "/api/r/notes/resources?name=Holiday%20snap.png", img,
		map[string]string{"Content-Type": "application/octet-stream", "Origin": "http://localhost:7337"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("missing no-store")
	}
	body := decodeUpload(t, resp)
	if body != (uploadedBody{Root: "notes", Path: "_resources/Holiday-snap.png", Name: "Holiday-snap.png", Created: true}) {
		t.Fatalf("body = %+v", body)
	}
	got, err := os.ReadFile(filepath.Join(base, "notes", "_resources", "Holiday-snap.png"))
	if err != nil || string(got) != img {
		t.Fatalf("disk: %v", err)
	}
	// The same bytes again are the same file, answered 200.
	resp = do(t, ts, "POST", "/api/r/notes/resources?name=Holiday%20snap.png", img, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("again: status %d", resp.StatusCode)
	}
	if again := decodeUpload(t, resp); again.Path != body.Path || again.Created {
		t.Fatalf("again = %+v", again)
	}
	// A paste carries no name and is named by its content.
	resp = do(t, ts, "POST", "/api/r/notes/resources", testPNG(t, 2), nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("paste: status %d", resp.StatusCode)
	}
	if p := decodeUpload(t, resp).Path; !strings.HasPrefix(p, "_resources/") || len(p) != len("_resources/")+32+len(".png") {
		t.Fatalf("paste path = %q", p)
	}
}

func TestUploadRefusals(t *testing.T) {
	ts, base := newTestServer(t)
	for _, c := range []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"html under an image name", "/api/r/notes/resources?name=x.png", "<!doctype html><script>alert(1)</script>", 415, "unsupported_type"},
		{"text claiming to be svg", "/api/r/notes/resources?name=x.svg", "hello", 415, "unsupported_type"},
		{"empty body", "/api/r/notes/resources?name=x.png", "", 400, "invalid_body"},
		{"unknown root", "/api/r/nope/resources", testPNG(t, 3), 404, "not_found"},
	} {
		resp := do(t, ts, "POST", c.path, c.body, map[string]string{"Content-Type": "image/png"})
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.status)
			continue
		}
		if code := guardCode(t, resp); code != c.code {
			t.Errorf("%s: code %q, want %q", c.name, code, c.code)
		}
	}
	if got := resourcesListing(t, base); len(got) > 1 {
		t.Fatalf("refused uploads wrote %v", got)
	}
}

// Over the cap is refused without the whole body being taken in, and
// leaves nothing behind.
func TestUploadCapIsEnforcedOnTheBody(t *testing.T) {
	ts, base := newTestServer(t)
	big := testPNG(t, 4) + strings.Repeat("\x00", source.MaxUploadBytes)
	resp := do(t, ts, "POST", "/api/r/notes/resources?name=big.png", big, nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", resp.StatusCode)
	}
	if code := guardCode(t, resp); code != "too_large" {
		t.Fatalf("code %q", code)
	}
	if got := resourcesListing(t, base); len(got) > 1 {
		t.Fatalf("an over-cap upload wrote %v", got)
	}
}

func TestUploadRefusesAResourcesLinkOutOfTheRoot(t *testing.T) {
	ts, base := newTestServer(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "notes", "_resources")); err != nil {
		t.Fatal(err)
	}
	resp := do(t, ts, "POST", "/api/r/notes/resources?name=x.png", testPNG(t, 5), nil)
	if resp.StatusCode != http.StatusForbidden || guardCode(t, resp) != "outside_root" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("wrote outside the root")
	}
}

// The upload is behind the guard every other write is behind.
func TestUploadIsGuarded(t *testing.T) {
	ts, base := newTestServer(t)
	img := testPNG(t, 6)
	for _, c := range []struct {
		name   string
		hdr    map[string]string
		status int
		code   string
	}{
		{"a foreign origin", map[string]string{"Origin": "http://evil.example"}, 403, "cross_origin"},
		{"a null origin", map[string]string{"Origin": "null"}, 403, "cross_origin"},
		{"a wrong token", bearerHeader("wrong"), 401, "unauthorized"},
		{"a foreign origin with a wrong token", bearerHeader("wrong", "Origin", "http://evil.example"), 401, "unauthorized"},
		{"a rebinding host", map[string]string{"Host": "evil.example:7337"}, 403, "bad_host"},
	} {
		resp := do(t, ts, "POST", "/api/r/notes/resources?name=g.png", img, c.hdr)
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.name, resp.StatusCode, c.status)
			continue
		}
		if code := guardCode(t, resp); code != c.code {
			t.Errorf("%s: code %q, want %q", c.name, code, c.code)
		}
	}
	if got := resourcesListing(t, base); got != nil {
		t.Fatalf("a refused request wrote %v", got)
	}
	// The token lets the extension's origin through, as for every write.
	resp := do(t, ts, "POST", "/api/r/notes/resources?name=g.png", img,
		bearerHeader(daemonToken(t, base), "Origin", "chrome-extension://abc"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("with the token: status %d", resp.StatusCode)
	}
}

func TestTailnetAdmitsAnUpload(t *testing.T) {
	ts, base := newTailnetServer(t)
	cookie := login(t, ts, base)
	resp := tdo(t, ts, "POST", "/api/r/notes/resources?name=phone.png", testPNG(t, 7),
		map[string]string{"Cookie": cookie, "Origin": tailnetOrigin})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("session upload: status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	resp = tdo(t, ts, "POST", "/api/r/notes/resources?name=evil.png", testPNG(t, 8),
		map[string]string{"Cookie": cookie, "Origin": "https://evil.example"})
	if resp.StatusCode != http.StatusForbidden || guardCode(t, resp) != "cross_origin" {
		t.Fatalf("cross-origin session upload: status %d", resp.StatusCode)
	}
	resp = tdo(t, ts, "POST", "/api/r/notes/resources?name=anon.png", testPNG(t, 9), nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no credential: status %d", resp.StatusCode)
	}
	// One method only: nothing else hangs off the path remotely.
	for _, m := range []string{"PUT", "DELETE"} {
		resp = tdo(t, ts, m, "/api/r/notes/resources", "", bearerHeader(daemonToken(t, base)))
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", m, resp.StatusCode)
		}
	}
	if got := resourcesListing(t, base); len(got) != 2 || got[1] != "phone.png" {
		t.Fatalf("_resources = %v, want phone.png alone", got)
	}
}

// An uploaded SVG can carry script. It is served back on the UI's origin
// under a sandbox policy, so opened directly it runs nothing with that
// origin's authority, and nosniff stops a browser reading it as anything
// but what it is.
func TestUploadedSVGIsServedSandboxed(t *testing.T) {
	ts, _ := newTestServer(t)
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><script>fetch("/api/roots",{method:"POST"})</script></svg>`
	resp := do(t, ts, "POST", "/api/r/notes/resources?name=evil.svg", svg, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", resp.StatusCode, readAll(t, resp.Body))
	}
	body := decodeUpload(t, resp)
	resp = do(t, ts, "GET", "/api/r/notes/raw/"+body.Path, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("raw: status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != "sandbox" {
		t.Errorf("CSP = %q, want sandbox", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", got)
	}
}

// Images in _resources are not notes: the navigator does not list the
// folder, before an upload or after one.
func TestResourcesStayOutOfTheTree(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed; CI installs it")
	}
	ts, _ := newTestServer(t)
	if resp := do(t, ts, "POST", "/api/r/notes/resources", testPNG(t, 10), nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: status %d", resp.StatusCode)
	}
	resp := do(t, ts, "GET", "/api/r/notes/tree", "", nil)
	var root tree.Node
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	for _, c := range root.Children {
		if c.Path == "_resources" {
			t.Fatalf("the tree lists _resources: %+v", root)
		}
	}
}
