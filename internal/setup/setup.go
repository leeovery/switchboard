// Package setup decides each step of setting switchboard up: the accounts
// and their tokens, which is the primary, priming, the service, the claude
// link and the session hook, and the skill. Its actions return typed
// outcomes, each in the words its screen draws, and Summary is how setup
// stands, a line a step, as Done says it and setup prints as it leaves; its
// screen, internal/setup/screen, runs them a page a step. Each step says
// what's done already, and does only what isn't, so setup is safe to run
// again.
package setup

import (
	"context"
	"errors"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/interrupts"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setuptoken"
	"github.com/leeovery/switchboard/internal/shell"
)

var logger = logs.For("setup")

// ErrNoTerminal is what setup fails with away from a terminal, as it asks as
// it goes: it says what each step does, and the command that does it alone.
var ErrNoTerminal = errors.New(`setup asks as it goes, so it needs a terminal: run it at one, or take each step with the command that does it alone:
  1. Accounts: switchboard accounts add <id> --label <label>, with --primary for the one the browser and the Claude apps are signed into, and switchboard accounts token <id> for one without a usable token
  2. Priming, optionally: day = "HH:MM-HH:MM" in the config's [prime] table
  3. The service: switchboard service install, or switchboard service restart once it's installed
  4. The claude link: a link named claude to "$(command -v switchboard)" in switchboard's bin directory, ~/.local/share/switchboard/bin, or $XDG_DATA_HOME/switchboard/bin, and that directory first on PATH; setup alone writes switchboard's file for the shell, with its completions, and the line in the shell's startup file that reads it. The session hook: setup alone writes it
  5. The skill: setup alone writes it`)

// errNoAccount is what a step that needs an account fails with where none is
// configured: there's nothing to set up without one.
var errNoAccount = errors.New("no account is configured, so setup stops here: run it again to add one, or switchboard accounts add <id>")

// Setup is what setting switchboard up takes.
type Setup struct {
	// Accounts are the config file and the token files, where accounts are
	// added and tokens saved.
	Accounts accounts.Registry
	// LaunchAgent is the service, which Install says how to run: this
	// switchboard binary, as the path it was run by names it, and the config
	// file given, if one was.
	LaunchAgent *service.Service
	Install     service.InstallOptions
	// Startable fails, saying why, where a router couldn't start from
	// start, as serve's check says: setup checks it before it installs the
	// service, so a router that couldn't start never replaces one that runs.
	Startable func(start service.Start) error
	// Path is PATH, which claude is found on, and InstallPaths where Claude
	// Code's installers put claude.
	Path         string
	InstallPaths []string
	// Bin is switchboard's own directory for PATH, which holds the claude
	// link alone, and Home the home directory, which switchboard's line in
	// the shell's startup file writes as $HOME.
	Bin, Home string
	// Shells are switchboard's files for the shells, which put Bin first on
	// PATH, and Login the user's login shell, asked which claude it runs.
	// Getenv reads $SHELL.
	Shells shell.Files
	Login  LoginShell
	Getenv func(string) string
	// Interrupt returns a context that ends at an interrupt, as
	// signal.NotifyContext does for SIGINT, which it is unless it's set:
	// asking the login shell, or making a token, setup takes the interrupt
	// itself, so as to stop the shell, or setup-token.
	Interrupt func(context.Context) (context.Context, context.CancelFunc)
	// TokenMaker makes an account's token, and Browser and PrivateWindow
	// open its sign-in address, in the browser as it is, and in a private
	// window, as setuptoken's Browser and PrivateWindow do.
	TokenMaker             TokenMaker
	Browser, PrivateWindow setuptoken.Opener
	// Hold holds the process's interrupts from the moment setup-token says
	// it has made a token till the token's saved, as it lasts a year on the
	// account: nil holds them through os/signal. Saving is told of the first
	// interrupt held, on a goroutine of its own, so MakeToken never waits on
	// it, for the caller to say setup saves the token first: nil tells none.
	// It may run after MakeToken has returned, so it keeps its own output in
	// order with whatever the caller says next. Tokened's InterruptHeld says
	// one came, however it's told.
	Hold   interrupts.Holder
	Saving func()
	// SkillFile is where Claude Code reads the skill, and Settings Claude
	// Code's settings file, where the session hook goes.
	SkillFile, Settings string

	// changed is set once setup changes the config or a token.
	changed bool
	// routerWait is how long Standing waits for a router to answer: zero
	// for launch.AskTimeout.
	routerWait time.Duration
}

// Binary is this switchboard binary, which the service runs and the claude
// link leads to, as the service's Binary has it: it fails for a build that
// won't last, such as go run's.
func (s *Setup) Binary() (string, error) {
	return s.LaunchAgent.Binary(s.Install.Executable)
}

// run is one taking of a step, and what it found and did.
type run struct {
	*Setup
	// switchboard is this switchboard binary, which the service runs and the
	// claude link leads to.
	switchboard string
	// cfg is the config as the service step found it.
	cfg *config.Config
	// tell is where the step tells what it says: nil where nothing hears it.
	tell func(Said)
	// claudeCode is Claude Code's own claude, as the claude link step found
	// it.
	claudeCode string
	// made is what the claude link step did to the claude link, and led,
	// where it repaired one, where it led before, from the home.
	made LinkMade
	led  string
	// health is the service's router's, where the service step saw it
	// answer, and swap how far the step's swap of the service got.
	health *router.Health
	swap   service.Swap
}

// wrote is told by each of setup's writes to the config or a token, which
// a router that answers takes up on its own.
func (s *Setup) wrote() {
	s.changed = true
}
