package router

import (
	"iter"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// trails are how each of an account's windows has been read lately, as it
// now runs, by the window's key: the levels its use was read at, each with
// when it was first and last read so, its baseline, the level read last
// before the half hour, and each since, in the order read. The pressure
// window's pace, and each window's recent rate, are measured over them. A
// router started afresh takes them up from the readings history, as seed
// says.
type trails map[string][]score.Reading

// note takes in kept, the reading of a window the account now stands as,
// taken in at a time, held being the one it stood as before, into the
// window's trail, as score.Noted says.
func (t trails) note(held, kept quota.Window, at time.Time) {
	t[kept.Key] = score.Noted(t[kept.Key], held, kept, at)
}

// seed takes up, as the router starts, the readings history gives, in the
// order they were read, into the trails of the accounts configured, as though
// they'd come in as read then: the readings history holds each change of a
// window's use, so its trail, baseline included, is as it was, and its recent
// rate outlasts a restart. A window whose reading, as the state file kept it,
// has another reset than its readings last read, as when it has reset since,
// keeps none of them. It returns how many the trails keep.
func (s *state) seed(history iter.Seq[readings.Reading]) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	type trailOf struct{ account, window string }
	last := make(map[trailOf]quota.Window)
	for r := range history {
		if u, configured := s.usage[r.Account]; configured {
			of, w := trailOf{r.Account, r.Key}, r.Window()
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

// recentRate returns how fast the account's window named key has been used
// lately, at now, as score.RecentRate judges it over its trail. It reports
// false when that can't be said, as when the window hasn't been read, or not
// for long enough.
func (u *usage) recentRate(key string, now time.Time) (float64, bool) {
	w, ok := u.windows[key]
	if !ok {
		return 0, false
	}
	rate, _, ok := score.RecentRate(w, u.trails[key], now)
	return rate, ok
}

// rates returns how fast each of the account's windows has been used lately,
// at now, in quota.Sort's order, as status.RatesOf says.
func (u *usage) rates(now time.Time) []status.Rate {
	return status.RatesOf(u.latest(), u.trails, now)
}

// pressure is how the account's pressure window stands at now, as the status
// document gives it: how fast it's being used, and when, at that rate, it
// reaches where the account's room ends, at its reserve, or, when spent is
// set, as a pin spends the reserve, at its limit; and whether that's before
// it resets while the account can take a request of some model, no limit,
// nor refusal of its token, holding back every request, as then pressure is
// what passes it over, as status.PressureFrom says. It's zero when the pace
// can't be said.
func (u *usage) pressure(a account, policy score.Policy, spent bool, now time.Time) status.Pressure {
	return status.PressureFrom(policy, status.Pressing{
		ID: a.ID, Reserve: a.Reserve, Latest: u.latest(), Trails: u.trails, Spent: spent, Shut: u.shut(now, policy.IsShared),
	}, now)
}
