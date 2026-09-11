// Package config loads the mdn configuration file and resolves the
// XDG paths the daemon uses for configuration and state.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

// DefaultClipsDir is where clips land inside the notes root when the
// configuration does not say otherwise.
const DefaultClipsDir = "clips"

// Config is the on-disk configuration, with any flag overrides applied.
type Config struct {
	// NotesRoot is the permanent notes folder. Required.
	NotesRoot string `yaml:"notes_root"`
	// Port is the loopback port the daemon listens on.
	Port int `yaml:"port"`
	// MaxWatches caps the directories watched per root for live update.
	// Zero is no budget at all; absent from the file asks for
	// DefaultMaxWatches. Resolve settles it to a non-nil value.
	MaxWatches *int `yaml:"max_watches"`
	// ClipsDir is where the clip endpoint writes, relative to the notes
	// root. Absent asks for DefaultClipsDir; a path that leaves the notes
	// root is refused.
	ClipsDir string `yaml:"clips_dir"`
	// TailnetHost is one extra Host name the daemon answers to, for
	// requests a `tailscale serve` proxy forwards to the loopback
	// listener. Empty, the default, means loopback only.
	TailnetHost string `yaml:"tailnet_host"`
}

// Overrides are the values a command line supplies, each taking precedence
// over the configuration file when it is not the zero value.
type Overrides struct {
	NotesRoot string
	Port      int
	// MaxWatches is nil when the flag was not given; zero is a request for
	// no budget, the same as the file's own zero.
	MaxWatches *int
	// TailnetHost overrides tailnet_host when it is not empty.
	TailnetHost string
}

// ErrEscapesRoot is returned for a configured path that would leave the
// notes root.
var ErrEscapesRoot = errors.New("must be a relative path inside the notes root")

// ErrBadTailnetHost is returned for a tailnet_host that is not a bare host
// name, optionally with a port.
var ErrBadTailnetHost = errors.New("must be a host name, optionally with a port, and nothing else")

// ErrLoopbackTailnetHost is returned for a tailnet_host naming the loopback
// interface. The extra name carries an authentication rule of its own, so
// letting it collide with the names the daemon already answers to would
// change what loopback means — which M3-R6 says it must not.
var ErrLoopbackTailnetHost = errors.New("must not be a loopback name; loopback is already served and is deliberately left alone")

// cleanTailnetHost normalises and validates the extra Host name the guard
// accepts. What arrives in a Host header is a name and an optional port,
// lower-cased for comparison; anything carrying a scheme, a path, a user or
// whitespace is a configuration mistake worth refusing at startup rather
// than silently never matching.
func cleanTailnetHost(h string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(h))
	if name == "" {
		return "", nil
	}
	if strings.ContainsAny(name, " \t/\\@?#") {
		return "", ErrBadTailnetHost
	}
	host := name
	switch {
	case strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]"):
		// A bare IPv6 literal, bracketed as a Host header requires.
		host = name[1 : len(name)-1]
	case strings.Contains(name, ":"):
		h, p, err := net.SplitHostPort(name)
		if err != nil {
			return "", ErrBadTailnetHost
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", ErrBadTailnetHost
		}
		host = h
	}
	// A fully qualified name with the root label spelled out is the same
	// name; browsers send the Host without it, so the stored form drops
	// it too rather than never matching.
	if trimmed := strings.TrimSuffix(host, "."); trimmed != host {
		name = strings.Replace(name, host, trimmed, 1)
		host = trimmed
	}
	if host == "" {
		return "", ErrBadTailnetHost
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.IsLoopback() {
			return "", ErrLoopbackTailnetHost
		}
		return name, nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", ErrLoopbackTailnetHost
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return "", ErrBadTailnetHost
		}
		for _, r := range label {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
				continue
			}
			return "", ErrBadTailnetHost
		}
	}
	return name, nil
}

// cleanRelative confines a configured path to the notes root lexically.
// Symlinks are the filesystem's business and are refused at write time by
// the confined directory handle the writer holds.
func cleanRelative(p string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(p)))
	sep := string(filepath.Separator)
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, sep) ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+sep) {
		return "", ErrEscapesRoot
	}
	return cleaned, nil
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

// TokenPath returns where the daemon keeps its bearer token:
// $XDG_STATE_HOME/mdn/token, falling back to ~/.local/state/mdn/token. It
// sits beside the state file and is kept at mode 0600.
func TokenPath() string {
	return filepath.Join(xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state")), "mdn", "token")
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
// the file. The returned NotesRoot is absolute and MaxWatches is settled to
// a non-nil value: positive is the per-root watch budget, zero is no budget
// at all.
func (c Config) Resolve(configPath string, over Overrides) (Config, error) {
	if over.NotesRoot != "" {
		c.NotesRoot = over.NotesRoot
	}
	if over.Port != 0 {
		c.Port = over.Port
	}
	if over.MaxWatches != nil {
		c.MaxWatches = over.MaxWatches
	}
	if over.TailnetHost != "" {
		c.TailnetHost = over.TailnetHost
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.MaxWatches == nil {
		n := DefaultMaxWatches
		c.MaxWatches = &n
	}
	if strings.TrimSpace(c.ClipsDir) == "" {
		c.ClipsDir = DefaultClipsDir
	}
	clips, err := cleanRelative(c.ClipsDir)
	if err != nil {
		return c, fmt.Errorf("clips_dir %q: %w", c.ClipsDir, err)
	}
	c.ClipsDir = clips
	tailnet, err := cleanTailnetHost(c.TailnetHost)
	if err != nil {
		return c, fmt.Errorf("tailnet_host %q: %w", c.TailnetHost, err)
	}
	c.TailnetHost = tailnet
	if *c.MaxWatches < 0 {
		return c, fmt.Errorf("max_watches %d is negative; use 0 for no limit", *c.MaxWatches)
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
