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
named the prefix followed by the account's id, such as cxwork. For programs
that start claude themselves, it exports the first account's token as
CLAUDE_CODE_OAUTH_TOKEN, by the name of the variable that holds it, so the
output holds no token.

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

// initZsh prints the zsh integration for this switchboard binary and the
// config's accounts. What switchboard can't read, it goes without, saying so
// on errOut: without the config, it prints the claude function alone, which
// run keeps working; without its own binary's path, nothing, leaving claude
// as it is.
func (a *app) initZsh(out, errOut io.Writer, prefix string) error {
	exe, err := a.Executable()
	if err == nil {
		exe, err = filepath.Abs(exe)
	}
	if err != nil {
		logger.Warn("printed no shell integration", "reason", "couldn't find this switchboard binary", "error", err)
		launch.Warn(errOut, "couldn't find its own binary", err, "defining nothing")
		return nil
	}
	var accounts []config.Account
	if cfg, err := a.loadConfig(); err != nil {
		logger.Warn("printed claude's function alone", "reason", "couldn't read the config", "error", err)
		launch.Warn(errOut, "couldn't read the config", err, "defining claude alone, with no launchers")
	} else {
		accounts = cfg.Accounts
	}
	return launch.Zsh(out, exe, prefix, accounts)
}
