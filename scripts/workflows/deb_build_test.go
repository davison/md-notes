// Assertions over the Debian package itself (davison/md-notes#137):
// packaging/deb/nfpm.yaml, which says what the package is, and
// packaging/deb/build.sh, which says what goes into it.
//
// The build script is run for real, against a stub nfpm that records the
// environment and the directory it was handed. Asserting things about the
// script's text instead would prove nothing about the package: what reaches
// nfpm is the product of a regular expression, a sed and a staging directory,
// and every bug this test has to catch lives in the gap between what those
// lines look like and what they do.
package workflows

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The repository root, from this package's directory.
const repoRoot = "../.."

type nfpmConfig struct {
	Name          string   `yaml:"name"`
	Arch          string   `yaml:"arch"`
	Platform      string   `yaml:"platform"`
	Version       string   `yaml:"version"`
	VersionSchema string   `yaml:"version_schema"`
	Section       string   `yaml:"section"`
	Priority      string   `yaml:"priority"`
	Maintainer    string   `yaml:"maintainer"`
	Homepage      string   `yaml:"homepage"`
	License       string   `yaml:"license"`
	Description   string   `yaml:"description"`
	Depends       []string `yaml:"depends"`
	Contents      []struct {
		Src      string `yaml:"src"`
		Dst      string `yaml:"dst"`
		FileInfo struct {
			Mode any `yaml:"mode"`
		} `yaml:"file_info"`
	} `yaml:"contents"`
}

