package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (Config{}) {
		t.Fatalf("cfg = %+v, want zero", cfg)
	}
}

func TestLoadParsesYAML(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	os.WriteFile(p, []byte("notes_root: /tmp/notes\nport: 9000\n"), 0o644)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NotesRoot != "/tmp/notes" || cfg.Port != 9000 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadBadYAMLNamesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	os.WriteFile(p, []byte("notes_root: [\n"), 0o644)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("err = %v, want mention of %s", err, p)
	}
}

func TestResolveDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Config{NotesRoot: "/elsewhere", Port: 1}.Resolve("cfg.yml", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NotesRoot != dir {
		t.Fatalf("root = %q, want flag override %q", cfg.NotesRoot, dir)
	}
	if cfg.Port != 1 {
		t.Fatalf("port = %d, want file value kept", cfg.Port)
	}
	cfg, err = Config{}.Resolve("cfg.yml", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != DefaultPort {
		t.Fatalf("port = %d, want default %d", cfg.Port, DefaultPort)
	}
}

func TestResolveRequiresRoot(t *testing.T) {
	_, err := Config{}.Resolve("/x/config.yml", "", 0)
	if err == nil || !strings.Contains(err.Error(), "/x/config.yml") {
		t.Fatalf("err = %v, want error naming config file", err)
	}
}

func TestResolveRejectsMissingOrFileRoot(t *testing.T) {
	if _, err := (Config{}).Resolve("c", filepath.Join(t.TempDir(), "missing"), 0); err == nil {
		t.Fatal("want error for missing root")
	}
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, nil, 0o644)
	if _, err := (Config{}).Resolve("c", f, 0); err == nil {
		t.Fatal("want error for non-directory root")
	}
}

func TestResolveRejectsBadPort(t *testing.T) {
	if _, err := (Config{}).Resolve("c", t.TempDir(), 70000); err == nil {
		t.Fatal("want error for port out of range")
	}
}

func TestPathsHonourXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xc")
	t.Setenv("XDG_STATE_HOME", "/xs")
	if got := Path(); got != "/xc/mdn/config.yml" {
		t.Fatalf("Path() = %q", got)
	}
	if got := StatePath(); got != "/xs/mdn/roots.json" {
		t.Fatalf("StatePath() = %q", got)
	}
}
