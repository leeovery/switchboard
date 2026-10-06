// Package launch starts Claude Code through switchboard: run, which hands
// this process over to Claude Code, pointed at the router while it's healthy.
package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

var logger = logs.For("launch")

// AskTimeout bounds each question run asks the router, so a router that's
// stuck holds Claude Code up for a moment at most. What else asks whether the
// router answers gives it as long.
const AskTimeout = 500 * time.Millisecond

// lookAgain is how long run waits to look at a token file again, when it
// finds it empty at first: a writer that empties the file before it writes
// the token leaves it so for a moment, which one look can catch.
const lookAgain = 200 * time.Millisecond

// Router is the router, as run asks it how it is: *router.Client is one.
type Router interface {
	Health(ctx context.Context) (router.Health, error)
}

// Launcher starts Claude Code in this process's place.
type Launcher struct {
	// Environ is the environment Claude Code starts from, as os.Environ
	// gives it, and whose PATH it's looked for on.
	Environ []string
	// Dir is the directory Claude Code starts in, as os.Getwd gives it, and
	// Home the user's home directory, as os.UserHomeDir does, each "" where
	// it isn't known: through the router, Claude Code's requests tell it Dir,
	// from Home where it's within it, as ~/Code/api.
	Dir  string
	Home string
	// InstallPaths are where Claude Code's installers put it, tried in order
	// when it isn't on PATH.
	InstallPaths []string
	// Executable returns the path of this switchboard binary, as
	// os.Executable does: a claude that leads to it is switchboard's link,
	// never Claude Code.
	Executable func() (string, error)
	// PID is this process's id, as os.Getpid gives it, which the claude
	// started in its place keeps.
	PID int
	// Now reads the wall clock, which says how old a mark is (see
	// startedEnv).
	Now func() time.Time
	// Exec replaces this process with the program at path, as Exec does.
	Exec func(path string, argv, env []string) error
	// Stderr hears why, when Claude Code starts without the router, or
	// without switchboard.
	Stderr io.Writer
}

// Route is what starting Claude Code on an account's token takes.
type Route struct {
	Config *config.Config
	// Token reads an account's token, by the account's id, from its file.
	Token func(id string) (tokens.Token, error)
	// Pause waits as long as it's given, as time.Sleep does, before a token
	// file is looked at again.
	Pause  func(time.Duration)
	Router Router
	// Account is the id of the account to pin the session to, or "" to leave
	// it to the router.
	Account string
}

// Run starts Claude Code with args in this process's place. While the router
// is healthy, Claude Code sends its requests there, to the address the router
// says its proxy listens on, whatever the config says, telling it the
// directory Claude Code starts in, its conversation pinned when an account
// is, on the primary's token, whichever account the conversation goes to:
// what isn't the conversation goes out on Claude Code's own token, and so
// lands on the primary. Otherwise it sends them straight to the API, on the
// pinned account's token, else the primary's, and Stderr hears why. Either
// way, failing those, it starts on the first account's with a usable token,
// and with none, Unaided. A pin that can't be kept, to an account not
// configured or without a usable token, fails: it's the command line's
// mistake. Run returns only when Claude Code couldn't start.
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
	c, err := r.choose(pin, state.healthy())
	if err != nil {
		return l.unaided(path, args, "no account has a usable token", err)
	}
	env := environ(l.Environ)
	if state.healthy() {
		env = env.with(claude.BaseURLEnv, "http://"+state.listen).with(claude.TokenEnv, c.token.Reveal()).pinnedTo(r.Account).workingIn(l.shownDir())
		logger.Info("starting claude", "mode", "routed", "router", state.name, "account", c.account.ID, "chosen", c.why, "pin", r.Account, "claude", path)
	} else {
		env = env.without(claude.BaseURLEnv).with(claude.TokenEnv, c.token.Reveal()).pinnedTo("").workingIn("")
		logger.Warn("starting claude", "mode", "direct", "router", state.name, "reason", state.reason, "account", c.account.ID, "chosen", c.why, "claude", path)
		Notice(l.Stderr, "the router "+state.String()+" — connecting directly on "+title(c.account))
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
	return l.exec(path, args, environ(l.Environ).without(claude.TokenEnv, claude.BaseURLEnv).pinnedTo("").workingIn(""))
}

// Local starts Claude Code with args, which run one of its local
// subcommands, as claude.IsLocal says, in this process's place as if
// switchboard weren't there: switchboard has no part in them. Its environment
// goes as it is, a pin inherited from a session included, but for the mark
// every claude switchboard starts carries (see startedEnv), and Stderr hears
// nothing. It returns only when Claude Code couldn't start.
func (l Launcher) Local(args []string) error {
	path, err := l.find()
	if err != nil {
		return err
	}
	logger.Debug("starting claude", "mode", "local", "claude", path)
	return l.exec(path, args, l.Environ)
}

// KeyEnv returns the variable of the environment Claude Code starts in that
// holds a key it may use in place of an account's token, as claude.KeyEnv
// finds one: "" when none does.
func (l Launcher) KeyEnv() string {
	return claude.KeyEnv(environ(l.Environ).get)
}

// StepAside starts Claude Code with args in this process's place as if
// switchboard weren't there, its environment as it is but for the mark every
// claude switchboard starts carries (see startedEnv), for when that
// environment sets key, a variable holding a key Claude Code may use in
// place of an account's token: its requests would go unrouted, and be billed
// to the key. Stderr hears why. It returns only when Claude Code couldn't
// start.
func (l Launcher) StepAside(args []string, key string) error {
	path, err := l.find()
	if err != nil {
		return err
	}
	logger.Info("starting claude without switchboard", "reason", key+" is set", "claude", path)
	Notice(l.Stderr, key+" is set, so Claude Code uses it — starting claude without switchboard")
	return l.exec(path, args, l.Environ)
}

