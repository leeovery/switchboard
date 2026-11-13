package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
)

// Period is the period the dashboard's entity tabs count their use over, as
// < and > step it, one dial they share: today, from midnight; this week, as
// it opens, from the day weeks start on; the last 30 days, today and the 29
// before it; the last 12 weeks, this week and the 11 before it; or the last
// 12 months, the days History's Year counts as LAST 12 MONTHS. Each is
// rolling, never a billing month, and priced whole while it's still running,
// but from its first day where the ledger began part-way through it.
type Period string

const (
	// PeriodThisWeek is this week, from the day weeks start on.
	PeriodThisWeek Period = ""
	// PeriodToday is today, from midnight.
	PeriodToday Period = "today"
	// PeriodThirtyDays is today and the 29 days before it.
	PeriodThirtyDays Period = "30-days"
	// PeriodTwelveWeeks is this week and the 11 before it.
	PeriodTwelveWeeks Period = "12-weeks"
	// PeriodTwelveMonths is today and the days back to the day after its
	// date a year back, as yearAfter has them.
	PeriodTwelveMonths Period = "12-months"
)

// periods are the periods in the order < and > step through them, the
// shortest first.
var periods = []Period{PeriodToday, PeriodThisWeek, PeriodThirtyDays, PeriodTwelveWeeks, PeriodTwelveMonths}

// periodIs is what a period is: what the heading and the key line call it;
// THIS WEEK's title, naming it; the unit against its price names it by; and
// the share of a month its plans cost over it.
type periodIs struct {
	name, title, unit string
	share             monthPart
}

// periodsAre are each period's words and price.
var periodsAre = map[Period]periodIs{
	PeriodToday:        {name: "today", title: "TODAY", unit: "day", share: days(1)},
	PeriodThisWeek:     {name: "this week", title: "THIS WEEK", unit: "week", share: aWeek},
	PeriodThirtyDays:   {name: "30 days", title: "LAST 30 DAYS", unit: "month", share: aMonth},
	PeriodTwelveWeeks:  {name: "12 weeks", title: "LAST 12 WEEKS", unit: "12 weeks", share: twelveWeeks},
	PeriodTwelveMonths: {name: "12 months", title: "LAST 12 MONTHS", unit: "12 months", share: aYear},
}

// Periods are the periods in the order < and > step through them, the
// shortest first.
func Periods() []Period {
	return slices.Clone(periods)
}

// PeriodNames are the periods' names, as Name has them, in the order < and
// > step through them, the shortest first.
func PeriodNames() []string {
	names := make([]string, len(periods))
	for i, p := range periods {
		names[i] = p.Name()
	}
	return names
}

// Known reports whether p is one of the periods, as a preference read back
// may not be.
func (p Period) Known() bool {
	_, ok := periodsAre[p]
	return ok
}

// Name is what the heading and the key line call the period: "today", "this
// week", "30 days", "12 weeks" or "12 months".
func (p Period) Name() string {
	return periodsAre[p].name
}

// Title is THIS WEEK's title, naming the period: "TODAY", "THIS WEEK",
// "LAST 30 DAYS", "LAST 12 WEEKS" or "LAST 12 MONTHS".
func (p Period) Title() string {
	return periodsAre[p].title
}

// Unit is what against its price names the period by, after the price, as
// in "its $46 week": "day", "week", "month", "12 weeks" or "12 months".
func (p Period) Unit() string {
	return periodsAre[p].unit
}

// Stepped is the period < steps to, by -1, a shorter one, or > does, by 1, a
// longer one. It reports false past either end, where p stays: today, the
// shortest, and the last 12 months, the longest.
func (p Period) Stepped(by int) (Period, bool) {
	i := slices.Index(periods, p) + by
	if i < 0 || i >= len(periods) {
		return p, false
	}
	return periods[i], true
}

// From is when the period begins at now, weeks starting on the day given:
// at the start of its first local day, as dayfile.Day has it, where a read
// of its days begins, as ReadSessionDays takes it.
func (p Period) From(now time.Time, starts time.Weekday) time.Time {
	start, _, _ := dayfile.Day(p.first(dayfile.CivilOf(now), starts).Format(time.DateOnly))
	return start
}

// first is the period's first day, today being the day given, as
// dayfile.Civil gives a day, weeks starting on the day given.
func (p Period) first(today time.Time, starts time.Weekday) time.Time {
	switch p {
	case PeriodToday:
		return today
	case PeriodThirtyDays:
		return today.AddDate(0, 0, -29)
	case PeriodTwelveWeeks:
		return weeksTo(today, starts, 12)
	case PeriodTwelveMonths:
		return yearAfter(today)
	}
	return weekOf(today, starts)
}

// last is the period's last day, today being the day given, as
// dayfile.Civil gives a day, weeks starting on the day given: today, of
// today and the last 30 days and 12 months; this week's last, of this week
// and the last 12 weeks, as a period still running is priced to its end.
func (p Period) last(today time.Time, starts time.Weekday) time.Time {
	switch p {
	case PeriodToday, PeriodThirtyDays, PeriodTwelveMonths:
		return today
	}
	return weekOf(today, starts).AddDate(0, 0, 6)
}

// share is the share of a month the plans cost over the period, whole.
func (p Period) share() monthPart {
	return periodsAre[p].share
}

// LimitsHitMost are the most times the limit of the window with the key
// given can be hit over the longest period the dial steps to, as a limit is
// hit once a window at most: as many windows as the period's span can touch,
// those it holds whole and a part of one at each end, of the window's length
// as quota.Length reads it from the key, or an hour's where the key doesn't
// say it. So 1,758 of a 5-hour window over 12 months of 366 days, and 54 of
// a week's.
func LimitsHitMost(key string) int {
	length, ok := quota.Length(key)
	if !ok {
		length = time.Hour
	}
	span := longestSpan()
	return int((span+length-1)/length) + 1
}

// longestSpan is how long the longest period the dial steps to runs, from
// its first day to the end of its last, as first and last have them of a
// day whose last 12 months hold a 29 February, the most days they hold,
// weeks starting on any weekday.
func longestSpan() time.Duration {
	day := time.Date(2028, time.March, 1, 0, 0, 0, 0, time.UTC)
	var longest time.Duration
	for _, p := range periods {
		longest = max(longest, p.last(day, time.Monday).AddDate(0, 0, 1).Sub(p.first(day, time.Monday)))
	}
	return longest
}
