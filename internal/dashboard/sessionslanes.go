package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Lanes are the Switchboard where cords would need width the terminal hasn't
// got: each account's line, as the board's lines draw it, with its sessions
// under it, a row each, then the key over the key line. Nothing travels
// them: what a session's doing says what moves.

// A lane's call starts laneIndent cells in, what it's doing laneDoingGap
// cells after its directory.
const (
	laneIndent   = 2
	laneDoingGap = 2
)

// laneColumnsOf are where a lane puts a call's parts, width cells wide, the
// from column's natural from cells: its parts laid as callParts declares
// them, laneIndent cells in, alike in every lane, so its columns align. What
// it has spare lies at the row's end.
func laneColumnsOf(width, from int) callColumns {
	s, _ := flex.Lay(width-laneIndent, callParts(laneDoingGap, from)...)
	return callColumnsOf(s, laneIndent)
}

// A lane's windows side by side, on the row under its name: the first's
// label at laneWindowsAt, its bar laneFirstBar cells after it; the second's
// bar laneNextBar cells after its label, room for "5-hour", as windowsBeside
// draws them.
const (
	laneWindowsAt = 1
	laneFirstBar  = 7
	laneNextBar   = 7
)

// laneBarsAfter are how many cells after its label each window's bar starts
// on a lane's windows' row: the week's, then the 5-hour's.
var laneBarsAfter = []int{laneFirstBar, laneNextBar}

// laneLineBeside are a lane's line's cells besides its bars and words, as
// lineBeside counts the board's, but the cell the board leaves at the edge:
// a lane's line runs to the frame's last column.
const laneLineBeside = lineBeside - afterWords

// stackedStep is the board's compaction step whose lines a lane's stacked
// windows are drawn as: a row a window, a blank row between.
const stackedStep = 1

// laneStep is a form the lanes take down the view: their windows stacked,
// as the board's lines at stackedStep, or side by side on a row; and the
// blank rows between accounts, and before an account's calls.
type laneStep struct {
	stacked            bool
	apart, beforeCalls int
}

// laneSteps are the lanes' forms, in the order they're taken as rows run
// short: each account's windows stacked, accounts 2 blank rows apart and a
// blank row before an account's calls; then its windows side by side; then
// the gaps back to their least, accounts a row apart and none before the
// calls. Where even the last doesn't fit the view with the key under it, it
// scrolls, as lanesBoard.scrolling has it.
var laneSteps = []laneStep{
	{stacked: true, apart: 2, beforeCalls: 1},
	{apart: 2, beforeCalls: 1},
	{apart: 1},
}

// lineRows are the rows an account's line takes at the step, of windows
// windows: stacked, as the board's at stackedStep; side by side, its name
// row and its windows' row.
func (s laneStep) lineRows(windows int) int {
	if s.stacked {
		return lineRows(stackedStep, windows)
	}
	return 2
}

// laneKeyGap is the fewest blank rows between the last lane and the key.
const laneKeyGap = 1

// lanesBoard is the Switchboard of a moment drawn in lanes: the frame, and
// the time it's drawn at; its accounts' lines, as the board's lines draw them
// across the frame from column 0, as laneLines lays them; where its calls'
// parts sit, as laneColumnsOf has them; its lanes, an account's each, in the
// config's order; the step it takes, as laneSteps has them; and the row of
// the view its key starts on, -1 where it gives way, as where the lanes
// scroll.
type lanesBoard struct {
	f     Frame
	now   time.Time
	lines linesDrawn
	calls callColumns
	lanes []boardLane
	step  laneStep
	keyAt int
}

// boardLane is an account's lane: the account, as the lines name it, and its
// calls, in the order the patch keeps them; and whether it says no session
// is running, as the first lane does where none is.
type boardLane struct {
	account  status.AccountNow
	name     accountName
	calls    []views.Call
	saysNone bool
}

// rows are the rows the lane takes at the step given, of windows windows:
// its line, as the step's lineRows has them; then, where it has calls or
// says none runs, the step's blank rows before them, and a row a session, or
// one saying none runs.
func (l boardLane) rows(step laneStep, windows int) int {
	calls := len(l.calls)
	if l.saysNone {
		calls++
	}
	if calls > 0 {
		calls += step.beforeCalls
	}
	return step.lineRows(windows) + calls
}

