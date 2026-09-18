// Assertions over the Chrome Web Store channel workflow
// (davison/md-notes#135, M8-R2).
//
// The properties here are the ones whose loss nothing else would report. A
// workflow that has quietly acquired a second trigger still passes actionlint;
// a workflow reading `secrets.CHROME_WEBSTORE_TOKEN` when the operator stored
// `CHROME_WEBSTORE_REFRESH_TOKEN` still parses; a step that uploads a zip it
// built itself rather than the release's own asset is green every time and
// wrong every time. Each of those is a line of YAML, and each has a test below.

package workflows

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The shape this file reads. A type of its own rather than an extension of
// release_test.go's: the two workflows are asserted over different things, and
// the sibling channel workflows land in this package next.
type channelWorkflow struct {
	Name string `yaml:"name"`
	// yaml.v3 resolves a plain `on` as the string it is written as, not as
	// YAML 1.1's boolean, so the trigger block arrives here intact.
	On map[string]struct {
		Types []string `yaml:"types"`
	} `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Steps []channelStep `yaml:"steps"`
	} `yaml:"jobs"`
}

type channelStep struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

const webstoreWorkflowFile = "publish-webstore.yml"

func webstoreRaw(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", webstoreWorkflowFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func webstore(t *testing.T) channelWorkflow {
	t.Helper()
	var parsed channelWorkflow
	if err := yaml.Unmarshal([]byte(webstoreRaw(t)), &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

// TestWebStoreRunsOnlyOnAPublishedRelease is the whole of the workflow's
// authority to act.
//
// The credentials this job holds can replace what every user of the extension
// has installed, so the question "what can start it" has exactly one answer: a
// Release a person published. Not `created`, which fires for a draft as well;
// not `released`, which does not fire for a pre-release; and not a
// workflow_dispatch, which would be a second way in carrying no release with
// it — a channel that fails is re-run with GitHub's own re-run, which replays
// this event.
//
// The workflow's permissions are asserted here too: it reads a release asset
// and writes nothing to the repository, so a compromise of this job is a
// compromise of the store credentials and not of the repository as well.
func TestWebStoreRunsOnlyOnAPublishedRelease(t *testing.T) {
	parsed := webstore(t)

	if len(parsed.On) != 1 {
		t.Fatalf("%s has %d triggers (%v), want exactly one: release", webstoreWorkflowFile, len(parsed.On), keys(parsed.On))
	}
	release, ok := parsed.On["release"]
	if !ok {
		t.Fatalf("%s is triggered by %v, want release", webstoreWorkflowFile, keys(parsed.On))
	}
	if len(release.Types) != 1 || release.Types[0] != "published" {
		t.Errorf("%s runs on release types %v, want exactly [published]: `created` fires for a draft too, and `released` never fires for a pre-release", webstoreWorkflowFile, release.Types)
	}

	if got := parsed.Permissions["contents"]; got != "read" {
		t.Errorf("%s asks for contents: %q, want read: it downloads a release asset and writes nothing", webstoreWorkflowFile, got)
	}
	for scope, level := range parsed.Permissions {
		if level == "write" {
			t.Errorf("%s asks for %s: write; the only thing this job may write is the store listing", webstoreWorkflowFile, scope)
		}
	}

	if parsed.Concurrency.Group == "" {
		t.Error(webstoreWorkflowFile + " declares no concurrency group; two releases published together would upload to one item at once")
	}
	if parsed.Concurrency.CancelInProgress {
		t.Error(webstoreWorkflowFile + " cancels an upload in progress; a half-sent package is worse than a queued one")
	}
}

// TestWebStoreReadsTheDocumentedNames binds the workflow to the four names the
// operator was asked to create, on the task issue and in docs/extension.md.
//
// A renamed secret does not fail anything: the expression resolves to an empty
// string and the job fails inside somebody else's API, at the one moment
// nobody can reproduce it locally. The kinds matter as much as the names — the
// item id is a *variable* on purpose, because Actions masks a secret's value
// everywhere it appears, including the line that would say which item was
// uploaded to.
func TestWebStoreReadsTheDocumentedNames(t *testing.T) {
	raw := webstoreRaw(t)

	for _, name := range []string{
		"CHROME_WEBSTORE_CLIENT_ID",
		"CHROME_WEBSTORE_CLIENT_SECRET",
		"CHROME_WEBSTORE_REFRESH_TOKEN",
	} {
		if !strings.Contains(raw, "secrets."+name) {
			t.Errorf("%s never reads secrets.%s; that is the name the operator was asked to store it under (davison/md-notes#135)", webstoreWorkflowFile, name)
		}
	}
	// The two identifiers are variables rather than secrets, deliberately:
	// neither grants anything without the three secrets above, and Actions
	// masks a secret's value everywhere it appears — including the line that
	// would say which publisher and item were uploaded to.
	for _, name := range []string{"CHROME_WEBSTORE_PUBLISHER_ID", "CHROME_WEBSTORE_ITEM_ID"} {
		if !strings.Contains(raw, "vars."+name) {
			t.Errorf("%s never reads vars.%s", webstoreWorkflowFile, name)
		}
		if strings.Contains(raw, "secrets."+name) {
			t.Errorf("%s reads %s as a secret; it is an identifier rather than a credential, and a secret is masked out of the log line that would name it", webstoreWorkflowFile, name)
		}
	}

	// And the preflight names all four, so a missing one fails in ten seconds
	// saying which rather than after a download.
	preflight := webstoreStep(t, "Check the store credentials are configured")
	for _, name := range []string{
		"CHROME_WEBSTORE_CLIENT_ID",
		"CHROME_WEBSTORE_CLIENT_SECRET",
		"CHROME_WEBSTORE_REFRESH_TOKEN",
		"CHROME_WEBSTORE_PUBLISHER_ID",
		"CHROME_WEBSTORE_ITEM_ID",
	} {
		if _, ok := preflight.Env[name]; !ok {
			t.Errorf("the credential preflight does not bind %s, so it cannot report it missing", name)
		}
	}
}

// TestWebStoreUploadsTheReleasesOwnAsset runs the workflow's resolve step, the
// script as written, against the asset name docs/releasing.md publishes.
//
// Two things could go wrong here and neither would ever fail a run. The asset
// name could drift from the release workflow's — the version in it is the tag
// verbatim, leading `v` and all, which is the one place in this repository
// where the `v` is kept — and the download would 404 on a release that is
// perfectly fine. Or the store could be sent a zip built in this job rather
// than the release's own asset: identical most days, and on the day it is not,
// the store holds a package that no checksum on the release page covers.
func TestWebStoreUploadsTheReleasesOwnAsset(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is the shell a `run:` step gets on the runner")
	}

	script := webstoreStep(t, "Resolve the release").Run
	if strings.Contains(script, "${{") {
		t.Fatal("the resolve step now interpolates a workflow expression, so it cannot be run as written: keep its inputs in `env:`")
	}

	resolve := func(t *testing.T, tag string) (string, error) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "resolve.sh")
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(dir, "step-output")
		if err := os.WriteFile(output, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bash, "-e", "-o", "pipefail", path)
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"RELEASE_TAG=" + tag,
			"GITHUB_OUTPUT=" + output,
		}
		err := cmd.Run()
		written, readErr := os.ReadFile(output)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(written), err
	}

	t.Run("names the release's asset, with the tag's v kept", func(t *testing.T) {
		written, err := resolve(t, "v0.1.0")
		if err != nil {
			t.Fatalf("v0.1.0 was refused: %v", err)
		}
		want := "tag=v0.1.0\nasset=mdn-extension-v0.1.0.zip\nversion=0.1.0\n"
		if written != want {
			t.Errorf("resolved to %q, want %q", written, want)
		}

		// The same spelling docs/releasing.md's asset table publishes. A
		// channel workflow that invents its own name is a 404 on a release
		// that is otherwise perfect.
		doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "releasing.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(doc), "mdn-extension-v0.1.0.zip") {
			t.Error("docs/releasing.md no longer spells the extension asset mdn-extension-v0.1.0.zip; the workflow and the record have to agree on it")
		}
	})

	t.Run("refuses anything that is not a release tag, and writes nothing", func(t *testing.T) {
		for _, tag := range []string{
			"v0.1.0-rc1",          // a pre-release the manifest cannot carry
			"v1.2",                // too few components
			"0.1.0",               // the v is part of the shape
			"v1.2.3 ",             // trailing whitespace
			"v1.2.3\nmalicious=1", // a second $GITHUB_OUTPUT line
			"",                    // a release with no tag at all
		} {
			written, err := resolve(t, tag)
			if err == nil {
				t.Errorf("tag %q was accepted, want it refused", tag)
			}
			if written != "" {
				t.Errorf("tag %q was refused but wrote %q to the step output", tag, written)
			}
		}
	})

	t.Run("sends the release's asset and does not build its own", func(t *testing.T) {
		download := webstoreStep(t, "Download the release's extension zip").Run
		if !strings.Contains(download, "gh release download") || !strings.Contains(download, `--pattern "$ASSET"`) {
			t.Errorf("the download step does not fetch the release's asset by name:\n%s", download)
		}

		upload := webstoreStep(t, "Upload to the Chrome Web Store and submit for review").Run
		if !strings.Contains(upload, "webstore.mjs") || !strings.Contains(upload, `--zip "$ASSET"`) {
			t.Errorf("the upload step does not send the downloaded asset:\n%s", upload)
		}

		raw := webstoreRaw(t)
		for _, build := range []string{"make release", "make extension", "pnpm --dir extension build", "run zip"} {
			if strings.Contains(raw, build) {
				t.Errorf("%s builds the extension itself (%q); the store must get the release's own asset, which the release page's checksums cover", webstoreWorkflowFile, build)
			}
		}
	})
}

// TestWebStoreVerifiesWhatItSends holds the two checks that stand between "the
// right file was downloaded" and "the right file was sent".
//
// `gh release download` verifies nothing. Provenance is not verification, and
// three places in this repository claim the store gets the artefact the release
// page's checksums cover — the workflow's own header, the PR that added it, and
// a comment in this file. A truncated or replaced asset reaches the store as a
// package it rejects for reasons that look like anything but this, so the claim
// has to be made true by a step rather than by a sentence.
//
// The version check is the other half: the store refuses a package whose
// version is not higher than the published one, so a zip carrying 0.0.0 — what
// an untagged build stamps, per docs/releasing.md — fails inside somebody
// else's API rather than here, saying something about versions rather than that
// the asset is not this release's.
func TestWebStoreVerifiesWhatItSends(t *testing.T) {
	checksum := webstoreStep(t, "Check the zip against the release's checksums").Run
	if !strings.Contains(checksum, "sha256sum") {
		t.Errorf("the checksum step runs no sha256sum:\n%s", checksum)
	}
	// `sha256sum --check` exits 0 having verified nothing when the file it was
	// given is not named in the sums, so the exit status alone is not the
	// assertion: the asset's own OK line has to be.
	if !strings.Contains(checksum, `"$ASSET: OK"`) {
		t.Errorf("the checksum step does not assert the asset's own OK line, so it passes when nothing was verified:\n%s", checksum)
	}

	download := webstoreStep(t, "Download the release's extension zip").Run
	if !strings.Contains(download, "--pattern SHA256SUMS") {
		t.Errorf("the download step never fetches SHA256SUMS, so there is nothing to check against:\n%s", download)
	}

	version := webstoreStep(t, "Check the package carries this release's version").Run
	if !strings.Contains(version, `"$VERSION"`) {
		t.Errorf("the version step does not compare against the release's version; printing it is not checking it:\n%s", version)
	}
	if !strings.Contains(version, "exit 1") {
		t.Errorf("the version step cannot fail:\n%s", version)
	}
}

// TestTheChecksumStepSaysWhatFailed runs the checksum step as written, because
// asserting on its text cannot tell whether its message is reachable.
//
// It was not. A `run:` step is `bash -e`, so `checked=$(sha256sum …)` on a
// failing sum killed the step at the assignment and the crafted message — the
// one naming the file that was not verified — never printed. The step failed
// for the right reason with the wrong words, which is the kind of thing only a
// person reading a real failing run would ever notice.
func TestTheChecksumStepSaysWhatFailed(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is the shell a `run:` step gets on the runner")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("sha256sum is coreutils, which the runner has")
	}

	script := webstoreStep(t, "Check the zip against the release's checksums").Run
	if strings.Contains(script, "${{") {
		t.Fatal("the checksum step now interpolates a workflow expression, so it cannot be run as written: keep its inputs in `env:`")
	}

	const asset = "mdn-extension-v0.1.0.zip"
	// Runs the step over a directory holding `asset` and a SHA256SUMS built
	// from `sums`, and hands back everything it said.
	check := func(t *testing.T, contents string, sums func(realDigest string) string) (string, error) {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, asset), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(contents)))
		if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums(digest)), 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "check.sh")
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bash, "-e", "-o", "pipefail", path)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "ASSET=" + asset}
		said, err := cmd.CombinedOutput()
		return string(said), err
	}

	t.Run("passes the release's own asset", func(t *testing.T) {
		said, err := check(t, "the package", func(d string) string {
			return d + "  " + asset + "\nffff  mdn-v0.1.0-linux-amd64\n"
		})
		if err != nil {
			t.Errorf("the release's own asset was refused: %v\n%s", err, said)
		}
		if !strings.Contains(said, asset+": OK") {
			t.Errorf("the step did not report the asset verified:\n%s", said)
		}
	})

	for _, tc := range []struct {
		name string
		sums func(string) string
	}{
		{
			// The case that killed the step at the assignment.
			name: "an asset that is not the one the release was built from",
			sums: func(string) string {
				return strings.Repeat("0", 64) + "  " + asset + "\n"
			},
		},
		{
			// --check exits 0 here having verified nothing at all, so only the
			// OK-line assertion catches it.
			name: "an asset the checksums do not cover",
			sums: func(string) string { return "ffff  mdn-v0.1.0-linux-amd64\n" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			said, err := check(t, "the package", tc.sums)
			if err == nil {
				t.Errorf("the step passed:\n%s", said)
			}
			if !strings.Contains(said, "SHA256SUMS did not verify "+asset) {
				t.Errorf("the step failed without naming what was not verified, so its message is unreachable:\n%s", said)
			}
		})
	}
}

// TestWebStoreUsesTheV2Api keeps the workflow's call in step with the client.
//
// The V2 API addresses an item as `publishers/<publisherId>/items/<itemId>`,
// which V1 did not: a workflow that forgets the publisher id calls a client
// that refuses before the network, which is the good failure, but only if the
// argument is there to forget. V1 stops being answered on 15 October 2026.
func TestWebStoreUsesTheV2Api(t *testing.T) {
	upload := webstoreStep(t, "Upload to the Chrome Web Store and submit for review").Run
	for _, argument := range []string{"--publisher-id", "--item-id", "--zip", "--expect-version"} {
		if !strings.Contains(upload, argument) {
			t.Errorf("the upload step does not pass %s:\n%s", argument, upload)
		}
	}

	client, err := os.ReadFile(filepath.Join("..", "..", "extension", "scripts", "webstore.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(client), `"https://chromewebstore.googleapis.com"`) {
		t.Error("extension/scripts/webstore.mjs does not name the V2 service host")
	}
	// A V1 *endpoint*, not a mention of one: the file's header explains the
	// deprecation and names V1 to do it, and a needle that cannot tell the two
	// apart fails on the sentence that exists to prevent the thing it checks.
	// The scope, `www.googleapis.com/auth/chromewebstore`, is V2's too.
	if strings.Contains(string(client), `"https://www.googleapis.com/chromewebstore`) {
		t.Error("extension/scripts/webstore.mjs still calls a V1 endpoint, which is answered only until 15 October 2026")
	}
}

// The named step of the one job, or a failure naming what is there instead.
func webstoreStep(t *testing.T, name string) channelStep {
	t.Helper()
	job, ok := webstore(t).Jobs["publish"]
	if !ok {
		t.Fatal(webstoreWorkflowFile + " has no `publish` job")
	}
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	var names []string
	for _, step := range job.Steps {
		names = append(names, step.Name)
	}
	t.Fatalf("%s has no step %q; its steps are %v", webstoreWorkflowFile, name, names)
	panic("unreachable")
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
