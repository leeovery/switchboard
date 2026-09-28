// Package cli is switchboard's command line. Commands stay thin: they parse
// flags, call the packages that do the work, and print.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Deps is what the commands take from the process around them. main passes the
// real ones; tests pass their own.
type Deps struct {
	Version string
	Getenv  func(key string) string
	// Environ lists the whole environment, as os.Environ does, for reading
	// which colours the terminal shows.
	Environ func() []string
	HomeDir func() (string, error)
	Now     func() time.Time
	// ClaudeVersion returns the Claude Code version that probes claim to be.
	// Every read asks it, as a watch can outlive the version it started with.
	ClaudeVersion func() string
	// Watch shows the dashboard full screen on out until the user quits, as
	// watch.Run does.
	Watch func(ctx context.Context, cfg watch.Config, out io.Writer, environ []string) error
}

// policy is Claude's say in scoring accounts.
var policy = score.Policy{Shared: claude.SharedWindows, Perishable: claude.PerishableWindow}

// NewRootCommand builds the switchboard command tree.
func NewRootCommand(deps Deps) *cobra.Command {
	a := &app{Deps: deps}
	root := &cobra.Command{
		Use:     "switchboard",
		Short:   "Spread Claude Code sessions across several Claude subscriptions",
		Version: deps.Version,
		// Flags and arguments have parsed by the time this runs, so usage still
		// follows a mistyped command line but not a failure after it.
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			cmd.SilenceUsage = true
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", "",
		"config file (default $SWITCHBOARD_CONFIG, else $XDG_CONFIG_HOME/switchboard/config.toml, else ~/.config/switchboard/config.toml)")
	root.AddCommand(newAccountsCommand(a), newStatusCommand(a), newUsageCommand(a))
	return root
}

// Execute runs a command tree from NewRootCommand and returns the process's
// exit status. Cobra has already printed any error.
func Execute(root *cobra.Command) int {
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

// app is what every command shares: the dependencies and the global flags.
type app struct {
	Deps
	configPath string
}

// loadConfig loads the config file named by --config, else the one config.Path finds.
func (a *app) loadConfig() (*config.Config, error) {
	path, err := a.configFile()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		example := strings.TrimSuffix(config.Example, "\n")
		return nil, fmt.Errorf("no config file at %s\n\nCreate one like this:\n\n%s", path, example)
	}
	return cfg, err
}

func (a *app) configFile() (string, error) {
	if a.configPath != "" {
		return a.configPath, nil
	}
	return config.Path(a.Getenv, a.HomeDir)
}

// collect loads the config and probes every account in it.
func (a *app) collect(ctx context.Context) (status.Document, error) {
	source, err := a.source()
	if err != nil {
		return status.Document{}, err
	}
	return source.Fetch(ctx)
}

// source loads the config and returns what probes every account in it. Every
// command that reports usage reads it here, so they all read it the same way.
func (a *app) source() (probeSource, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return probeSource{}, err
	}
	return probeSource{deps: a.Deps, upstream: cfg.Upstream, accounts: cfg.Accounts}, nil
}

// probeSource reads the status document by probing accounts.
type probeSource struct {
	deps     Deps
	upstream string
	accounts []config.Account
}

// Fetch probes every account, claiming the Claude Code version installed now:
// a watch can outlive the one it started with. It never fails: an account that
// can't be read says why in the document.
func (s probeSource) Fetch(ctx context.Context) (status.Document, error) {
	collector := status.Collector{
		Prober: &claude.Prober{Upstream: s.upstream, Version: s.deps.ClaudeVersion()},
		Policy: policy,
		Getenv: s.deps.Getenv,
		Now:    s.deps.Now,
	}
	return collector.Collect(ctx, s.accounts), nil
}