// laneLines are the lanes' lines of m, as the board's lines draw them at
// stackedStep, from column 0: their windows' rows' parts laid across the
// frame as lineParts declares them, running to its last column, so a bar
// stands at its natural, lineBar, with its words in full, from 73 columns.
func (f Frame) laneLines(m status.Moment) linesDrawn {
	s, _ := flex.Lay(f.Width, lineParts(laneLineBeside)...)
	return f.linesDrawn(m, boardLayout{step: stackedStep, columns: boardColumns{
		bar:        s[lineBarsPastPart].Size + s[lineBarsPart].Size,
		words:      s[lineSaidPart].Size,
		shortWords: s[lineSaidPart].Left,
	}})
}

// lanesForm is the Switchboard of doc at now in lanes, in place of cords,
// its accounts' lines and calls as laid gives them: drawn whole, where they
// fit the view with the key under them; else scrolled an account at a time,
// or a row at a time where a lane doesn't fit, as lanesBoard.scrolling has
// it.
func (f Frame) lanesForm(doc status.Document, now time.Time, laid boardLayout) sessionsForm {
	b := f.lanesOf(doc, now, laid)
	return sessionsForm{
		draw:      b.draw,
		scrolling: b.scrolling,
		targets:   b.targets,
		filled:    func() map[Ref]float64 { return b.shown().filled },
		byRows:    b.byRows(),
	}
}

// lanesOf is the Switchboard of doc at now in lanes: a lane an account of the
// moment, its line as laneLines draws it, and its calls the patch's on its
// line, as laid has them, each with what the look found of it; at the first
// of laneSteps whose lanes fit the view with the key under them, as keyed
// has them, else the last.
func (f Frame) lanesOf(doc status.Document, now time.Time, laid boardLayout) lanesBoard {
	m := f.Moment(doc, now)
	found := make(map[Plug]views.Call, len(f.SessionsTab.Calls))
	for _, c := range f.SessionsTab.Calls {
		found[Plug{Session: c.Session, Account: c.Account}] = c
	}
	b := lanesBoard{f: f, now: now, lines: f.laneLines(m), calls: laneColumnsOf(f.Width, f.fromCells(m)), keyAt: -1}
	for i, a := range m.Accounts {
		l := boardLane{account: a, name: b.lines.names[i]}
		ln, _ := laid.line(a.ID)
		for _, p := range ln.patched.Calls {
			plug := Plug{Session: p.Session, Account: a.ID}
			c, ok := found[plug]
			if !ok {
				c.Session, c.Account = plug.Session, plug.Account
			}
			l.calls = append(l.calls, c)
		}
		b.lanes = append(b.lanes, l)
	}
	if len(b.lanes) > 0 && !slices.ContainsFunc(b.lanes, func(l boardLane) bool { return len(l.calls) > 0 }) {
		b.lanes[0].saysNone = true
	}
	for _, b.step = range laneSteps {
		if b.keyed() {
			break
		}
	}
	return b
}

// keyed has b's key where its lanes, at its step, and the key under them fit
// the view, as keyPlaced has them, laneKeyGap rows apart at the least,
// reporting whether they do, as they do where there are no lanes, which
// leave nothing to key.
func (b *lanesBoard) keyed() bool {
	if len(b.lanes) == 0 {
		return true
	}
	at, ok := keyPlaced(b.f.viewRows(), b.height(), laneKeyGap)
	if ok {
		b.keyAt = at
	}
	return ok
}

// height is the rows the lanes take at b's step, its blank rows between
// each.
func (b lanesBoard) height() int {
	rows := 0
	for i, l := range b.lanes {
		if i > 0 {
			rows += b.step.apart
		}
		rows += b.rows(l)
	}
	return rows
}

// rows are the rows the lane takes at b's step.
func (b lanesBoard) rows(l boardLane) int {
	return l.rows(b.step, len(b.lines.windows))
}

// draw draws the lanes the view shows from row top, as shown has them, cut
// to the view's rows; the key, where it fits, from the row the lanes give
// it, its last over the key line; and right-aligned on that row, what shown
// says is out of view under the lanes.
func (b lanesBoard) draw(c *canvas, top int) {
	shown := b.shown()
	c.paste(shown.c, 0, top, b.f.viewRows())
	if b.keyAt >= 0 {
		b.lines.drawKey(c, top+b.keyAt)
	}
	c.right(b.f.Width, b.f.Height-underView, shown.out)
}

