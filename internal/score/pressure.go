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
	// steady is how far back a window's readings must reach for their rise
	// to be its rate: over less, a burst or a lull would pass for its pace.
	steady = 10 * time.Minute
)

// Reading is a level a window's use was read at: first at At, and again, the
// same, until Last.
type Reading struct {
	At          time.Time
	Utilization float64
	// Last is when the use was last read so, at At or after: zero is At.
	Last time.Time
}

// lastRead is when the reading's use was last read so.
func (r Reading) lastRead() time.Time {
	return later(r.At, r.Last)
}

// Pace is how fast a window is being used.
type Pace struct {
	// Rate is the share of the window used an hour.
	Rate float64
	// Recent is set when Rate is the window's recent rate, as RecentRate
	// judges it, rather than its use since it started. Since is when that's
	// measured from, zero when it isn't Recent.
	Recent bool
	Since  time.Time
}

// PaceOf returns how fast w is being used at now: at its recent rate, as
// RecentRate judges it, while it has one; else at its use since it started,
// once 5% of it has passed, as Project judges it. It reports false when
// neither can say, and once w has reset since it was read.
func PaceOf(w quota.Window, readings []Reading, now time.Time) (Pace, bool) {
	if rate, since, ok := RecentRate(w, readings, now); ok {
		return Pace{Rate: rate, Recent: true, Since: since}, true
	}
	if hasReset(w, now) {
		return Pace{}, false
	}
	rate, ok := averageRate(w, now)
	return Pace{Rate: rate}, ok
}

// Lately returns the readings of a window, of those given in the order they
// were first taken, that its recent rate at now goes by: the last first taken
// half an hour or more before now, its baseline, if there is one, and those
// first taken since. The rest are past use.
func Lately(readings []Reading, now time.Time) []Reading {
	since := slices.IndexFunc(readings, func(r Reading) bool { return r.At.After(now.Add(-Recent)) })
	if since < 0 {
		since = len(readings)
	}
	return readings[max(since-1, 0):]
}

// RecentRate returns how fast w has been used lately, at now, a share of it
// an hour, never less than none, and when that's measured from. It's the
// rise from its baseline, the level read last before the last half hour, to
// its latest: over the half hour when the baseline was read again since the
// half hour began, as its use held there till then; else over the time since
// the baseline was last read, as a rise across a gap in its readings, as of
// use outside the router that a probe reads, came at no telling when within
// the gap. Readings come only as a window's use changes, or as it's read, so
// one with a baseline but no level since has been quiet, and reads 0, and a
// burst holds its rate for the half hour, then drops to 0. Without a
// baseline, the rise is from its first level, over the time since it was
// first read, which must be 10 minutes back at least. The readings are of w's
// levels as it now runs, in the order they were first read. It reports false
// when none reaches that far back, and once w has reset since it was read.
func RecentRate(w quota.Window, readings []Reading, now time.Time) (rate float64, since time.Time, ok bool) {
	lately := Lately(readings, now)
	if len(lately) == 0 || hasReset(w, now) {
		return 0, time.Time{}, false
	}
	from, last := lately[0], lately[len(lately)-1]
	since = from.At
	if cutoff := now.Add(-Recent); !from.At.After(cutoff) {
		since = earlier(from.lastRead(), cutoff)
	}
	over := now.Sub(since)
	if over < steady {
		return 0, time.Time{}, false
	}
	return max(last.Utilization-from.Utilization, 0) / over.Hours(), since, true
}

// later returns the later of two times.
func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// earlier returns the earlier of two times.
func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// averageRate is w's use since it started, an hour, at now: the pace its use
// so far sets. It reports false when w's length or reset isn't known, or
// before 5% of it has passed, when a little use extrapolates wildly.
func averageRate(w quota.Window, now time.Time) (float64, bool) {
	start, length, ok := w.Span()
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
	// Rate is the rate it's judged at, a share of the window an hour.
	Rate float64
	// RunsOut is when, at that rate, the window reaches where its room ends:
	// zero when it never does, at no rate, and when it has already.
	RunsOut time.Time
	// Under is set when RunsOut comes before the window resets.
	Under bool
}

// PressureOf returns how the candidate's pressure window stands at now, used
// from now on at the candidate's rate: when it reaches where the candidate's
// room ends, at 1 − its reserve, and whether that comes before it resets.
// It's zero, its rate included, when the candidate has no such window, when
// the window's reset isn't known or has passed since it was read, and when it
// has no room left.
func (p Policy) PressureOf(c Candidate, now time.Time) Pressure {
	w, ok := find(c.Windows, p.Pressure)
	room := 1 - c.Reserve - w.Utilization
	if !ok || c.Rate <= 0 || room <= Tolerance || spent(w) || w.ResetsAt.IsZero() || hasReset(w, now) {
		return Pressure{}
	}
	at := now.Add(inHours(room / c.Rate))
	return Pressure{Rate: c.Rate, RunsOut: at, Under: at.Before(w.ResetsAt)}
}

// inHours is a count of hours as a duration, to the nanosecond, and endless
// past what a duration holds.
func inHours(h float64) time.Duration {
	if d := h * float64(time.Hour); d < float64(endless) {
		return time.Duration(math.Round(d))
	}
	return endless
}
