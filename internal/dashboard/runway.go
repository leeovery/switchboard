package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The columns Runway's lanes start at, after their labels: 24 on a frame
// from phoneUnder columns, the labels in the 22 cells before them; 14 on a
// phone's, its label column.
const (
	runwayLanesFrom      = 24
	phoneRunwayLanesFrom = 14
)

// runwayWindows are the windows Runway's views are named for, in the order
// < and > step between them, and runwayNames what the heading calls each.
var (
	runwayWindows = []views.RunwayWindow{views.RunwayFiveHour, views.RunwayWeek}
	runwayNames   = map[views.RunwayWindow]string{views.RunwayFiveHour: "5-hour", views.RunwayWeek: "week"}
)

// RunwayShown is Runway as the watch gives it: the window its view is named
// for, which the preferences keep; the day stepped back to, as dayfile.Civil
// gives one, of the 5-hour view, or the first day of the week stepped back
// to, of the week's, zero in the live view, which they never keep; and what
// the watch built at the look: the timeline the lanes are drawn over, as
// TimelineAt has it, the lanes, as views.Lanes builds them, nil till
// it's built them, and COMING UP's lines, as views.RunwayComing has them.
// What draws Runway reads what the watch built.
type RunwayShown struct {
	Window   views.RunwayWindow
	Back     time.Time
	Timeline views.Timeline
	Lanes    *views.Lanes
	Coming   []status.Upcoming
}

// RunwayKept is Runway as the preferences kept it: the window, by its name,
// live. What names none, as a later build's, shows the 5-hour.
func RunwayKept(kept theme.RunwayPrefs) RunwayShown {
	if w := views.RunwayWindow(kept.Window); slices.Contains(runwayWindows, w) {
		return RunwayShown{Window: w}
	}
	return RunwayShown{}
}

// Kept is what the preferences keep of Runway, as RunwayKept reads it: the
// window, never a step back.
func (r RunwayShown) Kept() theme.RunwayPrefs {
	return theme.RunwayPrefs{Window: string(r.Window)}
}

// TimelineAt is the timeline the lanes of Runway shown as r are drawn
// over at now on a terminal width cells wide, weeks starting on the day
// given: the live view's, or that of the day or week stepped back to, as
// views.Stepped has it, across the columns RunwayColumns gives them.
func (r RunwayShown) TimelineAt(now time.Time, starts time.Weekday, width int) views.Timeline {
	return views.Stepped(r.Window, r.Back, now, starts, RunwayColumns(width))
}

// RunwayColumns is how many columns Runway's lanes take on a terminal width
// cells wide: from their first, after the labels, to the edge.
func RunwayColumns(width int) int {
	from := runwayLanesFrom
	if Phone(width) {
		from = phoneRunwayLanesFrom
	}
	return max(width-from, 0)
}

// Windowed is Runway showing the live view of the window by places on from
// the one shown, round, as < and > step between them, a place each. It
// reports false where that's the live view shown already.
func (r RunwayShown) Windowed(by int) (RunwayShown, bool) {
	next := runwayWindows[round(slices.Index(runwayWindows, r.Window)+by, len(runwayWindows))]
	if next == r.Window && r.Back.IsZero() {
		return r, false
	}
	r.Window, r.Back = next, time.Time{}
	return r, true
}

// Stepped is Runway stepped back, by -1, as ← steps it, or on, by 1, as →
// does, at now, weeks starting on the day given: in the 5-hour view, from
// the live view to today, then a day at a time; in the week view, to this
// week, then a week at a time; and on from today, or this week, to the live
// view. The day stepped back to is kept, not how far back it is, so it stays
// itself as the clock passes midnight. It reports false where it stays, as
// stepping on from the live view, with nothing ahead to step to.
func (r RunwayShown) Stepped(by int, now time.Time, starts time.Weekday) (RunwayShown, bool) {
	latest, days := dayOf(now), 1
	if r.Window == views.RunwayWeek {
		latest, days = dayOf(views.WeekStart(now, starts)), 7
	}
	switch {
	case r.Back.IsZero() && by > 0:
		return r, false
	case r.Back.IsZero():
		r.Back = latest
	case by > 0 && !r.Back.Before(latest):
		r.Back = time.Time{}
	default:
		r.Back = r.Back.AddDate(0, 0, by*days)
	}
	return r, true
}

