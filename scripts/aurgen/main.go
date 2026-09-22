// Command aurgen renders the AUR package's PKGBUILD and .SRCINFO.
//
// The package is md-notes-bin: it installs the binaries a release publishes
// rather than building them, so everything that changes between releases is a
// version and five checksums. Two of those checksums are the release's own,
// read from its SHA256SUMS asset; the other three are of files in the tagged
// tree — the licence, the unit and the manual page — hashed here rather than
// guessed.
//
//	go run ./scripts/aurgen -version v0.1.0 -sums SHA256SUMS
//
// writes packaging/aur/PKGBUILD and packaging/aur/.SRCINFO for that release.
// The pair committed to this repository is what
//
//	go run ./scripts/aurgen -placeholder
//
// produces: version 0.0.0 and every checksum SKIP, so the shape is reviewable
// in a diff without inventing a release that does not exist. A test holds the
// committed files to that output, and .github/workflows/publish-aur.yml
// refuses to push anything still carrying SKIP.
//
// .SRCINFO is written here rather than shelled out to `makepkg --printsrcinfo`
// because the workflow renders on a runner that has no makepkg. It is the same
// file: a test compares the two byte for byte wherever makepkg exists, and the
// workflow runs the same diff inside the Arch container before it pushes.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "aurgen: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("aurgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		version     = fs.String("version", "", "release tag to render for, e.g. v0.1.0")
		placeholder = fs.Bool("placeholder", false, "render the committed template: version 0.0.0 and every checksum SKIP")
		sums        = fs.String("sums", "", "the release's SHA256SUMS asset")
		licensePath = fs.String("license", "LICENSE", "the packaged software's licence, hashed into the source array")
		unitPath    = fs.String("unit", filepath.Join("contrib", "mdn.service"), "the systemd user unit, hashed into the source array")
		manPath     = fs.String("manpage", filepath.Join("contrib", "mdn.1"), "the manual page, placeholders and all, hashed into the source array")
		maintainer  = fs.String("maintainer", filepath.Join("packaging", "aur", "MAINTAINER"), "file holding the # Maintainer: line's content")
		pkgrel      = fs.Int("pkgrel", 1, "package release, bumped only for packaging changes at the same version")
		out         = fs.String("out", filepath.Join("packaging", "aur"), "directory to write PKGBUILD and .SRCINFO into")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	in := inputs{PkgRel: *pkgrel}
	line, err := maintainerLine(*maintainer)
	if err != nil {
		return err
	}
	in.Maintainer = line

	switch {
	case *placeholder:
		if *version != "" || *sums != "" {
			return fmt.Errorf("-placeholder renders the committed template and takes neither -version nor -sums")
		}
		in.Version = placeholderVersion
		in.SumLicense, in.SumUnit, in.SumManpage, in.SumAMD64, in.SumARM64 = skip, skip, skip, skip, skip
	default:
		if *version == "" {
			return fmt.Errorf("-version is required (or -placeholder for the committed template)")
		}
		if *sums == "" {
			return fmt.Errorf("-sums is required: the release's SHA256SUMS carries the binaries' checksums")
		}
		if strings.Contains(in.Maintainer, placeholderMarker) {
			return fmt.Errorf("the maintainer line in %s is still the placeholder (%q): it is the first line the AUR shows, and it is answered on davison/md-notes#136", *maintainer, in.Maintainer)
		}
		in.Version = *version
		if in.SumAMD64, in.SumARM64, err = releaseSums(*sums, in.Version); err != nil {
			return err
		}
		if in.SumLicense, err = fileSum(*licensePath); err != nil {
			return err
		}
		if in.SumUnit, err = fileSum(*unitPath); err != nil {
			return err
		}
		if in.SumManpage, err = fileSum(*manPath); err != nil {
			return err
		}
	}

	pkgbuild, srcinfo, err := render(in)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "PKGBUILD"), []byte(pkgbuild), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, ".SRCINFO"), []byte(srcinfo), 0o644)
}

const (
	// skip is what a checksum reads as in the committed template: a version
	// that was never released has no assets to hash. Nothing carrying it is
	// ever pushed — the workflow's render fails without -sums, and its last
	// step before the AUR refuses a PKGBUILD that still says it.
	skip = "SKIP"

	// placeholderVersion is the committed template's. It is a valid pkgver, so
	// makepkg --printsrcinfo and namcap both read the committed files, and it
	// names no release anyone could have.
	placeholderVersion = "v0.0.0"

	// placeholderMarker is what an unanswered maintainer gate left in
	// packaging/aur/MAINTAINER before the operator answered it on
	// davison/md-notes#136 (plain name and address, not obfuscated). The
	// refusal outlives the answer: the file can be emptied or reset, and what
	// it holds is the first line the AUR shows.
	placeholderMarker = "example dot invalid"

	pkgname = "md-notes-bin"
)

// inputs are everything that differs between one rendering and the next. The
// rest of the package — what it depends on, where it installs, what it
// provides — is the template below.
type inputs struct {
	Maintainer string
	Version    string // the tag, with its leading v
	PkgRel     int
	SumLicense string
	SumUnit    string
	SumManpage string
	SumAMD64   string
	SumARM64   string
}

