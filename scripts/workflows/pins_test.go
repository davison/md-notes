// Assertions over what every workflow pins (davison/md-notes#193): the runner
// image, the Node runtime each action runs on, and the Go and Node a release is
// built with. The reasons are written once, at the top of ci.yml's jobs.
//
// Each of these regresses silently. A job copied from an older workflow
// brings `ubuntu-latest` or an `@v4` with it and still goes green: the runner
// forces a Node 20 action onto Node 24 and says so only in an annotation, and
// the moving label keeps working until the day it moves.
package workflows

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// pinnedRunner is the one image label a job may run on. Moving to another is a
// change to this constant and to the four files, reviewed together.
const pinnedRunner = "ubuntu-24.04"

// node24Majors is, for every action these workflows use, the first major whose
// tag resolves to an action.yml saying `runs.using: node24` — read from each
// action's action.yml at the major tag on 2026-09-22, not guessed from release
// notes, which are not always right: setup-go's v6.0.0 notes announce Node 24,
// and its action.yml said node20 until v6.2.0. The workflows are on the current
// majors, at or above these; this is the floor, so the test names the property
// (no Node 20 action) rather than a version to bump.
var node24Majors = map[string]int{
	"actions/checkout":        5,
	"actions/setup-node":      5,
	"actions/setup-go":        6,
	"actions/cache":           5,
	"actions/upload-artifact": 6, // v5 still says node20
	"pnpm/action-setup":       5,
}

type pinnedWorkflow struct {
	Jobs map[string]struct {
		RunsOn any    `yaml:"runs-on"`
		Uses   string `yaml:"uses"`
		Steps  []struct {
			Uses string            `yaml:"uses"`
			With map[string]string `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// allWorkflows parses every workflow file, by name.
func allWorkflows(t *testing.T) map[string]pinnedWorkflow {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("found no workflow files")
	}
	parsed := map[string]pinnedWorkflow{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var w pinnedWorkflow
		if err := yaml.Unmarshal(raw, &w); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		parsed[filepath.Base(path)] = w
	}
	return parsed
}

func TestEveryJobRunsOnThePinnedImage(t *testing.T) {
	for file, w := range allWorkflows(t) {
		for name, job := range w.Jobs {
			if job.Uses != "" {
				continue // a reusable-workflow call runs on the callee's runner
			}
			if label, _ := job.RunsOn.(string); label != pinnedRunner {
				t.Errorf("%s: job %q runs on %v, want %q — a moving label changes the image under a tag that has already been cut (davison/md-notes#193)",
					file, name, job.RunsOn, pinnedRunner)
			}
		}
	}
}

func TestEveryActionRunsOnNode24(t *testing.T) {
	ref := regexp.MustCompile(`^([^@]+)@v([0-9]+)$`)
	for file, w := range allWorkflows(t) {
		for name, job := range w.Jobs {
			for _, step := range job.Steps {
				if step.Uses == "" || strings.HasPrefix(step.Uses, "./") {
					continue
				}
				m := ref.FindStringSubmatch(step.Uses)
				if m == nil {
					t.Errorf("%s: job %q uses %q, which is not an owner/repo@v<major> reference", file, name, step.Uses)
					continue
				}
				floor, known := node24Majors[m[1]]
				if !known {
					t.Errorf("%s: job %q uses %s, which this test does not know: add it to node24Majors with the first major whose action.yml says `using: node24`", file, name, m[1])
					continue
				}
				if major, _ := strconv.Atoi(m[2]); major < floor {
					t.Errorf("%s: job %q uses %s, a major that runs on Node 20; v%d is the first on Node 24", file, name, step.Uses, floor)
				}
			}
		}
	}
}

// TestTheReleaseToolchainIsPinnedExactly: the Go and the Node that build a
// release are named by the tag, exactly, so "rebuild it and compare" has an
// answer (davison/md-notes#167, measured there). A floating `node-version: 24`
// or a go.mod without a toolchain line would still build, from whatever the
// runner resolved on the day.
func TestTheReleaseToolchainIsPinnedExactly(t *testing.T) {
	exact := regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

	raw, err := os.ReadFile(filepath.Join(repoRoot, ".node-version"))
	if err != nil {
		t.Fatalf("no .node-version: %v", err)
	}
	if node := strings.TrimSpace(string(raw)); !exact.MatchString(node) {
		t.Errorf(".node-version is %q, want one exact version such as 24.21.0", node)
	}

	gomod, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^toolchain go[0-9]+\.[0-9]+\.[0-9]+$`).Match(gomod) {
		t.Error("go.mod has no exact `toolchain goX.Y.Z` line, so setup-go builds a release with whichever patch release it finds")
	}

	setups := 0
	for file, w := range allWorkflows(t) {
		for name, job := range w.Jobs {
			for _, step := range job.Steps {
				switch {
				case strings.HasPrefix(step.Uses, "actions/setup-node@"):
					setups++
					if step.With["node-version-file"] != ".node-version" || step.With["node-version"] != "" {
						t.Errorf("%s: job %q sets up Node with node-version-file %q and node-version %q, want the file .node-version and no node-version", file, name, step.With["node-version-file"], step.With["node-version"])
					}
				case strings.HasPrefix(step.Uses, "actions/setup-go@"):
					setups++
					if step.With["go-version-file"] != "go.mod" || step.With["go-version"] != "" {
						t.Errorf("%s: job %q sets up Go with go-version-file %q and go-version %q, want the file go.mod and no go-version", file, name, step.With["go-version-file"], step.With["go-version"])
					}
				}
			}
		}
	}
	if setups == 0 {
		t.Fatal("found no setup-node or setup-go step at all: the parse is broken, not the workflows")
	}
}
