// Package workflows holds assertions over this repository's GitHub Actions
// workflows: the few properties that the written record depends on and that
// nothing else would notice the loss of.
//
// actionlint checks that a workflow is well formed. It cannot check that this
// one does what the task issue says it does, and the difference between
// publishing on a tag push and publishing on anything that reaches the job is
// one word in one `if`.
package workflows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	Name        string `yaml:"name"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Steps []struct {
			Name string `yaml:"name"`
			If   string `yaml:"if"`
			Run  string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func release(t *testing.T) workflow {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed workflow
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

// TestOnlyATagPushPublishes holds two claims the record makes.
//
// That a workflow_dispatch of the release workflow is always a dry run — said
// by the dispatch input's description, by the run summary, and by the decision
// recorded on davison/md-notes#134. A dispatch may name a tag as its ref, so a
// guard on github.ref does not say this: dispatched at a tag it would create a
// Release under the dispatch's input version, which need not be the tag the
// workflow ran at. Only the event distinguishes the two.
//
// And that what a tag push creates is a draft, for the operator to read and
// publish by hand — the gate resolved on davison/md-notes#133. A Release
// published by this workflow's token starts no workflow run, so it would never
// reach the channel workflows waiting on `release: published`; dropping
// --draft would leave every channel silently unstarted, which is the failure
// that gate was raised over.
func TestOnlyATagPushPublishes(t *testing.T) {
	steps := release(t).Jobs["release"].Steps
	if len(steps) == 0 {
		t.Fatal("release.yml has no release job with steps")
	}

	publishes := 0
	for _, step := range steps {
		if !strings.Contains(step.Run, "gh release create") {
			continue
		}
		publishes++
		if step.If != "github.event_name == 'push'" {
			t.Errorf("the step %q is guarded by %q, want %q: a dispatch can run at a tag, so only the event keeps a dry run dry",
				step.Name, step.If, "github.event_name == 'push'")
		}
		if !strings.Contains(step.Run, "--draft") {
			t.Errorf("the step %q creates a Release without --draft: a Release published by this workflow's token fires no release event, so no channel workflow would ever start (davison/md-notes#133)", step.Name)
		}
	}
	if publishes != 1 {
		t.Fatalf("found %d steps creating a Release, want exactly 1", publishes)
	}
}

// TestReleasesDoNotOverlap keeps two tags pushed together, or a tag re-run
// while its first run is still going, from building the same release twice at
// once — and keeps the remedy from being cancellation, which would abandon a
// release part-published.
func TestReleasesDoNotOverlap(t *testing.T) {
	parsed := release(t)
	if parsed.Concurrency.Group == "" {
		t.Error("release.yml declares no concurrency group")
	}
	if parsed.Concurrency.CancelInProgress {
		t.Error("release.yml cancels a release in progress; a queued release is better than a half-finished one")
	}
}

// TestOnlyAReleaseTagResolves runs the workflow's own version step, the script
// as written, rather than asserting something about its text.
//
// It is the one value in the release workflow a person types, and two earlier
// forms of this check both let something through — a `case` pattern that
// accepted `v0.1.0-rc1`, then a `grep -E` whose anchors bind a line rather
// than the string, so a version with a newline in it passed and wrote a second
// line of its own into $GITHUB_OUTPUT. A table run against the real script is
// the only form of this test that would have caught either.
func TestOnlyAReleaseTagResolves(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is the shell a `run:` step gets on the runner")
	}

	var script string
	for _, step := range release(t).Jobs["release"].Steps {
		if step.Name == "Resolve the version" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal(`release.yml has no "Resolve the version" step`)
	}
	if strings.Contains(script, "${{") {
		t.Fatal("the step now interpolates a workflow expression, so it cannot be run as written: keep its inputs in `env:`")
	}

	// The step reads the dispatch input, falls back to the ref, and writes the
	// version it settled on to the step-output file.
	resolve := func(t *testing.T, input, refName string) (string, error) {
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
		// A minimal environment, not the test process's: a run under Actions
		// already has GITHUB_OUTPUT and GITHUB_REF_NAME set to the real ones.
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"INPUT_VERSION=" + input,
			"GITHUB_REF_NAME=" + refName,
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
		for _, tc := range []struct{ input, refName, want string }{
			// A dispatch: the input wins.
			{input: "v0.1.0", refName: "main", want: "version=v0.1.0\n"},
			// A tag push: no input, and the ref is the tag.
			{input: "", refName: "v1.20.300", want: "version=v1.20.300\n"},
		} {
			written, err := resolve(t, tc.input, tc.refName)
			if err != nil {
				t.Errorf("input %q at %q: %v, want it accepted", tc.input, tc.refName, err)
			}
			if written != tc.want {
				t.Errorf("input %q at %q wrote %q, want %q", tc.input, tc.refName, written, tc.want)
			}
		}
	})

	t.Run("refuses anything else, and writes nothing", func(t *testing.T) {
		for _, version := range []string{
			"v0.1.0-rc1",          // a pre-release the manifest cannot carry
			"v0.1.0-dirty",        // a describe string off a dirty tree
			"v1.2",                // too few components
			"0.1.0",               // the v is part of the shape
			"v0.1.0 --draft",      // an argument smuggled in beside the version
			"v1.2.3 ",             // trailing whitespace
			"v1.2.3\nmalicious=1", // a second $GITHUB_OUTPUT line
			"",                    // a branch, not a tag: nothing to release
		} {
			written, err := resolve(t, version, "main")
			if err == nil {
				t.Errorf("version %q was accepted, want it refused", version)
			}
			if written != "" {
				t.Errorf("version %q was refused but wrote %q to the step output", version, written)
			}
		}
	})
}
