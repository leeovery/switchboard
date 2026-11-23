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

// toHistory hands the readings history the lines a taking in of the account
// with the given id gives, read at at from a source, as recording has them,
// as note does, reporting whether that changed the reserve any window's last
// line was recorded under, for the state file. s.mu must be held, the pools
// regrouped.
func (s *state) toHistory(id string, changed []quota.Window, at time.Time, from readings.Source) (renoted bool) {
	return s.note(id, s.recording(id, changed, at, from))
}

// recording returns the lines of the readings history a taking in of the
// account with the given id gives, read at at from a source: of changed, the
// windows that read otherwise, and of each other window the taking read, as
// its readAt says, whose last line recorded another reserve than the one the
// router holds the account to, as reserveOf judges it, or none, as of one
// restate passed over: so its stretch is judged by that reserve from its
// next reading on. Each is recorded under that reserve. s.mu must be held,
// the pools regrouped.
func (s *state) recording(id string, changed []quota.Window, at time.Time, from readings.Source) []readings.Reading {
	a, _ := s.accounts.byID(id)
	u, reserve := s.usage[id], s.reserveOf(a)
	lines := slices.Clone(changed)
	for _, w := range u.latest() {
		if u.readAt[w.Key].Equal(at) && !u.recordedAs(w.Key, reserve) && !slices.ContainsFunc(changed, func(c quota.Window) bool { return c.Key == w.Key }) {
			lines = append(lines, w)
		}
	}
	return readings.Under(readings.Of(id, lines, at, from), reserve)
}

// note hands the readings history lines of the account with the given id,
// each recorded under a reserve, and, where it queued them, notes that
// reserve as each window's last line's, reporting whether that changed any
// window's, as a window's first line does. s.mu must be held.
func (s *state) note(id string, lines []readings.Reading) (renoted bool) {
	if len(lines) == 0 || !s.history(lines) {
		return false
	}
	u := s.usage[id]
	for _, r := range lines {
		if !u.recordedAs(r.Key, *r.Reserve) {
			u.recordedUnder[r.Key], renoted = *r.Reserve, true
		}
	}
	return renoted
}

// restate records each account's windows in the readings history again, as
// the router holds them at at, where the history last recorded them under
// another reserve than the one the router holds the account to, as
// reserveOf judges it, or none: so a summary judges each stretch of the
// history by the cap in force through it, though the use reads as before. A
// window the router doesn't hold as freshly read at at, as fresh says, is
// passed over, as its reading may be long out of date: its next reading
// records it, as recording says. The state file is told of each reserve
// recorded afresh. s.mu must be held, the pools regrouped.
func (s *state) restate(at time.Time) {
	renoted := false
	for _, a := range s.accounts {
		u, reserve := s.usage[a.ID], s.reserveOf(a)
		var due []quota.Window
		for _, w := range u.latest() {
			if !u.recordedAs(w.Key, reserve) && u.fresh(w, at) {
				due = append(due, w)
			}
		}
		renoted = s.note(a.ID, readings.Under(readings.Of(a.ID, due, at, readings.FromReserve), reserve)) || renoted
	}
	if renoted {
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

// recordedAs reports whether the readings history last recorded the window
// named key under reserve.
func (u *usage) recordedAs(key string, reserve float64) bool {
	recorded, ok := u.recordedUnder[key]
	return ok && recorded == reserve
}

// fresh reports whether the router holds w, a window of the account, as
// freshly read at at: read no longer ago than staleAfter, past which a
// choice made afresh probes the account, and not reset since.
func (u *usage) fresh(w quota.Window, at time.Time) bool {
	read := u.readAt[w.Key]
	return !read.IsZero() && at.Sub(read) <= staleAfter && !w.ResetBy(at)
}
