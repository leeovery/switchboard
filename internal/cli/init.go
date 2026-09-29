package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
)

// defaultPrefix starts each account's launcher's name unless --prefix says
// otherwise.
const defaultPrefix = "cx"

func newInitCommand(a *app) *cobra.Command {
	var prefix string
	cmd := &cobra.Command{
		Use:   "init zsh",
		Short: "Print the shell integration, for .zshrc to eval",
		Long: `Print the zsh that has the shell start Claude Code through switchboard, for
.zshrc to load with: eval "$(switchboard init zsh)"

It defines claude as a function that starts Claude Code through switchboard
run, and for each account a launcher that starts it pinned to that account,
named the prefix followed by the account's id, such as cxwork. Each has run
read the config --config gives, when it's given. For programs that start
claude themselves, it exports as CLAUDE_CODE_OAUTH_TOKEN the token of the
first account whose token is set as it runs, else the first account's, by
the name of the variable that holds it, so the output holds no token.

Without a config it can read, it defines claude alone, and says so on stderr:
what it prints never fails the eval, nor stands between the shell and claude.`,
		Args: func(_ *cobra.Command, args []string) error {
			switch {
			case len(args) != 1:
				return errors.New("give the shell to print the integration for: zsh")
			case args[0] != "zsh":
				return fmt.Errorf("unsupported shell %q: only zsh is supported", args[0])
			}
			return launch.CheckPrefix(prefix)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.initZsh(cmd.OutOrStdout(), cmd.ErrOrStderr(), prefix)
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", defaultPrefix, "start each account's launcher's name with `PREFIX`")
	return cmd
}

// initZsh prints the zsh integration for this switchboard binary, the config
// --config gives, if any, and the config's accounts. What switchboard can't
// read, it goes without, saying so on errOut: without the config, it prints
// the claude function alone, which run keeps working; without the path of its
// own binary, or of the config given, nothing, leaving claude as it is.
func (a *app) initZsh(out, errOut io.Writer, prefix string) error {
	binary, err := a.Executable()
	if err == nil {
		binary, err = filepath.Abs(binary)
	}
	if err != nil {
		return defineNothing(errOut, "couldn't find its own binary", err)
	}
	configPath, err := a.configFlag()
	if err != nil {
		return defineNothing(errOut, "couldn't find the config given", err)
	}
	var accounts config.Accounts
	if cfg, err := a.loadConfig(); err != nil {
		logger.Warn("printed claude's function alone", "reason", "couldn't read the config", "error", err)
		launch.Warn(errOut, "couldn't read the config", err, "defining claude alone, with no launchers")
	} else {
		accounts = cfg.Accounts
	}
	return launch.Integration{Binary: binary, Config: configPath, Prefix: prefix, Accounts: accounts, Getenv: a.Getenv}.Zsh(out)
}

// defineNothing prints no integration, saying on errOut what switchboard
// couldn't do, and why: the eval goes ahead all the same, leaving claude as
// it is.
func defineNothing(errOut io.Writer, couldnt string, err error) error {
	logger.Warn("printed no shell integration", "reason", couldnt, "error", err)
	launch.Warn(errOut, couldnt, err, "defining nothing")
	return nil
}

// configFlag is the config file --config gives, by its absolute path, for
// what runs switchboard from elsewhere: "" when it isn't given.
func (a *app) configFlag() (string, error) {
	if a.configPath == "" {
		return "", nil
	}
	path, err := filepath.Abs(a.configPath)
	if err != nil {
		return "", fmt.Errorf("find the config file: %w", err)
	}
	return path, nil
}
