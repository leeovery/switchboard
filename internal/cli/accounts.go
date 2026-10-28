package cli

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
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

accounts add, edit, token and remove register an account, change its label,
plan, primary or reserve, replace its token, and remove it.`,
		Args: noArgs,
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
	cmd.AddCommand(newAccountsAddCommand(a), newAccountsEditCommand(a), newAccountsTokenCommand(a), newAccountsRemoveCommand(a))
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

// editFlags are accounts edit's flags, as they're given.
type editFlags struct {
	label, plan, reserve string
	primary              bool
}

// editFlagNames are accounts edit's flags, of which it's given one or more.
var editFlagNames = []string{"--label", "--plan", "--primary", "--reserve"}

func newAccountsEditCommand(a *app) *cobra.Command {
	var flags editFlags
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change an account's label, plan, primary or reserve",
		Long: `Change an account's label, its plan, whether it's the primary, or its reserve,
in its [[account]] table in the config. Give one or more of --label, --plan,
--primary and --reserve; what isn't given is left as it is, and when every
value given is what the account has already, nothing is written.

--plan takes none, to leave the plan unknown. --reserve takes a percentage,
to two decimal places at most, such as 5% or 2.5%, of every window, which the
router leaves unused, or 0 for none.
--primary makes the account the primary in place of the one that is; the
sessions running through the router keep the token they started on as Claude
Code's own until they restart: the former primary's, or, where it has no
usable token, the first account's with one, or a pinned session's pinned
account's. --label takes a label as it shows: without the characters drawn
as nothing, but where one makes an emoji or a flag, or shapes a joining
script, and a variation selector after a symbol, non-ASCII punctuation, a
letter outside Latin, Greek and Cyrillic, or in a keycap; each run of
whitespace within it one space; and the whitespace and direction marks at its
edges trimmed. The id, or "", shows the account by its id.

