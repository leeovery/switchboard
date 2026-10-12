package status

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// Pressing is what an account's pressure is worked out from, as the router
// holds it: its id and reserve, as configured; its windows as last read,
// Latest, and how each has been read lately, Trails, by key, as score.Noted
// keeps them; Spent, set where a pin spends its reserve; and Shut, set where
// it can take no request of any model, whatever its windows read.
type Pressing struct {
	ID      string
	Reserve float64
	Latest  []quota.Window
	Trails  map[string][]score.Reading
	Spent   bool
	Shut    bool
}

// PressureFrom is how the account p's pressure window, as policy names it,
// stands at now, as the router gives it and as PressureOf says: at the pace
// score.PaceOf judges over its trail, its windows as they stand at now, its
// room ending at its reserve or, Spent, at its limit; the account taking a
// request of some model while it isn't Shut and no window every model shares
// is spent, or at that reserve. It's zero where the pace can't be said, as
// where the window hasn't been read, or isn't running.
func PressureFrom(policy score.Policy, p Pressing, now time.Time) Pressure {
	i := slices.IndexFunc(p.Latest, func(w quota.Window) bool { return w.Key == policy.Pressure })
	if i < 0 {
		return Pressure{}
	}
	pace, ok := score.PaceOf(p.Latest[i], p.Trails[policy.Pressure], now)
	if !ok {
		return Pressure{}
	}
	current := policy.AsOf(p.Latest, now)
	c := score.Candidate{ID: p.ID, Windows: current, Reserve: p.Reserve, Rate: pace.Rate}
	if p.Spent {
		c.Reserve = 0
	}
	takes := func(reserve float64) bool {
		return !p.Shut && score.Available(current, reserve, policy.IsShared, now)
	}
	return PressureOf(policy, pace, c, takes, now)
}
