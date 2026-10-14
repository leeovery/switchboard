package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Lanes are the Switchboard where cords would need width the terminal hasn't
// got: each account's line, its windows on one row, with its sessions under
// it, a row each, then the pool and the key. Nothing travels them: what a
// session's doing says what moves.

// A lane's columns: a session's id at laneID, a cell, and its directory, cut
// to laneDir cells; what it's doing at laneDoing; and where it came from at
// laneFrom. Its windows' row: the first's label at laneWindowsAt, its bar
// laneFirstBar cells after it, a cell and its percentage; laneApart cells
// on, the second's label, its bar laneNextBar cells after it, a cell and its
// percentage. laneBeside are the cells the row takes beside its bars, so
// each bar is half of what the width leaves them.
const (
	laneID        = 2
	laneDir       = 11
	laneDoing     = 20
	laneFrom      = 33
	laneWindowsAt = 1
	laneFirstBar  = 7
	laneNextBar   = 5
	laneApart     = 3
	laneBeside    = laneWindowsAt + laneFirstBar + laneNextBar + 2*(1+percentCells) + laneApart
)

// laneBarsAfter are how many cells after its label each window's bar starts
// on a lane's windows' row: the 5-hour's, then the week's.
var laneBarsAfter = []int{laneFirstBar, laneNextBar}

// outOfPoolSpan marks an account the global pin leaves out of the pool, in
// text.faint, cross and words.
var outOfPoolSpan = span{"✕ out of the pool", faintInk}

// lanesBoard is the Switchboard of a moment drawn in lanes: the frame, the
// moment and the time it's drawn at; the accounts' names, unfaded, as ▸
// names the account a session came from; the usage block its lines are
// drawn from, of the windows a lane shows, its bars a lane's; its lanes, an
// account's each, in the config's order; the pool's row, nil where there's
// no pool, as with one account; and the rows it takes drawn whole, the foot
// under its lanes.
type lanesBoard struct {
	f     Frame
	m     status.Moment
	now   time.Time
	names []accountName
	block usageBlock
	lanes []boardLane
	pool  *usageRow
	whole int
}

// boardLane is an account's lane: the account, how it stands, its row of the
// block, and its calls, in the order the patch keeps them.
type boardLane struct {
	account status.AccountNow
	row     usageRow
	calls   []views.Call
}

// rows are the rows the lane takes: its name row, its windows' row and a row
// a session.
func (l boardLane) rows() int {
	return 2 + len(l.calls)
}

// lanesForm is the Switchboard of doc at now in lanes, in place of cords,
// its accounts' lines and calls as laid gives them: drawn whole, where they
// fit the view with the foot under them; else scrolled an account at a time,
// as lanesBoard.scrolling has it.
func (f Frame) lanesForm(doc status.Document, now time.Time, laid boardLayout) sessionsForm {
	b := f.lanesOf(doc, now, laid)
	return sessionsForm{
		draw:      b.draw,
		scrolling: b.scrolling,
		targets:   b.targets,
		filled:    func() map[Ref]float64 { return b.shown().filled },
	}
}

