package status

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
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
func (a Account) Project(w quota.Window, now time.Time, policy score.Policy) Heading {
	return a.project(w, now, policy)
}

// Project is where the document's project has its account a's window w
// heading, its weeks played as withWeeks plays them.
func (d Document) Project(a Account, w quota.Window, now time.Time, policy score.Policy) Heading {
	return d.withWeeks(policy).project(a, w, now, policy).Heading
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

// TokenRefused is tokenRefused.
func (a Account) TokenRefused(now time.Time) bool {
	return a.tokenRefused(now)
}

// Floor is floor.
func (d Document) Floor(a Account) float64 {
	return d.floor(a)
}

// RunsOut is runOutOf, the document's weeks played as withWeeks plays them.
func (d Document) RunsOut(a Account, w quota.Window, now time.Time, policy score.Policy) (RunOut, bool) {
	return d.withWeeks(policy).runOutOf(a, w, now, policy)
}

// AsGiven is the document as JSON gives it: without the weeks its forecast
// played, which it keeps for its moments alone.
func (d Document) AsGiven() Document {
	d.weeks = nil
	return d
}

// CountForecasts has every document's weeks played as they are, counted,
// until the test ends: it returns the count so far.
func CountForecasts(t *testing.T) func() int {
	t.Helper()
	was, played := playWeeks, 0
	playWeeks = func(p forecast.Pool) map[string]forecast.Out {
		played++
		return was(p)
	}
	t.Cleanup(func() { playWeeks = was })
	return func() int { return played }
}

// Holds is the limit's holds.
func (l Limit) Holds(now time.Time) bool {
	return l.holds(now)
}

// Holds is the refusal's holds.
func (r Refusal) Holds(now time.Time) bool {
	return r.holds(now)
}

// Near is near.
const Near = near

// Projection is projection.
func Projection(now time.Time, w WindowNow) string {
	return projection(now, w)
}

// Reserved is reservedOf.
func (d Document) Reserved(a Account, now time.Time) string {
	return d.reservedOf(a, now)
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
