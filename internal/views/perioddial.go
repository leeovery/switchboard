package views

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
)

// Dial is the dial's period as it's said at a moment: in the heading's
// readout, by its range between - and + and when it is between < and >; and
// in titles and sentences, by its name; each word of it worked out here
// once.
type Dial struct {
	span periodSpan
	// local is now in the local time zone, the one the period's days are
	// reckoned in, as dayfile.CivilOf reckons them, for its times to be told
	// on the days they fall on.
	local                            time.Time
	length                           Length
	name, short, title, within, when string
	earliest                         bool
}

// DialOf is the dial's period given as it's said at now, weeks starting on
// the day given, the work it counts having begun at began, zero where that
// isn't known: at now by its length's words; gone, as goneWhen and goneSaid
// have it.
func DialOf(period Period, now time.Time, starts time.Weekday, began time.Time) Dial {
	s := period.spanAt(dayfile.CivilOf(now), starts)
	l := period.length
	d := Dial{
		span: s, local: now.Local(), length: l, name: l.Name(), short: l.Name(), title: l.Title(), within: l.Within(),
		when: lengthsAre[l].now, earliest: s.earliest(began),
	}
	if s.gone {
		d.when = l.goneWhen(s)
		d.name, d.within = l.goneSaid(s, d.when)
		d.short, d.title = l.goneShort(s, d.name), strings.ToUpper(d.name)
	}
	return d
}

// goneWhen is when a period of the length gone, of the span given, is, as
// the heading's readout says it between < and >: a day yesterday, the day
// before today, else its date, as Tue 29 Sep; a week last week, the week
// before today's, else its days, as 14–20 Sep; and the longer lengths to
// their last day, as to 1 Sep. A date carries its year where it isn't
// today's, as prose's At forms name it.
func (l Length) goneWhen(s periodSpan) string {
	switch l {
	case LengthToday:
		if s.yesterday() {
			return "yesterday"
		}
		return prose.DayAt(s.today, s.last)
	case LengthThisWeek:
		if s.weeksBack == 1 {
			return "last week"
		}
		return prose.DaysAt(s.today, s.first, s.last)
	}
	return "to " + prose.DayMonthAt(s.today, s.last)
}

// goneSaid are the words a period of the length gone, of the span given, is
// said in, when it is as given, as goneWhen has it: its name, as titles and
// sentences give it, and within, as in "no sessions yesterday". A day and
// last week are named by when alone; another week by its first day, as week
// of 14 Sep; and the longer lengths by their name before when, as 30 days to
// 1 Sep; each but yesterday and last week said in the period so named.
func (l Length) goneSaid(s periodSpan, when string) (name, within string) {
	switch {
	case l == LengthToday && s.yesterday(), l == LengthThisWeek && s.weeksBack == 1:
		return when, when
	case l == LengthToday:
		return when, "on " + when
	case l == LengthThisWeek:
		name = "week of " + prose.DayMonthAt(s.today, s.first)
	default:
		name = l.Name() + " " + when
	}
	return name, "in the " + name
}

// yesterday reports whether the span's last day is the day before today.
func (s periodSpan) yesterday() bool {
	return s.last.AddDate(0, 0, 1).Equal(s.today)
}

// goneShort is the short name of a period of the length gone, of the span
// given, named as given: yesterday, or a day in today's year, its name
// itself, as Tue 29 Sep, and a date in another its date with its year, as
// 31 Dec 2025, without its weekday; the week, never this week; and the
// longer lengths their own name, as 30 days.
func (l Length) goneShort(s periodSpan, name string) string {
	switch l {
	case LengthToday:
		if !s.yesterday() && s.last.Year() != s.today.Year() {
			return prose.DayMonthYear(s.last)
		}
		return name
	case LengthThisWeek:
		return "the week"
	}
	return l.Name()
}

// Range is the period's length as the heading's readout names it between -
// and +, which change it alone: "day", "week", "30 days", "12 weeks" or "12
// months".
func (d Dial) Range() string {
	return lengthsAre[d.length].ranged
}

// When is when the period is, as the heading's readout says it between <
// and >, which change it alone: at now "today", "this week", or of the
// longer lengths "now"; gone, as goneWhen says it, as "yesterday", "14–20
// Sep" or "to 1 Sep".
func (d Dial) When() string {
	return d.when
}

// Name is what a sentence or a title calls the period, as Tokens' total row
// and the List's period line name it: at now its length's name, as "this
// week"; gone, as goneSaid names it, as "last week" or "30 days to 1 Sep".
func (d Dial) Name() string {
	return d.name
}