// lanesOf is the Switchboard of doc at now in lanes: a lane an account of the
// moment, its line its row of usage's block of the windows a lane shows,
// faded as the Overview fades an account out of the pool, and its calls the
// patch's on its line, as laid has them, each with what the look found of it.
func (f Frame) lanesOf(doc status.Document, now time.Time, laid boardLayout) lanesBoard {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	block := blockOf(m, f.Width, f.Look, ByAccount, f.laneWindows(m), names)
	block.accountBar, block.marks = max((f.Width-laneBeside)/2, 0), keyMarks(names, f.phone())
	found := make(map[Plug]views.Call, len(f.SessionsTab.Calls))
	for _, c := range f.SessionsTab.Calls {
		found[Plug{Session: c.Session, Account: c.Account}] = c
	}
	accounts, pool := block.split()
	b := lanesBoard{f: f, m: m, now: now, names: namesOf(m), block: block, pool: pool}
	for i, a := range m.Accounts {
		l := boardLane{account: a, row: accounts[i]}
		for j, g := range l.row.gauges {
			l.row.gauges[j] = f.overviewGauge(g, a.OutOfPool)
		}
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
	whole := growing(f.Width)
	b.foot(whole, b.drawLanes(&stack{c: whole, filled: make(map[Ref]float64)}, 0, b.lanes))
	b.whole = whole.height()
	return b
}

// laneWindows are the windows a lane's row shows of m, as step 3 shows them:
// the first that every model shares, as the policy m was judged by has
// them, in Claude's order, a bar for each laneBarsAfter has, each named as
// the frame names it.
func (f Frame) laneWindows(m status.Moment) []shownWindow {
	keys := slices.SortedFunc(slices.Values(m.Policy().Shared), quota.CompareKeys)
	shown := usageWindows(m, keys, f.windowNamer(m))
	return shown[:min(len(shown), len(laneBarsAfter))]
}

// draw draws the lanes the view shows from row top, as shown has them, cut
// to the view's rows; and where accounts are out of view below, ▼ and how
// many, right-aligned on the row over the key line.
func (b lanesBoard) draw(c *canvas, top int) {
	shown := b.shown()
	c.paste(shown.c, 0, top, b.f.viewRows())
	if below := shown.below; below > 0 {
		c.right(b.f.Width, b.f.Height-underView, outOfView(0, below, b.f.Width, usageOut))
	}
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
// rows, the zones a click acts on and the bars, as a stack has them; and how
// many accounts are out of view below.
type lanesShown struct {
	stack
	below int
}

// shown are the lanes the view shows: every lane, and the foot under them,
// where they fit; else as many as fit from the account the frame has the
// view scrolled to, as scrolling has it, the foot giving way, and ▲ and how
// many accounts are above, on the view's first row, while any are.
func (b lanesBoard) shown() lanesShown {
	s := lanesShown{c: growing(b.f.Width), filled: make(map[Ref]float64)}
	if b.fits() {
		b.foot(s.c, b.drawLanes(&s.stack, 0, b.lanes))
		return s
	}
	scrolling := b.scrolling(b.f.Scroll)
	above := scrolling.clamped(b.f.Scroll)
	y := 0
	if above > 0 {
		s.c.line(0, y, outOfView(above, 0, b.f.Width, usageOut))
		y++
	}
	b.drawLanes(&s.stack, y, b.lanes[above:above+scrolling.Page])
	s.below = len(b.lanes) - above - scrolling.Page
	return s
}

// fits reports whether every lane fits the view, the foot under them, as
// they do where there are none, which leaves nothing to scroll.
func (b lanesBoard) fits() bool {
	return len(b.lanes) == 0 || b.whole <= b.f.viewRows()
}

// scrolling is how far the lanes scroll, scrolled offset accounts down, an
// account at a time, its line and its sessions together: nowhere where they
// fit the view whole, the foot with them; else, the foot giving way, no
// further than shows the last account, a page being as many as show from the
// offset, under ▲ where any are above.
func (b lanesBoard) scrolling(offset int) Scrolling {
	if b.fits() {
		return Scrolling{}
	}
	most := 0
	for most < len(b.lanes)-1 && b.fitting(most) < len(b.lanes)-most {
		most++
	}
	return Scrolling{Most: most, Page: b.fitting(min(max(offset, 0), most))}
}

// fitting is how many lanes, from the one at first, fit the view, as many as
// do, at least one, under ▲ where any are above, a blank row between each.
func (b lanesBoard) fitting(first int) int {
	rows := b.f.viewRows()
	if first > 0 {
		rows--
	}
	shown, taken := 1, b.lanes[first].rows()
	for _, l := range b.lanes[first+1:] {
		if taken += 1 + l.rows(); taken > rows {
			break
		}
		shown++
	}
	return shown
}

// drawLanes draws the lanes given onto the stack from row y, a blank row
// between each, and returns the row after the last's last row.
func (b lanesBoard) drawLanes(s *stack, y int, lanes []boardLane) int {
	for i, l := range lanes {
		if i > 0 {
			y++
		}
		y = b.drawLane(s, y, l)
	}
	return y
}

// drawLane draws the lane onto the stack from row y: its name row, its
// windows' row and its calls, a row each, each a zone a click chooses it by;
// and returns the row after its last.
func (b lanesBoard) drawLane(s *stack, y int, l boardLane) int {
	b.nameRow(s.c, y, l)
	b.windowsRow(s, y+1, l.row)
	y += 2
	for _, call := range l.calls {
		b.callRow(s.c, y, call)
		s.targets = append(s.targets, Target{Y: y, Width: b.f.Width, On: OnSession, Session: call.Session, Account: call.Account})
		y++
	}
	return y
}

// nameRow draws the lane's name row along row y: its number, name and marks,
// cut to the name column; from it, how the account stands, in the Overview's
// short words, cut by whole clauses to end wordsGap short of what ends the
// row at the last column, where something does: ● pinned on an account the
// global pin names, ✕ out of the pool on one it leaves out.
func (b lanesBoard) nameRow(c *canvas, y int, l boardLane) {
	c.line(0, y, l.row.name)
	end := b.f.Width
	switch {
	case l.account.Pinned:
		end = c.right(end, y, line{pinnedSpan})
	case l.account.OutOfPool:
		end = c.right(end, y, line{outOfPoolSpan})
	}
	c.line(b.block.names, y, standingWords(l.account.Routing, true, end-wordsGap-b.block.names))
}

// windowsRow draws the windows of the account's row along row y onto the
// stack, as step 3 draws them: each its label, in text.subtle, its bar, a
// cell and its percentage, the next laneApart cells on; each bar marked as
// its window's, and counted among the bars the stack fills.
func (b lanesBoard) windowsRow(s *stack, y int, row usageRow) {
	bar, x := b.block.accountBar, laneWindowsAt
	for j, g := range row.gauges {
		after := laneBarsAfter[j]
		s.c.line(x, y, line{{prose.Cut(status.Short(b.block.windows[j].name), after-1), dimInk}})
		g.bar.draw(s.c, b.f.Look, x+after, y, bar)
		s.c.barred(x+after, y, bar, g.of)
		x += after + bar + 1 + percentCells
		s.c.right(x, y, g.percentage())
		x += laneApart
		if !g.bar.empty {
			s.filled[g.of] = g.bar.used
		}
	}
}

// callRow draws the call along row y: its session's id, bold text.primary
// while it works, else text.muted, and its directory in text.muted; what
// it's doing, in a session's page's words; ▸ from and the account it came
// from, where a move brought it; all on bg.selection across the row where
// it's the row chosen.
func (b lanesBoard) callRow(c *canvas, y int, call views.Call) {
	id := mutedInk
	if call.Activity.Works() {
		id = titleInk
	}
	x := c.line(laneID, y, line{{status.ShownID(call.Session), id}})
	if dir := status.DirName(call.Dir, laneDir); dir != "" {
		c.line(x, y, line{{" " + dir, mutedInk}})
	}
	c.line(laneDoing, y, line{{call.Activity.Said(b.now), callSaidInk(call.Activity.Doing)}})
	if call.From != "" {
		from := overviewNamed(b.m, b.names, call.From, secondaryInk)
		from.text = prose.Cut(from.text, fromAccount)
		c.line(laneFrom, y, line{{"▸", ink{token: theme.AccentKey}}, {" from ", mutedInk}, from})
	}
	if b.f.SessionsTab.Chosen == (Plug{Session: call.Session, Account: call.Account}) {
		c.surface(0, y, b.f.Width, hue{token: theme.BgSelection})
	}
}

// callSaidInk is the ink what a session's doing is said in, by what it's doing:
// asking or answering bold accent.key; refused bold state.destructive;
// throttled text.subtle; else text.faint.
func callSaidInk(d views.Doing) ink {
	switch d {
	case views.Asking, views.Answering:
		return keyInk
	case views.Refused:
		return exhaustedInk
	case views.Throttled:
		return dimInk
	}
	return faintInk
}

// foot draws the foot under the lanes onto c from row y, the row after the
// last lane's: where there's a pool, its rule across the row, Σ pool under
// it, and under that, from laneWindowsAt, the room it has left in each window;
// then, after a blank row, the key.
func (b lanesBoard) foot(c *canvas, y int) {
	if b.pool != nil {
		c.line(0, y, line{{rule(b.f.Width), borderInk}})
		c.line(0, y+1, b.pool.name)
		c.line(laneWindowsAt, y+2, b.room().fit(b.f.Width-laneWindowsAt))
		y += 3
	}
	b.block.key(c, y+1)
}

// room is the room the pool has left in each window it has, as in "5-hour
// 1.5 of 3 left   Week 1.2 of 3 left": each window's label in text.subtle,
// and its words, laneApart cells apart.
func (b lanesBoard) room() line {
	var parts []line
	for j, g := range b.pool.gauges {
		if said := g.said(b.f.Width, b.f.Width); len(said) > 0 {
			label := span{status.Short(b.block.windows[j].name) + " ", dimInk}
			parts = append(parts, slices.Concat(line{label}, said))
		}
	}
	return joined(spaces(laneApart), parts...)
}
