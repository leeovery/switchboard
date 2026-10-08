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
	whyLimit     = "hit its limit"
	whyRefused   = "was refused"
	whyThrottled = "was throttled"
	whyNewToken  = "has a new token"
	whyReset     = "was reset by hand"
)

// reasonBack is why a session goes back to the account it was on before a
// request every account it went out on refused, as the request stream tells
// of it: the moves the request made came to nothing.
const reasonBack = "back where it was before its request"

// replay is the transport a routed request goes upstream on. It sends the
// request on the exchange's account, and again, before the client has any of
// an answer, while the account can't serve it: on the same account after a
// pause when the account is throttled, when it refuses the token its token
// file no longer holds, as after the token is rotated, or when the limit it
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
	// reread is set once the account's token file has been read again after
	// the upstream refused its token, which happens once an account: a file
	// that holds a new token each time it's read mustn't keep the request
	// going round.
	reread bool
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
// its quota, whether it took the request.
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
	rp.sent = ex.account.token()
	attempt.Header.Set("Authorization", "Bearer "+rp.sent.Reveal())
	rp.when = rp.p.state.mark()
	rp.p.stream.publish(ex.event(StreamSent))
	resp, err := rp.p.transport.RoundTrip(attempt)
	if err != nil {
		return nil, err
	}
	usage := rp.p.provider.Usage(resp.Header)
	if len(usage.Windows) > 0 {
		rp.p.state.record(ex.account.ID, usage.Windows, rp.when)
		rp.p.state.learn(ex.req.Model, usage.Windows)
	}
	if usage.Extra.Given() {
		rp.p.state.recordExtra(ex.account.ID, usage.Extra, rp.when)
	}
	if ex.spends && succeeded(resp.StatusCode) {
		rp.p.state.admitted(ex.account.ID, rp.when)
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
		return rp.refused(ctx, resp, outcome.Verdict)
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

// tokenRefused has the request go out again on its account when the
// account's token file holds another token than the one the upstream
// refused, as after the token is rotated. Otherwise the account is refused,
// as refused says.
func (rp *replay) tokenRefused(ctx context.Context, resp *http.Response) (*http.Response, bool, error) {
	if !rp.renewed() {
		return rp.refused(ctx, resp, quota.Refused)
	}
	discard(resp)
	rp.replaying(rp.ex.account.ID, whyNewToken)
	return nil, true, nil
}

// renewed reads the token file of the account the request went out on again,
// unless it has been read again for this request already, and reports
// whether it holds another token than the one the request went out with,
// which the account goes out on from then on, the one it replaces still
// counting as the account's for clients started before, and no refusal of it
// holding the account back. A file that holds no token the account can use,
// as for a moment while it's rewritten, is no news: the account keeps its
// token. It logs what it found, but never the token.
func (rp *replay) renewed() bool {
	if rp.reread {
		return false
	}
	rp.reread = true
	a := rp.ex.account
	token, err := rp.p.readToken(a.ID)
	changed := err == nil && token.Reveal() != rp.sent.Reveal()
	if changed && a.secret.replace(token, rp.p.now()) {
		if lifted, ok := rp.p.state.tokenReplaced(a.ID); ok {
			rp.p.emit(lifted)
		}
		rp.p.tokensReplaced()
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

// refused bars an account that refused the request for a while, as bar
// does, tells the request stream of the refusal, and moves on from the
// account when another can take the request. When none can, the client has
// the answer of the first account whose limit the request reached, as that's
// why no account was left, else a refusal, giving the upstream's reason.
func (rp *replay) refused(ctx context.Context, resp *http.Response, verdict quota.Verdict) (*http.Response, bool, error) {
	rp.tell(StreamRefused, resp.StatusCode)
	reason := prose.Truncate(rp.p.provider.ErrorMessage(resp.Body, rp.sent.Reveal()), refusalShown)
	discard(resp)
	rp.p.emit(rp.bar(verdict, resp.StatusCode, reason))
	switch {
	case rp.moveOn(ctx, whyRefused):
		return nil, true, nil
	case rp.limit != nil:
		logger.Info("answering with the limit reached before", "id", rp.ex.id, "account", rp.limit.account)
		rp.ex.answered = rp.limit.pick
		return rp.limit.resp, false, nil
	}
	rp.takeBack()
	return nil, false, refusedError{status: resp.StatusCode, reason: reason}
}

// takeBack takes back what the request did when every account it went out
// on refused it, as a refusal of the request every account that judged it
// gives says more of the request than of the accounts: the moves of its
// session, which goes back where it was, as the request stream is told, and
// the bars it placed on its model family, each told of as it lifts. A bar on
// an account's token stands, as it says something of the account, and so
// does a bar another request placed; and so do the session's moves, the
// request's own among them, once another request of the session has been
// routed since.
func (rp *replay) takeBack() {
	if slices.ContainsFunc(rp.ex.req.Tried, func(a Attempt) bool { return a.Why != whyRefused }) {
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

// bar bars the account the request went out on, which refused it with
// status, for the reason given: from every request when the verdict is its
// token refused, else from the requests of the request's model family. It
// logs the refusal, and returns the news of it.
func (rp *replay) bar(verdict quota.Verdict, status int, reason string) Refused {
	ex := rp.ex
	news := Refused{Account: ex.account.ID, Status: status, Request: ex.id}
	if verdict == quota.Refused {
		news.Until = rp.p.state.refuse(news.Account, status, ex.id)
		logger.Warn("upstream refused the account's token", "id", ex.id, "account", news.Account, "status", status, "error", reason)
		return news
	}
	news.Family = rp.p.provider.Family(ex.req.Model)
	news.Until = rp.p.state.forbid(news.Account, news.Family, status, ex.id)
	logger.Warn("upstream refused the request on the account", "id", ex.id, "account", news.Account, "status", status, "family", news.Family, "error", reason)
	return news
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
	rp.throttled, rp.reread = 0, false
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
