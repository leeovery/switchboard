package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
)

// SESSIONS' rows at the fewest: a blank row over it, its heading, its heads
// and three of its lines, which stay however short the rows run.
const sessionsLeast = 1 + 1 + 1 + 3

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

// projectsLaying is Projects of doc at now as the frame lays it out, as
// projectsLaid has it: the list's projects in up to a third of the view's
// rows, as flex lays them down beside the rest, scrolling past them to show
// the one chosen; and under it the sections that fit, as kept keeps them,
// SESSIONS taking the rest.
func (f Frame) projectsLaying(doc status.Document, now time.Time) projectsLaid {
	m := f.Moment(doc, now)
	l := projectsLaid{f: f, top: f.top()}
	under := 0
	if p := f.Projects.Project; p != nil {
		d := projectDrawnOf(f, m, now, *p)
		l.project, l.numbers = &d, d.numbers(f.Width)
		under = 1 + 1 + len(l.numbers) + sessionsLeast
	}
	l.list = projectsListOf(f, l.listRows(under), !f.Projects.InSessions, now)
	if l.project != nil {
		l.project.kept = l.kept()
		l.sessions = projectSessionsOf(sessionWords{m: m, now: now}, f.Projects.Project, f.Projects.Measured, f.Width)
		at := l.sessionsAt()
		l.sessions = l.sessions.within(at, l.top+f.viewRows()-at)
		l.sessions.scroll = f.Scroll
		if f.Projects.InSessions {
			l.sessions.lit = f.Projects.Selection()
		}
	}
	return l
}

// listRows are the rows the list shows its projects in, the line saying how
// many more there are among them: each project's, up to a third of the
// view's rows, two at the fewest where there are two, as flex lays them
// down beside the list's heads, rule and every project's row, and the rows
// given under it.
func (l projectsLaid) listRows(under int) int {
	n := len(l.f.Projects.rows())
	spans, _ := flex.Lay(l.f.viewRows(),
		flex.Part{Least: min(n, 2), Natural: min(n, l.f.viewRows()/3)},
		flex.Part{Least: 3 + under, Natural: 3 + under},
	)
	return spans[0].Size
}

// above are the rows from the view's top to the row after the chosen
// project's numbers: the list's, a blank row, the head and the numbers.
func (l projectsLaid) above() int {
	return l.list.rowsTaken() + 1 + 1 + len(l.numbers)
}

// kept is how many of the chosen project's sections show, from the first:
// as many as flex fits down the view's rows between what's above them and
// SESSIONS at its least, a blank row before each, the last leaving first, so
// ON ACCOUNTS goes, then BY MODEL, then WORTH BY.
func (l projectsLaid) kept() int {
	for kept := projectSections; kept > 0; kept-- {
		h := l.project.height(kept)
		if _, fits := flex.Lay(l.f.viewRows(), flex.Part{Least: l.above()}, flex.Part{Least: h, Gap: 1}, flex.Part{Least: sessionsLeast}); fits {
			return kept
		}
	}
	return 0
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
