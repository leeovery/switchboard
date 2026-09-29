// Package router is the proxy Claude Code sends its requests to. It sends
// each on to the upstream, on the account chosen for it, keeping a session on
// one account while it can; reads every account's usage off the responses,
// and probes where there's no traffic to read; and reports what it knows, and
// takes pins, over a control socket.
package router

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
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
	// Family returns the family a model belongs to. A window reported on a
	// response to one of a family's models counts all of theirs.
	Family(model string) string
	// Usage reads the usage windows a response's headers report.
	Usage(h http.Header) []quota.Window
	// Classify says what a response, by its status and headers, says of the
	// account the request went out on: whether its limit is reached, it's
	// throttled or its token refused, or the response is the client's as it
	// came.
	Classify(status int, h http.Header) quota.Outcome
	// ErrorMessage returns the message of the error a response's body holds,
	// with token and anything else shaped like one hidden, or "" when it
	// holds none.
	ErrorMessage(body io.Reader, token string) string
}

// Prober reads an account's usage by spending requests on its token, and
// says which models reported each window.
type Prober interface {
	Probe(ctx context.Context, token string) (quota.Probe, error)
}

// Config is what a router is built from.
type Config struct {
	// Accounts are the configured accounts, in the order they're shown.
	Accounts config.Accounts
	// Token reads an account's token, by the account's id, from its file: as
	// the router starts, and again when the upstream refuses the token it
	// has, which may have been replaced since.
	Token func(id string) (tokens.Token, error)
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
	// Events hears each of the router's events as it happens, on the
	// goroutine it happens on: see Event. It mustn't block, nor call the
	// router, but can hand an event on to be dealt with elsewhere. Nil hears
	// nothing.
	Events func(Event)
	// Notifier posts the desktop notifications Notifications asks for, while
	// Run runs. Nil posts none.
	Notifier      Notifier
	Notifications config.Notifications
	// Listen is the proxy's address, and StateDir the directory its control
	// socket and state file go in: only Run uses them.
	Listen   string
	StateDir string
}

// notifying reports whether the router posts notifications: it has a
// notifier, and some to post.
func (c Config) notifying() bool {
	return c.Notifier != nil && c.Notifications != (config.Notifications{})
}

// Router is switchboard's router: the proxy, the scheduler that chooses the
// account each request goes out on, the live state of every account's usage,
// the router's own health, the control API that reports on it all, and the
// desktop notifications of what befalls the accounts.
type Router struct {
	cfg      Config
	upstream *url.URL
	accounts accounts
	state    *state
	sessions *sessions
	probes   *probes
	health   *health
	proxy    *proxy
	// notifications is nil when the router posts none.
	notifications *notifications
	started       time.Time
	// proxyAddr is the address the proxy listens on, once Run has it
	// listening.
	proxyAddr string
}

// New builds a router for the accounts configured. An account without a
// usable token is listed, but nothing goes out on it; New fails when no
// account has one, as there'd be nothing to route to, saying why of each.
func New(cfg Config) (*Router, error) {
	cfg.Now = wallClock(cfg.Now)
	accounts := resolve(cfg.Accounts, cfg.Token)
	if err := accounts.checkTokens(); err != nil {
		return nil, err
	}
	upstream, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	state := newState(accounts, cfg.Policy, cfg.Provider.Family, cfg.Now)
	listeners := []func(Event){cfg.Events}
	var notices *notifications
	if cfg.notifying() {
		notices = newNotifications(cfg.Notifications, cfg.Notifier, state, cfg.Now)
		listeners = append(listeners, notices.hear)
	}
	emit := hearing(listeners...)
	sessions := newSessions(cfg.Now)
	probes := newProbes(cfg.Prober, state, cfg.Now)
	health := newHealth(cfg.Now, emit)
	scheduler := &scheduler{accounts: accounts.sendable(), state: state, sessions: sessions, probes: probes, now: cfg.Now, emit: emit}
	return &Router{
		cfg:      cfg,
		upstream: upstream,
		accounts: accounts,
		state:    state,
		sessions: sessions,
		probes:   probes,
		health:   health,
		proxy: &proxy{
			upstream:  upstream,
			transport: newTransport(),
			accounts:  accounts,
			readToken: cfg.Token,
			state:     state,
			provider:  cfg.Provider,
			chooser:   scheduler,
			health:    health,
			emit:      emit,
			errorLog:  logs.StdLogger("router", slog.LevelWarn),
		},
		notifications: notices,
		started:       cfg.Now().UTC(),
	}, nil
}

// wallClock reads now without its monotonic reading, which stops while a Mac
// sleeps: kept, it would have the router take a night asleep for no time at
// all, and a session's cache for warm long after it went cold.
func wallClock(now func() time.Time) func() time.Time {
	return func() time.Time { return now().Round(0) }
}

// Proxy is the handler Claude Code's requests come to.
func (r *Router) Proxy() http.Handler {
	return r.proxy
}

// Status reports every account's usage as the router knows it, with how many
// sessions each has, and all have, the best account to use next, the global
// pin, and the router's own health.
func (r *Router) Status() status.Document {
	doc := r.state.document()
	doc.Pin = r.sessions.globalPin()
	doc.Router = r.health.report()
	var byAccount map[string]int
	byAccount, doc.Sessions = r.sessions.active(r.cfg.Now())
	for i, a := range doc.Accounts {
		doc.Accounts[i].Sessions = byAccount[a.ID]
	}
	return doc
}