func packageConfig(t *testing.T) nfpmConfig {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "packaging", "deb", "nfpm.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed nfpmConfig
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

// TestThePackageIsWhatM8R4Asks. Every field here is named by the requirement
// or by Debian policy, and each is one line that nothing else would notice the
// loss of: a package missing `Depends: ripgrep` installs perfectly and then
// serves an empty navigator, and a copyright file at the wrong path is a
// policy violation a user never sees.
func TestThePackageIsWhatM8R4Asks(t *testing.T) {
	config := packageConfig(t)

	// The name settled on davison/md-notes#136 and recorded on #137: `mdn` is
	// taken on the AUR by an unrelated project, so both packages are
	// md-notes and the binary stays mdn.
	if config.Name != "md-notes" {
		t.Errorf("the package is named %q, want md-notes", config.Name)
	}
	if want := []string{"ripgrep"}; len(config.Depends) != 1 || config.Depends[0] != want[0] {
		t.Errorf("depends is %v, want %v: the daemon shells out to rg to list and to search a root", config.Depends, want)
	}
	if config.Section != "utils" {
		t.Errorf("section is %q, want utils", config.Section)
	}
	if config.Priority != "optional" {
		t.Errorf("priority is %q, want optional", config.Priority)
	}
	if !strings.Contains(config.Maintainer, "<") || !strings.Contains(config.Maintainer, "@") {
		t.Errorf("maintainer is %q, want `Name <address>`: dpkg and lintian both read this field", config.Maintainer)
	}
	if !strings.HasPrefix(config.Homepage, "https://") {
		t.Errorf("homepage is %q", config.Homepage)
	}
	if config.License == "" {
		t.Error("no license field")
	}

	// nfpm expands environment variables in these and not in `contents`, which
	// is why the file list below is bare names. If that ever stops being true
	// the package is built for one architecture twice.
	if !strings.Contains(config.Arch, "$") || !strings.Contains(config.Version, "$") {
		t.Errorf("arch=%q version=%q: build.sh passes both in through the environment", config.Arch, config.Version)
	}
	// Left to nfpm's semver schema, a version with a prerelease or metadata in
	// it comes out as a different string than the one build.sh put in the file
	// name.
	if config.VersionSchema != "none" {
		t.Errorf("version_schema is %q, want none: the version build.sh derives is the version the package carries", config.VersionSchema)
	}

	// The synopsis, then the extended description. dpkg renders the first line
	// on its own, so a missing one makes every other line the summary.
	if lines := strings.SplitN(config.Description, "\n", 2); len(lines) < 2 || lines[0] == "" {
		t.Error("the description has no synopsis line followed by an extended description")
	}

	want := map[string]string{
		// The three M8-R4 names.
		"/usr/bin/mdn":                      "0755",
		"/usr/lib/systemd/user/mdn.service": "0644",
		"/usr/share/doc/md-notes/copyright": "0644",
		// And the three Debian policy and lintian ask for, without which the
		// package does not pass the check M8-R4 ends on.
		"/usr/share/doc/md-notes/changelog.gz":  "0644",
		"/usr/share/man/man1/mdn.1.gz":          "0644",
		"/usr/share/lintian/overrides/md-notes": "0644",
	}
	got := map[string]string{}
	for _, entry := range config.Contents {
		if strings.Contains(entry.Src, "$") || strings.HasPrefix(entry.Src, "/") {
			t.Errorf("content %q has src %q; nfpm does not expand variables in contents and resolves the path against the working directory, so build.sh stages everything under a bare name", entry.Dst, entry.Src)
		}
		got[entry.Dst] = strings.TrimPrefix(modeString(entry.FileInfo.Mode), "0o")
	}
	for dst, mode := range want {
		if got[dst] == "" {
			t.Errorf("the package does not install %s", dst)
			continue
		}
		if got[dst] != mode {
			t.Errorf("%s has mode %s, want %s", dst, got[dst], mode)
		}
	}
	for dst := range got {
		if want[dst] == "" {
			t.Errorf("the package installs %s, which no requirement asks for", dst)
		}
	}
}

// modeString renders a YAML mode however it was written: 0755 unquoted is an
// octal integer to a YAML parser, "0755" is a string.
func modeString(mode any) string {
	switch m := mode.(type) {
	case string:
		return m
	case int:
		return "0" + strconv.FormatInt(int64(m), 8)
	default:
		return ""
	}
}

// build runs packaging/deb/build.sh against a stub nfpm and reports what nfpm
// was handed: one record per architecture, with the staging directory it was
// run in copied aside.
type invocation struct {
	arch    string
	version string
	target  string
	staging string // a copy of the directory nfpm ran in
}

const stubNfpm = `#!/bin/sh
# Stands in for nfpm under scripts/workflows/deb_build_test.go. Records the
# environment and a copy of the directory it was run in, then writes an empty
# file where the package would go.
set -e
target=
while [ $# -gt 0 ]; do
	case "$1" in
	--target) target="$2"; shift 2 ;;
	*) shift ;;
	esac
done
dir="$STUB_RECORD/$DEB_ARCH"
mkdir -p "$dir"
cp -a ./. "$dir/"
printf '%s\t%s\t%s\n' "$DEB_ARCH" "$DEB_VERSION" "$target" >>"$STUB_RECORD/invocations"
: >"$target"
`

func runBuild(t *testing.T, tag string, binaries map[string]string) ([]invocation, string, error) {
	t.Helper()
	return runBuildAgainstUnit(t, tag, binaries, "")
}

// runBuildAgainstUnit is runBuild against a copy of the repository whose
// contrib/mdn.service is the given text, so a test can ask what the packaging
// does with a unit file this repository does not have yet. An empty unit runs
// the script where it lives, against the real one.
func runBuildAgainstUnit(t *testing.T, tag string, binaries map[string]string, unit string) ([]invocation, string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash runs the build script")
	}

	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	out := filepath.Join(dir, "out")
	record := filepath.Join(dir, "record")
	for _, d := range []string{assets, record} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for arch, content := range binaries {
		name := filepath.Join(assets, "mdn-"+tag+"-linux-"+arch)
		// 0644, as a binary downloaded over HTTP arrives: the script is what
		// has to make it executable.
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stub := filepath.Join(dir, "nfpm")
	if err := os.WriteFile(stub, []byte(stubNfpm), 0o755); err != nil {
		t.Fatal(err)
	}

	root := repoRoot
	if unit != "" {
		root = fakeRepo(t, filepath.Join(dir, "repo"), unit)
	}
	script, err := filepath.Abs(filepath.Join(root, "packaging", "deb", "build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(script, tag, assets, out)
	cmd.Env = append(os.Environ(),
		"NFPM="+stub,
		"STUB_RECORD="+record,
		// A fixed date, so the changelog entry this test reads is the one the
		// workflow would write for a release published at that moment.
		"SOURCE_DATE_EPOCH=1758153600",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var invocations []invocation
	if raw, err := os.ReadFile(filepath.Join(record, "invocations")); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) != 3 {
				continue
			}
			invocations = append(invocations, invocation{
				arch:    fields[0],
				version: fields[1],
				target:  fields[2],
				staging: filepath.Join(record, fields[0]),
			})
		}
	}
	return invocations, stderr.String(), runErr
}

// TestBuildPackagesBothArchitectures: one run, two packages, each made of its
// own binary, both carrying the version the tag names with its v stripped.
func TestBuildPackagesBothArchitectures(t *testing.T) {
	invocations, stderr, err := runBuild(t, "v0.1.0", map[string]string{
		"amd64": "the amd64 binary",
		"arm64": "the arm64 binary",
	})
	if err != nil {
		t.Fatalf("build.sh failed: %v\n%s", err, stderr)
	}
	if len(invocations) != 2 {
		t.Fatalf("nfpm ran %d times, want 2 — one per architecture: %v", len(invocations), invocations)
	}

	seen := map[string]invocation{}
	for _, inv := range invocations {
		seen[inv.arch] = inv
	}
	for _, arch := range []string{"amd64", "arm64"} {
		inv, ok := seen[arch]
		if !ok {
			t.Errorf("nfpm never ran for %s", arch)
			continue
		}
		// The v belongs to the tag and to the asset names, not to a Debian
		// version: dpkg would sort a leading v before every digit.
		if inv.version != "0.1.0" {
			t.Errorf("%s was built as version %q, want 0.1.0 from the tag v0.1.0", arch, inv.version)
		}
		if want := "md-notes_0.1.0_" + arch + ".deb"; filepath.Base(inv.target) != want {
			t.Errorf("%s was written to %q, want a file named %q", arch, inv.target, want)
		}
		// Each package gets its own binary. Staging both architectures
		// through one file name is exactly the mistake that would put the
		// amd64 binary in the arm64 package, and nothing downstream would
		// notice until it was installed.
		staged, err := os.ReadFile(filepath.Join(inv.staging, "mdn"))
		if err != nil {
			t.Errorf("%s: no binary staged: %v", arch, err)
			continue
		}
		if string(staged) != "the "+arch+" binary" {
			t.Errorf("the %s package was built from %q", arch, staged)
		}
		info, err := os.Stat(filepath.Join(inv.staging, "mdn"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("the staged %s binary is mode %o, want 755: the asset arrives over HTTP without its executable bit", arch, info.Mode().Perm())
		}
	}
}

// TestOnlyAReleaseVersionIsPackaged. The version reaches a package file name,
// a control field and a command line, and dpkg's rules are narrower than a
// shell's. It is also the thing that decides whether the package is native:
// anything with a hyphen in it turns the part after the last one into a Debian
// revision, and the changelog this package ships as changelog.gz would then
// have to be named changelog.Debian.gz for lintian — so a version outside the
// release shape does not produce a worse package, it produces one that fails
// the check M8-R4 ends on, for a reason nothing would connect to the version.
func TestOnlyAReleaseVersionIsPackaged(t *testing.T) {
	binaries := func(tag string) map[string]string {
		return map[string]string{"amd64": "a", "arm64": "b"}
	}

	t.Run("accepts a release tag", func(t *testing.T) {
		for _, tag := range []string{"v0.0.0", "v0.1.0", "v1.20.300"} {
			invocations, stderr, err := runBuild(t, tag, binaries(tag))
			if err != nil {
				t.Errorf("tag %q: %v\n%s", tag, err, stderr)
			}
			if len(invocations) != 2 {
				t.Errorf("tag %q produced %d packages, want 2", tag, len(invocations))
			}
		}
	})

	t.Run("refuses anything else, and packages nothing", func(t *testing.T) {
		for _, tag := range []string{
			"v0.1.0-rc1",   // a prerelease: the hyphen makes the package non-native
			"v0.0.0-test",  // and so does a local proving build's suffix
			"0.1.0",        // the v is part of the shape release.yml publishes
			"v1.2",         // too few components
			"v1.2.3.4",     // too many
			"v1_2_3",       // an underscore is not legal in a Debian version
			"v1.2.3 ",      // trailing whitespace
			"v1.2.3/../x",  // a path, aimed at the output directory
			"$(id)",        // a command, aimed at the shell
			"",             //
			"latest",       //
			"v1.2.3\nx1.0", // a second line
		} {
			invocations, _, err := runBuild(t, tag, binaries(tag))
			if err == nil {
				t.Errorf("tag %q was accepted, want it refused", tag)
			}
			if len(invocations) != 0 {
				t.Errorf("tag %q was refused but nfpm still ran %d times", tag, len(invocations))
			}
		}
	})
}

// TestThePackagedUnitIsTheRepositorysUnit: the package ships
// contrib/mdn.service, byte for byte, and that file runs the binary the
// package installs.
//
// It did not always. The packaging used to rewrite the ExecStart, because the
// unit ran %h/.local/bin/mdn and a package cannot — the gate raised on
// davison/md-notes#137. The operator resolved it as option (b): `make install`
// installs system-wide by default and the unit runs /usr/bin/mdn, so the two
// routes agree on one path and the packaging has nothing left to change. This
// test is what keeps that true from the package's side — a byte-for-byte copy
// is a claim worth checking precisely because it looks like it cannot fail.
func TestThePackagedUnitIsTheRepositorysUnit(t *testing.T) {
	invocations, stderr, err := runBuild(t, "v0.1.0", map[string]string{"amd64": "a", "arm64": "b"})
	if err != nil {
		t.Fatalf("build.sh failed: %v\n%s", err, stderr)
	}
	staged, err := os.ReadFile(filepath.Join(invocations[0].staging, "mdn.service"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(repoRoot, "contrib", "mdn.service"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(staged, source) {
		t.Errorf("the packaged unit is not contrib/mdn.service verbatim:\n--- packaged ---\n%s\n--- contrib ---\n%s", staged, source)
	}
	// Said separately, because the equality above would be just as happy with
	// two files that agree on a path the package does not install to.
	if got := execStart(string(staged)); got != "ExecStart=/usr/bin/mdn serve" {
		t.Errorf("the packaged unit runs %q, want ExecStart=/usr/bin/mdn serve — the path `make install` and this package both use", got)
	}
}

// TestTheStagedDocumentsCarryTheVersion. The changelog and the manual page are
// generated, and a generated file with a placeholder still in it is the kind
// of thing that ships.
func TestTheStagedDocumentsCarryTheVersion(t *testing.T) {
	invocations, stderr, err := runBuild(t, "v0.1.0", map[string]string{"amd64": "a", "arm64": "b"})
	if err != nil {
		t.Fatalf("build.sh failed: %v\n%s", err, stderr)
	}
	staging := invocations[0].staging

	changelog := gunzip(t, filepath.Join(staging, "changelog.gz"))
	// Policy 12.7's shape: dpkg-parsechangelog and lintian both read it.
	if !strings.HasPrefix(changelog, "md-notes (0.1.0) unstable; urgency=") {
		t.Errorf("the changelog does not open with a Debian entry for this version:\n%s", changelog)
	}
	if !strings.Contains(changelog, "releases/tag/v0.1.0") {
		t.Error("the changelog entry does not point at the release's own notes, which is where this project's changelog is")
	}
	if !strings.Contains(changelog, "\n -- ") {
		t.Errorf("the changelog has no trailer line naming the maintainer and the date:\n%s", changelog)
	}

	manpage := gunzip(t, filepath.Join(staging, "mdn.1.gz"))
	if strings.Contains(manpage, "@VERSION@") || strings.Contains(manpage, "@DATE@") {
		t.Error("the manual page still carries a placeholder")
	}
	if !strings.Contains(manpage, "0.1.0") {
		t.Error("the manual page does not name the version it shipped with")
	}
	// lintian's whatis check: the NAME section has to read `name \- summary`.
	if !strings.Contains(manpage, `mdn \- `) {
		t.Errorf("the manual page has no `mdn \\- summary` NAME entry, which is what apropos(1) reads")
	}

	// The copyright file is the licence itself, not a description of it, under
	// the one line Policy 12.5 asks for that the licence text does not give:
	// where the sources it covers came from.
	licence, err := os.ReadFile(filepath.Join(repoRoot, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	copyright, err := os.ReadFile(filepath.Join(staging, "copyright"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(copyright, licence) {
		t.Errorf("/usr/share/doc/md-notes/copyright does not carry LICENSE verbatim:\n%s", copyright)
	}
	source, _, _ := bytes.Cut(copyright, []byte("\n"))
	if !bytes.HasPrefix(source, []byte("Source: https://")) {
		t.Errorf("the copyright file opens with %q, want a Source: line naming where the sources came from", source)
	}
	if homepage := packageConfig(t).Homepage; !bytes.Contains(source, []byte(homepage)) {
		t.Errorf("the copyright names %q and the control file's Homepage is %q; they are the same place", source, homepage)
	}
}

func gunzip(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// fakeRepo lays out just enough of this repository at root — the packaging
// directory as it stands, the licence, and a contrib/mdn.service of the
// caller's choosing — for build.sh to run against. build.sh finds the
// repository from its own location, so copying it is what makes a different
// unit file reachable.
func fakeRepo(t *testing.T, root, unit string) string {
	t.Helper()
	deb := filepath.Join(root, "packaging", "deb")
	if err := os.MkdirAll(deb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "contrib"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot, "packaging", "deb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		copyFile(t, filepath.Join(repoRoot, "packaging", "deb", entry.Name()), filepath.Join(deb, entry.Name()))
	}
	copyFile(t, filepath.Join(repoRoot, "LICENSE"), filepath.Join(root, "LICENSE"))
	if err := os.WriteFile(filepath.Join(root, "contrib", "mdn.service"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	content, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, content, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
}

// TestAUnitThePackageCannotStartIsRefused is the other half, from the
// repository's side: if contrib/mdn.service ever stops running the path this
// package installs to, the build stops rather than shipping a unit that
// cannot start.
//
// This is the regression the gate on davison/md-notes#137 was raised over,
// standing the other way round. Under option (a) the packaging rewrote the
// ExecStart, and the risk was that the rewrite would quietly drop part of the
// line; under option (b), which the operator chose, the packaging copies the
// file, and the risk is that the file drifts back to a home-directory path
// and the copy ships it. A `make install` that changed its default prefix
// again, or an edit to the unit's header instructions, is exactly how that
// would happen — and it would fail at `systemctl --user start mdn` on a
// stranger's machine, which is the worst place to find out.
func TestAUnitThePackageCannotStartIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, execStart string }{
		{"the home-directory path the unit used to carry", "ExecStart=%h/.local/bin/mdn serve"},
		{"another prefix entirely", "ExecStart=/usr/local/bin/mdn serve"},
		{"a path this one is a prefix of", "ExecStart=/usr/bin/mdn-wrapper serve"},
		{"no ExecStart at all", "Restart=on-failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unit := "[Unit]\nDescription=mdn\n\n[Service]\n" + tc.execStart + "\n"
			invocations, _, err := runBuildAgainstUnit(t, "v0.1.0",
				map[string]string{"amd64": "a", "arm64": "b"}, unit)
			if err == nil {
				t.Errorf("a unit running %q was packaged, want the build refused: this package installs the binary at /usr/bin/mdn and nothing else starts it", tc.execStart)
			}
			if len(invocations) != 0 {
				t.Errorf("the build was refused but still packaged %d times", len(invocations))
			}
		})
	}

	// What is guarded is the path, not the whole line: arguments after the
	// binary are the unit's own business and this package starts them fine.
	// Refusing them would turn a flag added to contrib/mdn.service into a red
	// build blaming the path, which is the wrong complaint about a working
	// unit.
	for _, execStart := range []string{
		"ExecStart=/usr/bin/mdn serve",
		"ExecStart=/usr/bin/mdn serve --port 7337",
		"ExecStart=/usr/bin/mdn serve --port 7337 --root %h/notes",
	} {
		unit := "[Unit]\nDescription=mdn\n\n[Service]\n" + execStart + "\n"
		if _, stderr, err := runBuildAgainstUnit(t, "v0.1.0",
			map[string]string{"amd64": "a", "arm64": "b"}, unit); err != nil {
			t.Errorf("a unit running %q was refused: %v\n%s", execStart, err, stderr)
		}
	}

	// And the unit as the repository holds it goes through.
	unit, err := os.ReadFile(filepath.Join(repoRoot, "contrib", "mdn.service"))
	if err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runBuildAgainstUnit(t, "v0.1.0",
		map[string]string{"amd64": "a", "arm64": "b"}, string(unit)); err != nil {
		t.Fatalf("contrib/mdn.service itself was refused: %v\n%s", err, stderr)
	}
}

// execStart is the unit's ExecStart line, without its newline.
func execStart(unit string) string {
	for _, line := range strings.Split(unit, "\n") {
		if strings.HasPrefix(line, "ExecStart=") {
			return line
		}
	}
	return ""
}

// TestTheInstallPrefixAndTheUnitAgree is the one string pair the gate
// resolution on davison/md-notes#137 rests on.
//
// The operator resolved that gate by making `make install` install
// system-wide, so that the from-source route and the package put the binary in
// the same place and contrib/mdn.service can name one path. Everything else
// follows from those two strings agreeing: the unit the package ships, the
// unit a from-source install copies, and the instruction in the unit's own
// header are all correct only while they do.
//
// The package's half is pinned three ways over in build.sh and the tests
// around it. The from-source half was pinned by nothing at all — reverting
// PREFIX to $(HOME)/.local left `make check` green, which is how the drift the
// gate was raised about would come back. This reads both files and asserts the
// pair, so a change to either side without the other fails here rather than at
// `systemctl --user start mdn` on a stranger's machine.
func TestTheInstallPrefixAndTheUnitAgree(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	prefix := ""
	for _, line := range strings.Split(string(makefile), "\n") {
		if rest, ok := strings.CutPrefix(line, "PREFIX ?="); ok {
			prefix = strings.TrimSpace(rest)
		}
	}
	if prefix == "" {
		t.Fatal("the Makefile declares no `PREFIX ?=` default; `make install` and contrib/mdn.service have to agree on one, and this is half of it")
	}

	unit, err := os.ReadFile(filepath.Join(repoRoot, "contrib", "mdn.service"))
	if err != nil {
		t.Fatal(err)
	}
	line := execStart(string(unit))
	if line == "" {
		t.Fatal("contrib/mdn.service has no ExecStart= line")
	}
	// The path, without whatever arguments follow it.
	path, _, _ := strings.Cut(strings.TrimPrefix(line, "ExecStart="), " ")

	// `make install` runs `install -Dm755 mdn $(PREFIX)/bin/mdn`.
	if want := prefix + "/bin/mdn"; path != want {
		t.Errorf("contrib/mdn.service runs %q but `make install` puts the binary at %q (PREFIX ?= %s).\n"+
			"These are the two strings the gate resolution on davison/md-notes#137 made equal; whichever one moved, the other has to move with it — or a from-source install gets a unit that cannot start it.",
			path, want, prefix)
	}
}
