package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// sizes returns each account's size, in Pro weeks, by its id, as the price
// table's Sizes has them of the plans configured, in their order.
func (as accounts) sizes() map[string]float64 {
	plans := make([]string, len(as))
	for i, a := range as {
		plans[i] = a.Plan
	}
	each, _ := ledger.Pricing.Sizes(plans)
	sizes := make(map[string]float64, len(as))
	for i, size := range each {
		sizes[as[i].ID] = float64(size)
	}
	return sizes
}

// rose takes in a rise of the account's window named key, a share of it, at
// at: the pool's day rate takes in its shared week's, the policy's
// perishable window, alone, in Pro weeks, as the account's size makes it.
// s.mu must be held.
func (s *state) rose(id, key string, rise float64, at time.Time) {
	if key == s.policy.Perishable {
		s.day = s.day.Rose(rise*s.sizes[id], at)
	}
}

// judgeRoom has the pool's day rate count the time from at on as time the
// pool has room for a new session in, or none, as roomy judges it, week
// giving each account's shared week, and pin the accounts the pools' pins
// name. s.mu must be held.
func (s *state) judgeRoom(week func(id string) (quota.Window, bool), pin []string, at time.Time) {
	s.day = s.day.Open(s.roomy(week, pin, at), at)
}

// roomy reports whether the pool has room for a new session at at: the
// router would find an account for one, choosing as score.Policy.Choose does
// by the accounts' shared weeks alone, week giving each, of those with a
// usable token whose week has been read, but those it couldn't go out on,
// their token refused or a limit holding back their week, as shut says. So
// an account has room within its reserve, or within its limit where pin, the
// accounts the pools' pins name, names it, which spends the reserve. s.mu
// must be held.
func (s *state) roomy(week func(id string) (quota.Window, bool), pin []string, at time.Time) bool {
	isWeek := func(key string) bool { return key == s.policy.Perishable }
	var candidates []score.Candidate
	for _, a := range s.accounts {
		if w, ok := week(a.ID); ok && a.hasToken() && !s.usage[a.ID].shut(at, isWeek) {
			candidates = append(candidates, score.Candidate{ID: a.ID, Windows: []quota.Window{w}, Reserve: a.Reserve})
		}
	}
	_, ok := s.policy.Choose(candidates, pin, isWeek, "", at)
	return ok
}

// repin notes each pool's pin from now on, as the sessions hold them, and
// judges the pool's room afresh then, as judgeRoom says, so a pin that
// spends an account's reserve gives the pool room from its own moment.
func (s *state) repin(ps pins) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins = ps
	s.judgeRoom(s.weekHeld, s.pinned(), at)
}

// recallPins takes in each pool's pin the state file kept, as the router
// starts, as repin does, but judging nothing: seed, taking up the history
// after it, judges the pool's room by each from when it was set, as pinnedAt
// says, and the pool judged at now first would have the day's rate take
// every time seed replays as now.
func (s *state) recallPins(ps pins) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins = ps
}

// pinned returns the ids of the accounts their pools' pins name, as spends
// says, in the order configured. s.mu must be held.
func (s *state) pinned() []string {
	return s.pinnedIf(func(status.Pin) bool { return true })
}

// pinnedAt returns the ids of the accounts their pools' pins named at at, as
// seed takes up the history: those each names now from when it was set, and
// none before, as no pin that may have held then is kept. s.mu must be
// held.
func (s *state) pinnedAt(at time.Time) []string {
	return s.pinnedIf(func(pin status.Pin) bool { return !at.Before(pin.Since) })
}

// pinnedIf returns the ids of the accounts their pools' pins name, as the
// pools stand, of those pins held says hold, in the order configured. s.mu
// must be held.
func (s *state) pinnedIf(held func(pin status.Pin) bool) []string {
	t := s.pools.current()
	var ids []string
	for _, a := range s.accounts {
		pool, in := t.in(a.ID)
		if pin := s.pins.of(t, pool); in && pin.Has(a.ID) && held(pin) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// weekHeld returns the shared week of the account with the given id, the
// policy's perishable window, as last read, reporting false where it hasn't
// been read. s.mu must be held.
func (s *state) weekHeld(id string) (quota.Window, bool) {
	w, ok := s.usage[id].windows[s.policy.Perishable]
	return w, ok
}

// answered has the hold curve take in an answer of the tokens given, at at,
// from a session on its account for onAccount, weighed as forecast.Weight
// weighs them. The state file keeps it as a routine change, as it does the
// reading off an answer.
func (s *state) answered(tokens quota.Tokens, onAccount time.Duration, at time.Time) {
	weight := forecast.Weight(int64(tokens.Input), int64(tokens.Output), int64(tokens.CacheWrite), int64(tokens.CacheRead))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hold = s.hold.Answered(weight, onAccount, at)
	s.routine()
}

// demand is the pool's use at now, as the forecast takes it: its day rate,
// the pool judged afresh for room at now, so a week that has reset since it
// was read has given it room by then; or, with too few hours of room for
// one, the pool's fast rate, as poolFast has it; and the hold curve at now,
// learning while it's the default. It's nil where no account has been read.
func (s *state) demand(now time.Time) *status.Demand {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.accounts, func(a account) bool { return len(s.usage[a.ID].windows) > 0 }) {
		return nil
	}
	s.judgeRoom(s.weekHeld, s.pinned(), now)
	demand := &status.Demand{Hold: holdPoints(s.hold.Points(now)), Learning: s.hold.Learning(now)}
	if rate, over, ok := s.day.Rate(now); ok {
		demand.Day, demand.Hours = rate, over.Hours()
	} else {
		demand.Day = s.poolFast(now)
	}
	return demand
}

// holdPoints are the hold curve's points as the status document gives them.
func holdPoints(points []forecast.Point) []status.HoldPoint {
	held := make([]status.HoldPoint, len(points))
	for i, p := range points {
		held[i] = status.HoldPoint{Hours: p.After.Hours(), Share: p.Share}
	}
	return held
}

// poolFast is the pool's fast rate at now, in Pro weeks an hour: each
// account's shared week's fast rate, as fastRate has it, by the account's
// size. s.mu must be held.
func (s *state) poolFast(now time.Time) float64 {
	var fast float64
	for _, a := range s.accounts {
		fast += s.sizes[a.ID] * s.usage[a.ID].fastRate(s.policy.Perishable, now)
	}
	return fast
}
