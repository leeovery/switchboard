package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// PinHeader pins a request to an account, by id: run --account sets it
// through ANTHROPIC_CUSTOM_HEADERS. It never goes upstream.
const PinHeader = "X-Switchboard-Account"

const (
	// maxBody caps the body of a routed request, which is held in memory.
	maxBody = 64 << 20
	// refusalShown is how many characters of the upstream's reason for
	// refusing a token the log shows.
	refusalShown = 200
)

// proxy sends each request on to the upstream. A request it routes goes out
// on the account chosen for it, and on others while that one can't serve it,
// and each account's usage is read off its answers; anything else goes as it
// came.
type proxy struct {
	upstream  *url.URL
	transport http.RoundTripper
	accounts  accounts
	// readToken reads an account's token from its file again, as when the
	// upstream refuses the one the router has.
	readToken func(id string) (tokens.Token, error)
	// tokensReplaced hears that an account's token has been replaced, which
	// the state file is to keep.
	tokensReplaced func()
	state          *state
	provider       Provider
	chooser        Chooser
	health         *health
	emit           func(Event)
	now            func() time.Time
	// errorLog takes what the reverse proxy reports itself, such as an
	// upstream failing mid-stream.
	errorLog *log.Logger
}

// exchange is one request on its way through the proxy, and what's known of
// it so far.
type exchange struct {
	// req is what a routed request's account is chosen on, account the account
	// it goes out on, and reason why: all three are zero for a request passed
	// through.
	req     Request
	account account
	reason  string
	// id ties a routed request's log lines together.
	id      string
	started time.Time
	// arrived is when a routed request arrived, by the router's clock.
	arrived time.Time
	// attempts counts the times a routed request has gone upstream.
	attempts int
	// status is what the client was answered, or zero before it's known.
	status int
	// failed is set when the router answered the request with a failure of
	// its own.
	failed bool
	// newSession is set when a routed request's first choice said its session
	// was new.
	newSession bool
}

func (ex *exchange) routed() bool {
	return ex.account.ID != ""
}

// succeeded reports whether the client was answered with success.
func (ex *exchange) succeeded() bool {
	return succeeded(ex.status)
}

// succeeded reports whether status is success.
func succeeded(status int) bool {
	return status >= 200 && status < 300
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if client, ok := p.routable(r); ok {
		p.route(w, r, client)
		return
	}
	p.passThrough(w, r)
}

// routable returns the account whose token a request carries, or carried
// before it was replaced, and reports whether the request may go out on
// another: only when it's to a path the provider routes and its token is, or
// was, one of the accounts', so a process that doesn't hold a token can't
// borrow one.
func (p *proxy) routable(r *http.Request) (account, bool) {
	if !p.provider.Routable(r.URL.Path) {
		return account{}, false
	}
	return p.accounts.byToken(bearer(r.Header), p.now())
}

// route sends a request on the account the chooser picks for it, and on
// others while that one can't serve it. When the chooser picks none, as only
// accounts held back by their reserves are left, the router answers as the
// upstream would at a limit.
func (p *proxy) route(w http.ResponseWriter, r *http.Request, client account) {
	started := time.Now()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		refuseBody(w, r, err)
		return
	}
	ex := &exchange{id: newID(), started: started, arrived: p.now()}
	ex.req = p.request(r, body, ex, client)
	choice := p.chooser.Choose(r.Context(), ex.req)
	ex.newSession = choice.New
	defer p.done(r, ex)
	if choice.Reserved {
		p.reserved(w, ex, choice)
		return
	}
	ex.account, ex.reason = p.chosen(ex, choice, client)
	p.forward(w, withBody(r, body), ex)
}

// request is what the chooser is to know of a routed request, whose body is
// body, sent by client's token: its session, its model and whether the
// model's thinking is bound to its account, and its pin.
func (p *proxy) request(r *http.Request, body []byte, ex *exchange, client account) Request {
	model := p.provider.Model(body)
	return Request{
		ID:      ex.id,
		Session: p.provider.Session(r.Header),
		Model:   model,
		Bound:   p.provider.ThinkingBound(model),
		Pin:     p.pin(r, ex),
		Client:  client.ID,
	}
}

