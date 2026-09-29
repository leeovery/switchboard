// Package launch starts Claude Code through switchboard: run, which hands
// this process over to Claude Code, pointed at the router while it's healthy,
// and the shell integration that has the shell start claude through run.
package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// askTimeout bounds each question run asks the router, so a router that's
// stuck holds Claude Code up for a moment at most.
const askTimeout = 500 * time.Millisecond

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
	// Stderr hears why, when Claude Code starts without the router.
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
// pinned when an account is; otherwise it sends them straight to the API, and
// Stderr hears why. Run returns only when Claude Code couldn't start.
func (l Launcher) Run(ctx context.Context, r Route, args []string) error {
	path, err := l.find()
	if err != nil {
		return err
	}
	state := r.health(ctx)
	c, err := r.choose(ctx, state)
	if err != nil {
		return err
	}
	env := environ(l.Environ)
	if state.healthy() {
		env = env.with(claude.BaseURLEnv, "http://"+r.Config.Listen).with(claude.TokenEnv, c.token.Reveal()).pinnedTo(r.Account)
		logger.Info("starting claude", "mode", "routed", "router", state.name, "account", c.account.ID, "chosen", c.why, "claude", path)
	} else {
		env = env.without(claude.BaseURLEnv).with(claude.TokenEnv, c.token.Reveal()).pinnedTo("")
		logger.Warn("starting claude", "mode", "direct", "router", state.name, "reason", state.reason, "account", c.account.ID, "chosen", c.why, "claude", path)
		if _, err := fmt.Fprintf(l.Stderr, "switchboard: the router %s — connecting directly on %s\n", state, title(c.account)); err != nil {
			return err
		}
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

// exec starts Claude Code, at path, with args and env, in this process's
// place. Its arguments are never logged: they can hold a prompt.
func (l Launcher) exec(path string, args []string, env environ) error {
	if err := l.Exec(path, append([]string{claude.Command}, args...), env); err != nil {
		return fmt.Errorf("start claude at %s: %w", path, err)
	}
	return nil
}

// find returns where Claude Code is: on PATH, else the first of its install
// paths that holds a program.
func (l Launcher) find() (string, error) {
	if path, err := l.LookPath(claude.Command); err == nil {
		return path, nil
	}
	for _, path := range l.InstallPaths {
		if isProgram(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("can't find claude: it isn't on PATH, nor at %s", strings.Join(l.InstallPaths, ", "))
}

// isProgram reports whether path holds a file that can be run.
func isProgram(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
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

// health asks the router how it is, giving it askTimeout to answer.
func (r Route) health(ctx context.Context) routerState {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	h, err := r.Router.Health(ctx)
	switch {
	case errors.Is(err, router.ErrNotRunning):
		return routerState{name: notRunning}
	case errors.Is(err, context.DeadlineExceeded):
		return routerState{name: unhealthy, reason: "no answer within " + askTimeout.String()}
	case err != nil:
		return routerState{name: unhealthy, reason: err.Error()}
	case !h.OK:
		return routerState{name: unhealthy, reason: h.Reason}
	}
	return routerState{name: healthy}
}

// choice is the account whose token Claude Code starts on, and why it was
// chosen.
type choice struct {
	account config.Account
	token   config.Token
	why     string
}

// choose picks the account whose token Claude Code starts on: the one pinned;
// else, while the router is healthy, the one it rates best; else the first
// with a token.
func (r Route) choose(ctx context.Context, state routerState) (choice, error) {
	if r.Account != "" {
		return r.pinned()
	}
	if state.healthy() {
		if c, ok := r.best(ctx); ok {
			return c, nil
		}
	}
	return r.first()
}

// pinned is the account the session is pinned to, which must have a token.
func (r Route) pinned() (choice, error) {
	i := slices.IndexFunc(r.Config.Accounts, func(a config.Account) bool { return a.ID == r.Account })
	if i < 0 {
		return choice{}, fmt.Errorf("there's no account %q: pin %s", r.Account, strings.Join(ids(r.Config.Accounts), " or "))
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
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
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

// first is the first account with a token.
func (r Route) first() (choice, error) {
	var envs []string
	for _, a := range r.Config.Accounts {
		if token, ok := a.Token(r.Getenv); ok {
			return choice{account: a, token: token, why: "the first with a token"}, nil
		}
		envs = append(envs, a.TokenEnv)
	}
	return choice{}, fmt.Errorf("no account has a token for Claude Code to start on: set %s, or give --direct to start it on its own login", strings.Join(envs, " or "))
}

func ids(accounts []config.Account) []string {
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	return ids
}

// title names an account as every command does, such as "work · Work".
func title(a config.Account) string {
	return status.Account{ID: a.ID, Label: a.Label}.Title()
}
