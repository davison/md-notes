package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/davison/md-notes/internal/config"
	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/server"
	"github.com/davison/md-notes/internal/token"
	"github.com/davison/md-notes/ui"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdn serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", config.Path(), "configuration file")
	root := fs.String("root", "", "notes root (overrides notes_root in the config file)")
	port := fs.Int("port", 0, "loopback port (overrides port in the config file)")
	statePath := fs.String("state", config.StatePath(), "file that remembers roots added with mdn open")
	tokenFile := fs.String("token-file", config.TokenPath(), "file holding the bearer token clients present (created on first start)")
	maxWatches := fs.Int("max-watches", config.DefaultMaxWatches, "directories watched per root for live update; 0 for no limit (overrides max_watches in the config file)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "mdn serve: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	cfg, err = cfg.Resolve(*configPath, overrides(fs, *root, *port, maxWatches))
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	logger := log.New(stderr, "", log.LstdFlags)
	reg, err := roots.New(cfg.NotesRoot, *statePath, logger.Printf)
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	secret, created, err := token.Load(*tokenFile)
	if err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	if created {
		logger.Printf("generated a bearer token in %s; print it with `mdn token`", *tokenFile)
	}

	srv := server.New(reg, cfg.Port, ui.FS(), logger,
		server.WithWatchBudget(*cfg.MaxWatches),
		server.WithToken(token.NewStore(*tokenFile, secret)),
		server.WithClipsDir(cfg.ClipsDir),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	return 0
}

// overrides collects the command-line values that beat the configuration
// file. A flag's zero value is a real request — --max-watches 0 asks for no
// watch budget — so what counts is whether the flag was given at all, which
// only the flag set knows.
func overrides(fs *flag.FlagSet, root string, port int, maxWatches *int) config.Overrides {
	over := config.Overrides{NotesRoot: root, Port: port}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-watches" {
			over.MaxWatches = maxWatches
		}
	})
	return over
}
