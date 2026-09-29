// Package setup walks the user at a terminal through setting switchboard up:
// the accounts and their tokens, which is the primary, priming, the service,
// the claude link and the skill, then shows usage. Each step says what's done
// already, and does only what isn't, so setup is safe to run again.
package setup

import (
	"context"
	"errors"
	"io"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/status"
)

var logger = logs.For("setup")

// ErrNoTerminal is what setup fails with away from a terminal, as it asks as
// it goes: it says what each step does, and the command that does it alone.
var ErrNoTerminal = errors.New(`setup asks as it goes, so it needs a terminal: run it at one, or take each step with the command that does it alone:
  1. Accounts: switchboard accounts add <id> --label <label>, with --primary for the one the browser and the Claude apps are signed into, and switchboard accounts token <id> for one without a usable token
  2. Priming, optionally: day = "HH:MM-HH:MM" in the config's [prime] table
  3. The service: switchboard service install, or switchboard service restart once it's installed
  4. The claude link: ln -s "$(command -v switchboard)" <a directory ahead of claude's on PATH>/claude
  5. The skill: setup alone writes it
  6. Every account's usage: switchboard usage`)

// errNoAccount is what setup stops with when there's no account, and the
// user adds none: there's nothing to set up without one.
var errNoAccount = errors.New("no account is configured, so setup stops here: run it again to add one, or switchboard accounts add <id>")

// Setup is what setting switchboard up takes.
type Setup struct {
	// Terminal is where the user answers, and hears what setup finds and
	// does.
	Terminal Terminal
	// Accounts are the config file and the token files, where accounts are
	// added and tokens taken. Setup takes tokens at the terminal, whatever
	// its Input.
	Accounts accounts.Registry
	// Service is the LaunchAgent, which Install says how to run: this
	// switchboard binary, as the path it was run by names it, and the config
	// file given, if one was.
	Service *service.Service
	Install service.InstallOptions
	// Path is PATH, which claude is found on, and the claude link goes on,
	// and InstallPaths where Claude Code's installers put claude.
	Path         string
	InstallPaths []string
	// Skill is where Claude Code reads the skill.
	Skill string
	// Usage shows every account's usage on out, as switchboard usage does.
	Usage func(ctx context.Context, out io.Writer) error
}

// Run takes each step in turn, stopping at one that fails, or when the
// user's input ends, which a run of setup again carries on from. It refuses
// a switchboard binary that won't last, such as go run's, before it asks
// anything: the service and the claude link both run this one.
func (s *Setup) Run(ctx context.Context) error {
	switchboard, err := s.Service.Binary(s.Install.Executable)
	if err != nil {
		return err
	}
	r := &run{Setup: s, switchboard: switchboard, registry: s.registry()}
	r.Terminal.sayf("Setting switchboard up: each step says what's done already, and does only what isn't.")
	steps := []struct {
		title string
		take  func(context.Context) error
	}{
		{"Accounts", r.accounts},
		{"Priming", r.priming},
		{"The service", r.service},
		{"The claude link", r.link},
		{"The skill", r.skill},
		{"Usage", r.usage},
	}
	for i, step := range steps {
		r.Terminal.heading(i+1, step.title)
		if err := step.take(ctx); err != nil {
			return err
		}
	}
	return nil
}

// registry is the accounts, taking a token the user types at the terminal.
func (s *Setup) registry() accounts.Registry {
	r := s.Accounts
	r.Input = accounts.Input{Hidden: s.Terminal.Hidden, Prompt: s.Terminal.Out}
	return r
}

// run is one walk through the steps, and what they've found and changed.
type run struct {
	*Setup
	// switchboard is this switchboard binary, which the service runs and the
	// claude link leads to.
	switchboard string
	registry    accounts.Registry
	// cfg is the config as the steps have left it.
	cfg *config.Config
	// changed is set once a step changes what the router reads as it
	// starts: the config, or a token.
	changed bool
}

func (r *run) usage(ctx context.Context) error {
	return r.Usage(ctx, r.Terminal.Out)
}

// title names an account as every command does, such as "work · Work".
func title(a config.Account) string {
	return status.Account{ID: a.ID, Label: a.Label}.Title()
}
