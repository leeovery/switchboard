package watch

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// retryReadAfter is how soon to read again after a read failed, in whole
	// or in part: after a wake, the network may not be back yet.
	retryReadAfter = 2 * time.Minute
	// resetGrace is how long after a window resets to read it again, giving
	// the provider time to roll it over.
	resetGrace = time.Minute
	// tickSlack sets each tick just past the boundary it waits for, so the
	// clock it reads has turned over.
	tickSlack = 50 * time.Millisecond
)

// backoff counts a read into the reads that failed in a row before it, and
// says how long to wait for the next. After a read that failed, it's
// retryReadAfter, doubled for each failure in a row before it, up to
// interval: a failure that lasts, such as a refused token, settles to a read
// an interval rather than a read of every account every two minutes. After a
// read that didn't fail, it's the interval, and the count starts again.
func backoff(failures int, failed bool, interval time.Duration) (int, time.Duration) {
	if !failed {
		return 0, interval
	}
	failures++
	wait := retryReadAfter
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

// rereadAfterReset returns when a window in doc wants reading again for its
// reset, a grace period after it resets: the first whose grace period runs
// past since, or zero when there's none. An account with a window that has
// lapsed is passed over, as the router probes it only while it can take no
// request: a probe would start the window, which reads empty until a request
// does.
func rereadAfterReset(doc status.Document, since time.Time) time.Time {
	var first time.Time
	for _, a := range doc.Accounts {
		if len(a.Lapsed) > 0 {
			continue
		}
		for _, w := range a.Windows {
			due := w.ResetsAt.Add(resetGrace)
			if !w.ResetsAt.IsZero() && due.After(since) && (first.IsZero() || due.Before(first)) {
				first = due
			}
		}
	}
	return first
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
// second, as the clock on screen shows it, and how long ago the document was
// read.
func tickDelay(now time.Time) time.Duration {
	return now.Truncate(time.Second).Add(time.Second).Sub(now) + tickSlack
}
