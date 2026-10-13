package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// The rules a moment works its facts out by, for the tests that pin each rule
// alone, and hold a moment's facts to them.

// PressureOf is pressureOf.
func PressureOf(policy score.Policy, pace score.Pace, c score.Candidate, takes func(reserve float64) bool, now time.Time) Pressure {
	return pressureOf(policy, pace, c, takes, now)
}

// Held is held.
func (a Account) Held(now time.Time, policy score.Policy) (Hold, bool) {
	return a.held(now, policy)
}

// HasLapsed is hasLapsed.
func (a Account) HasLapsed(key string, at time.Time, policy score.Policy) bool {
	return a.hasLapsed(key, at, policy)
}

// ReachedReserve is reachedReserve.
func (a Account) ReachedReserve(now time.Time) []string {
	return a.reachedReserve(now)
}

// Project is the account's project.
func (a Account) Project(w quota.Window, now time.Time) Heading {
	return a.project(w, now)
}

// Project is the document's project.
func (d Document) Project(a Account, w quota.Window, now time.Time) Heading {
	return d.project(a, w, now)
}

// ReserveHolds is reserveHolds.
func (d Document) ReserveHolds(a Account) bool {
	return d.reserveHolds(a)
}

// CapHolds is capHolds.
func (d Document) CapHolds(a Account, now time.Time, policy score.Policy) bool {
	return d.capHolds(a, now, policy)
}

// Pressed is pressureLine's words of doc's account a, as it stands at now,
// as policy judges it.
func (d Document) Pressed(a Account, now time.Time, policy score.Policy) string {
	an, _ := d.At(now, policy).Account(a.ID)
	pressed, _ := pressureLine(an.Pressure, now)
	return pressed
}

// UnderPressure is underPressure.
func (a Account) UnderPressure(now time.Time) bool {
	return a.underPressure(now)
}

// NotStarted is notStartedNote of the account with the given id, as the
// router primes it.
func (d Document) NotStarted(id string, now time.Time) string {
	return notStartedNote(d.primeOf(id), now)
}
