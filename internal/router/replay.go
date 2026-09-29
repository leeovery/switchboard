package router

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"net/http"
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
)

// replay is the transport a routed request goes upstream on. It sends the
// request on the exchange's account, and again, before the client has any of
// an answer, while the account can't serve it: on the same account after a
// pause when the account is throttled, or when it refuses the token its token
// file no longer holds, as after the token is rotated, and on another when
// its limit is reached or it refuses the request. It reads the usage off
// every answer. What the client gets is the answer to the last attempt,
// unless that refused the request: then it's the answer of the first account
// whose limit the request reached, if one did, and a refusal if not.
type replay struct {
	p  *proxy
	ex *exchange
	// sent is the token the last attempt went out with.
	sent tokens.Token
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
// the client can still have it, and the account that gave it.
type heldAnswer struct {
	account string
	resp    *http.Response
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
// own and the account's token as the router holds it now, and reads the
// account's usage off the answer, which it returns.
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
	resp, err := rp.p.transport.RoundTrip(attempt)
	if err != nil {
		return nil, err
	}
	if windows := rp.p.provider.Usage(resp.Header); len(windows) > 0 {
		rp.p.state.record(ex.account.ID, windows)
		rp.p.state.learn(ex.req.Model, windows)
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
// counting as the account's for clients started before. It logs what it
// found, but never the token.
func (rp *replay) renewed() bool {
	if rp.reread {
		return false
	}
	rp.reread = true
	a := rp.ex.account
	token, err := rp.p.readToken(a.ID)
	changed := err == nil && token.Reveal() != rp.sent.Reveal()
	if changed && a.secret.replace(token, rp.p.now()) {
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
// those windows count, and moves on from it, when another account can take
// the request, holding the answer back. When none can, the answer is the
// client's.
func (rp *replay) limitReached(ctx context.Context, resp *http.Response, rejected []string, until time.Time) (*http.Response, bool, error) {
	reached := rp.p.state.limit(rp.ex.account.ID, rejected, until)
	news := "limit reached"
	if reached.Again {
		news = "limit reached again"
	}
	logger.Warn(news, "id", rp.ex.id, "account", reached.Account, "windows", strings.Join(reached.Windows, ","), "until", reached.Until)
	rp.p.emit(reached)
	if !rp.moveOn(ctx, whyLimit) {
		return resp, false, nil
	}
	rp.holdBack(reached.Account, resp)
	return nil, true, nil
}

// holdBack keeps the answer of the first account whose limit the request
// reached, with the account's id, should no other account serve the request:
// it's why. Any other, and one whose body can't be kept whole, it discards.
func (rp *replay) holdBack(account string, resp *http.Response) {
	if rp.limit != nil {
		discard(resp)
		return
	}
	if held, ok := hold(resp); ok {
		rp.limit = &heldAnswer{account: account, resp: held}
	}
}

// throttle waits, and has the request sent again on its account, while it has
// been throttled there no more than throttleRetries times; after that, the
// answer is the client's. The wait is the one the upstream asks for, within
// reason, and ends early, failing, should the client go.
func (rp *replay) throttle(ctx context.Context, resp *http.Response, retryAfter time.Duration) (*http.Response, bool, error) {
	id := rp.ex.account.ID
	if rp.throttled >= throttleRetries {
		logger.Warn("still throttled; passing the answer on", "id", rp.ex.id, "account", id, "attempts", rp.ex.attempts)
		return resp, false, nil
	}
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
// does, and moves on from it when another account can take the request. When
// none can, the client has the answer of the first account whose limit the
// request reached, as that's why no account was left, else a refusal.
func (rp *replay) refused(ctx context.Context, resp *http.Response, verdict quota.Verdict) (*http.Response, bool, error) {
	reason := rp.p.provider.ErrorMessage(resp.Body, rp.sent.Reveal())
	discard(resp)
	rp.p.emit(rp.bar(verdict, resp.StatusCode, prose.Truncate(reason, refusalShown)))
	switch {
	case rp.moveOn(ctx, whyRefused):
		return nil, true, nil
	case rp.limit != nil:
		logger.Info("answering with the limit reached before", "id", rp.ex.id, "account", rp.limit.account)
		return rp.limit.resp, false, nil
	default:
		return nil, false, refusedError{status: resp.StatusCode}
	}
}

// bar bars the account the request went out on, which refused it with
// status, for the reason given: from every request when the verdict is its
// token refused, else from the requests of the request's model family. It
// logs the refusal, and returns the news of it.
func (rp *replay) bar(verdict quota.Verdict, status int, reason string) Refused {
	ex := rp.ex
	news := Refused{Account: ex.account.ID, Status: status}
	if verdict == quota.Refused {
		rp.p.state.refuse(news.Account, status)
		logger.Warn("upstream refused the account's token", "id", ex.id, "account", news.Account, "status", status, "error", reason)
		return news
	}
	news.Family = rp.p.provider.Family(ex.req.Model)
	rp.p.state.forbid(news.Account, news.Family, status)
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
	to, reason, ok := rp.p.next(ctx, ex.req)
	if !ok {
		logger.Warn("no account left to try", "id", ex.id, "attempts", ex.attempts)
		return false
	}
	ex.account, ex.reason = to, reason
	rp.throttled, rp.reread = 0, false
	rp.replaying(from, why)
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
