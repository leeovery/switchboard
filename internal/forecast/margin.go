package forecast

import (
	"slices"
	"time"
)

const (
	// MarginStep is how far apart, in whole percents of the pool's pace, the
	// paces a Margin is played at are: the finest it tells.
	MarginStep = 5
	// MarginReach is how far, in whole percents of the pool's pace, a Margin
	// plays the pace up or down from it: past it, the pool lasts by more
	// than it tells.
	MarginReach = 50
	// pace is the pool's own pace, in whole percents of it.
	pace = 100
)

// MarginKind says what a Margin tells of the pool's week ahead.
type MarginKind int

// The kinds of Margin.
const (
	// MarginNone tells nothing: the pool has no room at its Now, fewer than
	// two accounts, or nothing to play.
	MarginNone MarginKind = iota
	// MarginLasts is a pool that lasts the week at its pace.
	MarginLasts
	// MarginRunsOut is a pool that runs out within the week at its pace.
	MarginRunsOut
)

// Margin is how near the pool is to running out within the week ahead, its
// use played at its own pace and at paces MarginStep apart, up or down by
// MarginReach at most, as MarginOf plays them. Its zero value tells nothing.
type Margin struct {
	Kind MarginKind
	// Spare is, of a pool that lasts, how much busier, in whole percents of
	// its pace, its use could get and the pool still last: 0 where it runs
	// out a MarginStep busier, MarginReach where it lasts MarginReach busier
	// but runs out a MarginStep more, and past MarginReach, at
	// MarginReach+MarginStep, where it lasts even that busier.
	Spare int
	// RunsOut is, of a pool that runs out, when; and Less how much less of
	// its use, in whole percents of its pace, would last the week: 0 where
	// MarginReach less wouldn't.
	RunsOut time.Time
	Less    int
}

// MarginOf is how near the pool is to running out within the week ahead, as
// Forecast plays it: where it lasts at its pace, how much busier it could get
// and last, played MarginStep busier at a time until it runs out, or lasts
// MarginReach+MarginStep busier; where it runs out, when, and how much less
// of its use would last, played MarginStep less at a time until it lasts, or
// runs out MarginReach less. The pool runs out where some of its use has no
// account that takes requests to go to, as Policy.Choose finds none with
// room. A pace is every account's Fast and the pool's Day, scaled alike. It
// tells nothing of a pool with fewer than two accounts, or none with room at
// its Now.
func MarginOf(p Pool) Margin {
	if len(p.Accounts) < 2 || !newPlay(p).roomAt(p.Now) {
		return Margin{}
	}
	out := runsOutAt(p, pace)
	if out.IsZero() {
		spare := 0
		for busier := pace + MarginStep; busier <= pace+MarginReach+MarginStep && runsOutAt(p, busier).IsZero(); busier += MarginStep {
			spare = busier - pace
		}
		return Margin{Kind: MarginLasts, Spare: spare}
	}
	m := Margin{Kind: MarginRunsOut, RunsOut: out}
	for less := pace - MarginStep; less >= pace-MarginReach; less -= MarginStep {
		if runsOutAt(p, less).IsZero() {
			m.Less = pace - less
			break
		}
	}
	return m
}

// runsOutAt is when the pool runs out within the week ahead, its pace scaled
// to percent of it: zero where it lasts.
func runsOutAt(p Pool, percent int) time.Time {
	return newPlay(scaled(p, percent)).runOut()
}

// scaled is the pool at percent of its pace: every account's Fast and its
// Day scaled, the rest as it is.
func scaled(p Pool, percent int) Pool {
	by := float64(percent) / pace
	p.Accounts = slices.Clone(p.Accounts)
	for i := range p.Accounts {
		p.Accounts[i].Fast *= by
	}
	p.Day *= by
	return p
}

// roomAt reports whether a new session has an account to go to at t, as
// place looks for one.
func (p *play) roomAt(t time.Time) bool {
	p.gather(t)
	_, ok := p.Policy.Choose(p.candidates, p.Pinned, p.applies, "", t)
	return ok
}

// runOut plays the pool's use forward, a step at a time, for a week, until
// the pool runs out, and returns when it does: zero where it lasts.
func (p *play) runOut() time.Time {
	for until := p.Now.Add(playSpan); p.next.Before(until) && p.ranOut.IsZero(); p.next = p.next.Add(playStep) {
		p.step(p.next)
	}
	return p.ranOut
}
