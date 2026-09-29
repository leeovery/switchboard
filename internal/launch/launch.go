// Package launch starts Claude Code through switchboard: run, which hands
// this process over to Claude Code, pointed at the router while it's healthy,
// and the shell integration that has the shell start claude through run.
package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

var logger = logs.For("launch")

// AskTimeout bounds each question run asks the router, so a router that's
// stuck holds Claude Code up for a moment at most. What else asks whether the
// router answers gives it as long.
const AskTimeout = 500 * time.Millisecond

// Router is the router, as run asks it how it is and which account it rates
// best: *router.Client is one.
type Router interface {
	Health(ctx context.Context) (router.Health, error)
	Status(ctx context.Context) (status.Document, error)
}

// Launcher starts Claude Code in this process's place.
type Launcher struct {
	// Environ is the environment Claude Code starts from, as os.Environ
	// gives it.
	Environ []string
	// LookPath finds a program on PATH, as exec.LookPath does.
	LookPath func(file string) (string, error)
	// InstallPaths are where Claude Code's installers put it, tried in order
	// when it isn't on PATH.
	InstallPaths []string
	// Exec replaces this process with the program at path, as Exec does.
	Exec func(path string, argv, env []string) error
	// Stderr hears why, when Claude Code starts without the router, or
	// without switchboard.
	Stderr io.Writer
}

// Route is what starting Claude Code on an account's token takes.
type Route struct {
	Config *config.Config
	// Getenv reads the accounts' tokens.
	Getenv func(key string) string
	Router Router
	// Account is the id of the account to pin the session to, or "" to leave
	// it to the router.
	Account string
}

// Run starts Claude Code with args in this process's place, on the token of
// the account pinned, else the one the router rates best, else the first with
// a token. While the router is healthy, Claude Code sends its requests there,
// to the address the router says its proxy listens on, whatever the config
// says, pinned when an account is; otherwise it sends them straight to the
// API, and Stderr hears why. With no account's token to start on, it starts
// Unaided. A pin that can't be kept, to an account not configured or without
// a token, fails: it's the command line's mistake. Run returns only when
// Claude Code couldn't start.
func (l Launcher) Run(ctx context.Context, r Route, args []string) error {
	path, err := l.find()
	if err != nil {
		return err
	}
	pin, err := r.pinned()
	if err != nil {
		return err
	}
	state := r.health(ctx)
	c, ok := r.choose(ctx, pin, state)
	if !ok {
		return l.unaided(path, args, "no account has a token", fmt.Errorf("set %s", strings.Join(r.Config.Accounts.TokenEnvs(), " or ")))
	}
	env := environ(l.Environ)
	if state.healthy() {
		env = env.with(claude.BaseURLEnv, "http://"+state.listen).with(claude.TokenEnv, c.token.Reveal()).pinnedTo(r.Account)
		logger.Info("starting claude", "mode", "routed", "router", state.name, "account", c.account.ID, "chosen", c.why, "claude", path)
	} else {
		env = env.without(claude.BaseURLEnv).with(claude.TokenEnv, c.token.Reveal()).pinnedTo("")
		logger.Warn("starting claude", "mode", "direct", "router", state.name, "reason", state.reason, "account", c.account.ID, "chosen", c.why, "claude", path)
		notice(l.Stderr, "the router "+state.String(), "connecting directly on "+title(c.account))
	}
	return l.exec(path, args, env)
}

// Direct starts Claude Code with args in this process's place, on its own
// login: without the router or an account's token, for what needs the login.
// It returns only when Claude Code couldn't start.
func (l Launcher) Direct(args []string) error {
	path, err := l.find()
	if err != nil {
		return err
	}
	logger.Info("starting claude", "mode", "own login", "claude", path)
	return l.exec(path, args, environ(l.Environ).without(claude.TokenEnv, claude.BaseURLEnv).pinnedTo(""))
}

// Unaided starts Claude Code with args in this process's place as if
// switchboard weren't there, its environment as it is, for when switchboard
// can't take part: switchboard mustn't stand between the user and claude.
// Stderr hears what switchboard couldn't do, and why, as err's first line
// says. It returns only when Claude Code couldn't start.
func (l Launcher) Unaided(args []string, couldnt string, err error) error {
	path, findErr := l.find()
	if findErr != nil {
		return findErr
	}
	return l.unaided(path, args, couldnt, err)
}

func (l Launcher) unaided(path string, args []string, couldnt string, err error) error {
	logger.Warn("starting claude without switchboard", "reason", couldnt, "error", err, "claude", path)
	Warn(l.Stderr, couldnt, err, "starting claude without it")
	return l.exec(path, args, l.Environ)
}

// Warn tells w, in a line, what switchboard couldn't do, why, as err's first
// line says, and what happens instead, such as "switchboard: couldn't read
// the config (no config file at …) — starting claude without it".
func Warn(w io.Writer, couldnt string, err error, instead string) {
	notice(w, couldnt+" ("+firstLine(err)+")", instead)
}

