package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The List's places, in cells: where its rows' words start, its mark two
// cells before them; where routing starts, wide; the fewest cells routing
// is cut to before it's left out, as at 115 columns; and how far the ended
// rule's rule is from its words.
const (
	listFrom         = 2
	listRouting      = 91
	listRoutingLeast = 24
	endedApart       = 2
)

// The List's words where it has no session to list: where its rows would
// be, and as its today line.
var (
	noSessionsToday = line{{"no sessions today", faintInk}}
	endedToday      = line{{"ended today", dimInk}}
)

// listColumn is a column of the List: its head; from column x, width cells
// wide, routing's to the last column; how its words sit; and what it says
// of a session, in its cells, as the List has it laid out.
type listColumn struct {
	head     string
	x, width int
	aligned  alignment
	says     func(l listed, s views.ListedSession, cells int) line
}

// The List's columns, wide and, under 100 columns, on a phone, as its frames
// lay them out; and routing, wide, where it has room.
var (
	wideList = []listColumn{
		{"session", listFrom, 17, alignLeft, sessionSays},
		{"on", 21, 8, alignLeft, onSays},
		{"model", 31, 9, alignLeft, modelSays},
		{"state", 42, 11, alignLeft, stateSays},
		{"started", 55, 15, alignLeft, startedSays},
		{"requests", 72, 8, alignRight, requestsSays},
		{"worth", 82, 7, alignRight, worthSays},
	}
	phoneList = []listColumn{
		{"session", listFrom, 16, alignLeft, sessionSays},
		{"on", 19, 8, alignLeft, onSays},
		{"state", 29, 11, alignLeft, stateSays},
		{"requests", 44, 8, alignRight, requestsSays},
	}
	routingColumn = listColumn{head: "routing", x: listRouting, aligned: alignLeft, says: routingSays}
)

// listLine is a line of the List's table: a session's row; the rule over
// those that ended today; or, with neither, the words saying it has none.
type listLine struct {
	session *views.ListedSession
	ruled   bool
}

// listed is the List as a frame lays it out of a moment at now: its columns;
// its lines; its today line, a row each, the last on the row over the key
// line; its heads' row; and how many rows its lines show in under them.
type listed struct {
	f       Frame
	m       status.Moment
	now     time.Time
	columns []listColumn
	lines   []listLine
	today   []line
	top     int
	rows    int
}

// listForm is Sessions' List as its form draws it, of doc at now, once the
// watch has built it: nothing before.
func (f Frame) listForm(doc status.Document, now time.Time) sessionsForm {
	if f.SessionsTab.List.GeneratedAt.IsZero() {
		return sessionsForm{}
	}
	l := f.listing(doc, now)
	return sessionsForm{draw: l.draw, scrolling: l.scrolling, scrolls: l.scrolls, targets: l.targets}
}

// listing is the List of doc at now, laid out as the frame has it: its
// lines from the row under its heads to the one before the blank row over
// its today line.
func (f Frame) listing(doc status.Document, now time.Time) listed {
	l := listed{
		f: f, m: f.Moment(doc, now), now: now, columns: f.listColumns(),
		lines: listLines(f.SessionsTab.List.Sessions), today: todayLine(f.SessionsTab.List.Today, f.Width), top: f.top(),
	}
	l.rows = max(l.todayAt()-l.top-2, 0)
	return l
}

// listColumns are the List's columns at the frame's width: a phone's four
// under 100 columns; else the wide ones, and routing to the last column,
// where it has listRoutingLeast cells.
func (f Frame) listColumns() []listColumn {
	if f.phone() {
		return phoneList
	}
	room := f.Width - listRouting
	if room < listRoutingLeast {
		return wideList
	}
	routing := routingColumn
	routing.width = room
	return slices.Concat(wideList, []listColumn{routing})
}

