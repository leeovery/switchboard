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
// healthy or not. Otherwise, when r allows, it builds one by probing every
// account, saying why the router's wasn't read when the router was asked.
func (s usageSource) Read(ctx context.Context, r watch.Read) (status.Document, error) {
	doc, fallback, ok := s.fromRouter(ctx, r.Refresh)
	switch {
	case ok:
		return doc, nil
	case !r.Probe:
		return status.Document{}, watch.ErrNoRouter
	}
	logger.Debug("probing directly", "router", fallback.Router, "reason", fallback.Reason)
	doc, err := s.probe.Fetch(ctx)
	doc.Fallback = fallback
	return doc, err
}

// fromRouter reads the router's document, having the router probe the
// accounts it hasn't read for refresh first, when that's more than zero. It
// reports false, and why when the router was asked, when the router isn't to
// be asked, doesn't answer its health check in time, or doesn't give its
// document.
func (s usageSource) fromRouter(ctx context.Context, refresh time.Duration) (status.Document, status.Fallback, bool) {
	switch {
	case !s.ask:
		return status.Document{}, status.Fallback{}, false
	case s.router == nil:
		return status.Document{}, status.Fallback{Router: status.RouterNotRunning}, false
	}
	if fallback, ok := s.answers(ctx); !ok {
		return status.Document{}, fallback, false
	}
	doc, err := s.document(ctx, refresh)
	if err != nil {
		return status.Document{}, fallbackFrom(err), false
	}
	logger.Debug("read the router", "refresh", refresh, "healthy", doc.Router.Healthy)
	return doc, status.Fallback{}, true
}

// answers reports whether the router answers its health check within
// launch.AskTimeout, as run gives it, and why not when it doesn't.
func (s usageSource) answers(ctx context.Context) (status.Fallback, bool) {
	ctx, cancel := context.WithTimeout(ctx, launch.AskTimeout)
	defer cancel()
	_, err := s.router.Health(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within " + launch.AskTimeout.String()}, false
	}
	return fallbackFrom(err), err == nil
}

// RouterAnswers reports whether the router answers its health check within
// launch.AskTimeout, as run gives it, healthy or not: while it does, it posts
// the desktop notifications, whether its document is read or not.
func (s usageSource) RouterAnswers(ctx context.Context) bool {
	if s.router == nil {
		return false
	}
	_, ok := s.answers(ctx)
	return ok
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

// Pin has the router send every new session to the account with the given
// id, and with move, every running session too.
func (s usageSource) Pin(ctx context.Context, account string, move bool) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.Pin(ctx, router.PinRequest{Account: account, Move: move})
	return fromRouter(err)
}

// Unpin has the router route every session on its merits again.
func (s usageSource) Unpin(ctx context.Context) error {
	if s.router == nil {
		return errRouterDown
	}
	_, err := s.router.Unpin(ctx, false)
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
		Policy: policy,
		Prime:  s.prime,
		Token:  s.readToken,
		Now:    s.deps.Now,
	}
	doc := collector.Collect(ctx, s.accounts)
	logger.Debug("probed accounts", "accounts", len(doc.Accounts), "best", doc.Best, "claude_version", version)
	return doc, nil
}
