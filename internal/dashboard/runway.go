package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Runway's parts across a frame, in runwayParts' order: the label column,
// its gap after it, and the lanes, which take the rest.
const (
	runwayLabelColumn = iota
	runwayLaneColumns
)

// collapsedGap are the cells after the widest label of Runway's label
// column, collapsed.
const collapsedGap = 1

// runwayWindows are the windows Runway's views are named for, in the order
// < and > step between them, and runwayNames what the heading calls each.
var (
	runwayWindows = views.RunwayWindows()
	runwayNames   = map[views.RunwayWindow]string{views.RunwayFiveHour: "5-hour", views.RunwayWeek: "week"}
)

// RunwayShown is Runway as the watch gives it: the window its view is named
// for, and whether its account column is collapsed, as a has it, which the
// preferences keep; the day stepped back to, as dayfile.Civil gives one, of
// the 5-hour view, or the first day of the week stepped back to, of the
// week's, zero in the live view, which they never keep; and what
// the watch built at the look: the timeline the lanes are drawn over, as
// TimelineAt has it, the lanes, as views.Lanes builds them, nil till
// it's built them, and COMING UP's lines, as views.RunwayComing has them;
// and the cells its accounts' labels take whole, as Labelled sizes them,
// none till it does. What draws Runway reads what the watch built.
type RunwayShown struct {
	Window    views.RunwayWindow
	Collapsed bool
	Back      time.Time
	Timeline  views.Timeline
	Lanes     *views.Lanes
	Coming    []status.Upcoming
	labels    runwayLabels
}

// runwayLabels are Runway's accounts' labels, as runwayLabelsOf has them:
// expanded, their name column; and with the account column collapsed, the
// cells the widest takes.
type runwayLabels struct {
	expanded  flex.Part
	collapsed int
}

// Labelled is Runway shown as r with its accounts' labels sized for the
// accounts of the moment m, as runwayLabelsOf sizes them.
func (r RunwayShown) Labelled(m status.Moment) RunwayShown {
	r.labels = runwayLabelsOf(namesOf(m))
	return r
}

// RunwayKept is Runway as the preferences kept it: the window, by its name,
// live, and its account column collapsed or not. What names no window, as a
// later build's, shows the week.
func RunwayKept(kept theme.RunwayPrefs) RunwayShown {
	r := RunwayShown{Collapsed: kept.Collapsed}
	if w := views.RunwayWindow(kept.Window); slices.Contains(runwayWindows, w) {
		r.Window = w
	}
	return r
}

// Kept is what the preferences keep of Runway, as RunwayKept reads it: the
// window and whether its account column is collapsed, never a step back.
func (r RunwayShown) Kept() theme.RunwayPrefs {
	return theme.RunwayPrefs{Window: string(r.Window), Collapsed: r.Collapsed}
}

// Collapsing is Runway with its account column collapsed where it's
// expanded, and expanded where it's collapsed, as a does.
func (r RunwayShown) Collapsing() RunwayShown {
	r.Collapsed = !r.Collapsed
	return r
}

// TimelineAt is the timeline the lanes of Runway shown as r are drawn
// over at now on a terminal width cells wide, weeks starting on the day
// given: the live view's, or that of the day or week stepped back to, as
// views.Stepped has it, across the columns Columns gives them.
func (r RunwayShown) TimelineAt(now time.Time, starts time.Weekday, width int) views.Timeline {
	return views.Stepped(r.Window, r.Back, now, starts, r.Columns(width))
}

// Columns is how many columns Runway shown as r has its lanes take on a
// terminal width cells wide: from their first, as lanesFrom has it, to the
// edge.
func (r RunwayShown) Columns(width int) int {
	return max(width-r.lanesFrom(width), 0)
}

// lanesFrom is the column Runway shown as r starts its lanes at on a
// terminal width cells wide, after its label column, its parts laid as
// runwayParts declares them.
func (r RunwayShown) lanesFrom(width int) int {
	s, _ := flex.Lay(width, r.runwayParts()...)
	return s[runwayLaneColumns].At
}

