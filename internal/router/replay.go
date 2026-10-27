package router

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

const (
	// throttleRetries is how many times a throttled request is sent again on
	// its account before the throttling goes to the client.
	throttleRetries = 2
	// throttleWait is how long a throttled request waits before it's sent
	// again when the upstream doesn't say, and maxThrottleWait the longest it
	// waits whatever the upstream says.
	throttleWait    = 2 * time.Second
	maxThrottleWait = 10 * time.Second
	// maxDiscard is how much of an answer held back from the client is read
	// before it's closed, which lets its connection carry the next request.
	// An answer the client may yet have is kept only when that's all of it.
	maxDiscard = 64 << 10
)

// What went wrong on an account a request went out on, as the log, and the
// reason for moving on from the account, give it.
const (
	whyLimit     = status.WhyLimit
	whyRefused   = status.WhyRefused
	whyThrottled = "was throttled"
	whyNewToken  = "has a new token"
	whyReset     = "was reset by hand"
)

// reasonBack is why a session goes back to the account it was on before a
// request every account it went out on refused, as the request stream tells
// of it: the moves the request made came to nothing.
const reasonBack = status.ReasonBack

// replay is the transport a routed request goes upstream on. It sends the
// request on the exchange's account, and again, before the client has any of
// an answer, while the account can't serve it: on the same account after a
// pause when the account is throttled, when it refuses a token the account
// has replaced since, as after the token is rotated, or when the limit it
// answers with was reset by hand since the request was sent, and on another
// when its limit is reached or it refuses the request. It reads the usage off
// every answer. What the client gets is the answer to the last attempt,
// unless that refused the request: then it's the answer of the first account
// whose limit the request reached, if one did, and a refusal if not.
type replay struct {
	p  *proxy
	ex *exchange
	// sent is the token the last attempt went out with, and when the moment
	// it went out, as the state orders what it takes in.
	sent tokens.Token
	when moment
	// throttled counts the times the request was sent again on its account
	// after being throttled there.
	throttled int
	// newToken is set once the request has gone out again on its account for
	// a new token, after the upstream refused the one before, which happens
	// once an account: a token that changes between each attempt and its 401
	// mustn't keep the request going round.
	newToken bool
	// limit is the answer of the first account whose limit the request
	// reached, held back from the client, or nil while there's none.
	limit *heldAnswer
}

// heldAnswer is an answer held back from the client, its body read whole, so
// the client can still have it, and the account that gave it, as it was
// picked when the request went out on it.
type heldAnswer struct {
	pick
	resp *http.Response
}

func (rp *replay) RoundTrip(out *http.Request) (*http.Response, error) {
	for {
		resp, err := rp.send(out)
		if err != nil {
			// A failure to reach the upstream isn't the account's, so
			// another would fare no better.
			return nil, err
		}
		answer, again, err := rp.settle(out.Context(), resp)
		if !again {
			return answer, err
		}
	}
}

// send sends the request out on the exchange's account, with a body of its
// own and the account's token as the router holds it now, and reads off the
// answer, which it returns, the account's usage and, of a request that spends
// its quota, whether it took the request, telling of each of the account's
// limits lifting early where either shows it.
func (rp *replay) send(out *http.Request) (*http.Response, error) {
	ex := rp.ex
	ex.attempts++
	attempt := out.Clone(out.Context())
	if out.Body != nil {
		body, err := out.GetBody()
		if err != nil {
			return nil, err
		}
		attempt.Body = body
	}
	// when is marked before the token is read, so a token the account takes
	// up after the read is marked after when, and the state's refuse notes no
	// refusal of this one against the account.
	rp.when = rp.p.state.mark()
	rp.sent = ex.account.token()
	attempt.Header.Set("Authorization", "Bearer "+rp.sent.Reveal())
	rp.p.stream.publishOf(ex.event(StreamSent), ex.req.key())
	resp, err := rp.p.transport.RoundTrip(attempt)
	if err != nil {
		return nil, err
	}
	usage := rp.p.provider.Usage(resp.Header)
	if len(usage.Windows) > 0 {
		for _, lifted := range rp.p.state.record(ex.account.ID, usage.Windows, rp.when) {
			rp.p.emit(lifted)
		}
		rp.p.state.learn(ex.req.Model, usage.Windows)
	}
	if usage.Extra.Given() {
		rp.p.state.recordExtra(ex.account.ID, usage.Extra, rp.when)
	}
	if ex.spends && succeeded(resp.StatusCode) {
		for _, lifted := range rp.p.state.admitted(ex.account.ID, rp.when) {
			rp.p.emit(lifted)
		}
	}
	return resp, nil
}

