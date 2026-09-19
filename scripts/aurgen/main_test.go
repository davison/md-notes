package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
	// The shape contrib/mdn.service has since the gate on davison/md-notes#137
	// resolved: the packaged path, installed verbatim. All this test does with
	// it is hash it, but a fixture that still carried the old ~/.local/bin line
	// would read as though something here still rewrote it.
	write(t, unit, "[Service]\nExecStart=/usr/bin/mdn serve\n")
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
			name: "a maintainer line still holding the gate's placeholder",
			args: []string{"-version", "v1.2.3", "-sums", sums, "-license", license, "-unit", license, "-maintainer", placeholderMaintainerFile(t, dir)},
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

// placeholderMaintainerFile writes the maintainer file as the gate on
// davison/md-notes#136 left it before the operator answered: a line nobody
// should ever push to the AUR. The refusal outlives the answer, because the
// file can be emptied or reset by anyone.
func placeholderMaintainerFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "MAINTAINER.placeholder")
	write(t, path, "Someone <maintainer "+placeholderMarker+">\n")
	return path
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

// TestThePackageStillDescribesTheProgramItPackages is the check the AUR
// submission guidelines ask a maintainer for and that no automation here
// otherwise does: "projects can change license, add or remove dependencies,
// and other notable changes even for 'minor' releases".
//
// `license` and `depends` are literals in the template. The operator's publish
// click reads the release notes, not the PKGBUILD, and namcap cannot see a
// relicensing or a new subprocess — a Go binary's dependencies on other
// programs are invisible to it, which is why it calls ripgrep redundant. So
// the drift is caught here, against the tree the package packages: the
// licence, and every program the code runs.
//
// It reads literals, so a subprocess launched through a variable would slip
// past. Both of the ones here are literal, and a test that catches the common
// case beats the one nobody writes.
func TestThePackageStillDescribesTheProgramItPackages(t *testing.T) {
	pkgbuild := read(t, filepath.Join(packaging, "PKGBUILD"))

	license := read(t, filepath.Join("..", "..", "LICENSE"))
	if first, _, _ := strings.Cut(license, "\n"); first != "MIT License" {
		t.Errorf("the repository's LICENSE now begins %q; the PKGBUILD's license=() field is the upstream licence in SPDX form and must follow it", first)
	}
	if !strings.Contains(pkgbuild, "license=('MIT')") {
		t.Error("the PKGBUILD does not declare license=('MIT')")
	}

	// Every program the daemon and the client run, and what the package has to
	// say so that it is there. A dependency is `depends` when the package does
	// not work without it and `optdepends` when one command does not.
	declares := map[string]string{
		"rg":       "depends=('ripgrep')",
		"xdg-open": "optdepends=('xdg-utils:",
	}
	found := executables(t, filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "internal"))
	for _, program := range found {
		declared, known := declares[program]
		if !known {
			t.Errorf("the code now runs %q and the PKGBUILD says nothing about it: add the package providing it to depends or optdepends, and name it here", program)
			continue
		}
		if !strings.Contains(pkgbuild, declared) {
			t.Errorf("the code runs %q but the PKGBUILD has no %s", program, declared)
		}
	}
	for program, declared := range declares {
		if !slices.Contains(found, program) {
			t.Errorf("nothing in cmd/ or internal/ runs %q any more, but the PKGBUILD still carries %s", program, declared)
		}
	}
}

// executables collects the programs named as literals in exec.Command,
// exec.CommandContext, exec.LookPath and this repository's lookPath
// indirection, under the given directories, ignoring test files.
func executables(t *testing.T, dirs ...string) []string {
	t.Helper()
	call := regexp.MustCompile(`(?:exec\.Command|exec\.CommandContext|exec\.LookPath|lookPath)\(\s*(?:[A-Za-z_][A-Za-z0-9_.]*,\s*)?"([^"]+)"`)
	var found []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			for _, match := range call.FindAllStringSubmatch(read(t, path), -1) {
				if !slices.Contains(found, match[1]) {
					found = append(found, match[1])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) == 0 {
		t.Fatal("found no subprocess at all in cmd/ or internal/: the scan is broken, not the code")
	}
	slices.Sort(found)
	return found
}