// Short is the period's name where its own is too wide, as Tokens' total
// row names it then, the heading naming it in full: at now its length's
// name, as Name is; gone, never one that says now (today, this week): a
// day's name, as "yesterday", or in another year its date, as "31 Dec 2025";
// "the week" of a week; and the longer lengths' own names, as "30 days".
func (d Dial) Short() string {
	return d.short
}

// Title is the period's name in capitals, as Detail's figures and Tokens'
// first row title it: at now its length's title, as "LAST 30 DAYS"; gone,
// its name's, as "30 DAYS TO 1 SEP".
func (d Dial) Title() string {
	return d.title
}

// Within is the period said after what's in it, as in "no sessions this
// week": at now as its length's Within says it; gone, as goneSaid says it,
// as "yesterday", "on Tue 29 Sep" or "in the week of 14 Sep".
func (d Dial) Within() string {
	return d.within
}

// Gone reports whether the period is gone, ending before today, as < steps
// back to one.
func (d Dial) Gone() bool {
	return d.span.gone
}

// AtNow reports whether the period reaches today, where > has nothing later
// to step on to.
func (d Dial) AtNow() bool {
	return !d.span.gone
}

// Earliest reports whether the period holds the day the work it counts
// began, or lies before it, where < has nothing earlier to step back to.
func (d Dial) Earliest() bool {
	return d.earliest
}

// Shortest reports whether the period is of the shortest length, where -
// has nothing shorter to step to.
func (d Dial) Shortest() bool {
	_, shorter := d.length.Stepped(-1)
	return !shorter
}

// Longest reports whether the period is of the longest length, where + has
// nothing longer to step to.
func (d Dial) Longest() bool {
	_, longer := d.length.Stepped(1)
	return !longer
}

// Widest are the range and when of the widest readout the heading keeps
// room for, as widestNow and widestGone have them: at now, the widest of
// every length's at now, so no step at now moves the heading; gone, the
// widest of every period gone, so no step back or on moves it.
func (d Dial) Widest() (ranged, when string) {
	if d.span.gone {
		return widestGone()
	}
	return widestNow()
}

// widestNow are the range and when of the widest of every length's readout
// at now, "week" and "this week".
func widestNow() (ranged, when string) {
	for _, l := range lengths {
		ranged, when = wider(ranged, when, lengthsAre[l].ranged, lengthsAre[l].now)
	}
	return ranged, when
}

// widestGone are the range and when of the widest readout of a period gone:
// of each length's ending on every day of two years, a year's end between
// them, said on a day of a later year, so both carry their years. Weeks
// start on Monday: a week over a year's end begins on a day of two figures
// whichever day weeks start on, so the widths don't hang on it.
var widestGone = sync.OnceValues(func() (ranged, when string) {
	from, today := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)
	for _, l := range lengths {
		for day := from; day.Year() < 2026; day = day.AddDate(0, 0, 1) {
			s := l.endingOn(day, today, time.Monday).spanAt(today, time.Monday)
			ranged, when = wider(ranged, when, lengthsAre[l].ranged, l.goneWhen(s))
		}
	}
	return ranged, when
})

// wider is the range and when of the readout that takes more cells, of the
// one given first and the other, the first where they tie.
func wider(ranged, when, otherRanged, otherWhen string) (string, string) {
	if cells(otherRanged)+cells(otherWhen) > cells(ranged)+cells(when) {
		return otherRanged, otherWhen
	}
	return ranged, when
}

// cells is how many cells the words take, a cell a rune, as every word the
// readout says is.
func cells(words string) int {
	return utf8.RuneCountInString(words)
}

// TimeSaid says when t, a time in the period, was, as Projects' last says a
// project's last request, told on the local day it falls on: at now, as
// prose.Lately says it; gone, never as now, but over a period of one day by
// its time of day alone, as "21:40"; over more, with its weekday where it
// falls in the period's last seven days, as "Sun 21:40", else with its date,
// as "21 Aug 21:40", and in a year other than now's with it, as "21 Aug 2025
// 21:40", as a date carries its year wherever it isn't now's.
func (d Dial) TimeSaid(t time.Time) string {
	switch {
	case !d.span.gone:
		return prose.Lately(d.local, t)
	case d.span.first.Equal(d.span.last):
		return prose.TimeOfDay(d.local, t)
	case d.span.last.Sub(dayfile.CivilOf(t)) < 7*24*time.Hour:
		return prose.Clock(d.local, t)
	}
	return prose.DayMonthTimeAt(d.local, t)
}
