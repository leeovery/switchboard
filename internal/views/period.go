package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
)

// Length is how long the period the dashboard's entity tabs count their use
// over runs, as < and > step it, one dial they share: today, from midnight;
// this week, as it opens, from the day weeks start on; the last 30 days,
// today and the 29 before it; the last 12 weeks, this week and the 11 before
// it; or the last 12 months, the days Trends' YEAR counts as LAST 12 MONTHS.
// Each is rolling, never a billing month, and priced whole while it's still
// running, but from its first day where the ledger began part-way through
// it. Its values are what the preferences keep.
type Length string

const (
	// LengthThisWeek is this week, from the day weeks start on.
	LengthThisWeek Length = ""
	// LengthToday is today, from midnight.
	LengthToday Length = "today"
	// LengthThirtyDays is today and the 29 days before it.
	LengthThirtyDays Length = "30-days"
	// LengthTwelveWeeks is this week and the 11 before it.
	LengthTwelveWeeks Length = "12-weeks"
	// LengthTwelveMonths is today and the days back to the day after its
	// date a year back, as yearAfter has them.
	LengthTwelveMonths Length = "12-months"
)

// lengths are the lengths in the order < and > step through them, the
// shortest first.
var lengths = []Length{LengthToday, LengthThisWeek, LengthThirtyDays, LengthTwelveWeeks, LengthTwelveMonths}

// lengthIs is what a length is: what the heading and the key line call it;
// THIS WEEK's title, naming it; the unit against its price names it by; the
// words Within says it in; the share of a month its plans cost over it; and
// the period each of Accounts' Tokens' rows is over it.
type lengthIs struct {
	name, title, unit, within string
	share                     monthPart
	rows                      TokensPeriod
}

// lengthsAre are each length's words and price.
var lengthsAre = map[Length]lengthIs{
	LengthToday:        {name: "today", title: "TODAY", unit: "day", within: "today", share: days(1), rows: TokensByDay},
	LengthThisWeek:     {name: "this week", title: "THIS WEEK", unit: "week", within: "this week", share: aWeek, rows: TokensByDay},
	LengthThirtyDays:   {name: "30 days", title: "LAST 30 DAYS", unit: "month", within: "in the last 30 days", share: aMonth, rows: TokensByDay},
	LengthTwelveWeeks:  {name: "12 weeks", title: "LAST 12 WEEKS", unit: "12 weeks", within: "in the last 12 weeks", share: twelveWeeks, rows: TokensByWeek},
	LengthTwelveMonths: {name: "12 months", title: "LAST 12 MONTHS", unit: "12 months", within: "in the last 12 months", share: aYear, rows: TokensByMonth},
}

// Lengths are the lengths in the order < and > step through them, the
// shortest first.
func Lengths() []Length {
	return slices.Clone(lengths)
}

// LengthNames are the lengths' names, as Name has them, in the order < and
// > step through them, the shortest first.
func LengthNames() []string {
	names := make([]string, len(lengths))
	for i, l := range lengths {
		names[i] = l.Name()
	}
	return names
}

// Known reports whether l is one of the lengths, as a preference read back
// may not be.
func (l Length) Known() bool {
	_, ok := lengthsAre[l]
	return ok
}

// Name is what the heading and the key line call the length: "today", "this
// week", "30 days", "12 weeks" or "12 months".
func (l Length) Name() string {
	return lengthsAre[l].name
}

// Title is THIS WEEK's title, naming the length: "TODAY", "THIS WEEK",
// "LAST 30 DAYS", "LAST 12 WEEKS" or "LAST 12 MONTHS".
func (l Length) Title() string {
	return lengthsAre[l].title
}

// Within is the length said after what's in it, as in "no sessions this
// week": "today", "this week", or "in the last 30 days", "in the last 12
// weeks" or "in the last 12 months".
func (l Length) Within() string {
	return lengthsAre[l].within
}

// Unit is what against its price names the length by, after the price, as
// in "its $46 week": "day", "week", "month", "12 weeks" or "12 months".
func (l Length) Unit() string {
	return lengthsAre[l].unit
}

