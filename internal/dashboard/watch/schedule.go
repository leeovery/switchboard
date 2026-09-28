package watch

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// retryAfter is how soon to read again after a read failed, in whole or in
	// part: after a wake, the network may not be back yet.
	retryAfter = 2 * time.Minute
	// resetGrace is how long after a window resets to read it again, giving
	// the provider time to roll it over.
	resetGrace = time.Minute
	// tickSlack sets each tick just past the boundary it waits for, so the
	// clock it reads has turned over.
	tickSlack = 50 * time.Millisecond
)

// backoff counts a read into the reads that failed in a row before it, and
// says how long to wait for the next. After a read that failed, it's
// retryAfter, doubled for each failure in a row before it, up to interval: a
// failure that lasts, such as a refused token, settles to a read an interval
// rather than a read of every account every two minutes. After a read that
// didn't fail, it's the interval, and the count starts again.
func backoff(failures int, failed bool, interval time.Duration) (int, time.Duration) {
	if !failed {
		return 0, interval
	}
	failures++
	wait := retryAfter
	for i := 1; i < failures && wait < interval; i++ {
		wait *= 2
	}
	return failures, min(wait, interval)
}

// nextFetch is when to read again after a read at read: once wait has passed,
// or sooner for a window in doc that resets before then, a grace period after
// it does. A reset already past at the read is stale, and doesn't count: it
// would bring on a read at every tick.
func nextFetch(doc status.Document, read time.Time, wait time.Duration) time.Time {
	due := []time.Time{read.Add(wait)}
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			if w.ResetsAt.After(read) {
				due = append(due, w.ResetsAt.Add(resetGrace))
			}
		}
	}
	return slices.MinFunc(due, time.Time.Compare)
}

// incomplete reports whether a read of doc failed in part: an account wasn't
// read in full, in a way another try might mend. A missing token isn't one: it
// needs the user, and retrying would only probe every other account again.
func incomplete(doc status.Document) bool {
	return slices.ContainsFunc(doc.Accounts, func(a status.Account) bool {
		return (a.TokenSet && a.Error != "") || len(a.Failures) > 0
	})
}

// tickDelay is how long from now until the next tick: just past the next
// second while a frame of doc counts seconds, else just past the next minute.
func tickDelay(doc status.Document, now time.Time) time.Duration {
	step := time.Minute
	if dashboard.CountsSeconds(doc, now) {
		step = time.Second
	}
	return now.Truncate(step).Add(step).Sub(now) + tickSlack
}
