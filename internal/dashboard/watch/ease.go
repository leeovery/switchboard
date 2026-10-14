package watch

import (
	"maps"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// easeFor is how long a bar takes to move to a new reading.
	easeFor = time.Second
	// frameEvery paces the frames while bars move: about 30 a second.
	frameEvery = time.Second / 30
)

// easing moves each bar from where it stood when a document arrived to the
// bar its view draws of that document, as Filled has them, view by view, as
// each draws its own: by its window's account and key, the pool's by its key
// alone. A bar it has no start for rises from nothing.
type easing struct {
	from  map[dashboard.View]map[dashboard.Ref]float64
	start time.Time
}

// done reports whether the easing is over by now.
func (e easing) done(now time.Time) bool {
	return now.Sub(e.start) >= easeFor
}

// eased are how much of each bar of f's view the easing moves fills at now,
// by its window's account and key, the pool's by its key alone, on its way
// from where it stood to filling as the view draws it of doc, as Filled has
// it of f, of the moment f holds, as the frame drawn reads it: none of a bar
// that stands where it's drawn, and none once the easing's over.
func (e easing) eased(f dashboard.Frame, doc status.Document, now time.Time) map[dashboard.Ref]float64 {
	if e.done(now) {
		return nil
	}
	return e.towards(f.View, f.Filled(doc, now), now)
}

// towards are how much of each bar of the view given, filled as filled has
// it, the easing has it fill at now: none of a bar that stands where it's
// filled.
func (e easing) towards(view dashboard.View, filled map[dashboard.Ref]float64, now time.Time) map[dashboard.Ref]float64 {
	t := easeOutCubic(progress(now.Sub(e.start), easeFor))
	eased := make(map[dashboard.Ref]float64)
	for ref, to := range filled {
		if from := e.from[view][ref]; from != to {
			eased[ref] = lerp(from, to, t)
		}
	}
	return eased
}

// standing are how much of each bar of each view that draws any of doc fills
// at now, view by view, by its window's account and key, the pool's by its
// key alone: as far as it has eased, where it's easing, else as the view
// draws it, as Filled has it of f, each view of the one moment f holds.
func (e easing) standing(f dashboard.Frame, doc status.Document, now time.Time) map[dashboard.View]map[dashboard.Ref]float64 {
	stand := make(map[dashboard.View]map[dashboard.Ref]float64)
	for _, view := range dashboard.Views() {
		f.View = view
		filled := f.Filled(doc, now)
		if filled == nil {
			continue
		}
		if !e.done(now) {
			maps.Copy(filled, e.towards(view, filled, now))
		}
		stand[view] = filled
	}
	return stand
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
