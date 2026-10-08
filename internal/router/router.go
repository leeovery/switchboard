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
	"github.com/leeovery/switchboard/internal/ledger"
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
	// Spends reports whether a request to path spends its account's quota:
	// the success of one that doesn't says nothing of the account's limits.
	Spends(path string) bool
	// Session returns the id of the session a request belongs to, or "" when
	// it doesn't say.
	Session(h http.Header) string
	// Betas returns the features a request's header asks the API for beyond
	// its version, some of which change what a request costs.
	Betas(h http.Header) []string
	// Asks reads a request's body for the model it asks for, or "" when it
	// doesn't say; whether it's the client's quota check: a request that
	// asks nothing of the model, but spends a token to see the account has
	// quota; and its shape, as the request ledger keeps it, the zero Shape
	// where the body doesn't read as a request.
	Asks(body []byte) (model string, check bool, shape ledger.Shape)
	// Family returns the family a model belongs to. A window reported on a
	// response to one of a family's models counts all of theirs.
	Family(model string) string
	// ThinkingBound reports whether the thinking a model produces is bound to
	// the account that produced it, so a session on the model moved to
	// another account carries on without its earlier reasoning.
	ThinkingBound(model string) bool
	// Usage reads the usage a response's headers report: the account's
	// windows, and its extra usage.
	Usage(h http.Header) quota.Usage
	// Classify says what a response, by its status and headers, says of the
	// account the request went out on: whether its limit is reached, it's
	// throttled or its token refused, or the response is the client's as it
	// came.
	Classify(status int, h http.Header) quota.Outcome
	// MarkLimited sets the headers with which the upstream refuses a request
	// over its account's limit, until until, or until a time unknown when
	// that's zero, as the client reads a limit off them.
	MarkLimited(h http.Header, until time.Time)
	// ErrorMessage returns the message of the error a response's body holds,
	// with token and anything else shaped like one hidden, or "" when it
	// holds none.
	ErrorMessage(body io.Reader, token string) string
	// Count reads an answer, by its header, and its body, decoded, as far as
	// it needs, by its content type, calling chars with how many characters
	// of text, thinking and tools' input it has streamed so far as each part
	// of them comes. It returns the tokens the answer's closing usage gives,
	// nil when none came, as for an answer cut short, and what the request
	// ledger keeps of the answer.
	Count(h http.Header, body io.Reader, chars func(int)) (*quota.Tokens, ledger.Reply)
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
	// the router starts, again every so often while Run runs, and whenever
	// the upstream refuses the token it has, which may have been replaced
	// since.
	Token func(id string) (tokens.Token, error)
	// Upstream is the API's base URL, such as https://api.anthropic.com.
	Upstream string
	Provider Provider
	// Prober reads the usage of accounts the router has had no traffic for,
	// and primes them.
	Prober Prober
	// Policy is the provider's say in which account is best, and which
	// window a request starts.
	Policy score.Policy
	// Prime says when priming starts the accounts' windows: while Run runs,
	// the router primes them on its schedule, and its status gives it.
	Prime config.Prime
	// Now reads the clock as time.Now does: the wall clock, which the router
	// goes by, and the monotonic, which it tells the Mac's sleeps by.
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
	// History says how long the readings history is kept: a zero Keep keeps
	// it for config.DefaultHistoryKeep.
	History config.History
	// Ledger says how long the request ledger's lines are kept: a zero Keep
	// keeps them for config.DefaultLedgerKeep.
	Ledger config.Ledger
	// Listen is the proxy's address, and StateDir the directory its control
	// socket and state file go in: only Run uses them.
	Listen   string
	StateDir string
	// ConfigFile is the config file the router was started from, as Watch
	// found it before the config was read from it: once it makes another
	// valid config while Run runs, the router restarts to take it up. The
	// zero Watched is none.
	ConfigFile Watched
	// Binary is the switchboard binary the router was started as, by the
	// path it was run by, such as the Homebrew link the service runs, as
	// Watch found it before the config was read: once that leads to another
	// file than the one running while Run runs, as after an upgrade, the
	// router restarts. The zero Watched is none.
	Binary Watched
	// Zone is the file the system's time zone is read from, /etc/localtime,
	// as Watch found it as the router started: a program reads the time zone
	// once, as it starts, so once that leads to another file, as when the Mac
	// is taken to another time zone, the router restarts, for the priming
	// schedule to keep to the clock's times of day. The zero Watched is none.
	Zone Watched
	// Supervised is set when the router is started again whenever it exits,
	// as launchd starts the service's: it restarts by replacing itself, as
	// Exec says, or else by stopping as it does when ctx ends. A router that
	// isn't logs, once, that a restart is due.
	Supervised bool
	// Exec replaces this process with the program at path, run as argv with
	// env, as launch.Exec does: a supervised router restarts by replacing
	// itself with its binary, by Binary's path, run as Args with Environ,
	// handing its listeners over to the router it becomes, as Handed names
	// them there. It returns only when it fails, and the router then stops,
	// as it does with none, for launchd to start it again.
	Exec func(path string, argv, env []string) error
	// Args and Environ are the command line and the environment the router
	// was started with, as os.Args and os.Environ give them.
	Args    []string
	Environ []string
	// Handed names the listeners handed over by the router this one
	// replaced, as handover.Variable's value: the router takes them up where
	// they're at the addresses it listens on, in place of listening afresh.
	// "" is none.
	Handed string
	// ExecRetry is how long the router waits to exec its binary again while
	// it isn't there, as for a moment while an upgrade moves its link on.
	// Zero means half a second.
	ExecRetry time.Duration
	// WatchEvery is how often, while Run runs, the router reads the token
	// files again, and looks at the config file, the binary and the time
	// zone's file. Zero means every 3 seconds.
	WatchEvery time.Duration
	// DrainFor is how long requests in flight get to finish once Run's
	// router is stopping, before it cuts them off. Zero means 30 seconds.
	DrainFor time.Duration
}

