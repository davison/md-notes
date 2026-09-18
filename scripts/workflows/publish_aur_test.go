package workflows

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The AUR channel is the one place in this repository that writes to something
// outside it. What it pushes is public, permanent enough that the AUR records
// the author forever, and it runs unattended — so the four properties below
// are the ones worth a test rather than a careful reading.

type aurWorkflow struct {
	Name string               `yaml:"name"`
	On   map[string]yaml.Node `yaml:"on"`
	Jobs map[string]struct {
		Environment string    `yaml:"environment"`
		Steps       []aurStep `yaml:"steps"`
	} `yaml:"jobs"`
}

type aurStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	If   string            `yaml:"if"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

func publishAURPath() string {
	return filepath.Join("..", "..", ".github", "workflows", "publish-aur.yml")
}

func publishAUR(t *testing.T) aurWorkflow {
	t.Helper()
	var parsed aurWorkflow
	if err := yaml.Unmarshal(readRepoFile(t, publishAURPath()), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Jobs) != 1 {
		t.Fatalf("publish-aur.yml has %d jobs, want 1", len(parsed.Jobs))
	}
	return parsed
}

func readRepoFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestTheAURIsPublishedOnlyWhenAReleaseIs. M8-R1 puts every channel on
// `release: published`, and the reason is in docs/releasing.md: the operator
// reading the draft's notes and pressing Publish is the human review point for
// everything that follows. A second trigger — a workflow_dispatch with a tag
// input, a push, a schedule — would be a way to reach the AUR that skips that
// reading. Re-running a failed channel does not need one: the Actions page
// re-runs the run.
func TestTheAURIsPublishedOnlyWhenAReleaseIs(t *testing.T) {
	on := publishAUR(t).On
	if len(on) != 1 {
		var triggers []string
		for name := range on {
			triggers = append(triggers, name)
		}
		t.Fatalf("publish-aur.yml triggers on %v, want release alone", triggers)
	}
	node, ok := on["release"]
	if !ok {
		t.Fatalf("publish-aur.yml does not trigger on a release")
	}
	var release struct {
		Types []string `yaml:"types"`
	}
	if err := node.Decode(&release); err != nil {
		t.Fatal(err)
	}
	if len(release.Types) != 1 || release.Types[0] != "published" {
		t.Errorf("the release trigger takes types %v, want [published]: a draft Release must start nothing", release.Types)
	}
}

// TestTheDeployKeyIsTheSecretTheGateNames. The operator was asked, on
// davison/md-notes#136, for an AUR account and a new SSH key pair whose
// private half lands in a repository secret of exactly this name. A workflow
// that read a differently named secret would fail its first real release with
// an empty key, a long way from anyone who could see why.
func TestTheDeployKeyIsTheSecretTheGateNames(t *testing.T) {
	const want = "AUR_SSH_PRIVATE_KEY"

	body := string(readRepoFile(t, publishAURPath()))
	secret := regexp.MustCompile(`secrets\.([A-Za-z0-9_]+)`)
	for _, match := range secret.FindAllStringSubmatch(body, -1) {
		if match[1] != want && match[1] != "GITHUB_TOKEN" {
			t.Errorf("publish-aur.yml reads the secret %s; the gate on davison/md-notes#136 asked for %s and nothing else", match[1], want)
		}
	}

	pushes := pushStep(t)
	if pushes.Env[want] != "${{ secrets."+want+" }}" {
		t.Errorf("the push step reads %s from %q, want the secret of that name", want, pushes.Env[want])
	}
}

// TestNothingReachesTheAURUntilItHasBeenBuilt. The AUR submission guidelines
// say automated PKGBUILD updates are the maintainer's own risk and that
// malfunctioning accounts and their packages may be removed without notice. So
// the push is last, after the package has been built, linted and installed in
// a container — and the test is over the order of the steps, because the
// difference between a workflow that verifies and one that verifies afterwards
// is where one step sits in a list.
func TestNothingReachesTheAURUntilItHasBeenBuilt(t *testing.T) {
	steps := theJob(t)
	verifies, pushes := -1, -1
	for i, step := range steps {
		switch {
		case strings.Contains(step.Run, "archlinux:latest") && strings.Contains(step.Run, "verify.sh"):
			verifies = i
		case strings.Contains(step.Run, "git push"):
			pushes = i
		}
	}
	if verifies < 0 {
		t.Fatal("no step builds the package in an archlinux:latest container")
	}
	if pushes < 0 {
		t.Fatal("no step pushes to the AUR")
	}
	if verifies > pushes {
		t.Errorf("the push (step %d) runs before the container build (step %d)", pushes, verifies)
	}

	// And what that container step is asked to do. verify.sh is a file of its
	// own so a person can run the same checks by hand; these are the ones the
	// requirement names.
	verify := string(readRepoFile(t, filepath.Join("..", "..", "packaging", "aur", "verify.sh")))
	for _, want := range []struct{ fragment, why string }{
		{"makepkg --printsrcinfo", "the AUR reads the version from .SRCINFO alone, so it has to be the one makepkg would write"},
		{"makepkg --syncdeps", "the package is built before it is pushed"},
		{"namcap PKGBUILD", "M8-R3 asks for namcap over the PKGBUILD"},
		{"namcap ./*.pkg.tar.zst", "M8-R3 asks for namcap over the built package"},
		{"pacman -U", "the built package is installed"},
		{"mdn version", "the installed binary reports the release's version"},
		{"sudo -u builder", "makepkg refuses to run as root, and should"},
	} {
		if !strings.Contains(verify, want.fragment) {
			t.Errorf("packaging/aur/verify.sh does not run %q: %s", want.fragment, want.why)
		}
	}
}

// TestThePushGoesOnlyToTheAURsMaster. One remote, named in full; the branch the
// AUR accepts and no other; and no force, which on a repository whose history
// is the package's record would rewrite what users have already fetched.
func TestThePushGoesOnlyToTheAURsMaster(t *testing.T) {
	// The package the PKGBUILD declares is the repository the AUR keeps it in:
	// pkgbase is the repository name. If one changes, so must the other.
	pkgbuild := string(readRepoFile(t, filepath.Join("..", "..", "packaging", "aur", "PKGBUILD")))
	name := regexp.MustCompile(`(?m)^pkgname=(\S+)$`).FindStringSubmatch(pkgbuild)
	if name == nil {
		t.Fatal("packaging/aur/PKGBUILD declares no pkgname")
	}
	want := "ssh://aur@aur.archlinux.org/" + name[1] + ".git"

	step := pushStep(t)
	remotes := regexp.MustCompile(`ssh://\S+`).FindAllString(step.Run+strings.Join(mapValues(step.Env), "\n"), -1)
	if len(remotes) == 0 {
		t.Fatal("the push step names no ssh remote")
	}
	for _, remote := range remotes {
		if remote != want {
			t.Errorf("the push step reaches %s, want %s", remote, want)
		}
	}

	for _, line := range strings.Split(step.Run, "\n") {
		if !strings.Contains(line, "git push") {
			continue
		}
		if !strings.Contains(line, "origin master") {
			t.Errorf("the push is %q; the AUR only accepts pushes to master", strings.TrimSpace(line))
		}
		if strings.Contains(line, "--force") || strings.Contains(line, " -f ") {
			t.Errorf("the push is forced: %q", strings.TrimSpace(line))
		}
	}
}