// dayOf is the day t falls on in its own time zone, as dayfile.Civil gives
// a day, as views.Stepped reads the day it's given in now's.
func dayOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// heading is Runway's heading row: its name, then, stepped back, the days
// its timeline shows, as daysNamed has them; and at the right, the window
// between < and >.
func (r RunwayShown) heading() heading {
	h := heading{name: strings.ToUpper(Runway.Title()), period: runwayNames[r.Window]}
	if first, last, ok := r.Timeline.Named(); ok {
		h.state = line{{daysNamed(first, last), faintInk}}
	}
	return h
}

// daysNamed names the days from first to last, as dayfile.Civil gives each: a
// day alone as Wed 30 Sep, a week as Mon 28 Sep – Sun 4 Oct.
func daysNamed(first, last time.Time) string {
	if first.Equal(last) {
		return prose.Day(first)
	}
	return prose.Day(first) + " – " + prose.Day(last)
}

// runwayForm is Runway as its form draws it: what it draws from row top; how
// far its lanes scroll, scrolled to each offset; and what of it scrolls, from
// column x along row y, width cells wide and rows tall. Each is nothing
// where it's nil: it draws nothing, never scrolls, and its view scrolls
// whole.
type runwayForm struct {
	draw      func(c *canvas, top int)
	scrolling func(offset int) Scrolling
	scrolls   func() (x, y, width, rows int)
}

// runwayLaid is Runway laid out on the page, for each of its drawers to draw
// its part where it lies: from row top, its lanes from column x, its rows
// fitted as fit has them, of the moment m, which each reads.
type runwayLaid struct {
	top, x int
	fit    runwayFit
	m      status.Moment
}

// runwayForm is the form Runway is drawn in, of doc at now: a phone's, as
// phoneRunway has it; else as wideRunway has it.
func (f Frame) runwayForm(doc status.Document, now time.Time) runwayForm {
	if f.phone() {
		return f.phoneRunway(doc, now)
	}
	return f.wideRunway(doc, now)
}

// wideRunway is Runway of doc at now from phoneUnder columns, as its form
// draws it, laid out as runwayFitted fits it: the axis, the lanes and the
// words under them, the pool's lane, the legend, COMING UP and what's out of
// view, each as its drawer has it.
func (f Frame) wideRunway(doc status.Document, now time.Time) runwayForm {
	return runwayForm{
		draw: func(c *canvas, top int) {
			laid := runwayLaid{top: top, x: runwayLanesFrom, fit: f.runwayFitted(top), m: f.Moment(doc, now)}
			f.runwayAxis(c, laid)
			f.runwayLanes(c, laid)
			f.runwayWords(c, laid)
			f.runwayPool(c, laid)
			f.runwayLegend(c, laid)
			f.runwayComing(c, laid)
			f.runwayOut(c, laid)
		},
		scrolling: f.runwayFitted(f.top()).scrolling,
	}
}

// runway draws Runway of doc at now from row top, as its form draws it.
func (f Frame) runway(c *canvas, doc status.Document, now time.Time, top int) {
	if form := f.runwayForm(doc, now); form.draw != nil {
		form.draw(c, top)
	}
}

// runwayScrolling is how far Runway of doc at now scrolls, scrolled to each
// offset given, as its form says: nowhere where it doesn't say.
func (f Frame) runwayScrolling(doc status.Document, now time.Time) func(offset int) Scrolling {
	if form := f.runwayForm(doc, now); form.scrolling != nil {
		return form.scrolling
	}
	return func(int) Scrolling { return Scrolling{} }
}

// runwayScrolls is what of Runway of doc at now scrolls, as its form says:
// else the view's rows, across the frame.
func (f Frame) runwayScrolls(doc status.Document, now time.Time) (x, y, width, rows int) {
	if form := f.runwayForm(doc, now); form.scrolls != nil {
		return form.scrolls()
	}
	return 0, f.top(), f.Width, f.viewRows()
}
