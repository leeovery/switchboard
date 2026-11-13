package cli

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

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
token switchboard can use, or why not, and what would put it right. Where the
router runs, a token it found is another subscription's shows as wrong token.
Once there's a second pool, each account's pool shows too, by its label, and
the primary is its pool's.

accounts add, edit, token and remove register an account, change its label,
plan, pool, primary or reserve, replace its token, and remove it.`,
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
			return writeTable(cmd.OutOrStdout(), accountRows(a.standing(cmd.Context(), cfg), files.Read, a.foreignTokens(cmd.Context())))
		},
	}
	cmd.AddCommand(newAccountsAddCommand(a), newAccountsEditCommand(a), newAccountsTokenCommand(a), newAccountsRemoveCommand(a))
	return cmd
}

func newAccountsAddCommand(a *app) *cobra.Command {
	var account config.NewAccount
	var answer accounts.Answer
	var pool string
	cmd := &cobra.Command{
		Use:   "add <label>",
		Short: "Register an account, taking its token",
		Long: `Register an account, shown as the label given, taking its token: add an
[[account]] table for it to the config, making the config if there's none,
and its token file, <state dir>/tokens/<id>. Every account has a label, and
no two configured accounts the same, ignoring case.

At a terminal, it asks for the token, which doesn't show as it's pasted: make
one with claude setup-token, run while signed in to that subscription.
Otherwise it reads the token from stdin. The token is checked with the API
before anything's saved, and nothing is where the API refuses it, or doesn't
answer for it, so its subscription isn't known: try again then.

The organization the API names the token in decides the account's id, which
switchboard makes: a subscription this Mac has never had gets a new id; one
it had, removed since, comes back under its id, so its history carries on;
and one configured already is refused, the other accounts' tokens checked
then to know theirs: give it a new token with accounts token. Where the
organization doesn't settle it, as where the API names none, or the token may
be an account removed before switchboard made ids, it asks whether it's a new
account or one of those, at a terminal; otherwise it says the choices, and
--new or --returning answers.

It goes in the pool --pool names, by its id; without it, the only pool, where
there's one, else the one fenced to no directory; where every pool is fenced,
it asks for --pool. --primary makes it its pool's primary.

` + editingTheConfig,
		Args: func(cmd *cobra.Command, args []string) error {
			switch returning := cmd.Flags().Changed("returning"); {
			case answer.New && returning:
				return errors.New("--new answers that it's a new account, so it takes no --returning")
			case returning && answer.ID == "":
				return errors.New("--returning takes the id of the account this Mac had")
			}
			return oneLabel(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			account.Label = args[0]
			var err error
			if account.Pool, err = a.accountsPool(pool); err != nil {
				return err
			}
			return a.addAccount(cmd, account, answer)
		},
	}
	cmd.Flags().BoolVar(&account.Primary, "primary", false, "make it its pool's primary, the account the browser and the Claude apps use")
	addPoolFlag(cmd, &pool, "add it to")
	cmd.Flags().BoolVar(&answer.New, "new", false, "where it asks, answer that it's a new account")
	cmd.Flags().StringVar(&answer.ID, "returning", "", "where it asks, answer that it's the account `ID` this Mac had")
	return cmd
}

// editFlags are accounts edit's flags, as they're given.
type editFlags struct {
	label, plan, reserve, pool string
	primary                    bool
}

// editFlagNames are accounts edit's flags, of which it's given one or more,
// as said where it's given none: --pool among them only where the config
// has pools to move an account between, two or more.
func (a *app) editFlagNames() []string {
	names := []string{"--label", "--plan", "--primary", "--reserve"}
	if cfg, err := a.loadConfig(); err == nil && len(cfg.Pools) > 1 {
		names = append(names, "--pool")
	}
	return names
}

func newAccountsEditCommand(a *app) *cobra.Command {
	var flags editFlags
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change an account's label, plan, pool, primary or reserve",
		Long: `Change an account's label, its plan, its pool, whether it's its pool's
primary, or its reserve, in its [[account]] table in the config. Give one or
more of --label, --plan, --pool, --primary and --reserve; what isn't given is
left as it is, and when every value given is what the account has already,
nothing is written.

--pool moves the account into the pool with the id given, its pool's primary
there only with --primary; the pool it leaves takes its next account as its
primary, where it was the primary, or has none left, where it was the last.
The sessions the router sent to it in the pool it left move off it on their
next request; those that started on its token as Claude Code's own, as the
primary's, are refused once it has left, and must be resumed.

