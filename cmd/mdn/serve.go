package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/davison/md-notes/internal/config"
	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/server"
	"github.com/davison/md-notes/internal/token"
	"github.com/davison/md-notes/ui"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	srv, code := prepareServe(args, stderr)
	if srv == nil {
		return code
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	return 0
}

// prepareServe is everything `mdn serve` does before it binds the port:
// flags, configuration, the registry and the token, each refusal printed
// to stderr with the exit code it earns. A nil server is a refusal.
func prepareServe(args []string, stderr io.Writer) (*server.Server, int) {
	fs := flag.NewFlagSet("mdn serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", config.Path(), "configuration file")
	// --root is repeatable, every one kept in order: a flag.String here
	// kept the last and dropped the rest without a word (#162).
	var rootFlags []string
	fs.Func("root", "a folder to serve; repeat for several, the first being the notes root (any --root replaces notes_root in the config file)", func(v string) error {
		rootFlags = append(rootFlags, v)
		return nil
	})
	port := fs.Int("port", 0, "loopback port (overrides port in the config file)")
	statePath := fs.String("state", config.StatePath(), "file that remembers roots added with mdn open")
	tokenFile := fs.String("token-file", config.TokenPath(), "file holding the bearer token clients present (created on first start)")
	maxWatches := fs.Int("max-watches", config.DefaultMaxWatches, "directories watched per root for live update; 0 for no limit (overrides max_watches in the config file)")
	tailnetHost := fs.String("tailnet-host", "", "extra Host name to answer to, for a tailscale serve proxy; every request under it must authenticate (overrides tailnet_host in the config file)")
	if err := fs.Parse(args); err != nil {
		return nil, 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "mdn serve: unexpected argument %q\n", fs.Arg(0))
		return nil, 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return nil, 1
	}
	cfg, err = cfg.Resolve(*configPath, overrides(fs, rootFlags, *port, *tailnetHost, maxWatches))
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return nil, 1
	}
	logger := log.New(stderr, "", log.LstdFlags)
	reg, err := roots.NewConfigured(cfg.Roots, *statePath, logger.Printf)
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return nil, 1
	}
	secret, created, err := token.Open(*tokenFile)
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return nil, 1
	}
	if created {
		logger.Print(tokenCreatedLine(*tokenFile, given(fs, "token-file")))
	}

	srv := server.New(reg, cfg.Port, ui.FS(), logger,
		server.WithWatchBudget(*cfg.MaxWatches),
		server.WithToken(secret),
		server.WithClipsDir(cfg.ClipsDir),
		server.WithTailnetHost(cfg.TailnetHost),
	)
	if cfg.TailnetHost != "" {
		logger.Printf("also answering to https://%s; every request under that name must present the token or a session cookie", cfg.TailnetHost)
	}
	return srv, 0
}

// tokenCreatedLine is the first-start hint. With --token-file given, a bare
// `mdn token` would read — and, if absent, mint — the token under the
// default state directory rather than the one this daemon uses, so the
// command it names carries the same flag (#155).
func tokenCreatedLine(path string, flagGiven bool) string {
	cmd := "mdn token"
	if flagGiven {
		cmd = "mdn token --token-file " + shellQuote(path)
	}
	return fmt.Sprintf("generated a bearer token in %s; print it with `%s`", path, cmd)
}

// shellQuote quotes a path for a command line a reader will paste, and
// leaves an ordinary one as it is.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+,:@%=", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// given reports whether the flag called name was set on the command line.
func given(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// overrides collects the command-line values that beat the configuration
// file. A flag's zero value is a real request — --max-watches 0 asks for no
// watch budget — so what counts is whether the flag was given at all, which
// only the flag set knows.
func overrides(fs *flag.FlagSet, roots []string, port int, tailnetHost string, maxWatches *int) config.Overrides {
	over := config.Overrides{Roots: roots, Port: port, TailnetHost: tailnetHost}
	if given(fs, "max-watches") {
		over.MaxWatches = maxWatches
	}
	return over
}
