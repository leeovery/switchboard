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

// rates returns how fast each of the account's windows has been used lately,
// at now, in quota.Sort's order, as status.RatesOf says.
func (u *usage) rates(now time.Time) []status.Rate {
	return status.RatesOf(u.latest(), u.trails, now)
}

// pressure is how the account's pressure window stands at now, as the status
// document gives it: how fast it's being used, and when, at that rate, it
// reaches where the account's room ends, at its reserve, or, when spent is
// set, as a pin spends the reserve, at its limit; and whether that's before
// it resets while the account can take a request of some model, as then
// pressure is what passes it over. It's zero when the pace can't be said.
func (u *usage) pressure(a account, policy score.Policy, spent bool, now time.Time) status.Pressure {
	pace, ok := u.pace(policy, now)
	if !ok {
		return status.Pressure{}
	}
	c := score.Candidate{ID: a.ID, Windows: u.current(policy, now), Reserve: a.Reserve, Rate: pace.Rate}
	if spent {
		c.Reserve = 0
	}
	return status.PressureOf(policy, pace, c, func(reserve float64) bool { return u.takesAny(policy, reserve, now) }, now)
}

// takesAny reports whether the account can take a request of some model at
// now: no limit, nor refusal of its token, holds back every request, and no
// window every model shares is spent, or at reserve, the share of it left
// unused.
func (u *usage) takesAny(policy score.Policy, reserve float64, now time.Time) bool {
	return !u.shut(now, policy.IsShared) && score.Available(u.current(policy, now), reserve, policy.IsShared, now)
}
