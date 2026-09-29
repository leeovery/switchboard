package status

import (
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prime"
)

// Prime is the priming schedule, as the document gives it: the day, in local
// time, the key of the window a prime starts, and each account's daily
// prime, in the order they fall. It's zero when priming is off.
type Prime struct {
	// Day is the part of each day the resets are spread over, such as
	// "08:00-23:00": an end before the start is past midnight.
	Day string `json:"day"`
	// Window is the key of the window a prime starts, such as "5h".
	Window string `json:"window"`
	Slots  []Slot `json:"slots"`
}

// Slot is an account's daily prime.
type Slot struct {
	Account string `json:"account"`
	// At is the local time of day the account is primed, such as "03:50".
	At string `json:"at"`
	// Next is when the router next primes the account, as its windows stand:
	// zero in a document that isn't the router's.
	Next time.Time `json:"next,omitzero"`
}

// Priming is the schedule as the document gives it.
func Priming(s prime.Schedule) Prime {
	day := s.Day()
	p := Prime{Day: clockOf(day.Start) + "-" + clockOf(day.End), Window: s.Window()}
	for _, slot := range s.Slots() {
		p.Slots = append(p.Slots, Slot{Account: slot.Account, At: clockOf(slot.At)})
	}
	return p
}

// clockOf shows a time of day, as the time since midnight, as HH:MM.
func clockOf(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d/time.Hour), int(d%time.Hour/time.Minute))
}

// Coming is what comes next of priming on an account, at a time: its window
// a prime starts resetting, or the router priming it.
type Coming struct {
	// What says which, as status and the dashboard show it: "next reset" or
	// "next prime".
	What    string
	Account Account
	At      time.Time
}

// Coming returns what comes next of priming at now, as the document gives
// it: of the windows a prime starts, the next to reset, and the next prime
// the router has due. It's none when priming is off.
func (d Document) Coming(now time.Time) []Coming {
	var coming []Coming
	if a, at, ok := d.nextReset(now); ok {
		coming = append(coming, Coming{What: "next reset", Account: a, At: at})
	}
	if a, at, ok := d.nextPrime(); ok {
		coming = append(coming, Coming{What: "next prime", Account: a, At: at})
	}
	return coming
}

// nextReset returns the account whose window a prime starts resets first
// after now, and when, reporting false when none is running, or priming is
// off.
func (d Document) nextReset(now time.Time) (Account, time.Time, bool) {
	var first Account
	var at time.Time
	if d.Prime.Window == "" {
		return first, at, false
	}
	for _, a := range d.Accounts {
		for _, w := range a.Windows {
			if w.Key == d.Prime.Window && w.ResetsAt.After(now) && (at.IsZero() || w.ResetsAt.Before(at)) {
				first, at = a, w.ResetsAt
			}
		}
	}
	return first, at, !at.IsZero()
}

// nextPrime returns the account the router primes next, and when, reporting
// false when the document isn't the router's, or priming is off.
func (d Document) nextPrime() (Account, time.Time, bool) {
	slots := slices.DeleteFunc(slices.Clone(d.Prime.Slots), func(s Slot) bool { return s.Next.IsZero() })
	if len(slots) == 0 {
		return Account{}, time.Time{}, false
	}
	first := slices.MinFunc(slots, func(a, b Slot) int { return a.Next.Compare(b.Next) })
	account, ok := d.Account(first.Account)
	if !ok {
		account = Account{ID: first.Account, Label: first.Account}
	}
	return account, first.Next, true
}

// NotStarted says what a window that has lapsed on the account with the
// given id reads: that it hasn't started, and when the router next primes
// the account, as far as the document says, in now's time zone, such as
// "not started · next prime Tue 03:50".
func (d Document) NotStarted(id string, now time.Time) string {
	i := slices.IndexFunc(d.Prime.Slots, func(s Slot) bool { return s.Account == id })
	if i < 0 || d.Prime.Slots[i].Next.IsZero() {
		return notStarted
	}
	return notStarted + " · next prime " + Clock(now, d.Prime.Slots[i].Next)
}