// targets are the zones of the lanes the view shows a click acts on, at the
// rows it shows them on: each session's row, which chooses its call.
func (b lanesBoard) targets() []Target {
	top, rows := b.f.top(), b.f.viewRows()
	var shown []Target
	for _, t := range b.shown().targets {
		if t.Y < rows {
			t.Y += top
			shown = append(shown, t)
		}
	}
	return shown
}

// lanesShown are the lanes the view shows, drawn from its first row: the
// rows and the zones a click acts on, as a stack has them; how much of each
// bar they draw it fills, as Filled has them; and what's out of view under
// them, said as the row over the key line says it.
type lanesShown struct {
	stack
	filled map[Ref]float64
	out    line
}

// shown are the lanes the view shows: every lane, where they fit with the
// key under them; a row at a time, where byRows says so, as paged has them;
// else as many as fit from the account the frame has the view scrolled to,
// as scrolling has it, the key giving way, and ▲ and how many accounts are
// above, on the view's first row, while any are, and ▼ and how many below.
func (b lanesBoard) shown() lanesShown {
	switch {
	case b.fits():
		return b.laned()
	case b.byRows():
		return b.pagedShown()
	}
	s := lanesShown{c: growing(b.f.Width), filled: make(map[Ref]float64)}
	scrolling := b.scrolling(b.f.Scroll)
	above := scrolling.clamped(b.f.Scroll)
	y := 0
	if above > 0 {
		s.c.line(0, y, outOfView(above, 0, b.f.Width, boardOut))
		y++
	}
	b.drawLanes(&s, y, b.lanes[above:above+scrolling.Page])
	s.out = outOfView(0, len(b.lanes)-above-scrolling.Page, b.f.Width, boardOut)
	return s
}

// byRows reports whether the lanes scroll a row at a time: where they don't
// fit the view, and a lane doesn't either, under ▲ where any are above it,
// so an account at a time would cut it short.
func (b lanesBoard) byRows() bool {
	if b.fits() {
		return false
	}
	for i, l := range b.lanes {
		if b.rows(l)+min(i, 1) > b.f.viewRows() {
			return true
		}
	}
	return false
}

// laned are the lanes drawn whole from row 0.
func (b lanesBoard) laned() lanesShown {
	s := lanesShown{c: growing(b.f.Width), filled: make(map[Ref]float64)}
	b.drawLanes(&s, 0, b.lanes)
	return s
}

// paged is how the lanes, drawn whole, page in the view's rows, a row at a
// time, each lane's name row with its windows' row.
func (b lanesBoard) paged() rowsPaged {
	return pagedWhole(b.laned().c, b.f.viewRows())
}

// pagedShown are the lanes the view shows a row at a time, from as far down
// as the frame has them scrolled, as paged has them, and how many of their
// rows are out of view, above and below, as USAGE says it.
func (b lanesBoard) pagedShown() lanesShown {
	whole := b.laned()
	paged := pagedWhole(whole.c, b.f.viewRows())
	from, to := paged.shown(b.f.Scroll)
	s := lanesShown{c: growing(b.f.Width), filled: whole.filled, out: outOfView(from, paged.content-to, b.f.Width, usageRowsOut)}
	s.c.paste(whole.c, from, 0, to-from)
	for _, t := range whole.targets {
		if t.Y >= from && t.Y < to {
			t.Y -= from
			s.targets = append(s.targets, t)
		}
	}
	return s
}

// fits reports whether every lane fits the view, the key under them, as
// keyed has it, as they do where there are none, which leaves nothing to
// scroll.
func (b lanesBoard) fits() bool {
	return len(b.lanes) == 0 || b.keyAt >= 0
}

// scrolling is how far the lanes scroll, scrolled offset accounts down, an
// account at a time, its line and its sessions together: nowhere where they
// fit the view whole, the key with them; a row at a time, where byRows says
// so, as paged has it; else, the key giving way, no further than shows the
// last account, a page down starting at the first lane the page didn't
// show, and a page up ending at the one above the first it showed, as
// pageDown and pageUp have them, a page being as many as show from where
// it starts, under ▲ where any are above.
func (b lanesBoard) scrolling(offset int) Scrolling {
	if b.fits() {
		return Scrolling{}
	}
	if b.byRows() {
		return b.paged().scrolling(offset)
	}
	most := b.mostScrolled()
	from := min(max(offset, 0), most)
	shows := func(at, lane int) bool {
		at = min(at, most)
		return at <= lane && lane < at+b.fitting(at)
	}
	return pagedFrom(most, from, pageDown(from, from+b.fitting(from), shows), pageUp(0, from, from-1, shows))
}

