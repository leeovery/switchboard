package router

import (
	"cmp"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
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
	// maxDiscard is how much of an answer the client isn't to have is read
	// before it's closed, which lets its connection carry the next request.
	maxDiscard = 64 << 10
)

// What went wrong on an account a request went out on, as the log, and the
// reason for moving on from the account, give it.
const (
	whyLimit     = "hit its limit"
	whyRefused   = "was refused"
	whyThrottled = "was throttled"
)

// replay is the transport a routed request goes upstream on. It sends the
// request on the exchange's account, and again, before the client has any of
// an answer, while the account can't serve it: on the same account after a
// pause when the account is throttled, and on another when its limit is
// reached or its token refused. It reads the usage off every answer. What the
// client gets is the answer to the last attempt, or a refusal when that
// refused the account's token.
type replay struct {
	p  *proxy
	ex *exchange
	// throttled counts the times the request was sent again on its account
	// after being throttled there.
	throttled int
}

func (rp *replay) RoundTrip(out *http.Request) (*http.Response, error) {
	for {
		resp, windows, err := rp.send(out)
		if err != nil {
			// A failure to reach the upstream isn't the account's, so
			// another would fare no better.
			return nil, err
		}
		again, err := rp.settle(out.Context(), resp, windows)
		switch {
		case err != nil:
			return nil, err
		case !again:
			return resp, nil
		}
	}
}

// send sends the request out on the exchange's account, with a body of its
// own, and reads the account's usage off the answer, which it returns with
// the windows read.
func (rp *replay) send(out *http.Request) (*http.Response, []quota.Window, error) {
	ex := rp.ex
	ex.attempts++
	attempt := out.Clone(out.Context())
	if out.Body != nil {
		body, err := out.GetBody()
		if err != nil {
			return nil, nil, err
		}
		attempt.Body = body
	}
	attempt.Header.Set("Authorization", "Bearer "+ex.account.token.Reveal())
	resp, err := rp.p.transport.RoundTrip(attempt)
	if err != nil {
		return nil, nil, err
	}
	windows := rp.p.provider.Usage(resp.Header)
	if len(windows) > 0 {
		rp.p.state.record(ex.account.ID, windows, fromResponse)
		rp.p.state.learn(ex.req.Model, windows)
	}
	return resp, windows, nil
}

// settle decides what's to become of an answer: it's the client's, or the
// request goes out again, when settle reports true, or the client has err
// instead. An answer the client isn't to have is closed.
func (rp *replay) settle(ctx context.Context, resp *http.Response, windows []quota.Window) (bool, error) {
	outcome := rp.p.provider.Classify(resp.StatusCode, resp.Header)
	switch outcome.Verdict {
	case quota.LimitReached:
		return rp.limitReached(ctx, resp, windows), nil
	case quota.Throttled:
		return rp.throttle(ctx, resp, outcome.RetryAfter)
	case quota.Refused:
		return rp.refused(ctx, resp)
	default:
		return false, nil
	}
}

// limitReached moves on from an account whose limit the request reached, when
// another account can take it. When none can, the answer is the client's.
func (rp *replay) limitReached(ctx context.Context, resp *http.Response, windows []quota.Window) bool {
	id := rp.ex.account.ID
	keys := rejected(windows)
	logger.Warn("limit reached", "id", rp.ex.id, "account", id, "windows", strings.Join(keys, ","))
	if !rp.moveOn(ctx, whyLimit) {
		return false
	}
	discard(resp)
	return true
}

// throttle waits, and has the request sent again on its account, while it has
// been throttled there no more than throttleRetries times; after that, the
// answer is the client's. The wait is the one the upstream asks for, within
// reason, and ends early, failing, should the client go.
func (rp *replay) throttle(ctx context.Context, resp *http.Response, retryAfter time.Duration) (bool, error) {
	id := rp.ex.account.ID
	if rp.throttled >= throttleRetries {
		logger.Warn("still throttled; passing the answer on", "id", rp.ex.id, "account", id, "attempts", rp.ex.attempts)
		return false, nil
	}
	rp.throttled++
	wait := min(cmp.Or(retryAfter, throttleWait), maxThrottleWait)
	logger.Warn("throttled", "id", rp.ex.id, "account", id, "wait", wait)
	discard(resp)
	if err := sleep(ctx, wait); err != nil {
		return false, err
	}
	rp.replaying(id, whyThrottled)
	return true, nil
}

// refused bars an account whose token the upstream refused for a while, and
// moves on from it when another account can take the request. When none can,
// the client has a refusal.
func (rp *replay) refused(ctx context.Context, resp *http.Response) (bool, error) {
	a := rp.ex.account
	message := rp.p.provider.ErrorMessage(resp.Body, a.token.Reveal())
	discard(resp)
	rp.p.state.refuse(a.ID)
	logger.Warn("upstream refused the account's token", "id", rp.ex.id, "account", a.ID, "status", resp.StatusCode, "error", prefix(message, refusalShown))
	if rp.moveOn(ctx, whyRefused) {
		return true, nil
	}
	return false, refusal{status: resp.StatusCode}
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
	rp.throttled = 0
	rp.replaying(from, why)
	return true
}

// replaying notes that the request goes out again, on the exchange's
// account, as the account it went out on, from, couldn't serve it.
func (rp *replay) replaying(from, why string) {
	logger.Info("replaying", "id", rp.ex.id, "attempt", rp.ex.attempts+1, "from", from, "to", rp.ex.account.ID, "why", why)
}

// rejected returns the keys of the windows that rejected a request.
func rejected(windows []quota.Window) []string {
	var keys []string
	for _, w := range windows {
		if w.Status == quota.StatusRejected {
			keys = append(keys, w.Key)
		}
	}
	return keys
}

// discard reads what's left of an answer the client isn't to have, up to
// maxDiscard, and closes it.
func discard(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDiscard))
	_ = resp.Body.Close()
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
