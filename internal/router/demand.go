package router

import (
	"maps"
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

// sizesIn returns each account's size, in Pro weeks, by its id, as sizes has
// them of the accounts of its pool, as t has the pools, as the forecast
// sizes those of the pool's document: none for an account in no pool. s.mu
// must be held.
func (s *state) sizesIn(t *poolTable) map[string]float64 {
	sizes := make(map[string]float64, len(s.accounts))
	for _, pool := range t.each() {
		maps.Copy(sizes, t.routed(pool, s.accounts).sizes())
	}
	return sizes
}

// regroup has the state reckon each pool's demand in the pools as they
// stand, where they've changed since it last did, and returns them: an
// account that has moved to another pool has its fast rates forgotten, so
// it brings none of its old pool's demand, while its old pool's day rate
// keeps what it took in, as that pool's use; each account is sized among its
// pool's, as sizesIn says; and as the pools first come in, the day rate of
// the one pool of a config from before pools is the first pool's, as all
// work before pools was. The day rate of a pool no longer configured goes. A
// closed table, which can't tell which pool any account is in, leaves the
// pools as last reckoned, so no pool's demand goes while nothing is routed.
// s.mu must be held.
func (s *state) regroup() *poolTable {
	t := s.pools.current()
	if t == s.grouped || t.closed != "" {
		return s.grouped
	}
	s.forgetMoved(s.grouped, t)
	if s.grouped.within("") && t.pooled() {
		s.days[t.first] = s.days[""]
	}
	maps.DeleteFunc(s.days, func(pool string, _ forecast.Day) bool { return !t.within(pool) })
	s.sizes, s.grouped = s.sizesIn(t), t
	return t
}

// forgetMoved forgets the fast rates of each account in another pool in now
// than it was in was, where both have pools: none moves as the pools first
// come in. It notes each in the log. s.mu must be held.
func (s *state) forgetMoved(was, now *poolTable) {
	if !was.pooled() || !now.pooled() {
		return
	}
	for _, a := range s.accounts {
		from, _ := was.in(a.ID)
		if to, _ := now.in(a.ID); to != from {
			clear(s.usage[a.ID].fast)
			logger.Debug("forgot the rates of an account that moved pools", "account", a.ID, "from", from, "pool", to)
		}
	}
}

// rose takes in a rise of the account's window named key, a share of it, at
// at: the day rate of the account's pool takes in its shared week's, the
// policy's perishable window, alone, in Pro weeks, as the account's size
// makes it. s.mu must be held, the pools regrouped.
func (s *state) rose(id, key string, rise float64, at time.Time) {
	pool, in := s.grouped.in(id)
	if key == s.policy.Perishable && in {
		s.days[pool] = s.days[pool].Rose(rise*s.sizes[id], at)
	}
}

// judgeRooms has each pool's day rate count the time from at on as time the
// pool has room for a new session in, or none, as roomy judges it, week
// giving each account's shared week, and pinned the accounts each pool's pin
// names. s.mu must be held, the pools regrouped.
func (s *state) judgeRooms(week func(id string) (quota.Window, bool), pinned func(pool string) []string, at time.Time) {
	for _, pool := range s.grouped.each() {
		s.days[pool] = s.days[pool].Open(s.roomy(week, pool, pinned(pool), at), at)
	}
}

// roomy reports whether the pool with the given id has room for a new
// session at at: the router would find one of its accounts for one, choosing
// as score.Policy.Choose does by the accounts' shared weeks alone, week
// giving each, of those with a usable token whose week has been read, but
// those it couldn't go out on, their token refused or a limit holding back
// their week, as shut says. So an account has room within its reserve, or
// within its limit where pin, the accounts the pool's pin names, names it,
// which spends the reserve. s.mu must be held, the pools regrouped.
func (s *state) roomy(week func(id string) (quota.Window, bool), pool string, pin []string, at time.Time) bool {
	isWeek := func(key string) bool { return key == s.policy.Perishable }
	var candidates []score.Candidate
	for _, a := range s.grouped.routed(pool, s.accounts) {
		if w, ok := week(a.ID); ok && a.hasToken() && !s.usage[a.ID].shut(at, isWeek) {
			candidates = append(candidates, score.Candidate{ID: a.ID, Windows: []quota.Window{w}, Reserve: a.Reserve})
		}
	}
	_, ok := s.policy.Choose(candidates, pin, isWeek, "", at)
	return ok
}

// repin notes each pool's pin from now on, as the sessions hold them, and
// judges the pools' room afresh then, as judgeRooms says, so a pin that
// spends an account's reserve gives its pool room from its own moment.
func (s *state) repin(ps pins) {
	at := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regroup()
	s.pins = ps
	s.judgeRooms(s.weekHeld, s.pinnedIn, at)
}

// recallPins takes in each pool's pin the state file kept, as the router
// starts, as repin does, but judging nothing: seed, taking up the history
// after it, judges the pools' room by each from when it was set, as pinnedAt
// says, and the pools judged at now first would have the day's rate take
// every time seed replays as now.
func (s *state) recallPins(ps pins) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pins = ps
}

// pinnedIn returns the ids of the accounts the pin of the pool with the given
// id names. s.mu must be held.
func (s *state) pinnedIn(pool string) []string {
	return s.pins[pool].Accounts
}

// pinnedAt returns what gives the ids of the accounts the pin of each pool
// named at at, as seed takes up the history: those it names now from when
// it was set, and none before, as no pin that may have held then is kept.
// s.mu must be held as what it returns is called.
func (s *state) pinnedAt(at time.Time) func(pool string) []string {
	return func(pool string) []string {
		if pin := s.pins[pool]; !at.Before(pin.Since) {
			return pin.Accounts
		}
		return nil
	}
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

// demand is the use of the pool with the given id at now, as the forecast
// takes it: its day rate, the pools judged afresh for room at now, so a week
// that has reset since it was read has given its pool room by then; or, with
// too few hours of room for one, the pool's fast rate, as poolFast has it;
// and the hold curve at now, which every pool shares, learning while it's
// the default. It's nil where none of the pool's accounts has been read.
func (s *state) demand(now time.Time, pool string) *status.Demand {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.regroup()
	if !slices.ContainsFunc(t.routed(pool, s.accounts), func(a account) bool { return len(s.usage[a.ID].windows) > 0 }) {
		return nil
	}
	s.judgeRooms(s.weekHeld, s.pinnedIn, now)
	demand := &status.Demand{Hold: holdPoints(s.hold.Points(now)), Learning: s.hold.Learning(now)}
	if rate, over, ok := s.days[pool].Rate(now); ok {
		demand.Day, demand.Hours = rate, over.Hours()
	} else {
		demand.Day = s.poolFast(pool, now)
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

// poolFast is the fast rate of the pool with the given id at now, in Pro
// weeks an hour: each of its accounts' shared week's fast rate, as fastRate
// has it, by the account's size. s.mu must be held, the pools regrouped.
func (s *state) poolFast(pool string, now time.Time) float64 {
	var fast float64
	for _, a := range s.grouped.routed(pool, s.accounts) {
		fast += s.sizes[a.ID] * s.usage[a.ID].fastRate(s.policy.Perishable, now)
	}
	return fast
}
