package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mdnToken(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{"token"}, args...), &out, &errb)
	return strings.TrimSpace(out.String()), errb.String(), code
}

func TestTokenPrintsAndCreates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "token")
	value, stderr, code := mdnToken(t, "--token-file", path)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if value == "" {
		t.Fatal("no token printed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode %v, want 0600", got)
	}
	again, _, code := mdnToken(t, "--token-file", path)
	if code != 0 || again != value {
		t.Errorf("second print = %q (exit %d), want the same token %q", again, code, value)
	}
}

func TestTokenRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "token")
	first, _, _ := mdnToken(t, "--token-file", path)
	second, stderr, code := mdnToken(t, "--token-file", path, "--rotate")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if second == first || second == "" {
		t.Fatalf("rotate printed %q, want a new token (was %q)", second, first)
	}
	stored, _, _ := mdnToken(t, "--token-file", path)
	if stored != second {
		t.Errorf("stored token %q, want the rotated %q", stored, second)
	}
}

func TestTokenRejectsArgumentsAndUnknownFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if _, stderr, code := mdnToken(t, "--token-file", path, "extra"); code != 2 {
		t.Errorf("exit %d, want 2 (stderr %q)", code, stderr)
	}
	if _, _, code := mdnToken(t, "--nonsense"); code != 2 {
		t.Errorf("unknown flag: exit %d, want 2", code)
	}
}

func TestTokenReportsAnUnwritableFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory mode")
	}
	_, stderr, code := mdnToken(t, "--token-file", filepath.Join(dir, "token"))
	if code != 1 || !strings.Contains(stderr, "mdn token:") {
		t.Errorf("exit %d, stderr %q, want 1 and a reported error", code, stderr)
	}
}

func TestUsageNamesTheTokenCommand(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"help"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "token") {
		t.Errorf("usage = %q, want the token command", out.String())
	}
}
