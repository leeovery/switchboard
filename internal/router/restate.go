package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// reserveOf returns the reserve the router holds account a to: its own, but
// none where its pool's pin names it, as a pin spends the reserves of the
// accounts it names. s.mu must be held, the pools regrouped.
func (s *state) reserveOf(a account) float64 {
	if pool, ok := s.grouped.in(a.ID); ok && slices.Contains(s.pinnedIn(pool), a.ID) {
		return 0
	}
	return a.Reserve
}

// readingsOf returns windows of the account with the given id, read at at
// from a source, as lines of the readings history, each recorded under the
// reserve the router holds the account to, as reserveOf judges it. s.mu must
// be held, the pools regrouped.
func (s *state) readingsOf(id string, windows []quota.Window, at time.Time, from readings.Source) []readings.Reading {
	a, _ := s.accounts.byID(id)
	return readings.Under(readings.Of(id, windows, at, from), s.reserveOf(a))
}

// restate records each account's windows in the readings history again, as
// it holds them at at, but those that have reset by then, where the reserve
// the router holds it to, as reserveOf judges it, isn't the one the history
// last recorded them under, as recordedUnder says: so a summary judges each
// stretch of the history by the cap in force through it, though the
// account's use reads as before. The state file keeps the reserve each is
// recorded under from then. s.mu must be held, the pools regrouped.
func (s *state) restate(at time.Time) {
	restated := false
	for _, a := range s.accounts {
		u, reserve := s.usage[a.ID], s.reserveOf(a)
		if u.recordedUnder != nil && *u.recordedUnder == reserve {
			continue
		}
		running := slices.DeleteFunc(u.latest(), func(w quota.Window) bool { return w.ResetBy(at) })
		s.history(readings.Under(readings.Of(a.ID, running, at, readings.FromReserve), reserve))
		u.recordedUnder, restated = new(reserve), true
	}
	if restated {
		s.changed()
	}
}

// restateNow restates the accounts' windows at now, as restate does: as the
// router starts, once its readings history is open, on the reserves its
// config gives and the pins the state file kept.
func (s *state) restateNow() {
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regroup()
	s.restate(now)
}
