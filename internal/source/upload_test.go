package source

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/davison/md-notes/internal/roots"
)

// pngBytes is a real, decodable PNG; seed varies its pixels so two calls
// with different seeds are different files.
func pngBytes(t *testing.T, seed uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{seed, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// webpBytes is the RIFF/WEBP header content sniffing looks for, which is
// all the type check reads.
func webpBytes() []byte {
	return []byte("RIFF\x1a\x00\x00\x00WEBPVP8 \x0e\x00\x00\x00\x30\x01\x00\x9d\x01\x2a\x01\x00\x01\x00\x02\x00\x34\x25")
}

const svgDoc = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" width="4" height="4"><rect width="4" height="4"/></svg>`

func readDisk(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestUploadCreatesResourcesAndHashNamesAPaste(t *testing.T) {
	s, dir := fixture(t)
	data := pngBytes(t, 1)
	up, err := s.Upload("notes", "", data)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^_resources/[0-9a-f]{32}\.png$`).MatchString(up.Path) {
		t.Fatalf("path = %q, want _resources/<32 hex>.png", up.Path)
	}
	if !up.Created || up.Name != strings.TrimPrefix(up.Path, "_resources/") {
		t.Fatalf("upload = %+v", up)
	}
	if got := readDisk(t, filepath.Join(dir, filepath.FromSlash(up.Path))); !bytes.Equal(got, data) {
		t.Fatal("bytes on disk differ from the upload")
	}
	// The same image pasted again is the same file: nothing new is written.
	again, err := s.Upload("notes", "", data)
	if err != nil {
		t.Fatal(err)
	}
	if again.Path != up.Path || again.Created {
		t.Fatalf("second paste = %+v, want %s reused", again, up.Path)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "_resources"))
	if len(entries) != 1 {
		t.Fatalf("_resources holds %d files, want 1", len(entries))
	}
}

func TestUploadKeepsADroppedName(t *testing.T) {
	s, _ := fixture(t)
	cases := []struct{ hint, want string }{
		{"holiday.png", "_resources/holiday.png"},
		{"Photo 12 (copy).PNG", "_resources/Photo-12-copy.png"},
		// The extension is the daemon's, from the content.
		{"shot.jpeg.html", "_resources/shot.jpeg.png"},
		{".hidden.png", "_resources/hidden.png"},
		// A name, never a path.
		{"../../escape.png", "_resources/escape.png"},
		{`..\..\win.png`, "_resources/win.png"},
	}
	for i, c := range cases {
		up, err := s.Upload("notes", c.hint, pngBytes(t, uint8(10+i)))
		if err != nil {
			t.Fatalf("%q: %v", c.hint, err)
		}
		if up.Path != c.want {
			t.Errorf("%q landed at %q, want %q", c.hint, up.Path, c.want)
		}
	}
}

func TestUploadFallsBackToAHashForAnEmptyName(t *testing.T) {
	s, _ := fixture(t)
	up, err := s.Upload("notes", "...", pngBytes(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^_resources/[0-9a-f]{32}\.png$`).MatchString(up.Path) {
		t.Fatalf("path = %q", up.Path)
	}
}

// A name that is taken by a different file is never replaced: the upload
// takes the next suffix and the file already there keeps its bytes.
func TestUploadNeverOverwrites(t *testing.T) {
	s, dir := fixture(t)
	res := filepath.Join(dir, "_resources")
	if err := os.Mkdir(res, 0o755); err != nil {
		t.Fatal(err)
	}
	old := pngBytes(t, 1)
	if err := os.WriteFile(filepath.Join(res, "shot.png"), old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(res, "shot-2.png"), []byte("not even an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := pngBytes(t, 2)
	up, err := s.Upload("notes", "shot.png", data)
	if err != nil {
		t.Fatal(err)
	}
	if up.Path != "_resources/shot-3.png" || !up.Created {
		t.Fatalf("upload = %+v, want a new _resources/shot-3.png", up)
	}
	if got := readDisk(t, filepath.Join(res, "shot.png")); !bytes.Equal(got, old) {
		t.Fatal("shot.png was replaced")
	}
	if got := readDisk(t, filepath.Join(res, "shot-3.png")); !bytes.Equal(got, data) {
		t.Fatal("shot-3.png does not hold the upload")
	}
	// The same bytes dropped again under the same name reuse shot-3.
	again, err := s.Upload("notes", "shot.png", data)
	if err != nil || again.Path != "_resources/shot-3.png" || again.Created {
		t.Fatalf("again = %+v, %v", again, err)
	}
}

// A link at the name an upload would take is never written through, even
// one pointing at a file with the very same bytes, and never reused.
func TestUploadDoesNotWriteThroughALink(t *testing.T) {
	s, dir := fixture(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "victim.png")
	data := pngBytes(t, 4)
	if err := os.WriteFile(target, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	same := filepath.Join(outside, "same.png")
	if err := os.WriteFile(same, data, 0o644); err != nil {
		t.Fatal(err)
	}
	res := filepath.Join(dir, "_resources")
	if err := os.Mkdir(res, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(res, "shot.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(same, filepath.Join(res, "shot-2.png")); err != nil {
		t.Fatal(err)
	}
	up, err := s.Upload("notes", "shot.png", data)
	if err != nil {
		t.Fatal(err)
	}
	if up.Path != "_resources/shot-3.png" || !up.Created {
		t.Fatalf("upload = %+v, want a new _resources/shot-3.png", up)
	}
	if got := readDisk(t, target); string(got) != "precious" {
		t.Fatalf("written through the link: %q", got)
	}
}

func TestUploadRefusesAResourcesLinkThatLeavesTheRoot(t *testing.T) {
	s, dir := fixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "_resources")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Upload("notes", "x.png", pngBytes(t, 5))
	if !errors.Is(err, roots.ErrOutside) {
		t.Fatalf("err = %v, want ErrOutside", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote %d files outside the root", len(entries))
	}
}

func TestUploadRefusesADanglingResourcesLinkThatLeavesTheRoot(t *testing.T) {
	s, dir := fixture(t)
	outside := filepath.Join(t.TempDir(), "not-yet")
	if err := os.Symlink(outside, filepath.Join(dir, "_resources")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Upload("notes", "x.png", pngBytes(t, 5))
	if !errors.Is(err, roots.ErrOutside) {
		t.Fatalf("err = %v, want ErrOutside", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("created %s outside the root", outside)
	}
}

func TestUploadRefusesAResourcesThatIsAFile(t *testing.T) {
	s, dir := fixture(t)
	put(t, filepath.Join(dir, "_resources"), "a file", 0o644)
	_, err := s.Upload("notes", "x.png", pngBytes(t, 5))
	if !errors.Is(err, roots.ErrNotDir) {
		t.Fatalf("err = %v, want ErrNotDir", err)
	}
}

func TestUploadChecksTheContentNotTheName(t *testing.T) {
	s, dir := fixture(t)
	refused := map[string][]byte{
		"page.png":    []byte("<!doctype html><script>alert(1)</script>"),
		"text.jpg":    []byte("just some text"),
		"empty.png":   {},
		"fake.svg":    []byte("<html><svg xmlns=\"http://www.w3.org/2000/svg\"/></html>"),
		"nons.svg":    []byte(`<svg><rect/></svg>`),
		"broken.svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect`),
		"binary.svg":  append([]byte(`<svg xmlns="http://www.w3.org/2000/svg">`), 0xff, 0xfe),
		"archive.png": {0x50, 0x4b, 0x03, 0x04, 0, 0, 0, 0},
		"pdf.png":     []byte("%PDF-1.7\n"),
	}
	for name, data := range refused {
		if _, err := s.Upload("notes", name, data); !errors.Is(err, ErrImageType) {
			t.Errorf("%s: err = %v, want ErrImageType", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "_resources")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(dir, "_resources"))
		if len(entries) != 0 {
			t.Fatalf("refused uploads left %d files", len(entries))
		}
	}
	accepted := map[string]struct {
		data []byte
		ext  string
	}{
		"a": {pngBytes(t, 6), ".png"},
		"b": {jpegBytes(t), ".jpg"},
		"c": {gifBytes(t), ".gif"},
		"d": {webpBytes(), ".webp"},
		"e": {[]byte(svgDoc), ".svg"},
		// Script in an SVG is accepted: the raw route's sandbox is what
		// keeps it from running (see the server's test).
		"f": {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), ".svg"},
	}
	for name, c := range accepted {
		up, err := s.Upload("notes", name+".bin", c.data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if up.Path != "_resources/"+name+c.ext {
			t.Errorf("%s landed at %s, want extension %s", name, up.Path, c.ext)
		}
	}
}

func TestUploadCap(t *testing.T) {
	s, dir := fixture(t)
	big := append(pngBytes(t, 7), make([]byte, MaxUploadBytes)...)
	if _, err := s.Upload("notes", "big.png", big); !errors.Is(err, ErrUploadTooLarge) {
		t.Fatalf("err = %v, want ErrUploadTooLarge", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_resources", "big.png")); !os.IsNotExist(err) {
		t.Fatal("an over-cap upload left a file")
	}
	fits := append(pngBytes(t, 7), make([]byte, MaxUploadBytes-200)...)
	if _, err := s.Upload("notes", "fits.png", fits); err != nil {
		t.Fatalf("an upload under the cap: %v", err)
	}
}

func TestUploadUnknownRoot(t *testing.T) {
	s, _ := fixture(t)
	if _, err := s.Upload("nope", "", pngBytes(t, 1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
