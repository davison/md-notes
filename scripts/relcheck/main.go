// Command relcheck refuses a release whose version is not the same in all
// three places a release carries one: the tag the build was told to build, the
// binary's own `mdn version`, and the extension manifest the build stamped.
//
// The three derive from one source — the Makefile's VERSION — so they can only
// disagree if something in the chain stopped working: an ldflags typo, a build
// that skipped the manifest stamp, a stale dist/ left over from another
// version. Each of those publishes a release that lies about what it is, and
// each is silent. `make release` runs this before it writes SHA256SUMS, so the
// release fails instead.
//
// Run by hand against a built dist/:
//
//	go run ./scripts/relcheck -version v0.1.0 \
//	    -binary dist/mdn-v0.1.0-linux-amd64 \
//	    -manifest dist/mdn-extension-v0.1.0.zip
package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("relcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	version := flags.String("version", "", "the version being released, as the Makefile's VERSION holds it")
	binary := flags.String("binary", "", "path to a built mdn binary, asked for its version")
	manifest := flags.String("manifest", "", "the built extension: either the release zip or a manifest.json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	for name, value := range map[string]string{"version": *version, "binary": *binary, "manifest": *manifest} {
		if value == "" {
			fmt.Fprintf(stderr, "relcheck: -%s is required\n", name)
			return 2
		}
	}

	reported, err := binaryVersion(*binary)
	if err != nil {
		fmt.Fprintf(stderr, "relcheck: %v\n", err)
		return 1
	}
	stamped, err := manifestVersionOf(*manifest)
	if err != nil {
		fmt.Fprintf(stderr, "relcheck: %v\n", err)
		return 1
	}
	if err := check(*version, reported, stamped); err != nil {
		fmt.Fprintf(stderr, "relcheck: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "relcheck: %s — %s reports %s, the manifest carries %s\n", *version, *binary, reported, stamped)
	return 0
}

// check reports whether the version the release is being built as, the version
// the binary prints, and the version the manifest carries are the same version.
//
// The binary carries VERSION verbatim, so it must match exactly. The manifest
// cannot: the Chrome Web Store takes only dotted integers, so it carries the
// normalised form, and it is compared against the normalised version.
func check(version, reported, stamped string) error {
	if reported != version {
		return fmt.Errorf("the binary reports %q, but the release is being built as %q — check the -X main.version ldflag", reported, version)
	}
	if want := manifestVersion(version); stamped != want {
		return fmt.Errorf("the manifest carries %q, but %q makes it %q — the extension build did not stamp this version", stamped, version, want)
	}
	return nil
}

// binaryVersion is what the built binary says it is.
func binaryVersion(path string) (string, error) {
	out, err := exec.Command(path, "version").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("%s version failed: %w: %s", path, err, strings.TrimSpace(string(exit.Stderr)))
		}
		// Running the binary is the only way to know what it was built with,
		// so a release is built on a host that can run one of its own targets.
		return "", fmt.Errorf("%s version could not be run: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// manifestVersionOf is the version key of a built extension manifest, read
// from the release zip itself when it is given one.
//
// The zip is the thing that ships. Checking the manifest staged in
// extension/dist/ instead would be checking a file that happens to agree with
// the asset today, because the zip is packed from it two lines earlier in the
// release target — an ordering nothing here enforces and a later edit could
// quietly break, which is the same silent staleness this command exists to
// catch.
func manifestVersionOf(path string) (string, error) {
	read := os.ReadFile
	if strings.HasSuffix(path, ".zip") {
		read = manifestInZip
	}
	raw, err := read(path)
	if err != nil {
		return "", fmt.Errorf("the extension manifest could not be read: %w", err)
	}
	var manifest struct {
		Version *string `json:"version"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if manifest.Version == nil {
		return "", fmt.Errorf("%s carries no version: this is the built manifest, so the build's stamping step did not run", path)
	}
	return *manifest.Version, nil
}

// manifestInZip is the manifest.json entry of a packed extension.
func manifestInZip(path string) ([]byte, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer archive.Close()
	entry, err := archive.Open("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("%s holds no manifest.json: %w", path, err)
	}
	defer entry.Close()
	return io.ReadAll(entry)
}

// release matches one to four dot-separated integers with no leading zeros,
// after an optional leading v.
var release = regexp.MustCompile(`^v?(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*)){0,3}$`)

// devVersion is what a build that is not a release tag calls itself.
const devVersion = "0.0.0"

// maxComponent is the largest value a Chrome manifest version component holds.
const maxComponent = 65535

// manifestVersion mirrors extension/scripts/version.mjs: the manifest version
// a build of raw carries. The two are kept in step by this command, which
// fails the release the moment they part company.
func manifestVersion(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !release.MatchString(trimmed) {
		return devVersion
	}
	version := strings.TrimPrefix(trimmed, "v")
	for _, part := range strings.Split(version, ".") {
		if n, err := strconv.Atoi(part); err != nil || n > maxComponent {
			return devVersion
		}
	}
	return version
}