// notifying reports whether the router posts notifications: it has a
// notifier, and some to post.
func (c Config) notifying() bool {
	return c.Notifier != nil && c.Notifications != (config.Notifications{})
}

// Router is switchboard's router: the proxy, the scheduler that chooses the
// account each request goes out on, the live state of every account's usage,
// the router's own health, what has happened lately, the request stream of
// what befalls each request as it happens, the request ledger of each once
// it's done, the control API that reports on it all, the desktop
// notifications of what befalls the accounts, and its upkeep, which keeps it
// in step with what it was started from.
type Router struct {
	cfg      Config
	upstream *url.URL
	accounts accounts
	state    *state
	sessions *sessions
	// recent keeps the router's newest events.
	recent *recent
	// stream tells its readers of what befalls each routed request as it
	// happens.
	stream *stream
	// file keeps what should outlast the router, once Run has loaded it.
	file *stateFile
	// history keeps each account's readings as they change, and ledger each
	// routed request's line, once Run has opened them.
	history *history
	ledger  *requestLedger
	probes  *probes
	health  *health
	proxy   *proxy
	// primer is nil when priming is off.
	primer *primer
	// inFlight counts the proxy's requests in flight.
	inFlight *inFlight
	upkeep   *upkeep
	// notifications is nil when the router posts none.
	notifications *notifications
	started       time.Time
	// proxyAddr is the address the proxy listens on, once Run has it
	// listening.
	proxyAddr string
}

// New builds a router for the accounts configured. An account without a
// usable token is listed, but nothing goes out on it until its token file
// holds one, even when that's every account.
func New(cfg Config) (*Router, error) {
	clock := cfg.Now
	cfg.Now = wallClock(cfg.Now)
	accounts := resolve(cfg.Accounts, cfg.Token)
	upstream, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	changes := newChanges()
	state := newState(accounts, cfg.Policy, cfg.Provider.Family, cfg.Now, changes.note, changes.routine)
	history := newHistory(cfg.History, cfg.Now)
	state.history = history.note
	sessions := newSessions(cfg.Now, changes.note, changes.routine)
	sessions.bound = cfg.Provider.ThinkingBound
	recent := newRecent(state, sessions, cfg.Now)
	listeners := []func(Event){cfg.Events, recent.hear}
	var notices *notifications
	if cfg.notifying() {
		notices = newNotifications(cfg.Notifications, cfg.Notifier, state, cfg.Now)
		listeners = append(listeners, notices.hear)
	}
	emit := hearing(listeners...)
	probes := newProbes(cfg.Prober, state, cfg.Now, emit)
	health := newHealth(cfg.Now, emit)
	recent.judge = health.look
	scheduler := &scheduler{accounts: accounts, state: state, sessions: sessions, probes: probes, now: cfg.Now, emit: emit}
	primer := newPrimer(cfg.Prime, accounts, state, probes, cfg.Now)
	inFlight := newInFlight()
	stream := newStream(cfg.Now)
	requests := newRequestLedger(cfg.Ledger, cfg.Now)
	history.pruning = requests.summariseEnded
	transport := newPool()
	awake := &wakes{now: clock, woke: func() {
		transport.renew()
		health.wake()
	}}
	return &Router{
		cfg:      cfg,
		upstream: upstream,
		accounts: accounts,
		state:    state,
		sessions: sessions,
		recent:   recent,
		stream:   stream,
		file:     newStateFile(cfg.Now, changes, sessions, accounts, state),
		history:  history,
		ledger:   requests,
		probes:   probes,
		health:   health,
		proxy: &proxy{
			upstream:       upstream,
			transport:      transport,
			accounts:       accounts,
			readToken:      cfg.Token,
			tokensReplaced: changes.note,
			state:          state,
			provider:       cfg.Provider,
			chooser:        scheduler,
			health:         health,
			emit:           emit,
			stream:         stream,
			ledger:         requests,
			routing:        newInFlight(),
			now:            cfg.Now,
			errorLog:       logs.StdLogger("router", slog.LevelWarn),
		},
		primer:        primer,
		inFlight:      inFlight,
		upkeep:        newUpkeep(cfg, accounts, state, changes, primer, inFlight, awake, emit),
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
	return r.inFlight.count(r.proxy)
}

// Status reports every account's usage as the router knows it, with how many
// sessions each has, and all have, the best account to use next, the priming
// schedule, with when it next primes each account, the global pin, the
// router's own health, a restart it has due, and what has happened lately.
func (r *Router) Status() status.Document {
	now := r.cfg.Now()
	pin := r.sessions.globalPin()
	doc := r.state.document(pin.Accounts...)
	doc.Pin = pin
	doc.Router = r.health.report()
	doc.Restart = r.upkeep.restartDue()
	doc.Events = r.recent.events()
	if r.primer != nil {
		doc.Prime = r.primer.report(now)
	}
	var byAccount map[string]int
	byAccount, doc.Sessions = r.sessions.active(now)
	for i, a := range doc.Accounts {
		doc.Accounts[i].Sessions = byAccount[a.ID]
	}
	return doc
}