// mostScrolled is the furthest the lanes scroll an account at a time: the
// first lane from which every lane after it fits the view.
func (b lanesBoard) mostScrolled() int {
	most := 0
	for most < len(b.lanes)-1 && b.fitting(most) < len(b.lanes)-most {
		most++
	}
	return most
}

// fitting is how many lanes, from the one at first, fit the view, as many as
// do, at least one, under ▲ where any are above, at the step's rows apart.
func (b lanesBoard) fitting(first int) int {
	rows := b.f.viewRows()
	if first > 0 {
		rows--
	}
	shown, taken := 1, b.rows(b.lanes[first])
	for _, l := range b.lanes[first+1:] {
		if taken += b.step.apart + b.rows(l); taken > rows {
			break
		}
		shown++
	}
	return shown
}

// drawLanes draws the lanes given onto s from row y, at the step's rows
// apart, and returns the row after the last's last row.
func (b lanesBoard) drawLanes(s *lanesShown, y int, lanes []boardLane) int {
	for i, l := range lanes {
		if i > 0 {
			y += b.step.apart
		}
		y = b.drawLane(s, y, l)
	}
	return y
}

// drawLane draws the lane onto s from row y: its line, as drawLine draws
// it; then, where it has calls, or says none runs, the step's blank rows
// before them, and its calls, a row each, each a zone a click chooses it by,
// or where it says none runs, a row saying so in text.faint, where its first
// call would be; and returns the row after its last.
func (b lanesBoard) drawLane(s *lanesShown, y int, l boardLane) int {
	y = b.drawLine(s, y, l)
	if l.saysNone || len(l.calls) > 0 {
		y += b.step.beforeCalls
	}
	if l.saysNone {
		s.c.line(b.calls.id, y, line{{noSessionsRunning, faintInk}})
		y++
	}
	for _, call := range l.calls {
		b.callRow(s.c, y, call)
		s.targets = append(s.targets, Target{Y: y, Width: b.f.Width, On: OnSession, Session: call.Session, Account: call.Account})
		y++
	}
	return y
}

// drawLine draws the lane's account's line onto s from row y, and returns
// the row after it: its name row, as the board's lines draw it, its state in
// their short words, heading the rows under it; then, at a step that stacks
// them, its windows as the board's lines stack them at stackedStep; else its
// week and its 5-hour side by side, as windowsBeside draws them from
// laneWindowsAt, each bar half what the width leaves them. Each bar it draws
// is counted among the bars they fill.
func (b lanesBoard) drawLine(s *lanesShown, y int, l boardLane) int {
	s.c.heading(y, y+1)
	shown := b.lines.beside
	if b.step.stacked {
		shown = b.lines.windows
	}
	gauges := make([]gauge, len(shown))
	for i, w := range shown {
		gauges[i] = b.lines.gauge(l.account, w.key)
	}
	b.lines.nameRow(s.c, l.account, l.name, y, true)
	if b.step.stacked {
		b.lines.stacked(s.c, l.account, y+1)
	} else {
		windowsBeside(s.c, b.f.Look, laneWindowsAt, y+1, sideBySideBar(b.f.Width, laneBarsAfter), laneBarsAfter, shown, gauges)
	}
	for _, g := range gauges {
		if !g.bar.empty {
			s.filled[g.of] = g.bar.used
		}
	}
	return y + b.step.lineRows(len(b.lines.windows))
}

// callRow draws the call along row y, as drawCall draws it at a lane's
// columns, all on bg.selection across the row where it's the row chosen.
func (b lanesBoard) callRow(c *canvas, y int, call views.Call) {
	drawCall(c, y, call, b.calls, b.lines.m, b.now)
	if b.f.SessionsTab.Chosen == (Plug{Session: call.Session, Account: call.Account}) {
		c.surface(0, y, b.f.Width, hue{token: theme.BgSelection})
	}
}
