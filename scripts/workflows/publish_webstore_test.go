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
	if !strings.Contains(raw, "vars.CHROME_WEBSTORE_ITEM_ID") {
		t.Errorf("%s never reads vars.CHROME_WEBSTORE_ITEM_ID", webstoreWorkflowFile)
	}
	if strings.Contains(raw, "secrets.CHROME_WEBSTORE_ITEM_ID") {
		t.Errorf("%s reads the item id as a secret; it is public, and a secret is masked out of the log line that would name it", webstoreWorkflowFile)
	}

	// And the preflight names all four, so a missing one fails in ten seconds
	// saying which rather than after a download.
	preflight := webstoreStep(t, "Check the store credentials are configured")
	for _, name := range []string{
		"CHROME_WEBSTORE_CLIENT_ID",
		"CHROME_WEBSTORE_CLIENT_SECRET",
		"CHROME_WEBSTORE_REFRESH_TOKEN",
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
		want := "tag=v0.1.0\nasset=mdn-extension-v0.1.0.zip\n"
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
