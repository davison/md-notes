package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/davison/md-notes/internal/config"
	"github.com/davison/md-notes/internal/roots"
)

// openBrowser launches the user's browser. Replaced in tests.
var openBrowser = func(url string) error {
	return exec.Command("xdg-open", url).Start()
}

func runOpen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdn open", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", config.Path(), "configuration file")
	port := fs.Int("port", 0, "daemon port (overrides port in the config file)")
	noBrowser := fs.Bool("no-browser", false, "print the URL instead of opening a browser")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: mdn open [--port N] [--no-browser] DIR")
		return 2
	}
	dir, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "mdn open:", err)
		return 1
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "mdn open:", err)
		return 1
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if cfg.Port == 0 {
		cfg.Port = config.DefaultPort
	}
	base := fmt.Sprintf("http://localhost:%d", cfg.Port)

	url, err := registerRoot(base, dir)
	if err != nil {
		fmt.Fprintln(stderr, "mdn open:", err)
		return 1
	}
	if *noBrowser {
		fmt.Fprintln(stdout, url)
		return 0
	}
	if err := openBrowser(url); err != nil {
		fmt.Fprintf(stderr, "mdn open: could not open a browser (%v); the URL is %s\n", err, url)
		return 1
	}
	fmt.Fprintln(stdout, url)
	return 0
}

// registerRoot asks the daemon at base to serve dir and returns the URL of
// the root's page.
func registerRoot(base, dir string) (string, error) {
	body, _ := json.Marshal(map[string]string{"path": dir})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(base+"/api/roots", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("daemon not reachable at %s: start it with `mdn serve` (or `systemctl --user start mdn`)", base)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return "", fmt.Errorf("daemon refused %s: %s", dir, e.Error)
	}
	var root roots.Root
	if err := json.NewDecoder(resp.Body).Decode(&root); err != nil {
		return "", fmt.Errorf("unexpected response from daemon: %w", err)
	}
	return fmt.Sprintf("%s/r/%s/", base, root.Slug), nil
}
