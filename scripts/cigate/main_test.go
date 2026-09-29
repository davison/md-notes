package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecide(t *testing.T) {
	cases := []struct {
		name string
		in   input
		want bool
	}{
		{"docs only push", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"docs: say more (#1)"}}, false},
		{"chore only PR", input{Workflow: "ci", Event: "pull_request", RefType: "branch", Subjects: []string{"chore(deps): bump x"}}, false},
		{"scoped and breaking forms", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"docs(readme): a", "chore!: b", "chore(x)!: c"}}, false},
		{"one feat among docs runs everything", input{Workflow: "ci", Event: "pull_request", RefType: "branch", Subjects: []string{"docs: a", "feat(ui): b", "docs: c"}}, true},
		{"the head commit alone is not enough", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"fix: a", "docs: b"}}, true},
		{"the tail commit alone is not enough", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"docs: a", "fix: b"}}, true},
		{"fix with a docs scope is not docs", input{Workflow: "ci", Event: "pull_request", RefType: "branch", Subjects: []string{"fix(docs): a"}}, true},
		{"a type merely starting with docs", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"docsy: a"}}, true},
		{"a subject with no type", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"Merge branch x"}}, true},
		{"no commits found runs everything", input{Workflow: "ci", Event: "push", RefType: "branch"}, true},
		{"blank subject runs everything", input{Workflow: "ci", Event: "push", RefType: "branch", Subjects: []string{"docs: a", ""}}, true},
		// The release: release.yml calls ci.yml, and inside the called workflow
		// github.event_name is the caller's — a push, of a tag — while
		// github.workflow is the caller's name.
		{"release tag push through workflow_call", input{Workflow: "release", Event: "push", RefType: "tag", Subjects: []string{"docs: a"}}, true},
		{"release ref_type alone", input{Workflow: "ci", Event: "push", RefType: "tag", Subjects: []string{"docs: a"}}, true},
		{"release workflow name alone", input{Workflow: "release", Event: "push", RefType: "branch", Subjects: []string{"docs: a"}}, true},
		{"release dry run", input{Workflow: "release", Event: "workflow_dispatch", RefType: "branch", Subjects: []string{"docs: a"}}, true},
		{"ci dispatched by hand", input{Workflow: "ci", Event: "workflow_dispatch", RefType: "branch", Subjects: []string{"docs: a"}}, true},
		{"an event the gate does not know", input{Workflow: "ci", Event: "schedule", RefType: "branch", Subjects: []string{"docs: a"}}, true},
	}
	for _, c := range cases {
		if got := decide(c.in); got != c.want {
			t.Errorf("%s: run = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRunReadsSubjectsAndPrintsTheOutput(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-workflow", "ci", "-event", "pull_request", "-ref-type", "branch"},
		strings.NewReader("docs: a\nchore(x): b\n"), &out, &errb)
	if code != 0 || out.String() != "run=false\n" {
		t.Errorf("code %d, out %q, err %q", code, out.String(), errb.String())
	}
	out.Reset()
	run([]string{"-workflow", "ci", "-event", "pull_request", "-ref-type", "branch"},
		strings.NewReader("docs: a\nfix: b\n"), &out, &errb)
	if out.String() != "run=true\n" {
		t.Errorf("out %q", out.String())
	}
}
