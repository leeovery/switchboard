package views

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
)

// Dial is the dial's period as it's said at a moment, on the heading's right,
// in the key line and in titles, each word of it worked out here once.
type Dial struct {
	length              Length
	span                periodSpan
	now                 time.Time
	name, title, within string
	earliest            bool
}

// DialOf is the dial's period given as it's said at now, weeks starting on
// the day given, the work it counts having begun at began, zero where that
// isn't known: at now by its length's words; gone, as goneSaid has it.
func DialOf(period Period, now time.Time, starts time.Weekday, began time.Time) Dial {
	s := period.spanAt(dayfile.CivilOf(now), starts)
	l := period.length
	d := Dial{length: l, span: s, now: now, name: l.Name(), title: l.Title(), within: l.Within(), earliest: s.earliest(began)}
	if s.gone {
		d.name, d.within = l.goneSaid(now, s)
		d.title = strings.ToUpper(d.name)
	}
	return d
}

// goneSaid are the words a period of the length gone, of the span given, is
// said in at now: its name, as the dial gives it, and within, as in "no
// sessions yesterday". A day is yesterday, the day before today, else its
// date, as Tue 29 Sep, said on it; a week last week, the week before
// today's, else the week of its first day, as week of 14 Sep; and the longer
// lengths their name to their last day, as 30 days to 1 Sep; each but
// yesterday and last week said in the period so named. A date carries its
// year where that isn't now's, as prose's At forms name it.
func (l Length) goneSaid(now time.Time, s periodSpan) (name, within string) {
	switch l {
	case LengthToday:
		if s.last.AddDate(0, 0, 1).Equal(s.today) {
			return "yesterday", "yesterday"
		}
		day := prose.DayAt(now, s.last)
		return day, "on " + day
	case LengthThisWeek:
		if s.weeksBack == 1 {
			return "last week", "last week"
		}
		name = "week of " + prose.DayMonthAt(now, s.first)
	default:
		name = l.Name() + " to " + prose.DayMonthAt(now, s.last)
	}
	return name, "in the " + name
}

// Name is what the heading and the key line call the period: at now its
// length's name, as "this week"; gone, as goneSaid names it, as "last week"
// or "30 days to 1 Sep".
func (d Dial) Name() string {
	return d.name
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
// began, or begins before it, where < has nothing earlier to step back to.
func (d Dial) Earliest() bool {
	return d.earliest
}

// Widest is the widest name the period's length takes at any step, as
// widestSaid has it, which the heading keeps room for, so stepping back and
// on never moves it.
func (d Dial) Widest() string {
	return d.length.widestSaid()
}

// widestSaid is the widest name a period of the length takes, as Name gives
// it: its name at now, one step back, or gone further, by a date of two
// figures in another year than now's, the widest a date's said.
func (l Length) widestSaid() string {
	now := time.Date(2001, time.March, 28, 12, 0, 0, 0, time.Local)
	today := dayfile.CivilOf(now)
	back, _ := l.Now().Back(today, time.Monday, time.Time{})
	dated := l.endingOn(time.Date(2000, time.January, 19, 0, 0, 0, 0, time.UTC), today, time.Monday)
	widest := l.Name()
	for _, p := range []Period{back, dated} {
		if name := DialOf(p, now, time.Monday, time.Time{}).Name(); utf8.RuneCountInString(name) > utf8.RuneCountInString(widest) {
			widest = name
		}
	}
	return widest
}

// TimeSaid says when t, a time in the period, was, as Projects' last says a
// project's last request: at now, as prose.Lately says it; gone, never as
// now, but over a period of one day by its time of day alone, as "21:40";
// over more, with its weekday where it falls in the period's last seven
// days, as "Sun 21:40", else with its date, as "21 Aug 21:40". No period
// holds a date twice, so a date needs no year.
func (d Dial) TimeSaid(t time.Time) string {
	switch {
	case !d.span.gone:
		return prose.Lately(d.now, t)
	case d.span.first.Equal(d.span.last):
		return prose.TimeOfDay(d.now, t)
	case d.span.last.Sub(dayfile.CivilOf(t)) < 7*24*time.Hour:
		return prose.Clock(d.now, t)
	}
	return prose.DayMonthTime(d.now, t)
}
