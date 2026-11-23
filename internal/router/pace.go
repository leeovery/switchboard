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

// FastLeast is the least fast rate, a share of a window an hour, that counts
// as one: one faded below it is none, so the window's entry in the rates
// goes once its use has long stopped.
const FastLeast = 1e-6

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
// configured, an account's fast rates forgotten at each move between pools
// the record of the pools it has been in holds, as movesKept has them, as
// forgetMovedBy says, before any reading at or after it, and the day rate of
// the pool each was in as it was read, as inAt has it, as though they'd come
// in as read then, the pools judged for room at each, by the reserve each
// account's week's line recorded, as judgeSeeded says, and at each line
// recording a window again as its account's reserve changed, as
// takeUpReserve says, and at each moment a pin kept was set, an account
// joined another pool or an account's read refused began to hold it, as
// judgedAlso has them, where the history read anything before it, as
// judgeSeeded says, each pool by the accounts in it then: the readings
// history holds each change of a window's use, so its trail, baseline
// included, is as it was, and its rates outlast a restart. It takes up no
// reading of a line takenUp passes over. A window the account doesn't hold,
// as one its answers no longer report, keeps no trail nor fast rate, and nor
// does one whose reading, as the state file kept it, has another reset than
// its readings last read, as when it has reset since. It returns how many
// readings the trails keep.
func (s *state) seed(history iter.Seq[readings.Reading]) int {
	read := slices.Collect(history)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regroup()
	taking := newSeeding(read, s.policy.Perishable)
	also, moves := s.judgedAlso(), s.movesKept()
	for i, r := range read {
		for ; len(also) > 0 && !r.At.Before(also[0]); also = also[1:] {
			if i > 0 {
				s.judgeSeeded(taking, also[0])
			}
		}
		moves = s.forgetMovedBy(moves, r.At)
		if u, ok := s.takenUp(r); ok {
			s.takeUpReading(taking, u, r)
		} else if s.restates(r) {
			s.takeUpReserve(taking, r)
		}
	}
	for _, m := range moves {
		clear(s.usage[m.account].fast)
	}
	if len(read) > 0 {
		for _, at := range also {
			s.judgeSeeded(taking, at)
		}
	}
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

// judgedAlso returns the moments seed judges the pools' room at besides the
// readings', in order, once each: when each of the pins kept was set, when
// each account joined another pool, as joinedAt has them, and when each
// account's read refused began to hold it, as heldByReadAt has it, as the
// pools' room changed then with no reading to show it. s.mu must be held.
func (s *state) judgedAlso() []time.Time {
	set := s.joinedAt()
	for _, pin := range s.pins {
		if !pin.IsZero() {
			set = append(set, pin.Since)
		}
	}
	for _, u := range s.usage {
		if u.readRefused() {
			set = append(set, u.refusedSince)
		}
	}
	slices.SortFunc(set, time.Time.Compare)
	return slices.CompactFunc(set, time.Time.Equal)
}

// takenUp returns the usage of the account a line of the readings history
// reads, reporting false where the line is no reading to take up: one of an
// account not configured, or of a window recorded again as its reserve
// changed, which nothing read then. s.mu must be held.
func (s *state) takenUp(r readings.Reading) (*usage, bool) {
	u, configured := s.usage[r.Account]
	return u, configured && r.Source != readings.FromReserve
}

// takeUpReading takes up r, a reading of the history of the account whose
// usage u is, after those taking has taken up: into its window's trail and
// fast rate, as note says, from the reading of the window taken up last, and
// the day rate of the pool its account was in then, as rose and inAt say,
// the pools judged for room at it, as judgeSeeded says. s.mu must be held,
// the pools regrouped.
func (s *state) takeUpReading(taking seeding, u *usage, r readings.Reading) {
	of, w := windowOf{account: r.Account, key: r.Key}, r.Window()
	rise := u.note(taking.last[of], w, r.At)
	taking.last[of], taking.under[of] = w, r.Reserve
	s.rose(s.inAt(r.At), r.Account, r.Key, rise, r.At)
	s.judgeSeeded(taking, r.At)
}

// restates reports whether a line of the readings history records a window
// of an account configured again as its reserve changed. s.mu must be held.
func (s *state) restates(r readings.Reading) bool {
	_, configured := s.usage[r.Account]
	return configured && r.Source == readings.FromReserve
}

// takeUpReserve takes up r, a line recording a window again as its account's
// reserve changed, after those taking has taken up: the reserve the window
// was held to from then, the pools judged for room at it, as judgeSeeded
// says, as their room can change with no reading to show it. s.mu must be
// held, the pools regrouped.
func (s *state) takeUpReserve(taking seeding, r readings.Reading) {
	taking.under[windowOf{account: r.Account, key: r.Key}] = r.Reserve
	s.judgeSeeded(taking, r.At)
}

// forgetMovedBy forgets, as forgetMoved does live, the fast rates of the
// account of each of moves, in order, made at or before at, its trails
// standing, and returns the moves after at. s.mu must be held.
func (s *state) forgetMovedBy(moves []poolMove, at time.Time) []poolMove {
	i := 0
	for ; i < len(moves) && !moves[i].at.After(at); i++ {
		clear(s.usage[moves[i].account].fast)
	}
	return moves[i:]
}

// judgeSeeded judges the pools' room at at as the history has been taken up
// by then, as judgeRooms does: by the accounts in each pool then, as inAt
// has them, each account's week as seeding's weekAt has it then, under the
// reserve its line recorded, as readings.ReserveThen has it, its reserve as
// configured where the line recorded none, each pool's pin as pinnedAt has
// it, and each account shut then as shutThen says. s.mu must be held, the
// pools regrouped.
func (s *state) judgeSeeded(taking seeding, at time.Time) {
	week := func(a account) (quota.Window, float64, bool) {
		w, under, ok := taking.weekAt(a.ID, s.weekHeld, at)
		return w, readings.ReserveThen(under, a.Reserve), ok
	}
	s.judgeRooms(rooms{in: s.inAt(at), week: week, pinned: s.pinnedAt(at), shut: (*usage).shutThen}, at)
}

// seeding is the readings history as seed takes it up: the reading of each
// account's window taken up last, and the reserve the window's line taken up
// last recorded, of a reading or of the window recorded again as the reserve
// changed; and each account's first line of its shared week, the window
// named week, the history holds.
type seeding struct {
	week  string
	last  map[windowOf]quota.Window
	under map[windowOf]*float64
	first map[string]readings.Reading
}

// newSeeding starts the taking up of read, the readings history in the order
// read, of the shared week, the window named week.
func newSeeding(read []readings.Reading, week string) seeding {
	taking := seeding{week: week, last: make(map[windowOf]quota.Window), under: make(map[windowOf]*float64), first: make(map[string]readings.Reading)}
	for _, r := range read {
		if _, seen := taking.first[r.Account]; !seen && r.Key == week {
			taking.first[r.Account] = r
		}
	}
	return taking
}

// weekAt returns the shared week of the account with the given id at at, as
// the history has read it by then, and the reserve its line recorded, nil
// where none did: as taken up last; before that, as the history first reads
// it, or, where it holds none of it, as held gives it, the state file's, from
// when that week began, as its Span says, on, each under the reserve the
// window's line taken up last recorded, where one was. Before then, as far as
// is known, the account's week hadn't been read, and weekAt reports false; so
// it does where when the week began can't be said.
func (t seeding) weekAt(id string, held func(id string) (quota.Window, bool), at time.Time) (quota.Window, *float64, bool) {
	of := windowOf{account: id, key: t.week}
	under, restated := t.under[of]
	if w, ok := t.last[of]; ok {
		return w, under, true
	}
	first, ok := t.first[id]
	w := first.Window()
	if !restated {
		under = first.Reserve
	}
	if !ok {
		w, ok = held(id)
	}
	began, _, known := w.Span()
	return w, under, ok && known && !at.Before(began)
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
// it has faded below FastLeast.
func (u *usage) fastRate(key string, now time.Time) float64 {
	if w, ok := u.windows[key]; !ok || w.ResetBy(now) {
		return 0
	}
	if rate := u.fast[key].Rate(now); rate >= FastLeast {
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
