package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// packaging is the committed package directory, from this package's own
// directory.
const packaging = "../../packaging/aur"

// maintainerFile is read by every rendering here, so the operator's answer to
// the maintainer gate reaches the tests the same way it reaches the workflow:
// by editing one file.
var maintainerFile = filepath.Join(packaging, "MAINTAINER")

// TestTheCommittedTemplateIsWhatTheRendererProduces keeps the two from
// drifting.
//
// The PKGBUILD and .SRCINFO in packaging/aur are committed so the package is
// reviewable in a diff, but nothing reads them at release time: the workflow
// renders its own from the tag and the release's checksums. A hand edit to the
// committed pair would therefore be invisible — it would look like a change to
// the package and change nothing at all. This is the test that says so.
func TestTheCommittedTemplateIsWhatTheRendererProduces(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"-placeholder", "-maintainer", maintainerFile, "-out", dir}, os.Stderr); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PKGBUILD", ".SRCINFO"} {
		want, err := os.ReadFile(filepath.Join(packaging, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("packaging/aur/%s is not what `go run ./scripts/aurgen -placeholder` writes.\n"+
				"Re-render it rather than editing it by hand.\n--- committed\n%s\n--- rendered\n%s", name, want, got)
		}
	}
}

// TestTheRenderIsByteStable: the workflow diffs what it renders against what
// the AUR already holds and pushes nothing when they agree, so a rendering
// that varied between runs would push a commit per release saying nothing.
func TestTheRenderIsByteStable(t *testing.T) {
	in := inputs{
		Maintainer: "A Maintainer <a at b dot c>",
		Version:    "v1.2.3",
		PkgRel:     1,
		SumLicense: strings.Repeat("a", 64),
		SumUnit:    strings.Repeat("b", 64),
		SumAMD64:   strings.Repeat("c", 64),
		SumARM64:   strings.Repeat("d", 64),
	}
	firstPKGBUILD, firstSRCINFO, err := render(in)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		pkgbuild, srcinfo, err := render(in)
		if err != nil {
			t.Fatal(err)
		}
		if pkgbuild != firstPKGBUILD || srcinfo != firstSRCINFO {
			t.Fatal("rendering the same inputs twice gave different bytes")
		}
	}
}

// TestTheVersionAndTheChecksumsAreSubstituted is what the committed SKIPs
// stand in for. There is no release yet, so the only proof that a real one
// renders a complete package is this one: every SKIP gone, each binary's
// checksum in its own architecture's array rather than swapped, the tag's v in
// the URLs and gone from pkgver.
func TestTheVersionAndTheChecksumsAreSubstituted(t *testing.T) {
	const (
		amd64Sum = "1111111111111111111111111111111111111111111111111111111111111111"
		arm64Sum = "2222222222222222222222222222222222222222222222222222222222222222"
		zipSum   = "3333333333333333333333333333333333333333333333333333333333333333"
	)
	dir := t.TempDir()
	// The shape `sha256sum mdn-* > SHA256SUMS` leaves in dist/, extension zip
	// and all: the renderer takes the two binaries and ignores the rest.
	sums := filepath.Join(dir, "SHA256SUMS")
	write(t, sums, amd64Sum+"  mdn-v1.2.3-linux-amd64\n"+
		arm64Sum+"  mdn-v1.2.3-linux-arm64\n"+
		zipSum+"  mdn-extension-v1.2.3.zip\n")
	license := filepath.Join(dir, "LICENSE")
	write(t, license, "MIT, as it happens\n")
	unit := filepath.Join(dir, "mdn.service")
	write(t, unit, "[Service]\nExecStart=%h/.local/bin/mdn serve\n")
	out := filepath.Join(dir, "out")

	if err := run([]string{
		"-version", "v1.2.3", "-sums", sums, "-license", license, "-unit", unit,
		"-maintainer", maintainerFileWithout(t, dir, placeholderMarker), "-out", out,
	}, os.Stderr); err != nil {
		t.Fatal(err)
	}

	pkgbuild := read(t, filepath.Join(out, "PKGBUILD"))
	srcinfo := read(t, filepath.Join(out, ".SRCINFO"))
	for _, name := range []string{"PKGBUILD", ".SRCINFO"} {
		body := pkgbuild
		if name == ".SRCINFO" {
			body = srcinfo
		}
		if strings.Contains(body, skip) {
			t.Errorf("%s still carries %s after a real rendering", name, skip)
		}
		if !strings.Contains(body, "1.2.3") || strings.Contains(body, "0.0.0") {
			t.Errorf("%s does not carry the version it was rendered for", name)
		}
	}
	for _, want := range []string{
		"pkgver=1.2.3",
		"sha256sums_x86_64=('" + amd64Sum + "')",
		"sha256sums_aarch64=('" + arm64Sum + "')",
	} {
		if !strings.Contains(pkgbuild, want) {
			t.Errorf("the rendered PKGBUILD does not contain %q", want)
		}
	}
	// The PKGBUILD reaches the assets through $pkgver, so it is .SRCINFO that
	// carries the URLs a release is actually fetched from — and .SRCINFO is
	// what the AUR reads.
	for _, want := range []string{
		"sha256sums_x86_64 = " + amd64Sum,
		"sha256sums_aarch64 = " + arm64Sum,
		"releases/download/v1.2.3/mdn-v1.2.3-linux-amd64",
		"releases/download/v1.2.3/mdn-v1.2.3-linux-arm64",
		"raw.githubusercontent.com/davison/md-notes/v1.2.3/contrib/mdn.service",
	} {
		if !strings.Contains(srcinfo, want) {
			t.Errorf("the rendered .SRCINFO does not contain %q", want)
		}
	}
	// The licence and the unit are hashed from the tree rather than read from
	// the release, which carries neither.
	if strings.Contains(pkgbuild, zipSum) {
		t.Error("the extension zip's checksum reached the PKGBUILD")
	}
	if !strings.Contains(pkgbuild, "sha256sums=('"+sha256Of(t, license)+"'") ||
		!strings.Contains(pkgbuild, "            '"+sha256Of(t, unit)+"')") {
		t.Errorf("the licence and the unit are not hashed from the files given:\n%s", pkgbuild)
	}
}

