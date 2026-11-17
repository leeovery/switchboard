// Package launch starts Claude Code through switchboard: run, which hands
// this process over to Claude Code, pointed at the router while it can be
// routed through.
package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

var logger = logs.For("launch")

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
	// it isn't known: through a router that keeps it from the API, Claude
	// Code's requests tell it Dir, and the project Dir is in (see project),
	// each from Home where it's within it, as ~/Code/api.
	Dir  string
	Home string
	// Project finds the project a session started in a directory runs in, and
	// whether the directory is in a repository, as FindProject does: nil for
	// none.
	Project func(dir string) (string, bool)
	// Processes tells of the processes running, as ProcessTable does, so a
	// launch is found within the session whose claude started it, or started
	// what did (see findOuter): nil tells of none, and no launch is within a
	// session.
	Processes Processes
	// EvalSymlinks resolves the links in a path, as filepath.EvalSymlinks
	// does, so the pool Dir is in is the pool of where it leads.
	EvalSymlinks func(path string) (string, error)
	// Worktree finds a directory's places in the repositories of the linked
	// worktrees it's in, as FindWorktree does, so the pool Dir is in is its
	// repository's too: nil for none.
	Worktree func(dir string) Worktree
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
	// without switchboard, or doesn't start, the router down, and how to go
	// on; and when it starts through a router that isn't healthy, or without
	// the config.
	Stderr io.Writer

	// outer holds the sessions the launch is started within, as findOuter
	// finds them once, at the launch's start, so every rule it's placed by
	// sees the same.
	outer outerSessions
}

// Route is what starting Claude Code on an account's token takes.
type Route struct {
	// Config is the config. RunUnread, for when it can't be read, makes one
	// of the pools and accounts the router routes for in its place.
	Config *config.Config
	// Written is the pools the config file writes, as read where the config
	// can't be, which RunUnread takes alone of the file.
	Written Written
	// from is where Config's pools, and each account's pool, come from: the
	// config's own, or, in their place, the router's or the accounts
	// record's.
	from poolsSource
	// Token reads an account's token, by the account's id, from its file.
	Token func(id string) (tokens.Token, error)
	// Pause waits as long as it's given, as time.Sleep does, before a token
	// file is looked at again.
	Pause  func(time.Duration)
	Router Router
	// Document asks the router for its status document of the pool with the
	// given id, as *router.Client's StatusOf does: of the session's pool, for
	// how its accounts stand, which a routed launch chooses Claude Code's own
	// token by (see choose); and, naming none, for the accounts RunUnread
	// starts on in place of the config's, where the router doesn't report its
	// pools. nil: there's none to be had.
	Document func(ctx context.Context, pool string) (status.Document, error)
	// Refused is why the API refused the last read of the account with the
	// given id, on token, as the router's state file keeps it, as
	// router.KeptRefusals has it, which a launch with the router down chooses
	// Claude Code's token by, as the router, started again, would hold the
	// account back (see choose). nil keeps none.
	Refused func(id string, token tokens.Token) claude.ReadRefusal
	// Patience is how long the router is waited for, as Patience.Ask waits.
	Patience Patience
	// Account is the id of the account to pin the session to, or "" to leave
	// it to the router.
	Account string
	// Moved is set once the move to pools is done, as idmove.PoolsMoved finds
	// its mark in the state directory, after which Run never launches a config
	// naming no pools as it is (see withPools).
	Moved bool
	// Kept returns cfg, a config naming no pools, with the pools the accounts
	// record keeps, each of its accounts in the pool the record keeps it in:
	// false where the record keeps none, or can't be read. nil keeps none.
	Kept func(cfg *config.Config) (*config.Config, bool)
}

