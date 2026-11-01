package cli

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/interrupts"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

// What accounts token says of a token's making, a line a step, in the token
// card's words where they fit a line.
const (
	makingCompares    = "Checking the other accounts' tokens, to tell their subscriptions from this one's."
	makingStarts      = "Starting claude setup-token: its sign-in page opens in the browser once it's ready."
	makingOpened      = "Its sign-in page is open in the browser."
	makingWaits       = "Waiting for you to approve it in the browser: the token's saved automatically, so there's nothing to copy."
	makingMade        = "claude setup-token created the token: an interrupt now waits until it's checked, and what came of it is said."
	makingChecks      = "Checking the token with the API, then saving it, a year from today."
	makingInterrupted = "interrupted: saving the token first, as it lasts a year on the account"
)

// errTokenNotSaved is what accounts token fails with where the token it made
// isn't saved, having said why.
var errTokenNotSaved = errors.New("the token isn't saved")

// tokenSetup is setup as far as making and saving an account's token takes
// it: the accounts, as the config file and the token files hold them, each
// token checked with the API as a probe does; tokens made with claude
// setup-token, looking where Claude Code installs, or with the TokenMaker
// given, their sign-ins opened by the CLI's openers; and the process's
// interrupts taken as Interrupt and Hold take them.
func (a *app) tokenSetup() (*setup.Setup, error) {
	path, err := a.configFile()
	if err != nil {
		return nil, err
	}
	files, err := a.tokens()
	if err != nil {
		return nil, err
	}
	// Without a home directory, only the install paths outside it are tried.
	home, _ := a.HomeDir()
	installPaths := claude.InstallPaths(home)
	maker := a.TokenMaker
	if maker == nil {
		maker = setuptoken.Maker{Environ: a.Environ(), InstallPaths: installPaths, Executable: a.Executable}
	}
	return &setup.Setup{
		Accounts:      accounts.Registry{ConfigPath: path, Tokens: files, Prober: a.prober, Now: a.Now},
		InstallPaths:  installPaths,
		Home:          home,
		TokenMaker:    maker,
		Browser:       a.Browser,
		PrivateWindow: a.PrivateWindow,
		Interrupt:     a.Interrupt,
		Hold:          a.Hold,
	}, nil
}

// makeToken makes the account's token with claude setup-token, as setup
// does, saying each step as it comes, once the other accounts' tokens are
// checked, so the duplicate check knows their organizations; then says how
// it ended. An interrupt before the token's made stops setup-token, saving
// nothing; after, it's held till the token's saved, or printed to copy, and
// the command ends as interrupted.
func (a *app) makeToken(cmd *cobra.Command, id string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	s, err := a.tokenSetup()
	if err != nil {
		return err
	}
	if err := s.Accounts.Configured(id); err != nil {
		return a.notConfigured(err, id, addIt(id))
	}
	said := &makingSaid{out: cmd.OutOrStdout(), errOut: cmd.ErrOrStderr(), hold: a.Hold}
	if len(cfg.Accounts) > 1 {
		said.lines(makingCompares)
	}
	ctx := cmd.Context()
	organizations, err := s.OtherOrganizations(ctx, id)
	if err != nil {
		return err
	}
	s.Saving = said.interrupted
	said.lines(makingStarts)
	tokened, err := s.MakeToken(ctx, setup.TokenFor{Account: config.NewAccount{ID: id}, Organizations: organizations}, said.step)
	if err != nil {
		said.let()
		return a.notConfigured(err, id, addIt(id))
	}
	if tokened.InterruptHeld {
		said.interrupted()
	}
	ended := said.ended(cmd, id, tokened)
	if !said.let() && !tokened.InterruptHeld {
		return ended
	}
	said.interrupted()
	cmd.SilenceErrors = true
	return heldBack(ended)
}

// heldBack is the interrupt held through a making, with why the making
// failed, as err says, where it did.
func heldBack(err error) error {
	if err != nil {
		return fmt.Errorf("%w: %w", accounts.ErrInterrupted, err)
	}
	return accounts.ErrInterrupted
}

