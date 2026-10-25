package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The List's places, in cells: where its rows' words start, its mark two
// cells before them; the cells between two columns; and how far the ended
// rule's rule is from its words.
const (
	listFrom   = 2
	listGap    = 2
	endedApart = 2
)

// The List's columns' widths, in cells: the session's least, its id and
// listDirLeast cells of its directory, and its natural, its id and its
// directory at its most; and the most on's least takes, where the widest
// name is wider.
const (
	listDirLeast     = 6
	listSessionLeast = status.IDShown + 1 + listDirLeast
	listSessionWhole = status.IDShown + 1 + views.SessionDirMost
	listOnLeast      = 14
)

// The List's words where it has no session to list: where its rows would
// be, and as its today line.
var (
	noSessionsToday = line{{"no sessions today", faintInk}}
	endedToday      = line{{"ended today", dimInk}}
)

// listPart is one of the List's columns.
type listPart int

// The List's columns, in the row's order.
const (
	listSession listPart = iota
	listOn
	listModel
	listState
	listStarted
	listRequests
	listWorth
	listRouting
)

// The turns the List's columns give way in as it narrows, in this order, so
// routing never shows beside either name cut short, nor worth or started
// beside the session cut short: routing leaves while the leasts and the
// session and on whole don't fit; on narrows toward its least; worth, then
// started, leave while the leasts and the session whole don't fit; the
// session narrows toward its least; and model leaves only while the leasts
// don't fit, so it leaves before on is cut below listOnLeast.
const (
	routingLeaves = iota + 1
	onNarrows
	worthLeaves
	startedLeaves
	sessionNarrows
	modelLeaves
)

// listPartFacts are each column's head; its least and its natural, 0 for
// its least, on's both placeholders for the widest name, as listParts gives
// them; how its words sit; and the turns it narrows and leaves in, 0 never.
var listPartFacts = [...]struct {
	head            string
	least, natural  int
	aligned         alignment
	narrows, leaves int
}{
	listSession:  {"session", listSessionLeast, listSessionWhole, alignLeft, sessionNarrows, 0},
	listOn:       {"on", 0, 0, alignLeft, onNarrows, 0},
	listModel:    {"model", 9, 0, alignLeft, 0, modelLeaves},
	listState:    {"state", 11, 0, alignLeft, 0, 0},
	listStarted:  {"started", 15, 0, alignLeft, 0, startedLeaves},
	listRequests: {"requests", 8, 0, alignRight, 0, 0},
	listWorth:    {"worth", 7, 0, alignRight, 0, worthLeaves},
	listRouting:  {"routing", routingLeast, 0, alignLeft, 0, routingLeaves},
}

// listParts are the List's columns as flex lays them, listGap apart, each
// from its least to its natural, holding its natural until it narrows in its
// turn: on's natural the cells given, its least that, listOnLeast at the
// most; and routing taking what's left to the last column.
//
// No natural comes from the rows shown, so the columns hold still as the
// List scrolls and as sessions come and go.
func listParts(on int) []flex.Part {
	parts := make([]flex.Part, len(listPartFacts))
	for p, facts := range listPartFacts {
		parts[p] = flex.Part{
			Least: facts.least, Natural: cmp.Or(facts.natural, facts.least),
			Narrows: facts.narrows, Leaves: facts.leaves, Gap: listGap,
		}
	}
	parts[listOn].Least, parts[listOn].Natural = min(on, listOnLeast), on
	parts[listRouting].Grows = true
	return parts
}

// listColumn is a column of the List as it's laid out: which it is, from
// column x, width cells wide.
type listColumn struct {
	part     listPart
	x, width int
}

