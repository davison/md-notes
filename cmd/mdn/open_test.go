package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/server"
)

// startDaemon runs the real server handler on a random loopback port and
// returns that port, so `mdn open` is exercised against the guard too.
func startDaemon(t *testing.T) int {
	t.Helper()
	notes := t.TempDir()
	reg, err := roots.New(notes, filepath.Join(t.TempDir(), "roots.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := server.New(reg, port, fstest.MapFS{"index.html": {Data: []byte("x")}}, log.New(io.Discard, "", 0))
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Listener = ln
	ts.Start()
	t.Cleanup(ts.Close)
	return port
}

func TestOpenRegistersAndOpensBrowser(t *testing.T) {
	port := startDaemon(t)
	dir := filepath.Join(t.TempDir(), "Project Docs")
	os.Mkdir(dir, 0o755)

	var opened string
	orig := openBrowser
	openBrowser = func(url string) error { opened = url; return nil }
	t.Cleanup(func() { openBrowser = orig })

	var out, errb bytes.Buffer
	code := run([]string{"open", "--config", "/nonexistent", "--port", strconv.Itoa(port), dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	want := "http://localhost:" + strconv.Itoa(port) + "/r/project-docs/"
	if opened != want {
		t.Fatalf("opened %q, want %q", opened, want)
	}
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("stdout %q, want %q", out.String(), want)
	}

	resp, err := http.Get("http://localhost:" + strconv.Itoa(port) + "/api/roots")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct{ Roots []roots.Root }
	json.NewDecoder(resp.Body).Decode(&list)
	if len(list.Roots) != 2 || list.Roots[1].Path != dir {
		t.Fatalf("daemon roots = %+v", list.Roots)
	}
}

func TestOpenNoBrowserPrintsURL(t *testing.T) {
	port := startDaemon(t)
	orig := openBrowser
	openBrowser = func(string) error { t.Fatal("browser must not open"); return nil }
	t.Cleanup(func() { openBrowser = orig })
	var out, errb bytes.Buffer
	code := run([]string{"open", "--config", "/nonexistent", "--port", strconv.Itoa(port), "--no-browser", t.TempDir()}, &out, &errb)
	if code != 0 || !strings.HasPrefix(out.String(), "http://localhost:") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}
}

func TestOpenDaemonDown(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	orig := openBrowser
	openBrowser = func(string) error { t.Fatal("browser must not open"); return nil }
	t.Cleanup(func() { openBrowser = orig })
	var out, errb bytes.Buffer
	code := run([]string{"open", "--config", "/nonexistent", "--port", strconv.Itoa(port), t.TempDir()}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "not reachable") || !strings.Contains(errb.String(), "mdn serve") {
		t.Fatalf("stderr %q", errb.String())
	}
}

func TestOpenRefusedPath(t *testing.T) {
	port := startDaemon(t)
	f := filepath.Join(t.TempDir(), "file.md")
	os.WriteFile(f, nil, 0o644)
	var out, errb bytes.Buffer
	code := run([]string{"open", "--config", "/nonexistent", "--port", strconv.Itoa(port), "--no-browser", f}, &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "not a directory") {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestOpenUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"open"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
