// Package config loads the mdn configuration file and resolves the
// XDG paths the daemon uses for configuration and state.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultPort is the port the daemon listens on when none is configured.
const DefaultPort = 7337

// DefaultMaxWatches is how many directories the daemon watches per root
// when none is configured. A notes folder costs tens of watches; a large
// ad-hoc root can cost tens of thousands, so the budget stops one root
// exhausting fs.inotify.max_user_watches for the whole login session. The
// value covers /usr/share whole (5,790 directories when measured), is a
// sixty-fourth of the 524,288 watches a typical Linux desktop allows, and
// matches the smallest limit still shipped, so one root cannot exhaust an
// unraised system on its own.
const DefaultMaxWatches = 8192

// Config is the on-disk configuration, with any flag overrides applied.
type Config struct {
	// NotesRoot is the permanent notes folder. Required.
	NotesRoot string `yaml:"notes_root"`
	// Port is the loopback port the daemon listens on.
	Port int `yaml:"port"`
	// MaxWatches caps the directories watched per root for live update.
	// Zero asks for DefaultMaxWatches; a negative value removes the cap.
	// Resolve never leaves it zero.
	MaxWatches int `yaml:"max_watches"`
}

// Overrides are the values a command line supplies, each taking precedence
// over the configuration file when it is not the zero value.
type Overrides struct {
	NotesRoot  string
	Port       int
	MaxWatches int
}

// Path returns the configuration file location:
// $XDG_CONFIG_HOME/mdn/config.yml, falling back to ~/.config/mdn/config.yml.
func Path() string {
	return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), "mdn", "config.yml")
}

// StatePath returns where the daemon persists ad-hoc roots:
// $XDG_STATE_HOME/mdn/roots.json, falling back to ~/.local/state/mdn/roots.json.
func StatePath() string {
	return filepath.Join(xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state")), "mdn", "roots.json")
}

func xdgDir(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, fallback)
}

// Load reads the configuration file at path. A missing file yields an
// empty Config and no error so that flags alone can configure the daemon;
// any other read or parse failure is returned.
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Resolve applies defaults and validates, with over taking precedence over
// the file. The returned NotesRoot is absolute and MaxWatches is settled:
// positive is the per-root watch budget, negative is no budget at all.
func (c Config) Resolve(configPath string, over Overrides) (Config, error) {
	if over.NotesRoot != "" {
		c.NotesRoot = over.NotesRoot
	}
	if over.Port != 0 {
		c.Port = over.Port
	}
	if over.MaxWatches != 0 {
		c.MaxWatches = over.MaxWatches
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.MaxWatches == 0 {
		c.MaxWatches = DefaultMaxWatches
	}
	if c.Port < 1 || c.Port > 65535 {
		return c, fmt.Errorf("port %d out of range", c.Port)
	}
	if c.NotesRoot == "" {
		return c, fmt.Errorf("no notes root: set notes_root in %s or pass --root", configPath)
	}
	abs, err := filepath.Abs(c.NotesRoot)
	if err != nil {
		return c, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return c, fmt.Errorf("notes root: %w", err)
	}
	if !info.IsDir() {
		return c, fmt.Errorf("notes root %s is not a directory", abs)
	}
	c.NotesRoot = abs
	return c, nil
}