// notice tells w, in a line, what's wrong and what happens instead. What
// happens goes ahead all the same, so a line that can't be written is let go.
func notice(w io.Writer, trouble, instead string) {
	_, _ = fmt.Fprintf(w, "switchboard: %s — %s\n", trouble, instead)
}

// firstLine is the first line of err's text, which is enough for a notice:
// the rest, such as the example a missing config's error shows, is for the
// commands that fail with it.
func firstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return strings.TrimSuffix(strings.TrimSpace(line), ":")
}

// exec starts Claude Code, at path, with args and env, in this process's
// place. Its arguments are never logged: they can hold a prompt.
func (l Launcher) exec(path string, args []string, env environ) error {
	if err := l.Exec(path, append([]string{claude.Command}, args...), env); err != nil {
		return fmt.Errorf("start claude at %s: %w", path, err)
	}
	return nil
}

// find returns where Claude Code is, as claude.Find finds it.
func (l Launcher) find() (string, error) {
	return claude.Find(l.LookPath, l.InstallPaths)
}

// How the router can answer its health check, as the log names it.
const (
	healthy    = "healthy"
	notRunning = "not running"
	unhealthy  = "unhealthy"
)

// routerState is how the router answered its health check.
type routerState struct {
	// name is healthy, notRunning or unhealthy.
	name string
	// reason says why the router is unhealthy.
	reason string
	// listen is where a healthy router's proxy listens.
	listen string
}

func (s routerState) healthy() bool {
	return s.name == healthy
}

// String says what's wrong with the router, as "isn't running" or "is
// unhealthy (…)".
func (s routerState) String() string {
	switch {
	case s.name == notRunning:
		return "isn't running"
	case s.reason != "":
		return "is " + s.name + " (" + s.reason + ")"
	default:
		return "is " + s.name
	}
}

// health asks the router how it is, and where its proxy listens, giving it
// AskTimeout to answer. One that doesn't say where can't be sent requests.
func (r Route) health(ctx context.Context) routerState {
	ctx, cancel := context.WithTimeout(ctx, AskTimeout)
	defer cancel()
	h, err := r.Router.Health(ctx)
	switch {
	case errors.Is(err, router.ErrNotRunning):
		return routerState{name: notRunning}
	case errors.Is(err, context.DeadlineExceeded):
		return routerState{name: unhealthy, reason: "no answer within " + AskTimeout.String()}
	case err != nil:
		return routerState{name: unhealthy, reason: err.Error()}
	case !h.OK:
		return routerState{name: unhealthy, reason: h.Reason}
	case h.Listen == "":
		return routerState{name: unhealthy, reason: "it doesn't say where it listens"}
	}
	return routerState{name: healthy, listen: h.Listen}
}

// choice is the account whose token Claude Code starts on, and why it was
// chosen.
type choice struct {
	account config.Account
	token   config.Token
	why     string
}

// choose picks the account whose token Claude Code starts on: pin's, when
// there's one; else, while the router is healthy, the one it rates best;
// else the first with a token. It reports false when there's none.
func (r Route) choose(ctx context.Context, pin choice, state routerState) (choice, bool) {
	if pin.account.ID != "" {
		return pin, true
	}
	if state.healthy() {
		if c, ok := r.best(ctx); ok {
			return c, true
		}
	}
	return r.first()
}

// pinned is the account the session is pinned to, which must be configured
// and have a token; none when it isn't pinned.
func (r Route) pinned() (choice, error) {
	if r.Account == "" {
		return choice{}, nil
	}
	i := slices.IndexFunc(r.Config.Accounts, func(a config.Account) bool { return a.ID == r.Account })
	if i < 0 {
		return choice{}, fmt.Errorf("there's no account %q: pin %s", r.Account, strings.Join(r.Config.Accounts.IDs(), " or "))
	}
	a := r.Config.Accounts[i]
	token, ok := a.Token(r.Getenv)
	if !ok {
		return choice{}, fmt.Errorf("account %s has no token for Claude Code to start on: set %s", a.ID, a.TokenEnv)
	}
	return choice{account: a, token: token, why: "pinned"}, nil
}

// best is the account the router rates best, when it names one this process
// has the token of.
func (r Route) best(ctx context.Context) (choice, bool) {
	ctx, cancel := context.WithTimeout(ctx, AskTimeout)
	defer cancel()
	doc, err := r.Router.Status(ctx)
	if err != nil {
		logger.Warn("can't ask the router which account is best", "error", err)
		return choice{}, false
	}
	for _, a := range r.Config.Accounts {
		if a.ID != doc.Best {
			continue
		}
		if token, ok := a.Token(r.Getenv); ok {
			return choice{account: a, token: token, why: "the router's best"}, true
		}
	}
	return choice{}, false
}

// first is the first account with a token, reporting false when there's
// none.
func (r Route) first() (choice, bool) {
	for _, a := range r.Config.Accounts {
		if token, ok := a.Token(r.Getenv); ok {
			return choice{account: a, token: token, why: "the first with a token"}, true
		}
	}
	return choice{}, false
}

// title names an account as every command does, such as "work · Work".
func title(a config.Account) string {
	return status.Account{ID: a.ID, Label: a.Label}.Title()
}
