package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// trails are how each of an account's windows has been read over the last
// half hour, as it now runs, by the window's key, each in the order its
// readings were taken in: the pressure window's pace, and each window's
// recent rate, are measured over them. A router started afresh takes them up
// from the readings history, as seed says.
type trails map[string][]score.Reading

// note takes in kept, the reading of a window the account now stands as,
// taken in at a time, held being the one it stood as before: a reading that
// starts the window afresh, as startsAfresh says, leaves none of its readings
// before it. Readings older than the half hour go.
func (t trails) note(held, kept quota.Window, at time.Time) {
	readings := t[kept.Key]
	if startsAfresh(held, kept) {
		readings = nil
	}
	i := slices.IndexFunc(readings, func(r score.Reading) bool { return !r.At.Before(at.Add(-score.Recent)) })
	if i < 0 {
		i = len(readings)
	}
	t[kept.Key] = append(readings[i:], score.Reading{At: at, Utilization: kept.Utilization})
}

// seed takes up, as the router starts, readings of the last half hour, as the
// readings history holds them, in the order they were read, into the trails
// of the accounts configured, as though they'd come in as read then, so each
// window's recent rate outlasts a restart. A window whose reading, as the
// state file kept it, has another reset than its readings last read, as when
// it has reset since, keeps none of them. It returns how many the trails
// keep.
func (s *state) seed(readings []reading) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	type trailOf struct{ account, window string }
	last := make(map[trailOf]quota.Window)
	for _, r := range readings {
		if u, configured := s.usage[r.Account]; configured {
			of, w := trailOf{r.Account, r.Window}, r.window()
			u.trails.note(last[of], w, r.At)
			last[of] = w
		}
	}
	kept := 0
	for of, w := range last {
		u := s.usage[of.account]
		if !u.windows[of.window].ResetsAt.Equal(w.ResetsAt) {
			delete(u.trails, of.window)
		}
		kept += len(u.trails[of.window])
	}
	return kept
}

// startsAfresh reports whether kept, the reading a window now stands as, starts
// it afresh from held, as it stood before: kept has another reset, a new
// window, or shows it reset by hand, as resetByHand says.
func startsAfresh(held, kept quota.Window) bool {
	return !kept.ResetsAt.Equal(held.ResetsAt) || resetByHand(held, kept)
}

// pace returns how fast the account's pressure window, as policy names it, is
// being used at now, as score.PaceOf judges it over its trail. It reports
// false when that can't be said, as when the window hasn't been read, or isn't
// running.
func (u *usage) pace(policy score.Policy, now time.Time) (score.Pace, bool) {
	w, ok := u.windows[policy.Pressure]
	if !ok {
		return score.Pace{}, false
	}
	return score.PaceOf(w, u.trails[w.Key], now)
}

// rates returns how fast each of the account's windows has been used lately,
// at now, as score.RecentRate judges it over its trail, in quota.Sort's order:
// those that have a recent rate alone.
func (u *usage) rates(now time.Time) []status.Rate {
	var rates []status.Rate
	for _, w := range u.latest() {
		if rate, ok := score.RecentRate(w, u.trails[w.Key], now); ok {
			rates = append(rates, status.Rate{Window: w.Key, Rate: rate})
		}
	}
	return rates
}

// pressure is how the account's pressure window stands at now, as the status
// document gives it: how fast it's being used, and when, at that rate, it
// reaches where the account's room ends, at its reserve, or, when spent is
// set, as a pin spends the reserve, at its limit. It's zero when the pace
// can't be said.
func (u *usage) pressure(a account, policy score.Policy, spent bool, now time.Time) status.Pressure {
	pace, ok := u.pace(policy, now)
	if !ok {
		return status.Pressure{}
	}
	c := score.Candidate{ID: a.ID, Windows: u.current(policy, now), Reserve: a.Reserve, Rate: pace.Rate}
	if spent {
		c.Reserve = 0
	}
	p := policy.PressureOf(c, now)
	return status.Pressure{Window: policy.Pressure, Rate: pace.Rate, Recent: pace.Recent, RunsOut: p.RunsOut, Under: p.Under}
}
