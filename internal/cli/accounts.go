package cli

import (
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/tokens"
)

func newAccountsCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "accounts",
		Short: "List the accounts, which is the primary, and whether each has a usable token",
		Long: `List the accounts, in the config's order: each one's id and label, whether it's
the primary, and whether its token file, <state dir>/tokens/<id>, holds a
token switchboard can use, or why not, and what would put it right.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			files, err := a.tokens()
			if err != nil {
				return err
			}
			return writeTable(cmd.OutOrStdout(), accountRows(cfg.Accounts, files.Read))
		},
	}
}

func accountRows(accounts config.Accounts, readToken func(id string) (tokens.Token, error)) [][]string {
	rows := [][]string{{"ID", "LABEL", "PRIMARY", "TOKEN"}}
	for _, acct := range accounts {
		rows = append(rows, []string{acct.ID, acct.Label, primary(acct), tokenState(acct.ID, readToken)})
	}
	return rows
}

// primary marks the primary account.
func primary(acct config.Account) string {
	if acct.Primary {
		return "yes"
	}
	return ""
}

// tokenState says the account's token is usable, or why it isn't.
func tokenState(id string, readToken func(id string) (tokens.Token, error)) string {
	if _, err := readToken(id); err != nil {
		return err.Error()
	}
	return "usable"
}
