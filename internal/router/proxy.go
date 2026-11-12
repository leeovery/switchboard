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
	"github.com/leeovery/switchboard/internal/redact"
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
	// PoolHeader names the pool run started claude in, by its id, which the
	// router routes the session's requests within.
	PoolHeader = ownPrefix + "Pool"
)

const (
	// maxBody caps the body of a routed request, which is held in memory.
	maxBody = 64 << 20
	// refusalShown is how many characters of the upstream's reason for
	// refusing a request the log shows, and the client, when no account is
	// left to try.
	refusalShown = 200
	// toldAtMost bounds what warned holds: the sessions of an account,
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
	// tokenTaken reads when a token read again was taken.
	tokenTaken TakenFunc
	// formerly gives the id of the account that had the id given before
	// switchboard made ids, as Config's Formerly does.
	formerly func(id string) (string, bool)
	// tokensReplaced hears that an account's token has been replaced, which
	// the state file is to keep.
	tokensReplaced func()
	state          *state
	// pools are the pools a request is routed within, which its header
	// names.
	pools    *membership
	provider Provider
	chooser  Chooser
	health   *health
	emit     func(Event)
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
	// ignored is what the log has said of the pins the proxy ignores, and
	// outside of those it ignores as they name an account outside the
	// request's pool, kept apart, as a pin honoured in the account's own pool
	// says nothing of them: a session's is warned of once for each spell the
	// account spends outside its pool, as honouredFor says. left is what it
	// has said of the sessions refused as their token's account isn't in
	// their pool, by that account: each is warned of once.
	ignored, outside, left warned
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
	// shape is a routed request's shape, as the request ledger keeps it, and
	// hints what its header says of it, their texts cut as a line gives them.
	shape ledger.Shape
	hints ledger.Hints
	// answered is the account whose answer the client has, as it was picked,
	// when it isn't the one the request went out on last: the first whose
	// limit the request reached, its answer held back, where every account
	// after refused it. It's zero while it is.
	answered pick
	// from is the account a routed request's session was on before the
	// request moved it: "" while it hasn't, and once its moves are taken
	// back.
	from string
	// pool is the id of the pool a routed request was found in, as the last
	// choice made for it found it, which its line records.
	pool string
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
	if p.strayedThrough(w, r) {
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
// others while that one can't serve it. One from a session whose token has
// left its pool switchboard refuses, as strayed says, choosing nothing. When
// the chooser picks none, as only accounts held back by their reserves are
// left, or none in the request's pool will take it, the router answers as the
// upstream would at a limit, and as unchosen says where no account it can
// send on will take the request. The request counts as routing until its line
// is noted, but for one that upgrades its connection.
func (p *proxy) route(w http.ResponseWriter, r *http.Request, client account) {
	if !upgrades(r) {
		p.routing.begin()
		defer p.routing.end()
	}
	ex := &exchange{id: newID(), started: time.Now(), arrived: p.now(), spends: p.provider.Spends(r.URL.Path), hints: p.provider.Hints(r.Header).Cut()}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		p.unread(w, r, ex, err)
		return
	}
	ex.req, ex.shape = p.request(r, body, ex, client)
	defer p.done(r, ex)
	if p.strayed(w, ex, client.ID) {
		return
	}
	choice := p.rechosen(r.Context(), ex.req, p.chooser.Choose(r.Context(), ex.req))
	ex.newSession, ex.pool = choice.New, choice.Pool
	switch {
	case choice.Reserved:
		p.reserved(w, ex, choice)
		return
	case choice.Closed:
		p.closed(w, ex, choice)
		return
	}
	a, ok := p.sendable(choice.Account)
	if !ok {
		p.unchosen(w, ex, choice, client)
		return
	}
	ex.account, ex.reason = a, choice.Reason
	p.moved(ex, choice.From)
	p.forward(w, withBody(r, body), ex)
}

// request is what the chooser is to know of a routed request, whose body is
// body, sent by client's token: its session, the directory it was started in
// and the pool it names, its model and whether the model's thinking is bound
// to its account, whether it's the client's quota check, whether it's the
// first of its session's main conversation after that was compacted, and its
// pin; and the request's shape, which the provider reads in the same pass
// over the body. A subagent's own conversation is compacted too, and marked
// alike, but its session's main conversation keeps its cache, which a move
// would throw away.
func (p *proxy) request(r *http.Request, body []byte, ex *exchange, client account) (Request, ledger.Shape) {
	model, check, shape := p.provider.Asks(body)
	session, pool := p.provider.Session(r.Header), namedPool(r.Header)
	return Request{
		ID:        ex.id,
		Session:   session,
		Dir:       dirOf(r.Header),
		Pool:      pool,
		Model:     model,
		Check:     check,
		Bound:     p.provider.ThinkingBound(model),
		Compacted: ex.hints.Class == ledger.ClassMain && ex.hints.Compacted != "",
		Pin:       p.pin(r, ex, session, pool),
		Client:    client.ID,
	}, shape
}

// namedPool returns the id of the pool a request whose header is h names, ""
// for none, cut short, with anything shaped like a token hidden, as the
// request is remembered and logged by the pool it's in, which may be one the
// config doesn't have.
func namedPool(h http.Header) string {
	return bounded(redact.Text(strings.TrimSpace(h.Get(PoolHeader))))
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
// it names one the router can send on in pool, the pool the request names, as
// the pools stand now: by its id, or, from a session started before
// switchboard made ids, the id it had before, as accounts' idNow gives it. It
// warns of a pin it ignores, once for each session and account, as
// warned says, and notes it at debug after.
func (p *proxy) pin(r *http.Request, ex *exchange, session, pool string) string {
	id := p.accounts.idNow(p.formerly, r.Header.Get(PinHeader))
	if id == "" {
		return ""
	}
	a, ok := p.accounts.byID(id)
	why, told := "", &p.ignored
	switch {
	case !ok:
		why = "pin ignored: no such account"
	case !a.hasToken():
		why = "pin ignored: account has no usable token"
	case !p.pools.current().serves(pool, id):
		why, told = "pin ignored: account isn't in the request's pool", &p.outside
	default:
		p.ignored.honoured(id)
		p.outside.honouredFor(id, session)
		return id
	}
	level := slog.LevelDebug
	if told.first(id, session) {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, why, "id", ex.id, "pin", id, "session", status.ShortID(session))
	return ""
}

// warned holds, by account, the sessions the log has warned of something to
// do with the account, as a pin to it ignored, so it warns once for each, not
// at every request, until a pin to the account is honoured, where that's what
// it warned of. Sessions come and go, and an account that isn't configured is
// never sent on, so it forgets an account's sessions once it holds toldAtMost
// of them, and every account's once it holds that many accounts, the log
// warning of each once more. It's safe for concurrent use.
type warned struct {
	mu   sync.Mutex
	told map[string]map[string]bool
}

// first reports whether what's warned of, to do with account, of a request of
// session, is the first since a pin to the account was last honoured.
func (p *warned) first(account, session string) bool {
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
func (p *warned) honoured(account string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.told, account)
}

// honouredFor notes that a pin to account of a request of session is
// honoured: that session's pin to it ignored after is warned of afresh, and
// another's isn't, as the pin of a session in another pool says nothing of
// it.
func (p *warned) honouredFor(account, session string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.told[account], session)
}

// rechosen is choice, the chooser's for req, unless it names an account that
// can no longer be sent on, as a probe found its token another
// subscription's, or its file gone, since the chooser chose it: then the
// chooser takes back what it chose for req, which came to nothing, and is
// asked again, once an account at most. A request's first choice is had so,
// and each after, as next has it.
func (p *proxy) rechosen(ctx context.Context, req Request, choice Choice) Choice {
	for range len(p.accounts) {
		if a, ok := p.accounts.byID(choice.Account); !ok || a.hasToken() {
			return choice
		}
		p.chooser.Forget(req)
		choice = p.chooser.Choose(ctx, req)
	}
	return choice
}

// sendable returns the account with the given id, reporting false where the
// router can't send on it, or has none of it, as for "".
func (p *proxy) sendable(id string) (account, bool) {
	a, ok := p.accounts.byID(id)
	return a, ok && a.hasToken()
}

// unchosen answers a request the chooser chose no account for that the router
// can send on, as unsendable says, naming why the account chosen can't take
// it, else why its client's can't: where the choice names none, no account
// the router can send on in the request's pool takes it. The request never
// goes out on its client's token in place of the account chosen, as the
// client's may be another pool's. A choice of one the router can't send on,
// which rechosen has asked again for already, is a bug, which it logs as an
// error.
func (p *proxy) unchosen(w http.ResponseWriter, ex *exchange, choice Choice, client account) {
	ex.reason = choice.Reason
	chosen, ok := p.accounts.byID(choice.Account)
	if !ok {
		p.unsendable(w, ex, client)
		return
	}
	logger.Error("chooser picked an account it can't send on; refusing the request", "id", ex.id, "picked", choice.Account)
	p.unsendable(w, ex, chosen)
}

// unsendable answers a request that can go out on no account the router can
// send on, not on a, its client's or the one it was to go out on again: a
// 503, shaped as the API shapes its errors, saying why a can't take it, and
// telling the client not to retry, as nothing changes till the account has a
// token of its own. Nothing more goes out, so nothing goes out on a token
// another subscription's.
func (p *proxy) unsendable(w http.ResponseWriter, ex *exchange, a account) {
	ex.status = http.StatusServiceUnavailable
	logger.Warn("no account can take the request; answering 503", "id", ex.id, "account", a.ID)
	message := "switchboard: no account can take the request"
	if problem := a.problem(); problem != "" {
		message += "; account " + a.ID + ": " + problem
	}
	w.Header().Set("X-Should-Retry", "false")
	writeError(w, ex.status, "api_error", message)
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

// closed answers a request that no account in its pool will take, its
// client's account outside the pool, as the upstream answers one over its
// account's limit, as reserved does, when it can go out again not known:
// nothing goes out on another pool's account, its client's own token among
// them. Where there are pools to tell apart, it names the pool. It says why,
// as whyClosed has it, as the limit the answer reads as isn't. A request
// whose client's account is outside its pool as it arrives strayed refuses,
// so this answers one whose account left the pool between that check and a
// choice, as the pools changed.
func (p *proxy) closed(w http.ResponseWriter, ex *exchange, choice Choice) {
	ex.reason, ex.status = choice.Reason, http.StatusTooManyRequests
	pools := p.pools.current()
	why := p.whyClosed(pools, ex.pool)
	logger.Warn("no account in the request's pool can take it; answering 429", "id", ex.id, "pool", ex.pool, "why", why)
	p.provider.MarkLimited(w.Header(), time.Time{})
	message := "switchboard: no account can take the request"
	if pool := pools.shown(ex.pool); pool != "" {
		message = "switchboard: no account in pool " + pool + " can take the request"
	}
	writeError(w, ex.status, "rate_limit_error", message+"; "+why)
}

// whyClosed says why no account in the pool with the given id, as pools has
// it, takes a request closed answers, account by account: of each the router
// has, in the order configured, the problem of one without a usable token,
// as unsendable names one, which says how to fix it, else that the upstream
// refused it lately, as that alone leaves it nothing to fall back to; then
// of each the config added since the router started, that it's taken up
// when the router restarts. Where the pool has none, it says so.
func (p *proxy) whyClosed(pools *poolTable, pool string) string {
	members := pools.members(pool)
	if len(members) == 0 {
		return "the pool has no accounts"
	}
	whys := make([]string, 0, len(members))
	for _, a := range p.accounts {
		if pools.holds(pool, a.ID) {
			whys = append(whys, "account "+a.ID+": "+cmp.Or(a.problem(), "the upstream refused it lately"))
		}
	}
	for _, id := range members {
		if !p.accounts.includes(id) {
			whys = append(whys, "account "+id+": taken up when the router restarts")
		}
	}
	return strings.Join(whys, "; ")
}

// next asks the chooser which account a request goes out on after those it
// has been tried on, as rechosen has it, and returns it with the choice,
// which says why. It reports false when none other has room for it.
func (p *proxy) next(ctx context.Context, req Request) (account, Choice, bool) {
	choice := p.rechosen(ctx, req, p.chooser.Choose(ctx, req))
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
// so, and goes upstream offering none, as the router's transport adds none.
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

// errUnsendable is a routed request left with no account to go out on, as
// the one it was to go out on again can no longer be sent on, and no other
// takes it.
var errUnsendable = errors.New("no account can take the request")

// fail answers a request the upstream didn't answer, or answered with a
// refusal the client mustn't see: a 502, shaped as the API shapes its errors.
// One left with no account to go out on is answered as unsendable says.
func (p *proxy) fail(w http.ResponseWriter, r *http.Request, ex *exchange, err error) {
	if r.Context().Err() != nil {
		// The client has gone, so there's no one to answer.
		return
	}
	if errors.Is(err, errUnsendable) {
		p.unsendable(w, ex, ex.account)
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
// went upstream; in the hold curve, the log and the request ledger, once its
// answer is counted, as the request finished by then, as the router can cut
// it off meanwhile, stopping, though its time is taken before, as is whether
// the upstream broke its answer off; and, once it was answered, in the
// router's health, a failure where the router failed it itself or the
// upstream broke its answer off. A new session whose request wasn't answered
// with success, the chooser forgets first, so the line is of what stands.
func (p *proxy) done(r *http.Request, ex *exchange) {
	took, broken := time.Since(ex.started), ex.brokenOff(r.Context())
	p.ended(ex, broken)
	p.weigh(ex)
	f := finishing(r, took, broken)
	p.logRouted(ex, f)
	if forgottenOnceDone(ex.newSession, ex.status) {
		p.forget(ex)
	}
	p.ledger.note(p.line(ex, r.Header, f))
	if ex.status != 0 {
		p.health.record(ex.arrived, ex.failed || f.broken)
	}
}

// brokenOff reports whether the upstream broke off the answer the client of
// the routed request ex has, after it began: reading it failed short of its
// end while the request, whose context is ctx, still stood, as its client
// going or the router cutting it off fails the read too.
func (ex *exchange) brokenOff(ctx context.Context) bool {
	return ex.tap != nil && ex.tap.broke && ctx.Err() == nil
}

// forgottenOnceDone reports whether the session of a routed request, once
// it's done, is forgotten: it was new to the request, as newSession says,
// and the request wasn't answered with success, as status says.
func forgottenOnceDone(newSession bool, status int) bool {
	return newSession && !succeeded(status)
}

// weigh has the hold curve take in the answer the client of the routed
// request ex has, once it's counted, by the tokens its closing usage gave,
// from a session as long on the account that gave it as the chooser says.
// An answer that gave none, as an error, and one to a request that spends no
// quota, add nothing.
func (p *proxy) weigh(ex *exchange) {
	if !ex.spends || ex.tap == nil || ex.tap.tokens == nil {
		return
	}
	now := p.now().UTC()
	p.state.answered(*ex.tap.tokens, p.chooser.OnAccount(ex.req, ex.answering().account, now), now)
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
// and, where it ended early, whichever came first of being cut off by the
// router as it stopped, canceled, its client gone, or broken, its answer
// broken off by the upstream after it began.
type finish struct {
	took                     time.Duration
	cutOff, canceled, broken bool
}

// finishing returns how the routed request r finished, as long after it
// arrived as took: broken, where the upstream broke its answer off, as broken
// says; else, where its context has ended, what ended it first, as its cause
// says.
func finishing(r *http.Request, took time.Duration, broken bool) finish {
	f := finish{took: took, broken: broken}
	if !broken && r.Context().Err() != nil {
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
	case f.broken:
		return []any{"broken", true}
	}
	return nil
}

// ended tells the request stream of a routed request that went upstream
// being done, with what its client was answered, and whether the upstream
// broke that off, as broken says: once its answer is counted, when it has
// one.
func (p *proxy) ended(ex *exchange, broken bool) {
	if ex.attempts == 0 {
		return
	}
	done := ex.event(StreamDone)
	done.Status, done.Broken = ex.status, broken
	if ex.tap != nil {
		ex.tap.end(done)
		return
	}
	p.stream.publish(done)
}

// event returns the request stream's event of the kind given of a routed
// request, as it stands: on the account whose answer the client has, once
// it has one, its session and model cut short where they run too long, its
// directory, the prompt it serves, its class and its agent as the request
// ledger gives them, and whether it's the client's quota check, or a count
// of tokens.
func (ex *exchange) event(kind string) StreamEvent {
	return StreamEvent{
		Kind:    kind,
		Request: ex.id,
		Attempt: ex.attempts,
		Session: bounded(ex.req.Session),
		Dir:     ex.req.Dir,
		Model:   bounded(ex.req.Model),
		Account: ex.answering().account,
		Prompt:  ex.hints.Prompt,
		Class:   ex.hints.Class,
		AgentID: ex.hints.AgentID,
		Check:   ex.req.Check,
		Count:   !ex.spends,
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
// once, and whether it was cut off, its client went away, or the upstream
// broke its answer off, before its end.
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
	f := finishing(r, time.Since(ex.started), false)
	logger.Warn("request refused: body unread", append([]any{"id", ex.id, "method", r.Method, "path", r.URL.Path, "error", err}, f.attrs()...)...)
	ex.req.Session, ex.req.Dir, ex.req.Pool = p.provider.Session(r.Header), dirOf(r.Header), namedPool(r.Header)
	ex.pool = p.pools.current().of(ex.req.Pool)
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
