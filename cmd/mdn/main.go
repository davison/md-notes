// Command mdn is the md-notes daemon and its command line client.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: mdn <command> [flags]

commands:
  serve     run the daemon against the configured notes root
  open DIR  register DIR with the running daemon and open it in the browser
            (--no-browser prints the URL instead)
  token     print the daemon's bearer token (--rotate replaces it)
  version   print the version
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "open":
		return runOpen(args[1:], stdout, stderr)
	case "token":
		return runToken(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "mdn: unknown command %q\n%s", args[0], usage)
		return 2
	}
}
