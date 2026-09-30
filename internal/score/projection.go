package score

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// minElapsed is how much of a window must have passed before its use says
// where it's heading: early on, a little use extrapolates wildly.
const minElapsed = 0.05

// Kind is which of its forms a Projection takes.
type Kind int

const (
	// Unknown says nothing: the window's reading can't honestly be projected.
	Unknown Kind = iota
	// OnPace ends the window below its limit, having used AtReset.
	OnPace
	// RunsOut reaches the window's limit At a time before it resets.
	RunsOut
	// Exhausted has no room left until the window is back At its reset,
	// which is zero when unknown.
	Exhausted
)

// Projection is where a window's use is heading.
type Projection struct {
	Kind Kind
	// AtReset is the utilization an OnPace window will have reached when it
	// resets, below 1.
	AtReset float64
	// At is when a RunsOut window runs out, or when an Exhausted one is back.
	At time.Time
}

// Elapsed returns how much of w has passed at now, from 0 to 1: where even
// use across the window would have put its utilization, which is what a pace
// marker shows. It reports false unless w's length and reset are known.
func Elapsed(w quota.Window, now time.Time) (float64, bool) {
	start, length, ok := span(w)
	if !ok {
		return 0, false
	}
	return fraction(now.Sub(start), length), true
}

// Project says where w is heading at now. A window with no room left is
// Exhausted. Otherwise its use so far sets the pace: OnPace when that pace
// ends the window below its limit, RunsOut when it reaches the limit first.
// It's Unknown when w's length or reset isn't known, when w has reset since it
// was read, or before 5% of w has passed.
func Project(w quota.Window, now time.Time) Projection {
	if p, ok := settled(w, now); ok {
		return p
	}
	rate, ok := averageRate(w, now)
	if !ok {
		return Projection{}
	}
	return heading(w, rate, now)
}

// ProjectAt says where w is heading at now, used from now on at rate, a share
// of it an hour, as Project says it at the pace its use so far sets. It's
// Unknown when w's reset isn't known, or w has reset since it was read.
func ProjectAt(w quota.Window, rate float64, now time.Time) Projection {
	if p, ok := settled(w, now); ok || w.ResetsAt.IsZero() {
		return p
	}
	return heading(w, rate, now)
}

// settled returns where w is heading at now whatever the pace of its use,
// reporting false when the pace decides: once it has reset since it was read,
// its reading says nothing, and with no room left, it's Exhausted.
func settled(w quota.Window, now time.Time) (Projection, bool) {
	switch {
	case hasReset(w, now):
		return Projection{}, true
	case spent(w):
		return Projection{Kind: Exhausted, At: w.ResetsAt}, true
	}
	return Projection{}, false
}

// heading is where w is heading at now, used from now on at rate, a share of
// it an hour: OnPace when that ends it below its limit at its reset, else
// RunsOut when it reaches the limit.
func heading(w quota.Window, rate float64, now time.Time) Projection {
	if atReset := w.Utilization + rate*w.ResetsAt.Sub(now).Hours(); atReset < 1 {
		return Projection{Kind: OnPace, AtReset: atReset}
	}
	return Projection{Kind: RunsOut, At: now.Add(inHours((1 - w.Utilization) / rate))}
}

// span returns when w began and how long it runs until it resets: from a
// whole length before its reset, or from when it started again, where it did
// since. It reports false unless both its length and its reset are known.
func span(w quota.Window) (time.Time, time.Duration, bool) {
	length, ok := quota.Length(w.Key)
	if !ok || w.ResetsAt.IsZero() {
		return time.Time{}, 0, false
	}
	start := w.ResetsAt.Add(-length)
	if w.RestartedAt.After(start) && w.RestartedAt.Before(w.ResetsAt) {
		start = w.RestartedAt
	}
	return start, w.ResetsAt.Sub(start), true
}

// fraction is how much of length d makes up, from 0 to 1.
func fraction(d, length time.Duration) float64 {
	return min(max(float64(d)/float64(length), 0), 1)
}

// hasReset reports whether w has reset by now. Its reading is then stale:
// the window has started afresh.
func hasReset(w quota.Window, now time.Time) bool {
	return !w.ResetsAt.IsZero() && !w.ResetsAt.After(now)
}

// spent reports whether w's reading leaves no room: it's used up, or the
// provider refuses it.
func spent(w quota.Window) bool {
	return w.Utilization >= 1 || w.Status == quota.StatusRejected
}

// atReserve reports whether w's reading has reached the reserve, the share of
// the window left unused, and nothing else holds it back: 1 − reserve of it is
// used, but it isn't spent. Without a reserve, nothing reaches it.
func atReserve(w quota.Window, reserve float64) bool {
	return reserve > 0 && w.Utilization >= 1-reserve-tolerance && !spent(w)
}