// Rows is the period each of Accounts' Tokens' rows is over the length: a
// day over today, this week and 30 days, a week over 12 weeks, and a month
// over 12 months.
func (l Length) Rows() TokensPeriod {
	return lengthsAre[l].rows
}

// Stepped is the length < steps to, by -1, a shorter one, or > does, by 1, a
// longer one. It reports false past either end, where l stays: today, the
// shortest, and the last 12 months, the longest.
func (l Length) Stepped(by int) (Length, bool) {
	i := slices.Index(lengths, l) + by
	if i < 0 || i >= len(lengths) {
		return l, false
	}
	return lengths[i], true
}

// From is when the period of the length reaching today begins at now, weeks
// starting on the day given: at the start of its first local day, as
// dayfile.Day has it, where a read of its days begins, as ReadSessionDays
// takes it.
func (l Length) From(now time.Time, starts time.Weekday) time.Time {
	start, _, _ := dayfile.Day(l.first(dayfile.CivilOf(now), starts).Format(time.DateOnly))
	return start
}

// first is the first day of the period of the length reaching today, today
// being the day given, as dayfile.Civil gives a day, weeks starting on the
// day given.
func (l Length) first(today time.Time, starts time.Weekday) time.Time {
	switch l {
	case LengthToday:
		return today
	case LengthThirtyDays:
		return today.AddDate(0, 0, -29)
	case LengthTwelveWeeks:
		return weeksTo(today, starts, 12)
	case LengthTwelveMonths:
		return yearAfter(today)
	}
	return weekOf(today, starts)
}

// last is the last day of the period of the length reaching today, today
// being the day given, as dayfile.Civil gives a day, weeks starting on the
// day given: today, of today and the last 30 days and 12 months; this
// week's last, of this week and the last 12 weeks, as a period still running
// is priced to its end.
func (l Length) last(today time.Time, starts time.Weekday) time.Time {
	switch l {
	case LengthToday, LengthThirtyDays, LengthTwelveMonths:
		return today
	}
	return weekOf(today, starts).AddDate(0, 0, 6)
}

// shape is the period of the length reaching today as one of Tokens' rows,
// today being the day given, as dayfile.Civil gives a day, weeks starting on
// the day given: every day from its first to its last one row, by its first
// day's date, priced as Accounts prices the period, its share of a month
// whole.
func (l Length) shape(today time.Time, starts time.Weekday) periodShape {
	first, last := l.first(today, starts).Format(time.DateOnly), l.last(today, starts).Format(time.DateOnly)
	return periodShape{
		of:    func(date string) (string, bool) { return first, first <= date && date <= last },
		dates: func(string) (string, string) { return first, last },
		share: l.share(),
	}
}

// share is the share of a month the plans cost over a period of the length,
// whole.
func (l Length) share() monthPart {
	return lengthsAre[l].share
}

// Period is the period the dashboard's entity tabs count their use over:
// one of the dial's lengths, reaching today. It's comparable, for what a
// page was built of to be told alike.
type Period struct {
	length Length
}

// Now is the period of the length reaching today.
func (l Length) Now() Period {
	return Period{length: l}
}

// Length is how long the period runs.
func (p Period) Length() Length {
	return p.length
}

// From is when the period begins at now, weeks starting on the day given,
// as its length's From has it.
func (p Period) From(now time.Time, starts time.Weekday) time.Time {
	return p.length.From(now, starts)
}

// firstDay is the period's first day, today being the day given, as its
// length's first has it.
func (p Period) firstDay(today time.Time, starts time.Weekday) time.Time {
	return p.length.first(today, starts)
}

// lastDay is the period's last day, today being the day given, as its
// length's last has it.
func (p Period) lastDay(today time.Time, starts time.Weekday) time.Time {
	return p.length.last(today, starts)
}

// shape is the period as one of Tokens' rows, today being the day given, as
// its length's shape has it.
func (p Period) shape(today time.Time, starts time.Weekday) periodShape {
	return p.length.shape(today, starts)
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
	for _, l := range lengths {
		longest = max(longest, l.last(day, time.Monday).AddDate(0, 0, 1).Sub(l.first(day, time.Monday)))
	}
	return longest
}
