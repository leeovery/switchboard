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
// recent rate, are measured over them. They're kept in memory alone: a router
// started afresh goes by each window's use since it started until its
// readings span long enough again.
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

// startsAfresh reports whether kept, the reading a window now stands as, starts
// it afresh from held, as it stood before: kept has another reset, a new
// window, or the same reset but less used, which only a window started again
// reads, as a reset made by hand leaves it, which drops use but may keep the
// reset.
func startsAfresh(held, kept quota.Window) bool {
	return !kept.ResetsAt.Equal(held.ResetsAt) || kept.Utilization < held.Utilization
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