// listLines are the List's lines of its sessions, in the order it lists
// them: those running, then, where any ended today, the ended rule and
// them; or, of none, the words saying so.
func listLines(sessions []views.ListedSession) []listLine {
	if len(sessions) == 0 {
		return []listLine{{}}
	}
	lines := make([]listLine, 0, len(sessions)+1)
	for i := range sessions {
		s := &sessions[i]
		if !s.Running && (i == 0 || sessions[i-1].Running) {
			lines = append(lines, listLine{ruled: true})
		}
		lines = append(lines, listLine{session: s})
	}
	return lines
}

// todayLine is the List's today line, width cells wide, its rows top to
// bottom: on one row where it fits; else broken after the requests, the
// worth on a row of its own. Of no sessions, it says so.
func todayLine(t views.SessionsToday, width int) []line {
	said, apart := line{{"today  ", dimInk}}, line{{" · ", mutedInk}}
	if t.Sessions == 0 {
		return []line{slices.Concat(said, line{{prose.SessionCount(0), mutedInk}})}
	}
	counts := slices.Concat(said, logCounted(t.Sessions, "session", secondaryInk), apart, logCounted(t.Requests, "request", secondaryInk))
	worth := todayWorth(t.Worth)
	if whole := slices.Concat(counts, apart, worth); whole.width() <= width {
		return []line{whole}
	}
	return []line{counts.fit(width), worth.fit(width)}
}

// todayWorth is what today's sessions were worth, to the cent, bold
// text.secondary, " at API prices" after it in text.muted, but where none
// of it is priced.
func todayWorth(w views.Worth) line {
	said := line{{w.Cents(), strongInk}}
	if w.Amount == nil && len(w.Unpriced) > 0 {
		return said
	}
	return append(said, span{" at API prices", mutedInk})
}

// draw draws the List from row top: its heads; its lines in view, as far as
// the frame has them scrolled, the session chosen lit across the width; and
// its today line, right-aligned to the last column, held over the key line.
func (l listed) draw(c *canvas, top int) {
	l.heads(c, top)
	first, last := l.shown()
	for i := first; i < last; i++ {
		l.drawLine(c, top+1+i-first, l.lines[i])
	}
	for i, said := range l.today {
		c.right(l.f.Width, l.todayAt()+i, said)
	}
}

// todayAt is the row the List's today line starts on: its last row the one
// over the key line.
func (l listed) todayAt() int {
	return l.f.Height - underView - len(l.today) + 1
}

// heads draws the List's heads along row y, in text.subtle: each at its
// column's start, or ending at its end where it's right-aligned.
func (l listed) heads(c *canvas, y int) {
	for _, col := range l.columns {
		l.cell(c, y, col, line{{col.head, dimInk}})
	}
}

// cell draws what a column says along row y, in its cells.
func (l listed) cell(c *canvas, y int, col listColumn, said line) {
	if col.aligned == alignRight {
		c.right(col.x+col.width, y, said)
		return
	}
	c.line(col.x, y, said)
}

// shown are the List's lines in view, from first to before last, as far as
// the frame has them scrolled.
func (l listed) shown() (first, last int) {
	first = l.scrolling(0).clamped(l.f.Scroll)
	return first, min(first+l.rows, len(l.lines))
}

// drawLine draws the line ll along row y: a session's row, its mark two
// cells before its first column, lit where it's chosen; the ended rule, to
// the last column; or the words saying there's none.
func (l listed) drawLine(c *canvas, y int, ll listLine) {
	switch {
	case ll.ruled:
		x := c.line(listFrom, y, endedToday) + endedApart
		c.line(x, y, line{{rule(l.f.Width - x), borderInk}})
	case ll.session == nil:
		c.line(listFrom, y, noSessionsToday.fit(l.f.Width-listFrom))
	default:
		l.row(c, y, *ll.session)
	}
}

