package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// readOnce is what status and usage ask of the source they read, reading
// once: with --refresh, the read the dashboard's R asks for, else the
// document as it stands.
func readOnce(refresh bool) status.Read {
	if refresh {
		return status.Fresh()
	}
	return status.Read{Probe: true}
}

// usageSource is where status, usage and the dashboard read the status
// document: the router's, while it answers, else one built by probing every
// account the config lists. It's the one place that decides, so they all
// decide alike.
type usageSource struct {
	probe probeSource
	// ask is set when the router is to be read first, as it is unless probing
	// was asked for.
	ask bool
	// router is the router, nil when there's no socket to find it at: read
	// when ask is set, and asked after either way.
	router *router.Client
	// pool is the id of the pool read, as inPool names it: "" for the one
	// the verbs act on.
	pool string
	// patience is how long the router is waited for, as run waits.
	patience launch.Patience
}

// inPool returns the source reading the router's document, sessions,
// windows' history and request stream of the pool with the given id, as the
// router's client's InPool asks: "" for the one the verbs act on.
func (s usageSource) inPool(pool string) usageSource {
	if s.router != nil {
		s.router = s.router.InPool(pool)
	}
	s.pool = pool
	return s
}

// Read reads the router's document, of the pool the source reads, having
// the router refresh first when r asks, when the router answers its health
// check, as patiently as run asks it, healthy or not, and says which router
// it was, as its health check answered. Otherwise, when r allows, it builds
// one by probing every account of that pool, saying why the router's wasn't
// read when the router was asked. A router refusing for want of a pool, and
// a config whose pool can't be told, fail with router.PoolNeeded: nothing is
// read in its place.
func (s usageSource) Read(ctx context.Context, r status.Read) (status.Document, router.Health, error) {
	read, err := s.fromRouter(ctx, r.Refresh)
	switch {
	case err != nil:
		return status.Document{}, router.Health{}, err
	case read.ok:
		return read.doc, read.health, nil
	case !r.Probe:
		return status.Document{}, router.Health{}, status.ErrNoRouter
	}
	probe, err := s.probe.inPool(s.pool)
	if err != nil {
		return status.Document{}, router.Health{}, err
	}
	logger.Debug("probing directly", "router", read.fallback.Router, "reason", read.fallback.Reason)
	doc, err := probe.Fetch(ctx)
	doc.Fallback = read.fallback
	return doc, router.Health{}, err
}

// routerRead is what reading the router's document came to: the document,
// worked out, and the router's health, where ok is set; else why it wasn't
// read, where the router was asked.
type routerRead struct {
	doc      status.Document
	health   router.Health
	fallback status.Fallback
	ok       bool
}

// fromRouter reads the router's document, having the router probe the
// accounts it hasn't read for refresh first, when that's more than zero, and
// its health check's answer. What's worked out of the document, as
// WorkedOut says, is worked out as it's read, as a router from before it
// gave that, still running between an upgrade and its restart, gives none.
// It's unread, and says why when the router was asked, when the router isn't
// to be asked, doesn't answer its health check in time, doesn't give its
// document, or is one from before pools, as status.BeforePools has it of the
// config's pools as they stand; and fails with router.PoolNeeded where it refuses for want of a
// pool.
func (s usageSource) fromRouter(ctx context.Context, refresh time.Duration) (routerRead, error) {
	switch {
	case !s.ask:
		return routerRead{}, nil
	case s.router == nil:
		return routerRead{fallback: status.Fallback{Router: status.RouterNotRunning}}, nil
	}
	health, fallback, ok := s.answers(ctx)
	switch {
	case !ok:
		return routerRead{fallback: fallback}, nil
	case status.BeforePools(health.ReportsPools, s.probe.pools):
		return routerRead{fallback: status.Fallback{Router: status.RouterBeforePools}}, nil
	}
	doc, err := s.document(ctx, refresh)
	if _, needs := errors.AsType[router.PoolNeeded](err); needs {
		return routerRead{}, err
	}
	if err != nil {
		return routerRead{fallback: fallbackFrom(err)}, nil
	}
	logger.Debug("read the router", "refresh", refresh, "healthy", doc.Router.Healthy)
	return routerRead{doc: doc.WorkedOut(claude.Policy()), health: health, ok: true}, nil
}

// answers asks the router's health check, as patiently as run does, and
// reports whether it answered, with its answer, or why not.
func (s usageSource) answers(ctx context.Context) (router.Health, status.Fallback, bool) {
	health, err := s.patience.Ask(ctx, s.router)
	return health, fallbackFrom(err), err == nil
}

// RouterAnswers reports whether the router answers its health check, as
// patiently as run asks it, healthy or not: while it does, it posts the
// desktop notifications, whether its document is read or not.
func (s usageSource) RouterAnswers(ctx context.Context) bool {
	if s.router == nil {
		return false
	}
	_, _, ok := s.answers(ctx)
	return ok
}

// everyPool reads the router's status document of every pool it routes for,
// as its client's StatusInEveryPool asks, once the router answers its health
// check, as patiently as run asks it: for what's said of every account,
// whatever pool it's in. It fails where the router doesn't answer, or give
// them.
func (s usageSource) everyPool(ctx context.Context) ([]status.Document, error) {
	if s.router == nil {
		return nil, errRouterDown
	}
	health, fallback, ok := s.answers(ctx)
	if !ok {
		return nil, fmt.Errorf("the router doesn't answer: %s", fallback.Router)
	}
	return s.router.StatusInEveryPool(ctx, health)
}

