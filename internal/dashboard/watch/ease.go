package watch

import (
	"maps"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// easeFor is how long a bar takes to move to a new reading.
	easeFor = time.Second
	// frameEvery paces the frames while bars move: about 30 a second.
	frameEvery = time.Second / 30
)

// easing moves each bar from where it stood when a document arrived to that
// document's reading, by its account and window's key. A bar it has no start
// for rises from nothing.
type easing struct {
	from  map[dashboard.Ref]float64
	start time.Time
}

// done reports whether the easing is over by now.
func (e easing) done(now time.Time) bool {
	return now.Sub(e.start) >= easeFor
}

// eased are how much of each window whose bar the easing moves to doc's
// reading the bar fills at now, by its account and key: none of a bar that
// stands where it's read, and none once the easing's over.
func (e easing) eased(doc status.Document, now time.Time, policy score.Policy) map[dashboard.Ref]float64 {
	if e.done(now) {
		return nil
	}
	t := easeOutCubic(progress(now.Sub(e.start), easeFor))
	eased := make(map[dashboard.Ref]float64)
	for ref, to := range utilizations(doc, now, policy) {
		if from := e.from[ref]; from != to {
			eased[ref] = lerp(from, to, t)
		}
	}
	return eased
}

// standing are how much of each of doc's windows its bar fills at now, by
// its account and key: as far as it has eased, where it's easing, else as
// read.
func (e easing) standing(doc status.Document, now time.Time, policy score.Policy) map[dashboard.Ref]float64 {
	stand := utilizations(doc, now, policy)
	maps.Copy(stand, e.eased(doc, now, policy))
	return stand
}

// utilizations are how much of each of doc's windows is used, by its
// account and key, and of each of its pool's at now, as policy judges it, by
// its key alone, as the pool's bars fill to.
func utilizations(doc status.Document, now time.Time, policy score.Policy) map[dashboard.Ref]float64 {
	used := make(map[dashboard.Ref]float64)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			used[dashboard.Ref{Account: a.ID, Window: w.Key}] = w.Utilization
		}
	}
	for _, p := range doc.PoolAt(now, policy).Windows {
		used[dashboard.Ref{Window: p.Key}] = p.Used
	}
	return used
}

// easeOutCubic eases t, from 0 to 1: quick at first, slowing to a stop.
func easeOutCubic(t float64) float64 {
	left := 1 - t
	return 1 - left*left*left
}

// easeInCubic eases t, from 0 to 1: slow at first, quickening to the end.
func easeInCubic(t float64) float64 {
	return t * t * t
}

// progress is how far elapsed has come through length, from 0 to 1.
func progress(elapsed, length time.Duration) float64 {
	return min(max(float64(elapsed)/float64(length), 0), 1)
}

// lerp is the value t of the way from a to b: a itself at 0, b itself at 1.
func lerp(a, b, t float64) float64 {
	return a*(1-t) + b*t
}
