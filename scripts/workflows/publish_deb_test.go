// Assertions over the Debian channel's workflow (davison/md-notes#137).
//
// The file is short and every line of it is load-bearing in a way nothing
// would notice the loss of until a release went out wrong: a second trigger
// would build a package for something that is not a release, a checksum check
// moved one step later would package bytes it had not verified, and an upload
// to a release of its own would leave the .deb somewhere nobody is looking.
// None of those is a syntax error, so actionlint has nothing to say about any
// of them.
package workflows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type debStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

type debWorkflow struct {
	Name string `yaml:"name"`
	// A map rather than named fields: `workflow_dispatch:` with nothing after
	// it is a null value, which decodes into a nil pointer and reads exactly
	// like a trigger that is not there. The key is the fact; the value is not.
	On          map[string]yaml.Node `yaml:"on"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Permissions map[string]string `yaml:"permissions"`
		Steps       []debStep         `yaml:"steps"`
	} `yaml:"jobs"`
}

func publishDeb(t *testing.T) debWorkflow {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish-deb.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed debWorkflow
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

// steps is the one job's steps, and the index of each, so a test can say that
// one thing happens before another rather than only that both happen.
func debSteps(t *testing.T) []debStep {
	t.Helper()
	jobs := publishDeb(t).Jobs
	if len(jobs) != 1 {
		t.Fatalf("publish-deb.yml has %d jobs, want 1", len(jobs))
	}
	for name, job := range jobs {
		if job.Permissions["contents"] != "write" {
			t.Errorf("the %q job has contents: %q, want write: it uploads the packages to the release",
				name, job.Permissions["contents"])
		}
		if len(job.Steps) == 0 {
			t.Fatalf("the %q job has no steps", name)
		}
		return job.Steps
	}
	return nil
}

// at is the index of the first step whose run script contains substr, or -1.
func at(steps []debStep, substr string) int {
	for i, step := range steps {
		if strings.Contains(step.Run, substr) {
			return i
		}
	}
	return -1
}

// TestOnlyAPublishedReleaseBuildsADeb is the trigger, and the gate resolved on
// davison/md-notes#133 is why it is the only one.
//
// The operator publishes the draft by hand and that fires `release:
// published`; a re-run of a failed channel happens from the Actions page,
// against the run's own release, and needs no trigger. Anything else here
// would be a way to build a .deb — under whatever version was typed, from
// whatever assets happened to be lying around — for something that is not a
// release the operator published, and then upload it to a release page as if
// it were.
func TestOnlyAPublishedReleaseBuildsADeb(t *testing.T) {
	on := publishDeb(t).On

	for name := range on {
		if name != "release" {
			t.Errorf("publish-deb.yml also triggers on %q: the channel runs when the operator publishes a release, and on nothing else", name)
		}
	}

	release, ok := on["release"]
	if !ok {
		t.Fatal("publish-deb.yml does not trigger on a release at all")
	}
	var types struct {
		Types []string `yaml:"types"`
	}
	if err := release.Decode(&types); err != nil {
		t.Fatalf("the release trigger does not parse: %v", err)
	}
	if got := types.Types; len(got) != 1 || got[0] != "published" {
		t.Errorf("release types are %v, want exactly [published]: `created` and `released` fire for a draft and for an edit", got)
	}
}

// TestTheDebComesFromTheReleasesOwnAssets holds the shape of the job: the
// binaries are the release's, they are downloaded under the names
// docs/releasing.md publishes them by, and their checksums are verified before
// anything packages them.
func TestTheDebComesFromTheReleasesOwnAssets(t *testing.T) {
	steps := debSteps(t)

	// The three assets by name, tag and all. docs/releasing.md is what fixes
	// these; a rename there and a rename here have to happen together.
	download := at(steps, "gh release download")
	if download < 0 {
		t.Fatal("no step downloads the release's assets: the packages must be built from what the release published, not from a fresh build")
	}
	for _, pattern := range []string{
		`mdn-$TAG-linux-amd64`,
		`mdn-$TAG-linux-arm64`,
		`SHA256SUMS`,
	} {
		if !strings.Contains(steps[download].Run, pattern) {
			t.Errorf("the download step does not ask for %q; docs/releasing.md is what names the release's assets", pattern)
		}
	}

	// Both, and in one step: `sha256sum -c` on its own also describes the
	// digest check on the nfpm download, which runs before the build too and
	// would satisfy an ordering assertion while the release's own binaries
	// went unchecked.
	verify := -1
	for i, step := range steps {
		if strings.Contains(step.Run, "sha256sum -c") && strings.Contains(step.Run, "SHA256SUMS") {
			verify = i
			break
		}
	}
	if verify < 0 {
		t.Fatal("no step checks the downloaded binaries against the release's SHA256SUMS")
	}
	build := at(steps, "packaging/deb/build.sh")
	if build < 0 {
		t.Fatal("no step runs packaging/deb/build.sh")
	}

	if !(download < verify && verify < build) {
		t.Errorf("the steps run download=%d, verify=%d, build=%d; the checksums have to be checked after the download and before the packaging, or the package is made of bytes nothing vouched for",
			download, verify, build)
	}

	lintian := at(steps, "lintian ")
	if lintian < 0 {
		t.Fatal("no step runs lintian: M8-R4 asks for a package that passes it")
	}
	if lintian < build {
		t.Errorf("lintian runs at step %d, before the build at %d", lintian, build)
	}

	upload := at(steps, "gh release upload")
	if upload < 0 {
		t.Fatal("no step uploads the packages with `gh release upload`")
	}
	if upload < lintian {
		t.Errorf("the upload runs at step %d, before lintian at %d: a package nobody checked would reach the release page", upload, lintian)
	}
	if at(steps, "gh release create") >= 0 {
		t.Error("publish-deb.yml creates a Release; it uploads to the one that was published")
	}
	if !strings.Contains(steps[upload].Run, `"$TAG"`) {
		t.Errorf("the upload step is %q; it must upload to the release's own tag", steps[upload].Run)
	}

	// The tagged tree, not the default branch: contrib/mdn.service, LICENSE
	// and packaging/deb/ have to be the ones the release was cut from.
	var checkout *debStep
	for i := range steps {
		if strings.HasPrefix(steps[i].Uses, "actions/checkout@") {
			checkout = &steps[i]
			break
		}
	}
	if checkout == nil {
		t.Fatal("publish-deb.yml never checks the repository out, but the package carries files from it")
	}
	if !strings.Contains(checkout.With["ref"], "release.tag_name") {
		t.Errorf("the checkout takes ref %q, want the release's tag: main may have moved on since the tag was cut", checkout.With["ref"])
	}
}

// TestTheNfpmDownloadIsPinnedAndChecked. nfpm is the program that assembles
// the package a stranger installs as root, fetched over the network at run
// time. A floating version would change what it produces without anything in
// this repository changing; an unchecked download would let whatever answered
// the request produce it.
func TestTheNfpmDownloadIsPinnedAndChecked(t *testing.T) {
	steps := debSteps(t)
	fetch := at(steps, "goreleaser/nfpm/releases/download")
	if fetch < 0 {
		t.Fatal("no step fetches nfpm")
	}
	if strings.Contains(steps[fetch].Run, "latest") {
		t.Error("the nfpm download asks for `latest`: pin a version, so the package is produced by a packager this repository names")
	}
	if !strings.Contains(steps[fetch].Run, "sha256sum -c") {
		t.Error("the nfpm download is not checked against a digest")
	}
}

// TestOnlyAReleaseTagResolvesForTheDeb runs the workflow's own tag step, the
// script as written, the way release_test.go runs release.yml's.
//
// A Release can be published for any tag at all — the tag is typed by a
// person and this workflow is started by the event, not by the tag pattern
// that guards release.yml. The tag reaches an asset name, a package version
// and a file name, so it is checked here against the same anchored shape
// release.yml builds under.
func TestOnlyAReleaseTagResolvesForTheDeb(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is the shell a `run:` step gets on the runner")
	}

	var script string
	for _, step := range debSteps(t) {
		if step.Name == "Resolve the release tag" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal(`publish-deb.yml has no "Resolve the release tag" step`)
	}
	if strings.Contains(script, "${{") {
		t.Fatal("the step now interpolates a workflow expression, so it cannot be run as written: keep its inputs in `env:`")
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
			"TAG_NAME=" + tag,
			"GITHUB_OUTPUT=" + output,
		}
		err := cmd.Run()
		written, readErr := os.ReadFile(output)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(written), err
	}

	t.Run("accepts a release tag", func(t *testing.T) {
		for _, tag := range []string{"v0.1.0", "v1.20.300"} {
			written, err := resolve(t, tag)
			if err != nil {
				t.Errorf("tag %q: %v, want it accepted", tag, err)
			}
			if want := "tag=" + tag + "\n"; written != want {
				t.Errorf("tag %q wrote %q, want %q", tag, written, want)
			}
		}
	})

	t.Run("refuses anything else, and writes nothing", func(t *testing.T) {
		for _, tag := range []string{
			"v0.1.0-rc1",          // a prerelease: a hyphen makes the package non-native
			"0.1.0",               // the v is part of the shape
			"v1.2",                // too few components
			"nightly",             // a Release can be published for any tag at all
			"v0.1.0 --clobber",    // an argument smuggled in beside the tag
			"v1.2.3 ",             // trailing whitespace
			"v1.2.3\nmalicious=1", // a second $GITHUB_OUTPUT line
			"",
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
}
