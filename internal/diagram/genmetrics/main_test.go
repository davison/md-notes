package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const fontPath = "/usr/share/fonts/noto/NotoSans-Regular.ttf"

func TestParseRejectsNonFonts(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("not a font at all"), make([]byte, 64)} {
		if _, err := parse(b); err == nil {
			t.Errorf("parse(%q) succeeded", b)
		}
	}
}

// The committed table is what the generator makes from the font, so a
// hand edit to metrics.go, or a generator change not re-run, fails here.
// Skipped where the font is not installed (the Arch path go:generate
// names); the table's values are also spot-checked against FreeType in
// the diagram package's own tests, which need no font.
func TestCommittedTableIsGenerated(t *testing.T) {
	if _, err := os.Stat(fontPath); err != nil {
		t.Skipf("%s not installed", fontPath)
	}
	out := filepath.Join(t.TempDir(), "metrics.go")
	cmd := exec.Command("go", "run", ".", fontPath, out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("genmetrics: %v\n%s", err, b)
	}
	got, _ := os.ReadFile(out)
	want, err := os.ReadFile("../metrics.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("internal/diagram/metrics.go differs from what genmetrics generates; run go generate ./internal/diagram/...")
	}
}
