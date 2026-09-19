// Assertions over the Makefile's `install`, `clean` and `distclean` targets
// (davison/md-notes#164).
//
// `make` is run for real, in a temporary tree holding a copy of the Makefile,
// rather than asserted over the Makefile's text. The bug this pins was a
// prerequisite — `install: build` — which pulled `pnpm install` and `go build`
// into `sudo make install`, downloading the toolchain as root and leaving
// root-owned files in the operator's checkout (#163). No assertion about what
// the recipe *looks* like would have caught it: the damage was done by a line
// that is not in the target at all.
package workflows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeTool is the `make` binary, or a skip: these tests are about what make
// does, and there is nothing to assert without one.
func makeTool(t *testing.T) string {
	t.Helper()
	tool, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is what these tests exercise")
	}
	return tool
}

// runMake runs make in dir and returns its combined output and its error.
func runMake(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(makeTool(t), args...)
	cmd.Dir = dir
	// A tree that is not a repository, and a make that must not reach for the
	// network: keep the environment, but let nothing inherit a stale MAKEFLAGS.
	cmd.Env = append(os.Environ(), "MAKEFLAGS=")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// installTree lays out a temporary tree that `make install` can run in: the
// repository's Makefile, and whichever of the two payload files the caller
// asked for. It returns the tree and the DESTDIR to install into.
func installTree(t *testing.T, withBinary, withUnit bool) (tree, destdir string) {
	t.Helper()
	tree = t.TempDir()

	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "Makefile"), makefile, 0o644); err != nil {
		t.Fatal(err)
	}

	if withBinary {
		// Not a real binary; `install` copies bytes and must not care.
		if err := os.WriteFile(filepath.Join(tree, "mdn"), []byte("#!/bin/sh\necho mdn\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if withUnit {
		unit, err := os.ReadFile(filepath.Join(repoRoot, "contrib", "mdn.service"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(tree, "contrib"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tree, "contrib", "mdn.service"), unit, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return tree, filepath.Join(tree, "destdir")
}

// TestInstallNeverBuilds is the heart of #163: `make install` must not depend
// on `build`, because as root that prerequisite runs `pnpm install` and
// `go build` with root's empty caches.
//
// A dry run, so it is safe in the checkout, and it answers the same whether or
// not ./mdn exists — which is itself the point: the old `install: build` would
// have printed the pnpm and go lines here either way, `ui-deps` being phony.
func TestInstallNeverBuilds(t *testing.T) {
	out, err := runMake(t, repoRoot, "-n", "install")
	if err != nil {
		t.Fatalf("make -n install: %v\n%s", err, out)
	}
	for _, forbidden := range []string{"pnpm", "go build"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("`make -n install` would run %q:\n%s\n"+
				"install must copy what `make build` already made and nothing else — under sudo that command downloads the toolchain and leaves root-owned files in the checkout (davison/md-notes#163).",
				forbidden, out)
		}
	}
	for _, want := range []string{"install -Dm755 mdn", "lib/systemd/user/mdn.service"} {
		if !strings.Contains(out, want) {
			t.Errorf("`make -n install` does not mention %q:\n%s", want, out)
		}
	}
}

// TestInstallRefusesWhatIsNotThere: one line naming the file, pointing at
// `make build`, and nothing copied — the refusal comes before the first copy,
// so a tree missing the unit does not get a half-install with the binary in it.
func TestInstallRefusesWhatIsNotThere(t *testing.T) {
	for _, tc := range []struct {
		name               string
		binary, unit       bool
		names, alsoMention string
	}{
		{name: "no binary", binary: false, unit: true, names: "mdn", alsoMention: "make build"},
		{name: "no unit", binary: true, unit: false, names: "contrib/mdn.service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, destdir := installTree(t, tc.binary, tc.unit)

			out, err := runMake(t, tree, "install", "DESTDIR="+destdir)
			if err == nil {
				t.Fatalf("`make install` succeeded with %s missing; it must refuse:\n%s", tc.names, out)
			}

			refusal := ""
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				if strings.Contains(line, tc.names) {
					refusal = line
					break
				}
			}
			if refusal == "" {
				t.Errorf("the refusal does not name %s:\n%s", tc.names, out)
			}
			if tc.alsoMention != "" && !strings.Contains(out, tc.alsoMention) {
				t.Errorf("the refusal does not point at `%s`:\n%s", tc.alsoMention, out)
			}

			if _, err := os.Stat(destdir); !os.IsNotExist(err) {
				t.Errorf("`make install` wrote into %s before refusing; it must install nothing at all", destdir)
			}
		})
	}
}

// TestInstallCopiesTheBinaryAndTheUnit: the two paths the .deb installs, the
// modes it installs them with, DESTDIR honoured, and the `systemctl --user`
// lines printed for the invoking user to run (the decision on #164: a user
// unit cannot be enabled for them from under sudo).
func TestInstallCopiesTheBinaryAndTheUnit(t *testing.T) {
	tree, destdir := installTree(t, true, true)

	out, err := runMake(t, tree, "install", "DESTDIR="+destdir)
	if err != nil {
		t.Fatalf("make install: %v\n%s", err, out)
	}

	for _, want := range []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(destdir, "usr", "bin", "mdn"), 0o755},
		{filepath.Join(destdir, "usr", "lib", "systemd", "user", "mdn.service"), 0o644},
	} {
		info, err := os.Stat(want.path)
		if err != nil {
			t.Errorf("`make install` did not install %s: %v\n%s", want.path, err, out)
			continue
		}
		if got := info.Mode().Perm(); got != want.mode {
			t.Errorf("%s has mode %o, want %o", want.path, got, want.mode)
		}
	}

	// The unit is copied byte for byte, as the .deb copies it: the packaged
	// unit and the from-source unit are the same file.
	installed, err := os.ReadFile(filepath.Join(destdir, "usr", "lib", "systemd", "user", "mdn.service"))
	if err == nil {
		source, err := os.ReadFile(filepath.Join(repoRoot, "contrib", "mdn.service"))
		if err != nil {
			t.Fatal(err)
		}
		if string(installed) != string(source) {
			t.Error("the installed unit is not contrib/mdn.service byte for byte")
		}
	}

	for _, want := range []string{"systemctl --user daemon-reload", "systemctl --user enable --now mdn"} {
		if !strings.Contains(out, want) {
			t.Errorf("`make install` does not print %q, which is the step it cannot take for the invoking user:\n%s", want, out)
		}
	}
}

