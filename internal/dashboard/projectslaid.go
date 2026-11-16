package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
)

// SESSIONS' rows at the fewest: a blank row over it, its heading, its heads
// and three of its lines, which stay however short the rows run.
const sessionsLeast = 1 + 1 + 1 + 3

// listBeforeLeaving are the rows the list gives back to, from its third,
// before any of the chosen project's sections leaves.
const listBeforeLeaving = 5

// projectsLaid is Projects as a frame lays it out of a moment at now, from
// the view's top: the list, across the width; and under it, a blank row
// between, the chosen project, where there's one: its head, its numbers,
// the sections kept, and SESSIONS to the row over the key line.
type projectsLaid struct {
	f        Frame
	top      int
	list     projectsList
	project  *projectDrawn
	numbers  []line
	sessions projectSessions
}

// projectsGiving is how far Projects gives way with a project chosen: the
// rows the list shows its projects in, the line saying how many more among
// them; how many of the project's sections show, from the first; and WORTH
// BY's columns' rows.
type projectsGiving struct{ list, kept, chart int }

// projectsLaying is Projects of doc at now as the frame lays it out, as
// projectsLaid has it: the list's projects in up to a third of the view's
// rows, scrolling past them to show the one chosen; and under it, where a
// project is chosen, its sections as given has them give way, SESSIONS
// taking the rest.
func (f Frame) projectsLaying(doc status.Document, now time.Time) projectsLaid {
	l := projectsLaid{f: f, top: f.top()}
	lit := !f.Projects.InSessions
	p := f.Projects.Project
	if p == nil {
		l.list = projectsListOf(f, l.listFitting(l.listThird(), 0), lit, now)
		return l
	}
	m := f.Moment(doc, now)
	d := projectDrawnOf(f, m, now, *p)
	l.project, l.numbers = &d, d.numbers(f.Width)
	g := l.given()
	l.list = projectsListOf(f, g.list, lit, now)
	l.project.kept, l.project.chart = g.kept, g.chart
	l.sessions = projectSessionsOf(sessionWords{m: m, now: now}, p, f.Projects.Measured, f.Width)
	at := l.sessionsAt()
	l.sessions = l.sessions.within(at, l.top+f.viewRows()-at)
	l.sessions.scroll = f.Scroll
	if f.Projects.InSessions {
		l.sessions.lit = f.Projects.Selection()
	}
	return l
}

// listThird are the rows the list shows its projects in, whatever it costs
// below: each project's, up to a third of the view's rows, two at the
// fewest where there are two.
func (l projectsLaid) listThird() int {
	n := len(l.f.Projects.rows())
	return max(min(n, l.f.viewRows()/3), min(n, 2))
}

// listFitting are the rows the list shows its projects in, as flex lays
// them down beside the list's heads, rule and every project's row, and the
// rows given under it: as many as given, two at the fewest where there are
// two.
func (l projectsLaid) listFitting(rows, under int) int {
	spans, _ := flex.Lay(l.f.viewRows(),
		flex.Part{Least: min(len(l.f.Projects.rows()), 2), Natural: rows},
		flex.Part{Least: listTaken(0) + under, Natural: listTaken(0) + under},
	)
	return spans[0].Size
}

// given is how far Projects gives way as the view's rows run short: the
// first of projectsGivingWay that fits, as fits has it, WORTH BY's columns
// as roomy as costs nothing; else, every section gone, the list as it is
// with nothing between it and SESSIONS, up to its third, giving way to two
// rows, as listFitting has it, so SESSIONS keeps its least.
func (l projectsLaid) given() projectsGiving {
	third := l.listThird()
	for _, g := range projectsGivingWay(third, min(third, listBeforeLeaving)) {
		if l.fits(g) {
			return l.roomiest(g)
		}
	}
	return projectsGiving{list: l.listFitting(third, 1+1+len(l.numbers)+sessionsLeast), chart: worthRowsLeast}
}

// projectsGivingWay are the ways Projects gives way, in their order, to
// WORTH BY kept alone, the list showing third rows at the most and least
// before any section leaves: WORTH BY's columns squeeze from
// worthRows to worthRowsLeast; then the list gives rows back to least; then
// the sections leave, the last first, so ON ACCOUNTS goes, then BY MODEL.
func projectsGivingWay(third, least int) []projectsGiving {
	var way []projectsGiving
	for chart := worthRows; chart >= worthRowsLeast; chart-- {
		way = append(way, projectsGiving{list: third, kept: projectSections, chart: chart})
	}
	for list := third - 1; list >= least; list-- {
		way = append(way, projectsGiving{list: list, kept: projectSections, chart: worthRowsLeast})
	}
	for kept := projectSections - 1; kept > 0; kept-- {
		way = append(way, projectsGiving{list: least, kept: kept, chart: worthRowsLeast})
	}
	return way
}