// Run starts Claude Code with args in this process's place. While the router
// answers saying where its proxy listens, healthy or not, Claude Code sends
// its requests there, to that address, whatever the config says, telling it
// the directory Claude Code starts in and its project, where it keeps them
// from the API, its conversation pinned when an account is, on the primary's
// token, whichever account the conversation goes to: what isn't the
// conversation goes out on Claude Code's own token, and so lands on the
// primary. Claude Code tells the router too what it tells the API of each
// request, such as the prompt it serves, unless the environment says already
// whether to. Stderr hears of a router that isn't healthy, whose failures a
// direct connection would meet all the same. Otherwise, as whenDown says, it
// starts nothing, failing with ErrNotStarted, or sends them straight to the
// API, on the pinned account's token, else the primary's; either way, Stderr
// hears why. Routed or direct, failing those, it starts on the first
// account's with a usable token, and with none, Unaided. Of those, but for a
// pin direct, Claude Code's own token passes over an account the router
// doesn't route yet, or holds back from every request, its last read
// refused, as the router's status document of the session's pool says, or,
// with the router down, its state file, while another's will do (see
// choose). A pin that can't be kept, to an account not configured or
// without a usable token, fails: it's the command line's mistake. It starts
// from the environment as the user set it (see own), and marks what it sets
// there (see setEnv).
//
// In a config with pools, the session is in the pool the directory it starts
// in is in, for its whole life (see placed), and every account above is that
// pool's alone: the primary its primary, and an account pinned in another
// pool refused. Routed, the router is told the pool, and with more than one,
// a router whose pools aren't the config's, once it has had the time to take
// them up, counts as down (see takingUp). Routed or direct, the session's
// environment names the pool, in PoolEnv. Where no account of the pool has a
// usable token, it refuses, failing with ErrNotStarted, Stderr hearing why,
// but for a config of one pool, unfenced, which starts Unaided, as before
// pools. A config naming no pools once the move to pools is done takes them
// from elsewhere (see withPools). What it refuses for the pools and the pin
// alone, it refuses before the router is asked how it is, where the pools
// don't need its answer.
//
// Started within another session (see findOuter), the session is in that
// session's pool, but where a fence holds Dir (see sessionPool), and, while
// it's in that session's pool, runs in that session's project, where Dir is
// in no repository (see project).
// Run returns only when Claude Code didn't start.
func (l Launcher) Run(ctx context.Context, r Route, args []string) error {
	l.outer = l.findOuter()
	path, err := l.find()
	if err != nil {
		return err
	}
	ask := r.asker(ctx)
	if r, err = l.withPools(r, ask); err != nil {
		return err
	}
	in, err := l.placed(r)
	if err != nil {
		return err
	}
	pin, err := r.pinned(in)
	if err != nil {
		return err
	}
	h, state := ask()
	if state.routable() {
		state = r.takingUp(ctx, h, state, in)
	}
	if !state.routable() {
		if err := l.goDirect(r.Config.WhenRouterDown, state); err != nil {
			return err
		}
	}
	c, err := r.choose(in.accounts, pin, state.routable(), r.standing(ctx, state, in))
	if err != nil {
		return l.tokenless(in, err, func() error {
			return l.unaided(path, args, cantTakePart{couldnt: "no account has a usable token", err: err}, !state.routable(), in.pools)
		})
	}
	if state.routable() {
		return l.routed(path, args, state, c, r.Account, in, nil)
	}
	return l.connectDirectly(path, args, state, c, in)
}

// tokenless starts Claude Code where no account it can start on, of in's,
// has a usable token, as err says of each: as aside starts it, unaided,
// where the pools are as before them. Otherwise it refuses, failing with
// ErrNotStarted, Stderr hearing why: Claude Code's own login would go out on
// whatever account it's signed into, in another pool or none.
func (l Launcher) tokenless(in place, err error, aside func() error) error {
	if !in.asBefore() {
		return l.refuse(in.noToken(), "pool", in.id(), "error", err)
	}
	return aside()
}

// CouldntReadConfig is what a launch says switchboard couldn't do where it
// can't read the config.
const CouldntReadConfig = "couldn't read the config"

// What keeps RunUnread from routing through a router that answers, as Stderr
// hears it, so claude starting without switchboard isn't read as the router
// being down. A router's config has an account, so a document naming none
// hasn't said which.
const (
	routerUnsaid    = "the running router doesn't say which accounts it routes for"
	routerTokenless = "the running router names no account with a usable token"
)

// routerUnpooled is what keeps RunUnread from routing through a router that
// answers where it doesn't route by the pools from says, as Stderr hears it:
// "the config's", as whose names them; else, naming no source, what differs,
// as Written's unroutedBy refuses a router for pools that aren't the
// config's only where it reports none.
func routerUnpooled(from poolsSource) string {
	if whose := from.whose(); whose != "" {
		return "the running router doesn't route by " + whose + " pools"
	}
	return "the running router reports no pools"
}