// TestTheInstalledUnitPathAndThePackageAgree is the unit's half of
// TestTheInstallPrefixAndTheUnitAgree: `make install` now installs the unit as
// well as the binary, and it has to land where the package puts it, or a
// machine that has had both gets two units that shadow each other.
func TestTheInstalledUnitPathAndThePackageAgree(t *testing.T) {
	prefix := makefilePrefix(t)

	var packaged string
	for _, entry := range packageConfig(t).Contents {
		if entry.Src == "mdn.service" {
			packaged = entry.Dst
		}
	}
	if packaged == "" {
		t.Fatal("packaging/deb/nfpm.yaml installs no mdn.service")
	}

	if want := prefix + "/lib/systemd/user/mdn.service"; packaged != want {
		t.Errorf("the .deb installs the unit at %q but `make install` puts it at %q (PREFIX ?= %s); the two routes have to agree on the path, as they do on the binary's",
			packaged, want, prefix)
	}
}

// TestCleanReportsWhatItCannotRemove: a clean that dies on the first
// undeletable file hides the rest of them, and the rest are what the operator
// has to go and remove. It removes everything it can, names what is left, and
// still fails.
func TestCleanReportsWhatItCannotRemove(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove anything, so there is nothing to report")
	}
	tree := t.TempDir()

	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "Makefile"), makefile, 0o644); err != nil {
		t.Fatal(err)
	}

	write := func(rel, content string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(tree, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("mdn", "binary", 0o755)
	write("dist/mdn-v0.1.0-linux-amd64", "binary", 0o755)
	write("ui/dist/.gitkeep", "", 0o644)
	write("ui/dist/index.html", "<!doctype html>", 0o644)
	write("extension/dist/manifest.json", "{}", 0o644)
	// The zip is inside the locked directory too, and deliberately: it is the
	// *second* path the recipe attempts, so locking it is what makes a
	// die-on-first-failure clean visibly different from this one. With only
	// extension/dist locked — the last path — a recipe that stopped at the
	// first failure would still have removed everything else, and this test
	// passed against both implementations (the review of PR #166, finding 1).
	write("extension/mdn-extension.zip", "PK", 0o644)

	// Stands in for the root-owned leftovers of an older `sudo make`: rm needs
	// write permission on the *parent*, so a read-only extension/ makes both
	// of those undeletable without being undeletable by root.
	locked := filepath.Join(tree, "extension")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	out, err := runMake(t, tree, "clean")
	if err == nil {
		t.Fatalf("`make clean` succeeded with extension/ read-only; it has to fail, having said so:\n%s", out)
	}
	// Both survivors, read out of the report itself rather than found anywhere
	// in the output: the old recipe echoed `rm -f mdn extension/mdn-extension.zip`
	// before failing on it, so a plain substring match would take the echo of a
	// command for the report of its failure.
	reported := map[string]bool{}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "could not remove:") {
			continue
		}
		for _, entry := range lines[i+1:] {
			if !strings.HasPrefix(entry, "  ") || strings.TrimSpace(entry) == "" {
				break
			}
			reported[strings.TrimSpace(entry)] = true
		}
	}
	for _, survivor := range []string{"extension/mdn-extension.zip", "extension/dist"} {
		if !reported[survivor] {
			t.Errorf("`make clean` does not report %s among the paths it could not remove:\n%s", survivor, out)
		}
	}

	// Everything else went, which is the half a die-on-first clean gets wrong:
	// the zip is attempted second, so a recipe that stops there leaves dist/
	// and ui/dist/index.html behind.
	for _, gone := range []string{"mdn", "dist", "ui/dist/index.html"} {
		if _, err := os.Stat(filepath.Join(tree, gone)); !os.IsNotExist(err) {
			t.Errorf("`make clean` stopped before removing %s", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(tree, "ui", "dist", ".gitkeep")); err != nil {
		t.Errorf("`make clean` removed ui/dist/.gitkeep, which is tracked: %v", err)
	}
}

// TestCleanLeavesTheDependencyTreesToDistclean pins the other decision on
// #164: node_modules is a lockfile-keyed cache, minutes and a network to
// rebuild, so `clean` stays cheap and `distclean` is the one that takes it.
func TestCleanLeavesTheDependencyTreesToDistclean(t *testing.T) {
	tree := t.TempDir()

	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "Makefile"), makefile, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"ui/node_modules/left-pad", "extension/node_modules/left-pad"} {
		if err := os.MkdirAll(filepath.Join(tree, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if out, err := runMake(t, tree, "clean"); err != nil {
		t.Fatalf("make clean: %v\n%s", err, out)
	}
	for _, dir := range []string{"ui/node_modules", "extension/node_modules"} {
		if _, err := os.Stat(filepath.Join(tree, dir)); err != nil {
			t.Errorf("`make clean` removed %s; it is distclean's, not clean's (davison/md-notes#164)", dir)
		}
	}

	if out, err := runMake(t, tree, "distclean"); err != nil {
		t.Fatalf("make distclean: %v\n%s", err, out)
	}
	for _, dir := range []string{"ui/node_modules", "extension/node_modules"} {
		if _, err := os.Stat(filepath.Join(tree, dir)); !os.IsNotExist(err) {
			t.Errorf("`make distclean` left %s behind", dir)
		}
	}
}

// TestTheHousekeepingTargetsArePhony: `install`, `clean` and `distclean` name
// nothing on disk, and a repository that grew a file called `install` would
// otherwise turn `make install` into "nothing to be done".
func TestTheHousekeepingTargetsArePhony(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	phony := ""
	for _, line := range strings.Split(string(makefile), "\n") {
		if rest, ok := strings.CutPrefix(line, ".PHONY:"); ok {
			phony += " " + rest + " "
		}
	}
	for _, target := range []string{"install", "clean", "distclean"} {
		if !strings.Contains(phony, " "+target+" ") {
			t.Errorf(".PHONY does not list %s (it reads:%s)", target, phony)
		}
	}
}

// makefilePrefix is the Makefile's `PREFIX ?=` default, which is half of every
// agreement between the from-source install and the package.
func makefilePrefix(t *testing.T) string {
	t.Helper()
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	prefix := ""
	for _, line := range strings.Split(string(makefile), "\n") {
		if rest, ok := strings.CutPrefix(line, "PREFIX ?="); ok {
			prefix = strings.TrimSpace(rest)
		}
	}
	if prefix == "" {
		t.Fatal("the Makefile declares no `PREFIX ?=` default; `make install` and contrib/mdn.service have to agree on one, and this is half of it")
	}
	return prefix
}
