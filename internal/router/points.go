package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
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

// historyAsk is what GET /history asks for: the window, by its key, and its
// length; and the step between points, as read and as asked.
type historyAsk struct {
	window string
	length time.Duration
	step   time.Duration
	asked  string
}

// windowHistory is what GET /history answers at now: every account's use of
// the window asked over its length back from now, a point each step asked,
// from the readings history and the readings held in memory since.
func (r *Router) windowHistory(asked historyAsk) History {
	now := r.cfg.Now()
	from := now.Add(-asked.length)
	held := r.state.windowsHeld(asked.window, now)
	ids := make([]string, len(held))
	for i, a := range held {
		ids[i] = a.id
	}
	written := r.history.windowReadings(asked.window, ids, from, now)
	answer := History{Window: asked.window, Step: asked.asked, Accounts: make([]AccountHistory, len(held))}
	for i, a := range held {
		answer.Accounts[i] = a.history(written[a.id], asked.step, from, now)
	}
	return answer
}

// heldWindow is what the router holds of an account's window: the window as
// the status document reads it, zero when it has none, and the readings of it
// held in memory.
type heldWindow struct {
	id       string
	window   quota.Window
	readings []readings.Reading
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
func (u *usage) readingsOf(key string) []readings.Reading {
	var held []readings.Reading
	for _, level := range u.trails[key] {
		held = append(held, readings.Reading{At: level.At, Key: key, Utilization: level.Utilization})
	}
	if w, ok := u.windows[key]; ok {
		held = append(held, readings.Reading{At: u.updated, Key: key, Utilization: w.Utilization, ResetsAt: w.ResetsAt})
	}
	return held
}

// history returns the account's use of its window from from until now, a
// point each step, as HistoryOf has it, from written, the readings of it the
// history holds, and those held in memory.
func (a heldWindow) history(written []readings.Reading, step time.Duration, from, now time.Time) AccountHistory {
	return HistoryOf(a.id, a.window, slices.Concat(written, a.readings), step, from, now)
}

// HistoryOf is the use of the window w of the account with the given id, as
// the status document reads it at now, from from until now, a point each
// step, from read, the readings of it, in any order, as GET /history gives
// it: where w can be placed as running now, as startOf says, from its start,
// as it has run since, and a step apart back from then, as the windows
// before it ran; where it can't, all of it as they ran.
func HistoryOf(id string, w quota.Window, read []readings.Reading, step time.Duration, from, now time.Time) AccountHistory {
	answer := AccountHistory{ID: id}
	read = slices.Clone(read)
	slices.SortStableFunc(read, byTaken)
	start, ok := startOf(w, now)
	if !ok {
		answer.Points = pointsOf(read, from, step, now)
		return answer
	}
	answer.Start = start.UTC()
	steps := start.Sub(from) / step
	answer.Points = slices.Concat(
		pointsOf(read, start.Add(-steps*step), step, start.Add(-step)),
		pointsOf(sinceStart(read, start), start, step, now),
	)
	return answer
}

// startOf returns when the window w, as it runs at now, started: as
// quota.Window.Span says, its reset less its length, or when it started again
// since. It reports false when the window can't be placed as running now: it
// hasn't been read, has lapsed, was read without a reset, or has reset since.
func startOf(w quota.Window, now time.Time) (time.Time, bool) {
	start, _, ok := w.Span()
	return start, ok && w.ResetsAt.After(now)
}

// sinceStart returns those of read, in the order taken, that are of a window
// as it has run since start: taken at start or after, and of no earlier
// window, whose reset, where they give one, is at or before start.
func sinceStart(read []readings.Reading, start time.Time) []readings.Reading {
	return slices.DeleteFunc(slices.Clone(read), func(r readings.Reading) bool {
		return r.At.Before(start) || r.Window().ResetBy(start)
	})
}

// byTaken orders readings by when they were taken.
func byTaken(a, b readings.Reading) int {
	return a.At.Compare(b.At)
}

// pointsOf returns the use of a window from from, a point each step until
// until, each the use of the last of taken, in the order taken, read at or
// before it, as score.Current has it then, so none once its reset has
// passed; leaving out those before the first.
func pointsOf(taken []readings.Reading, from time.Time, step time.Duration, until time.Time) []HistoryPoint {
	var points []HistoryPoint
	read := 0
	for at := from; !at.After(until); at = at.Add(step) {
		for read < len(taken) && !taken[read].At.After(at) {
			read++
		}
		if read > 0 {
			points = append(points, HistoryPoint{At: at.UTC(), Utilization: score.Current(taken[read-1].Window(), at).Utilization})
		}
	}
	return points
}