// settle decides what's to become of an answer: the request goes out again,
// when settle reports true, or else the client has an answer, this one or
// one held back before it, or err instead. An answer the client isn't to
// have is closed.
func (rp *replay) settle(ctx context.Context, resp *http.Response) (*http.Response, bool, error) {
	outcome := rp.p.provider.Classify(resp.StatusCode, resp.Header)
	switch outcome.Verdict {
	case quota.LimitReached:
		return rp.limitReached(ctx, resp, outcome.Rejected, outcome.LimitedUntil)
	case quota.Throttled:
		return rp.throttle(ctx, resp, outcome.RetryAfter)
	case quota.Refused:
		return rp.tokenRefused(ctx, resp)
	case quota.Forbidden:
		return rp.refused(ctx, resp, quota.Forbidden, rp.forbid(resp.StatusCode))
	default:
		return resp, false, nil
	}
}

// tell tells the request stream of an answer, with status, that the router
// has judged to hold against the account the request went out on, as kind
// says: its limit reached, its throttling, or a refusal. An answer the router
// takes as the client's is told of as the client has it.
func (rp *replay) tell(kind string, status int) {
	e := rp.ex.event(kind)
	e.Status = status
	rp.p.stream.publish(e)
}

// tokenRefused bars the account the request went out on from every request,
// as the upstream refused the token it holds, and has the request move on, as
// refused says. A token the account has replaced since, as after it's
// rotated, is no refusal: its token file holds another, as renewed says, or
// the router took another up while the request was out, as barToken says.
// Then the request goes out again on the account, once an account, and after
// that on another, as elsewhere says, the account not barred, nor anything
// told of a refusal.
func (rp *replay) tokenRefused(ctx context.Context, resp *http.Response) (*http.Response, bool, error) {
	renewed := !rp.newToken && rp.renewed()
	if !renewed {
		if news, barred := rp.barToken(resp.StatusCode); barred {
			return rp.refused(ctx, resp, quota.Refused, news)
		}
	}
	if rp.newToken {
		return rp.elsewhere(ctx, whyNewToken, rp.refusalOf(resp))
	}
	rp.newToken = true
	discard(resp)
	rp.replaying(rp.ex.account.ID, whyNewToken)
	return nil, true, nil
}

// barToken bars the account the request went out on from every request, as
// the upstream refused its token, answering with status, and returns the news
// of it. It reports false, barring nothing, when the account has gone out on
// another token since the request went out, as the state's refuse says: the
// token refused is the one before.
func (rp *replay) barToken(status int) (Refused, bool) {
	ex := rp.ex
	until, barred := rp.p.state.refuse(ex.account.ID, status, ex.id, rp.when)
	if !barred {
		return Refused{}, false
	}
	return Refused{Account: ex.account.ID, Status: status, Request: ex.id, Until: until}, true
}

// forbid bars the account the request went out on from the requests of the
// request's model family, as the upstream refused the request there,
// answering with status, though not its token, and returns the news of it.
func (rp *replay) forbid(status int) Refused {
	ex := rp.ex
	news := Refused{Account: ex.account.ID, Status: status, Request: ex.id, Family: rp.p.provider.Family(ex.req.Model)}
	news.Until = rp.p.state.forbid(news.Account, news.Family, status, ex.id)
	return news
}

// renewed reads the token file of the account the request went out on again,
// and reports whether it holds another token than the one the request went
// out with, which the account goes out on from then on, the one it replaces
// still counting as the account's for clients started before, and no refusal
// of it holding the account back. A file that holds no token the account can
// use, as for a moment while it's rewritten, is no news: the account keeps
// its token. It logs what it found, but never the token.
func (rp *replay) renewed() bool {
	a := rp.ex.account
	token, err := rp.p.readToken(a.ID)
	changed := err == nil && token.Reveal() != rp.sent.Reveal()
	if changed && a.secret.replace(token, rp.p.now()) {
		if lifted, ok := rp.p.state.tokenReplaced(a.ID); ok {
			rp.p.emit(lifted)
		}
		rp.p.tokensReplaced()
	}
	if err == nil {
		a.noteTaken(token, rp.p.tokenTaken)
	}
	attrs := []any{"id", rp.ex.id, "account", a.ID, "changed", changed}
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	logger.Info("read the token file again", attrs...)
	return changed
}

