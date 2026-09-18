package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckAcceptsAgreement(t *testing.T) {
	if err := check("v0.1.0", "v0.1.0", "0.1.0"); err != nil {
		t.Fatalf("check() = %v, want nil", err)
	}
	// An untagged local build: git describe gives a bare sha, the binary
	// prints it, and the manifest falls back to the development version.
	if err := check("4bcf322", "4bcf322", "0.0.0"); err != nil {
		t.Fatalf("check() on an untagged build = %v, want nil", err)
	}
}

func TestCheckCatchesEachMismatch(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		version, reported, stamped string
		want                       string
	}{
		{
			name:     "the binary was built from another version",
			version:  "v0.1.0",
			reported: "v0.0.9",
			stamped:  "0.1.0",
			want:     "the binary reports",
		},
		{
			name:     "the binary carries no version at all",
			version:  "v0.1.0",
			reported: "dev",
			stamped:  "0.1.0",
			want:     "the binary reports",
		},
		{
			name:     "a stale manifest survived from an earlier build",
			version:  "v0.1.0",
			reported: "v0.1.0",
			stamped:  "0.0.9",
			want:     "the manifest carries",
		},
		{
			name:     "the manifest was never stamped",
			version:  "v0.1.0",
			reported: "v0.1.0",
			stamped:  "0.0.0",
			want:     "the manifest carries",
		},
		{
			name:     "the manifest kept the tag's v",
			version:  "v0.1.0",
			reported: "v0.1.0",
			stamped:  "v0.1.0",
			want:     "the manifest carries",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := check(tc.version, tc.reported, tc.stamped)
			if err == nil {
				t.Fatalf("check(%q, %q, %q) = nil, want an error", tc.version, tc.reported, tc.stamped)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("check() = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestManifestVersion(t *testing.T) {
	// The same table extension/scripts/version.test.mjs holds: the two
	// derivations are separate implementations of one rule, and a release
	// where they disagree is exactly what this command exists to stop.
	for raw, want := range map[string]string{
		"v0.1.0":            "0.1.0",
		"0.1.0":             "0.1.0",
		" v0.1.0\n":         "0.1.0",
		"v1.20.300":         "1.20.300",
		"1.2.3.4":           "1.2.3.4",
		"65535.65535.65535": "65535.65535.65535",
		"v0.1.0-3-gabc1234": "0.0.0",
		"v0.1.0-dirty":      "0.0.0",
		"4bcf322":           "0.0.0",
		"dev":               "0.0.0",
		"":                  "0.0.0",
		"v0.01.0":           "0.0.0",
		"1.2.3.4.5":         "0.0.0",
		"1.65536.0":         "0.0.0",
		"v1.2.x":            "0.0.0",
	} {
		if got := manifestVersion(raw); got != want {
			t.Errorf("manifestVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestManifestVersionOf(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	got, err := manifestVersionOf(write("stamped.json", `{"name":"md-notes","version":"0.1.0"}`))
	if err != nil || got != "0.1.0" {
		t.Fatalf("manifestVersionOf() = %q, %v, want %q, nil", got, err, "0.1.0")
	}

	// The committed manifest carries no version; finding one that way means
	// the build's stamping step never ran.
	if _, err := manifestVersionOf(write("bare.json", `{"name":"md-notes"}`)); err == nil {
		t.Fatal("manifestVersionOf() on an unstamped manifest = nil, want an error")
	}
	if _, err := manifestVersionOf(filepath.Join(dir, "absent.json")); err == nil {
		t.Fatal("manifestVersionOf() on a missing file = nil, want an error")
	}
}

// TestRunAgainstABuild drives the command the way make release does, against a
// stand-in binary, so the wiring — running the binary, reading the manifest,
// the exit status — is covered and not just the comparison.
func TestRunAgainstABuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in binary is a shell script")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "mdn")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho v0.1.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"version":"0.1.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"-version", "v0.1.0", "-binary", binary, "-manifest", manifest}
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "v0.1.0") {
		t.Fatalf("stdout = %q, want the version in it", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-version", "v0.2.0", "-binary", binary, "-manifest", manifest}, &stdout, &stderr); code != 1 {
		t.Fatalf("run() on a mismatch = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "the binary reports") {
		t.Fatalf("stderr = %q, want the mismatch named", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-version", "v0.1.0", "-manifest", manifest}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() without -binary = %d, want 2", code)
	}
}
