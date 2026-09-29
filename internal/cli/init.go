package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

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
output holds no token.`,
		ValidArgs: []string{"zsh"},
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
			return a.initZsh(cmd.OutOrStdout(), prefix)
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", defaultPrefix, "start each account's launcher's name with `PREFIX`")
	return cmd
}

// initZsh prints the zsh integration for this switchboard binary and the
// config's accounts.
func (a *app) initZsh(out io.Writer, prefix string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	exe, err := a.Executable()
	if err == nil {
		exe, err = filepath.Abs(exe)
	}
	if err != nil {
		return fmt.Errorf("find this switchboard binary: %w", err)
	}
	return launch.Zsh(out, exe, prefix, cfg.Accounts)
}
