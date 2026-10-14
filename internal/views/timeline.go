package views

import (
	"math/bits"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
)

// RunwayWindow is the window Runway's view is named for, as < and > step
// between them: the 5-hour, the first the dashboard shows, or the week.
type RunwayWindow string

const (
	// RunwayFiveHour is the 5-hour view.
	RunwayFiveHour RunwayWindow = ""
	// RunwayWeek is the week view.
	RunwayWeek RunwayWindow = "week"
)

// The live views' spans: the 5-hour's, from the hour before now, 22 hours
// 40 minutes, as 136 columns of 10 minutes; the week's, from a day before
// now, a week of 10,080 minutes; each start taken back to its ten minutes.
const (
	fiveHourBefore = time.Hour
	fiveHourSpan   = 22*time.Hour + 40*time.Minute
	weekBefore     = quota.Day
	weekSpan       = 7 * quota.Day
	tenMinutes     = 10 * time.Minute
)

// Timeline is the time Runway's lanes show across Columns cells, from Start
// up to End, the span cut evenly among them to the nanosecond, as Bounds has
// it. Now is now, where the timeline holds it: zero on a day, or a week,
// stepped back to that has gone, which has none.
type Timeline struct {
	Start, End time.Time
	Columns    int
	Now        time.Time
	// first and last are the first and the last day a timeline stepped back
	// to shows, as dayfile.Civil gives a day: zero of a live one.
	first, last time.Time
}

// LiveTimeline is the live view of the window w at now, across columns
// cells: the 5-hour's from the hour before now, for 22 hours 40 minutes; the
// week's from a day before now, for a week; each from its ten minutes, as
// onItsTenMinutes takes it back.
func LiveTimeline(w RunwayWindow, now time.Time, columns int) Timeline {
	before, span := fiveHourBefore, fiveHourSpan
	if w == RunwayWeek {
		before, span = weekBefore, weekSpan
	}
	start := onItsTenMinutes(now.Add(-before))
	return Timeline{Start: start, End: start.Add(span), Columns: columns, Now: now}
}

// Stepped is the window w's view stepped back to the day given, as
// dayfile.Civil gives one, at now, across columns cells: of the 5-hour, that
// day, from its midnight to the next, as the clocks put them, 23 or 25 hours
// where they change; of the week, the week it falls in, weeks starting on
// the day given, from its first day's midnight for seven days. It holds now
// where now falls in it, as on today and this week. The day is kept, not
// how many steps back it is, so a day stepped back to stays itself as the
// clock passes midnight. A day given zero is the live view's.
func Stepped(w RunwayWindow, day, now time.Time, starts time.Weekday, columns int) Timeline {
	if day.IsZero() {
		return LiveTimeline(w, now, columns)
	}
	first, days := day, 1
	if w == RunwayWeek {
		first, days = weekOf(day, starts), 7
	}
	start := dayStartOf(first, now.Location())
	tl := Timeline{Start: start, End: dayfile.DayStart(start, days), Columns: columns, first: first, last: first.AddDate(0, 0, days-1)}
	if tl.holds(now) {
		tl.Now = now
	}
	return tl
}

// onItsTenMinutes is t taken back to its ten minutes by its clock, as from
// its day's start, as dayfile.DayStart gives it, so a zone a quarter or half
// an hour off UTC's is taken back to its own ten minutes.
func onItsTenMinutes(t time.Time) time.Time {
	dayStart := dayfile.DayStart(t, 0)
	return dayStart.Add(t.Sub(dayStart).Truncate(tenMinutes))
}

// dayStartOf is when the day given, as dayfile.Civil gives one, starts in
// the time zone given, as dayfile.DayStart has it.
func dayStartOf(day time.Time, loc *time.Location) time.Time {
	y, m, d := day.Date()
	return dayfile.DayStart(time.Date(y, m, d, 12, 0, 0, 0, loc), 0)
}

// holds reports whether t falls in the timeline.
func (tl Timeline) holds(t time.Time) bool {
	return !t.Before(tl.Start) && t.Before(tl.End)
}

