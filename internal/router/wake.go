package router

import "time"

// minSleep is the least the wall clock must run ahead of the monotonic clock
// between two looks for the router to take it that the Mac slept: less is
// the clock being set.
const minSleep = 5 * time.Second

// wakes notices the Mac waking from sleep, looking every so often: the wall
// clock runs on while it sleeps, and the monotonic clock stops.
type wakes struct {
	// now reads both clocks, as time.Now does.
	now func() time.Time
	// woke hears of each wake, as it's noticed.
	woke func()
	// last is what now read at the last look: zero before the first.
	last time.Time
}

// look reads the clocks, and has woke hear of a sleep since the last look.
func (w *wakes) look() {
	now := w.now()
	if !w.last.IsZero() {
		if slept := now.Round(0).Sub(w.last.Round(0)) - now.Sub(w.last); slept >= minSleep {
			logger.Info("woke from sleep", "slept", slept.Round(time.Second))
			w.woke()
		}
	}
	w.last = now
}
