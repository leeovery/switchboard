package score

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// day is the longest window whose allowance is given by the hour: one longer
// has its allowance given by the day.
const day = 24 * time.Hour

// Current returns w as it runs at now: as read, until its reset; once that
// has passed since it was read, started afresh from it, empty, its next reset
// a length on, or as many lengths on as have passed since. A window whose
// length isn't known stands as read.
func Current(w quota.Window, now time.Time) quota.Window {
	length, ok := quota.Length(w.Key)
	if !ok || !w.ResetBy(now) {
		return w
	}
	lengths := int64(now.Sub(w.ResetsAt)/length) + 1
	return quota.Window{Key: w.Key, Label: w.Label, ResetsAt: w.ResetsAt.Add(time.Duration(lengths) * length)}
}

// EvenPace returns w's even pace at now: where even use across it would have
// put its use, from 0 to 1, as Elapsed says, as it runs then, as Current has
// it, so from its last reset once that has passed since it was read. It
// reports false unless w's length and reset are known.
func EvenPace(w quota.Window, now time.Time) (float64, bool) {
	return Elapsed(Current(w, now), now)
}

// Room returns how much of w is left at now before its use reaches floor, a
// share of it at or short of its limit, such as where a reserve starts, as
// it runs then, as Current has it: none where it's spent, and never less than
// none, as its use can pass its limit.
func Room(w quota.Window, floor float64, now time.Time) float64 {
	if w = Current(w, now); Spent(w) {
		return 0
	}
	return max(floor-w.Utilization, 0)
}

// AllowanceOf returns what can be spent of w at now and still last to its
// reset, its use running to floor, as it runs then, as Current has it: its
// Room over the hours to its reset, for a window of a day or less, or the
// days, for a longer one; or the room itself, Per left out, where its reset
// comes within that hour or day. It reports false where no room is left, or
// w's length or reset isn't known.
func AllowanceOf(w quota.Window, floor float64, now time.Time) (quota.Allowance, bool) {
	w = Current(w, now)
	length, ok := quota.Length(w.Key)
	room := Room(w, floor, now)
	if !ok || w.ResetsAt.IsZero() || room <= Tolerance {
		return quota.Allowance{}, false
	}
	per, name := time.Hour, quota.PerHour
	if length > day {
		per, name = day, quota.PerDay
	}
	until := w.ResetsAt.Sub(now)
	if until < per {
		return quota.Allowance{Share: room}, true
	}
	return quota.Allowance{Share: room / (float64(until) / float64(per)), Per: name}, true
}