// passThrough sends a request upstream as it came.
func (p *proxy) passThrough(w http.ResponseWriter, r *http.Request) {
	ex := &exchange{started: time.Now()}
	defer func() {
		logger.Debug("passed through", "method", r.Method, "path", r.URL.Path, "status", ex.status)
	}()
	p.forward(w, r, ex)
}

// pin returns the account a request's pin header names, when it names one the
// router can send on. It warns of a pin it ignores.
func (p *proxy) pin(r *http.Request, ex *exchange) string {
	id := r.Header.Get(PinHeader)
	if id == "" {
		return ""
	}
	a, ok := p.accounts.byID(id)
	switch {
	case !ok:
		logger.Warn("pin ignored: no such account", "id", ex.id, "pin", id)
	case !a.hasToken():
		logger.Warn("pin ignored: account has no usable token", "id", ex.id, "pin", id)
	default:
		return id
	}
	return ""
}

// chosen returns the account a request goes out on first, as the chooser
// chose it, and why. Should it name one the router can't send on, the request
// keeps to its client's.
func (p *proxy) chosen(ex *exchange, choice Choice, client account) (account, string) {
	if a, ok := p.accounts.byID(choice.Account); ok && a.hasToken() {
		return a, choice.Reason
	}
	logger.Error("chooser picked an account it can't send on; keeping the client's", "id", ex.id, "picked", choice.Account)
	return client, "client"
}

// reserved answers a request that no account can take without spending its
// reserve, which the router never does, as the upstream answers one over its
// account's limit: a 429, shaped as the API shapes its errors, whose usage
// headers reject it until the first of those accounts has room again, as far
// as that's known, as Claude Code reads a limit off them.
func (p *proxy) reserved(w http.ResponseWriter, ex *exchange, choice Choice) {
	ex.reason, ex.status = choice.Reason, http.StatusTooManyRequests
	attrs := []any{"id", ex.id}
	if !choice.Back.IsZero() {
		attrs = append(attrs, "until", choice.Back)
	}
	logger.Warn("no account has room outside its reserve; answering 429", attrs...)
	p.provider.MarkLimited(w.Header(), choice.Back)
	writeError(w, ex.status, "rate_limit_error",
		"switchboard: no account has room outside its reserve, which switchboard leaves unused but on the accounts pinned (switchboard pin <id>...)")
}

// next asks the chooser which account a request goes out on after those it
// has been tried on, and why. It reports false when none other has room for
// it.
func (p *proxy) next(ctx context.Context, req Request) (account, string, bool) {
	choice := p.chooser.Choose(ctx, req)
	a, ok := p.accounts.byID(choice.Account)
	if _, tried := req.attempt(a.ID); choice.NoRoom || !ok || !a.hasToken() || tried {
		return account{}, "", false
	}
	return a, choice.Reason, true
}

// forward sends a request upstream and its answer back, for ex: a routed one
// as its replay has it.
func (p *proxy) forward(w http.ResponseWriter, r *http.Request, ex *exchange) {
	transport := p.transport
	if ex.routed() {
		transport = &replay{p: p, ex: ex}
	}
	rp := &httputil.ReverseProxy{
		Rewrite:       p.rewrite,
		Transport:     transport,
		FlushInterval: -1,
		ErrorLog:      p.errorLog,
		ModifyResponse: func(resp *http.Response) error {
			ex.status = resp.StatusCode
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) { p.fail(w, r, ex, err) },
	}
	rp.ServeHTTP(w, r)
}

// rewrite points a request at the upstream. A routed request's replay sets its
// token, as each attempt's account has it.
func (p *proxy) rewrite(pr *httputil.ProxyRequest) {
	// Rewrite drops query parameters it can't parse. The router never reads
	// the query, so the upstream gets it as the client sent it.
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetURL(p.upstream)
	pr.Out.Header.Del(PinHeader)
}