// RunUnread starts Claude Code with args in this process's place where the
// config can't be read, as unread says why, but the router's state directory
// was located, as it can be without a config. While the router answers saying
// where its proxy listens, healthy or not, it starts through it, as Run does,
// on the pools and accounts the router routes for, in place of the config's:
// as its health answer reports them, or, from a router that reports no pools,
// on the accounts its status document names, in its order, in the one pool
// the config file writes, where it writes one (see routersConfig). Claude
// Code's own token is the primary's of the session's pool, as the router names
// it, while that's usable, else the pinned account's, else the first's with a
// usable token, passing over one the router holds back, as Run does. Stderr
// hears that the config can't be read, and why, and that
// claude starts through the running router. A pin that can't be kept fails,
// as Run's does, and the session's pool is decided, and refused, as Run
// decides it. Where the router can't be routed through, or none of the
// accounts it names has a usable token, or it doesn't route by the pools the
// config file writes, nor, once the move to pools is done, reports any where
// the file writes none (see Written's unroutedBy), it starts Unaided, as for a
// config that can't be read, Stderr hearing, of a router that answers, why it
// couldn't be routed through: what only the config says, such as
// when_router_down, isn't known. Unaided, it refuses inside a fenced pool's
// directories, as OwnLoginFences finds the pools, and anywhere where it
// can't find them once the move to pools is done. It returns only when
// Claude Code didn't start.
func (l Launcher) RunUnread(ctx context.Context, r Route, args []string, unread error) error {
	l.outer = l.findOuter()
	path, err := l.find()
	if err != nil {
		return err
	}
	h, state := r.health(ctx)
	r.Written.Pools = l.placedInHome(r.Written.Pools)
	fences, known := OwnLoginFences(r.Written, r.Moved, func() config.Pools {
		pools, _ := RoutedPools(h)
		return pools
	}, r.keptFences)
	aside := func(cant cantTakePart, foundDown bool) error {
		if !known {
			return l.refuse(cant.said()+" — "+l.unpooled(), "error", cant.err, "dir", l.Dir)
		}
		return l.unaided(path, args, cant, foundDown, fences)
	}
	cant := cantTakePart{couldnt: CouldntReadConfig, err: unread}
	if !state.routable() {
		logger.Info("the router can't be routed through", "router", state.name, "reason", state.reason, "asked", state.asked)
		return aside(cant, true)
	}
	if why, whose := r.Written.unroutedBy(h, r.Moved); why != "" {
		logger.Warn("the router doesn't route by the pools", "router", state.name, "reason", why)
		return aside(cant.besides(routerUnpooled(whose)), true)
	}
	if r.Config, err = r.routersConfig(ctx, h); err != nil {
		logger.Warn("can't read which accounts the router routes for", "error", err)
		return aside(cant.besides(routerUnsaid), false)
	}
	r.from = fromRouter
	return l.routeUnread(ctx, path, args, r, state, cant, aside)
}

// routeUnread starts Claude Code, at path, with args, through the router in
// state, on the config of what the router routes for, r's, as RunUnread says,
// the config unread as cant says why, or, where none of its accounts has a
// usable token, as aside starts it unaided.
func (l Launcher) routeUnread(ctx context.Context, path string, args []string, r Route, state routerState, cant cantTakePart, aside func(cantTakePart, bool) error) error {
	in, err := l.placed(r)
	if err != nil {
		return err
	}
	pin, err := r.pinned(in)
	if err != nil {
		return err
	}
	c, err := r.choose(in.accounts, pin, true, r.standing(ctx, state, in))
	if err != nil {
		logger.Warn("no account the router routes for has a usable token", "error", err)
		return l.tokenless(in, err, func() error { return aside(cant.besides(routerTokenless), false) })
	}
	Warn(l.Stderr, CouldntReadConfig, cant.err, "starting claude through the running router")
	return l.routed(path, args, state, c, r.Account, in, cant.err)
}

// routed starts Claude Code, at path, with args, through the router in state,
// on c's token, pinned to pin unless it's "", in in's pool, as Run says, the
// config unread as unread says why: nil where it was read.
func (l Launcher) routed(path string, args []string, state routerState, c choice, pin string, in place, unread error) error {
	own, project := l.own(), l.project(in)
	env := own.with(claude.BaseURLEnv, "http://"+state.listen).with(claude.TokenEnv, c.token.Reveal()).
		pinnedTo(pin).workingIn(l.told(state, project)).inPool(state.told(in.id())).
		withOrUnset(PoolEnv, in.id()).withOrUnset(projectEnv, project).hinting()
	l.routedThrough(state, c, pin, in, path, unread)
	return l.execSession(path, args, env, own)
}

// routedThrough logs Claude Code starting at path through the router in
// state, on c's token, pinned to pin unless it's "", in in's pool, where it's
// in one: at info, or at warn where the router isn't healthy, Stderr hearing
// so, or where the config can't be read, as unread, nil where it was read,
// says why.
func (l Launcher) routedThrough(state routerState, c choice, pin string, in place, path string, unread error) {
	attrs := []any{"mode", "routed", "router", state.name}
	if !state.healthy() {
		attrs = append(attrs, "reason", state.reason)
	}
	attrs = append(attrs, "asked", state.asked, "account", c.account.ID, "chosen", c.why, "pin", pin)
	if in.pooled() {
		attrs = append(attrs, "pool", in.id())
	}
	attrs = append(attrs, in.within.attrs()...)
	if unread != nil {
		attrs = append(attrs, "config_error", unread)
	}
	level := slog.LevelInfo
	if !state.healthy() || unread != nil {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, "starting claude", append(attrs, "claude", path)...)
	if !state.healthy() {
		Notice(l.Stderr, "the router "+state.String()+" — starting claude through it all the same")
	}
}

