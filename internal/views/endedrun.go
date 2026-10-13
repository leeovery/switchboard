package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// endedRun is the run of an account's window that ended last, as its days
// tell it: the share it had left as it reset, its use read just before that
// reset taken from the whole; and the date of the local day it last reached
// its limit in that run, "" where it reached none.
type endedRun struct {
	left   float64
	ranOut string
}

// lastEnded is the run of the window with the given key that ended last among
// an account's days, oldest first, as their resets give it: from the reset
// before it, where that's within the window's length of its end, else from
// that length before its end, as began has it, to the reset that ended it. It
// reports false where no reset of the window ended one.
func lastEnded(days []ledger.AccountDay, key string) (endedRun, bool) {
	var resets []ledger.Reset
	for _, d := range days {
		for _, r := range d.Resets {
			if r.Window == key {
				resets = append(resets, r)
			}
		}
	}
	if len(resets) == 0 {
		return endedRun{}, false
	}
	slices.SortStableFunc(resets, func(a, b ledger.Reset) int { return a.At.Compare(b.At) })
	end := resets[len(resets)-1]
	var last time.Time
	if i := slices.IndexFunc(resets, func(r ledger.Reset) bool { return r.At.Equal(end.At) }); i > 0 {
		last = resets[i-1].At
	}
	run := endedRun{left: millionthsOf(end.Before).left().share()}
	if at, ok := lastLimit(days, key, runBegan(key, last, end.At), end.At); ok {
		run.ranOut = dayfile.DateOf(at)
	}
	return run, true
}

// runBegan is when a run of the window with the given key that ended at end
// began: at the reset before it, at last, where there was one, as began has
// it for a window as long as the key says; else, of a key that doesn't say,
// at last, or ever, where there was none.
func runBegan(key string, last, end time.Time) time.Time {
	length, ok := quota.Length(key)
	if !ok {
		return last
	}
	return began(last, end, length)
}

// began is when a run of a window length long that ended at end began: at
// the reset before it, at last, where that was within its length of end;
// else, where there was none, or one came between that the days don't give,
// length before end, as a window's span gives it.
func began(last, end time.Time, length time.Duration) time.Time {
	if start := end.Add(-length); last.Before(start) {
		return start
	}
	return last
}

// lastLimit is when, of the limits of the window with the given key that an
// account's days tell of, it last reached one from from up to to, reporting
// false where it reached none then.
func lastLimit(days []ledger.AccountDay, key string, from, to time.Time) (time.Time, bool) {
	var at time.Time
	found := false
	for _, d := range days {
		for _, l := range d.Limits {
			if l.Window == key && !l.At.Before(from) && l.At.Before(to) {
				at, found = score.Later(at, l.At), true
			}
		}
	}
	return at, found
}