// fits reports whether Projects given way as g fits the view's rows, as
// flex lays them down: what's above the sections, the sections kept, a blank
// row before them, and SESSIONS at its least.
func (l projectsLaid) fits(g projectsGiving) bool {
	h := l.project.squeezed(g.chart).height(g.kept)
	_, fits := flex.Lay(l.f.viewRows(), flex.Part{Least: l.aboveOf(g.list)}, flex.Part{Least: h, Gap: 1}, flex.Part{Least: sessionsLeast})
	return fits
}

// roomiest is g with WORTH BY's columns given back the rows that cost
// nothing, as where, wide, BY MODEL and ON ACCOUNTS beside it are taller.
func (l projectsLaid) roomiest(g projectsGiving) projectsGiving {
	h := l.project.squeezed(g.chart).height(g.kept)
	for g.chart < worthRows && l.project.squeezed(g.chart+1).height(g.kept) == h {
		g.chart++
	}
	return g
}

// above are the rows from the view's top to the row after the chosen
// project's numbers, as aboveOf has them of the list's lines shown.
func (l projectsLaid) above() int {
	return l.aboveOf(l.list.linesShown())
}

// aboveOf are the rows from the view's top to the row after the chosen
// project's numbers, the list showing as many lines as given: the list's,
// a blank row, the head and the numbers.
func (l projectsLaid) aboveOf(lines int) int {
	return listTaken(lines) + 1 + 1 + len(l.numbers)
}

// sessionsAt is the row SESSIONS' heading is on: a blank row under the
// sections kept, or under the numbers where none is.
func (l projectsLaid) sessionsAt() int {
	at := l.top + l.above()
	if l.project.kept > 0 {
		at += 1 + l.project.height(l.project.kept)
	}
	return at + 1
}

// draw draws Projects from its top: the list; then, where a project is
// chosen, a blank row under it, its head and its numbers, its sections, a
// blank row before them, and SESSIONS.
func (l projectsLaid) draw(c *canvas) {
	y := l.list.draw(c, l.top)
	if l.project == nil {
		return
	}
	y++
	l.project.head(c, y)
	for i, n := range l.numbers {
		c.line(0, y+1+i, n)
	}
	if l.project.kept > 0 {
		l.project.sections(c, y+1+len(l.numbers)+1)
	}
	l.sessions.draw(c)
}

// scrolling is how far SESSIONS' lines scroll under its heads, a row at a
// time, scrolled offset down, as paged has them: nowhere without a project.
func (l projectsLaid) scrolling(offset int) Scrolling {
	if l.project == nil {
		return Scrolling{}
	}
	return l.sessions.paged().scrolling(offset)
}

// scrolls is what of Projects scrolls: SESSIONS' lines under its heads,
// across the frame; the view's rows, without a project.
func (l projectsLaid) scrolls() (x, y, width, rows int) {
	if l.project == nil {
		return 0, l.top, l.f.Width, l.f.viewRows()
	}
	return 0, l.sessions.top + 2, l.f.Width, l.sessions.rows - 2
}

// chooses is what of Projects the wheel chooses a project through: the
// list's projects shown and the line saying how many more, under its heads,
// across the frame.
func (l projectsLaid) chooses() (x, y, width, rows int) {
	return 0, l.top + 1, l.f.Width, l.list.linesShown()
}

// targets are the zones of Projects a click acts on: the list's projects
// shown, and SESSIONS' sessions in view.
func (l projectsLaid) targets() []Target {
	targets := l.list.targets(l.top, l.f.Width)
	if l.project != nil {
		targets = append(targets, l.sessions.targets()...)
	}
	return targets
}

// projects draws Projects of doc at now, as projectsLaid lays it out, once
// its page is read: nothing before, as its heading says it's loading.
func (f Frame) projects(c *canvas, doc status.Document, now time.Time) {
	if f.Projects.Page != nil {
		f.projectsLaying(doc, now).draw(c)
	}
}

// ProjectsListScroll is how far Projects' list is scrolled, by rows, to show
// the project chosen, as projectsLaid lays it out of doc at now: as far as
// the frame's ListScroll has it, where that shows it, else no further than
// it must go to.
func (f Frame) ProjectsListScroll(doc status.Document, now time.Time) int {
	return f.projectsLaying(doc, now).list.first
}

// ProjectsSessionsScroll is how far Projects' SESSIONS is scrolled, by rows,
// to show the session with the given id, as projectsLaid lays it out of doc
// at now: as far as the frame's Scroll has it, where that shows it, else as
// little further as shows it.
func (f Frame) ProjectsSessionsScroll(doc status.Document, now time.Time, id string) int {
	l := f.projectsLaying(doc, now)
	if l.project == nil {
		return f.Scroll
	}
	return l.sessions.paged().revealing(f.Scroll, l.sessions.lineOf(id))
}
