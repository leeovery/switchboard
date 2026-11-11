package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/interrupts"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/sessionhook"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setup/screen"
	"github.com/leeovery/switchboard/internal/shell"
	"github.com/leeovery/switchboard/internal/skill"
)

func newSetupCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Walk through setting switchboard up, or what's left of it, full screen",
		Long: `Walk through setting switchboard up, full screen, a page a step: the
accounts, each one's token, made with claude setup-token, and which is the
primary; priming; the service; the claude link and the session hook; and the
skill. Each step says what's done already, and does only what isn't, so
setup is safe to run again.

enter moves on once a step is done. At Done, the last, enter opens the
dashboard, and q leaves without it; q at any step before leaves too, and
setup run again carries on. It needs a terminal: without one, it says which
command takes each step alone.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.setup(cmd)
		},
	}
}

// carryOn is what setup says as it's left before Done.
const carryOn = "run setup again to carry on"

// savedFirst is what setup says of an interrupt held as a token it made was
// saved, which it saved before it ended.
const savedFirst = "interrupted once the token was saved, as it lasts a year on the account"

// setup walks the user at the terminal through setting switchboard up, on
// setup's screen, refusing a switchboard that won't last before it shows
// anything. Once the screen has gone, it prints each step's line, as Done
// says it, as wide as the terminal; then a making the screen gave back,
// which saved nothing, or else, where an interrupt was held as a token was
// saved, says the token was saved first; then leaves as the screen was
// left. From the screen going with a token's making in hand, it holds the
// process's interrupts, through the printing of a making given back, which
// one would otherwise lose, the token lasting a year on the account; or
// else till the screen has gone. One held leaves as an interrupt has it.
func (a *app) setup(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	if !a.TerminalInput(cmd.InOrStdin()) || !a.Terminal(out) {
		return setup.ErrNoTerminal
	}
	s, err := a.newSetup(cmd.Root())
	if err != nil {
		return err
	}
	if _, err := s.Binary(); err != nil {
		return err
	}
	ctx := cmd.Context()
	kept := &interrupts.Kept{Holder: a.Hold}
	cfg := a.setupScreen(s)
	cfg.Holding = func() { kept.Keep(nil) }
	left, err := a.SetupScreen(ctx, cfg, out, a.Environ())
	if errors.Is(err, screen.ErrNotTerminal) {
		return setup.ErrNoTerminal
	}
	var caught bool
	if len(left.Unsaved) == 0 {
		caught = kept.Let()
	}
	printLines(out, dashboard.SetupPrinted(left.Summary, a.terminalWidth(out)))
	switch {
	case len(left.Unsaved) > 0:
		for _, unsaved := range left.Unsaved {
			printLines(out, unsaved.Printed())
		}
		caught = kept.Let()
	case left.Interrupted:
		launch.Notice(cmd.ErrOrStderr(), savedFirst)
	}
	left.Interrupted = left.Interrupted || caught
	return a.leave(ctx, out, left, err)
}

// printLines prints each of lines on out.
func printLines(out io.Writer, lines []string) {
	for _, line := range lines {
		_, _ = fmt.Fprintln(out, line)
	}
}

// leave ends setup as left says, its screen having failed as err says, where
// it did: an interrupt, held as a token made was saved, as an interrupt ends
// a command, with why the screen or the step failed, where either did; the
// screen failing, with why; a step that failed, failing with why; before
// Done, saying to run setup again; at Done by enter, opening the dashboard;
// else, at Done by q, there and then.
func (a *app) leave(ctx context.Context, out io.Writer, left screen.Left, err error) error {
	failed := errors.Join(err, left.Err)
	switch {
	case left.Interrupted && failed != nil:
		return fmt.Errorf("%w: %w", accounts.ErrInterrupted, failed)
	case left.Interrupted:
		return accounts.ErrInterrupted
	case failed != nil:
		return failed
	case left.Ending == screen.Paused:
		_, _ = fmt.Fprintln(out, carryOn)
	case left.Ending == screen.ToDashboard:
		return a.dashboard(ctx, out, dashboardOptions{interval: defaultInterval})
	}
	return nil
}

// setupScreen is setup's screen, of s, drawn as the dashboard is: in the
// pair of themes the dashboard's picker chose, unless NO_COLOR says
// otherwise, at the environment's size until the terminal says its own.
func (a *app) setupScreen(s *setup.Setup) screen.Config {
	cfg := screen.Config{Steps: s, Now: a.Now, Size: screen.Size(a.environSize())}
	if !a.noColour() {
		t := a.themes()
		pair := t.library.Pair(t.kept().Choice)
		cfg.Pair = &pair
	}
	return cfg
}

// newSetup returns setup, making tokens as tokenSetup has it, for the
// service, this switchboard binary, PATH, and the user's login shell; its
// files for the shells loading the completions of root, the command tree;
// and the skill and the session hook where Claude Code reads them. The
// process's interrupts are held as a token's saved, telling no one, as the
// screen's Left says one was held, and the service's install says nothing of
// one it waits for, as setup's screen says it waits.
func (a *app) newSetup(root *cobra.Command) (*setup.Setup, error) {
	s, err := a.tokenSetup()
	if err != nil {
		return nil, err
	}
	cfg, err := a.serviceConfig()
	if err != nil {
		return nil, err
	}
	if s.LaunchAgent, err = service.New(cfg); err != nil {
		return nil, err
	}
	exe, err := a.Executable()
	if err != nil {
		return nil, fmt.Errorf("find this switchboard binary: %w", err)
	}
	if s.SkillFile, err = skill.Path(a.Getenv, a.HomeDir); err != nil {
		return nil, err
	}
	if s.Settings, err = sessionhook.Path(a.Getenv, a.HomeDir); err != nil {
		return nil, err
	}
	if s.Shells, err = a.shellFiles(root); err != nil {
		return nil, err
	}
	s.Install = service.InstallOptions{Executable: exe, Config: a.configPath}
	s.Startable = a.startableIn
	s.Path = a.Getenv("PATH")
	s.Bin = s.Shells.Bin
	s.Login = shell.Login{Environ: a.Environ(), Run: a.LoginShell}
	s.Getenv = a.Getenv
	return s, nil
}