// TestARealRenderingRefusesWhatWouldReachTheAURWrong covers the four ways this
// could push something it should not: a version that is not a release tag, a
// SHA256SUMS that is not this release's, a maintainer line still holding the
// gate's placeholder, and a bare -placeholder asked to do a real release.
func TestARealRenderingRefusesWhatWouldReachTheAURWrong(t *testing.T) {
	dir := t.TempDir()
	sums := filepath.Join(dir, "SHA256SUMS")
	write(t, sums, strings.Repeat("a", 64)+"  mdn-v1.2.3-linux-amd64\n")
	license := filepath.Join(dir, "LICENSE")
	write(t, license, "MIT\n")
	real := maintainerFileWithout(t, dir, placeholderMarker)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a SHA256SUMS missing an architecture",
			args: []string{"-version", "v1.2.3", "-sums", sums, "-license", license, "-unit", license, "-maintainer", real},
			want: "mdn-v1.2.3-linux-arm64",
		},
		{
			name: "the SHA256SUMS of another release",
			args: []string{"-version", "v9.9.9", "-sums", sums, "-license", license, "-unit", license, "-maintainer", real},
			want: "mdn-v9.9.9-linux-amd64",
		},
		{
			name: "a version that is not a release tag",
			args: []string{"-version", "1.2.3", "-sums", sums, "-license", license, "-unit", license, "-maintainer", real},
			want: "mdn-1.2.3-linux-amd64",
		},
		{
			name: "the maintainer gate still unanswered",
			args: []string{"-version", "v1.2.3", "-sums", sums, "-license", license, "-unit", license, "-maintainer", maintainerFile},
			want: "placeholder",
		},
		{
			name: "no checksums at all",
			args: []string{"-version", "v1.2.3", "-maintainer", real},
			want: "-sums is required",
		},
		{
			name: "the template asked to stand in for a release",
			args: []string{"-placeholder", "-version", "v1.2.3", "-maintainer", real},
			want: "neither -version nor -sums",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out")
			err := run(append(tc.args, "-out", out), &bytes.Buffer{})
			if err == nil {
				t.Fatal("rendered, want refused")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refused with %q, want something naming %q", err, tc.want)
			}
			if _, statErr := os.Stat(filepath.Join(out, "PKGBUILD")); statErr == nil {
				t.Error("a refused rendering still wrote a PKGBUILD")
			}
		})
	}
}

// TestTheSRCINFOIsWhatMakepkgPrints is the one that matters: the AUR reads the
// version off .SRCINFO and refuses a push whose last commit lacks it, and this
// renderer writes it from a template rather than calling makepkg, because the
// runner that renders has no makepkg on it. So the two have to be compared
// somewhere. Here, wherever makepkg exists — and in the workflow, inside the
// Arch container, where it cannot skip.
func TestTheSRCINFOIsWhatMakepkgPrints(t *testing.T) {
	makepkg, err := exec.LookPath("makepkg")
	if err != nil {
		t.Skip("makepkg is Arch's; the publish-aur workflow runs this same diff in an archlinux container")
	}
	for _, version := range []string{placeholderVersion, "v10.20.300"} {
		t.Run(version, func(t *testing.T) {
			in := inputs{
				Maintainer: "A Maintainer <a at b dot c>",
				Version:    version,
				PkgRel:     1,
				SumLicense: strings.Repeat("a", 64),
				SumUnit:    strings.Repeat("b", 64),
				SumAMD64:   strings.Repeat("c", 64),
				SumARM64:   strings.Repeat("d", 64),
			}
			pkgbuild, srcinfo, err := render(in)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			write(t, filepath.Join(dir, "PKGBUILD"), pkgbuild)
			// makepkg refuses a PKGBUILD whose install file is missing.
			write(t, filepath.Join(dir, pkgname+".install"), read(t, filepath.Join(packaging, pkgname+".install")))

			cmd := exec.Command(makepkg, "--printsrcinfo")
			cmd.Dir = dir
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			printed, err := cmd.Output()
			if err != nil {
				t.Fatalf("makepkg --printsrcinfo: %v: %s", err, stderr.String())
			}
			if string(printed) != srcinfo {
				t.Errorf("the rendered .SRCINFO is not what makepkg prints.\n--- makepkg\n%s\n--- rendered\n%s", printed, srcinfo)
			}
		})
	}
}

// maintainerFileWithout writes a maintainer file that has been through the
// gate: a real line, with none of the placeholder in it.
func maintainerFileWithout(t *testing.T, dir, marker string) string {
	t.Helper()
	line := "A Maintainer <a at b dot c>"
	if strings.Contains(line, marker) {
		t.Fatalf("the stand-in maintainer line contains %q", marker)
	}
	path := filepath.Join(dir, "MAINTAINER")
	write(t, path, line+"\n")
	return path
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func sha256Of(t *testing.T, path string) string {
	t.Helper()
	sum, err := fileSum(path)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}
