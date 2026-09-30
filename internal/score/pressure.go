package score

import (
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// Recent is how far back the readings go that a window's recent rate of
	// use is measured over.
	Recent = 30 * time.Minute
	// steady is how far back the first of a window's recent readings must
	// go for their rise to be its rate: over less, a burst or a lull would
	// pass for its pace.
	steady = 10 * time.Minute
)

// Reading is how much of a window was used, as read at a time.
type Reading struct {
	At          time.Time
	Utilization float64
}

// Pace is how fast a window is being used.
type Pace struct {
	// Rate is the share of the window used an hour.
	Rate float64
	// Recent is set when Rate is the window's rise across its readings of
	// the last half hour, rather than its use since it started.
	Recent bool
}

// PaceOf returns how fast w is being used at now: at its recent rate, as
// RecentRate judges it, while it has one; else at its use since it started,
// once 5% of it has passed, as Project judges it. It reports false when
// neither can say, and once w has reset since it was read.
func PaceOf(w quota.Window, readings []Reading, now time.Time) (Pace, bool) {
	if rate, ok := RecentRate(w, readings, now); ok {
		return Pace{Rate: rate, Recent: true}, true
	}
	if hasReset(w, now) {
		return Pace{}, false
	}
	rate, ok := averageRate(w, now)
	return Pace{Rate: rate}, ok
}

// RecentRate returns how fast w has been used lately, at now: the rise across
// the readings of it taken in the last half hour, over the time from the
// first of them until now, a share of it an hour. Readings come only as the
// window is used, so the rate of one gone quiet falls as time passes, rather
// than holding at its last burst's. The readings are of w as it now runs, in
// the order they were taken. It reports false when the first of them was
// taken less than 10 minutes before now, and once w has reset since it was
// read.
func RecentRate(w quota.Window, readings []Reading, now time.Time) (float64, bool) {
	i := slices.IndexFunc(readings, func(r Reading) bool { return !r.At.Before(now.Add(-Recent)) })
	if i < 0 || hasReset(w, now) {
		return 0, false
	}
	first, last := readings[i], readings[len(readings)-1]
	since := now.Sub(first.At)
	if since < steady {
		return 0, false
	}
	return (last.Utilization - first.Utilization) / since.Hours(), true
}

// averageRate is w's use since it started, an hour, at now: the pace its use
// so far sets. It reports false when w's length or reset isn't known, or
// before 5% of it has passed, when a little use extrapolates wildly.
func averageRate(w quota.Window, now time.Time) (float64, bool) {
	start, length, ok := span(w)
	if !ok {
		return 0, false
	}
	passed := now.Sub(start)
	if fraction(passed, length) < minElapsed {
		return 0, false
	}
	return w.Utilization / passed.Hours(), true
}

// Pressure is whether a window runs out before it resets, used from now on at
// the rate it's being used.
type Pressure struct {
	// RunsOut is when, at that rate, the window reaches where its room ends:
	// zero when it never does, at no rate, and when it has already.
	RunsOut time.Time
	// Under is set when RunsOut comes before the window resets.
	Under bool
}

// PressureOf returns how the candidate's pressure window stands at now, used
// from now on at the candidate's rate: when it reaches where the candidate's
// room ends, at 1 − its reserve, and whether that comes before it resets.
// It's zero when the candidate has no such window, when the window's reset
// isn't known or has passed since it was read, and when it has no room left.
func (p Policy) PressureOf(c Candidate, now time.Time) Pressure {
	w, ok := find(c.Windows, p.Pressure)
	room := 1 - c.Reserve - w.Utilization
	if !ok || c.Rate <= 0 || room <= tolerance || spent(w) || w.ResetsAt.IsZero() || hasReset(w, now) {
		return Pressure{}
	}
	at := now.Add(inHours(room / c.Rate))
	return Pressure{RunsOut: at, Under: at.Before(w.ResetsAt)}
}

// inHours is a count of hours as a duration, to the nanosecond, and endless
// past what a duration holds.
func inHours(h float64) time.Duration {
	if d := h * float64(time.Hour); d < float64(endless) {
		return time.Duration(math.Round(d))
	}
	return endless
}