--plan takes none, to leave the plan unknown. --reserve takes a percentage,
to two decimal places at most, such as 5% or 2.5%, of every window, which the
router leaves unused, or 0 for none.
--primary makes the account its pool's primary in place of the one that is; the
sessions running through the router keep the token they started on as Claude
Code's own until they restart: the former primary's, or, where it has no
usable token, the first account's with one, or a pinned session's pinned
account's. --label takes a label as it shows: without the characters drawn
as nothing, but where one makes an emoji or a flag, or shapes a joining
script, and a variation selector after a symbol, non-ASCII punctuation, a
letter outside Latin, Greek and Cyrillic, or in a keycap; each run of
whitespace within it one space; and the whitespace and direction marks at its
edges trimmed. Every account has a label, and no two configured accounts the
same, ignoring case.

` + editingTheConfig,
		Args: oneAccount,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.idGiven(args[0]); err != nil {
				return err
			}
			return a.editAccount(cmd, args[0], flags)
		},
	}
	plans := prose.ListOr(config.PlanChoices())
	cmd.Flags().StringVar(&flags.label, "label", "", "show the account as `LABEL`")
	cmd.Flags().StringVar(&flags.plan, "plan", "", "the `PLAN` its subscription is on: "+plans)
	cmd.Flags().BoolVar(&flags.primary, "primary", false, "make it its pool's primary, the account the browser and the Claude apps use")
	cmd.Flags().StringVar(&flags.reserve, "reserve", "", "leave `PERCENT` of every window unused, such as 5%, or 0 for none")
	cmd.Flags().StringVar(&flags.pool, "pool", "", "move it into pool `ID`")
	return cmd
}

// change is the change the flags ask for, those given as given reports, each
// read as the config reads it, failing where they ask for none, saying to
// give one of those named.
func (f editFlags) change(given func(name string) bool, named []string) (accounts.Change, error) {
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
	if f.pool != "" {
		change.Pool = &f.pool
	}
	if !change.Asks() {
		return accounts.Change{}, errors.New("give one or more of " + prose.ListOr(named))
	}
	return change, nil
}

func newAccountsTokenCommand(a *app) *cobra.Command {
	var paste bool
	cmd := &cobra.Command{
		Use:   "token <id>",
		Short: "Replace an account's token, making it with claude setup-token",
		Long: `Replace an account's token, in its token file, <state dir>/tokens/<id>.

At a terminal, it makes the token as setup does, saying each step as it
goes: it runs claude setup-token, which opens its sign-in page in the
browser, as it is for the primary, as the browser is signed in to it, and in
a private window for any other account, where the browser can open one, or
else shows the address to open in one. Once you approve it there, the token
is checked with the API and saved, a year from today. It isn't saved where
the API refuses it, or where its subscription is another account's already,
as the organization the API names it in says: the other accounts' tokens are
checked first, to know theirs. An interrupt before the token's made stops
claude setup-token, saving nothing. Once it's made, as it lasts a year on the
account, it's saved whatever stops the command, saved unchecked where the
API doesn't answer, or shown to copy and paste where it can't be.

--paste asks for the token instead, which doesn't show as it's pasted: make
one with claude setup-token, run while signed in to that subscription. Away
from a terminal, it reads the token from stdin. A token given is checked
with the API before it's saved: one the API refuses isn't, nor one whose
subscription is another account's already, the other accounts' tokens checked
then to know theirs; and one the API doesn't answer for is, with a warning.`,
		Args: oneAccount,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.idGiven(args[0]); err != nil {
				return err
			}
			if paste || !a.TerminalInput(cmd.InOrStdin()) || !a.Terminal(cmd.OutOrStdout()) {
				return a.replaceToken(cmd, args[0])
			}
			return a.makeToken(cmd, args[0])
		},
	}
	cmd.Flags().BoolVar(&paste, "paste", false, "take a token made with claude setup-token, pasted, in place of making one")
	return cmd
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
			if err := a.idGiven(args[0]); err != nil {
				return err
			}
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

// oneLabel accepts one account's label alone.
func oneLabel(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("give the account's label alone")
	}
	return nil
}