// limitReached bars an account whose limit the request reached in the
// windows rejected, if any, until when the answer says, from the requests
// those windows count, tells the request stream of it, and moves on from it,
// when another account can take the request, holding the answer back. When
// none can, the answer is the client's. A limit reached only in windows reset
// by hand since the request was sent, as the state's limit says, is no limit:
// the request goes out again on the account, after the reset, where the
// answer is the account's as it now stands.
func (rp *replay) limitReached(ctx context.Context, resp *http.Response, rejected []string, until time.Time) (*http.Response, bool, error) {
	reached, ok := rp.p.state.limit(rp.ex.account.ID, rejected, until, rp.when)
	if !ok {
		logger.Info("limit from before a reset passed over", "id", rp.ex.id, "account", reached.Account, "windows", strings.Join(reached.Windows, ","))
		discard(resp)
		rp.replaying(reached.Account, whyReset)
		return nil, true, nil
	}
	rp.tell(StreamLimited, resp.StatusCode)
	news := "limit reached"
	if reached.Again {
		news = "limit reached again"
	}
	logger.Warn(news, "id", rp.ex.id, "account", reached.Account, "windows", strings.Join(reached.Windows, ","), "until", reached.Until)
	rp.p.emit(reached)
	limited := rp.ex.picked()
	if !rp.moveOn(ctx, whyLimit) {
		return resp, false, nil
	}
	rp.holdBack(limited, resp)
	return nil, true, nil
}

// holdBack keeps the answer of the first account whose limit the request
// reached, with the account, as it was picked, should no other account serve
// the request: it's why. Any other, and one whose body can't be kept whole,
// it discards.
func (rp *replay) holdBack(limited pick, resp *http.Response) {
	if rp.limit != nil {
		discard(resp)
		return
	}
	if held, ok := hold(resp); ok {
		rp.limit = &heldAnswer{pick: limited, resp: held}
	}
}

// throttle waits, and has the request sent again on its account, telling the
// request stream of the throttling, while it has been throttled there no more
// than throttleRetries times; after that, the answer is the client's, which
// the stream tells of as the request ends. The wait is the one the upstream
// asks for, within reason, and ends early, failing, should the client go.
func (rp *replay) throttle(ctx context.Context, resp *http.Response, retryAfter time.Duration) (*http.Response, bool, error) {
	id := rp.ex.account.ID
	if rp.throttled >= throttleRetries {
		logger.Warn("still throttled; passing the answer on", "id", rp.ex.id, "account", id, "attempts", rp.ex.attempts)
		return resp, false, nil
	}
	rp.tell(StreamThrottled, resp.StatusCode)
	rp.throttled++
	wait := min(cmp.Or(retryAfter, throttleWait), maxThrottleWait)
	logger.Warn("throttled", "id", rp.ex.id, "account", id, "wait", wait)
	discard(resp)
	if err := sleep(ctx, wait); err != nil {
		return nil, false, err
	}
	rp.replaying(id, whyThrottled)
	return nil, true, nil
}

// refused tells the request stream of the upstream's refusal of the request
// on the account it went out on, which has barred the account for a while, as
// verdict says and news tells of, logs it, and moves on from the account, as
// elsewhere says.
func (rp *replay) refused(ctx context.Context, resp *http.Response, verdict quota.Verdict, news Refused) (*http.Response, bool, error) {
	rp.tell(StreamRefused, resp.StatusCode)
	answer := rp.refusalOf(resp)
	logRefused(verdict, news, answer.reason)
	rp.p.emit(news)
	return rp.elsewhere(ctx, whyRefused, answer)
}

// refusalOf returns the upstream's refusal of the request, resp, as the client
// has it, its reason read off resp, which it closes.
func (rp *replay) refusalOf(resp *http.Response) refusedError {
	reason := prose.Truncate(rp.p.provider.ErrorMessage(resp.Body, rp.sent.Reveal()), refusalShown)
	discard(resp)
	return refusedError{status: resp.StatusCode, reason: reason}
}

