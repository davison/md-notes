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
	"github.com/davison/md-notes/ui"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdn serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", config.Path(), "configuration file")
	root := fs.String("root", "", "notes root (overrides notes_root in the config file)")
	port := fs.Int("port", 0, "loopback port (overrides port in the config file)")
	statePath := fs.String("state", config.StatePath(), "file that remembers roots added with mdn open")
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
	cfg, err = cfg.Resolve(*configPath, *root, *port)
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

	srv := server.New(reg, cfg.Port, ui.FS(), logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.ListenAndServe(ctx); err != nil {
		fmt.Fprintln(stderr, "mdn serve:", err)
		return 1
	}
	return 0
}
