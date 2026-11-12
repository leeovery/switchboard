package dashboard

import (
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

// runwayWeek is what Runway and Graphs call the week they show, as the use
// under a lane's name and the words of the legend and help name it.
const runwayWeek = "week"

// RunwayShown is Runway as the watch gives it: whether its account column is
// collapsed, as a has it, which the preferences keep, as Kept has them; the
// first day of the week stepped back to, as dayfile.Civil gives a day, zero
// in the live view, which they never keep; and what the watch built at the
// look: the timeline the lanes are drawn over, as TimelineAt has it, the
// lanes, as views.Lanes builds them, nil till it's built them, COMING UP's
// lines, as views.RunwayComing has them, and LATELY's, as views.RunwayLately
// has them; and its accounts' name column, as Labelled sizes it, none till it
// does. What draws Runway reads what the watch built.
type RunwayShown struct {
	Collapsed bool
	Back      time.Time
	Timeline  views.Timeline
	Lanes     *views.Lanes
	Coming    []status.Upcoming
	Lately    []views.Sentence
	labels    flex.Part
}

// Labelled is Runway shown as r with its accounts' name column sized for the
// accounts of the moment m, as runwayLabelsOf sizes it.
func (r RunwayShown) Labelled(m status.Moment) RunwayShown {
	r.labels = runwayLabelsOf(namesOf(m))
	return r
}

// RunwayKept is Runway as the preferences kept it: live, its account column
// collapsed or not.
func RunwayKept(kept theme.RunwayPrefs) RunwayShown {
	return RunwayShown{Collapsed: kept.Collapsed}
}

// Kept is what the preferences keep of Runway, as RunwayKept reads it:
// whether its account column is collapsed; never a step back.
func (r RunwayShown) Kept() theme.RunwayPrefs {
	return theme.RunwayPrefs{Collapsed: r.Collapsed}
}

// Collapsing is Runway with its account column collapsed where it's
// expanded, and expanded where it's collapsed, as a does.
func (r RunwayShown) Collapsing() RunwayShown {
	r.Collapsed = !r.Collapsed
	return r
}

// TimelineAt is the timeline the lanes of Runway shown as r are drawn
// over at now on a terminal width cells wide, weeks starting on the day
// given: the live view's, or that of the week stepped back to, as
// views.Stepped has it, across the columns Columns gives them.
func (r RunwayShown) TimelineAt(now time.Time, starts time.Weekday, width int) views.Timeline {
	return views.Stepped(r.Back, now, starts, r.Columns(width))
}

// Columns is how many columns Runway shown as r has its lanes take on a
// terminal width cells wide: from their first, as lanesFrom has it, to the
// edge.
func (r RunwayShown) Columns(width int) int {
	return max(width-r.lanesFrom(width), 0)
}

// lanesFrom is the column Runway shown as r starts its lanes at on a
// terminal width cells wide: with the account column collapsed, the first,
// as it has no label column; else after its label column, its parts laid as
// runwayParts declares them.
func (r RunwayShown) lanesFrom(width int) int {
	if r.Collapsed {
		return 0
	}
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

// labelsColumn is Runway's label column shown as r, its gap after it: its
// accounts' names' column, as runwayLabelsOf has it, its natural no less than
// the widest use under a name, as runwayUseCells has it.
func (r RunwayShown) labelsColumn() flex.Part {
	labels := r.labels
	labels.Natural = max(labels.Natural, runwayUseCells())
	labels.Most = max(labels.Most, labels.Natural)
	return labels
}

// runwayUseCells are the cells the label column keeps for the widest use
// under a lane's name, as "week 100%", so the lanes hold still as the
// figures change: from runwayUseFrom, and the cell before the lanes useLine
// keeps.
func runwayUseCells() int {
	return runwayUseFrom + len(runwayWeek) + 1 + widestFigure + 1
}

// Live is Runway shown as r showing its live view, its account column as it
// is, as Graphs reads Runway's lanes over the week.
func (r RunwayShown) Live() RunwayShown {
	r.Back = time.Time{}
	return r
}

// Stepped is Runway stepped back, by -1, as ← steps it, or on, by 1, as →
// does, at now, weeks starting on the day given: to this week, then a week
// at a time; and on from this week to the live view. The first day of the
// week stepped back to is kept, not how far back it is, so it stays itself
// as the clock passes midnight. It reports false where it stays, as stepping
// on from the live view, with nothing ahead to step to.
func (r RunwayShown) Stepped(by int, now time.Time, starts time.Weekday) (RunwayShown, bool) {
	latest := dayOf(views.WeekStart(now, starts))
	switch {
	case r.Back.IsZero() && by > 0:
		return r, false
	case r.Back.IsZero():
		r.Back = latest
	case by > 0 && !r.Back.Before(latest):
		r.Back = time.Time{}
	default:
		r.Back = r.Back.AddDate(0, 0, by*7)
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
// its timeline shows, as weekNamed has them; its rule runs to the frame's
// edge, as Sessions' does.
func (r RunwayShown) heading() heading {
	h := heading{name: strings.ToUpper(Runway.Title())}
	if first, last, ok := r.Timeline.Named(); ok {
		h.state = line{{weekNamed(first, last), faintInk}}
	}
	return h
}

// weekNamed names the week from first to last, as dayfile.Civil gives each
// day, as Mon 28 Sep – Sun 4 Oct.
func weekNamed(first, last time.Time) string {
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
// COMING UP, LATELY and what's out of view, each as its drawer has it;
// then now's column picked out under them.
func (f Frame) runwayFormed(doc status.Document, now time.Time, x int, fitted func(top int) runwayFit) runwayForm {
	return runwayForm{
		draw: func(c *canvas, top int) {
			laid := f.runwayLaidOut(f.Moment(doc, now), x, fitted(top))
			f.runwayLearning(c, laid)
			f.runwayAxis(c, laid)
			f.runwayLanes(c, laid)
			f.runwayWords(c, laid)
			f.runwayPool(c, laid)
			f.runwayLegend(c, laid)
			f.runwayComing(c, laid)
			f.runwayLately(c, laid)
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

// learningLine is the line Runway shows under its heading while the
// forecast's run-outs play the hold curve's default, as views.Lanes'
// Learning says.
var learningLine = line{{"run-outs firm up once switchboard has a day of your sessions", mutedInk}}

// runwayLearning draws the line saying the forecast is learning, from column
// 0, where the fit gives it a row.
func (f Frame) runwayLearning(c *canvas, laid runwayLaid) {
	if fit := laid.fit; fit.draws(fit.learning) {
		c.line(0, fit.learning, learningLine)
	}
}
