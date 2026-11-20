package dashboard

import (
	"cmp"
	"math"
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

// The List's columns' widths, in cells: a session's directory's least, as
// the Overview's SESSIONS cuts it; the session's least, listProjectLeast
// cells of its project, as the List cuts one before its name, and as many
// of its name as the dashboard shows of an id; its natural at its most, a
// project and a name of listNameMost cells each, either taking what the
// other leaves; and the most on's least takes, where the widest name is
// wider.
const (
	listDirLeast     = 6
	listProjectLeast = 6
	listNameMost     = 14
	listSessionLeast = listProjectLeast + 1 + status.IDShown
	listSessionWhole = 2*listNameMost + 1
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
// its least, on's both placeholders for the widest name, the session's
// natural a placeholder for its project and name whole, and model's,
// started's and worth's each raised to its widest said, as listParts gives
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

// listNaturals are the cells the List's columns take whole where they're
// what the List holds says: on the widest account's name; model the widest
// model named; started the widest start said; worth the widest worth said,
// its + counted; and within the session's, project the widest project
// named, and name the widest name said.
type listNaturals struct {
	on, model, started, worth, project, name int
}

// listParts are the List's columns as flex lays them, listGap apart, each
// from its least to its natural, holding its natural until it narrows in its
// turn: the session's natural its project and name whole, as sessionNatural
// has it of sessionWhole; on's natural as given, its least that,
// listOnLeast at the most;
// model, started and worth whole wherever they show, their leasts and
// naturals the widest given where it's past their own, so routing never
// shows, nor the session grows past its least, beside one cut; and routing
// taking what's left to the last column.
//
// No natural comes from the rows shown, but from the whole log, so the
// columns hold still as the List scrolls.
func listParts(n listNaturals) []flex.Part {
	parts := make([]flex.Part, len(listPartFacts))
	for p, facts := range listPartFacts {
		parts[p] = flex.Part{
			Least: facts.least, Natural: cmp.Or(facts.natural, facts.least),
			Narrows: facts.narrows, Leaves: facts.leaves, Gap: listGap,
		}
	}
	parts[listSession].Natural = sessionNatural(sessionWhole(n))
	parts[listOn].Least, parts[listOn].Natural = min(n.on, listOnLeast), n.on
	for p, widest := range map[listPart]int{listModel: n.model, listStarted: n.started, listWorth: n.worth} {
		parts[p].Least = max(parts[p].Least, widest)
		parts[p].Natural = parts[p].Least
	}
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
// wide, of the naturals given, on's at least its head's, in two passes of
// flex, as the Log's Requests are: the first decides which of listParts
// show; the second lays out only those, as flex's Decided has them, so
// what's past the leasts goes to the session first, to its natural, then on,
// to its whole, in the row's order. In the second, the session grows past
// its natural to its project and name whole, as sessionWhole has them,
// before routing takes what's left, so no name is cut where the row has
// room; where routing's left, the session takes what's left itself, so the
// columns after it end at the row's end.
func listColumnsOf(width int, n listNaturals) []listColumn {
	room := width - listFrom
	n.on = max(n.on, ansi.StringWidth(listPartFacts[listOn].head))
	parts := listParts(n)
	decided, _ := flex.Lay(room, parts...)
	routingLeft := decided[listRouting].Left
	var shown []listPart
	var kept []flex.Part
	for p, span := range decided {
		part, ok := parts[p].Decided(span)
		if !ok {
			continue
		}
		if listPart(p) == listSession {
			part.Grows, part.Most = true, max(part.Natural, sessionWhole(n))
			if routingLeft {
				part.Most = 0
			}
		}
		shown, kept = append(shown, listPart(p)), append(kept, part)
	}
	spans, _ := flex.Lay(room, kept...)
	return columnsAt(spans, shown)
}

// sessionWhole is the cells the session column takes to show the widest
// project and the widest name, as the naturals given measure them, uncut, a
// cell apart: the name's alone where no session has a project.
func sessionWhole(n listNaturals) int {
	if n.project == 0 {
		return n.name
	}
	return n.project + 1 + n.name
}

// sessionNatural is the session column's natural, of the cells its widest
// is whole: those, at least listSessionLeast and at most listSessionWhole,
// so a log of short names leaves the other columns its cells.
func sessionNatural(whole int) int {
	return min(listSessionWhole, max(whole, listSessionLeast))
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
	foot   []line
	top    int
	page   int
	marked bool
}

// sessionsOut are the forms of the line saying how many of the List's
// sessions are out of view, as Detail's list says its accounts: "▲ 12 more
// sessions · ▼ 130 more sessions", and for less room, the counts alone.
var sessionsOut = []outForm{{noun: "session", gap: " · ", inks: keyedOut}, {gap: " · ", inks: keyedOut}}

// LogMeasured are the cells of the widest model, start and worth the ended
// sessions of the List's log say, as the watch measures them as it builds
// the log, and the log it measured, so those columns are as wide as the log
// needs, whatever it scrolls to, and a frame measures only the sessions
// running, not every session of a long period at every frame. project is
// the widest project they name, and name and marked the widest name they
// say, as nameSaid says it, plain and with Nerd Font's marks, so names line
// up under the project however the List scrolls.
type LogMeasured struct {
	of                    *views.SessionLog
	model, started, worth int
	project, name, marked int
}

// MeasureLog measures the sessions of the log given at now that hold still,
// as sessionStill has them, as LogMeasured has them, said as the log's
// period, at now or gone, has them said.
func MeasureLog(log *views.SessionLog, now time.Time) LogMeasured {
	words := sessionWords{now: now, gone: log.Gone}
	return LogMeasured{
		of: log, model: words.widest(listModel, log.Sessions, sessionStill),
		started: words.widest(listStarted, log.Sessions, sessionStill), worth: words.widest(listWorth, log.Sessions, sessionStill),
		project: widestSaid(log.Sessions, sessionStill, projectCells),
		name:    widestSaid(log.Sessions, sessionStill, nameCells(false)), marked: widestSaid(log.Sessions, sessionStill, nameCells(true)),
	}
}

// naturals are the List's naturals of the log's sessions given, said as
// words says them: on's its widest account's name, as widestLabel has it of
// words' moment; and model's, started's, worth's, project's and name's the
// widest the sessions say, the ended as measured, those running as they say
// now, a name as words' glyphs mark it.
func (m LogMeasured) naturals(words sessionWords, sessions []views.PeriodSession) listNaturals {
	name := m.name
	if words.glyphs {
		name = m.marked
	}
	return listNaturals{
		on:      widestLabel(words.m),
		model:   max(m.model, words.widest(listModel, sessions, sessionRunning)),
		started: max(m.started, words.widest(listStarted, sessions, sessionRunning)),
		worth:   max(m.worth, words.widest(listWorth, sessions, sessionRunning)),
		project: max(m.project, widestSaid(sessions, sessionRunning, projectCells)),
		name:    max(name, widestSaid(sessions, sessionRunning, nameCells(words.glyphs))),
	}
}

// widestSaid is the most cells any of the sessions of says measures, as cells
// measures each: none of none.
func widestSaid(sessions []views.PeriodSession, of func(views.PeriodSession) bool, cells func(views.PeriodSession) int) int {
	widest := 0
	for _, s := range sessions {
		if of(s) {
			widest = max(widest, cells(s))
		}
	}
	return widest
}

// projectCells are the cells the session's project takes, as the List
// names it.
func projectCells(s views.PeriodSession) int {
	return ansi.StringWidth(s.ProjectName)
}

// nameCells measures the cells a session's name takes, whole, as nameSaid
// says it, glyphs marking it.
func nameCells(glyphs nerdFont) func(views.PeriodSession) int {
	return func(s views.PeriodSession) int {
		return sessionWords{glyphs: glyphs}.nameSaid(s, math.MaxInt).width()
	}
}

// Of is what was measured, as of the log given: one views' SessionLog's
// Live told afresh of the log measured, its sessions ended saying what they
// said.
func (m LogMeasured) Of(log *views.SessionLog) LogMeasured {
	m.of = log
	return m
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
// lines a row under the period line's two at their most. Its model, started
// and worth columns are as wide as the widest of the whole log says, as
// LogMeasured's naturals has them, of measured where it's of the log.
func (f Frame) listing(doc status.Document, now time.Time) listed {
	m := f.Moment(doc, now)
	log := f.SessionsTab.Log
	words := sessionWords{m: m, now: now, gone: log.Gone, glyphs: nerdFont(f.NerdFont)}
	measured := f.SessionsTab.Measured
	if measured.of != log {
		measured = MeasureLog(log, now)
	}
	naturals := measured.naturals(words, log.Sessions)
	columns := listColumnsOf(f.Width, naturals)
	words.projectCells = projectCellsOf(sessionCells(columns), naturals)
	rows := f.Height - underView - f.top()
	l := listed{
		f: f, top: f.top(), foot: periodLine(*log, f.Dial, f.Width),
		words: words, columns: columns,
		lines: listLines(log.Sessions, log.Unkept, f.Dial, now), width: f.Width, lit: f.SessionsTab.Chosen.Session,
	}
	l.page = rows - len(l.foot)
	if l.marked = len(l.lines) > l.page && l.page > 1; l.marked {
		l.page--
	}
	l.sessionsByDay = l.within(l.page)
	return l
}

// periodLine is the List's period line, width cells wide, its rows top to
// bottom: the dial's period's name, in text.subtle, then what its sessions
// come to, as periodForms lays them, in the first form whose rows fit whole,
// so nothing of it is cut where a form fits; the period's name alone where
// the log tells no counts, as CountsTold has it.
func periodLine(log views.SessionLog, dial views.Dial, width int) []line {
	name := line{{dial.Name(), dimInk}}
	if !log.CountsTold() {
		return []line{name.fit(width)}
	}
	parts := []line{{{prose.SessionCount(0), mutedInk}}}
	forms := emptyPeriodForms
	if log.All.Sessions > 0 {
		parts = []line{
			logCounted(log.All.Sessions, "session", secondaryInk), logCounted(log.All.Requests, "request", secondaryInk),
			periodWorth(log.All.Worth),
		}
		forms = periodForms
	}
	var rows []line
	for _, form := range forms {
		if rows = form.rows(name, parts); rowsFit(rows, width) {
			return rows
		}
	}
	for i := range rows {
		rows[i] = rows[i].fit(width)
	}
	return rows
}

// periodForm is a form of the period line: the parts on its first row,
// after the period's name, and those on its second, by their place among
// the parts, none on a second row of a line of one row.
type periodForm struct {
	first, second []int
}

// The forms of the period line, the first first, of a period's sessions, its
// requests and its worth: all on one row; broken after the requests, the
// worth on a row of its own; the name alone over the rest; then, the
// requests given way, the sessions after the name and the worth under it;
// the name over the sessions and the worth; and the name over the worth,
// which never gives way. Of no sessions, saying so after the name, or under
// it.
var (
	periodForms = []periodForm{
		{first: []int{0, 1, 2}}, {first: []int{0, 1}, second: []int{2}}, {second: []int{0, 1, 2}},
		{first: []int{0}, second: []int{2}}, {second: []int{0, 2}}, {second: []int{2}},
	}
	emptyPeriodForms = []periodForm{{first: []int{0}}, {second: []int{0}}}
)

// rows are the form's rows of the parts given, after the period's name,
// each row's parts " · " apart, in text.muted, the name two cells before
// those on its row.
func (f periodForm) rows(name line, parts []line) []line {
	joined := func(at []int) line {
		var said line
		for i, p := range at {
			if i > 0 {
				said = append(said, span{" · ", mutedInk})
			}
			said = append(said, parts[p]...)
		}
		return said
	}
	first := slices.Clone(name)
	if len(f.first) > 0 {
		first = slices.Concat(first, line{{"  ", dimInk}}, joined(f.first))
	}
	if len(f.second) == 0 {
		return []line{first}
	}
	return []line{first, joined(f.second)}
}

// rowsFit reports whether every row given fits width cells whole.
func rowsFit(rows []line, width int) bool {
	for _, r := range rows {
		if r.width() > width {
			return false
		}
	}
	return true
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

// listLines are the lines of the sessions given of the dial's period at
// now, as the List and a project's SESSIONS list them: their own, as
// sessionLines has them, or of none, the words saying the period had none,
// as noSessionsIn says them, but where every day of it is no longer kept, as
// unkept tells; then, where any is, the words saying so, as unkept says
// them, in text.subtle.
func listLines(sessions []views.PeriodSession, unkept views.UnkeptDays, dial views.Dial, now time.Time) []sessionLine {
	lines := sessionLines(sessions)
	if len(lines) == 0 && !unkept.Every {
		lines = append(lines, sessionLine{said: noSessionsIn(dial)})
	}
	if said := unkept.Said(now); said != "" {
		lines = append(lines, sessionLine{said: line{{said, dimInk}}})
	}
	return lines
}

// noSessionsIn are the List's words, and Projects', where it has no session
// of the dial's period to list, in place of its rows, the period as its
// Within says it: "no sessions today", "no sessions this week", "no sessions
// in the last 30 days" or "no sessions on Tue 29 Sep"; in text.faint at now,
// and in text.subtle of a period gone.
func noSessionsIn(dial views.Dial) line {
	said := prose.SessionCount(0) + " " + dial.Within()
	if dial.Gone() {
		return line{{said, dimInk}}
	}
	return line{{said, faintInk}}
}

// draw draws the List from row top: its heads; its lines in view, as far as
// the frame has them scrolled, a day's rule kept on the first row while its
// sessions scroll under it, the session chosen lit across the width, as
// listLines has them; its period line from column 0, as the key line
// is, held over it; and where they show, how many sessions are out of view,
// on the row over the key line.
func (l listed) draw(c *canvas, top int) {
	columnHeads(c, top, l.columns)
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
	l, r, ok := f.listedChosen(doc, now)
	if !ok {
		return f.Scroll
	}
	return l.paged().revealing(f.Scroll, r)
}

// ListStepScroll is how far Sessions' List is scrolled once ↑ or ↓ chooses
// a session: as ListScroll has it; but where the session chosen is the last,
// as far as shows the words after it too, where that still shows the
// session, so a step to the foot shows the List's foot. Only a step goes
// there: a scroll or a click keeps the List where it was.
func (f Frame) ListStepScroll(doc status.Document, now time.Time) int {
	l, r, ok := f.listedChosen(doc, now)
	if !ok {
		return f.Scroll
	}
	paged := l.paged()
	offset := paged.revealing(f.Scroll, r)
	if l.lastRow() == r {
		if after := paged.revealing(offset, len(l.lines)-1); paged.shows(after, r) {
			return after
		}
	}
	return offset
}

// listedChosen is the List of doc at now as listing lays it out, and the
// line of the session chosen in it: false where there's no log, or the
// session chosen isn't in it.
func (f Frame) listedChosen(doc status.Document, now time.Time) (listed, int, bool) {
	if f.SessionsTab.Log == nil {
		return listed{}, 0, false
	}
	l := f.listing(doc, now)
	r := l.lineOf(f.SessionsTab.Chosen.Session)
	return l, r, r >= 0
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
func (w sessionWords) sessionRow(c *canvas, y int, columns []listColumn, s views.PeriodSession, width int, lit bool) {
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
// on another day with that day, as the rules name the days. Where
// projectCells gives its projects cells, as the List does, a session is
// named after its project, in a column of its own within the session's, as
// a project's SESSIONS, under the project, names one without it; and glyphs
// draws the marks of its name. gone says the sessions are of a period gone,
// whose starts are dated as startedSays dates them.
type sessionWords struct {
	m            status.Moment
	now          time.Time
	gone         bool
	projectCells int
	glyphs       nerdFont
}

// says is what the column says of the session, in its cells: the session
// named as named names it; the model of its latest request, its id less
// claude-, and its requests counted, in text.muted; its routing in a word or
// two, as its BriefSaid says it, each account named as the moment names it,
// cut as the Log cuts its routing, a move's cost kept whole; and the rest as
// their own say them.
func (w sessionWords) says(col listColumn, s views.PeriodSession) line {
	switch col.part {
	case listSession:
		return w.named(s, col.width)
	case listOn:
		return w.onSays(s.ListedSession)
	case listModel:
		return line{{modelNamed(s.Model), mutedInk}}
	case listState:
		return w.stateSays(s)
	case listStarted:
		return w.startedSays(s)
	case listRequests:
		return line{{prose.Grouped(s.Requests), mutedInk}}
	case listWorth:
		return worthSays(s.ListedSession)
	default:
		return rowWordsIn(s.Routing.BriefSaid(), func(id string) string { return named(w.m, id) }, col.width)
	}
}

// named names the session in the cells given: where w gives projects their
// projectCells, its project, as Projects names the project's row, cut to
// them with an ellipsis and padded to them, in text.muted, blank of a
// session of no project, so the names line up, then a space; then its name,
// as nameSaid has it, in the rest.
func (w sessionWords) named(s views.PeriodSession, cells int) line {
	if w.projectCells == 0 {
		return w.nameSaid(s, cells)
	}
	project := padded(prose.Cut(s.ProjectName, w.projectCells), w.projectCells) + " "
	return append(line{{project, mutedInk}}, w.nameSaid(s, cells-w.projectCells-1)...)
}

// nameSaid is the session's name in the cells given, as its ShownName has
// it: a worktree's marked as glyphs marks one, in text.secondary; then,
// where it shows its id, a space and the id in text.subtle, kept whole
// while the name keeps a cell, the name cut with an ellipsis to the rest.
func (w sessionWords) nameSaid(s views.PeriodSession, cells int) line {
	shown := s.ShownName()
	name := line{{w.glyphs.marked(shown), secondaryInk}}
	if shown.ID == "" {
		return name.fit(cells)
	}
	id := line{{" " + shown.ID, dimInk}}
	if room := cells - id.width(); room > 0 {
		return slices.Concat(name.fit(room), id)
	}
	return slices.Concat(name, id).fit(cells)
}

// sessionCells are the cells of the session column among those given: none
// where it isn't among them.
func sessionCells(columns []listColumn) int {
	if i := slices.IndexFunc(columns, func(c listColumn) bool { return c.part == listSession }); i >= 0 {
		return columns[i].width
	}
	return 0
}

// projectCellsOf are the cells the List gives its sessions' projects, of a
// session column cells wide, as the naturals given measure its projects and
// names: the widest project's, cut to leave the widest name its cells,
// listNameMost of them at the most, so one long name doesn't cut every
// project, and listProjectLeast at the least; none where no session has a
// project, each then named by its name alone.
func projectCellsOf(cells int, n listNaturals) int {
	if n.project == 0 {
		return 0
	}
	return min(n.project, max(listProjectLeast, cells-1-min(n.name, listNameMost)))
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

// stateSays is what it's doing at now, as its DayState says it, as last
// known where asKnown says: one that went on after a period gone in
// text.muted, as one ended is.
func (w sessionWords) stateSays(s views.PeriodSession) line {
	return line{{s.DayState(w.now), asKnown(s.Activity, listStateInks[s.Activity.Doing])}}
}

// startedSays is when its first request came, in text.muted: by the clock
// alone where it came on the day the session's grouped under, as one that
// went on after a period gone is under none; else, at now, as prose.Begun
// says it, its day as the rules name days, as "yesterday 15:08" or "Mon 21
// Sep 09:12", and of a period gone, as prose.PastAt says it, as "Tue
// 12:08", "21 Sep 12:08" or "21 Sep 2025 12:08"; nothing where that isn't
// known.
func (w sessionWords) startedSays(s views.PeriodSession) line {
	switch {
	case s.Started.IsZero():
		return nil
	case !s.WentOn && prose.SameDay(w.now, w.dayOf(s.ListedSession), s.Started):
		return line{{prose.TimeOfDay(w.now, s.Started), mutedInk}}
	case w.gone:
		return line{{prose.PastAt(w.now, s.Started), mutedInk}}
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
