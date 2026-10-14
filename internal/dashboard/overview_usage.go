package dashboard

import (
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The cells of USAGE by account from 150 columns, beside ROUTING: its bars
// are besideBar cells with a name column of matrixNames, a cell fewer for
// each cell the column is wider, and ROUTING's blocks start besideCells after
// its words.
const (
	besideBar   = 50
	besideCells = 4
)

// usageSection is the Overview's USAGE of doc at now: usage's block, as
// overviewBlock has it. At its least it's the block as usage lays it out at
// the frame's width, which from 150 columns, by window, is the compact
// matrix, 2a + 7 rows for a accounts and the pool; at its most, from 150
// columns, by window, the big bars, its windows stacked as sections at the
// full width; otherwise its least. USAGE never folds: given fewer rows than
// its least, it scrolls within them, an account at a time. By account from
// 150 columns, ROUTING is drawn beside its blocks, from routingAt, and
// follows them as they scroll; a click along an account's rows, its own or
// ROUTING's beside them, moves the focus to it. ROUTING's table beside them
// is built only as it's first drawn.
func (f Frame) usageSection(doc status.Document, now time.Time) section {
	block := f.overviewBlock(doc, now)
	views := block.views()
	aside := 0
	var routing func() routingTable
	if f.routingBesideUsage() {
		aside = block.routingAt()
		routing = sync.OnceValue(func() routingTable { return f.routingTableOf(doc, now) })
	}
	return section{
		Section: SectionUsage, least: sectionHead + views.all, most: sectionHead + views.big, aside: aside,
		description: func() line { return f.roomLeft(doc, now) },
		draw: func(c *canvas, body bounds) {
			shown := views.fitted(body.rows, f.Scroll)
			shown.drawIn(c, body)
			if aside > 0 {
				f.routingBeside(c, routing(), shown.beside(aside, body))
			}
		},
		targets: func(body bounds) []Target {
			return views.fitted(body.rows, f.Scroll).targets(body)
		},
		scrolling: views.scrolling,
	}
}

// overviewBlock is usage's block of doc at now as the Overview's USAGE draws
// it, grouped as the frame's By has it: as usage prints it, but for what the
// Overview alone has. By account from 150 columns, its bars leave room for
// ROUTING beside them. While GRAPHS shows, the matrix picks out the account
// the focus is on. Its key line names the marks the accounts carry. An
// account out of the pool at now is faint, as overviewGauge draws its windows
// and overviewNames its name.
func (f Frame) overviewBlock(doc status.Document, now time.Time) usageBlock {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	b := blockOf(m, f.Width, f.Look, f.By, f.overviewWindows(doc, now), names)
	b.marks = keyMarks(names, f.phone())
	if f.By == ByAccount && f.Width >= matrixWidth {
		b.accountBar = besideBar - (b.names - matrixNames)
	}
	if f.Sections.Has(SectionGraphs) {
		b.focus = f.Focus
	}
	for i, row := range b.rows {
		out := i < len(names) && names[i].out
		for j, g := range row.gauges {
			row.gauges[j] = f.overviewGauge(g, out)
		}
	}
	return b
}

// overviewWindows are the windows of doc the Overview shows, in USAGE and
// GRAPHS, as usage prints them: those every model shares, as the policy the
// moment was judged by has them, so every account keeps its rows before any
// is read, and
// every other any account reports, in Claude's order, each named as
// windowNamer names it of doc at now.
func (f Frame) overviewWindows(doc status.Document, now time.Time) []shownWindow {
	m := f.Moment(doc, now)
	return usageWindows(m, nil, f.windowNamer(m))
}

// windowNamer names m's windows by their keys as the Overview names them
// wherever it does: as the frame's WindowName has it, else as m names them,
// as windowNamed has them.
func (f Frame) windowNamer(m status.Moment) func(key string) string {
	if f.WindowName != nil {
		return f.WindowName
	}
	return func(key string) string { return windowNamed(m, key).Name }
}

// overviewNames are the names of m's accounts as the Overview names them
// wherever it does: as namesOf has them, but faint, number, label and marks,
// where an account is out of the pool.
func overviewNames(m status.Moment) []accountName {
	names := namesOf(m)
	for i := range names {
		names[i].faint = names[i].out
	}
	return names
}

// overviewNamed is the account of m with the given id as the Overview names
// it in a line, in k: its label, as names, overviewNames' of m, have it,
// faint where it's out of the pool; where m lacks it, its id, cleaned.
func overviewNamed(m status.Moment, names []accountName, id string, k ink) span {
	if i := place(m, id); i > 0 {
		return span{names[i-1].label, names[i-1].inked(k)}
	}
	return span{status.Clean(id), k}
}

// overviewGauge is g, an account's window or the pool's, as the Overview
// draws it: where its account is out of the pool, its bar faded as a stale
// reading's is, the two fades compounding where it's stale too, and its
// percentage and words faint; its bar filled as far as it has eased, while
// it eases after a read; and its words cued while their cue for turning to
// out ~ fades back, as the frame's TurnedOut says, where they say it still.
func (f Frame) overviewGauge(g gauge, out bool) gauge {
	if out {
		g.bar.out, g.words = true, g.words.faint()
	}
	if used, ok := f.Eased[g.of]; ok {
		g.bar.eased = &used
	}
	if fade, ok := f.TurnedOut[g.of]; ok && g.runsOut {
		g.cue = &fade
	}
	return g
}

// keyMarks are the marks the accounts named carry, as USAGE's key line names
// them after the cap's: ◆ on the primary, and ▲ on the account new sessions
// go to, which a phone words short.
func keyMarks(names []accountName, phone bool) []line {
	var marks []line
	if slices.ContainsFunc(names, func(n accountName) bool { return n.primary }) {
		marks = append(marks, line{{"◆", primaryInk}, {" primary", mutedInk}})
	}
	if slices.ContainsFunc(names, func(n accountName) bool { return n.best }) {
		goes := " new sessions go here"
		if phone {
			goes = " new"
		}
		marks = append(marks, line{{"▲", accentInk}, {goes, mutedInk}})
	}
	return marks
}

// roomLeft is USAGE's description of doc at now: with one account, which has
// no pool, the room it has left in its 5-hour window and its week at now,
// where it has them, as their Room has it, and numbered as its window's room,
// as in "room left  5h 0.4 of 1  ·  week 0.6 of 1"; none on a phone, nor with
// more accounts, whose pool's row says the room.
func (f Frame) roomLeft(doc status.Document, now time.Time) line {
	m := f.Moment(doc, now)
	if len(m.Accounts) != 1 || f.phone() {
		return nil
	}
	a := m.Accounts[0]
	session, hasSession := a.Window(m.Policy().Started)
	week, hasWeek := sharedWeek(a)
	var parts []line
	for _, w := range []struct {
		window status.WindowNow
		has    bool
		word   string
	}{{session, hasSession, "5h"}, {week, hasWeek, "week"}} {
		if w.has {
			left := span{tenths(w.window.Room), strongInk}.numbered(Number{Kind: NumberRoom, Account: a.ID, Of: w.window.Key})
			parts = append(parts, line{{w.word + " ", dimInk}, left, {" of 1", dimInk}})
		}
	}
	if parts == nil {
		return nil
	}
	return slices.Concat(line{{"room left  ", faintInk}}, joined(span{"  ·  ", faintInk}, parts...))
}

// routingAt is the column ROUTING's blocks start at beside the block's, by
// account from 150 columns: hx = n + 10 + bar + 7 + 22 + 4, its words'
// cells and besideCells after them, 107 with a name column of 16.
func (b usageBlock) routingAt() int {
	return b.names + windowLabelCells + b.accountBar + afterBar + fullCells + besideCells
}

// enlarged is the block with its big bars, where it has them, as enlarges
// says. Otherwise it's as it is.
func (b usageBlock) enlarged() usageBlock {
	if b.enlarges() {
		b.big = true
	}
	return b
}

// enlarges reports whether the block has big bars: by window from 150
// columns, its windows stacked as sections at the full width.
func (b usageBlock) enlarges() bool {
	return b.by == ByWindow && b.width >= matrixWidth
}

// usageViews are the block's views, as USAGE shows it in however many rows
// it's given, scrolled however far: the block; the rows it takes with its big
// bars, and as it's laid out; how many accounts it has; and, worked out only
// once it scrolls, the rows each account in view adds, and each mark, ▲ or
// ▼, as of counts them.
type usageViews struct {
	block    usageBlock
	big, all int
	accounts int
	steps    func() (account, mark int)
}

// views are the block's views, laid out once as it is, and once more with
// its big bars where it has them, as enlarges says.
func (b usageBlock) views() usageViews {
	accounts, _ := b.split()
	all := b.height()
	v := usageViews{block: b, big: all, all: all, accounts: len(accounts)}
	if b.enlarges() {
		v.big = b.enlarged().height()
	}
	v.steps = sync.OnceValues(func() (int, int) { return b.steps(all) })
	return v
}

// steps are the rows the block, all rows tall with every account in view,
// takes for each account in view, and for each mark, ▲ or ▼: worked out from
// two more layouts of it at most, as every account's rows are alike, and ▲
// and ▼ take a row each in each window's section: all but the last account
// in view, and all but the last two. With two accounts, a view of one has a
// mark, so the account's rows count the mark's.
func (b usageBlock) steps(all int) (account, mark int) {
	accounts, _ := b.split()
	n := len(accounts)
	if n < 2 {
		return 0, 0
	}
	oneBelow := b.shows(0, n-1).height()
	account = all - oneBelow
	if n > 2 {
		account = oneBelow - b.shows(0, n-2).height()
		mark = oneBelow + account - all
	}
	return account, mark
}

// of is how many rows the block takes, as height has them, with above
// accounts' rows out of view above, shown in view, and the rest below.
func (v usageViews) of(above, shown int) int {
	account, mark := v.steps()
	marks := 0
	if above > 0 {
		marks++
	}
	if v.accounts-above-shown > 0 {
		marks++
	}
	return v.all - (v.accounts-shown)*account + marks*mark
}

// fitted is the block as USAGE draws it in rows rows, scrolled offset
// accounts' rows down: with its big bars, where they fit; else as it's laid
// out, scrolled within the rows where it doesn't fit.
func (v usageViews) fitted(rows, offset int) usageBlock {
	if v.big <= rows {
		return v.block.enlarged()
	}
	return v.scrolled(rows, offset)
}

// scrolling is how far the block scrolls in rows rows, scrolled offset
// accounts' rows down, an account's rows at a time: no further than shows the
// last account, a page being as many as show at the offset, taken no further
// than it goes: more at the top or the foot, with no ▲ or ▼ by them, than
// between; nowhere where it fits.
func (v usageViews) scrolling(rows, offset int) Scrolling {
	n := v.accounts
	if n < 2 || v.all <= rows {
		return Scrolling{}
	}
	last := 1
	for last < n-1 && v.of(n-last-1, last+1) <= rows {
		last++
	}
	most := n - last
	return Scrolling{Most: most, Page: v.shownFrom(rows, min(max(offset, 0), most))}
}

// scrolled is the block in rows rows, scrolled offset accounts' rows down, no
// further than scrolling says it goes, showing as many as fit, at least one,
// the rows they leave spare under them, so its pool and its key line keep
// their rows at the foot however far it's scrolled.
func (v usageViews) scrolled(rows, offset int) usageBlock {
	s := v.scrolling(rows, offset)
	if s.Most == 0 {
		return v.block
	}
	above := min(max(offset, 0), s.Most)
	shown := v.block.shows(above, s.Page)
	shown.spare = max(rows-v.of(above, s.Page), 0)
	return shown
}

// shownFrom is how many accounts' rows show in rows rows, from the one at
// first: as many as fit, at least one.
func (v usageViews) shownFrom(rows, first int) int {
	shown := 1
	for shown < v.accounts-first && v.of(first, shown+1) <= rows {
		shown++
	}
	return shown
}

// shows is the block with above accounts' rows out of view above, shown in
// view, and the rest below.
func (b usageBlock) shows(above, shown int) usageBlock {
	accounts, _ := b.split()
	b.above, b.below = above, len(accounts)-above-shown
	return b
}

// drawIn draws the block in body, its key line after a blank row, cut to
// body's rows.
func (b usageBlock) drawIn(c *canvas, body bounds) {
	part := newCanvas(c.width, body.rows)
	b.key(part, b.draw(part, 0)+1)
	c.paste(part, 0, body.y, body.rows)
}

// placed is where the block draws each of its rows in view, the accounts'
// and the pool's, counted from its first row.
func (b usageBlock) placed() []rowAt {
	return b.lay(growing(b.width), 0).placed
}

// beside is where the block draws its rows in body, for ROUTING to be drawn
// beside them from column x.
func (b usageBlock) beside(x int, body bounds) blocksBeside {
	at := blocksBeside{x: x}
	for _, p := range b.placed() {
		p.y += body.y
		at.blocks = append(at.blocks, p)
	}
	return at
}

// targets are the block's click targets as it's drawn in body: each row of
// an account's, across the body, which moves the focus to it.
func (b usageBlock) targets(body bounds) []Target {
	var targets []Target
	for _, p := range b.placed() {
		if p.id == "" {
			continue
		}
		for y := p.y; y < min(p.y+p.rows, body.rows); y++ {
			targets = append(targets, Target{X: body.x, Y: body.y + y, Width: body.width, On: OnAccount, Account: p.id})
		}
	}
	return targets
}
