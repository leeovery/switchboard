// Package cli is switchboard's command line. Commands stay thin: they parse
// flags, call the packages that do the work, and print.
package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
)

// Deps is what the commands take from the process around them. main passes the
// real ones; tests pass their own.
type Deps struct {
	Version string
	Getenv  func(key string) string
	HomeDir func() (string, error)
}

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
	root.AddCommand(newAccountsCommand(a))
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
