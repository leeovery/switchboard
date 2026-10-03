package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// maxPoints is how many points GET /history gives of an account's window at
// most: the steps it takes over the window's whole length.
const maxPoints = 1000

// leastStep is the shortest step GET /history takes over a window of the
// given length: the one that covers its whole length in maxPoints steps.
func leastStep(length time.Duration) time.Duration {
	step := length / maxPoints
	if length%maxPoints != 0 {
		step++
	}
	return step
}

// historyAsk is what GET /history asks for: the window, by its key, and the
// step between points, as read and as asked.
type historyAsk struct {
	window string
	step   time.Duration
	asked  string
}

// windowHistory is what GET /history answers at now: every account's use of
// the window asked over its current length, a point each step asked, from the
// readings history and the readings held in memory since.
func (r *Router) windowHistory(asked historyAsk) History {
	now := r.cfg.Now()
	held := r.state.windowsHeld(asked.window, now)
	var placed []string
	from := now
	for _, a := range held {
		if start, ok := a.start(now); ok {
			placed = append(placed, a.id)
			from = score.Earlier(from, start)
		}
	}
	written := r.history.windowReadings(asked.window, placed, from, now)
	answer := History{Window: asked.window, Step: asked.asked, Accounts: make([]AccountHistory, len(held))}
	for i, a := range held {
		answer.Accounts[i] = a.history(written[a.id], asked.step, now)
	}
	return answer
}

// heldWindow is what the router holds of an account's window: the window as
// the status document reads it, zero when it has none, and the readings of it
// held in memory.
type heldWindow struct {
	id       string
	window   quota.Window
	readings []reading
}

// windowsHeld returns what the router holds of each account's window with the
// given key, in the order configured: the window as the status document reads
// it at now, and the readings of it in memory, which take in those still
// queued for the history.
func (s *state) windowsHeld(key string, now time.Time) []heldWindow {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := make([]heldWindow, len(s.accounts))
	for i, a := range s.accounts {
		u := s.usage[a.ID]
		w, _ := u.status(a, s.policy, now).Window(key)
		held[i] = heldWindow{id: a.ID, window: w, readings: u.readingsOf(key)}
	}
	return held
}

// readingsOf returns the readings held in memory of the window with the given
// key: its trail's levels, each as first read, and its latest, as of when a
// reading of the account last came in.
func (u *usage) readingsOf(key string) []reading {
	var readings []reading
	for _, level := range u.trails[key] {
		readings = append(readings, reading{At: level.At, Utilization: level.Utilization})
	}
	if w, ok := u.windows[key]; ok {
		readings = append(readings, reading{At: u.updated, Utilization: w.Utilization, ResetsAt: w.ResetsAt})
	}
	return readings
}

// start returns when the account's window, as it runs now, started: as
// quota.Window.Span says, its reset less its length, or when it started again
// since. It reports false when the window can't be placed as running now: it
// hasn't been read, has lapsed, was read without a reset, or has reset since.
func (a heldWindow) start(now time.Time) (time.Time, bool) {
	start, _, ok := a.window.Span()
	return start, ok && a.window.ResetsAt.After(now)
}

// history returns the account's use of its window over its current length at
// now, a point each step, from written, the readings of it the history holds,
// and those held in memory.
func (a heldWindow) history(written []reading, step time.Duration, now time.Time) AccountHistory {
	answer := AccountHistory{ID: a.id}
	start, ok := a.start(now)
	if !ok {
		return answer
	}
	answer.Start = start.UTC()
	answer.Points = pointsOf(sinceStart(slices.Concat(written, a.readings), start), start, step, now)
	return answer
}

// sinceStart returns those of readings that are of a window as it has run
// since start, in the order taken: taken at start or after, and of no earlier
// window, whose reset, where they give one, is at or before start.
func sinceStart(readings []reading, start time.Time) []reading {
	readings = slices.DeleteFunc(readings, func(r reading) bool {
		return r.At.Before(start) || !r.ResetsAt.IsZero() && !r.ResetsAt.After(start)
	})
	slices.SortStableFunc(readings, func(a, b reading) int { return a.At.Compare(b.At) })
	return readings
}

// pointsOf returns the use of a window that started at start, a point each
// step from then until now, each the use of the last of readings, in the
// order taken, read at or before it, leaving out those before the first.
func pointsOf(readings []reading, start time.Time, step time.Duration, now time.Time) []HistoryPoint {
	var points []HistoryPoint
	read := 0
	for at := start; !at.After(now); at = at.Add(step) {
		for read < len(readings) && !readings[read].At.After(at) {
			read++
		}
		if read > 0 {
			points = append(points, HistoryPoint{At: at.UTC(), Utilization: readings[read-1].Utilization})
		}
	}
	return points
}
