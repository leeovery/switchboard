package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// sizes returns each account's size, in Pro weeks, by its id, as
// forecast.Sizes has them of the plans configured, in their order.
func (as accounts) sizes() map[string]float64 {
	plans := make([]string, len(as))
	for i, a := range as {
		plans[i] = a.Plan
	}
	sizes := make(map[string]float64, len(as))
	for i, size := range forecast.Sizes(plans) {
		sizes[as[i].ID] = size
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
// giving each account's shared week. s.mu must be held.
func (s *state) judgeRoom(week func(id string) (quota.Window, bool), at time.Time) {
	s.day = s.day.Open(s.roomy(week, at), at)
}

// roomy reports whether the pool has room for a new session at at: an account
// with a usable token has room within its reserve in its shared week, as week
// gives it, as score.Available judges it, and a new session's choice of
// account would. One whose week hasn't been read has none, as that choice
// can't score it. s.mu must be held.
func (s *state) roomy(week func(id string) (quota.Window, bool), at time.Time) bool {
	return slices.ContainsFunc(s.accounts, func(a account) bool {
		w, ok := week(a.ID)
		return ok && a.hasToken() && score.Available([]quota.Window{w}, a.Reserve, func(string) bool { return true }, at)
	})
}

// weekHeld returns the shared week of the account with the given id, the
// policy's perishable window, as last read, reporting false where it hasn't
// been read. s.mu must be held.
func (s *state) weekHeld(id string) (quota.Window, bool) {
	w, ok := s.usage[id].windows[s.policy.Perishable]
	return w, ok
}

// demand is the pool's use at now, as the forecast takes it: its day rate,
// the pool judged afresh for room at now, so a week that has reset since it
// was read has given it room by then; or, with too few hours of room for
// one, the pool's fast rate, as poolFast has it. It's nil where no account
// has been read.
func (s *state) demand(now time.Time) *status.Demand {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.accounts, func(a account) bool { return len(s.usage[a.ID].windows) > 0 }) {
		return nil
	}
	s.judgeRoom(s.weekHeld, now)
	if rate, over, ok := s.day.Rate(now); ok {
		return &status.Demand{Day: rate, Hours: over.Hours()}
	}
	return &status.Demand{Day: s.poolFast(now)}
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