// connectDirectly starts Claude Code, at path, with args, straight to the
// API, the router down as state says, on c's token, in in's pool, as Run
// says: Stderr hears so.
func (l Launcher) connectDirectly(path string, args []string, state routerState, c choice, in place) error {
	own := l.own()
	env := own.without(claude.BaseURLEnv).with(claude.TokenEnv, c.token.Reveal()).pinnedTo("").workingIn("", "").inPool("").
		withOrUnset(PoolEnv, in.id()).withOrUnset(projectEnv, l.project(in))
	attrs := []any{"mode", "direct", "router", state.name, "reason", state.reason, "asked", state.asked, "account", c.account.ID, "chosen", c.why}
	if in.pooled() {
		attrs = append(attrs, "pool", in.id())
	}
	attrs = append(attrs, in.within.attrs()...)
	logger.Warn("starting claude", append(attrs, "claude", path)...)
	Notice(l.Stderr, "the router "+state.String()+" — connecting directly on "+title(c.account))
	return l.execSession(path, args, env, own)
}

// Direct starts Claude Code with args in this process's place, on its own
// login: without the router or an account's token, for what needs the login.
// It starts from the environment as the user set it (see own). Inside the
// directories of a fenced pool of pools, or where it can't tell, it refuses,
// failing with ErrNotStarted, Stderr hearing why (see ownLoginBarred), the
// pools placed in Home first (see placedInHome). It returns only when Claude
// Code couldn't start.
func (l Launcher) Direct(args []string, pools config.Pools) error {
	l.outer = l.findOuter()
	path, err := l.find()
	if err != nil {
		return err
	}
	if why := l.ownLoginBarred(l.placedInHome(pools)); why != "" {
		return l.refuse(why, "mode", "own login", "dir", l.Dir)
	}
	logger.Info("starting claude", "mode", "own login", "claude", path)
	return l.exec(path, args, environ(OwnLogin(l.Environ)))
}

// OwnLogin returns env, an environment as os.Environ gives it, as Claude Code
// starts on its own login, as Direct starts it: as the user set it (see own),
// without the token, the base URL, a pin, the directory, project or pool told
// the router, nor the session's pool or project, nor any mark of
// switchboard's.
func OwnLogin(env []string) []string {
	return environ(env).unmarked().without(claude.TokenEnv, claude.BaseURLEnv, startedEnv, whenDownEnv, PoolEnv, projectEnv).pinnedTo("").workingIn("", "").inPool("")
}

// Local starts Claude Code with args, which run one of its local
// subcommands, as claude.IsLocal says, in this process's place as if
// switchboard weren't there: switchboard has no part in them. Its environment
// goes as it is, a pin inherited from a session included, but for the mark
// every claude switchboard starts carries (see startedEnv), and Stderr hears
// nothing. A mark of what run set goes on as it is, made for a session's
// claude, so this claude is never taken for one (see sessionOf). It returns
// only when Claude Code couldn't start.
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
// switchboard weren't there, its environment as the user set it (see own)
// but for the mark every claude switchboard starts carries (see startedEnv),
// for when that environment sets key, a variable holding a key Claude Code
// may use in place of an account's token: its requests would go unrouted,
// and be billed to the key. Stderr hears why. It returns only when Claude
// Code couldn't start.
func (l Launcher) StepAside(args []string, key string) error {
	path, err := l.find()
	if err != nil {
		return err
	}
	logger.Info("starting claude without switchboard", "reason", key+" is set", "claude", path)
	Notice(l.Stderr, key+" is set, so Claude Code uses it — starting claude without switchboard")
	return l.exec(path, args, l.own())
}

// Unaided starts Claude Code with args in this process's place as if
// switchboard weren't there, in the environment unaidedEnv gives, but for the
// mark every claude switchboard starts carries (see startedEnv), for when
// switchboard can't take part: switchboard mustn't stand between the user and
// claude. Stderr hears what switchboard couldn't do, and why, as err's first
// line says. Inside the directories of a fenced pool of pools it refuses, as
// Direct does. It returns only when Claude Code couldn't start.
func (l Launcher) Unaided(args []string, pools config.Pools, couldnt string, err error) error {
	l.outer = l.findOuter()
	path, findErr := l.find()
	if findErr != nil {
		return findErr
	}
	return l.unaided(path, args, cantTakePart{couldnt: couldnt, err: err}, false, pools)
}

