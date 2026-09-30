package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// trail is how an account's pressure window, the one whose pace of use is
// watched, has been read over the last half hour, as it now runs, in the order
// the readings were taken in. It's kept in memory alone: a router started
// afresh goes by the window's use since it started until its readings span
// long enough again.
type trail struct {
	// key is the pressure window's, as the policy names it.
	key      string
	readings []score.Reading
}

// note takes in kept, the reading of a window the account now stands as,
// taken in at a time, held being the one it stood as before: of the pressure
// window alone, a reading that starts it afresh, as startsAfresh says, leaving
// none before it. Readings older than the half hour go.
func (t *trail) note(held, kept quota.Window, at time.Time) {
	if kept.Key != t.key {
		return
	}
	if startsAfresh(held, kept) {
		t.readings = nil
	}
	i := slices.IndexFunc(t.readings, func(r score.Reading) bool { return !r.At.Before(at.Add(-score.Recent)) })
	if i < 0 {
		i = len(t.readings)
	}
	t.readings = append(t.readings[i:], score.Reading{At: at, Utilization: kept.Utilization})
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
// being used at now, as score.PaceOf judges it over the trail. It reports false
// when that can't be said, as when the window hasn't been read, or isn't
// running.
func (u *usage) pace(policy score.Policy, now time.Time) (score.Pace, bool) {
	w, ok := u.windows[policy.Pressure]
	if !ok {
		return score.Pace{}, false
	}
	return score.PaceOf(w, u.trail.readings, now)
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
