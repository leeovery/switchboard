package forecast

import (
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

const (
	// fastTau is the fast rate's time constant: a rise counts e^(−1) as much
	// an hour on as it did when it came.
	fastTau = time.Hour
	// dayRoom is how much time with room the day's rate is measured over, at
	// the most.
	dayRoom = 24 * time.Hour
	// dayLeast is the least time with room the day's rate is measured over:
	// over less, as on a new install, it has none.
	dayLeast = 2 * time.Hour
)

// Fast is a window's fast rate: its rises, each counting less as it ages,
// with a time constant of an hour, as a load average is smoothed. A point's
// tick reads as a point an hour as it comes, and as e^(−1) of that an hour
// on, not as 2% an hour for the half hour after it. Its zero value has taken
// in no rise, and reads 0.
type Fast struct {
	rate float64
	at   time.Time
}

// Rose returns f having taken in a rise of by, a share of the window, at at.
// A rise before the last f took in counts as at the last, and one of nothing
// or less, as no rise, leaves f as it was.
func (f Fast) Rose(by float64, at time.Time) Fast {
	if by <= 0 {
		return f
	}
	at = score.Later(at, f.at)
	return Fast{rate: f.Rate(at) + by/fastTau.Hours(), at: at}
}

// Rate returns f's rate at at, a share of the window an hour, never
// negative: as of its last rise where at comes before it.
func (f Fast) Rate(at time.Time) float64 {
	return f.rate * decay(at.Sub(f.at), fastTau)
}

// decay is how much of a value smoothed with the time constant tau is left
// after d: all of it after no time, or less.
func decay(d, tau time.Duration) float64 {
	return math.Exp(-max(d, 0).Hours() / tau.Hours())
}

// Day is the pool's day rate: its accounts' rises, in Pro weeks, over the
// last 24 hours in which it had room for a new session, divided by those
// hours. Hours without room don't count, as they weren't quiet, but rises in
// them do, as use can be read late. Its zero value has no room and has taken
// in nothing.
type Day struct {
	// rooms are the stretches it had room in, oldest first, the last with no
	// end while it has room now.
	rooms []room
	// rises are oldest first.
	rises []rise
	open  bool
	// last is the latest time it has been given.
	last time.Time
}

// room is a stretch of time the pool had room in, to being zero while it
// still has.
type room struct{ from, to time.Time }

// rise is a rise of the pool's use, in Pro weeks.
type rise struct {
	at time.Time
	by float64
}

// Rose returns d having taken in a rise of the pool's use of by, in Pro weeks,
// at at. A rise before the latest time d was given counts as at that time,
// and one of nothing or less, as no rise, leaves d as it was.
func (d Day) Rose(by float64, at time.Time) Day {
	if by <= 0 {
		return d
	}
	d.last = score.Later(at, d.last)
	rises := slices.Clone(d.rises)
	if n := len(rises); n > 0 && d.closedSince(rises[n-1].at) {
		// Rises while the pool has no room all fall on one side of where its
		// span starts, which moves only through time with room: kept as one,
		// they stay one however long it has none.
		rises[n-1] = rise{at: d.last, by: rises[n-1].by + by}
	} else {
		rises = append(rises, rise{at: d.last, by: by})
	}
	d.rises = rises
	return d.pruned()
}

// Open returns d with the pool having room for a new session from at on, or
// none. A time before the latest d was given counts as that time.
func (d Day) Open(open bool, at time.Time) Day {
	d.last = score.Later(at, d.last)
	if open != d.open {
		d.rooms = d.turned(open)
		d.open = open
	}
	return d.pruned()
}

// turned is d's rooms with the pool's room turned open or closed at its
// latest time: a room opened, or the open one ended, and dropped where it
// ends as it began.
func (d Day) turned(open bool) []room {
	rooms := slices.Clone(d.rooms)
	if open {
		return append(rooms, room{from: d.last})
	}
	n := len(rooms) - 1
	if rooms[n].from.Equal(d.last) {
		return rooms[:n]
	}
	rooms[n].to = d.last
	return rooms
}

// Rate returns the pool's day rate at at, in Pro weeks an hour, and the time
// with room it's measured over, 24 hours at the most. It reports false with
// less than 2 hours of room. A time before the latest d was given counts as
// that time.
func (d Day) Rate(at time.Time) (rate float64, over time.Duration, ok bool) {
	at = score.Later(at, d.last)
	from, over := d.span(at)
	if over < dayLeast {
		return 0, 0, false
	}
	var risen float64
	for _, r := range d.rises {
		if r.at.After(from) {
			risen += r.by
		}
	}
	return risen / over.Hours(), over, true
}

// span returns when d's span starts, going back from at: 24 hours with room
// before it, or where its room began with less; and the time with room in
// it. A rise counts in it where it comes after its start, as a rise read
// then is of use before it.
func (d Day) span(at time.Time) (from time.Time, over time.Duration) {
	need := dayRoom
	for _, r := range slices.Backward(d.rooms) {
		to := r.to
		if to.IsZero() {
			to = at
		}
		held := to.Sub(r.from)
		if held >= need {
			return to.Add(-need), dayRoom
		}
		need -= held
	}
	if len(d.rooms) == 0 {
		return at, 0
	}
	return d.rooms[0].from, dayRoom - need
}

// closedSince reports whether the pool has had no room since at.
func (d Day) closedSince(at time.Time) bool {
	n := len(d.rooms)
	return !d.open && (n == 0 || at.After(d.rooms[n-1].to))
}

// pruned is d without what lies before its span at its latest time: as that
// only moves on, none of it counts again.
func (d Day) pruned() Day {
	from, _ := d.span(d.last)
	keep := slices.IndexFunc(d.rooms, func(r room) bool { return r.to.IsZero() || r.to.After(from) })
	if keep < 0 {
		keep = len(d.rooms)
	}
	since := slices.IndexFunc(d.rises, func(r rise) bool { return r.at.After(from) })
	if since < 0 {
		since = len(d.rises)
	}
	d.rooms, d.rises = d.rooms[keep:], d.rises[since:]
	return d
}
