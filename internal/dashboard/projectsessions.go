package dashboard

import (
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
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

// sessionLine is a line of SESSIONS: a session's row; or the rule over the
// sessions that ended on a day, by the session first under it.
type sessionLine struct {
	session *views.ProjectSession
	ruled   bool
}

// projectSessions are the chosen project's SESSIONS as a frame lays them out
// from row top, rows tall, its heading and its heads among them: their
// words, as the List's say them grouped by day; their columns; their lines;
// and the session lit, by id, "" where none is.
type projectSessions struct {
	words   sessionWords
	columns []listColumn
	lines   []sessionLine
	top     int
	rows    int
	width   int
	lit     string
	scroll  int
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
	words := sessionWords{m: m, now: now, byDay: true}
	widest := make([]int, len(projectColumns))
	for i, p := range projectColumns {
		widest[i] = words.widest(p, page.Sessions, sessionEnded)
	}
	return SessionsMeasured{of: page, widest: widest}
}

// sessionEnded reports whether the session has ended.
func sessionEnded(s views.ProjectSession) bool {
	return s.EndedOn != ""
}

// sessionRunning reports whether the session runs.
func sessionRunning(s views.ProjectSession) bool {
	return !sessionEnded(s)
}

// projectSessionsOf are the chosen project's sessions, of the page given, as
// SESSIONS lays them out across a frame width cells wide, as columnsOf lays
// its columns out, of the ended sessions as measured measures them, where
// it's of the page, their lines as sessionLines has them.
func projectSessionsOf(words sessionWords, page *views.ProjectPage, measured SessionsMeasured, width int) projectSessions {
	words.byDay = true
	s := projectSessions{words: words, lines: sessionLines(page.Sessions), width: width}
	if measured.of != page {
		measured = MeasureSessions(page, words.m, words.now)
	}
	s.columns = s.columnsOf(width, page.Sessions, measured)
	return s
}

// sessionLines are SESSIONS' lines of the sessions given, in their order:
// those running first, with no rule; then, for each day the rest ended on,
// its rule and the sessions that ended on it.
func sessionLines(sessions []views.ProjectSession) []sessionLine {
	lines := make([]sessionLine, 0, len(sessions))
	for i := range sessions {
		s := &sessions[i]
		if s.EndedOn != "" && (i == 0 || sessions[i-1].EndedOn != s.EndedOn) {
			lines = append(lines, sessionLine{session: s, ruled: true})
		}
		lines = append(lines, sessionLine{session: s})
	}
	return lines
}

// columnsOf are SESSIONS' columns of the sessions given across a frame
// width cells wide, from listFrom, listGap apart, as flex lays them: the
// session's as the List's, from its least to its whole, taking what's spare
// first; then each other at the widest of its head and what it says of the
// sessions, the ended as measured has them and those running as they say
// now, on's narrowing as the List's does toward listOnLeast. As it narrows,
// model, started and requests leave in the turns above. No natural comes
// from the rows shown, so the columns hold still as SESSIONS scrolls.
func (s projectSessions) columnsOf(width int, sessions []views.ProjectSession, measured SessionsMeasured) []listColumn {
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
func (w sessionWords) widest(p listPart, sessions []views.ProjectSession, of func(views.ProjectSession) bool) int {
	col := listColumn{part: p}
	cells := ansi.StringWidth(col.head())
	for _, s := range sessions {
		if of(s) {
			cells = max(cells, w.says(col, s.ListedSession).width())
		}
	}
	return cells
}

// within is SESSIONS laid out from row top, rows tall: where that leaves a
// line alone under its heads, without its day rules, as a rule shows with the
// first session under it or not at all, and there can't.
func (s projectSessions) within(top, rows int) projectSessions {
	s.top, s.rows = top, rows
	if _, fits := flex.Lay(s.paged().page(), flex.Part{Least: 1}, flex.Part{Least: 1}); !fits {
		s.lines = slices.DeleteFunc(slices.Clone(s.lines), func(l sessionLine) bool { return l.ruled })
	}
	return s
}

// paged is how SESSIONS' lines page under its heads, a row at a time, each
// day's rule shown with the first session under it, or not at all.
func (s projectSessions) paged() rowsPaged {
	var heads []headed
	for i, l := range s.lines {
		if l.ruled {
			heads = append(heads, headed{at: i, first: i + 1})
		}
	}
	return rowsPaged{rows: s.rows - 2, content: len(s.lines), heads: heads}
}

// draw draws SESSIONS from its top: its heading, in text.subtle; its heads,
// as the List's; and its lines in view, as far as it's scrolled, as drawLine
// draws each; nothing where no line fits under its heading and heads.
func (s projectSessions) draw(c *canvas) {
	if s.paged().page() == 0 {
		return
	}
	c.line(0, s.top, line{{"SESSIONS", dimInk}})
	columnHeads(c, s.top+1, s.columns)
	from, to := s.paged().shown(s.scroll)
	for i := from; i < to; i++ {
		s.drawLine(c, s.top+2+i-from, s.lines[i])
	}
}

// drawLine draws the line l along row y: a day's rule, as the List's ended
// today is drawn, naming the day as prose.DayGone does; or the session's
// row, as the List draws one, lit where it's the one lit.
func (s projectSessions) drawLine(c *canvas, y int, l sessionLine) {
	if l.ruled {
		endedRule(c, y, s.width, line{{"ended " + prose.DayGone(s.words.now, l.session.Ended), dimInk}})
		return
	}
	s.words.sessionRow(c, y, s.columns, l.session.ListedSession, s.width, l.session.Session == s.lit)
}

// lineOf is the index of the line of the session with the given id: -1
// where none is.
func (s projectSessions) lineOf(id string) int {
	for i, l := range s.lines {
		if !l.ruled && l.session.Session == id {
			return i
		}
	}
	return -1
}

// targets are the sessions' rows in view, a click on which selects the
// session, and opens the one selected, each across the width.
func (s projectSessions) targets() []Target {
	var targets []Target
	from, to := s.paged().shown(s.scroll)
	for i := from; i < to; i++ {
		if l := s.lines[i]; !l.ruled {
			targets = append(targets, Target{Y: s.top + 2 + i - from, Width: s.width, On: OnSession, Session: l.session.Session})
		}
	}
	return targets
}