// PkgVer is the version a pkgver takes: the tag without its v. The asset URLs
// want the v and the package version cannot have one, which docs/releasing.md
// says in as many words.
func (in inputs) PkgVer() string { return strings.TrimPrefix(in.Version, "v") }

// PkgName is in the template's reach so the .SRCINFO and the PKGBUILD cannot
// disagree about it.
func (in inputs) PkgName() string { return pkgname }

var (
	versionPattern  = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	checksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func render(in inputs) (pkgbuild, srcinfo string, err error) {
	if !versionPattern.MatchString(in.Version) {
		return "", "", fmt.Errorf("not a v<major>.<minor>.<patch> version: %q", in.Version)
	}
	if in.PkgRel < 1 {
		return "", "", fmt.Errorf("pkgrel starts at 1, got %d", in.PkgRel)
	}
	if in.Maintainer == "" {
		return "", "", fmt.Errorf("the maintainer line is empty")
	}
	for _, sum := range []struct{ what, value string }{
		{"the licence", in.SumLicense},
		{"the unit", in.SumUnit},
		{"the manual page", in.SumManpage},
		{"the amd64 binary", in.SumAMD64},
		{"the arm64 binary", in.SumARM64},
	} {
		if sum.value != skip && !checksumPattern.MatchString(sum.value) {
			return "", "", fmt.Errorf("the checksum of %s is neither %s nor 64 lowercase hex digits: %q", sum.what, skip, sum.value)
		}
	}

	var b strings.Builder
	if err := pkgbuildTemplate.Execute(&b, in); err != nil {
		return "", "", err
	}
	pkgbuild = b.String()

	b.Reset()
	if err := srcinfoTemplate.Execute(&b, in); err != nil {
		return "", "", err
	}
	return pkgbuild, b.String(), nil
}

// maintainerLine reads the one line the AUR sees first. Keeping it in a file
// of its own meant the gate's answer — the name, the address, whether the
// address is obfuscated — was a one-line edit rather than a change to the
// renderer, and it still is if the operator ever changes it.
func maintainerLine(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(raw))
	if line == "" {
		return "", fmt.Errorf("%s is empty: it holds the content of the PKGBUILD's # Maintainer: line", path)
	}
	if strings.ContainsAny(line, "\n\r") {
		return "", fmt.Errorf("%s has more than one line: the PKGBUILD's first line is one maintainer", path)
	}
	return line, nil
}

