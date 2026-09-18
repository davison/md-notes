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

// TestOnlyATagPushPublishes holds the claim the record makes in three places —
// the dispatch input's description, the run summary, and the decision recorded
// on davison/md-notes#134 — that a workflow_dispatch of the release workflow is
// always a dry run.
//
// A dispatch may name a tag as its ref, so a guard on github.ref does not say
// this: dispatched at a tag, it would publish a Release under the dispatch's
// input version, which need not be the tag the workflow ran at. Only the event
// distinguishes the two.
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