// History asks the router for every account's use of the window with the
// given key over its current length, a point each step: it fails with
// router.ErrNoHistory, wrapped, from a router from before GET /history.
func (s usageSource) History(ctx context.Context, window string, step time.Duration) (router.History, error) {
	if s.router == nil {
		return router.History{}, errRouterDown
	}
	return s.router.History(ctx, window, step)
}

// Sessions lists the sessions the router has routed in the last hour, the
// one seen last first.
func (s usageSource) Sessions(ctx context.Context) ([]status.Session, error) {
	if s.router == nil {
		return nil, errRouterDown
	}
	return s.router.Sessions(ctx)
}

// Stream opens the router's request stream, which tells of what befalls each
// routed request as it happens: it fails with router.ErrNoStream, wrapped,
// from a router from before GET /stream.
func (s usageSource) Stream(ctx context.Context) (<-chan router.StreamEvent, error) {
	if s.router == nil {
		return nil, errRouterDown
	}
	return s.router.Stream(ctx)
}

// document is the router's status document, once it has probed the accounts
// it hasn't read for refresh, when that's more than zero.
func (s usageSource) document(ctx context.Context, refresh time.Duration) (status.Document, error) {
	if refresh > 0 {
		return s.router.Refresh(ctx, refresh)
	}
	return s.router.Status(ctx)
}

// fallbackFrom says why the router's document couldn't be read, from what
// asking the router failed with: it isn't running, as router.IsNotRunning
// says, or what answers on its socket doesn't answer as a router does. It's
// zero for no failure.
func fallbackFrom(err error) status.Fallback {
	switch {
	case err == nil:
		return status.Fallback{}
	case router.IsNotRunning(err):
		return status.Fallback{Router: status.RouterNotRunning}
	default:
		return status.Fallback{Router: status.RouterUnhealthy, Reason: err.Error()}
	}
}

// Pin has the router send every new session to the best of the accounts with
// the given ids, and with move, every running session on another account too.
func (s usageSource) Pin(ctx context.Context, accounts []string, move bool) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.Pin(ctx, router.PinRequest{Accounts: accounts, Move: move, By: router.ByDashboard})
	return fromRouterChange(err)
}

// Unpin has the router route every session on its merits again.
func (s usageSource) Unpin(ctx context.Context) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.Unpin(ctx, false, router.ByDashboard)
	return fromRouterChange(err)
}

// PinSession has the router send every request of the session with the
// given id to the account with the given id from its next request on, as
// pin --session does.
func (s usageSource) PinSession(ctx context.Context, session, account string) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.PinSession(ctx, session, account, router.ByDashboard)
	return fromRouterChange(err)
}

// UnpinSession has the router clear the session's own pin, routing it on its
// merits from its next request on, as pin auto --session does.
func (s usageSource) UnpinSession(ctx context.Context, session string) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.UnpinSession(ctx, session, router.ByDashboard)
	return fromRouterChange(err)
}

// probeSource reads the status document by probing accounts.
type probeSource struct {
	deps Deps
	// readToken reads an account's token, by its id, from its file.
	readToken func(id string) (tokens.Token, error)
	// tokenTaken reads when an account's token was taken, by its id and the
	// token.
	tokenTaken func(id string, token tokens.Token) (time.Time, error)
	upstream   string
	// pooled gives the config as its pools stand, as pooledConfig has it,
	// asked only where the accounts are probed.
	pooled func() (*config.Config, error)
	// accounts are those probed, and pool the pool they're of, as inPool
	// names it: zero where the config has none.
	accounts []config.Account
	pool     config.Pool
	prime    config.Prime
}

// pools are the config's pools as they stand, as pooled gives them: none
// where they can't be told, as probing them then fails, nor where there's no
// config to probe by, as of a source reading the router alone where the
// config can't be read.
func (s probeSource) pools() config.Pools {
	if s.pooled == nil {
		return nil
	}
	cfg, err := s.pooled()
	if err != nil {
		return nil
	}
	return cfg.Pools
}

// inPool returns the source probing the accounts of the pool with the given
// id alone, else of the one the verbs act on, as config.Pools' Implicit has
// it, of the config as its pools stand: every account, in a config with no
// pools. It fails where those pools can't be told, as pooledConfig does,
// and with router.PoolNeeded where the pool named isn't among them, or none
// is named and there's none the verbs act on.
func (s probeSource) inPool(named string) (probeSource, error) {
	cfg, err := s.pooled()
	if err != nil {
		return probeSource{}, err
	}
	s.accounts = cfg.Accounts
	if len(cfg.Pools) == 0 {
		return s, nil
	}
	p, err := askedPool(cfg.Pools, named)
	if err != nil {
		return probeSource{}, err
	}
	s.accounts, s.pool = cfg.Accounts.InPool(p.ID), p
	return s, nil
}

// Fetch probes every account, claiming the Claude Code version installed now:
// a watch can outlive the one it started with. It never fails: an account that
// can't be read says why in the document.
func (s probeSource) Fetch(ctx context.Context) (status.Document, error) {
	version := s.deps.ClaudeVersion()
	collector := status.Collector{
		Prober: &claude.Prober{Upstream: s.upstream, Version: version},
		Policy: claude.Policy(),
		Prime:  s.prime,
		Token:  s.readToken,
		Taken:  s.tokenTaken,
		Now:    s.deps.Now,
	}
	doc := collector.Collect(ctx, s.accounts)
	doc.PoolID, doc.PoolLabel = s.pool.ID, s.pool.Label
	logger.Debug("probed accounts", "accounts", len(doc.Accounts), "best", doc.Best, "claude_version", version)
	return doc, nil
}
