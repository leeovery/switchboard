// Command switchboard spreads Claude Code sessions across several Claude
// subscriptions.
package main

import (
	"os"
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
		Args:          os.Args,
		HomeDir:       os.UserHomeDir,
		Executable:    os.Executable,
		Now:           time.Now,
		ClaudeVersion: claude.InstalledVersion(os.Getenv, os.UserHomeDir, os.Executable),
		Watch:         watch.Run,
		Notifier:      notify.NewDesktop(),
		Exec:          launch.Exec,
		Launchctl:     service.Launchctl,
		Hidden:        cli.HiddenInput,
		GOOS:          runtime.GOOS,
		UID:           os.Getuid(),
		PID:           os.Getpid(),
		Pause:         time.Sleep,
		ZoneFile:      "/etc/localtime",
	})
	root.SetArgs(cli.Args(os.Args))
	os.Exit(cli.Execute(root))
}
