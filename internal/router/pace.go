package router

import (
	"iter"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// fastLeast is the least fast rate, a share of a window an hour, that counts
// as one: one faded below it is none, so the window's entry in the rates
// goes once its use has long stopped.
const fastLeast = 1e-6

// trails are how each of an account's windows has been read lately, as it
// now runs, by the window's key: the levels its use was read at, each with
// when it was first and last read so, its baseline, the level read last
// before the half hour, and each since, in the order read. The pressure
// window's pace, and each window's recent rate, are measured over them, and
// its fast rate takes in the rises they take, as note says. A router started
// afresh takes them up from the readings history, as seed says.
type trails map[string][]score.Reading

// note takes in kept, the reading of a window the account now stands as,
// taken in at a time, held being the one it stood as before: into the
// window's trail, as score.Noted says, and into its fast rate, as the rise it
// takes the trail up by, as score.Risen says, the fast rate starting afresh
// where the trail does. It returns that rise, a share of the window.
func (u *usage) note(held, kept quota.Window, at time.Time) float64 {
	rise, afresh := score.Risen(u.trails[kept.Key], held, kept)
	fast := u.fast[kept.Key]
	if afresh {
		fast = forecast.Fast{}
	}
	u.fast[kept.Key] = fast.Rose(rise, at)
	u.trails[kept.Key] = score.Noted(u.trails[kept.Key], held, kept, at)
	return rise
}

// forgetRates forgets the trail and the fast rate of the account's window
// named key.
func (u *usage) forgetRates(key string) {
	delete(u.trails, key)
	delete(u.fast, key)
}

// seed takes up, as the router starts, the readings history gives, in the
// order they were read, into the trails and fast rates of the accounts
// configured, and the pool's day rate, as though they'd come in as read then,
// the pool judged for room at each, and at the moment the global pin kept was
// set where the history read anything before it, as judgeSeeded says: the
// readings history holds each change of a window's use, so its trail,
// baseline included, is as it was, and its rates outlast a restart. A window
// the account doesn't hold, as one its answers no longer report, keeps no
// trail nor fast rate, and nor does one whose reading, as the state file kept
// it, has another reset than its readings last read, as when it has reset
// since. It returns how many readings the trails keep.
func (s *state) seed(history iter.Seq[readings.Reading]) int {
	read := slices.Collect(history)
	s.mu.Lock()
	defer s.mu.Unlock()
	taking := newSeeding(read, s.policy.Perishable)
	beforePin := slices.IndexFunc(read, func(r readings.Reading) bool { return !r.At.Before(s.pin.Since) })
	if beforePin < 0 {
		beforePin = len(read)
	}
	s.takeUp(taking, read[:beforePin])
	if beforePin > 0 {
		s.judgeSeeded(taking, s.pin.Since)
	}
	s.takeUp(taking, read[beforePin:])
	kept := 0
	for of, w := range taking.last {
		u := s.usage[of.account]
		if held, ok := u.windows[of.key]; !ok || !held.ResetsAt.Equal(w.ResetsAt) {
			u.forgetRates(of.key)
		}
		kept += len(u.trails[of.key])
	}
	return kept
}

// takeUp takes up read, readings of the history in the order read, as seed
// does. s.mu must be held.
func (s *state) takeUp(taking seeding, read []readings.Reading) {
	for _, r := range read {
		u, configured := s.usage[r.Account]
		if !configured {
			continue
		}
		of, w := windowOf{account: r.Account, key: r.Key}, r.Window()
		rise := u.note(taking.last[of], w, r.At)
		taking.last[of] = w
		s.rose(r.Account, r.Key, rise, r.At)
		s.judgeSeeded(taking, r.At)
	}
}

// judgeSeeded judges the pool's room at at as the history has been taken up
// by then, as judgeRoom does: by each account's week as seeding's weekAt has
// it then, and the global pin as pinnedAt has it. s.mu must be held.
func (s *state) judgeSeeded(taking seeding, at time.Time) {
	s.judgeRoom(func(id string) (quota.Window, bool) { return taking.weekAt(id, s.weekHeld, at) }, s.pinnedAt(at), at)
}

// seeding is the readings history as seed takes it up: the reading of each
// account's window taken up last, and each account's first reading of its
// shared week, the window named week, the history holds.
type seeding struct {
	week  string
	last  map[windowOf]quota.Window
	first map[string]quota.Window
}

// newSeeding starts the taking up of read, the readings history in the order
// read, of the shared week, the window named week.
func newSeeding(read []readings.Reading, week string) seeding {
	taking := seeding{week: week, last: make(map[windowOf]quota.Window), first: make(map[string]quota.Window)}
	for _, r := range read {
		if _, seen := taking.first[r.Account]; !seen && r.Key == week {
			taking.first[r.Account] = r.Window()
		}
	}
	return taking
}

// weekAt returns the shared week of the account with the given id at at, as
// the history has read it by then: as taken up last; before that, as the
// history first reads it, or, where it holds none of it, as held gives it,
// the state file's, from when that week began, as its Span says, on. Before
// then, as far as is known, the account's week hadn't been read, and weekAt
// reports false; so it does where when the week began can't be said.
func (t seeding) weekAt(id string, held func(id string) (quota.Window, bool), at time.Time) (quota.Window, bool) {
	if w, ok := t.last[windowOf{account: id, key: t.week}]; ok {
		return w, true
	}
	w, ok := t.first[id]
	if !ok {
		w, ok = held(id)
	}
	began, _, known := w.Span()
	return w, ok && known && !at.Before(began)
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
// at now, in quota.Sort's order, as status.RatesOf says, with its fast rate,
// as fastRate has it.
func (u *usage) rates(now time.Time) []status.Rate {
	fast := make(map[string]float64, len(u.fast))
	for key := range u.fast {
		fast[key] = u.fastRate(key, now)
	}
	return status.RatesOf(u.latest(), u.trails, fast, now)
}

// fastRate returns the fast rate of the account's window named key at now, a
// share of it an hour: none where the window hasn't been read, or has reset
// since, as its fast rate is of the window that ran before, and none where
// it has faded below fastLeast.
func (u *usage) fastRate(key string, now time.Time) float64 {
	if w, ok := u.windows[key]; !ok || w.ResetBy(now) {
		return 0
	}
	if rate := u.fast[key].Rate(now); rate >= fastLeast {
		return rate
	}
	return 0
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
