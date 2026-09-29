// The release keeps its checks (davison/md-notes#215). ci.yml is the one place
// a commit is checked, and release.yml reaches it through `workflow_call`; the
// operator's other way in is `workflow_dispatch`. Whatever later decides that
// a docs-only change may skip the test jobs must never apply to either, and
// these tests are what notices if it does.
package workflows

import (
	"os"
	"path/filepath"
	"strings"
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

type gatedCI struct {
	On   map[string]any `yaml:"on"`
	Jobs map[string]struct {
		Needs any    `yaml:"needs"`
		If    string `yaml:"if"`
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestTheSkipGateDecidesOnlyThroughCigate: the test jobs are skipped by one
// condition, the output of the gate job, and the gate job hands cigate the
// three values it needs to tell a release (workflow `release`, ref_type `tag`,
// the caller's event) and a dispatch from a push or pull request to ci itself.
// Nothing else may skip them: not an `if` on the event name, which is the
// caller's under workflow_call, and not a `paths-ignore`, which would apply to
// a push and would make the release's checks depend on what the tagged commit
// touched.
func TestTheSkipGateDecidesOnlyThroughCigate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var ci gatedCI
	if err := yaml.Unmarshal(raw, &ci); err != nil {
		t.Fatal(err)
	}

	for event, cfg := range ci.On {
		if m, ok := cfg.(map[string]any); ok {
			for _, key := range []string{"paths", "paths-ignore"} {
				if _, has := m[key]; has {
					t.Errorf("ci.yml's %s trigger filters on %s: the skip is decided by scripts/cigate from the commit subjects, and a path filter would apply on its own terms", event, key)
				}
			}
		}
	}

	gate, ok := ci.Jobs["gate"]
	if !ok {
		t.Fatal("ci.yml has no gate job")
	}
	if gate.If != "" {
		t.Errorf("the gate job runs under `if: %s`: it must always run, so that it is the one place that decides", gate.If)
	}
	var script string
	for _, s := range gate.Steps {
		script += s.Run + "\n"
	}
	for _, want := range []string{
		"./scripts/cigate",
		`-workflow "$GITHUB_WORKFLOW"`,
		`-event "$GITHUB_EVENT_NAME"`,
		`-ref-type "$GITHUB_REF_TYPE"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the gate job's script does not contain %s", want)
		}
	}

	const want = "needs.gate.outputs.run == 'true'"
	for _, name := range []string{"check", "e2e"} {
		job := ci.Jobs[name]
		if needs, _ := job.Needs.(string); needs != "gate" {
			t.Errorf("job %q needs %v, want gate", name, job.Needs)
		}
		if job.If != want {
			t.Errorf("job %q runs under `if: %s`, want exactly `if: %s`", name, job.If, want)
		}
	}
	for name, job := range ci.Jobs {
		if name != "check" && name != "e2e" && name != "gate" {
			t.Errorf("ci.yml has a job %q that the gate test does not know: decide whether it is gated, and say so here", name)
		}
		if strings.Contains(job.If, "github.event_name") {
			t.Errorf("job %q tests github.event_name, which is the caller's under workflow_call", name)
		}
	}
}
