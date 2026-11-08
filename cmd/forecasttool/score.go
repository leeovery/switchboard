package main

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
)

// missBy is how far a forecast misses either way before it counts as early or
// late.
const missBy = 12 * time.Hour

// usedFrom are where the buckets of how much of its week was used as a
// forecast was made begin, each running to the next, the last to the whole
// week.
var usedFrom = [...]float64{0, 0.4, 0.7}

// week is an account's week, known by when it resets.
type week struct {
	account string
	resets  time.Time
}

// weekOf is the account with the given id's week that resets at resets.
func weekOf(id string, resets time.Time) week {
	return week{account: id, resets: resets.UTC()}
}

// forecast is where a week was forecast at an hour to head: how much of it
// was used then, and when it was to end, at its run-out where it was forecast
// to run out, else at its reset.
type forecast struct {
	week
	at   time.Time
	used float64
	ends time.Time
}

// outcomes are what became of the accounts' weeks, as the readings history
// read them: when each that reached its floor before its reset first did, and
// when the history ends.
type outcomes struct {
	reached map[week]time.Time
	end     time.Time
}

// outcomesOf are what became of the weeks, the windows named key, of the
// accounts configured, as read, the readings history in the order read, never
// empty, reads them: each reached its floor where it was first read there, as
// reachedFloor says.
func outcomesOf(configured config.Accounts, key string, read []readings.Reading) outcomes {
	reserves := make(map[string]float64, len(configured))
	for _, a := range configured {
		reserves[a.ID] = a.Reserve
	}
	o := outcomes{reached: make(map[week]time.Time), end: read[len(read)-1].At}
	for _, r := range read {
		reserve, configured := reserves[r.Account]
		if !configured || r.Key != key || !reachedFloor(r.Window(), reserve, r.At) {
			continue
		}
		k := weekOf(r.Account, r.ResetsAt)
		if _, seen := o.reached[k]; !seen {
			o.reached[k] = r.At
		}
	}
	return o
}

// reachedFloor reports whether w, read at at, had reached its floor before it
// reset, where its account's room ends: used up, or rejected, as score.Spent
// says, or at its reserve, as score.HeldByReserve says, where the account
// keeps one, which no global pin spends in a playback.
func reachedFloor(w quota.Window, reserve float64, at time.Time) bool {
	return !w.ResetsAt.IsZero() && at.Before(w.ResetsAt) && (score.Spent(w) || score.HeldByReserve(w, reserve))
}

// ended returns when w ended: when it reached its floor, else at its reset,
// reporting false where the history doesn't say, as it ends before w resets
// with w's floor not reached by then.
func (o outcomes) ended(w week) (time.Time, bool) {
	if at, ok := o.reached[w]; ok {
		return at, true
	}
	return w.resets, !w.resets.After(o.end)
}

// tally is how a set of forecasts scored: how many there were, how far off
// they were all told, either way, how many were early and how many late by
// more than missBy, and how many moved from the hour before's forecast of
// their week, and how far all told.
type tally struct {
	forecasts, early, late, moves int
	off, moved                    time.Duration
}

// add takes in a forecast off by off, later than what came where it's more
// than none.
func (t *tally) add(off time.Duration) {
	t.forecasts++
	t.off += off.Abs()
	switch {
	case off < -missBy:
		t.early++
	case off > missBy:
		t.late++
	}
}

// move takes in a forecast that moved by moved from the hour before's.
func (t *tally) move(moved time.Duration) {
	t.moves++
	t.moved += moved.Abs()
}

// scores are the tallies of a set of forecasts by how much of their week was
// used as they were made, as usedFrom buckets them, and of them all.
type scores struct {
	byUse [len(usedFrom)]tally
	all   tally
}

// scoresOf are how forecasts, made an hour apart in the order made, scored
// against what became of their weeks, as o has it: each of a week the
// history says the end of, off by how far its end was from that, and moved by
// how far it was from the forecast of its week an hour before, where that's
// among them.
func scoresOf(forecasts []forecast, o outcomes) scores {
	var s scores
	before := make(map[string]forecast)
	for _, f := range forecasts {
		ended, known := o.ended(f.week)
		if !known {
			continue
		}
		bucket := &s.byUse[bucketOf(f.used)]
		off := f.ends.Sub(ended)
		bucket.add(off)
		s.all.add(off)
		if last, ok := before[f.account]; ok && last.week == f.week && last.at.Add(time.Hour).Equal(f.at) {
			moved := f.ends.Sub(last.ends)
			bucket.move(moved)
			s.all.move(moved)
		}
		before[f.account] = f
	}
	return s
}

// bucketOf is the bucket of usedFrom a forecast made with used of its week
// used falls in.
func bucketOf(used float64) int {
	bucket := 0
	for i, from := range usedFrom {
		if used >= from {
			bucket = i
		}
	}
	return bucket
}
