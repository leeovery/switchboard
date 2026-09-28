// Command switchboard spreads Claude Code sessions across several Claude
// subscriptions.
package main

import (
	"os"

	"github.com/leeovery/switchboard/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	root := cli.NewRootCommand(cli.Deps{
		Version: version,
		Getenv:  os.Getenv,
		HomeDir: os.UserHomeDir,
	})
	os.Exit(cli.Execute(root))
}
