package cli

import (
	"context"
	"errors"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

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
}

// Read reads the router's document, having the router refresh first when r
// asks, when the router answers its health check in time, as run gives it,
// healthy or not, and says which router it was, as its health check
// answered. Otherwise, when r allows, it builds one by probing every
// account, saying why the router's wasn't read when the router was asked.
func (s usageSource) Read(ctx context.Context, r watch.Read) (status.Document, router.Health, error) {
	doc, health, fallback, ok := s.fromRouter(ctx, r.Refresh)
	switch {
	case ok:
		return doc, health, nil
	case !r.Probe:
		return status.Document{}, router.Health{}, watch.ErrNoRouter
	}
	logger.Debug("probing directly", "router", fallback.Router, "reason", fallback.Reason)
	doc, err := s.probe.Fetch(ctx)
	doc.Fallback = fallback
	return doc, router.Health{}, err
}

// fromRouter reads the router's document, having the router probe the
// accounts it hasn't read for refresh first, when that's more than zero, and
// its health check's answer. It reports false, and why when the router was
// asked, when the router isn't to be asked, doesn't answer its health check
// in time, or doesn't give its document.
func (s usageSource) fromRouter(ctx context.Context, refresh time.Duration) (status.Document, router.Health, status.Fallback, bool) {
	switch {
	case !s.ask:
		return status.Document{}, router.Health{}, status.Fallback{}, false
	case s.router == nil:
		return status.Document{}, router.Health{}, status.Fallback{Router: status.RouterNotRunning}, false
	}
	health, fallback, ok := s.answers(ctx)
	if !ok {
		return status.Document{}, router.Health{}, fallback, false
	}
	doc, err := s.document(ctx, refresh)
	if err != nil {
		return status.Document{}, router.Health{}, fallbackFrom(err), false
	}
	logger.Debug("read the router", "refresh", refresh, "healthy", doc.Router.Healthy)
	return doc, health, status.Fallback{}, true
}

// answers asks the router's health check, giving it launch.AskTimeout, as
// run does, and reports whether it answered, with its answer, or why not.
func (s usageSource) answers(ctx context.Context) (router.Health, status.Fallback, bool) {
	ctx, cancel := context.WithTimeout(ctx, launch.AskTimeout)
	defer cancel()
	health, err := s.router.Health(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return router.Health{}, status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within " + launch.AskTimeout.String()}, false
	}
	return health, fallbackFrom(err), err == nil
}

// RouterAnswers reports whether the router answers its health check within
// launch.AskTimeout, as run gives it, healthy or not: while it does, it posts
// the desktop notifications, whether its document is read or not.
func (s usageSource) RouterAnswers(ctx context.Context) bool {
	if s.router == nil {
		return false
	}
	_, _, ok := s.answers(ctx)
	return ok
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
// asking the router failed with: nothing answers on its socket, or what does
// doesn't answer as a router does. It's zero for no failure.
func fallbackFrom(err error) status.Fallback {
	switch {
	case err == nil:
		return status.Fallback{}
	case errors.Is(err, router.ErrNotRunning):
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
	return fromRouter(err)
}

// Unpin has the router route every session on its merits again.
func (s usageSource) Unpin(ctx context.Context) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.Unpin(ctx, false, router.ByDashboard)
	return fromRouter(err)
}

// PinSession has the router send every request of the session with the
// given id to the account with the given id from its next request on, as
// pin --session does.
func (s usageSource) PinSession(ctx context.Context, session, account string) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.PinSession(ctx, session, account, router.ByDashboard)
	return fromRouter(err)
}

// UnpinSession has the router clear the session's own pin, routing it on its
// merits from its next request on, as pin auto --session does.
func (s usageSource) UnpinSession(ctx context.Context, session string) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.UnpinSession(ctx, session, router.ByDashboard)
	return fromRouter(err)
}

// probeSource reads the status document by probing accounts.
type probeSource struct {
	deps Deps
	// readToken reads an account's token, by its id, from its file.
	readToken func(id string) (tokens.Token, error)
	upstream  string
	accounts  []config.Account
	prime     config.Prime
}

// Fetch probes every account, claiming the Claude Code version installed now:
// a watch can outlive the one it started with. It never fails: an account that
// can't be read says why in the document.
func (s probeSource) Fetch(ctx context.Context) (status.Document, error) {
	version := s.deps.ClaudeVersion()
	collector := status.Collector{
		Prober: &claude.Prober{Upstream: s.upstream, Version: version},
		Policy: claude.Policy,
		Prime:  s.prime,
		Token:  s.readToken,
		Now:    s.deps.Now,
	}
	doc := collector.Collect(ctx, s.accounts)
	logger.Debug("probed accounts", "accounts", len(doc.Accounts), "best", doc.Best, "claude_version", version)
	return doc, nil
}
