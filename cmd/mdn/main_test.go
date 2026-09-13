package main

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/davison/md-notes/internal/config"
)

func TestRunNoArgsPrintsUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.HasPrefix(errb.String(), "usage:") {
		t.Fatalf("stderr = %q, want usage", errb.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"bogus"}, &out, &errb); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown command "bogus"`) {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Fatalf("stdout = %q, want %q", out.String(), version)
	}
}

func TestServeOverrides(t *testing.T) {
	build := func(args []string) config.Overrides {
		t.Helper()
		fs := flag.NewFlagSet("mdn serve", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		root := fs.String("root", "", "")
		port := fs.Int("port", 0, "")
		maxWatches := fs.Int("max-watches", config.DefaultMaxWatches, "")
		tailnetHost := fs.String("tailnet-host", "", "")
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		return overrides(fs, *root, *port, *tailnetHost, maxWatches)
	}
	if over := build(nil); over.MaxWatches != nil {
		t.Fatalf("max-watches = %d without the flag, want nothing to override the file", *over.MaxWatches)
	}
	if over := build([]string{"--max-watches", "0"}); over.MaxWatches == nil || *over.MaxWatches != 0 {
		t.Fatalf("--max-watches 0 = %v, want a request for no budget", over.MaxWatches)
	}
	if over := build([]string{"--max-watches", "500", "--root", "/n", "--port", "9"}); over.MaxWatches == nil ||
		*over.MaxWatches != 500 || over.NotesRoot != "/n" || over.Port != 9 {
		t.Fatalf("overrides = %+v", over)
	}
	if over := build(nil); over.TailnetHost != "" {
		t.Fatalf("tailnet-host = %q without the flag, want nothing to override the file", over.TailnetHost)
	}
	if over := build([]string{"--tailnet-host", "laptop.ts.net"}); over.TailnetHost != "laptop.ts.net" {
		t.Fatalf("tailnet-host = %q", over.TailnetHost)
	}
}
