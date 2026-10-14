package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// AccountsPeriod is the period Accounts counts its accounts' use over, Compare
// and Detail alike, as < and > step it: today, from midnight; this week, as
// it opens, from the day weeks start on; the last 30 days, today and the 29
// before it; or the last 12 weeks, this week and the 11 before it. Each is
// rolling, never a billing month, and priced whole while it's still running.
type AccountsPeriod string

const (
	// AccountsThisWeek is this week, from the day weeks start on.
	AccountsThisWeek AccountsPeriod = ""
	// AccountsToday is today, from midnight.
	AccountsToday AccountsPeriod = "today"
	// AccountsThirtyDays is today and the 29 days before it.
	AccountsThirtyDays AccountsPeriod = "30-days"
	// AccountsTwelveWeeks is this week and the 11 before it.
	AccountsTwelveWeeks AccountsPeriod = "12-weeks"
)

// accountsPeriods are the periods in the order < and > step through them,
// the shortest first.
var accountsPeriods = []AccountsPeriod{AccountsToday, AccountsThisWeek, AccountsThirtyDays, AccountsTwelveWeeks}

// accountsPeriod is what a period is: what the heading and the key line call
// it; THIS WEEK's title, naming it; the unit against its price names it by;
// and the share of a month its plans cost over it.
type accountsPeriod struct {
	name, title, unit string
	share             monthPart
}

// accountsPeriodsAre are each period's words and price.
var accountsPeriodsAre = map[AccountsPeriod]accountsPeriod{
	AccountsToday:       {name: "today", title: "TODAY", unit: "day", share: days(1)},
	AccountsThisWeek:    {name: "this week", title: "THIS WEEK", unit: "week", share: aWeek},
	AccountsThirtyDays:  {name: "30 days", title: "LAST 30 DAYS", unit: "month", share: aMonth},
	AccountsTwelveWeeks: {name: "12 weeks", title: "LAST 12 WEEKS", unit: "12 weeks", share: twelveWeeks},
}

// Known reports whether p is one of the periods, as a preference read back
// may not be.
func (p AccountsPeriod) Known() bool {
	_, ok := accountsPeriodsAre[p]
	return ok
}

// Name is what the heading and the key line call the period: "today", "this
// week", "30 days" or "12 weeks".
func (p AccountsPeriod) Name() string {
	return accountsPeriodsAre[p].name
}

// Title is THIS WEEK's title, naming the period: "TODAY", "THIS WEEK",
// "LAST 30 DAYS" or "LAST 12 WEEKS".
func (p AccountsPeriod) Title() string {
	return accountsPeriodsAre[p].title
}

// Unit is what against its price names the period by, after the price, as
// in "its $46 week": "day", "week", "month" or "12 weeks".
func (p AccountsPeriod) Unit() string {
	return accountsPeriodsAre[p].unit
}

// Shorter is the period < steps to, reporting false at today, the shortest,
// where it stays.
func (p AccountsPeriod) Shorter() (AccountsPeriod, bool) {
	return p.stepped(-1)
}

// Longer is the period > steps to, reporting false at the last 12 weeks, the
// longest, where it stays.
func (p AccountsPeriod) Longer() (AccountsPeriod, bool) {
	return p.stepped(1)
}

// stepped is the period by places on from p, reporting false past either
// end, where p stands.
func (p AccountsPeriod) stepped(by int) (AccountsPeriod, bool) {
	i := slices.Index(accountsPeriods, p) + by
	if i < 0 || i >= len(accountsPeriods) {
		return p, false
	}
	return accountsPeriods[i], true
}

// From is when the period begins at now, weeks starting on the day given: at
// the start of its first local day, this week's where WeekStart has it.
func (p AccountsPeriod) From(now time.Time, starts time.Weekday) time.Time {
	if p == AccountsThisWeek {
		return WeekStart(now.Local(), starts)
	}
	start, _, _ := dayfile.Day(p.first(dayfile.CivilOf(now), starts).Format(time.DateOnly))
	return start
}

// first is the period's first day, today being the day given, as
// dayfile.Civil gives a day, weeks starting on the day given.
func (p AccountsPeriod) first(today time.Time, starts time.Weekday) time.Time {
	switch p {
	case AccountsToday:
		return today
	case AccountsThirtyDays:
		return today.AddDate(0, 0, -29)
	case AccountsTwelveWeeks:
		return weeksTo(today, starts, 12)
	}
	return weekOf(today, starts)
}

// share is the share of a month the plans cost over the period, whole.
func (p AccountsPeriod) share() monthPart {
	return accountsPeriodsAre[p].share
}