// listColumnsOf are the List's columns shown across a frame width cells
// wide, on's natural the cells given, at least its head's, in two passes of
// flex, as the Log's Requests are: the first decides which of listParts
// show; the second lays out only those, as flex's Decided has them, so
// what's past the leasts goes to the session first, then on, in the row's
// order. Nothing grows but routing, so where it's left, the spare lies at
// the row's end.
func listColumnsOf(width, on int) []listColumn {
	room := width - listFrom
	parts := listParts(max(on, ansi.StringWidth(listPartFacts[listOn].head)))
	decided, _ := flex.Lay(room, parts...)
	var shown []listPart
	var kept []flex.Part
	for p, span := range decided {
		part, ok := parts[p].Decided(span)
		if !ok {
			continue
		}
		if listPart(p) == listOn {
			// What on gave up in the first pass, to keep worth or started, it
			// never takes back, so it never narrows as the frame widens.
			part.Natural = span.Size
		}
		shown, kept = append(shown, listPart(p)), append(kept, part)
	}
	spans, _ := flex.Lay(room, kept...)
	columns := make([]listColumn, len(shown))
	for i, p := range shown {
		columns[i] = listColumn{part: p, x: listFrom + spans[i].At, width: spans[i].Size}
	}
	return columns
}

// head is the column's head.
func (c listColumn) head() string {
	return listPartFacts[c.part].head
}

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
// its today line, the ended rule among them where ruledIn says those rows
// hold it.
func (f Frame) listing(doc status.Document, now time.Time) listed {
	m := f.Moment(doc, now)
	l := listed{
		f: f, m: m, now: now, columns: listColumnsOf(f.Width, widestLabel(m)),
		today: todayLine(f.SessionsTab.List.Today, f.Width), top: f.top(),
	}
	l.rows = max(l.todayAt()-l.top-2, 0)
	l.lines = listLines(f.SessionsTab.List.Sessions, ruledIn(l.rows))
	return l
}

// ruledIn reports whether the List's lines, rows of them, hold its ended
// rule with the first session under it, as flex lays the two down them: the
// rule shown with that session or not at all, so where they don't, the rule
// gives way and the ended sessions follow the running.
func ruledIn(rows int) bool {
	ruled, first := flex.Part{Least: 1, Natural: 1, Leaves: 1}, flex.Part{Least: 1, Natural: 1}
	spans, _ := flex.Lay(rows, ruled, first)
	return !spans[0].Left
}

