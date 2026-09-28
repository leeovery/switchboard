package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
)

const (
	// maxBody caps the body of a routed request, which is held in memory.
	maxBody = 64 << 20
	// pinHeader pins a request to an account, by id: run --account sets it
	// through ANTHROPIC_CUSTOM_HEADERS. It never goes upstream.
	pinHeader = "X-Switchboard-Account"
	// sessionShown is how many characters of a session's id the log shows.
	sessionShown = 8
	// refusalShown is how many characters of the upstream's reason for
	// refusing a token the log shows.
	refusalShown = 200
)

// proxy sends each request on to the upstream. A request it routes goes out
// on the account chosen for it, and the account's usage is read off its
// response; anything else goes as it came.
type proxy struct {
	upstream  *url.URL
	transport http.RoundTripper
	accounts  accounts
	state     *state
	provider  Provider
	chooser   Chooser
	// errorLog takes what the reverse proxy reports itself, such as an
	// upstream failing mid-stream.
	errorLog *log.Logger
}

func newProxy(upstream *url.URL, accounts accounts, state *state, provider Provider, chooser Chooser) *proxy {
	return &proxy{
		upstream:  upstream,
		transport: newTransport(),
		accounts:  accounts,
		state:     state,
		provider:  provider,
		chooser:   chooser,
		errorLog:  logs.StdLogger("router", slog.LevelWarn),
	}
}

// exchange is one request on its way through the proxy, and what's known of
// it so far.
type exchange struct {
	// account is the account a routed request goes out on, and reason why;
	// both are zero for a request passed through.
	account account
	reason  string
	// id ties a routed request's log lines together.
	id      string
	session string
	model   string
	started time.Time
	// status is what the client was answered, or zero before it's known.
	status int
}

func (ex *exchange) routed() bool {
	return ex.account.ID != ""
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if client, ok := p.routable(r); ok {
		p.route(w, r, client)
		return
	}
	p.passThrough(w, r)
}

// routable returns the account whose token a request carries, and reports
// whether the request may go out on another: only when it's to a path the
// provider routes and its token is one of the accounts', so a process that
// doesn't hold a token can't borrow one.
func (p *proxy) routable(r *http.Request) (account, bool) {
	if !p.provider.Routable(r.URL.Path) {
		return account{}, false
	}
	return p.accounts.byToken(bearer(r.Header))
}

// route sends a request on the account the chooser picks for it.
func (p *proxy) route(w http.ResponseWriter, r *http.Request, client account) {
	started := time.Now()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		refuseBody(w, r, err)
		return
	}
	ex := &exchange{id: newID(), session: p.provider.Session(r.Header), model: p.provider.Model(body), started: started}
	ex.account, ex.reason = p.choose(r.Context(), ex, p.pin(r, ex), client)
	defer p.logRouted(r, ex)
	p.forward(w, withBody(r, body), ex)
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
	id := r.Header.Get(pinHeader)
	if id == "" {
		return ""
	}
	a, ok := p.accounts.byID(id)
	switch {
	case !ok:
		logger.Warn("pin ignored: no such account", "id", ex.id, "pin", id)
	case !a.hasToken:
		logger.Warn("pin ignored: account has no token", "id", ex.id, "pin", id)
	default:
		return id
	}
	return ""
}

// choose asks the chooser which account a request goes out on, and why. Should
// it name one the router can't send on, the request keeps to its client's.
func (p *proxy) choose(ctx context.Context, ex *exchange, pin string, client account) (account, string) {
	choice := p.chooser.Choose(ctx, Request{Session: ex.session, Model: ex.model, Pin: pin, Client: client.ID})
	if a, ok := p.accounts.byID(choice.Account); ok && a.hasToken {
		return a, choice.Reason
	}
	logger.Error("chooser picked an account it can't send on; keeping the client's", "id", ex.id, "picked", choice.Account)
	return client, "client"
}