// elsewhere has the request go out next on another account, as the one it
// went out on couldn't serve it, for the reason why. When none can take it,
// the client has the answer of the first account whose limit the request
// reached, as that's why no account was left, else answer.
func (rp *replay) elsewhere(ctx context.Context, why string, answer refusedError) (*http.Response, bool, error) {
	switch {
	case rp.moveOn(ctx, why):
		return nil, true, nil
	case rp.limit != nil:
		logger.Info("answering with the limit reached before", "id", rp.ex.id, "account", rp.limit.account)
		rp.ex.answered = rp.limit.pick
		return rp.limit.resp, false, nil
	}
	rp.takeBack()
	return nil, false, answer
}

// takeBack takes back what the request did when an account judged it, and
// every account that did refused it, as a refusal of the request every
// account that judged it gives says more of the request than of the
// accounts: the moves of its session, which goes back where it was, as the
// request stream is told, and the bars it placed on its model family, each
// told of as it lifts. An account the request left for a new token judged
// nothing, as its 401 was to a token it no longer held. A bar on an
// account's token stands, as it says something of the account, and so does a
// bar another request placed; and so do the session's moves, the request's
// own among them, once another request of the session has been routed since.
func (rp *replay) takeBack() {
	judged := slices.DeleteFunc(slices.Clone(rp.ex.req.Tried), func(a Attempt) bool { return a.Why == whyNewToken })
	if len(judged) == 0 || slices.ContainsFunc(judged, func(a Attempt) bool { return a.Why != whyRefused }) {
		return
	}
	if back := rp.p.forget(rp.ex); back != "" && back != rp.ex.account.ID {
		rp.p.tellMoved(rp.ex, rp.ex.account.ID, back, reasonBack)
	}
	lifted := rp.p.state.takeBack(rp.ex.id)
	for _, e := range lifted {
		rp.p.emit(e)
	}
	if len(lifted) > 0 {
		logger.Info("refused on every account it went out on; its refusals of the request hold none back", "id", rp.ex.id, "attempts", rp.ex.attempts)
	}
}

// logRefused logs the upstream's refusal of a request, which news tells of,
// for the reason given: of the account's token when the verdict says so,
// else of the request alone.
func logRefused(verdict quota.Verdict, news Refused, reason string) {
	if verdict == quota.Refused {
		logger.Warn("upstream refused the account's token", "id", news.Request, "account", news.Account, "status", news.Status, "error", reason)
		return
	}
	logger.Warn("upstream refused the request on the account", "id", news.Request, "account", news.Account, "status", news.Status, "family", news.Family, "error", reason)
}

// moveOn has the request go out next on another account, as the one it went
// out on couldn't serve it, for the reason why. It reports false, having
// said so, when no other account has room for it.
func (rp *replay) moveOn(ctx context.Context, why string) bool {
	ex := rp.ex
	from := ex.account.ID
	ex.req.Tried = append(ex.req.Tried, Attempt{Account: from, Why: why})
	to, choice, ok := rp.p.next(ctx, ex.req)
	if !ok {
		logger.Warn("no account left to try", "id", ex.id, "attempts", ex.attempts)
		return false
	}
	ex.account, ex.reason = to, choice.Reason
	rp.throttled, rp.newToken = 0, false
	rp.replaying(from, why)
	rp.p.moved(ex, choice.From)
	return true
}

// replaying notes that the request goes out again, on the exchange's
// account, as the account it went out on, from, couldn't serve it.
func (rp *replay) replaying(from, why string) {
	logger.Info("replaying", "id", rp.ex.id, "attempt", rp.ex.attempts+1, "from", from, "to", rp.ex.account.ID, "why", why)
}

// discard reads what's left of an answer the client isn't to have, up to
// maxDiscard, and closes it.
func discard(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDiscard))
	_ = resp.Body.Close()
}

// hold reads the rest of an answer held back from the client, up to
// maxDiscard, and closes it, returning the answer with a body of what was
// read. It reports false when that isn't all of the body.
func hold(resp *http.Response) (*http.Response, bool) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscard+1))
	_ = resp.Body.Close()
	if err != nil || len(body) > maxDiscard {
		return nil, false
	}
	held := *resp
	held.Body, held.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
	return &held, true
}

// sleep waits for d, or fails as soon as ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