// listLines are the List's lines of its sessions, in the order it lists
// them: those running, then, where any ended today, the ended rule, where
// ruled, and them; or, of none, the words saying so.
func listLines(sessions []views.ListedSession, ruled bool) []listLine {
	if len(sessions) == 0 {
		return []listLine{{}}
	}
	lines := make([]listLine, 0, len(sessions)+1)
	for i := range sessions {
		s := &sessions[i]
		if ruled && !s.Running && (i == 0 || sessions[i-1].Running) {
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
// its today line from column 0, as the key line is, held over it.
func (l listed) draw(c *canvas, top int) {
	l.heads(c, top)
	first, last := l.shown()
	for i := first; i < last; i++ {
		l.drawLine(c, top+1+i-first, l.lines[i])
	}
	for i, said := range l.today {
		c.line(0, l.todayAt()+i, said)
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
		l.cell(c, y, col, line{{col.head(), dimInk}})
	}
}

// cell draws what a column says along row y, in its cells.
func (l listed) cell(c *canvas, y int, col listColumn, said line) {
	if listPartFacts[col.part].aligned == alignRight {
		c.right(col.x+col.width, y, said)
		return
	}
	c.line(col.x, y, said)
}

// shown are the List's lines in view, from first to before last, as far as
// the frame has them scrolled, as paged has them.
func (l listed) shown() (first, last int) {
	return l.paged().shown(l.f.Scroll)
}

// paged is how the List's lines page in its rows, a row at a time, the
// ended rule shown with the first session under it, or not at all. It says
// nothing of what's out of view, as the Log's table doesn't.
func (l listed) paged() rowsPaged {
	var heads []headed
	for i, ll := range l.lines {
		if ll.ruled {
			heads = append(heads, headed{at: i, first: i + 1})
		}
	}
	return rowsPaged{rows: l.rows, content: len(l.lines), heads: heads}
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
// the Log's, but a mark of what's in flight as last known, as asKnown has
// it; each column's words, cut to its cells; lit across the width where it's
// chosen.
func (l listed) row(c *canvas, y int, s views.ListedSession) {
	mark := rowMarkInks[s.Mark]
	if s.Mark == views.MarkAsking || s.Mark == views.MarkAnswering {
		mark = asKnown(s.Activity, mark)
	}
	c.line(0, y, line{{s.Mark.String(), mark}})
	for _, col := range l.columns {
		l.cell(c, y, col, l.says(col, s).fit(col.width))
	}
	if l.f.SessionsTab.Chosen.Session == s.Session {
		c.surface(0, y, l.f.Width, litInk)
	}
}

// scrolling is how far the List's lines scroll under its heads, a row at a
// time, scrolled offset down, as paged has them.
func (l listed) scrolling(offset int) Scrolling {
	return l.paged().scrolling(offset)
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

// says is what the column says of the session, in its cells: the model of
// its latest request, its id less claude-, and its requests today counted,
// in text.muted; its routing in the Log's routing words, cut as the Log cuts
// them, a move's cost kept whole; and the rest as their own say them.
func (l listed) says(col listColumn, s views.ListedSession) line {
	switch col.part {
	case listSession:
		return sessionSays(s, col.width)
	case listOn:
		return l.onSays(s)
	case listModel:
		return line{{modelNamed(s.Model), mutedInk}}
	case listState:
		return l.stateSays(s)
	case listStarted:
		return l.startedSays(s)
	case listRequests:
		return line{{prose.Grouped(s.Requests), mutedInk}}
	case listWorth:
		return worthSays(s)
	default:
		return rowWordsIn(s.Routing.Said(), col.width)
	}
}

// sessionSays names the session in the cells given: its id in
// text.secondary, a space and its directory, cut to the rest, in text.muted.
func sessionSays(s views.ListedSession, cells int) line {
	said := line{{status.ShownID(s.Session), secondaryInk}}
	if dir := status.DirName(s.Dir, cells-status.IDShown-1); dir != "" {
		said = append(said, span{" " + dir, mutedInk})
	}
	return said
}

// onSays names the account its latest request went to, in text.primary.
func (l listed) onSays(s views.ListedSession) line {
	if s.Account == "" {
		return nil
	}
	return line{{named(l.m, s.Account), accountInk}}
}

// listStateInks are the inks of what a session's doing, as the List's state
// says it, by what it's doing: asking and answering bold accent.key; a
// refusal bold state.destructive; throttled text.subtle, as the
// Switchboard's; idle and ended text.muted.
var listStateInks = map[views.Doing]ink{
	views.Asking: keyInk, views.Answering: keyInk, views.Refused: exhaustedInk, views.Throttled: dimInk,
	views.Idle: mutedInk, views.Ended: mutedInk,
}

// stateSays is what it's doing at now, as its Activity's State says it, as
// last known where asKnown says.
func (l listed) stateSays(s views.ListedSession) line {
	return line{{s.Activity.State(l.now), asKnown(s.Activity, listStateInks[s.Activity.Doing])}}
}

// startedSays is when its first request came, as prose.Past says it, in
// text.muted: nothing where that isn't known.
func (l listed) startedSays(s views.ListedSession) line {
	if s.Started.IsZero() {
		return nil
	}
	return line{{prose.Past(l.now, s.Started), mutedInk}}
}

// worthSays is what its requests today were worth, to the cent, as the Log
// says a worth, in text.muted: — in text.faint where it made none today.
func worthSays(s views.ListedSession) line {
	if s.Requests == 0 {
		return line{{"—", faintInk}}
	}
	return line{{s.Cents(), mutedInk}}
}
