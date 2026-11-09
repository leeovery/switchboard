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
// read them: each week's runs, the first from its start, and another from
// each time it was reset by hand, in order; and when the history ends.
type outcomes struct {
	runs map[week][]weekRun
	end  time.Time
}

// weekRun is a run of a week's use: from when it began, zero for the week's
// start, and when it reached its floor, zero where it didn't before the next
// run began.
type weekRun struct {
	from, reached time.Time
}

// outcomesOf are what became of the weeks, the windows named key, of the
// accounts configured, as read, the readings history in the order read, never
// empty, reads them: a week begins a run afresh where a reading shows it
// reset by hand since the one before, as score.ResetByHand says, and a run
// reached its floor where it was first read there, as reachedFloor says.
func outcomesOf(configured config.Accounts, key string, read []readings.Reading) outcomes {
	reserves := make(map[string]float64, len(configured))
	for _, a := range configured {
		reserves[a.ID] = a.Reserve
	}
	o := outcomes{runs: make(map[week][]weekRun), end: read[len(read)-1].At}
	last := make(map[string]quota.Window, len(configured))
	for _, r := range read {
		reserve, configured := reserves[r.Account]
		if !configured || r.Key != key {
			continue
		}
		w, k := r.Window(), weekOf(r.Account, r.ResetsAt)
		if held, ok := last[r.Account]; ok && score.ResetByHand(held, w) {
			o.runs[k] = append(o.runsOf(k), weekRun{from: r.At})
		}
		last[r.Account] = w
		if reachedFloor(w, reserve, r.At) {
			o.reach(k, r.At)
		}
	}
	return o
}

// runsOf are the runs of week k, as o has them: one, from its start, where it
// has none of its own yet.
func (o outcomes) runsOf(k week) []weekRun {
	if runs, ok := o.runs[k]; ok {
		return runs
	}
	return []weekRun{{}}
}

// reach notes that week k was read at its floor at at: where its last run is
// yet to reach it, that's when it did.
func (o outcomes) reach(k week, at time.Time) {
	runs := o.runsOf(k)
	if last := &runs[len(runs)-1]; last.reached.IsZero() {
		last.reached = at
	}
	o.runs[k] = runs
}

// reachedFloor reports whether w, read at at, had reached its floor before it
// reset, where its account's room ends: used up, or rejected, as score.Spent
// says, or at its reserve, as score.HeldByReserve says, where the account
// keeps one, which no global pin spends in a playback.
func reachedFloor(w quota.Window, reserve float64, at time.Time) bool {
	return !w.ResetsAt.IsZero() && at.Before(w.ResetsAt) && (score.Spent(w) || score.HeldByReserve(w, reserve))
}

// outcome is what became of the run of its week a forecast was made in: when
// the run began, and when it ended, where known says the history tells.
type outcome struct {
	began, ended time.Time
	known        bool
}

// of is what became of the run of its week f was made in, the last to begin
// by then: it ended when it reached its floor, else at the week's reset. The
// history doesn't tell where the week was reset by hand before the run
// reached its floor, which cut it short, or where it ends before the week
// resets, the run's floor not reached by then.
func (o outcomes) of(f forecast) outcome {
	runs := o.runsOf(f.week)
	i := len(runs) - 1
	for runs[i].from.After(f.at) {
		i--
	}
	r := runs[i]
	switch {
	case !r.reached.IsZero():
		return outcome{began: r.from, ended: r.reached, known: true}
	case i+1 < len(runs):
		return outcome{began: r.from}
	}
	return outcome{began: r.from, ended: f.resets, known: !f.resets.After(o.end)}
}

// tally is how a set of forecasts scored: how many there were, how far off
// they were all told, either way, and how many were early and how many late
// by more than missBy; and how many pairs of them were compared, each of a
// forecast and the hour before's of its week, moved or not, and how far they
// moved all told.
type tally struct {
	forecasts, early, late, pairs int
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

// pair takes in a pair of forecasts compared, the later having moved by
// moved from the earlier, none where it didn't move.
func (t *tally) pair(moved time.Duration) {
	t.pairs++
	t.moved += moved.Abs()
}

// scores are the tallies of a set of forecasts by how much of their week was
// used as they were made, as usedFrom buckets them, and of them all.
type scores struct {
	byUse [len(usedFrom)]tally
	all   tally
}

// scored is a forecast scored, with when the run of its week it was made in
// began.
type scored struct {
	forecast
	began time.Time
}

// scoresOf are how forecasts, made an hour apart in the order made, scored
// against what became of the runs of their weeks they were made in, as o has
// it: each made before its run's end, which the history tells, off by how far
// its end was from that, and compared with the forecast of its run an hour
// before, where that's among those scored, by how far it moved from it.
func scoresOf(forecasts []forecast, o outcomes) scores {
	var s scores
	before := make(map[string]scored)
	for _, f := range forecasts {
		out := o.of(f)
		if !out.known || !f.at.Before(out.ended) {
			continue
		}
		bucket := &s.byUse[bucketOf(f.used)]
		off := f.ends.Sub(out.ended)
		bucket.add(off)
		s.all.add(off)
		if last, ok := before[f.account]; ok && last.week == f.week && last.began.Equal(out.began) && last.at.Add(time.Hour).Equal(f.at) {
			moved := f.ends.Sub(last.ends)
			bucket.pair(moved)
			s.all.pair(moved)
		}
		before[f.account] = scored{forecast: f, began: out.began}
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
