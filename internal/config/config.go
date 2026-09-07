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

// Config is the on-disk configuration, with any flag overrides applied.
type Config struct {
	// NotesRoot is the permanent notes folder. Required.
	NotesRoot string `yaml:"notes_root"`
	// Port is the loopback port the daemon listens on.
	Port int `yaml:"port"`
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

// Resolve applies defaults and validates. root and port override the
// file's values when non-zero. The returned NotesRoot is absolute.
func (c Config) Resolve(configPath, root string, port int) (Config, error) {
	if root != "" {
		c.NotesRoot = root
	}
	if port != 0 {
		c.Port = port
	}
	if c.Port == 0 {
		c.Port = DefaultPort
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
