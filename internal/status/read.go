package status

import (
	"errors"
	"slices"
	"time"
)

// FreshFor is how lately the router must have read an account for a Read to
// leave it be: the least the router waits between probes of one.
const FreshFor = time.Minute

// ErrNoRouter is what a read that doesn't probe fails with when the router
// doesn't answer.
var ErrNoRouter = errors.New("the router isn't answering")

// A Read is what a read of the status document asks for.
type Read struct {
	// Refresh, when the router answers, has it first probe the accounts it
	// hasn't read for this long; zero takes its document as it stands.
	Refresh time.Duration
	// Probe, when the router doesn't answer, builds the document by probing
	// every account instead. Without it, such a read fails with ErrNoRouter.
	Probe bool
}

// Fresh is the read the dashboard's R asks for, which usage --refresh asks
// for too: the router first refreshes every account it hasn't read in the
// last minute, the least it waits between probes of one, and every one that
// can take no request anyway, or, without the router, every account is
// probed.
func Fresh() Read {
	return Read{Refresh: FreshFor, Probe: true}
}

// ResetGrace is how long after a window resets to read it again, giving the
// provider time to roll it over.
const ResetGrace = time.Minute

// NextFetchAsRead is when to read the document again after reading it at
// read: once wait has passed, or sooner for a window, as read, that resets
// before then, ResetGrace after it does. A reset already past at the read is
// stale, and doesn't count: it would bring on a read at every tick.
func (d Document) NextFetchAsRead(read time.Time, wait time.Duration) time.Time {
	due := []time.Time{read.Add(wait)}
	for _, a := range d.Accounts {
		for _, w := range a.Windows {
			if w.ResetsAt.After(read) {
				due = append(due, w.ResetsAt.Add(ResetGrace))
			}
		}
	}
	return slices.MinFunc(due, time.Time.Compare)
}

// RereadAfterResetAsRead is when a window of the document, as read, wants
// reading again for its reset, ResetGrace after it resets: the first whose
// grace runs past since, or zero where none does. An account with a window
// that has lapsed is passed over, as a refresh probes it only while it can
// take no request: a probe would start the window, which reads empty until a
// request does.
func (d Document) RereadAfterResetAsRead(since time.Time) time.Time {
	var first time.Time
	for _, a := range d.Accounts {
		if len(a.Lapsed) > 0 {
			continue
		}
		for _, w := range a.Windows {
			due := w.ResetsAt.Add(ResetGrace)
			if !w.ResetsAt.IsZero() && due.After(since) && (first.IsZero() || due.Before(first)) {
				first = due
			}
		}
	}
	return first
}

// IncompleteAsRead reports whether the read that gave the document failed in
// part: an account wasn't read in full, in a way another try might mend. A
// missing token isn't one: it needs the user, and retrying would only probe
// every other account again.
func (d Document) IncompleteAsRead() bool {
	return slices.ContainsFunc(d.Accounts, func(a Account) bool {
		return (a.TokenSet && a.Error != "") || len(a.Failures) > 0
	})
}