// runwayParts are Runway's parts across a frame, as flex lays them: the label
// column, as labelsColumn has it, and the lanes, which take the rest.
func (r RunwayShown) runwayParts() []flex.Part {
	return []flex.Part{
		runwayLabelColumn: r.labelsColumn(),
		runwayLaneColumns: {Grows: true},
	}
}

// labelsColumn is Runway's label column shown as r, its gap after it: with
// the account column collapsed, the widest of its labels beside a lane, a
// number and its marks or the widest use, 100%, and collapsedGap, whatever
// the width; else its accounts' names' column, as runwayLabelsOf has it, its
// natural no less than the widest use under a name, as runwayUseCells has
// it.
func (r RunwayShown) labelsColumn() flex.Part {
	if r.Collapsed {
		cells := max(r.labels.collapsed, widestFigure) + collapsedGap
		return flex.Part{Least: cells, Natural: cells}
	}
	labels := r.labels.expanded
	labels.Natural = max(labels.Natural, runwayUseCells())
	labels.Most = max(labels.Most, labels.Natural)
	return labels
}

// runwayUseCells are the cells the label column keeps for the widest use
// under a lane's name, of either window, as "5-hour 100%", so the lanes
// hold still as < and > step between them and as the figures change: from
// runwayUseFrom, and the cell before the lanes useLine keeps.
func runwayUseCells() int {
	widest := 0
	for _, name := range runwayNames {
		widest = max(widest, len(name))
	}
	return runwayUseFrom + widest + 1 + widestFigure + 1
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

// LiveOf is Runway shown as r showing the live view of the window w, its
// account column as it is, as Graphs reads Runway's lanes over each window.
func (r RunwayShown) LiveOf(w views.RunwayWindow) RunwayShown {
	r.Window, r.Back = w, time.Time{}
	return r
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
// its part where it lies: of the moment m, which each reads; its lanes from
// column x, their labels in the cells before them, those shown each on its
// row, as laidLane has it; and its rows fitted as fit has them.
type runwayLaid struct {
	x     int
	m     status.Moment
	lanes []laidLane
	fit   runwayFit
}

// runwayLaidOut is Runway of the moment m laid out as fit fits it, its lanes
// from column x, those shown as laidLanes has them.
func (f Frame) runwayLaidOut(m status.Moment, x int, fit runwayFit) runwayLaid {
	return runwayLaid{x: x, m: m, lanes: f.laidLanes(m, fit), fit: fit}
}

// runwayForm is the form Runway is drawn in, of doc at now: its lanes from
// where lanesFrom starts them, laid out as runwayFitted fits it, as
// runwayFormed draws them.
func (f Frame) runwayForm(doc status.Document, now time.Time) runwayForm {
	return f.runwayFormed(doc, now, f.Runway.lanesFrom(f.Width), f.runwayFitted)
}

// runwayFormed is Runway of doc at now as its form draws it, its lanes from
// column x, laid out from the row it's drawn from as fitted fits it: the
// axis, the lanes and the words under them, the pool's lane, the legend,
// COMING UP and what's out of view, each as its drawer has it; then now's
// column picked out under them.
func (f Frame) runwayFormed(doc status.Document, now time.Time, x int, fitted func(top int) runwayFit) runwayForm {
	return runwayForm{
		draw: func(c *canvas, top int) {
			laid := f.runwayLaidOut(f.Moment(doc, now), x, fitted(top))
			f.runwayAxis(c, laid)
			f.runwayLanes(c, laid)
			f.runwayWords(c, laid)
			f.runwayPool(c, laid)
			f.runwayLegend(c, laid)
			f.runwayComing(c, laid)
			f.runwayOut(c, laid)
			f.runwayNow(c, laid)
		},
		scrolling: fitted(f.top()).scrolling,
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