// forward sends a request upstream and its response back, for ex.
func (p *proxy) forward(w http.ResponseWriter, r *http.Request, ex *exchange) {
	rp := &httputil.ReverseProxy{
		Rewrite:        func(pr *httputil.ProxyRequest) { p.rewrite(pr, ex) },
		Transport:      p.transport,
		FlushInterval:  -1,
		ErrorLog:       p.errorLog,
		ModifyResponse: func(resp *http.Response) error { return p.inspect(resp, ex) },
		ErrorHandler:   func(w http.ResponseWriter, r *http.Request, err error) { p.fail(w, r, ex, err) },
	}
	rp.ServeHTTP(w, r)
}

// rewrite points a request at the upstream, and a routed one at its account.
func (p *proxy) rewrite(pr *httputil.ProxyRequest, ex *exchange) {
	// Rewrite drops query parameters it can't parse. The router never reads
	// the query, so the upstream gets it as the client sent it.
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.SetURL(p.upstream)
	pr.Out.Header.Del(pinHeader)
	if ex.routed() {
		pr.Out.Header.Set("Authorization", "Bearer "+ex.account.token.Reveal())
	}
}

// inspect reads a response before it goes back to the client. A routed
// request's response reports its account's usage, whatever its status, and
// must not reach the client when it refuses the account's token.
func (p *proxy) inspect(resp *http.Response, ex *exchange) error {
	if ex.routed() {
		if windows := p.provider.Usage(resp.Header); len(windows) > 0 {
			p.state.record(ex.account.ID, windows, fromResponse)
		}
		if refuses(resp.StatusCode) {
			message := p.provider.ErrorMessage(resp.Body, ex.account.token.Reveal())
			return refusal{status: resp.StatusCode, message: prefix(message, refusalShown)}
		}
	}
	ex.status = resp.StatusCode
	return nil
}

// refuses reports whether a status refuses the token a request went out on.
// Claude Code takes either as its own login failing, and drops it on a 403,
// but a routed request's token needn't be its own: so neither is relayed.
func refuses(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

// refusal is the upstream refusing the token a routed request went out on.
type refusal struct {
	status int
	// message is the upstream's reason, such as a model the account's plan
	// doesn't include, for the log alone.
	message string
}

func (e refusal) Error() string {
	return fmt.Sprintf("upstream answered HTTP %d", e.status)
}

// fail answers a request the upstream didn't answer, or answered with a
// refusal the client mustn't see: a 502, shaped as the API shapes its errors.
func (p *proxy) fail(w http.ResponseWriter, r *http.Request, ex *exchange, err error) {
	if refused, ok := errors.AsType[refusal](err); ok {
		logger.Error("upstream refused the account's token", "id", ex.id, "account", ex.account.ID, "status", refused.status, "error", refused.message)
		ex.status = http.StatusBadGateway
		// The API's clients retry a 5xx unless told not to, and the same
		// token would only be refused again.
		w.Header().Set("X-Should-Retry", "false")
		writeError(w, ex.status, "api_error", fmt.Sprintf("switchboard: the upstream refused account %s (HTTP %d)", ex.account.ID, refused.status))
		return
	}
	if r.Context().Err() != nil {
		// The client has gone, so there's no one to answer.
		return
	}
	logger.Error("upstream request failed", append(ex.identity(r), "error", err)...)
	ex.status = http.StatusBadGateway
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

// logRouted notes a routed request once it's done, and whether its client
// went away before then.
func (p *proxy) logRouted(r *http.Request, ex *exchange) {
	attrs := []any{
		"id", ex.id,
		"session", prefix(ex.session, sessionShown),
		"model", ex.model,
		"account", ex.account.ID,
		"reason", ex.reason,
		"status", ex.status,
		"duration", time.Since(ex.started).Round(time.Millisecond),
	}
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
// again, which lets the transport resend a request the upstream never began
// on, as when an HTTP/2 connection closes under it.
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

// prefix is s cut to its first n characters.
func prefix(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n])
	}
	return s
}