// RouterUnfound starts Claude Code with args in this process's place, or
// starts nothing, failing with ErrNotStarted, as whenDown says of the
// config's setting, r's, for when the config was read but the router
// couldn't be found, as err's first line says why: as for a router not
// running. In a config without pools it starts as Unaided does; in one with
// them, as Run starts with the router down, straight to the API on a token of
// the session's pool, the pin's, else the primary's, else the first usable,
// refusing as Run does. It returns only when Claude Code didn't start.
func (l Launcher) RouterUnfound(r Route, args []string, err error) error {
	l.outer = l.findOuter()
	path, findErr := l.find()
	if findErr != nil {
		return findErr
	}
	state := routerState{name: notFound, reason: firstLine(err)}
	cant := cantTakePart{couldnt: "couldn't find the router", err: err}
	if len(r.Config.Pools) == 0 {
		if stopped := l.goDirect(r.Config.WhenRouterDown, state); stopped != nil {
			return stopped
		}
		return l.unaided(path, args, cant, false, nil)
	}
	in, err := l.placed(r)
	if err != nil {
		return err
	}
	pin, err := r.pinned(in)
	if err != nil {
		return err
	}
	if stopped := l.goDirect(r.Config.WhenRouterDown, state); stopped != nil {
		return stopped
	}
	c, err := r.choose(in.accounts, pin, false, standing{kept: r.Refused})
	switch {
	case err == nil:
		return l.connectDirectly(path, args, state, c, in)
	case in.asBefore():
		return l.unaided(path, args, cant, false, in.pools)
	}
	// The token files are in the state directory, as the router's socket is:
	// why the router wasn't found is most often why no token can be read.
	return l.refuse(cant.said()+" — "+in.noToken(), "pool", in.id(), "error", err)
}

// cantTakePart is why switchboard can't take part in a launch: what it
// couldn't do, why, as err's first line says, and what else kept it out,
// also, unless that's "".
type cantTakePart struct {
	couldnt string
	err     error
	also    string
}

// besides returns c with what else kept switchboard out.
func (c cantTakePart) besides(also string) cantTakePart {
	c.also = also
	return c
}

// said says why, as "couldn't read the config (invalid config …) — the
// running router names no account with a usable token".
func (c cantTakePart) said() string {
	said := c.couldnt + " (" + firstLine(c.err) + ")"
	if c.also != "" {
		said += " — " + c.also
	}
	return said
}

// unaided starts Claude Code, at path, with args, in the environment
// unaidedEnv gives, saying why switchboard can't take part, as cant says, and
// where Claude Code goes instead: through the router of the session it's
// started within, or without switchboard. foundDown says this launch found
// the router couldn't be routed through. Inside the directories of a fenced
// pool of pools, or where it can't tell, it refuses, as Direct does, the
// pools placed in Home first, for the refusal and the environment alike.
func (l Launcher) unaided(path string, args []string, cant cantTakePart, foundDown bool, pools config.Pools) error {
	pools = l.placedInHome(pools)
	if why := l.ownLoginBarred(pools); why != "" {
		return l.refuse(cant.said()+" — "+why, "error", cant.err, "dir", l.Dir)
	}
	env := l.unaidedEnv(foundDown, pools)
	routed := env.routedByRun()
	instead := "starting claude without it"
	if routed {
		instead = "starting claude through this session's router"
	}
	logger.Warn("starting claude without switchboard", "reason", cant.couldnt, "error", cant.err, "session_router", routed, "claude", path)
	Notice(l.Stderr, cant.said()+" — "+instead)
	return l.exec(path, args, env)
}

// unaidedEnv returns the environment Claude Code starts in unaided. Within a
// session that still goes through the router run sent it to, it's passed on
// as it is, mark and all: that router still routes it, and records it, as it
// does the session; only this launch couldn't take part. But where this
// launch, foundDown, found that router couldn't be routed through, and as
// within a session pointed at a gateway of its own since, it's the user's
// own, so Claude Code isn't sent to a router that can't take it. Where a
// pool of pools is fenced, the session's is passed on only where it's in the
// pool a session started in Dir is in, as its mark records it: else that
// router would route this claude in the session's pool. And where it isn't
// passed on, Claude Code's token goes too, as a session's, run's or the
// user's, can't be told apart, and would go out on an account of another
// pool's. Where none is, it's as without pools: no session can leave a pool.
// The pools are placed in Home, as placedInHome places them, so a pool
// fenced by a directory written ~/ counts as fenced.
func (l Launcher) unaidedEnv(foundDown bool, pools config.Pools) environ {
	env := environ(l.Environ)
	fenced := slices.ContainsFunc(pools, config.Pool.Fenced)
	if env.routedByRun() && !foundDown && (!fenced || l.inSessionsPool(env, pools)) {
		return env
	}
	if !fenced {
		return env.unmarked()
	}
	return env.unmarked().without(claude.TokenEnv)
}

