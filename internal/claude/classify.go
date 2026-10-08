package claude

import (
	"math"
	"net/http"
	"slices"
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
// on. A 401 refuses the account's token, and a 403 forbids the account the
// request alone, as for a model or beta its plan lacks. A 429 that carries the
// usage headers is the account's limit reached when the overall status or any
// window's is rejected, carrying the windows rejected and until when as far as
// the headers say, and otherwise throttling, carrying the Retry-After it
// gives. A 429 without them, neither the overall status nor any window's,
// says nothing of the account's quota: it refuses the request itself, which
// would fare no better sent again. That, and anything else, a 529 or another
// 5xx among them, says nothing against the account: Claude Code retries those
// itself.
func (Provider) Classify(status int, h http.Header) quota.Outcome {
	switch {
	case status == http.StatusUnauthorized:
		return quota.Outcome{Verdict: quota.Refused}
	case status == http.StatusForbidden:
		return quota.Outcome{Verdict: quota.Forbidden}
	case status != http.StatusTooManyRequests || !carriesUsage(h):
		return quota.Outcome{Verdict: quota.Served}
	}
	if rejected, until, reached := limitReached(h); reached {
		return quota.Outcome{Verdict: quota.LimitReached, Rejected: rejected, LimitedUntil: until}
	}
	return quota.Outcome{Verdict: quota.Throttled, RetryAfter: retryAfter(h)}
}

// MarkLimited sets h's usage headers to say, as the API says it on a 429, that
// the request is rejected for want of quota, until until when that's known:
// the overall status rejected, and the overall reset. Claude Code reads a
// limit, and when it lifts, off them, and so does Classify.
func (Provider) MarkLimited(h http.Header, until time.Time) {
	h.Set(overallStatus, string(quota.StatusRejected))
	if !until.IsZero() {
		h.Set(overallReset, strconv.FormatInt(until.Unix(), 10))
	}
}

// carriesUsage reports whether the headers say anything of the account's
// quota: the overall status, or any window's fields.
func carriesUsage(h http.Header) bool {
	return h.Get(overallStatus) != "" || len(windowFields(h)) > 0
}

// limitReached reports whether the headers reject a request for want of
// quota: the overall status is rejected, or a window's is, whatever else the
// headers say of the window. Overage's status isn't a window's, and doesn't
// count: it reads rejected on every account without extra usage, throttled or
// not. It returns the keys of the windows rejected, in quota.Sort's order, and
// until when, as far as the headers say: the overall reset, the rejecting
// claim's; else the latest of the rejected windows' resets, as the account has
// room only once each has reset; else zero.
func limitReached(h http.Header) (rejected []string, until time.Time, reached bool) {
	for key, f := range windowFields(h) {
		if quota.Status(f.status) != quota.StatusRejected {
			continue
		}
		rejected = append(rejected, key)
		if reset := parseReset(f.reset); reset.After(until) {
			until = reset
		}
	}
	if overall := parseReset(h.Get(overallReset)); !overall.IsZero() {
		until = overall
	}
	slices.SortFunc(rejected, quota.CompareKeys)
	reached = len(rejected) > 0 || quota.Status(h.Get(overallStatus)) == quota.StatusRejected
	return rejected, until, reached
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
