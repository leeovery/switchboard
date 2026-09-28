package watch

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// easeFor is how long a bar takes to move to a new reading.
	easeFor = 600 * time.Millisecond
	// frameEvery paces the frames while bars move: about 30 a second.
	frameEvery = time.Second / 30
)

// windowRef names a window from one document to the next.
type windowRef struct {
	account, key string
}

// easing moves each bar from where it stood when a document arrived to that
// document's reading. A bar it has no start for rises from nothing.
type easing struct {
	from  map[windowRef]float64
	start time.Time
}

// done reports whether the easing is over by now.
func (e easing) done(now time.Time) bool {
	return now.Sub(e.start) >= easeFor
}

// moves reports whether the easing moves any of doc's bars.
func (e easing) moves(doc status.Document) bool {
	for ref, to := range utilizations(doc) {
		if e.from[ref] != to {
			return true
		}
	}
	return false
}

// apply returns doc as it's drawn at now: each window's utilization eased
// along from its start. It copies what it changes, leaving doc as it was.
func (e easing) apply(doc status.Document, now time.Time) status.Document {
	if e.done(now) {
		return doc
	}
	t := easeOutCubic(progress(now.Sub(e.start), easeFor))
	accounts := slices.Clone(doc.Accounts)
	for i, a := range accounts {
		windows := slices.Clone(a.Windows)
		for j, w := range windows {
			windows[j].Utilization = lerp(e.from[windowRef{a.ID, w.Key}], w.Utilization, t)
		}
		accounts[i].Windows = windows
	}
	doc.Accounts = accounts
	return doc
}

// utilizations are how much of each of doc's windows is used.
func utilizations(doc status.Document) map[windowRef]float64 {
	used := make(map[windowRef]float64)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			used[windowRef{a.ID, w.Key}] = w.Utilization
		}
	}
	return used
}

// easeOutCubic eases t, from 0 to 1: quick at first, slowing to a stop.
func easeOutCubic(t float64) float64 {
	left := 1 - t
	return 1 - left*left*left
}

// progress is how far elapsed has come through length, from 0 to 1.
func progress(elapsed, length time.Duration) float64 {
	return min(max(float64(elapsed)/float64(length), 0), 1)
}

// lerp is the value t of the way from a to b: a itself at 0, b itself at 1.
func lerp(a, b, t float64) float64 {
	return a*(1-t) + b*t
}
