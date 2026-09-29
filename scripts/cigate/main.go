// Command cigate decides whether ci.yml's test jobs run (davison/md-notes#215).
//
// They are skipped when a push to main or a pull request is made only of
// commits whose subject is `docs` or `chore`, scoped forms included. It reads
// the subjects of every commit in the push or PR on stdin, one per line, and
// prints `run=true` or `run=false` for the workflow to append to
// $GITHUB_OUTPUT. One commit of any other type, or anything it cannot be sure
// of, runs everything.
//
// The type in a subject is a claim: a change that affects what the tests
// check, or that is worth testing anyway, is a `fix`, not a `docs`
// (CONTRIBUTING, "Commit messages").
//
// A release never skips. release.yml calls ci.yml through workflow_call, and
// inside a called workflow github.event_name is the caller's (a push, of a
// tag) and github.workflow is the caller's name (`release`). So the gate only
// applies when the workflow is ci itself, on a branch, on a push or a pull
// request; workflow_dispatch, workflow_call and every other event run.
// scripts/workflows/ci_release_path_test.go holds the wiring, and main_test.go the
// decision.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type input struct {
	Workflow string
	Event    string
	RefType  string
	Subjects []string
}

var skippable = regexp.MustCompile(`^(docs|chore)(\([^)]*\))?!?:`)

// decide reports whether the test jobs run.
func decide(in input) bool {
	if in.Workflow != "ci" || in.RefType != "branch" {
		return true
	}
	if in.Event != "push" && in.Event != "pull_request" {
		return true
	}
	if len(in.Subjects) == 0 {
		return true // could not list the commits: check rather than guess
	}
	for _, s := range in.Subjects {
		if !skippable.MatchString(s) {
			return true
		}
	}
	return false
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("cigate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workflow := flags.String("workflow", "", "github.workflow")
	event := flags.String("event", "", "github.event_name")
	refType := flags.String("ref-type", "", "github.ref_type")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	in := input{Workflow: *workflow, Event: *event, RefType: *refType}
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		in.Subjects = append(in.Subjects, strings.TrimSpace(sc.Text()))
	}
	if sc.Err() != nil {
		in.Subjects = nil
	}
	fmt.Fprintf(stdout, "run=%t\n", decide(in))
	return 0
}
x
