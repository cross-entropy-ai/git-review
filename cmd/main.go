// git-review is a read-only, local pull-request-style branch review tool.
package main

import (
	"os"

	"github.com/cross-entropy-ai/git-review/internal/cli"
)

var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, version))
}
