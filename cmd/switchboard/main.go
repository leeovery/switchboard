// Command switchboard spreads Claude Code sessions across several Claude
// subscriptions.
package main

import (
	"os"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/notify"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	root := cli.NewRootCommand(cli.Deps{
		Version:       version,
		Getenv:        os.Getenv,
		Environ:       os.Environ,
		HomeDir:       os.UserHomeDir,
		Now:           time.Now,
		ClaudeVersion: claude.InstalledVersion,
		Watch:         watch.Run,
		Notifier:      notify.NewDesktop(),
	})
	os.Exit(cli.Execute(root))
}
