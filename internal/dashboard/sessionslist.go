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
// most; started at the cells given where they're more than its least; and
// routing taking what's left to the last column.
//
// No natural comes from the rows shown, but started's from the whole log,
// so the columns hold still as the List scrolls.
func listParts(on, started int) []flex.Part {
	parts := make([]flex.Part, len(listPartFacts))
	for p, facts := range listPartFacts {
		parts[p] = flex.Part{
			Least: facts.least, Natural: cmp.Or(facts.natural, facts.least),
			Narrows: facts.narrows, Leaves: facts.leaves, Gap: listGap,
		}
	}
	parts[listOn].Least, parts[listOn].Natural = min(on, listOnLeast), on
	started = max(started, parts[listStarted].Least)
	parts[listStarted].Least, parts[listStarted].Natural = started, started
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
// wide, on's natural the cells given, at least its head's, and started the
// cells given, as listParts has them, in two passes of
// flex, as the Log's Requests are: the first decides which of listParts
// show; the second lays out only those, as flex's Decided has them, so
// what's past the leasts goes to the session first, then on, in the row's
// order. Nothing grows but routing, so where it's left, the spare lies at
// the row's end.
func listColumnsOf(width, on, started int) []listColumn {
	room := width - listFrom
	parts := listParts(max(on, ansi.StringWidth(listPartFacts[listOn].head)), started)
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
	return columnsAt(spans, shown)
}

// columnsAt are the columns of the kinds given where the spans given lay
// them, as spannedColumns has them.
func columnsAt(spans []flex.Span, kinds []listPart) []listColumn {
	return spannedColumns(spans, kinds, func(p listPart, x, width int) listColumn { return listColumn{part: p, x: x, width: width} })
}

// spannedColumns are the columns of the kinds given where the spans given lay
// them, kinds[i] laid at spans[i], from listFrom, each as column makes it:
// none of a span that gave way.
func spannedColumns[P, C any](spans []flex.Span, kinds []P, column func(part P, x, width int) C) []C {
	columns := make([]C, 0, len(kinds))
	for i, p := range kinds {
		if !spans[i].Left {
			columns = append(columns, column(p, listFrom+spans[i].At, spans[i].Size))
		}
	}
	return columns
}

// head is the column's head.
func (c listColumn) head() string {
	return listPartFacts[c.part].head
}

// listed is the List as a frame lays it out of a moment at now, its heads
// on row top: the log's sessions by day, in the List's own columns; the
// log's period; its period line, a row each; how many rows its lines show
// in under its heads, a page; and whether the row over the key line says
// how many sessions are out of view, the period line a row up for it.
type listed struct {
	sessionsByDay
	f      Frame
	period views.Period
	foot   []line
	top    int
	page   int
	marked bool
}

// sessionsOut are the forms of the line saying how many of the List's
// sessions are out of view, as Detail's list says its accounts: "▲ 12 more
// sessions · ▼ 130 more sessions", and for less room, the counts alone.
var sessionsOut = []outForm{{noun: "session", gap: " · ", inks: keyedOut}, {gap: " · ", inks: keyedOut}}

// LogMeasured are the cells of the widest start the ended sessions of the
// List's log say, as the watch measures them as it builds the log, and the
// log it measured, so the List's started column is as wide as the log
// needs, whatever it scrolls to.
type LogMeasured struct {
	of      *views.SessionLog
	started int
}

// MeasureLog measures the ended sessions of the log given at now, as
// LogMeasured has them.
func MeasureLog(log *views.SessionLog, now time.Time) LogMeasured {
	words := sessionWords{now: now}
	return LogMeasured{of: log, started: words.widest(listStarted, log.Sessions, sessionEnded)}
}

// listForm is Sessions' List as its form draws it, of doc at now, once the
// watch has built its log of the period: nothing before, as its heading
// says it's loading.
func (f Frame) listForm(doc status.Document, now time.Time) sessionsForm {
	if f.SessionsTab.Log == nil {
		return sessionsForm{}
	}
	l := f.listing(doc, now)
	return sessionsForm{draw: l.draw, scrolling: l.scrolling, scrolls: l.scrolls, targets: l.targets}
}

// listing is the List of doc at now laid out as the frame has it: its lines
// from the row under its heads down to its period line, over the key line;
// and where they don't all fit, the row over the key line saying how many
// sessions are out of view, so long as that leaves them a row, the period
// line a row up over it. Its day rules show where its page has room, as
// sessionsByDay's within has it. The view's least, leastView, leaves its
// lines a row under the period line's two at their most. Its started column
// is as wide as the widest start of the whole log, the ended as measured
// has them, where it's of the log, and those running as they say now.
func (f Frame) listing(doc status.Document, now time.Time) listed {
	m := f.Moment(doc, now)
	log, words := f.SessionsTab.Log, sessionWords{m: m, now: now}
	measured := f.SessionsTab.Measured
	if measured.of != log {
		measured = MeasureLog(log, now)
	}
	started := max(measured.started, words.widest(listStarted, log.Sessions, sessionRunning))
	rows := f.Height - underView - f.top()
	l := listed{
		f: f, period: log.Period, top: f.top(), foot: periodLine(*log, f.Width),
		words: words, columns: listColumnsOf(f.Width, widestLabel(m), started),
		lines: sessionLines(log.Sessions), width: f.Width, lit: f.SessionsTab.Chosen.Session,
	}
	l.page = rows - len(l.foot)
	if l.marked = len(l.lines) > l.page && l.page > 1; l.marked {
		l.page--
	}
	l.sessionsByDay = l.within(l.page)
	return l
}

// periodLine is the List's period line, width cells wide, its rows top to
// bottom: the period's name, then its sessions, their requests and their
// worth, on one row where it fits; else broken after the requests, the worth
// on a row of its own. Of no sessions, it says so.
func periodLine(log views.SessionLog, width int) []line {
	said, apart := line{{log.Period.Name() + "  ", dimInk}}, line{{" · ", mutedInk}}
	if log.All.Sessions == 0 {
		return []line{slices.Concat(said, line{{prose.SessionCount(0), mutedInk}})}
	}
	counts := slices.Concat(said, logCounted(log.All.Sessions, "session", secondaryInk), apart, logCounted(log.All.Requests, "request", secondaryInk))
	worth := periodWorth(log.All.Worth)
	if whole := slices.Concat(counts, apart, worth); whole.width() <= width {
		return []line{whole}
	}
	return []line{counts.fit(width), worth.fit(width)}
}

// periodWorth is what the period's sessions were worth, to the cent, its
// thousands set apart, bold text.secondary, " at API prices" after it in
// text.muted, but where none of it is priced.
func periodWorth(w views.Worth) line {
	said := line{{w.GroupedCents(), strongInk}}
	if w.Amount == nil && len(w.Unpriced) > 0 {
		return said
	}
	return append(said, span{" at API prices", mutedInk})
}

// noSessionsIn are the List's words where it has no session of the period
// to list, in place of its rows, the period as its Within says it: "no
// sessions today", "no sessions this week" or "no sessions in the last 30
// days".
func noSessionsIn(p views.Period) line {
	return line{{prose.SessionCount(0) + " " + p.Within(), faintInk}}
}

// draw draws the List from row top: its heads; its lines in view, as far as
// the frame has them scrolled, a day's rule kept on the first row while its
// sessions scroll under it, the session chosen lit across the width, or the
// words saying there's none; its period line from column 0, as the key line
// is, held over it; and where they show, how many sessions are out of view,
// on the row over the key line.
func (l listed) draw(c *canvas, top int) {
	columnHeads(c, top, l.columns)
	if len(l.lines) == 0 {
		c.line(listFrom, top+1, noSessionsIn(l.period).fit(l.f.Width-listFrom))
	}
	paged := l.paged()
	l.drawPage(c, top+1, paged, l.f.Scroll)
	at := l.f.stateRow() - len(l.foot) + 1
	if l.marked {
		at--
		from, to := paged.shown(l.f.Scroll)
		above, below := l.outOfView(paged.top(from, to), to)
		c.line(listFrom, l.f.stateRow(), outOfView(above, below, l.f.Width-listFrom, sessionsOut...))
	}
	for i, said := range l.foot {
		c.line(0, at+i, said)
	}
}

// ListScroll is how far Sessions' List is scrolled, by rows, to show the
// session chosen, as listing lays it out of doc at now: as far as the
// frame's Scroll has it, where that shows it, else as little further as
// shows it, as its rows' places have it; as far as Scroll has it where
// there's no log, or the session chosen isn't in it.
func (f Frame) ListScroll(doc status.Document, now time.Time) int {
	if f.SessionsTab.Log == nil {
		return f.Scroll
	}
	l := f.listing(doc, now)
	r := l.lineOf(f.SessionsTab.Chosen.Session)
	if r < 0 {
		return f.Scroll
	}
	return l.paged().revealing(f.Scroll, r)
}

// paged is how the List's lines page in its rows, a row at a time, each
// day's rule shown with the first session under it, or not at all, and held
// over the first row while any of its sessions shows there, hiding none of
// them, so the day always shows.
func (l listed) paged() rowsPaged {
	return rowsPaged{rows: l.page, content: len(l.lines), heads: l.heads(), kept: l.kept(), inserts: true}
}

// scrolling is how far the List's lines scroll under its heads, a row at a
// time, scrolled offset down, as paged has them.
func (l listed) scrolling(offset int) Scrolling {
	return l.paged().scrolling(offset)
}

// scrolls is what of the List scrolls: its lines under its heads, across
// the frame.
func (l listed) scrolls() (x, y, width, rows int) {
	return 0, l.top + 1, l.f.Width, l.page
}

// targets are the List's sessions' rows in view, a click on which chooses
// the session, and opens the one chosen, each across the width, as
// sessionsByDay's targets has them.
func (l listed) targets() []Target {
	return l.sessionsByDay.targets(l.top+1, l.paged(), l.f.Scroll)
}

// columnHeads draws the heads of the columns given along row y, in
// text.subtle: each at its column's start, or ending at its end where it's
// right-aligned.
func columnHeads(c *canvas, y int, columns []listColumn) {
	for _, col := range columns {
		col.draw(c, y, line{{col.head(), dimInk}})
	}
}

// draw draws what the column says along row y, in its cells.
func (col listColumn) draw(c *canvas, y int, said line) {
	if listPartFacts[col.part].aligned == alignRight {
		c.right(col.x+col.width, y, said)
		return
	}
	c.line(col.x, y, said)
}

// endedRule draws the rule over the sessions that ended on a day along row
// y, across a view width cells wide: its words, said, from listFrom, then
// endedApart cells on, the rule to the last column.
func endedRule(c *canvas, y, width int, said line) {
	x := c.line(listFrom, y, said) + endedApart
	c.line(x, y, line{{rule(width - x), borderInk}})
}

// sessionRow draws the session's row along row y in the columns given, as
// the List draws one: its mark, bold, at column 0, as the Log's, but a mark
// of what's in flight as last known, as asKnown has it; each column's
// words, as says has them, cut to its cells; and where lit says, lit across
// width cells.
func (w sessionWords) sessionRow(c *canvas, y int, columns []listColumn, s views.ListedSession, width int, lit bool) {
	mark := rowMarkInks[s.Mark]
	if s.Mark == views.MarkAsking || s.Mark == views.MarkAnswering {
		mark = asKnown(s.Activity, mark)
	}
	c.line(0, y, line{{s.Mark.String(), mark}})
	for _, col := range columns {
		col.draw(c, y, w.says(col, s).fit(col.width))
	}
	if lit {
		c.surface(0, y, width, litInk)
	}
}

// sessionWords say what the List's columns say of a session, of the moment
// m at now, grouped by the day each ended, as the List and Projects'
// SESSIONS group them: its end and its start said on that day, and a start
// on another day with that day, as the rules name the days.
type sessionWords struct {
	m   status.Moment
	now time.Time
}

// says is what the column says of the session, in its cells: the session
// named as sessionIn names it, its id in text.secondary; the model of its
// latest request, its id less claude-, and its requests counted, in
// text.muted; its routing in the Log's routing words, cut as the Log cuts
// them, a move's cost kept whole; and the rest as their own say them.
func (w sessionWords) says(col listColumn, s views.ListedSession) line {
	switch col.part {
	case listSession:
		return sessionIn(s.Session, s.Dir, secondaryInk, col.width)
	case listOn:
		return w.onSays(s)
	case listModel:
		return line{{modelNamed(s.Model), mutedInk}}
	case listState:
		return w.stateSays(s)
	case listStarted:
		return w.startedSays(s)
	case listRequests:
		return line{{prose.Grouped(s.Requests), mutedInk}}
	case listWorth:
		return worthSays(s)
	default:
		return rowWordsIn(s.Routing.Said(), col.width)
	}
}

// sessionIn names the session with the given id, running in dir, in the
// cells given, as a column of sessions names one: its id, as status.ShownID
// has it, in the ink given; then a space and its directory, as
// status.DirName cuts it to the rest, in text.muted; its id alone where dir
// names none, or no cell is left for it.
func sessionIn(id, dir string, idInk ink, cells int) line {
	said := line{{status.ShownID(id), idInk}}
	if dir := status.DirName(dir, cells-status.IDShown-1); dir != "" {
		said = append(said, span{" " + dir, mutedInk})
	}
	return said
}

// onSays names the account its latest request went to, in text.primary.
func (w sessionWords) onSays(s views.ListedSession) line {
	if s.Account == "" {
		return nil
	}
	return line{{named(w.m, s.Account), accountInk}}
}

// listStateInks are the inks of what a session's doing, as the List's state
// says it, by what it's doing: asking and answering bold accent.key; a
// refusal bold state.destructive; throttled text.subtle, as the
// Switchboard's; idle and ended text.muted.
var listStateInks = map[views.Doing]ink{
	views.Asking: keyInk, views.Answering: keyInk, views.Refused: exhaustedInk, views.Throttled: dimInk,
	views.Idle: mutedInk, views.Ended: mutedInk,
}

// stateSays is what it's doing at now, as its Activity's DayState says it,
// as last known where asKnown says.
func (w sessionWords) stateSays(s views.ListedSession) line {
	return line{{s.Activity.DayState(w.now), asKnown(s.Activity, listStateInks[s.Activity.Doing])}}
}

// startedSays is when its first request came, in text.muted: by the clock
// alone where it came on the day the session's grouped under; else as
// prose.Begun says it, its day as the rules name days, as "yesterday 15:08"
// or "Mon 21 Sep 09:12"; nothing where that isn't known.
func (w sessionWords) startedSays(s views.ListedSession) line {
	switch {
	case s.Started.IsZero():
		return nil
	case prose.SameDay(w.now, w.dayOf(s), s.Started):
		return line{{prose.TimeOfDay(w.now, s.Started), mutedInk}}
	}
	return line{{prose.Begun(w.now, s.Started), mutedInk}}
}

// dayOf is a time on the day the session's grouped under, in now's time
// zone: when it ended, or now, while it runs.
func (w sessionWords) dayOf(s views.ListedSession) time.Time {
	if s.Activity.Doing == views.Ended {
		return s.Ended.In(w.now.Location())
	}
	return w.now
}

// worthSays is what its requests in the period were worth, to the cent, as
// the Log says a worth, in text.muted: — in text.faint where it made none.
func worthSays(s views.ListedSession) line {
	if s.Requests == 0 {
		return line{{"—", faintInk}}
	}
	return line{{s.Cents(), mutedInk}}
}