// refusedError is the upstream refusing a routed request on the account it
// went out on last, answering with status, when there was no other account to
// send it on. Claude Code takes a 401 or 403 as its own login failing, and
// drops it on a 403, but a routed request's token needn't be its own: so
// neither is relayed.
type refusedError struct {
	status int
}

func (e refusedError) Error() string {
	return fmt.Sprintf("upstream answered HTTP %d", e.status)
}

// fail answers a request the upstream didn't answer, or answered with a
// refusal the client mustn't see: a 502, shaped as the API shapes its errors.
func (p *proxy) fail(w http.ResponseWriter, r *http.Request, ex *exchange, err error) {
	if r.Context().Err() != nil {
		// The client has gone, so there's no one to answer.
		return
	}
	ex.status, ex.failed = http.StatusBadGateway, true
	if refused, ok := errors.AsType[refusedError](err); ok {
		// The API's clients retry a 5xx unless told not to, and the same
		// token would only be refused again.
		w.Header().Set("X-Should-Retry", "false")
		writeError(w, ex.status, "api_error", fmt.Sprintf("switchboard: the upstream refused account %s (HTTP %d)", ex.account.ID, refused.status))
		return
	}
	logger.Error("upstream request failed", append(ex.identity(r), "error", err)...)
	writeError(w, ex.status, "api_error", "switchboard: the request to the upstream failed: "+err.Error())
}

// identity names a request in the log: a routed one by its id and account,
// any other by its method and path.
func (ex *exchange) identity(r *http.Request) []any {
	if ex.routed() {
		return []any{"id", ex.id, "account", ex.account.ID}
	}
	return []any{"method", r.Method, "path", r.URL.Path}
}

// done notes a routed request once it's done: in the log, and, once it was
// answered, in the router's health. A new session's request that wasn't
// answered with success has the chooser forget the session: it's remembered
// once it's answered.
func (p *proxy) done(r *http.Request, ex *exchange) {
	p.logRouted(r, ex)
	if ex.status != 0 {
		p.health.record(ex.arrived, ex.failed)
	}
	if ex.newSession && !ex.succeeded() {
		p.chooser.Forget(ex.req)
	}
}

// logRouted notes a routed request once it's done, how many times it went
// upstream when that was more than once, and whether its client went away
// before then.
func (p *proxy) logRouted(r *http.Request, ex *exchange) {
	attrs := []any{
		"id", ex.id,
		"session", status.ShortID(ex.req.Session),
		"model", ex.req.Model,
		"account", ex.account.ID,
		"reason", ex.reason,
		"status", ex.status,
	}
	if ex.attempts > 1 {
		attrs = append(attrs, "attempts", ex.attempts)
	}
	attrs = append(attrs, "duration", time.Since(ex.started).Round(time.Millisecond))
	if r.Context().Err() != nil {
		attrs = append(attrs, "canceled", true)
	}
	logger.Info("routed", attrs...)
}

// refuseBody answers a routed request whose body couldn't be read in full: 413
// when it's over the cap.
func refuseBody(w http.ResponseWriter, r *http.Request, err error) {
	logger.Warn("request refused: body unread", "method", r.Method, "path", r.URL.Path, "error", err)
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large",
			fmt.Sprintf("switchboard: the request body is over its limit of %d MiB", maxBody>>20))
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_request_error", "switchboard: couldn't read the request body")
}

// apiError is an error shaped as the API shapes its own, so Claude Code shows
// its message.
type apiError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// writeError answers with an error of the API's type kind.
func writeError(w http.ResponseWriter, status int, kind, message string) {
	body := apiError{Type: "error"}
	body.Error.Type, body.Error.Message = kind, message
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// withBody returns a shallow copy of r whose body reads body. It can be read
// again, which lets a routed request go out more than once, and the transport
// resend a request the upstream never began on, as when an HTTP/2 connection
// closes under it.
func withBody(r *http.Request, body []byte) *http.Request {
	r = r.WithContext(r.Context())
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return r
}

// newID returns a short id to tie a request's log lines together.
func newID() string {
	return fmt.Sprintf("%08x", rand.Uint32())
}
