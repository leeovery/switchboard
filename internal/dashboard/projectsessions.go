package dashboard

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The turns SESSIONS' columns give way in as it narrows, once the session's
// is at its least: model, then started, then requests. Worth always shows,
// as the list ranks by it.
const (
	modelGivesWay = iota + 1
	startedGivesWay
	requestsGiveWay
)

// projectColumns are SESSIONS' columns, of the List's, in the row's order.
var projectColumns = []listPart{listSession, listOn, listModel, listState, listStarted, listRequests, listWorth}

// projectSessions are the chosen project's SESSIONS as a frame lays them out
// from row top, rows tall, its heading and its heads among them, scrolled
// as given: its sessions by day.
type projectSessions struct {
	sessionsByDay
	top    int
	rows   int
	scroll int
}

// SessionsMeasured are the cells of the widest each of SESSIONS' columns
// says of the ended sessions of a chosen project's page, in the columns'
// order, measured once, as the watch builds the page: what they say holds
// still, so a frame measures only the sessions running, whose words change
// as they run, and not every session of a long period at every frame.
type SessionsMeasured struct {
	of     *views.ProjectPage
	widest []int
}

// MeasureSessions measures the ended sessions of the page given at the
// moment m at now, as SessionsMeasured has them.
func MeasureSessions(page *views.ProjectPage, m status.Moment, now time.Time) SessionsMeasured {
	words := sessionWords{m: m, now: now, weekdays: true}
	widest := make([]int, len(projectColumns))
	for i, p := range projectColumns {
		widest[i] = words.widest(p, page.Sessions, sessionEnded)
	}
	return SessionsMeasured{of: page, widest: widest}
}

// sessionEnded reports whether the session has ended.
func sessionEnded(s views.PeriodSession) bool {
	return s.EndedOn != ""
}

// sessionRunning reports whether the session runs.
func sessionRunning(s views.PeriodSession) bool {
	return !sessionEnded(s)
}

// projectSessionsOf are the chosen project's sessions, of the page given, as
// SESSIONS lays them out across a frame width cells wide, as columnsOf lays
// its columns out, of the ended sessions as measured measures them, where
// it's of the page, their lines as sessionLines has them.
func projectSessionsOf(words sessionWords, page *views.ProjectPage, measured SessionsMeasured, width int) projectSessions {
	words.weekdays = true
	s := projectSessions{words: words, lines: sessionLines(page.Sessions), width: width}
	if measured.of != page {
		measured = MeasureSessions(page, words.m, words.now)
	}
	s.columns = s.columnsOf(width, page.Sessions, measured)
	return s
}

// columnsOf are SESSIONS' columns of the sessions given across a frame
// width cells wide, from listFrom, listGap apart, as flex lays them: the
// session's as the List's, from its least to its whole, taking what's spare
// first; then each other at the widest of its head and what it says of the
// sessions, the ended as measured has them and those running as they say
// now, on's narrowing as the List's does toward listOnLeast. As it narrows,
// model, started and requests leave in the turns above. No natural comes
// from the rows shown, so the columns hold still as SESSIONS scrolls.
func (s projectSessions) columnsOf(width int, sessions []views.PeriodSession, measured SessionsMeasured) []listColumn {
	parts := make([]flex.Part, len(projectColumns))
	for i, p := range projectColumns {
		w := max(measured.widest[i], s.words.widest(p, sessions, sessionRunning))
		parts[i] = flex.Part{Least: w, Natural: w, Gap: listGap}
	}
	parts[0] = flex.Part{Least: listSessionLeast, Natural: listSessionWhole}
	parts[1].Least = min(parts[1].Natural, listOnLeast)
	parts[2].Leaves, parts[4].Leaves, parts[5].Leaves = modelGivesWay, startedGivesWay, requestsGiveWay
	spans, _ := flex.Lay(width-listFrom, parts...)
	return columnsAt(spans, projectColumns)
}

// widest are the cells of the widest of the column's head and what it says
// of each of the sessions given that of says.
func (w sessionWords) widest(p listPart, sessions []views.PeriodSession, of func(views.PeriodSession) bool) int {
	col := listColumn{part: p}
	cells := ansi.StringWidth(col.head())
	for _, s := range sessions {
		if of(s) {
			cells = max(cells, w.says(col, s.ListedSession).width())
		}
	}
	return cells
}

// within is SESSIONS laid out from row top, rows tall, its sessions within
// the page under its heads, as sessionsByDay's within has them.
func (s projectSessions) within(top, rows int) projectSessions {
	s.top, s.rows = top, rows
	s.sessionsByDay = s.sessionsByDay.within(s.paged().page())
	return s
}

// paged is how SESSIONS' lines page under its heads, a row at a time, each
// day's rule shown with the first session under it, or not at all.
func (s projectSessions) paged() rowsPaged {
	return rowsPaged{rows: s.rows - 2, content: len(s.lines), heads: s.heads()}
}

// draw draws SESSIONS from its top: its heading, in text.subtle; its heads,
// as the List's; and its lines in view, as far as it's scrolled, as drawPage
// draws them; nothing where no line fits under its heading and heads.
func (s projectSessions) draw(c *canvas) {
	if s.paged().page() == 0 {
		return
	}
	c.line(0, s.top, line{{"SESSIONS", dimInk}})
	columnHeads(c, s.top+1, s.columns)
	s.drawPage(c, s.top+2, s.paged(), s.scroll)
}

// targets are the sessions' rows in view, as sessionsByDay's targets has
// them.
func (s projectSessions) targets() []Target {
	return s.sessionsByDay.targets(s.top+2, s.paged(), s.scroll)
}
