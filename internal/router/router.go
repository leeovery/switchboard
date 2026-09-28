// Package router is the proxy Claude Code sends its requests to. It sends
// each on to the upstream, on the account chosen for it; reads every
// account's usage off the responses, and probes where there's no traffic to
// read; and reports what it knows over a control socket.
package router

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

var logger = logs.For("router")

// Provider is what the router knows of the API it fronts. internal/claude's
// Provider is Claude's.
type Provider interface {
	// Routable reports whether a request to path may go out on another
	// account's token than the one it carries.
	Routable(path string) bool
	// Session returns the id of the session a request belongs to, or "" when
	// it doesn't say.
	Session(h http.Header) string
	// Model returns the model a request's body asks for, or "" when it doesn't
	// say.
	Model(body []byte) string
	// Usage reads the usage windows a response's headers report.
	Usage(h http.Header) []quota.Window
}

// Prober reads an account's usage by spending a request on its token.
type Prober interface {
	Probe(ctx context.Context, token string) (quota.Usage, error)
}

// Config is what a router is built from.
type Config struct {
	// Accounts are the configured accounts, in the order they're shown.
	Accounts []config.Account
	// Getenv reads the variables holding the accounts' tokens.
	Getenv func(key string) string
	// Upstream is the API's base URL, such as https://api.anthropic.com.
	Upstream string
	Provider Provider
	// Prober reads the usage of accounts the router has had no traffic for.
	Prober Prober
	// Policy is the provider's say in which account is best.
	Policy score.Policy
	// Now reads the wall clock.
	Now func() time.Time
	// Version is switchboard's, which the control API reports.
	Version string
	// Listen is the proxy's address, and StateDir the directory its control
	// socket goes in: only Run uses them.
	Listen   string
	StateDir string
}

// Router is switchboard's router: the proxy, the live state of every
// account's usage, and the control API that reports on it.
type Router struct {
	cfg      Config
	upstream *url.URL
	accounts accounts
	state    *state
	proxy    *proxy
	started  time.Time
}

// New builds a router for the accounts configured. An account without a token
// is listed, but nothing goes out on it; New fails when no account has one,
// as there'd be nothing to route to.
func New(cfg Config) (*Router, error) {
	accounts := resolve(cfg.Accounts, cfg.Getenv)
	if !accounts.anyToken() {
		return nil, fmt.Errorf("no account has a token, so there's nothing to route to: set %s", strings.Join(accounts.tokenEnvs(), " or "))
	}
	upstream, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	state := newState(accounts, cfg.Policy, cfg.Now)
	return &Router{
		cfg:      cfg,
		upstream: upstream,
		accounts: accounts,
		state:    state,
		proxy:    newProxy(upstream, accounts, state, cfg.Provider, pinOrClient{}),
		started:  cfg.Now().UTC(),
	}, nil
}

// Proxy is the handler Claude Code's requests come to.
func (r *Router) Proxy() http.Handler {
	return r.proxy
}

// Status reports every account's usage as the router knows it, and the best
// account to use next.
func (r *Router) Status() status.Document {
	return r.state.document()
}

// probeAll probes every account with a token, all at once, so the router knows
// the usage of accounts it hasn't had traffic for.
func (r *Router) probeAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, a := range r.accounts {
		if a.hasToken {
			wg.Go(func() { r.probe(ctx, a) })
		}
	}
	wg.Wait()
}

// probe reads an account's usage, and logs how that went.
func (r *Router) probe(ctx context.Context, a account) {
	started := time.Now()
	usage, err := r.cfg.Prober.Probe(ctx, a.token.Reveal())
	took := time.Since(started).Round(time.Millisecond)
	if ctx.Err() != nil {
		// Stopped mid-probe: its failure says nothing of the account.
		return
	}
	r.state.recordProbe(a.ID, usage, err)
	if err != nil {
		logger.Warn("probe failed", "account", a.ID, "duration", took, "error", err)
		return
	}
	logger.Debug("probed account", "account", a.ID, "duration", took, "windows", len(usage.Windows))
	for _, f := range usage.Failures {
		logger.Warn("window unread", "account", a.ID, "window", f.Window, "error", f.Error)
	}
}
