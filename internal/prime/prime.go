// Package prime works out the priming schedule: when each account is sent a
// prime, a request that starts the window a request starts, so that the
// accounts' windows reset at even steps through the day rather than together,
// and when a prime is due. Every function is pure, and is handed the clock
// and the readings. None knows a provider's windows by name: a score.Policy
// names the one a request starts, and its key gives its length.
package prime

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

const (
	// fullDay is how long a day runs on the clock.
	fullDay = 24 * time.Hour
	// afterReset is how long after a window's reset its account is primed:
	// the upstream's clock may be a little behind this one's, and a prime it
	// took in before the reset by its own would start nothing.
	afterReset = 5 * time.Second
)

// Schedule is when the accounts are primed: each at its slot, before the day
// starts, so that their first windows reset at even steps through it, the
// first half a step after it starts; and again whenever its window isn't
// running, from its slot until the day ends, so its windows run back to back
// through the day, and lapse overnight.
type Schedule struct {
	day config.Day
	// window is the key of the window a request starts, and length how long
	// it runs once started.
	window string
	length time.Duration
	slots  []Slot
}

// Slot is an account's daily prime.
type Slot struct {
	Account string
	// At is the time of day the account is primed, as the time since
	// midnight.
	At time.Duration
	// offset is when, as the time since the midnight that starts the day: At,
	// or At less a day when that falls the evening before.
	offset time.Duration
}

// New works out the schedule for the day over the accounts given, in their
// order, priming the window a request starts, as policy names it: with N
// accounts, their first windows reset every length ÷ N, the first half a step
// after the day starts, and each account is primed a length before its first
// reset, to the minute. It reports false when there's nothing to schedule: no
// day, no account, or a window whose key doesn't give its length.
func New(day config.Day, accounts []string, policy score.Policy) (Schedule, bool) {
	length, ok := quota.Length(policy.Started)
	if !ok || day == (config.Day{}) || len(accounts) == 0 {
		return Schedule{}, false
	}
	step := length / time.Duration(len(accounts))
	s := Schedule{day: day, window: policy.Started, length: length}
	for i, id := range accounts {
		reset := day.Start + step/2 + time.Duration(i)*step
		offset := (reset - length).Round(time.Minute)
		s.slots = append(s.slots, Slot{Account: id, At: timeOfDay(offset), offset: offset})
	}
	return s, true
}

// Day is the day the schedule spreads the resets over.
func (s Schedule) Day() config.Day {
	return s.day
}

// Window is the key of the window the schedule primes: the one a request
// starts.
func (s Schedule) Window() string {
	return s.window
}

// Slots are the accounts' slots, in the order they fall, which is the order
// the accounts were given in.
func (s Schedule) Slots() []Slot {
	return slices.Clone(s.slots)
}

// Next returns when the account is next due a prime, at or after now, whose
// time zone the day is kept in: when its window a request starts, among its
// windows as last read, isn't running, from its slot until the day ends. Its
// window isn't running when it has never been read, or has lapsed, its reset
// passed with nothing read since; one read running is primed afterReset after
// its reset. Next reports false when the window can't be judged, as when it
// was read without a reset, and may be running, and for an account the
// schedule doesn't have.
func (s Schedule) Next(account string, windows []quota.Window, now time.Time) (time.Time, bool) {
	i := slices.IndexFunc(s.slots, func(slot Slot) bool { return slot.Account == account })
	if i < 0 {
		return time.Time{}, false
	}
	idle, ok := s.idleFrom(windows, now)
	if !ok {
		return time.Time{}, false
	}
	return s.earliest(s.slots[i], idle), true
}

// idleFrom returns when, at now or after, the window a request starts, among
// windows as last read, is to be primed, in now's time zone: now, when it has
// never been read or has lapsed; else afterReset after its reset. It reports
// false when windows were read but not that one, or it was read without a
// reset: there's nothing to go by.
func (s Schedule) idleFrom(windows []quota.Window, now time.Time) (time.Time, bool) {
	if len(windows) == 0 {
		return now, true
	}
	i := slices.IndexFunc(windows, func(w quota.Window) bool { return w.Key == s.window })
	if i < 0 || windows[i].ResetsAt.IsZero() {
		return time.Time{}, false
	}
	if from := windows[i].ResetsAt.Add(afterReset); from.After(now) {
		return from.In(now.Location()), true
	}
	return now, true
}

// earliest returns the first time, at t or after, that the slot's account is
// primed on: t itself while a day's priming of it runs, from its slot until
// the day ends, else when the next day's does.
func (s Schedule) earliest(slot Slot, t time.Time) time.Time {
	y, m, d := t.Date()
	for date := d - 1; ; date++ {
		from, until := s.span(slot, y, m, date, t.Location())
		if t.Before(until) {
			if t.Before(from) {
				return from
			}
			return t
		}
	}
}

// span returns when the slot's account is primed on the day that starts on
// the date given, in loc: from its slot until the day ends.
func (s Schedule) span(slot Slot, y int, m time.Month, d int, loc *time.Location) (from, until time.Time) {
	return wallClock(y, m, d, slot.offset, loc), wallClock(y, m, d, s.end(), loc)
}

// end is when the day ends, as the time since the midnight that starts it:
// past the next midnight when its end comes before its start.
func (s Schedule) end() time.Duration {
	if s.day.End > s.day.Start {
		return s.day.End
	}
	return s.day.End + fullDay
}

// wallClock is the time the clock in loc shows offset after midnight on the
// date given: counted in the clock's minutes, not in time elapsed, so a date
// the clocks change on keeps its times of day.
func wallClock(y int, m time.Month, d int, offset time.Duration, loc *time.Location) time.Time {
	return time.Date(y, m, d, 0, int(offset/time.Minute), 0, 0, loc)
}

// timeOfDay is offset, the time since some midnight, as the time since the
// last midnight before it.
func timeOfDay(offset time.Duration) time.Duration {
	return (offset%fullDay + fullDay) % fullDay
}
