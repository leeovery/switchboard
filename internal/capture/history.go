package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// trail is an account's use of one of its windows as it runs now: when it
// started, and its use as read since, in order.
type trail struct {
	account, window string
	start           time.Time
	readings        []router.HistoryPoint
}

const (
	// sessionLength and weekLength are how long the session and the week
	// run.
	sessionLength = 5 * time.Hour
	weekLength    = 7 * 24 * time.Hour
	// sessionSteps are the steps a session's use rises in to its use now, as
	// the generator draws it, read again each sessionReading.
	sessionSteps   = 14
	sessionReading = time.Minute
	// weekReading is how often a week's use is read, as the generator draws
	// it.
	weekReading = 30 * time.Minute
)

// bursts weigh each of a session's steps, as the generator does, its seed
// picking which.
var bursts = []float64{3, 0, 1, 5, 2, 0, 0, 4, 6, 1, 2, 7, 3, 1, 0, 5}

// trailsOf are the samples' windows' use, from when each started until now,
// as the frames' charts draw them: each sample's session, while it runs, and
// its weeks.
func trailsOf(samples []sample, now time.Time) []trail {
	var trails []trail
	for _, s := range samples {
		if s.session.resets.After(now) {
			trails = append(trails, s.sessionTrail(now))
		}
		trails = append(trails, s.weekTrail(weekKey, s.week, now), s.weekTrail(fableKey, s.fable, now))
	}
	return trails
}

// sessionTrail is the sample's session's use from its start until now, a
// reading each sessionReading: rising in bursts to its use now, or, where it
// reached its limit, to its limit then, and level since.
func (s sample) sessionTrail(now time.Time) trail {
	start := s.session.resets.Add(-sessionLength)
	end := minuteOf(now)
	if !s.session.limited.IsZero() {
		end = s.session.limited
	}
	steps := []router.HistoryPoint{{At: start}}
	var weights []float64
	for i := range sessionSteps {
		weights = append(weights, bursts[(i*3+s.seed*5)%len(bursts)]+0.3)
	}
	for i, risen := range risings(weights) {
		steps = append(steps, router.HistoryPoint{At: start.Add(end.Sub(start) * time.Duration(i+1) / sessionSteps), Utilization: s.session.used * risen})
	}
	t := trail{account: s.id, window: fiveHourKey, start: start}
	for at := start; !at.After(minuteOf(now)); at = at.Add(sessionReading) {
		t.readings = append(t.readings, router.HistoryPoint{At: at, Utilization: along(steps, at)})
	}
	return t
}

// weekTrail is the sample's week w's use from its start until now, a reading
// each weekReading, rising as busily as rhythm says.
func (s sample) weekTrail(key string, w weekly, now time.Time) trail {
	t := trail{account: s.id, window: key, start: w.resets.Add(-weekLength)}
	var busy []float64
	for at := t.start; at.Before(minuteOf(now)); at = at.Add(weekReading) {
		busy = append(busy, rhythm(at, s.seed))
	}
	for i, risen := range risings(busy) {
		t.readings = append(t.readings, router.HistoryPoint{At: t.start.Add(time.Duration(i) * weekReading), Utilization: w.used * risen})
	}
	return t
}

// risings are how far use has risen after each of the steps weighed, as a
// share of its rise over them all: the last is 1, so the use read last is the
// use now, to the bit.
func risings(weights []float64) []float64 {
	var total float64
	for _, w := range weights {
		total += w
	}
	risen := make([]float64, len(weights))
	var sofar float64
	for i, w := range weights {
		sofar += w
		risen[i] = sofar / total
	}
	return risen
}

// history is the account's use of the window from its start until now, a
// point each step, as GET /history gives it: each the use last read at or
// before its time, but for points before the first reading.
func (t trail) history(step time.Duration, now time.Time) router.AccountHistory {
	h := router.AccountHistory{ID: t.account, Start: t.start}
	for at := t.start; !at.After(now); at = at.Add(step) {
		i, found := slices.BinarySearchFunc(t.readings, at, func(r router.HistoryPoint, at time.Time) int { return r.At.Compare(at) })
		if !found {
			i--
		}
		if i >= 0 {
			h.Points = append(h.Points, router.HistoryPoint{At: at, Utilization: t.readings[i].Utilization})
		}
	}
	return h
}

// rhythm is how busy an account with the given seed is at t, as the
// generator draws a week: busy through the working day, less so in the
// evening, hardly at all overnight, a third as busy at the weekend, and each
// day busier or quieter than the last.
func rhythm(t time.Time, seed int) float64 {
	day := daysInto(t)
	busy := 0.02
	switch hour := t.Hour(); {
	case 9 <= hour && hour < 19:
		busy = 1
	case 19 <= hour && hour < 23:
		busy = 0.3
	}
	if mod(day, 7) >= 5 {
		busy *= 0.35
	}
	return busy * (0.6 + 0.8*float64(mod(day*7+seed*3, 5))/4)
}

// daysInto counts the days from Monday 28 September, which starts the
// frames' week, to t's, negative before it.
func daysInto(t time.Time) int {
	date := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	monday := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	return int(date.Sub(monday) / (24 * time.Hour))
}

// along is the use steps reach at t, rising in a straight line from each
// step to the next, and level past the last.
func along(steps []router.HistoryPoint, t time.Time) float64 {
	for i := 1; i < len(steps); i++ {
		from, to := steps[i-1], steps[i]
		if !t.After(to.At) {
			share := float64(t.Sub(from.At)) / float64(to.At.Sub(from.At))
			return from.Utilization + (to.Utilization-from.Utilization)*share
		}
	}
	return steps[len(steps)-1].Utilization
}

// minuteOf is the minute now falls in, which the frames' clock reads.
func minuteOf(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
}

// mod is a modulo b, never negative, as the generator's is.
func mod(a, b int) int {
	return (a%b + b) % b
}