` + editingTheConfig,
		Args: oneAccount,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.editAccount(cmd, args[0], flags)
		},
	}
	plans := prose.ListOr(config.PlanChoices())
	cmd.Flags().StringVar(&flags.label, "label", "", "show the account as `LABEL`: \"\" shows it by its id")
	cmd.Flags().StringVar(&flags.plan, "plan", "", "the `PLAN` its subscription is on: "+plans)
	cmd.Flags().BoolVar(&flags.primary, "primary", false, "make it the primary, the account the browser and the Claude apps use")
	cmd.Flags().StringVar(&flags.reserve, "reserve", "", "leave `PERCENT` of every window unused, such as 5%, or 0 for none")
	return cmd
}

// change is the change the flags ask for, those given as given reports, each
// read as the config reads it.
func (f editFlags) change(given func(name string) bool) (accounts.Change, error) {
	if given("primary") && !f.primary {
		return accounts.Change{}, errors.New("--primary marks this account the primary, and can't be false: the primary moves by giving --primary to another account")
	}
	change := accounts.Change{Primary: f.primary}
	if given("label") {
		change.Label = &f.label
	}
	if given("plan") {
		plan, err := config.ParsePlan(f.plan)
		if err != nil {
			return accounts.Change{}, err
		}
		change.Plan = &plan
	}
	if given("reserve") {
		reserve, err := config.ParseReserve(f.reserve)
		if err != nil {
			return accounts.Change{}, err
		}
		change.Reserve = &reserve
	}
	if !change.Asks() {
		return accounts.Change{}, errors.New("give one or more of " + prose.ListOr(editFlagNames))
	}
	return change, nil
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
new primary's, for a week from when the router takes the change up, while
the new primary has a usable token.

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
		return labelRemedy(err, "leave --label out")
	}
	warnUnchecked(cmd.ErrOrStderr(), taken)
	said := "added " + account.ID
	if account.Primary {
		said += ", the primary"
	}
	if taken.Kept {
		said += ", keeping the token in " + registry.Tokens.Path(account.ID)
	}
	_, err = io.WriteString(cmd.OutOrStdout(), said+"\n")
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
	if err != nil {
		return a.notConfigured(err, id, addIt(id))
	}
	warnUnchecked(cmd.ErrOrStderr(), taken)
	_, err = io.WriteString(cmd.OutOrStdout(), "saved "+id+"'s token\n")
	return err
}

// notConfigured points an error of acting on the account with the given id,
// which no account has, to the account whose id it is in another case, when
// the config has one, else to what otherwise says, when it says anything.
func (a *app) notConfigured(err error, id, otherwise string) error {
	if !errors.Is(err, config.ErrNotConfigured) {
		return err
	}
	if cfg, loadErr := a.loadConfig(); loadErr == nil {
		if other, ok := cfg.Accounts.InAnotherCase(id); ok {
			return fmt.Errorf("%w: %s is; give an id in its own case", err, other.ID)
		}
	}
	if otherwise == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, otherwise)
}

// addIt says how to add the account with the given id.
func addIt(id string) string {
	return "add it with switchboard accounts add " + redact.Text(id)
}

// labelRemedy points an error of a label that would show as nothing to
// giving one, or to none, which none says how to give, to show the account
// by its id.
func labelRemedy(err error, none string) error {
	if errors.Is(err, config.ErrBlankLabel) {
		return fmt.Errorf("%w: give one that shows, or %s to show the account by its id", err, none)
	}
	return err
}

// editAccount makes the change to the account, and says what it changed.
func (a *app) editAccount(cmd *cobra.Command, id string, flags editFlags) error {
	registry, err := a.registry(cmd)
	if err != nil {
		return err
	}
	change, err := flags.change(cmd.Flags().Changed)
	if err != nil {
		if unknown := registry.Configured(id); unknown != nil {
			return a.notConfigured(unknown, id, addIt(id))
		}
		return err
	}
	edited, err := registry.Edit(id, change)
	if err != nil {
		return a.notConfigured(labelRemedy(err, `--label ""`), id, addIt(id))
	}
	said := editedSays(edited)
	if edited.FormerPrimary != "" {
		said += a.runningSessions(edited, registry.Tokens.Read)
		warnNoToken(cmd.ErrOrStderr(), id, registry.Tokens.Read)
	}
	_, err = io.WriteString(cmd.OutOrStdout(), said+"\n")
	return err
}

// runningSessions says, on a line of its own, what becomes of the sessions
// running through the router once an edit has made the account the primary:
// they keep the token they started on as Claude Code's own until they
// restart, which launch would choose from the config the edit read, the
// tokens as they stand now. That's the former primary's, pinned or not;
// where it has no usable token, the first account's with one, or a pinned
// session's pinned account's, said only where another account has a usable
// token, as no session can be pinned elsewhere otherwise; where the first is
// the new primary, the pinned sessions are all there is to say. It says
// nothing where no account has a usable token. Sessions started directly,
// while the router was down, are untouched by the move.
func (a *app) runningSessions(e accounts.Edited, readToken func(id string) (tokens.Token, error)) string {
	own, ok := launch.Route{Config: e.Before, Token: readToken, Pause: a.Pause}.OwnAccount()
	if !ok {
		return ""
	}
	const (
		running = "sessions running through the router"
		outside = ", so what they send outside the conversation, such as artifacts and uploads, still goes out "
	)
	if own.ID == e.FormerPrimary {
		return "\n" + running + " keep " + own.ID + "'s token as Claude Code's own until they restart" + outside + "on " + own.ID
	}
	pinned := otherWithAToken(e.Before.Accounts, own.ID, readToken)
	switch {
	case own.ID == e.Now.ID && !pinned:
		return ""
	case own.ID == e.Now.ID:
		return "\n" + e.FormerPrimary + " has no usable token, so " + running + " pinned to another account " +
			"started on their pinned account's token as Claude Code's own, and keep it until they restart" + outside + "there"
	}
	var orPinned string
	if pinned {
		orPinned = ", or, pinned, on their pinned account's token"
	}
	return "\n" + e.FormerPrimary + " has no usable token, so " + running + " started on " + own.ID +
		"'s, the first account with one" + orPinned + ", as Claude Code's own, and keep it until they restart" + outside + "there"
}

// otherWithAToken reports whether an account of among, but the one with the
// given id, has a usable token, as readToken finds it.
func otherWithAToken(among config.Accounts, id string, readToken func(id string) (tokens.Token, error)) bool {
	return slices.ContainsFunc(among, func(a config.Account) bool {
		if a.ID == id {
			return false
		}
		_, err := readToken(a.ID)
		return err == nil
	})
}

// warnNoToken tells w when the account with the given id, the primary now,
// has no usable token, as readToken finds it: new sessions take another
// account's as Claude Code's own until it has one.
func warnNoToken(w io.Writer, primary string, readToken func(id string) (tokens.Token, error)) {
	if _, err := readToken(primary); err != nil {
		launch.Warn(w, primary+", the primary now, has no usable token", err,
			"new sessions take another account's as Claude Code's own until it has one: switchboard accounts token "+primary)
	}
}

// editedSays says what an edit changed of the account, each value as it is
// now, then as it was, or that it changed nothing.
func editedSays(e accounts.Edited) string {
	if !e.Changed() {
		return e.Now.ID + " has every value given already, so nothing changed"
	}
	var changed []string
	if e.Was.Label != e.Now.Label {
		changed = append(changed, labelChanged(e))
	}
	if e.Was.Plan != e.Now.Plan {
		changed = append(changed, nowAndWas("plan", planSaid(e.Now.Plan), planSaid(e.Was.Plan)))
	}
	if e.FormerPrimary != "" {
		changed = append(changed, "the primary in place of "+e.FormerPrimary)
	}
	if e.Was.Reserve != e.Now.Reserve {
		changed = append(changed, nowAndWas("reserve", reserveSaid(e.Now.Reserve), reserveSaid(e.Was.Reserve)))
	}
	return "edited " + e.Now.ID + ": " + strings.Join(changed, ", ")
}

// nowAndWas says a value an edit changed, as "plan max20x (was max5x)".
func nowAndWas(name, now, was string) string {
	return name + " " + now + " (was " + was + ")"
}

// labelChanged says how an edit changed the account's label: as it is now
// and as it was, quoted, or none where the config writes none; and where the
// two show the same, as a label written untidily does once tidied, that it
// was written untidily.
func labelChanged(e accounts.Edited) string {
	now, was := labelSaid(e.Now.Label, e.NowLabelled), labelSaid(e.Was.Label, e.WasLabelled)
	switch {
	case now == was:
		return "label " + now + " (was written untidily)"
	case !e.NowLabelled:
		now += ", shown by its id"
	}
	return nowAndWas("label", now, was)
}

// labelSaid says a label, quoted, where the config writes it, else none.
func labelSaid(label string, written bool) string {
	if !written {
		return "none"
	}
	return prose.Quoted(label)
}

// planSaid says a plan, or none.
func planSaid(plan string) string {
	return cmp.Or(plan, config.PlanNone)
}

// reserveSaid says a reserve, as the percentage it was set as, or none.
func reserveSaid(reserve float64) string {
	if reserve == 0 {
		return "none"
	}
	return prose.PercentSet(reserve)
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
		return a.notConfigured(err, id, "")
	}
	said := "removed " + id
	switch {
	case removed.LinkedToGone:
		said += ", and its token file, a link, with the name it led to, " + removed.LinkedTo + ": the file's kept under its other name"
	case removed.LinkedTo != "":
		said += ", and its token file, a link: the file it led to, " + removed.LinkedTo + ", is left as it is"
	case removed.File:
		said += ", and its token file"
	}
	if removed.Primary != "" {
		said += "\n" + primaryNow(removed.Primary, id, registry.Tokens.Read)
	}
	_, err = io.WriteString(cmd.OutOrStdout(), said+"\n")
	return err
}

// primaryNow says which account is the primary now the one with the given
// id, the primary, is removed, and what becomes of the sessions running on
// the removed one's token: they stay routed, as the new primary's, while it
// has a usable token, as readToken finds it.
func primaryNow(primary, removed string, readToken func(id string) (tokens.Token, error)) string {
	if _, err := readToken(primary); err != nil {
		return primary + " is the primary now, but has no usable token: the sessions running on " + removed +
			"'s token aren't routed until it has one (switchboard accounts token " + primary + ")"
	}
	return primary + " is the primary now; the sessions running on " + removed + "'s token stay routed, as " +
		primary + "'s, for a week from when the router takes this up"
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
		Now:        a.Now,
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
		rows = append(rows, []string{acct.ID, prose.Clean(acct.Label), primary(acct), tokenState(acct.ID, readToken)})
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
