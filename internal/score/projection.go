package score

import (
	"math"
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
	// OnPace ends the window having used AtReset: short of its limit, or at
	// it just as it resets.
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
	// resets, 1 at the most.
	AtReset float64
	// At is when a RunsOut window runs out, or when an Exhausted one is back.
	At time.Time
}

// Elapsed returns how much of w has passed at now, from 0 to 1: where even
// use across the window would have put its utilization, which is what a pace
// marker shows. It reports false unless w's length and reset are known.
func Elapsed(w quota.Window, now time.Time) (float64, bool) {
	start, length, ok := w.Span()
	if !ok {
		return 0, false
	}
	return fraction(now.Sub(start), length), true
}

// Project says where w is heading at now. A window with no room left is
// Exhausted. Otherwise its use so far sets the pace: RunsOut when that pace
// reaches its limit before it resets, else OnPace, as heading says. It's
// Unknown when w's length or reset isn't known, when w has reset since it was
// read, or before 5% of w has passed.
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

// Reaches returns when w's use, heading as p projects it from now, reaches
// floor, a share of w at or short of its limit, such as where a reserve
// starts; at the limit, 1, that's when p has it run out. It reports false
// where that isn't before w resets: where p keeps its use short of floor,
// brings it there just as w resets, or says nothing; and where its use has
// reached floor already.
func Reaches(w quota.Window, p Projection, floor float64, now time.Time) (time.Time, bool) {
	used := w.Utilization
	var at time.Time
	switch {
	case used >= floor-Tolerance:
		return time.Time{}, false
	case p.Kind == RunsOut:
		at = now.Add(scaled(p.At.Sub(now), (floor-used)/(1-used)))
	case p.Kind == OnPace && p.AtReset > floor:
		at = now.Add(scaled(w.ResetsAt.Sub(now), (floor-used)/(p.AtReset-used)))
	default:
		return time.Time{}, false
	}
	// The reset is checked on the time, not the share: AtReset can land a few
	// ulps over a floor the window reaches just as it resets, and the time
	// rounds to the reset itself, as an experiment found of 1% used, resetting
	// in 22 minutes, at the rate that ends it at 90% then.
	if !at.Before(w.ResetsAt) {
		return time.Time{}, false
	}
	return at, true
}

// scaled is d times by.
func scaled(d time.Duration, by float64) time.Duration {
	return time.Duration(math.Round(float64(d) * by))
}

// Sooner reports whether b, a projection of a window, has it run out sooner
// than a, another of the same window as it stands: running out before a
// does, or at all where a doesn't, or ending the window more used; or saying
// anything, where a says nothing. Two that end alike aren't.
func Sooner(a, b Projection) bool {
	switch {
	case a.Kind != b.Kind:
		return urgency(b.Kind) > urgency(a.Kind)
	case a.Kind == RunsOut:
		return b.At.Before(a.At)
	case a.Kind == OnPace:
		return b.AtReset > a.AtReset
	default:
		return false
	}
}

// urgency ranks a projection's kind by how soon it has its window run out:
// saying nothing least, then keeping pace, then running out, then exhausted.
func urgency(k Kind) int {
	switch k {
	case Exhausted:
		return 3
	case RunsOut:
		return 2
	case OnPace:
		return 1
	default:
		return 0
	}
}

// settled returns where w is heading at now whatever the pace of its use,
// reporting false when the pace decides: once it has reset since it was read,
// its reading says nothing, and with no room left, it's Exhausted.
func settled(w quota.Window, now time.Time) (Projection, bool) {
	switch {
	case w.ResetBy(now):
		return Projection{}, true
	case Spent(w):
		return Projection{Kind: Exhausted, At: w.ResetsAt}, true
	}
	return Projection{}, false
}

// heading is where w is heading at now, used from now on at rate, a share of
// it an hour: RunsOut where that reaches its limit before it resets, as
// PressureOf judges a window running out; else OnPace, at its limit at the
// most, as it reaches it no sooner than it resets, or at no rate, never.
func heading(w quota.Window, rate float64, now time.Time) Projection {
	if out := now.Add(inHours((1 - w.Utilization) / rate)); rate > 0 && out.Before(w.ResetsAt) {
		return Projection{Kind: RunsOut, At: out}
	}
	return Projection{Kind: OnPace, AtReset: min(w.Utilization+rate*w.ResetsAt.Sub(now).Hours(), 1)}
}

// fraction is how much of length d makes up, from 0 to 1.
func fraction(d, length time.Duration) float64 {
	return min(max(float64(d)/float64(length), 0), 1)
}

// Spent reports whether w's reading leaves no room: it's used up, or the
// provider refuses it.
func Spent(w quota.Window) bool {
	return w.Utilization >= 1 || w.Status == quota.StatusRejected
}

// HeldByReserve reports whether w's reading has reached the reserve, the
// share of the window left unused, and nothing else holds it back: 1 − reserve
// of it is used, but it isn't spent. Without a reserve, nothing reaches it.
func HeldByReserve(w quota.Window, reserve float64) bool {
	return reserve > 0 && w.Utilization >= 1-reserve-Tolerance && !Spent(w)
}

// handReset is how far a window's use must fall, its reset kept, for the
// reading taken as current to show it started again, as a reset made by hand
// starts it, emptying it: a smaller dip, as a 429 reading a point below the
// use read just before, is noise, and the window runs on.
const handReset = 0.10

// ResetByHand reports whether kept, the reading a window now stands as, shows
// it reset by hand since held, the one it stood as before: its reset is the
// same, and its use has fallen by handReset at least, allowing for rounding,
// as 0.3 less 0.2 reads a hair under 0.1.
func ResetByHand(held, kept quota.Window) bool {
	return kept.ResetsAt.Equal(held.ResetsAt) && held.Utilization-kept.Utilization >= handReset-Tolerance
}
