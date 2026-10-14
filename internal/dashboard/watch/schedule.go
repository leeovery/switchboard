package watch

import "time"

const (
	// retryReadAfter is how soon to read again after a read failed, in whole
	// or in part: after a wake, the network may not be back yet.
	retryReadAfter = 2 * time.Minute
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

// tickDelay is how long from now until the next tick: just past the next
// second, as the clock on screen shows it, and how long ago the document was
// read.
func tickDelay(now time.Time) time.Duration {
	return now.Truncate(time.Second).Add(time.Second).Sub(now) + tickSlack
}