// row draws the session's row along row y: its mark, bold, at column 0, as
// the Log's; each column's words, cut to its cells; lit across the width
// where it's chosen.
func (l listed) row(c *canvas, y int, s views.ListedSession) {
	c.line(0, y, line{{s.Mark.String(), rowMarkInks[s.Mark]}})
	for _, col := range l.columns {
		l.cell(c, y, col, col.says(l, s, col.width).fit(col.width))
	}
	if l.f.SessionsTab.Chosen.Session == s.Session {
		c.surface(0, y, l.f.Width, litInk)
	}
}

// scrolling is how far the List's lines scroll under its heads, a row at a
// time, at every offset alike.
func (l listed) scrolling(int) Scrolling {
	return byRows(len(l.lines), l.rows)
}

// scrolls is what of the List scrolls: its lines under its heads, across
// the frame.
func (l listed) scrolls() (x, y, width, rows int) {
	return 0, l.top + 1, l.f.Width, l.rows
}

// targets are the List's sessions' rows in view, a click on which chooses
// the session, and opens the one chosen, each across the width.
func (l listed) targets() []Target {
	var targets []Target
	first, last := l.shown()
	for i := first; i < last; i++ {
		if s := l.lines[i].session; s != nil {
			targets = append(targets, Target{Y: l.top + 1 + i - first, Width: l.f.Width, On: OnSession, Session: s.Session})
		}
	}
	return targets
}

// sessionSays names the session: its id in text.secondary, a space and its
// directory, cut to the rest of the cells, in text.muted.
func sessionSays(_ listed, s views.ListedSession, cells int) line {
	said := line{{status.ShownID(s.Session), secondaryInk}}
	if dir := status.DirName(s.Dir, cells-status.IDShown-1); dir != "" {
		said = append(said, span{" " + dir, mutedInk})
	}
	return said
}

// onSays names the account its latest request went to, in text.primary.
func onSays(l listed, s views.ListedSession, _ int) line {
	if s.Account == "" {
		return nil
	}
	return line{{named(l.m, s.Account), accountInk}}
}

// modelSays is the model of its latest request, its id less claude-, in
// text.muted.
func modelSays(_ listed, s views.ListedSession, _ int) line {
	return line{{modelNamed(s.Model), mutedInk}}
}

// listStateInks are the inks of what a session's doing, as the List's state
// says it, by what it's doing: asking and answering bold accent.key; a
// refusal bold state.destructive; throttled text.subtle, as the
// Switchboard's; idle and ended text.muted.
var listStateInks = map[views.Doing]ink{
	views.Asking: keyInk, views.Answering: keyInk, views.Refused: exhaustedInk, views.Throttled: dimInk,
	views.Idle: mutedInk, views.Ended: mutedInk,
}

// stateSays is what it's doing at now, as its Activity's State says it.
func stateSays(l listed, s views.ListedSession, _ int) line {
	return line{{s.Activity.State(l.now), listStateInks[s.Activity.Doing]}}
}

// startedSays is when its first request came, as prose.Past says it, in
// text.muted: nothing where that isn't known.
func startedSays(l listed, s views.ListedSession, _ int) line {
	if s.Started.IsZero() {
		return nil
	}
	return line{{prose.Past(l.now, s.Started), mutedInk}}
}

// requestsSays counts its requests today, in text.muted.
func requestsSays(_ listed, s views.ListedSession, _ int) line {
	return line{{prose.Grouped(s.Requests), mutedInk}}
}

// worthSays is what its requests today were worth, to the cent, as the Log
// says a worth, in text.muted: — in text.faint where it made none today.
func worthSays(_ listed, s views.ListedSession, _ int) line {
	if s.Requests == 0 {
		return line{{"—", faintInk}}
	}
	return line{{s.Cents(), mutedInk}}
}

// routingSays is how it came to the account it's on, in the Log's routing
// words, cut to the cells as the Log cuts them, a move's cost kept whole.
func routingSays(_ listed, s views.ListedSession, cells int) line {
	return rowWordsIn(s.Routing.Said(), cells)
}
