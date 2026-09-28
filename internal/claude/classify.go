package claude

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// overallStatus is the header giving the API's verdict on a request across
	// every window, where each window's own status header gives that window's.
	overallStatus = "anthropic-ratelimit-unified-status"
	// overallReset is the header giving when the claim that decided the
	// overall verdict resets, in Unix seconds.
	overallReset = "anthropic-ratelimit-unified-reset"
)

// maxRetrySeconds is the longest Retry-After a duration can hold.
const maxRetrySeconds = math.MaxInt64 / int64(time.Second)

// Classify says what a response to a request says of the account it went out
// on. A 401 or 403 refuses the account's token. A 429 is the account's limit
// reached when the overall status or any window's is rejected, carrying until
// when as far as the headers say, and otherwise throttling, carrying the
// Retry-After it gives. Anything else, a 529 or another 5xx among them, says
// nothing against the account: Claude Code retries those itself.
func (Provider) Classify(status int, h http.Header) quota.Outcome {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return quota.Outcome{Verdict: quota.Refused}
	case status != http.StatusTooManyRequests:
		return quota.Outcome{Verdict: quota.Served}
	}
	if until, reached := limitReached(h); reached {
		return quota.Outcome{Verdict: quota.LimitReached, LimitedUntil: until}
	}
	return quota.Outcome{Verdict: quota.Throttled, RetryAfter: retryAfter(h)}
}

// limitReached reports whether the headers reject a request for want of
// quota: the overall status is rejected, or a window's is. Overage's status
// isn't a window's, and doesn't count: it reads rejected on every account
// without extra usage, throttled or not. It returns until when, as far as the
// headers say: the overall reset, the rejecting claim's; else the latest of
// the rejected windows' resets, as the account has room only once each has
// reset; else zero.
func limitReached(h http.Header) (until time.Time, reached bool) {
	reached = quota.Status(h.Get(overallStatus)) == quota.StatusRejected
	for _, fields := range windowFields(h) {
		if quota.Status(fields["status"]) != quota.StatusRejected {
			continue
		}
		reached = true
		if reset := parseReset(fields["reset"]); reset.After(until) {
			until = reset
		}
	}
	if overall := parseReset(h.Get(overallReset)); !overall.IsZero() {
		until = overall
	}
	return until, reached
}

// retryAfter reads the delay a Retry-After header gives in seconds, or zero
// when it gives none that can be read.
func retryAfter(h http.Header) time.Duration {
	seconds, err := strconv.ParseInt(strings.TrimSpace(h.Get("Retry-After")), 10, 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(min(seconds, maxRetrySeconds)) * time.Second
}