// inSessionsPool reports whether env, a session's, is in the pool of pools a
// session started in Dir is in, as sessionPool finds it: the pool its mark
// says run gave it (see runGave), whatever PoolEnv holds now, as the router
// routes it by the header run added beside it.
func (l Launcher) inSessionsPool(env environ, pools config.Pools) bool {
	p, why := l.sessionPool(pools)
	return why == "" && config.SamePool(env.runGave(PoolEnv), p.pool.ID)
}

// own returns the environment Claude Code starts from as the user set it:
// without what a run it's started within set there, as its mark says (see
// setEnv), so what this launch sets is all switchboard sets, and a claude
// stepped aside for carries nothing of switchboard's.
func (l Launcher) own() environ {
	return environ(l.Environ).unmarked()
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
// this process, now, as execMarked starts it.
func (l Launcher) exec(path string, args []string, env environ) error {
	return l.execMarked(path, args, env, l.starting(path))
}

// execSession starts Claude Code, at path, with args, in env, as exec does,
// for a session run starts: env marked with what run set there against own
// (see markedAgainst), made for the claude it starts, both marks naming it by
// the same second, read once.
func (l Launcher) execSession(path string, args []string, env, own environ) error {
	m := l.starting(path)
	return l.execMarked(path, args, env.markedAgainst(own, m), m)
}

// starting is the mark of the claude at path that this process starts in its
// place now (see startedEnv).
func (l Launcher) starting(path string) startedMark {
	return startedMark{pid: l.PID, at: l.Now(), path: path}
}

// execMarked starts Claude Code, at path, with args and env, marked as m
// says, in this process's place, without whenDownEnv, which is the launch's
// it was given to alone: every process the session starts would carry it on.
// Its arguments are never logged: they can hold a prompt.
func (l Launcher) execMarked(path string, args []string, env environ, m startedMark) error {
	env = env.without(whenDownEnv).startingAt(m)
	if err := l.Exec(path, append([]string{claude.Command}, args...), env); err != nil {
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

// told returns the directory the router in state is told Claude Code starts
// in, as shownDir has it, and project, the one it runs in, as shown shows
// it, none for "", where the router takes every header of switchboard's own
// off what it sends upstream: neither for one that doesn't, which would send
// them on to the API, as routerState's told has it, nor where Dir isn't
// known.
func (l Launcher) told(state routerState, project string) (dir, shownProject string) {
	if !state.strips || l.Dir == "" {
		return "", ""
	}
	if project == "" {
		return l.shownDir(), ""
	}
	return l.shownDir(), l.shown(project)
}

// project returns the project the session, in in, runs in, by its path: the
// repository Dir is in, as Project finds it; else, Dir in none, the project
// of the session it's started within, where it's in that one's pool, and
// that names one, so a claude its tooling starts in a scratchpad runs in its
// project; else Dir itself. "" where Dir isn't known, or there's no Project.
func (l Launcher) project(in place) string {
	if l.Project == nil || l.Dir == "" {
		return ""
	}
	project, inRepository := l.Project(l.Dir)
	if !inRepository && in.within.project != "" {
		return in.within.project
	}
	return project
}

// shownDir returns Dir as the router is told it, as shown shows it.
func (l Launcher) shownDir() string {
	return l.shown(l.Dir)
}

// shown returns path within Home from there, as ~/Code/api, and anywhere
// else as it is.
func (l Launcher) shown(path string) string {
	if l.Home == "" {
		return path
	}
	if rel, err := filepath.Rel(l.Home, path); err == nil && filepath.IsLocal(rel) {
		return filepath.Join("~", rel)
	}
	return path
}

// How the router can answer its health check, as the log names it, or
// notFound, where it couldn't be asked, or otherPools, where it answers with
// pools other than the launch's of several. Healthy and unhealthy alone can
// be routed through; a launch stops, or goes direct, for each of the others,
// so none of them reads "unhealthy".
const (
	healthy      = "healthy"
	unhealthy    = "unhealthy"
	notRunning   = "not running"
	notFound     = "not found"
	notAnswering = "not answering"
	unusable     = "unusable"
	otherPools   = "other pools"
)

// said is how String says the router is, by its state's name, where that
// isn't "is" and the name, nor otherPools, which text says.
var said = map[string]string{
	notRunning:   "isn't running",
	notFound:     "couldn't be found",
	notAnswering: "isn't answering",
	unusable:     "can't be used",
}

// routerState is how the router answered its health check.
type routerState struct {
	// name is one of the states above.
	name string
	// reason says why the router is as name says, "" where nothing does.
	reason string
	// listen is where the router's proxy listens, "" where it can't be routed
	// through, and strips whether it takes every header of switchboard's own
	// off what it sends upstream.
	listen string
	strips bool
	// asked is how long asking took, to the millisecond.
	asked time.Duration
	// pools is whose pools the router doesn't route by, where it's
	// otherPools.
	pools poolsSource
}

func (s routerState) healthy() bool {
	return s.name == healthy
}

// routable is whether Claude Code can be sent to the router, healthy or not.
func (s routerState) routable() bool {
	return s.listen != ""
}

// told returns value, of a header of switchboard's own, such as the
// directory Claude Code starts in, as the router is told it: as it is, where
// the router takes every header of switchboard's own off what it sends
// upstream, and "", for none, where it doesn't, which would send it on to the
// API.
func (s routerState) told(value string) string {
	if !s.strips {
		return ""
	}
	return value
}

// String says how the router is, as "isn't running", "isn't answering (…)"
// or "is unhealthy (…)", its reason in brackets where it has one.
func (s routerState) String() string {
	if s.reason == "" {
		return s.text()
	}
	return s.text() + " (" + s.reason + ")"
}

// text says how the router is, by its state's name: as said says, or, where
// it routes by other pools, as "doesn't route by the config's pools", whose
// as the pools' whose names them, else "routes by other pools".
func (s routerState) text() string {
	if whose := s.pools.whose(); s.name == otherPools && whose != "" {
		return "doesn't route by " + whose + " pools"
	}
	if s.name == otherPools {
		return "routes by other pools"
	}
	if text, ok := said[s.name]; ok {
		return text
	}
	return "is " + s.name
}

// health asks the router how it is, and where its proxy listens, as patiently
// as the route's Patience says, returning its answer, and the state it finds
// the router in, noting how long that took.
func (r Route) health(ctx context.Context) (router.Health, routerState) {
	began := time.Now()
	h, err := r.Patience.Ask(ctx, r.Router)
	state := stateOf(h, err)
	state.asked = since(began)
	return h, state
}

// asker returns a func that asks the router how it is, as health does, the
// first time it's called alone, each call after returning that answer.
func (r Route) asker(ctx context.Context) func() (router.Health, routerState) {
	return sync.OnceValues(func() (router.Health, routerState) { return r.health(ctx) })
}

// since is how long it has been since then, to the millisecond.
func since(then time.Time) time.Duration {
	return time.Since(then).Round(time.Millisecond)
}

// stateOf is how the router is, as its health check answered h, or failed
// with err: one that doesn't say where its proxy listens can't be sent
// requests, healthy or not.
func stateOf(h router.Health, err error) routerState {
	switch {
	case router.IsNotRunning(err):
		return routerState{name: notRunning}
	case err != nil:
		return routerState{name: notAnswering, reason: err.Error()}
	case h.Listen == "":
		return routerState{name: unusable, reason: "it doesn't say where it listens"}
	}
	state := routerState{name: healthy, listen: h.Listen, strips: h.StripsOwnHeaders}
	if !h.OK {
		state.name, state.reason = unhealthy, h.Reason
	}
	return state
}

// OwnAccount is the account whose token a routed session that isn't pinned
// starts on, as Claude Code's own, as choose chooses it from the config
// alone, knowing nothing of how the accounts stand with the router: the
// primary while its token is usable, else the first with a usable token. It
// reports false when no account has one. r's Config is to configure one
// pool's accounts alone, as a session starts on its own pool's.
func (r Route) OwnAccount() (config.Account, bool) {
	c, err := r.choose(r.Config.Accounts, choice{}, true, standing{})
	return c.account, err == nil
}

// routersConfig is a config of the pools and accounts the router routes for,
// for a launch to start on in place of the config's, which can't be read: as
// its health answer h reports them, where it does (see routedConfig); else
// the accounts its status document names, in its order, its primary marked,
// each in the one pool the config file writes, where it writes one by every
// rule the config holds it to, as a router that reports no pools routes
// every account it has in one. It fails where the document can't be had, as
// without a way to ask for it, or the router names no account.
func (r Route) routersConfig(ctx context.Context, h router.Health) (*config.Config, error) {
	if pools, reported := RoutedPools(h); reported {
		return routedConfig(h, pools)
	}
	if r.Document == nil {
		return nil, errors.New("the router can't be asked for its status document")
	}
	doc, err := r.Document(ctx, "")
	if err != nil {
		return nil, err
	}
	if len(doc.Accounts) == 0 {
		return nil, errors.New("the router's status document names no account")
	}
	cfg := &config.Config{Accounts: make(config.Accounts, len(doc.Accounts))}
	if len(r.Written.Pools) == 1 && !r.Written.Unchecked {
		cfg.Pools = r.Written.Pools
	}
	for i, a := range doc.Accounts {
		cfg.Accounts[i] = config.Account{ID: a.ID, Label: a.Label, Primary: a.ID == doc.Primary}
		if len(cfg.Pools) == 1 {
			cfg.Accounts[i].Pool = cfg.Pools[0].ID
		}
	}
	return cfg, nil
}

// choice is the account whose token Claude Code starts on, and why it was
// chosen.
type choice struct {
	account config.Account
	token   tokens.Token
	why     string
}

// choose picks the account of accounts, one pool's, whose token Claude Code
// starts on. Routed, it's the primary's, whatever account the conversation
// goes to, else pin's; direct, it's pin's, the whole session going out on
// it, else the primary's. Either way, failing those, it's the first with a
// usable token. A pin direct aside, it passes over each account known has
// something against, as the router doesn't route it yet, or its last read
// was refused, while another will do, the log saying so: what goes out on
// Claude Code's own token, such as artifacts, uploads and what never reaches
// the router, would pass the router by, or be refused. Where every one is
// passed over, it picks as if nothing were known. It fails, saying why of
// each account, when none has a usable token.
func (r Route) choose(accounts config.Accounts, pin choice, routed bool, known standing) (choice, error) {
	if pin.account.ID != "" && !routed {
		return pin, nil
	}
	var passed []choice
	for c := range r.inTurn(accounts, pin) {
		why := known.against(c.account.ID, c.token)
		if why == "" {
			return c, nil
		}
		logger.Info("passing over an account for Claude Code's own token", "account", c.account.ID, "chosen", c.why, "reason", why)
		passed = append(passed, c)
	}
	if len(passed) > 0 {
		return passed[0], nil
	}
	return r.first(accounts)
}

// inTurn yields the accounts of accounts, one pool's, that Claude Code can
// start on, each with its usable token, in the turn choose takes them, each
// once: the primary; pin's, where it's pinned; then each in the config's
// order.
func (r Route) inTurn(accounts config.Accounts, pin choice) iter.Seq[choice] {
	return func(yield func(choice) bool) {
		var offered []string
		offer := func(c choice) bool {
			offered = append(offered, c.account.ID)
			return yield(c)
		}
		if primary, ok := r.primary(accounts); ok && !offer(primary) {
			return
		}
		if pin.account.ID != "" && !slices.Contains(offered, pin.account.ID) && !offer(pin) {
			return
		}
		for _, a := range accounts {
			if slices.Contains(offered, a.ID) {
				continue
			}
			if token, err := r.Token(a.ID); err == nil && !offer(choice{account: a, token: token, why: "the first with a token"}) {
				return
			}
		}
	}
}

// pinned is the account the session is pinned to, which must be configured,
// in the session's pool, as in places it, and have a usable token; none when
// it isn't pinned. For one that isn't configured, it names the accounts of
// the pool that can be pinned, as pin does; for one in another pool, which
// pool each is in.
func (r Route) pinned(in place) (choice, error) {
	if r.Account == "" {
		return choice{}, nil
	}
	i := slices.IndexFunc(r.Config.Accounts, func(a config.Account) bool { return a.ID == r.Account })
	if i < 0 {
		return choice{}, router.UnknownAccount(r.Account, r.usable(in.accounts))
	}
	a := r.Config.Accounts[i]
	if !in.holds(a) {
		return choice{}, in.elsewhere(a)
	}
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

// usable lists the accounts of accounts with a usable token, in their order.
func (r Route) usable(accounts config.Accounts) config.Accounts {
	var usable config.Accounts
	for _, a := range accounts {
		if _, err := r.Token(a.ID); err == nil {
			usable = append(usable, a)
		}
	}
	return usable
}

// primary is the primary of accounts, one pool's, when its token is usable.
func (r Route) primary(accounts config.Accounts) (choice, bool) {
	if len(accounts) == 0 {
		return choice{}, false
	}
	a := accounts.Primary()
	token, err := r.lookTwice(a.ID)
	if err != nil {
		return choice{}, false
	}
	return choice{account: a, token: token, why: "the primary"}, true
}

// first is the first account of accounts with a usable token. It fails,
// saying why of each account, when there's none.
func (r Route) first(accounts config.Accounts) (choice, error) {
	if len(accounts) == 0 {
		return choice{}, errors.New("there's no account")
	}
	problems := make([]error, len(accounts))
	for i, a := range accounts {
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