// Named is the days a timeline stepped back to shows, its first and its
// last, each as dayfile.Civil gives a day, as Runway's heading names them:
// one day twice. It reports false of a live timeline, named by its window
// alone.
func (tl Timeline) Named() (first, last time.Time, ok bool) {
	return tl.first, tl.last, !tl.first.IsZero()
}

// Step is how long a column is: each within a nanosecond of it.
func (tl Timeline) Step() time.Duration {
	return tl.span() / time.Duration(tl.across())
}

// Bounds are when column i begins and ends: span × i ÷ Columns from Start,
// up to where the next begins, to the nanosecond, so the columns cut the
// span whole, never rounded to a minute.
func (tl Timeline) Bounds(i int) (from, to time.Time) {
	return tl.edge(i), tl.edge(i + 1)
}

// edge is where column i begins: at End for Columns, or past it.
func (tl Timeline) edge(i int) time.Time {
	n := tl.across()
	return tl.Start.Add(time.Duration(mulDiv(int64(tl.span()), int64(min(max(i, 0), n)), int64(n))))
}

// Column is the column t falls in, as Bounds has the columns: -1 before the
// first, and Columns from End on.
func (tl Timeline) Column(t time.Time) int {
	switch {
	case t.Before(tl.Start):
		return -1
	case !t.Before(tl.End):
		return tl.Columns
	}
	i := int(mulDiv(int64(t.Sub(tl.Start)), int64(tl.across()), int64(tl.span())))
	if !t.Before(tl.edge(i + 1)) {
		i++
	}
	return i
}

// NowColumn is the column now falls in: -1 where the timeline has no now.
func (tl Timeline) NowColumn() int {
	if tl.Now.IsZero() {
		return -1
	}
	return tl.Column(tl.Now)
}

// Moment is the moment column i stands for: its middle, or in now's column,
// now, which every column after it is ahead of.
func (tl Timeline) Moment(i int) time.Time {
	if i == tl.NowColumn() {
		return tl.Now
	}
	from, to := tl.Bounds(i)
	return from.Add(to.Sub(from) / 2)
}

// Midnights are the starts of the days that begin in the timeline, as the
// axis ticks them: each where the clocks put its midnight, as
// dayfile.DayStart has it, in Start's time zone.
func (tl Timeline) Midnights() []time.Time {
	var midnights []time.Time
	for at := dayfile.DayStart(tl.Start, 0); at.Before(tl.End); at = dayfile.DayStart(at, 1) {
		if !at.Before(tl.Start) {
			midnights = append(midnights, at)
		}
	}
	return midnights
}

// Hours are the whole hours of the clock in the timeline, every so many as
// given, those the hour of the day divides, as the axis ticks them: each
// where the clocks first read it, none the clocks went forward over, and no
// day's start, which Midnights has.
func (tl Timeline) Hours(every int) []time.Time {
	every = max(every, 1)
	var hours []time.Time
	for day := dayfile.DayStart(tl.Start, 0); day.Before(tl.End); day = dayfile.DayStart(day, 1) {
		for hour := every; hour < 24; hour += every {
			at := dayfile.TimeOfDay(day, hour, 0)
			if at.Hour() == hour && at.Minute() == 0 && tl.holds(at) && !at.Equal(dayfile.DayStart(at, 0)) {
				hours = append(hours, at)
			}
		}
	}
	return hours
}

// span is how long the timeline runs.
func (tl Timeline) span() time.Duration {
	return tl.End.Sub(tl.Start)
}

// across is how many columns the timeline's span is cut among: one, at the
// least.
func (tl Timeline) across() int {
	return max(tl.Columns, 1)
}

// mulDiv is a × b ÷ c, rounded down, for a and b none or more and c more
// than none, where it fits: a × b is worked out in 128 bits, so it never
// overflows.
func mulDiv(a, b, c int64) int64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	q, _ := bits.Div64(hi, lo, uint64(c))
	return int64(q)
}
