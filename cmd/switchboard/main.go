// Command switchboard spreads Claude Code sessions across several Claude
// subscriptions.
package main

import (
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/service"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	root := cli.NewRootCommand(cli.Deps{
		Version:       version,
		Getenv:        os.Getenv,
		Environ:       os.Environ,
		HomeDir:       os.UserHomeDir,
		Executable:    os.Executable,
		Now:           time.Now,
		ClaudeVersion: claude.InstalledVersion,
		Watch:         watch.Run,
		Notifier:      notify.NewDesktop(),
		LookPath:      exec.LookPath,
		Exec:          launch.Exec,
		Launchctl:     service.Launchctl,
		GOOS:          runtime.GOOS,
		UID:           os.Getuid(),
	})
	os.Exit(cli.Execute(root))
}
