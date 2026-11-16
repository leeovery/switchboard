package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
)

// SESSIONS' rows at the fewest: a blank row over it, its heading, its heads
// and three of its lines, which stay however short the rows run.
const sessionsLeast = 1 + 1 + 1 + 3

// The list's rows: those it gives back to, from its third, before any of
// the chosen project's sections leaves; and the fewest it shows, where it
// has as many projects.
const (
	listBeforeLeaving = 5
	listLeast         = 2
)

// projectsLaid is Projects as a frame lays it out of a moment at now, from
// the view's top: the list, across the width; and under it, a blank row
// between, the chosen project, where there's one: its head, its numbers,
// the sections kept, and SESSIONS to the row over the key line; and the
// most rows any project of the page takes, which it gives way against.
type projectsLaid struct {
	f        Frame
	top      int
	list     projectsList
	project  *projectDrawn
	numbers  []line
	sessions projectSessions
	most     pageMost
}

// pageMost are the most rows any project of the page takes, of its parts
// that differ from one to the next: its numbers' lines, and BY MODEL's
// versions and ON ACCOUNTS' accounts.
type pageMost struct{ numbers, versions, accounts int }

// projectsGiving is how far Projects gives way with a project chosen: the
// rows the list shows its projects in, the line saying how many more among
// them; how many of the project's sections show, from the first; and WORTH
// BY's columns' rows.
type projectsGiving struct{ list, kept, chart int }

// projectsLaying is Projects of doc at now as the frame lays it out, as
// projectsLaid has it: the list's projects in up to a third of the view's
// rows, scrolling past them to show the one chosen; and under it, where a
// project is chosen, its sections as given has them give way against the
// page's most, so the list, WORTH BY's columns and the sections kept hold
// still as the choice moves; the project draws them at its own heights,
// SESSIONS taking the rest.
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
	l.project, l.numbers = &d, projectNumbers(p.ProjectRow, f.Width)
	l.most = l.mostOf()
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
	return max(min(n, l.f.viewRows()/3), min(n, listLeast))
}

// listFitting are the rows the list shows its projects in, as flex lays
// them down beside the list's heads, rule and every project's row, and the
// rows given under it: as many as given, two at the fewest where there are
// two.
func (l projectsLaid) listFitting(rows, under int) int {
	spans, _ := flex.Lay(l.f.viewRows(),
		flex.Part{Least: min(len(l.f.Projects.rows()), listLeast), Natural: rows},
		flex.Part{Least: listTaken(0) + under, Natural: listTaken(0) + under},
	)
	return spans[0].Size
}

// mostOf are the most rows any project of the page takes, the chosen's
// among them: the numbers' lines at the frame's width, as projectNumbers
// wraps each row's; and the versions and accounts, as the page's Most has
// them.
func (l projectsLaid) mostOf() pageMost {
	most := pageMost{numbers: len(l.numbers), versions: len(l.project.page.ByModel), accounts: len(l.project.page.OnAccounts)}
	if page := l.f.Projects.Page; page != nil {
		most.versions, most.accounts = max(most.versions, page.Most.Versions), max(most.accounts, page.Most.Accounts)
	}
	for _, r := range l.f.Projects.rows() {
		most.numbers = max(most.numbers, len(projectNumbers(r, l.f.Width)))
	}
	return most
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
	return projectsGiving{list: l.listFitting(third, 1+1+l.most.numbers+sessionsLeast), chart: worthRowsLeast}
}

// projectsGivingWay are the ways Projects gives way, in their order, to
// WORTH BY kept alone, the list showing third rows at the most and least
// before any section leaves: WORTH BY's columns squeeze from worthRows to
// worthRowsLeast; then the list gives rows back to least; then the sections
// leave, the last first, so ON ACCOUNTS goes, then BY MODEL.
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

// fits reports whether Projects given way as g fits the view's rows at the
// page's most, as flex lays them down: what's above the sections, the
// sections kept, a blank row before them, and SESSIONS at its least.
func (l projectsLaid) fits(g projectsGiving) bool {
	_, fits := flex.Lay(l.f.viewRows(),
		flex.Part{Least: l.aboveOf(g.list, l.most.numbers)}, flex.Part{Least: l.mostHeight(g), Gap: 1}, flex.Part{Least: sessionsLeast})
	return fits
}

// mostHeight is the rows the sections kept in g take at the page's most, as
// the chosen project's lie.
func (l projectsLaid) mostHeight(g projectsGiving) int {
	d := l.project.squeezed(g.chart)
	return d.lain(d.heightsOf(l.most.versions, l.most.accounts), g.kept)
}

// roomiest is g with WORTH BY's columns given back the rows that cost
// nothing, as where, wide, BY MODEL and ON ACCOUNTS beside it are taller.
func (l projectsLaid) roomiest(g projectsGiving) projectsGiving {
	h := l.mostHeight(g)
	for g.chart < worthRows && l.mostHeight(projectsGiving{list: g.list, kept: g.kept, chart: g.chart + 1}) == h {
		g.chart++
	}
	return g
}

// above are the rows from the view's top to the row after the chosen
// project's numbers, as aboveOf has them of the list's lines shown and the
// numbers' own.
func (l projectsLaid) above() int {
	return l.aboveOf(l.list.linesShown(), len(l.numbers))
}

// aboveOf are the rows from the view's top to the row after the chosen
// project's numbers, the list and the numbers showing as many lines as
// given: the list's, a blank row, the head and the numbers.
func (l projectsLaid) aboveOf(lines, numbers int) int {
	return listTaken(lines) + 1 + 1 + numbers
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