// pushStep is the one step that writes to the AUR.
func pushStep(t *testing.T) aurStep {
	t.Helper()
	var found []aurStep
	for _, step := range theJob(t) {
		if strings.Contains(step.Run, "git push") {
			found = append(found, step)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d steps push to the AUR, want exactly 1", len(found))
	}
	return found[0]
}

func theJob(t *testing.T) []aurStep {
	t.Helper()
	for _, job := range publishAUR(t).Jobs {
		if len(job.Steps) == 0 {
			t.Fatal("publish-aur.yml's job has no steps")
		}
		return job.Steps
	}
	t.Fatal("publish-aur.yml has no job")
	return nil
}

func mapValues(m map[string]string) []string {
	var values []string
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// TestNothingButARealRenderingGetsPastTheGuard runs the guard step's script,
// as written, against four renderings.
//
// The step is the second line of defence — aurgen refuses a SKIP rendering and
// a placeholder maintainer itself, and those refusals are tested where they
// live — but it is the one that does not depend on the renderer being right,
// and the review of PR #152 measured that deleting it left every test in this
// package green (finding 4). It also holds the empty-secret refusal, which
// otherwise fails a release inside `git clone` with an ssh error a long way
// from anyone who could read it.
func TestNothingButARealRenderingGetsPastTheGuard(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is the shell a `run:` step gets on the runner")
	}

	var script string
	steps := theJob(t)
	guard, verifies, pushes := -1, -1, -1
	for i, step := range steps {
		switch {
		case strings.Contains(step.Run, "'SKIP'"):
			guard, script = i, step.Run
		case strings.Contains(step.Run, "archlinux:latest"):
			verifies = i
		case strings.Contains(step.Run, "git push"):
			pushes = i
		}
	}
	if script == "" {
		t.Fatal("no step refuses a PKGBUILD still carrying SKIP checksums")
	}
	if strings.Contains(script, "${{") {
		t.Fatal("the step now interpolates a workflow expression, so it cannot be run as written: keep its inputs in `env:`")
	}
	// Before the build as well as before the push: the plan on
	// davison/md-notes#136 put it there, and a condition known at render time
	// should not cost a container build to discover.
	if !(guard < verifies && verifies < pushes) {
		t.Errorf("the steps run guard=%d, build=%d, push=%d; want the guard first and the push last", guard, verifies, pushes)
	}

	// The committed PKGBUILD is the placeholder rendering: real metadata, real
	// maintainer line, and SKIP where a real release's checksums go. A release
	// renders the same file with those filled in.
	committed := string(readRepoFile(t, filepath.Join("..", "..", "packaging", "aur", "PKGBUILD")))
	released := strings.ReplaceAll(committed, "'SKIP'", "'"+strings.Repeat("a", 64)+"'")
	// And the state the gate on davison/md-notes#136 left behind until the
	// operator answered it: aurgen's placeholder marker, in the line the AUR
	// reads first. It is spelled out here rather than imported because aurgen
	// is a main package; if the marker changes there, this test says so by
	// letting the rendering through.
	unanswered := regexp.MustCompile(`(?m)^# Maintainer: .*$`).
		ReplaceAllString(released, "# Maintainer: Someone <maintainer at example dot invalid>")
	if unanswered == released {
		t.Fatal("the rendered PKGBUILD has no # Maintainer: line to replace")
	}

	for _, tc := range []struct {
		name     string
		pkgbuild string
		secret   string
		accepted bool
	}{
		{name: "a real release, with the key", pkgbuild: released, secret: "a private key", accepted: true},
		{name: "the committed template", pkgbuild: committed, secret: "a private key"},
		{name: "the maintainer gate still unanswered", pkgbuild: unanswered, secret: "a private key"},
		{name: "no deploy key in the secret", pkgbuild: released, secret: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "packaging", "aur", "PKGBUILD")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.pkgbuild), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bash, "-e", "-o", "pipefail", "-c", script)
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "AUR_SSH_PRIVATE_KEY=" + tc.secret}
			out, err := cmd.CombinedOutput()
			if tc.accepted && err != nil {
				t.Errorf("refused a rendering that should have gone through: %v: %s", err, out)
			}
			if !tc.accepted && err == nil {
				t.Errorf("let this reach the AUR, want it refused: %s", out)
			}
		})
	}
}