// addAccount registers the account, taking its token, answering, where the
// token's organization doesn't settle which account it is, as given says,
// else as the user does, asked, as asking has it; and says what it did, and,
// where given says otherwise than the organization settled it, that the
// answer went unused.
func (a *app) addAccount(cmd *cobra.Command, account config.NewAccount, given accounts.Answer) error {
	registry, err := a.registry(cmd)
	if err != nil {
		return err
	}
	asked := false
	ask := a.asking(cmd, given)
	added, err := registry.Add(cmd.Context(), account, func(q accounts.Question) (accounts.Answer, error) {
		asked = true
		return ask(q)
	})
	if err != nil {
		return addRefused(err, a.Now())
	}
	if !asked && contrary(given, added) {
		launch.Notice(cmd.ErrOrStderr(), unusedAnswer(given, added))
	}
	said := "added " + prose.Quoted(account.Label) + " as " + added.ID + a.inPoolOf(cmd.Context(), account.Pool)
	if added.Returning {
		said += ", an account this Mac had, so its history carries on"
	}
	if account.Primary {
		said += ", the primary"
	}
	_, err = io.WriteString(cmd.OutOrStdout(), said+"\n")
	return err
}

// accountsPool returns the id of the pool an account added goes in, or one
// edited moves into, as askedPool has it of the config's pools, named or
// not, failing as it does, saying --pool names one: the one named as it's
// given where the config has no pools, or can't be loaded, as where there's
// none yet, for adding or editing to say why.
func (a *app) accountsPool(named string) (string, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		logger.Debug("can't load the config, so the pool named goes as it's given", "error", err)
		return named, nil
	}
	if len(cfg.Pools) == 0 {
		return named, nil
	}
	p, err := askedPool(cfg.Pools, named)
	return p.ID, poolFlagNeeded(err)
}

// inPoolOf names the pool with the given id, as " in pool Acme", where
// there's more than one, as poolsStanding counts them and poolNamed names it:
// "" where there's one or none, and for "", no pool, as of an account added
// to a config naming none.
func (a *app) inPoolOf(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	pools := a.poolsStanding(ctx)
	return inPoolNamed(pools, poolLabel(pools, id))
}

// contrary reports whether given answers otherwise than the account was
// added, as added says: --new for one this Mac had, or --returning for a new
// account or another this Mac had. None given answers nothing.
func contrary(given accounts.Answer, added accounts.Added) bool {
	switch {
	case given.New:
		return added.Returning
	case given.ID != "":
		return !added.Returning || !strings.EqualFold(given.ID, added.ID)
	}
	return false
}

// unusedAnswer says the answer given went unused, as the token's
// organization settled which account it is, as added: a new account, or one
// this Mac had.
func unusedAnswer(given accounts.Answer, added accounts.Added) string {
	flag := "--new"
	if !given.New {
		flag = "--returning " + given.ID
	}
	settled := "a new account"
	if added.Returning {
		settled = added.ID + ", an account this Mac had"
	}
	return redact.Text(flag) + " went unused: the token's organization says it's " + settled
}

// addRefused is err, adding an account, with what to do about it: give a
// configured account a new token; give a label that shows; or answer with
// one of the accounts the token may be, as choicesSaid says them at now.
func addRefused(err error, now time.Time) error {
	if configured, ok := errors.AsType[*accounts.ConfiguredError](err); ok {
		return fmt.Errorf("%w: give it a new token with switchboard accounts token %s", err, configured.ID)
	}
	if notAChoice, ok := errors.AsType[*accounts.NotAChoiceError](err); ok {
		return fmt.Errorf("%w: answer with one of\n%s", err, choicesSaid(notAChoice.Question, now))
	}
	return labelRemedy(err)
}

// asking answers the question of which account a token is, where its
// organization doesn't settle it: as given says, where it says; at a
// terminal, as the user answers, asked there, as askWhich has it; else not,
// saying the choices and the flags that answer.
func (a *app) asking(cmd *cobra.Command, given accounts.Answer) accounts.Ask {
	return func(q accounts.Question) (accounts.Answer, error) {
		switch {
		case given != (accounts.Answer{}):
			return given, nil
		case a.TerminalInput(cmd.InOrStdin()):
			return askWhich(cmd.InOrStdin(), cmd.ErrOrStderr(), q, a.Now())
		}
		return accounts.Answer{}, fmt.Errorf("%w: answer with one of\n%s", &accounts.QuestionError{Question: q}, choicesSaid(q, a.Now()))
	}
}

