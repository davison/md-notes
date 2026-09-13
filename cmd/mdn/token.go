package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/davison/md-notes/internal/config"
	"github.com/davison/md-notes/internal/token"
)

func runToken(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdn token", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tokenFile := fs.String("token-file", config.TokenPath(), "file holding the daemon's bearer token")
	rotate := fs.Bool("rotate", false, "replace the token with a new one")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "mdn token: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	var value string
	var err error
	if *rotate {
		value, err = token.Rotate(*tokenFile)
	} else {
		// Printing the token before the daemon has ever run creates it, so
		// the extension can be set up first and the daemon started after.
		value, _, err = token.Load(*tokenFile)
	}
	if err != nil {
		fmt.Fprintln(stderr, "mdn token:", err)
		return 1
	}
	fmt.Fprintln(stdout, value)
	return 0
}
