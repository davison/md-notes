// The release keeps its checks (davison/md-notes#215). ci.yml is the one place
// a commit is checked, and release.yml reaches it through `workflow_call`; the
// operator's other way in is `workflow_dispatch`. Whatever later decides that
// a docs-only change may skip the test jobs must never apply to either, and
// these tests are what notices if it does.
package workflows

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciCaller struct {
	On   map[string]any `yaml:"on"`
	Jobs map[string]struct {
		Uses string `yaml:"uses"`
		If   string `yaml:"if"`
	} `yaml:"jobs"`
}

func readCaller(t *testing.T, name string) ciCaller {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var w ciCaller
	if err := yaml.Unmarshal(raw, &w); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return w
}

// TestCIStillRunsForAReleaseAndOnDemand: ci.yml answers to workflow_call and
// workflow_dispatch, and release.yml calls it as a job of its own with no
// condition on the call, so a release cannot skip a check by way of its jobs.
func TestCIStillRunsForAReleaseAndOnDemand(t *testing.T) {
	ci := readCaller(t, "ci.yml")
	for _, event := range []string{"workflow_call", "workflow_dispatch"} {
		if _, ok := ci.On[event]; !ok {
			t.Errorf("ci.yml no longer declares %s: a release, or a run on demand, cannot reach its checks", event)
		}
	}

	rel := readCaller(t, "release.yml")
	called := 0
	for name, job := range rel.Jobs {
		if job.Uses != "./.github/workflows/ci.yml" {
			continue
		}
		called++
		if job.If != "" {
			t.Errorf("release.yml job %q calls ci.yml under `if: %s`: a release must always run every check", name, job.If)
		}
	}
	if called != 1 {
		t.Errorf("release.yml calls ci.yml from %d jobs, want exactly 1", called)
	}
}
