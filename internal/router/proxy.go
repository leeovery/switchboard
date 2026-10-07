package router

import (
	"bytes"
	"cmp"
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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// The headers run tells the router of a session through, setting them in
// ANTHROPIC_CUSTOM_HEADERS. None goes upstream: the router takes every header
// named with ownPrefix off a request going upstream, whatever follows it, so
// one a newer run sends never reaches the API.
const (
	// ownPrefix begins the name of each header of switchboard's own.
	ownPrefix = "X-Switchboard-"
	// PinHeader pins a request to an account, by id, as run --account asks.
	PinHeader = ownPrefix + "Account"
	// DirHeader names the directory run started claude in, as EncodeDir
	// encodes it, for the request ledger.
	DirHeader = ownPrefix + "Dir"
)

const (
	// maxBody caps the body of a routed request, which is held in memory.
	maxBody = 64 << 20
	// refusalShown is how many characters of the upstream's reason for
	// refusing a request the log shows, and the client, when no account is
	// left to try.
	refusalShown = 200
	// toldAtMost bounds what ignoredPins holds: the sessions of an account,
	// and the accounts.
	toldAtMost = 1000
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
	// stream tells of what befalls each routed request as it happens, and
	// ledger keeps a line of each once it's done.
	stream *stream
	ledger *requestLedger
	// routing counts the routed requests in flight, each until its line is
	// noted, which the router waits for as it stops.
	routing *inFlight
	now     func() time.Time
	// errorLog takes what the reverse proxy reports itself, such as an
	// upstream failing mid-stream.
	errorLog *log.Logger
	// ignored is what the log has said of the pins the proxy ignores.
	ignored ignoredPins
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
	// shape is a routed request's shape, as the request ledger keeps it.
	shape ledger.Shape
	// answered is the account whose answer the client has, as it was picked,
	// when it isn't the one the request went out on last: the first whose
	// limit the request reached, its answer held back, where every account
	// after refused it. It's zero while it is.
	answered pick
	// from is the account a routed request's session was on before the
	// request moved it: "" while it hasn't, and once its moves are taken
	// back.
	from string
	// id is a routed request's own while it runs, which ties its lines in the
	// log together, and its events in the request stream.
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
	// spends is set when a routed request spends its account's quota, as the
	// provider says of its path.
	spends bool
	// tap is the answer a routed request's client has, counted for the
	// request stream as it passes: nil while it has none.
	tap *tap
}

func (ex *exchange) routed() bool {
	return ex.account.ID != ""
}

// pick is an account a routed request went out on, by its id, and how it was
// picked: why, and the account the request had moved its session from by
// then, "" where it hadn't.
type pick struct {
	account, reason, from string
}

// picked is the account the routed request goes out on now, as it was picked.
func (ex *exchange) picked() pick {
	return pick{account: ex.account.ID, reason: ex.reason, from: ex.from}
}

// answering is the account whose answer the client has, or is to have, as it
// was picked.
func (ex *exchange) answering() pick {
	if ex.answered.account != "" {
		return ex.answered
	}
	return ex.picked()
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
// upstream would at a limit. The request counts as routing until its line is
// noted, but for one that upgrades its connection.
func (p *proxy) route(w http.ResponseWriter, r *http.Request, client account) {
	if !upgrades(r) {
		p.routing.begin()
		defer p.routing.end()
	}
	ex := &exchange{id: newID(), started: time.Now(), arrived: p.now(), spends: p.provider.Spends(r.URL.Path)}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		p.unread(w, r, ex, err)
		return
	}
	ex.req, ex.shape = p.request(r, body, ex, client)
	choice := p.chooser.Choose(r.Context(), ex.req)
	ex.newSession = choice.New
	defer p.done(r, ex)
	if choice.Reserved {
		p.reserved(w, ex, choice)
		return
	}
	ex.account, ex.reason = p.chosen(ex, choice, client)
	p.moved(ex, choice.From)
	p.forward(w, withBody(r, body), ex)
}

// request is what the chooser is to know of a routed request, whose body is
// body, sent by client's token: its session, its model and whether the
// model's thinking is bound to its account, whether it's the client's quota
// check, and its pin; and the request's shape, which the provider reads in
// the same pass over the body.
func (p *proxy) request(r *http.Request, body []byte, ex *exchange, client account) (Request, ledger.Shape) {
	model, check, shape := p.provider.Asks(body)
	session := p.provider.Session(r.Header)
	return Request{
		ID:      ex.id,
		Session: session,
		Model:   model,
		Check:   check,
		Bound:   p.provider.ThinkingBound(model),
		Pin:     p.pin(r, ex, session),
		Client:  client.ID,
	}, shape
}

// passThrough sends a request upstream as it came.
func (p *proxy) passThrough(w http.ResponseWriter, r *http.Request) {
	ex := &exchange{started: time.Now()}
	defer func() {
		logger.Debug("passed through", "method", r.Method, "path", r.URL.Path, "status", ex.status)
	}()
	p.forward(w, r, ex)
}

// pin returns the account the pin header of a request of session names, when
// it names one the router can send on. It warns of a pin it ignores, once for
// each session and account, as ignoredPins says, and notes it at debug after.
func (p *proxy) pin(r *http.Request, ex *exchange, session string) string {
	id := r.Header.Get(PinHeader)
	if id == "" {
		return ""
	}
	a, ok := p.accounts.byID(id)
	var why string
	switch {
	case !ok:
		why = "pin ignored: no such account"
	case !a.hasToken():
		why = "pin ignored: account has no usable token"
	default:
		p.ignored.honoured(id)
		return id
	}
	level := slog.LevelDebug
	if p.ignored.first(id, session) {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, why, "id", ex.id, "pin", id, "session", status.ShortID(session))
	return ""
}

// ignoredPins holds, by account, the sessions whose pins to it the log has
// warned are ignored, so it warns once for each, not at every request, until
// the account can be sent on again. Sessions come and go, and an account
// that isn't configured is never sent on, so it forgets an account's sessions
// once it holds toldAtMost of them, and every account's once it holds that
// many accounts, the log warning of each pin once more. It's safe for
// concurrent use.
type ignoredPins struct {
	mu   sync.Mutex
	told map[string]map[string]bool
}

// first reports whether a pin to account, of a request of session, is the
// first ignored since the account was last sent on.
func (p *ignoredPins) first(account, session string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.told[account][session] {
		return false
	}
	if p.told == nil || len(p.told) >= toldAtMost {
		p.told = make(map[string]map[string]bool)
	}
	if p.told[account] == nil || len(p.told[account]) >= toldAtMost {
		p.told[account] = make(map[string]bool)
	}
	p.told[account][session] = true
	return true
}

// honoured notes that a pin to account is honoured, as it can be sent on: a
// pin to it ignored after is warned of afresh.
func (p *ignoredPins) honoured(account string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.told, account)
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
// has been tried on, and returns it with the choice, which says why. It
// reports false when none other has room for it.
func (p *proxy) next(ctx context.Context, req Request) (account, Choice, bool) {
	choice := p.chooser.Choose(ctx, req)
	a, ok := p.accounts.byID(choice.Account)
	if _, tried := req.attempt(a.ID); choice.NoRoom || !ok || !a.hasToken() || tried {
		return account{}, Choice{}, false
	}
	return a, choice, true
}

// moved tells the request stream of the move of a routed request's session
// from the account given to the one the request now goes out on, as the
// chooser made it choosing that account, and notes the account the request
// moved the session from first: none when from is "".
func (p *proxy) moved(ex *exchange, from string) {
	if from != "" {
		ex.from = cmp.Or(ex.from, from)
		p.tellMoved(ex, from, ex.account.ID, ex.reason)
	}
}

// tellMoved tells the request stream of the session of the routed request ex
// moving, for its requests of the request's model, from the account with the
// id from to the one with the id to, for the reason given: the event's
// account is the one it moved to.
func (p *proxy) tellMoved(ex *exchange, from, to, reason string) {
	e := ex.event(StreamMoved)
	e.Account, e.From, e.To, e.Reason = to, from, to, reason
	p.stream.publish(e)
}

// forward sends a request upstream and its answer back, for ex: a routed one
// as its replay has it, asking only for the encodings the router can count
// an answer in, its answer counted for the request stream as it passes.
func (p *proxy) forward(w http.ResponseWriter, r *http.Request, ex *exchange) {
	transport := p.transport
	rewrite := p.rewrite
	if ex.routed() {
		transport = &replay{p: p, ex: ex}
		rewrite = func(pr *httputil.ProxyRequest) {
			p.rewrite(pr)
			countable(pr.Out.Header)
		}
	}
	rp := &httputil.ReverseProxy{
		Rewrite:       rewrite,
		Transport:     transport,
		FlushInterval: -1,
		ErrorLog:      p.errorLog,
		ModifyResponse: func(resp *http.Response) error {
			ex.status = resp.StatusCode
			if ex.routed() {
				p.answered(ex)
				p.count(ex, resp)
			}
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
	stripOwn(pr.Out.Header)
}

// stripOwn takes each header of switchboard's own off h, by its prefix,
// whatever its case.
func stripOwn(h http.Header) {
	for name := range h {
		if len(name) >= len(ownPrefix) && strings.EqualFold(name[:len(ownPrefix)], ownPrefix) {
			delete(h, name)
		}
	}
}

// countable narrows the encodings a request's Accept-Encoding offers to
// those the router can read a copy of an answer in, to count it, in the
// order the client gave them: gzip, deflate and identity. With none of those
// offered, it asks for identity. A request that offers none at all is left
// so: Go's transport then asks for gzip itself, and decodes the answer.
func countable(h http.Header) {
	offered := h.Values("Accept-Encoding")
	if len(offered) == 0 {
		return
	}
	var kept []string
	for _, value := range offered {
		for coding := range strings.SplitSeq(value, ",") {
			name, _, _ := strings.Cut(coding, ";")
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "gzip", "x-gzip", "deflate", "identity":
				kept = append(kept, strings.TrimSpace(coding))
			}
		}
	}
	h.Set("Accept-Encoding", cmp.Or(strings.Join(kept, ", "), "identity"))
}

// answered notes that a routed request's answer has come, with its status:
// one of success tells the chooser, which tells of the request's session as
// started, the first time, on the account that answered.
func (p *proxy) answered(ex *exchange) {
	if ex.succeeded() {
		p.chooser.Answered(ex.req, ex.account.ID, ex.reason)
	}
}

// refusedError is the upstream refusing a routed request on the account it
// went out on last, answering with status, for reason, when there was no
// other account to send it on. Claude Code takes a 401 or 403 as its own login
// failing, and drops it on a 403, but a routed request's token needn't be its
// own: so neither is relayed.
type refusedError struct {
	status int
	// reason is the upstream's, cut short, with anything shaped like a token
	// hidden: "" when it gave none.
	reason string
}

func (e refusedError) Error() string {
	return fmt.Sprintf("upstream answered HTTP %d", e.status)
}

// message is what the client is told of the refusal on the account with the
// given id.
func (e refusedError) message(account string) string {
	message := fmt.Sprintf("switchboard: the upstream refused account %s (HTTP %d)", account, e.status)
	if e.reason != "" {
		message += ": " + e.reason
	}
	return message
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
		writeError(w, ex.status, "api_error", refused.message(ex.account.ID))
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

// done notes a routed request once it's done: in the request stream, once it
// went upstream; in the log and the request ledger, once its answer is
// counted, as the request finished by then, as the router can cut it off
// meanwhile, stopping, though its time is taken before; and, once it was
// answered, in the router's health. A new session whose request wasn't
// answered with success, the chooser forgets first, so the line is of what
// stands.
func (p *proxy) done(r *http.Request, ex *exchange) {
	took := time.Since(ex.started)
	p.ended(ex)
	f := finishing(r, took)
	p.logRouted(ex, f)
	if ex.newSession && !ex.succeeded() {
		p.forget(ex)
	}
	p.ledger.note(p.line(ex, r.Header, f))
	if ex.status != 0 {
		p.health.record(ex.arrived, ex.failed)
	}
}

// forget has the chooser forget the accounts chosen for the routed request
// ex, as Chooser's Forget says, and returns the account its session is back
// on, "" when it has none, or one chosen since stands. Where the chooser took
// the request's moves back, the request moved its session from no account,
// not even as it went out on the account whose answer was held back for it.
func (p *proxy) forget(ex *exchange) string {
	back, forgot := p.chooser.Forget(ex.req)
	if forgot {
		ex.from, ex.answered.from = "", ""
	}
	return back
}

// finish is how a routed request finished: as long after it arrived as took,
// and, where it ended early, cut off by the router as it stopped, or
// canceled, its client gone.
type finish struct {
	took             time.Duration
	cutOff, canceled bool
}

// finishing returns how the routed request r finished, as long after it
// arrived as took: where its context has ended, what ended it first, as its
// cause says.
func finishing(r *http.Request, took time.Duration) finish {
	f := finish{took: took}
	if r.Context().Err() != nil {
		f.cutOff = errors.Is(context.Cause(r.Context()), errCutOff)
		f.canceled = !f.cutOff
	}
	return f
}

// attrs are the log's attributes of how f ended early, where it did.
func (f finish) attrs() []any {
	switch {
	case f.cutOff:
		return []any{"cut_off", true}
	case f.canceled:
		return []any{"canceled", true}
	}
	return nil
}

// ended tells the request stream of a routed request that went upstream
// being done, with what its client was answered: once its answer is counted,
// when it has one.
func (p *proxy) ended(ex *exchange) {
	if ex.attempts == 0 {
		return
	}
	done := ex.event(StreamDone)
	done.Status = ex.status
	if ex.tap != nil {
		ex.tap.end(done)
		return
	}
	p.stream.publish(done)
}

// event returns the request stream's event of the kind given of a routed
// request, as it stands: on the account whose answer the client has, once
// it has one, its session and model cut short where they run too long.
func (ex *exchange) event(kind string) StreamEvent {
	return StreamEvent{
		Kind:    kind,
		Request: ex.id,
		Attempt: ex.attempts,
		Session: bounded(ex.req.Session),
		Model:   bounded(ex.req.Model),
		Account: ex.answering().account,
		Check:   ex.req.Check,
	}
}

// boundedMost is how many bytes of a session's id or a model's an event
// gives, at most: Claude Code's are a few dozen, and a request can send one
// as long as its header or body runs to.
const boundedMost = 200

// bounded is s cut to boundedMost bytes at most, at the end of a character.
func bounded(s string) string {
	return prose.TruncateBytes(s, boundedMost)
}

// logRouted notes a routed request once it's done, as f finished it: how long
// after it arrived, how many times it went upstream when that was more than
// once, and whether it was cut off, or its client went away, before its end.
func (p *proxy) logRouted(ex *exchange, f finish) {
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
	attrs = append(attrs, "duration", f.took.Round(time.Millisecond))
	logger.Info("routed", append(attrs, f.attrs()...)...)
}

// unread answers the routed request ex, whose body couldn't be read in full:
// 413 when it's over the cap, else 400, shaped as the API shapes its errors,
// whoever is left to read it, as a client that closed its side of the
// connection still reads. It logs the refusal, as the request finished, and
// notes the request's line in the request ledger, of what's known of it
// without the body: of no account, as the router answered it, and no shape.
func (p *proxy) unread(w http.ResponseWriter, r *http.Request, ex *exchange, err error) {
	f := finishing(r, time.Since(ex.started))
	logger.Warn("request refused: body unread", append([]any{"id", ex.id, "method", r.Method, "path", r.URL.Path, "error", err}, f.attrs()...)...)
	ex.req.Session = p.provider.Session(r.Header)
	ex.status = http.StatusBadRequest
	kind, message := "invalid_request_error", "switchboard: couldn't read the request body"
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		ex.status, kind, message = http.StatusRequestEntityTooLarge, "request_too_large",
			fmt.Sprintf("switchboard: the request body is over its limit of %d MiB", maxBody>>20)
	}
	writeError(w, ex.status, kind, message)
	p.ledger.note(p.line(ex, r.Header, f))
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

// lastID is the number the last routed request's id was made from: each
// takes the next, from a random start, so a request's id is its own while the
// router runs, and unlike those of the runs before it.
var lastID atomic.Uint32

func init() {
	lastID.Store(rand.Uint32())
}

// newID returns a routed request's id, which ties its lines in the log
// together, and its events in the request stream.
func newID() string {
	return fmt.Sprintf("%08x", lastID.Add(1))
}