// Unaided starts Claude Code with args in this process's place as if
// switchboard weren't there, its environment as it is but for the mark every
// claude switchboard starts carries (see startedEnv), for when switchboard
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
	Notice(w, couldnt+" ("+firstLine(err)+") — "+instead)
}

// Notice tells w what switchboard would have the user know, in a line of its
// own, such as "switchboard: the router isn't running — connecting directly
// on work · Work": every notice switchboard gives on stderr reads so. What
// it's about goes ahead all the same, so a line that can't be written is let
// go.
func Notice(w io.Writer, text string) {
	_, _ = fmt.Fprintf(w, "switchboard: %s\n", text)
}

// firstLine is the first line of err's text, which is enough for a notice:
// the rest, such as the example a missing config's error shows, is for the
// commands that fail with it.
func firstLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return strings.TrimSuffix(strings.TrimSpace(line), ":")
}

// exec starts Claude Code, at path, with args and env, marked as started by
// this process, in this process's place. Its arguments are never logged: they
// can hold a prompt.
func (l Launcher) exec(path string, args []string, env environ) error {
	if err := l.Exec(path, append([]string{claude.Command}, args...), env.startingAt(l.PID, l.Now(), path)); err != nil {
		return fmt.Errorf("start claude at %s: %w", path, err)
	}
	return nil
}

// find returns where Claude Code is, as claude.Find finds it on the PATH it
// starts with: past the claude this process started, when it has been
// started again in that one's place.
func (l Launcher) find() (string, error) {
	env := environ(l.Environ)
	after := env.startedAt(l.PID, l.Now())
	if after != "" {
		logger.Info("started again in place of the claude it started; looking past it", "claude", after)
	}
	return claude.FindAfter(after, env.get("PATH"), l.InstallPaths, l.Executable)
}

// shownDir returns Dir as the router is told it: within Home, from there, as
// ~/Code/api, and anywhere else as it is.
func (l Launcher) shownDir() string {
	if l.Home == "" {
		return l.Dir
	}
	if rel, err := filepath.Rel(l.Home, l.Dir); err == nil && filepath.IsLocal(rel) {
		return filepath.Join("~", rel)
	}
	return l.Dir
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
	token   tokens.Token
	why     string
}

// choose picks the account whose token Claude Code starts on. Routed, it's
// the primary's, whatever account the conversation goes to, else pin's;
// direct, it's pin's, the whole session going out on it, else the
// primary's. Either way, failing those, it's the first with a usable token.
// It fails, saying why of each account, when there's none.
func (r Route) choose(pin choice, routed bool) (choice, error) {
	pinned := pin.account.ID != ""
	if pinned && !routed {
		return pin, nil
	}
	if primary, ok := r.primary(); ok {
		return primary, nil
	}
	if pinned {
		return pin, nil
	}
	return r.first()
}

// pinned is the account the session is pinned to, which must be configured
// and have a usable token; none when it isn't pinned. For one that isn't
// configured, it names the accounts that can be pinned, as pin does.
func (r Route) pinned() (choice, error) {
	if r.Account == "" {
		return choice{}, nil
	}
	i := slices.IndexFunc(r.Config.Accounts, func(a config.Account) bool { return a.ID == r.Account })
	if i < 0 {
		return choice{}, router.UnknownAccount(r.Account, r.usable())
	}
	a := r.Config.Accounts[i]
	token, err := r.lookTwice(a.ID)
	if err != nil {
		return choice{}, fmt.Errorf("account %s has no usable token for Claude Code to start on: %w", a.ID, err)
	}
	return choice{account: a, token: token, why: "pinned"}, nil
}

// lookTwice reads the account's token, looking at its file again, lookAgain
// on, when it's there but empty at first, as for a moment while it's
// rewritten: the router takes two looks to find a file without a token, and
// the token Claude Code starts on is its own for the whole session. A file
// that's missing, or isn't the user's alone, is looked at once.
func (r Route) lookTwice(id string) (tokens.Token, error) {
	token, err := r.Token(id)
	if !errors.Is(err, tokens.ErrEmpty) {
		return token, err
	}
	logger.Debug("token file empty; looking again", "account", id, "error", err)
	r.Pause(lookAgain)
	return r.Token(id)
}

// usable lists the ids of the accounts with a usable token, in the config's
// order.
func (r Route) usable() []string {
	var ids []string
	for _, a := range r.Config.Accounts {
		if _, err := r.Token(a.ID); err == nil {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// primary is the primary account, when its token is usable.
func (r Route) primary() (choice, bool) {
	a := r.Config.Accounts.Primary()
	token, err := r.lookTwice(a.ID)
	if err != nil {
		return choice{}, false
	}
	return choice{account: a, token: token, why: "the primary"}, true
}

// first is the first account with a usable token. It fails, saying why of
// each account, when there's none.
func (r Route) first() (choice, error) {
	problems := make([]error, len(r.Config.Accounts))
	for i, a := range r.Config.Accounts {
		token, err := r.Token(a.ID)
		if err == nil {
			return choice{account: a, token: token, why: "the first with a token"}, nil
		}
		problems[i] = fmt.Errorf("%s: %w", a.ID, err)
	}
	return choice{}, errors.Join(problems...)
}

// title names an account as every command does, such as "work · Work".
func title(a config.Account) string {
	return status.Account{ID: a.ID, Label: a.Label}.Title()
}