// releaseSums picks the two binaries' checksums out of the release's
// SHA256SUMS, which carries the extension zip's as well. A missing line is an
// error rather than a SKIP: a package whose integrity check was quietly
// dropped is the thing the Arch guidelines single out.
func releaseSums(path, version string) (amd64, arm64 string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	want := []struct {
		asset string
		into  *string
	}{
		{fmt.Sprintf("mdn-%s-linux-amd64", version), &amd64},
		{fmt.Sprintf("mdn-%s-linux-arm64", version), &arm64},
	}
	for _, line := range strings.Split(string(raw), "\n") {
		// sha256sum's own format: the digest, two spaces (or a space and a
		// mode character), then the name.
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		for _, w := range want {
			if strings.TrimPrefix(fields[1], "*") == w.asset {
				*w.into = fields[0]
			}
		}
	}
	for _, w := range want {
		if !checksumPattern.MatchString(*w.into) {
			return "", "", fmt.Errorf("%s names no sha256 for %s: is it the SHA256SUMS of release %s?", path, w.asset, version)
		}
	}
	return amd64, arm64, nil
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// The PKGBUILD. Field order follows the PKGBUILD article, as the Arch package
// guidelines suggest, and every line stays under 100 characters.
//
// Both architectures rename their binary to the same name in srcdir, so
// package() needs no branch on $CARCH — and the sources are renamed at all
// because the guidelines require them to be unique in srcdir.
//
// The licence, the unit and the manual page come from the tagged tree over
// raw.githubusercontent rather than from the Release, because the release's
// assets are the bare binaries, the extension zip and SHA256SUMS: no tarball
// carries the other three. A tag cut before the page moved to contrib/
// (v0.1.0) has no page at that URL, so this renders a package only for a
// release made after davison/md-notes#168.
//
// The page is fetched with its @VERSION@ and @DATE@ placeholders and filled in
// by package(): the version from $pkgver, as the .deb writes it, and the date
// from SOURCE_DATE_EPOCH, which makepkg always sets — to the build time, or to
// the recorded one when a package is rebuilt to be checked, so the page does
// not stop the package being reproducible. makepkg's zipman compresses it.
var pkgbuildTemplate = template.Must(template.New("PKGBUILD").Parse(
	`# Maintainer: {{.Maintainer}}
# Rendered by scripts/aurgen in davison/md-notes, from the version and the
# SHA256SUMS of a GitHub Release. Edit the renderer, not this file.
pkgname={{.PkgName}}
pkgver={{.PkgVer}}
pkgrel={{.PkgRel}}
pkgdesc='Turns folders of markdown files into a notes application in the browser'
arch=('x86_64' 'aarch64')
url='https://github.com/davison/md-notes'
license=('MIT')
depends=('ripgrep')
optdepends=('xdg-utils: mdn open launches a browser')
provides=("md-notes=$pkgver")
conflicts=('md-notes')
# The binary is the release's own, already stripped at link time (-s -w): left
# alone it is byte-identical to the asset SHA256SUMS covers, and there is no
# debug package to be made from it.
options=('!strip' '!debug')
install={{.PkgName}}.install
source=("md-notes-$pkgver-LICENSE::https://raw.githubusercontent.com/davison/md-notes/v$pkgver/LICENSE"
        "md-notes-$pkgver-mdn.service::https://raw.githubusercontent.com/davison/md-notes/v$pkgver/contrib/mdn.service"
        "md-notes-$pkgver-mdn.1::https://raw.githubusercontent.com/davison/md-notes/v$pkgver/contrib/mdn.1")
sha256sums=('{{.SumLicense}}'
            '{{.SumUnit}}'
            '{{.SumManpage}}')
source_x86_64=("md-notes-$pkgver-mdn::https://github.com/davison/md-notes/releases/download/v$pkgver/mdn-v$pkgver-linux-amd64")
sha256sums_x86_64=('{{.SumAMD64}}')
source_aarch64=("md-notes-$pkgver-mdn::https://github.com/davison/md-notes/releases/download/v$pkgver/mdn-v$pkgver-linux-arm64")
sha256sums_aarch64=('{{.SumARM64}}')

package() {
	install -Dm755 "$srcdir/md-notes-$pkgver-mdn" "$pkgdir/usr/bin/mdn"
	# Verbatim, byte for byte, from the tag: the unit starts /usr/bin/mdn,
	# which is where this package puts the binary, and its header is written
	# for the reader who installed the package — including the drop-in for a
	# local prefix. davison/md-notes#137 settled that; verify.sh compares the
	# installed file with the one in the tagged tree.
	install -Dm644 "$srcdir/md-notes-$pkgver-mdn.service" \
		"$pkgdir/usr/lib/systemd/user/mdn.service"
	install -Dm644 "$srcdir/md-notes-$pkgver-LICENSE" \
		"$pkgdir/usr/share/licenses/$pkgname/LICENSE"
	# The same page the .deb and make install carry, filled in the same way.
	install -d "$pkgdir/usr/share/man/man1"
	sed -e "s/@VERSION@/$pkgver/g" \
		-e "s/@DATE@/$(date -u -d "@$SOURCE_DATE_EPOCH" +%Y-%m-%d)/g" \
		"$srcdir/md-notes-$pkgver-mdn.1" >"$pkgdir/usr/share/man/man1/mdn.1"
	chmod 644 "$pkgdir/usr/share/man/man1/mdn.1"
}
`))

// The .SRCINFO, in the order and spacing `makepkg --printsrcinfo` writes:
// tab-indented under the pkgbase, generic sources and their checksums before
// the per-architecture ones, a blank line before the pkgname stanza. A test
// compares the two.
var srcinfoTemplate = template.Must(template.New(".SRCINFO").Parse(
	`pkgbase = {{.PkgName}}
	pkgdesc = Turns folders of markdown files into a notes application in the browser
	pkgver = {{.PkgVer}}
	pkgrel = {{.PkgRel}}
	url = https://github.com/davison/md-notes
	install = {{.PkgName}}.install
	arch = x86_64
	arch = aarch64
	license = MIT
	depends = ripgrep
	optdepends = xdg-utils: mdn open launches a browser
	provides = md-notes={{.PkgVer}}
	conflicts = md-notes
	options = !strip
	options = !debug
	source = md-notes-{{.PkgVer}}-LICENSE::https://raw.githubusercontent.com/davison/md-notes/v{{.PkgVer}}/LICENSE
	source = md-notes-{{.PkgVer}}-mdn.service::https://raw.githubusercontent.com/davison/md-notes/v{{.PkgVer}}/contrib/mdn.service
	source = md-notes-{{.PkgVer}}-mdn.1::https://raw.githubusercontent.com/davison/md-notes/v{{.PkgVer}}/contrib/mdn.1
	sha256sums = {{.SumLicense}}
	sha256sums = {{.SumUnit}}
	sha256sums = {{.SumManpage}}
	source_x86_64 = md-notes-{{.PkgVer}}-mdn::https://github.com/davison/md-notes/releases/download/v{{.PkgVer}}/mdn-v{{.PkgVer}}-linux-amd64
	sha256sums_x86_64 = {{.SumAMD64}}
	source_aarch64 = md-notes-{{.PkgVer}}-mdn::https://github.com/davison/md-notes/releases/download/v{{.PkgVer}}/mdn-v{{.PkgVer}}-linux-arm64
	sha256sums_aarch64 = {{.SumARM64}}

pkgname = {{.PkgName}}
`))