// choicesSaid says the answers to q, a line each, set in, as the flags that
// give them: --new, then --returning and each account's id, as choiceSaid
// says it at now.
func choicesSaid(q accounts.Question, now time.Time) string {
	rows := [][]string{{"--new", "a new account"}}
	for _, c := range q.Choices {
		rows = append(rows, []string{"--returning " + c.ID, choiceSaid(c, now)})
	}
	var table strings.Builder
	_ = writeTable(&table, rows)
	var lines []string
	for line := range strings.Lines(table.String()) {
		lines = append(lines, "  "+strings.TrimSuffix(line, "\n"))
	}
	return strings.Join(lines, "\n")
}

// choiceSaid says an account a token may be, at now: its label, quoted,
// else its id, and the day it was last seen, in now's time zone, where it was
// removed.
func choiceSaid(c identity.Entry, now time.Time) string {
	said := prose.Quoted(cmp.Or(c.Label, c.ID))
	if !c.Removed.IsZero() {
		said += ", last seen " + prose.DayMonthYear(c.Removed.In(now.Location()))
	}
	return said
}

// askWhich asks the user at the terminal, on prompt, which account the token
// is, as q offers, numbering its answers, each as choiceSaid says it at now,
// and reads their answer from in, a line, asking again till it's one of the
// numbers. It fails where in ends first.
func askWhich(in io.Reader, prompt io.Writer, q accounts.Question, now time.Time) (accounts.Answer, error) {
	launch.Notice(prompt, (&accounts.QuestionError{Question: q}).Error())
	_, _ = fmt.Fprintln(prompt, "  1. a new account")
	for i, c := range q.Choices {
		_, _ = fmt.Fprintf(prompt, "  %d. %s\n", i+2, choiceSaid(c, now))
	}
	lines := bufio.NewScanner(in)
	for {
		_, _ = fmt.Fprintf(prompt, "Which is it (1-%d)? ", len(q.Choices)+1)
		if !lines.Scan() {
			return accounts.Answer{}, fmt.Errorf("read the answer: %w", cmp.Or(lines.Err(), io.ErrUnexpectedEOF))
		}
		n, err := strconv.Atoi(strings.TrimSpace(lines.Text()))
		switch {
		case err != nil || n < 1 || n > len(q.Choices)+1:
			continue
		case n == 1:
			return accounts.Answer{New: true}, nil
		}
		return accounts.Answer{ID: q.Choices[n-2].ID}, nil
	}
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
	if _, other := errors.AsType[*accounts.OtherSubscriptionError](err); other {
		return fmt.Errorf("%w, with switchboard accounts add <label>", err)
	}
	if err != nil {
		return a.notConfigured(err, id, findTheID)
	}
	warnUnchecked(cmd.ErrOrStderr(), taken.Unchecked)
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

// findTheID says where an account's id is found, for one acted on by an id
// no account has.
const findTheID = "switchboard accounts lists each account's id"

// labelRemedy points an error of a label that would show as nothing to
// giving one that shows.
func labelRemedy(err error) error {
	if errors.Is(err, config.ErrBlankLabel) {
		return fmt.Errorf("%w: give one that shows", err)
	}
	return err
}

// editAccount makes the change to the account, and says what it changed.
func (a *app) editAccount(cmd *cobra.Command, id string, flags editFlags) error {
	registry, err := a.registry(cmd)
	if err != nil {
		return err
	}
	change, err := a.editChange(cmd, flags)
	if err != nil {
		if unknown := registry.Configured(id); unknown != nil {
			return a.notConfigured(unknown, id, findTheID)
		}
		return err
	}
	edited, err := registry.Edit(id, change)
	if err != nil {
		return a.notConfigured(labelRemedy(err), id, findTheID)
	}
	said := editedSays(edited) + a.leftSays(edited, registry.Tokens.Read)
	if edited.FormerPrimary != "" {
		said += a.runningSessions(edited, registry.Tokens.Read)
		warnNoToken(cmd.ErrOrStderr(), id, registry.Tokens.Read)
	}
	_, err = io.WriteString(cmd.OutOrStdout(), said+"\n")
	return err
}

// editChange is the change flags ask for, as cmd was given them, as their
// change has it, the pool it moves the account into as accountsPool has it.
func (a *app) editChange(cmd *cobra.Command, flags editFlags) (accounts.Change, error) {
	change, err := flags.change(cmd.Flags().Changed, a.editFlagNames())
	if err != nil || change.Pool == nil {
		return change, err
	}
	pool, err := a.accountsPool(*change.Pool)
	change.Pool = &pool
	return change, err
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
// while the router was down, are untouched by the move. Where there are
// pools, it tells of the sessions of the account's pool alone, and of its
// accounts, as no session starts on another pool's.
func (a *app) runningSessions(e accounts.Edited, readToken func(id string) (tokens.Token, error)) string {
	before := e.Before
	if len(before.Pools) > 0 {
		inPool := *before
		inPool.Accounts = before.Accounts.InPool(e.Now.Pool)
		before = &inPool
	}
	own, ok := launch.Route{Config: before, Token: readToken, Pause: a.Pause}.OwnAccount()
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
	pinned := otherWithAToken(before.Accounts, own.ID, readToken)
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
	moved := !strings.EqualFold(e.Was.Pool, e.Now.Pool)
	if moved {
		changed = append(changed, nowAndWas("pool", poolLabel(e.Before.Pools, e.Now.Pool), poolLabel(e.Before.Pools, e.Was.Pool)))
	}
	if e.KeptPrimary != "" {
		changed = append(changed, "marked "+e.KeptPrimary+" the primary there, which it stays")
	}
	switch {
	case e.Was.Plan != e.Now.Plan:
		changed = append(changed, nowAndWas("plan", planSaid(e.Now.Plan), planSaid(e.Was.Plan)))
	case e.Unaliased != "":
		changed = append(changed, nowAndWas("plan", planSaid(e.Now.Plan), "written "+e.Unaliased))
	}
	switch {
	case e.FormerPrimary != "":
		changed = append(changed, "the primary in place of "+e.FormerPrimary)
	case e.Marked && moved:
		changed = append(changed, "marked the primary there, as the first in it")
	case e.Marked:
		changed = append(changed, "marked the primary, which it was as the first with none marked")
	}
	if e.Was.Reserve != e.Now.Reserve {
		changed = append(changed, nowAndWas("reserve", reserveSaid(e.Now.Reserve), reserveSaid(e.Was.Reserve)))
	}
	return "edited " + e.Now.ID + ": " + strings.Join(changed, ", ")
}

// poolLabel names an account's pool, of pools, by its id, as
// status.PoolLabels labels it, cleaned: an account's pool is configured, so
// no accounts record is read.
func poolLabel(pools config.Pools, id string) string {
	return prose.Clean(status.PoolLabels(pools, func() identity.Record { return identity.Record{} })(id))
}

// nowAndWas says a value an edit changed, as "plan ultra (was max)".
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
	said += emptiedSays(removed.Left)
	_, err = io.WriteString(cmd.OutOrStdout(), said+"\n")
	return err
}

// emptiedSays says, on a line of its own, that taking an account out of its
// pool left the pool no accounts, where left says it did, and how to give it
// one: "" where it didn't.
func emptiedSays(left accounts.Left) string {
	if left.Emptied.ID == "" {
		return ""
	}
	return "\n" + prose.Pool(left.Emptied.Label) + " has no accounts now: its sessions aren't routed until one is added to it " +
		"(switchboard accounts add <label> --pool " + prose.Clean(left.Emptied.ID) + ")"
}

// leftSays says, on lines of their own, what moving the account an edit
// moved into another pool did to the pool it left, as e's Left tells it,
// the tokens as readToken finds them: where it was the primary there, which
// account is now; and, where its token is usable, so sessions may have
// started on it, that those running on it are refused once it has left, and
// which account's token they start on resumed, as a launch in the pool
// would choose it now (launch.Route's OwnAccount). Or that the pool has no
// accounts now. It's "" where the edit moved it from no pool's primary.
func (a *app) leftSays(e accounts.Edited, readToken func(id string) (tokens.Token, error)) string {
	if e.Left.Primary == "" {
		return emptiedSays(e.Left)
	}
	pool := prose.Pool(poolLabel(e.Before.Pools, e.Was.Pool))
	said := "\n" + e.Left.Primary + " is " + pool + "'s primary now"
	if _, err := readToken(e.Now.ID); err != nil {
		return said
	}
	said += "; the sessions running on " + e.Now.ID + "'s token as Claude Code's own are refused now it has left the pool"
	own, ok := launch.Route{Config: leftConfig(e), Token: readToken, Pause: a.Pause}.OwnAccount()
	switch {
	case !ok:
		return said + ", and no account left there has a usable token to resume them on (switchboard accounts token " + e.Left.Primary + ")"
	case own.ID == e.Left.Primary:
		return said + ": resume them, and they start on " + own.ID + "'s"
	}
	return said + ": resume them, and they start on " + own.ID + "'s, the first account there with a usable token, as " +
		e.Left.Primary + " has none"
}

// leftConfig is the config of the pool the account an edit moved left, as
// the edit leaves it: its accounts but the one moved, the primary Left
// names marked.
func leftConfig(e accounts.Edited) *config.Config {
	var left config.Accounts
	for _, acct := range e.Before.Accounts.InPool(e.Was.Pool) {
		if acct.ID != e.Now.ID {
			acct.Primary = acct.ID == e.Left.Primary
			left = append(left, acct)
		}
	}
	return &config.Config{Accounts: left}
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

// registry returns the accounts as accountFiles has them, taking a token the
// user gives at cmd's stdin, then comparing its organization with the other
// accounts', as setup's OtherOrganizations finds them.
func (a *app) registry(cmd *cobra.Command) (accounts.Registry, error) {
	registry, err := a.accountFiles()
	if err != nil {
		return accounts.Registry{}, err
	}
	registry.Input = a.tokenInput(cmd.InOrStdin(), cmd.ErrOrStderr())
	registry.Others = (&setup.Setup{Accounts: registry}).OtherOrganizations
	return registry, nil
}

// accountFiles returns the accounts as the config file, the token files and
// the accounts record hold them, checking a token with the API as a probe
// does.
func (a *app) accountFiles() (accounts.Registry, error) {
	path, err := a.configFile()
	if err != nil {
		return accounts.Registry{}, err
	}
	files, err := a.tokens()
	if err != nil {
		return accounts.Registry{}, err
	}
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return accounts.Registry{}, err
	}
	return accounts.Registry{ConfigPath: path, HomeDir: a.HomeDir, Tokens: files, Record: identity.NewFile(dir), Prober: a.prober, Now: a.Now}, nil
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
// taken, as unchecked says why, which was saved all the same.
func warnUnchecked(w io.Writer, unchecked error) {
	if unchecked != nil {
		launch.Warn(w, "couldn't check the token with the API", unchecked, "saved it all the same")
	}
}

// foreignTokens reports, of an account by its id, whether the router found
// its token another subscription's, as the moment of its pool's status
// document judges it, every pool's asked once, as statusDocument asks for
// one: of none where the router doesn't give them, so the token files alone
// tell then.
func (a *app) foreignTokens(ctx context.Context) func(id string) bool {
	source := a.routerSource()
	source.patience = launch.Patience{Answer: cmp.Or(a.Patience.Answer, launch.AskTimeout)}
	docs, err := source.everyPool(ctx)
	if err != nil {
		logger.Debug("can't read the router's documents; telling each token by its file alone", "error", err)
		return func(string) bool { return false }
	}
	moments := make([]status.Moment, len(docs))
	for i, doc := range docs {
		moments[i] = doc.At(a.Now(), claude.Policy())
	}
	return func(id string) bool {
		return slices.ContainsFunc(moments, func(m status.Moment) bool {
			acct, ok := m.Account(id)
			return ok && acct.TokenForeign
		})
	}
}

// accountRows are the rows of accounts' table: each account cfg configures,
// cfg as its pools stand, as standing has it, with its pool's label once
// there's a second pool.
func accountRows(cfg *config.Config, readToken func(id string) (tokens.Token, error), foreign func(id string) bool) [][]string {
	pooled := len(cfg.Pools) > 1
	head := []string{"ID", "LABEL", "PRIMARY", "TOKEN"}
	if pooled {
		head = slices.Insert(head, 2, "POOL")
	}
	rows := [][]string{head}
	for _, acct := range cfg.Accounts {
		row := []string{acct.ID, prose.Clean(acct.Label), primary(acct), tokenState(acct.ID, readToken, foreign)}
		if pooled {
			row = slices.Insert(row, 2, poolLabel(cfg.Pools, acct.Pool))
		}
		rows = append(rows, row)
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

// tokenState says the account's token is usable, or why it isn't: another
// subscription's, as foreign says, else as its file says.
func tokenState(id string, readToken func(id string) (tokens.Token, error), foreign func(id string) bool) string {
	if foreign(id) {
		return prose.ForeignTokenShort
	}
	if _, err := readToken(id); err != nil {
		return err.Error()
	}
	return "usable"
}
