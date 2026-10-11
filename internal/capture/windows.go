package capture

import (
	"time"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
)

// The keys of the windows Claude reports, as the worlds' accounts have them.
const (
	sessionKey = fiveHourKey
	accountKey = weekKey
	modelKey   = fableKey
)

// session is a 5-hour window of an account's, as a day's use ran it: when it
// started, by a prime or a probe where it's primed, else by a request; the
// use it rose to, evenly, over the hours its use ran, ending then; and, where
// it reached its limit, when its use reached its cap, where the account has
// one, and its limit, rising to them from its start, after which it reads
// spent until its reset.
type session struct {
	start   time.Time
	primed  bool
	use     float64
	ran     time.Duration
	capped  time.Time
	limited time.Time
	// step is the use read between readings: a hundredth, as the router
	// reads a busy window, or coarser for days long past, whose readings
	// only their summaries draw from.
	step float64
}

// resets is when the window resets.
func (s session) resets() time.Time {
	return s.start.Add(sessionLength)
}

// read takes in the window's readings as the router reads them.
func (s session) read(r *readingsOf, account string) {
	w := window{account: account, key: sessionKey, resets: s.resets()}
	source := readings.FromAnswer
	if s.primed {
		source = readings.FromPrime
	}
	r.from(w, s.start, 0, source)
	step := s.step
	if step == 0 {
		step = 0.01
	}
	switch {
	case !s.limited.IsZero() && !s.capped.IsZero():
		r.rise(w, s.start, s.capped, 0, 0.95, step)
		r.at(w, s.limited, 1)
	case !s.limited.IsZero():
		r.rise(w, s.start, s.limited, 0, 1, step)
	default:
		r.rise(w, s.start, s.start.Add(s.ran), 0, s.use, step)
	}
}

// fableShares are how far each account's Fable week rises for each point of
// its week, as this week's do: none before March 2026, when the year's first
// Fable requests came.
var fableShares = map[string]float64{
	"work": .35, "side": .05, "client": .17, "lab": .36, "team": .36, "ops": .07, "studio": .24,
}

// fableFrom is the first day of the year of Fable requests: 1 March 2026.
const fableFrom = 153

// readings are the accounts' readings over the year to today, as the router
// took them in: each of their own weeks read as it starts, by a probe, then
// rising day by day, from 09:00 to 19:00, as far as the day's use rose it,
// Fable's week with it at the end of each day; and each day's 5-hour windows.
func (y year) readings() *readingsOf {
	r := &readingsOf{}
	for _, a := range tenAccounts {
		for _, w := range y.weeksOf(a.ID) {
			y.readWeek(r, a.ID, w)
		}
		for d := firstDay(a.ID); d < today; d++ {
			for _, s := range y.days[a.ID][d].windows {
				s.read(r, a.ID)
			}
		}
	}
	return r
}

// readWeek takes in the account with the given id's own week w as it was
// read up to today.
func (y year) readWeek(r *readingsOf, id string, w ownWeek) {
	week, fable := window{account: id, key: accountKey, resets: w.end}, window{account: id, key: modelKey, resets: w.end}
	if opened := w.start.Add(5 * time.Minute); !opened.Before(dayStart(y.now, firstDay(id))) {
		r.from(week, opened, 0, readings.FromProbe)
		r.from(fable, opened, 0, readings.FromProbe)
	}
	used, fabled := 0.0, 0.0
	for _, d := range w.days {
		if d < firstDay(id) || d >= today {
			continue
		}
		rise := y.days[id][d].rise
		from, to := score.Later(dayAt(y.now, d, 9, 0, 0), w.start.Add(10*time.Minute)), score.Earlier(dayAt(y.now, d, 19, 0, 0), w.end.Add(-time.Minute))
		r.rise(week, from, to, hundredths(used), hundredths(used+rise), 0.01)
		used += rise
		if d >= fableFrom {
			if next := fabled + rise*fableShares[id]; hundredths(next) > hundredths(fabled) {
				r.at(fable, to, hundredths(next))
			}
			fabled += rise * fableShares[id]
		}
	}
}

// soFar is how far the account with the given id's week still running, and
// its Fable week, had risen by today's start, and those windows.
func (y year) soFar(id string) (week, fable window, used, fabled float64) {
	weeks := y.weeksOf(id)
	w := weeks[len(weeks)-1]
	for _, d := range w.days {
		if d < today {
			used += y.days[id][d].rise
			if d >= fableFrom {
				fabled += y.days[id][d].rise * fableShares[id]
			}
		}
	}
	return window{account: id, key: accountKey, resets: w.end}, window{account: id, key: modelKey, resets: w.end}, used, fabled
}
