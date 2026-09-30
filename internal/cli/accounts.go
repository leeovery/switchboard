package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// takingAToken says, in the help of the commands that take a token, how they
// take it.
const takingAToken = `At a terminal, it asks for the token, which doesn't show as it's pasted: make
one with claude setup-token, run while signed in to that subscription.
Otherwise it reads the token from stdin. The token is checked with the API
before it's saved: one the API refuses isn't, and one the API doesn't answer
for is, with a warning.`

// editingTheConfig says, in the help of the commands that edit the config,
// how they edit it.
const editingTheConfig = `The config is edited as text, keeping its comments and layout, and written
through a link, to where it leads.`

func newAccountsCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List the accounts, which is the primary, and whether each has a usable token",
		Long: `List the accounts, in the config's order: each one's id and label, whether it's
the primary, and whether its token file, <state dir>/tokens/<id>, holds a
token switchboard can use, or why not, and what would put it right.

accounts add, token and remove register an account, replace its token, and
remove it.`,
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
	cmd.AddCommand(newAccountsAddCommand(a), newAccountsTokenCommand(a), newAccountsRemoveCommand(a))
	return cmd
}

func newAccountsAddCommand(a *app) *cobra.Command {
	var account config.NewAccount
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Register an account, taking its token",
		Long: `Register an account: add an [[account]] table for it to the config, making the
config if there's none, and when its token file, <state dir>/tokens/<id>,
holds no usable token, take one. The id names the account for good, and its
token file: letters, digits, '-' and '_'.

` + takingAToken + `

` + editingTheConfig,
		Args: oneAccount,
		RunE: func(cmd *cobra.Command, args []string) error {
			account.ID = args[0]
			return a.addAccount(cmd, account)
		},
	}
	cmd.Flags().StringVar(&account.Label, "label", "", "show the account as `LABEL` (default the id)")
	cmd.Flags().BoolVar(&account.Primary, "primary", false, "make it the primary, the account the browser and the Claude apps use")
	return cmd
}

func newAccountsTokenCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "token <id>",
		Short: "Replace an account's token",
		Long:  "Replace an account's token, in its token file, <state dir>/tokens/<id>.\n\n" + takingAToken,
		Args:  oneAccount,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.replaceToken(cmd, args[0])
		},
	}
}

func newAccountsRemoveCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an account, and its token file",
		Long: `Remove an account: its [[account]] table from the config, with the comments
directly above it, and its token file, <state dir>/tokens/<id>. A token file
that's a link goes, but not the file it leads to, which is left as it is. The
only account can't be removed, as a config needs one.

Removing the primary makes the account marked primary = true the primary, or
the first when none is, and says which. The sessions running on the removed
account's token, as every session holds the primary's, stay routed, as the
primary's, for a week.

` + editingTheConfig,
		Args: oneAccount,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.removeAccount(cmd, args[0])
		},
	}
}

// oneAccount accepts one account's id alone.
func oneAccount(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("give one account's id")
	}
	return nil
}

// addAccount registers the account, taking its token when it has no usable
// one, and says what it did.
func (a *app) addAccount(cmd *cobra.Command, account config.NewAccount) error {
	registry, err := a.registry(cmd)
	if err != nil {
		return err
	}
	taken, err := registry.Add(cmd.Context(), account)
	if errors.Is(err, config.ErrConfigured) {
		return fmt.Errorf("%w: replace its token with switchboard accounts token %s", err, account.ID)
	}
	if err != nil {
		return err
	}
	warnUnchecked(cmd.ErrOrStderr(), taken)
	said := "added " + account.ID
	if account.Primary {
		said += ", the primary"
	}
	if taken.Kept {
		said += ", keeping the token in " + registry.Tokens.Path(account.ID)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), said)
	return err
}

// replaceToken replaces the account's token with the one the user gives, and
// says so.
func (a *app) replaceToken(cmd *cobra.Command, id string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	registry, err := a.registry(cmd)
	if err != nil {
		return err
	}
	taken, err := registry.SetToken(cmd.Context(), cfg, id)
	if errors.Is(err, config.ErrNotConfigured) {
		return fmt.Errorf("%w: add it with switchboard accounts add %s", err, redact.Text(id))
	}
	if err != nil {
		return err
	}
	warnUnchecked(cmd.ErrOrStderr(), taken)
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "saved %s's token\n", id)
	return err
}

// removeAccount removes the account and its token file, and says what it
// removed.
func (a *app) removeAccount(cmd *cobra.Command, id string) error {
	registry, err := a.registry(cmd)
	if err != nil {
		return err
	}
	removed, err := registry.Remove(id)
	if err != nil {
		return err
	}
	said := "removed " + id
	switch {
	case removed.LinkedTo != "":
		said += ", and its token file, a link: the file it led to, " + removed.LinkedTo + ", is left as it is"
	case removed.File:
		said += ", and its token file"
	}
	if removed.Primary != "" {
		said += "\n" + removed.Primary + " is the primary now; the sessions running on " + id + "'s token stay routed, as " + removed.Primary + "'s, for a week"
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), said)
	return err
}

// registry returns the accounts as the config file and the token files hold
// them, taking a token the user gives at cmd's stdin and checking it with the
// API as a probe does.
func (a *app) registry(cmd *cobra.Command) (accounts.Registry, error) {
	path, err := a.configFile()
	if err != nil {
		return accounts.Registry{}, err
	}
	files, err := a.tokens()
	if err != nil {
		return accounts.Registry{}, err
	}
	return accounts.Registry{
		ConfigPath: path,
		Tokens:     files,
		Prober:     a.prober,
		Input:      a.tokenInput(cmd.InOrStdin(), cmd.ErrOrStderr()),
	}, nil
}

// prober checks a token with the API at upstream, as a probe does.
func (a *app) prober(upstream string) accounts.Prober {
	return &claude.Prober{Upstream: upstream, Version: a.ClaudeVersion()}
}

// tokenInput is where the user gives a token: typed, unseen, at the terminal
// stdin is, having been asked at stderr, else stdin, read to its end.
func (a *app) tokenInput(stdin io.Reader, stderr io.Writer) accounts.Input {
	if hidden, ok := a.Hidden(stdin); ok {
		return accounts.Input{Hidden: hidden, Prompt: stderr}
	}
	return accounts.Input{Piped: stdin}
}

// warnUnchecked tells w when the API didn't answer whether it takes the token
// taken, which was saved all the same.
func warnUnchecked(w io.Writer, taken accounts.Taken) {
	if taken.Unchecked != nil {
		launch.Warn(w, "couldn't check the token with the API", taken.Unchecked, "saved it all the same")
	}
}

func accountRows(configured config.Accounts, readToken func(id string) (tokens.Token, error)) [][]string {
	rows := [][]string{{"ID", "LABEL", "PRIMARY", "TOKEN"}}
	for _, acct := range configured {
		rows = append(rows, []string{acct.ID, status.Clean(acct.Label), primary(acct), tokenState(acct.ID, readToken)})
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