// makingSaid says a token's making as it goes, a line at a time, in the
// order it's told, from whichever goroutine tells it: each step, and how it
// ended, on out, and an interrupt held, once, on errOut. From the token
// made till its end is said, it holds the process's interrupts with hold,
// as setup does through the save, so the token isn't lost to one as it's
// printed to copy.
type makingSaid struct {
	out, errOut io.Writer
	hold        interrupts.Holder
	mu          sync.Mutex
	held        *interrupts.Held
	told        sync.Once
}

// lines says each of lines.
func (m *makingSaid) lines(lines ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	printLines(m.out, lines)
}

// step says the making has got to at, holding the interrupts from the token
// made.
func (m *makingSaid) step(at setup.Making) {
	if at.Step == setup.TokenMade {
		m.keep()
	}
	m.lines(stepSaid(at)...)
}

// keep holds the interrupts, where they aren't held already, telling
// interrupted of the first.
func (m *makingSaid) keep() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held == nil {
		m.held = interrupts.Hold(m.hold)
		m.held.Watch(m.interrupted)
	}
}

// let lets the interrupts go, where they're held, reporting whether one came
// while they were.
func (m *makingSaid) let() bool {
	m.mu.Lock()
	held := m.held
	m.mu.Unlock()
	return held != nil && held.End()
}

// interrupted says, the first time it's told, that an interrupt came once
// the token was made, which is saved first. Told again, as by the making as
// it ends, it waits for the first to have said so.
func (m *makingSaid) interrupted() {
	m.told.Do(func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		launch.Notice(m.errOut, makingInterrupted)
	})
}

// ended says how the making of the account's token ended, failing where it
// saved nothing: saved, as SetToken's are said, warning where the API didn't
// answer for it; interrupted before it was made; cancelled; or else, saved
// nothing, as setup prints it, what to do now being to run the command
// again, or to paste the token.
func (m *makingSaid) ended(cmd *cobra.Command, id string, t setup.Tokened) error {
	switch t.Result {
	case setup.TokenSaved:
		if t.Checked.Unchecked {
			m.mu.Lock()
			warnUnchecked(m.errOut, errors.New(t.Checked.Why))
			m.mu.Unlock()
		}
		m.lines("saved " + id + "'s token")
		return nil
	case setup.TokenInterrupted:
		return fmt.Errorf("%w: claude setup-token is stopped, and nothing is saved", accounts.ErrInterrupted)
	case setup.TokenCancelled:
		return fmt.Errorf("make the token: %w", cmd.Context().Err())
	}
	again := "switchboard accounts token " + id
	m.lines(t.PrintedFor(again, again+" --paste")...)
	cmd.SilenceErrors = true
	if t.Why == "" {
		return errTokenNotSaved
	}
	return fmt.Errorf("%w: %s", errTokenNotSaved, t.Why)
}

// stepSaid is what's said of the making having got to at: the sign-in
// address ready, as it opened, in setup's words where it's more than open,
// a private window's others sharing its sign-in, and one not opened, made
// safe to show, set in by two spaces, to open by hand; the wait for its
// approval; the token made; and its check.
func stepSaid(at setup.Making) []string {
	switch at.Step {
	case setup.AddressReady:
		return addressSaid(at)
	case setup.AwaitingApproval:
		return []string{makingWaits}
	case setup.TokenMade:
		return []string{makingMade}
	case setup.CheckingToken:
		return []string{makingChecks}
	}
	return nil
}

// addressSaid is what's said of the sign-in address, ready, as at has it.
func addressSaid(at setup.Making) []string {
	switch at.Opened {
	case setuptoken.OpenedInBrowser:
		return []string{makingOpened}
	case setuptoken.OpenedPrivately:
		return []string{prose.Clean(at.Opening()), setup.SharedSignIn}
	}
	return []string{prose.Clean(at.Opening()), "  " + prose.Visible(at.Address.Reveal())}
}
