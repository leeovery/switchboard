package watch

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// retryAfter is how soon to read again after an account couldn't be read
	// in full: after a wake, the network may not be back yet.
	retryAfter = 2 * time.Minute
	// resetGrace is how long after a window resets to read it again, giving
	// the provider time to roll it over.
	resetGrace = time.Minute
	// tickSlack sets each tick just past the boundary it waits for, so the
	// clock it reads has turned over.
	tickSlack = 50 * time.Millisecond
)

// nextFetch is when to read again after reading doc at read: once interval
// has passed, or sooner for a window that resets before then, a grace period
// after it does, or soon after an account wasn't read in full. A reset already
// past when doc was read is stale, and doesn't count: it would bring on a read
// at every tick.
func nextFetch(doc status.Document, read time.Time, interval time.Duration) time.Time {
	due := []time.Time{read.Add(interval)}
	for _, a := range doc.Accounts {
		if incomplete(a) {
			due = append(due, read.Add(retryAfter))
		}
		for _, w := range a.Windows {
			if w.ResetsAt.After(read) {
				due = append(due, w.ResetsAt.Add(resetGrace))
			}
		}
	}
	return slices.MinFunc(due, time.Time.Compare)
}

// incomplete reports whether an account wasn't read in full, in a way another
// try might mend. A missing token isn't one: it needs the user, and retrying
// would only probe every other account again.
func incomplete(a status.Account) bool {
	return (a.TokenSet && a.Error != "") || len(a.Failures) > 0
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
