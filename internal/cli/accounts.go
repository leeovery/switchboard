package cli

import (
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
)

func newAccountsCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "accounts",
		Short: "List configured accounts and whether each token is present",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			return writeTable(cmd.OutOrStdout(), accountRows(cfg.Accounts, a.Getenv))
		},
	}
}

func accountRows(accounts []config.Account, getenv func(string) string) [][]string {
	rows := [][]string{{"ID", "LABEL", "TOKEN_ENV", "TOKEN"}}
	for _, acct := range accounts {
		rows = append(rows, []string{acct.ID, acct.Label, acct.TokenEnv, tokenState(acct, getenv)})
	}
	return rows
}

func tokenState(acct config.Account, getenv func(string) string) string {
	if _, ok := acct.Token(getenv); ok {
		return "set"
	}
	return "missing"
}
