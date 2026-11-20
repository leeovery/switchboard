package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
)

// Length is how long the period the dashboard's entity tabs count their use
// over runs, as - and + step it, one dial they share, by its last day: today,
// that day from midnight; this week, the week holding it, from the day weeks
// start on; 30 days, that day and the 29 before it; 12 weeks, its week and
// the 11 before it; or 12 months, from the day after its date a year back,
// at now the days Trends' YEAR counts as LAST 12 MONTHS. Each is rolling,
// never a billing month, and priced whole while it's still running, but from
// its first day where the ledger began part-way through it. Its values are
// what the preferences keep.
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

// lengths are the lengths in the order - and + step through them, the
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

// Lengths are the lengths in the order - and + step through them, the
// shortest first.
func Lengths() []Length {
	return slices.Clone(lengths)
}

// LengthNames are the lengths' names, as Name has them, in the order - and
// + step through them, the shortest first.
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

// Stepped is the length - steps to, by -1, a shorter one, or + does, by 1, a
// longer one. It reports false past either end, where l stays: today, the
// shortest, and 12 months, the longest.
func (l Length) Stepped(by int) (Length, bool) {
	i := slices.Index(lengths, l) + by
	if i < 0 || i >= len(lengths) {
		return l, false
	}
	return lengths[i], true
}

// From is when the period of the length reaching today begins at now, weeks
// starting on the day given, as its period now's From has it.
func (l Length) From(now time.Time, starts time.Weekday) time.Time {
	return l.Now().From(now, starts)
}

// first is the first day of the period of the length whose last day is the
// day given, as dayfile.Civil gives a day, or of a week length, whose last
// week holds it, weeks starting on the day given.
func (l Length) first(day time.Time, starts time.Weekday) time.Time {
	switch l {
	case LengthToday:
		return day
	case LengthThirtyDays:
		return day.AddDate(0, 0, -29)
	case LengthTwelveWeeks:
		return weeksTo(day, starts, 12)
	case LengthTwelveMonths:
		return yearAfter(day)
	}
	return weekOf(day, starts)
}

// last is the last day of the period of the length whose last day is the
// day given, as dayfile.Civil gives a day, weeks starting on the day given:
// that day, of today and the 30 days and 12 months; the last of its week, of
// this week and the 12 weeks, as a week still running is priced to its end.
func (l Length) last(day time.Time, starts time.Weekday) time.Time {
	switch l {
	case LengthToday, LengthThirtyDays, LengthTwelveMonths:
		return day
	}
	return weekOf(day, starts).AddDate(0, 0, 6)
}

// after is the last day of the period of the length that begins the day
// after the day given, as dayfile.Civil gives one: a length on, a day, 7
// days, 30 or 84; or of 12 months, the day before the next day's date a
// year on. No 12 months begin on the 1 March after a 29 February, as
// yearAfter begins them: those after one end on the 28th a year on,
// beginning on the 29th.
func (l Length) after(day time.Time) time.Time {
	switch l {
	case LengthToday:
		return day.AddDate(0, 0, 1)
	case LengthThirtyDays:
		return day.AddDate(0, 0, 30)
	case LengthTwelveWeeks:
		return day.AddDate(0, 0, 7*12)
	case LengthTwelveMonths:
		return day.AddDate(0, 0, 1).AddDate(1, 0, -1)
	}
	return day.AddDate(0, 0, 7)
}

// endingOn is the period of the length whose last day is the day given, as
// dayfile.Civil gives one, or of a week length, whose last week holds it,
// today being the day given, weeks starting on the day given: gone where it
// ends before today; else now, as is every period that reaches today, a
// week length's where its last week holds today.
func (l Length) endingOn(day, today time.Time, starts time.Weekday) Period {
	if last := l.last(day, starts); last.Before(today) {
		return Period{length: l, last: last.Format(time.DateOnly)}
	}
	return l.Now()
}

// share is the share of a month the plans cost over a period of the length,
// whole.
func (l Length) share() monthPart {
	return lengthsAre[l].share
}

// Period is the period the dashboard's entity tabs count their use over:
// one of the dial's lengths, reaching today, or gone, ending before it, as
// < steps back to one. A period gone keeps its days as the clock moves on.
// It's comparable, for what a page was built of to be told alike.
type Period struct {
	length Length
	// last is the date of a period gone's last day, as time.DateOnly writes
	// it, a week length's its last week's last: "" while it reaches today.
	last string
}

// Now is the period of the length reaching today.
func (l Length) Now() Period {
	return Period{length: l}
}

// Length is how long the period runs.
func (p Period) Length() Length {
	return p.length
}

// Back is the period < steps back to at now, weeks starting on the day
// given: the period of its length ending the day before it begins, as from
// today, yesterday, and from 12 months, those to their last day's date a
// year back, a 29 February's the 28th, as yearAfter begins them. It reports
// false where the period begins on or before the day the work it counts
// began, at began, as the period holding that day, or lying before it, is
// the earliest < steps to; it steps back freely where began is zero, as
// where that isn't known.
func (p Period) Back(now time.Time, starts time.Weekday, began time.Time) (Period, bool) {
	today := dayfile.CivilOf(now)
	p = p.at(today, starts)
	s := p.spanAt(today, starts)
	if s.earliest(began) {
		return p, false
	}
	return p.length.endingOn(s.first.AddDate(0, 0, -1), today, starts), true
}

// On is the period > steps on to at now, weeks starting on the day given:
// the period of its length beginning the day after it ends, as after has
// it, now where that reaches today. It reports false at now, as nothing's
// later.
func (p Period) On(now time.Time, starts time.Weekday) (Period, bool) {
	today := dayfile.CivilOf(now)
	p = p.at(today, starts)
	s := p.spanAt(today, starts)
	if !s.gone {
		return p, false
	}
	return p.length.endingOn(p.length.after(s.last), today, starts), true
}

// Shorter is the period - steps to at now, weeks starting on the day given:
// of the next length shorter, ending on the period's last day, as endingOn
// has it, so from last week, its Sunday. It reports false at today, the
// shortest.
func (p Period) Shorter(now time.Time, starts time.Weekday) (Period, bool) {
	return p.lengthened(-1, dayfile.CivilOf(now), starts)
}

// Longer is the period + steps to at now, weeks starting on the day given:
// of the next length longer, ending on the period's last day, as endingOn
// has it, so from last week, the 30 days to its Sunday. It reports false at
// 12 months, the longest.
func (p Period) Longer(now time.Time, starts time.Weekday) (Period, bool) {
	return p.lengthened(1, dayfile.CivilOf(now), starts)
}

// lengthened is the period of the length Stepped steps the period's to by
// the count given, ending on the period's last day, as endingOn has it,
// today being the day given, as dayfile.Civil gives one, weeks starting on
// the day given. It reports false past either end.
func (p Period) lengthened(by int, today time.Time, starts time.Weekday) (Period, bool) {
	p = p.at(today, starts)
	length, ok := p.length.Stepped(by)
	if !ok {
		return p, false
	}
	return length.endingOn(p.spanAt(today, starts).last, today, starts), true
}

// at is the period on today, as dayfile.Civil gives a day, weeks starting
// on the day given: itself, but now where it reaches today, as endingOn has
// it, as a period gone does where the clock went back. Every step starts
// from it, and gives it back where it refuses.
func (p Period) at(today time.Time, starts time.Weekday) Period {
	if last, gone := p.ends(); gone {
		return p.length.endingOn(last, today, starts)
	}
	return p
}

// ends is the last day of the period gone, as dayfile.Civil gives a day,
// whatever today is; false while it reaches today.
func (p Period) ends() (time.Time, bool) {
	if p.last == "" {
		return time.Time{}, false
	}
	// last is written from a day, so it parses.
	day, _ := dayfile.Civil(p.last)
	return day, true
}

// periodSpan is a period's days on a day, today, as dayfile.Civil gives each:
// from first to last, a week length's last its last week's last, as a week
// still running is priced to its end; whether it's gone, ending before today;
// and how many weeks back its last week is from today's, none at now.
type periodSpan struct {
	first, last, today time.Time
	gone               bool
	weeksBack          int
}

// spanAt is the period's span on today, as dayfile.Civil gives a day, weeks
// starting on the day given: of a period gone, to its last day, but of now
// where it reaches today, as at has it; of now, to today. Every count of
// the period's days learns them here.
func (p Period) spanAt(today time.Time, starts time.Weekday) periodSpan {
	end, gone := p.at(today, starts).ends()
	if !gone {
		end = today
	}
	last := p.length.last(end, starts)
	return periodSpan{
		first: p.length.first(end, starts), last: last, today: today, gone: gone,
		weeksBack: int(weekOf(today, starts).Sub(weekOf(last, starts)) / (7 * 24 * time.Hour)),
	}
}

// after is the span of the days after s's last, to today: none of a span at
// now.
func (s periodSpan) after() periodSpan {
	return periodSpan{first: s.last.AddDate(0, 0, 1), last: s.today, today: s.today}
}

// earliest reports whether the span begins on or before the day the work it
// counts began, at began, as beganOn has it: the earliest < steps back to.
// It never does where began is zero, as where that isn't known.
func (s periodSpan) earliest(began time.Time) bool {
	return !began.IsZero() && s.first.Format(time.DateOnly) <= beganOn(began)
}

// From is when the period begins, weeks starting on the day given, today
// being now's: at the start of its first local day, as dayfile.Day has it,
// where a read of its days begins, as ReadSessionDays takes it.
func (p Period) From(now time.Time, starts time.Weekday) time.Time {
	start, _, _ := dayfile.Day(p.spanAt(dayfile.CivilOf(now), starts).first.Format(time.DateOnly))
	return start
}

// Until is when the period gone ends, weeks starting on the day given,
// today being now's: at the end of its last local day, as dayfile.Day has
// it, where what's told of it stops. It reports false while the period
// reaches today, as what's told of it then runs to now.
func (p Period) Until(now time.Time, starts time.Weekday) (time.Time, bool) {
	s := p.spanAt(dayfile.CivilOf(now), starts)
	if !s.gone {
		return time.Time{}, false
	}
	_, end, _ := dayfile.Day(s.last.Format(time.DateOnly))
	return end, true
}

// shape is the period as one of Tokens' rows, on today, as dayfile.Civil
// gives a day, weeks starting on the day given: every day of its span one
// row, by its first day's date, priced as Accounts prices the period, its
// share of a month whole.
func (p Period) shape(today time.Time, starts time.Weekday) periodShape {
	s := p.spanAt(today, starts)
	first, last := s.first.Format(time.DateOnly), s.last.Format(time.DateOnly)
	return periodShape{
		of:    func(date string) (string, bool) { return first, first <= date && date <= last },
		dates: func(string) (string, string) { return first, last },
		share: p.length.share(),
	}
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
