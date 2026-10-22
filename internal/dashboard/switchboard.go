package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// boardDrawn is the Switchboard of a moment as its form draws it with cords,
// laid out as laid lays it: its lines, as lines draws them; and the calls the
// watch built at the look, by their rows.
type boardDrawn struct {
	f     Frame
	lines linesDrawn
	laid  boardLayout
	calls map[Plug]views.Call
}

// switchboardForm is the Switchboard of doc at now, laid out as given, as its
// form draws it with cords, as boardDrawn draws it.
func (f Frame) switchboardForm(doc status.Document, now time.Time, laid boardLayout) sessionsForm {
	d := f.boardDrawn(f.Moment(doc, now), laid)
	return sessionsForm{draw: d.draw, scrolling: d.scrolling, targets: d.targets, filled: d.filled, byRows: d.byRows()}
}

// boardDrawn is the Switchboard of m, laid out as laid lays it.
func (f Frame) boardDrawn(m status.Moment, laid boardLayout) boardDrawn {
	calls := make(map[Plug]views.Call, len(f.SessionsTab.Calls))
	for _, c := range f.SessionsTab.Calls {
		calls[Plug{Session: c.Session, Account: c.Account}] = c
	}
	return boardDrawn{f: f, lines: f.linesDrawn(m, laid), laid: laid, calls: calls}
}

// draw draws the Switchboard from row top, as much of it as the view shows,
// scrolled as the frame has it, as shownAt has it, and what's out of view,
// each right-aligned: over the rows shown, what shownAt says is above them,
// and on the row over the key line, what it says is below.
func (d boardDrawn) draw(c *canvas, top int) {
	s := d.shownAt(d.f.Scroll)
	c.right(d.f.Width, top, s.over)
	c.paste(d.board(), s.first, top+s.lead, s.rows)
	c.right(d.f.Width, d.f.Height-underView, s.under)
}

// board is the whole Switchboard drawn, its rows counted from its first: its
// lines and their jacks, the stubs a limit hangs and the cords moves let go,
// each call and its cord, or, with none, a row saying so in text.faint, the
// key, and the row chosen picked out.
func (d boardDrawn) board() *canvas {
	c := growing(d.f.Width)
	d.lines.draw(c)
	for _, ln := range d.laid.lines {
		d.jacks(c, ln)
	}
	if len(d.laid.calls) == 0 {
		c.line(0, 0, line{{noSessionsRunning, faintInk}})
	}
	for _, call := range d.laid.calls {
		d.call(c, call)
	}
	d.key(c)
	d.chosen(c)
	return c
}

// jacks draws line ln's jacks, a row each, free ones ○ in text.faint, and
// each stub a limit hangs from its jack, as stub has it, and each cord a
// move no limit forced let go, as loose has it.
func (d boardDrawn) jacks(c *canvas, ln laidLine) {
	x := d.laid.columns.jacks
	for row := ln.top; row < ln.top+ln.rows; row++ {
		c.text(x, row, freeJack, faintInk)
	}
	for _, s := range ln.stubs {
		c.text(x-s.cells, s.row, strings.Repeat(hangingGlyph, s.cells), errorInk)
	}
	for _, l := range d.f.Traffic.Loose {
		d.loose(c, ln, l)
	}
}

// loose draws the cord l, let go by a move no limit forced, where it hangs
// from line ln, the jack its session left there: looseCells of ╌ to the
// jack's left, in text.subtle, faded as far as it has; nothing once it's
// gone, as gone says, nor where l hangs from another line, or its jack has
// been taken since.
func (d boardDrawn) loose(c *canvas, ln laidLine, l Loose) {
	i := slices.IndexFunc(ln.patched.Left, func(p views.Plugged) bool { return p.Session == l.Session })
	if l.From != ln.account || i < 0 || d.gone(l.Fade) {
		return
	}
	jacks, row := d.laid.columns.jacks, ln.top+ln.patched.Left[i].Jack
	k := dimInk
	k.fade = l.Fade
	c.text(jacks-looseCells, row, strings.Repeat(hangingGlyph, looseCells), k)
	c.hung(rowCells(jacks-looseCells, jacks, row), l)
}

// gone reports whether what's faded so far, from 0 to 1, shows no more: once
// it has faded all the way, or, in a look that can't fade it, halfway.
func (d boardDrawn) gone(fade float64) bool {
	return fade >= 1 || fade >= 0.5 && !d.f.Look.blends()
}

// call draws the call lc along its row, as drawCall draws it at the
// columns the layout gives it, and its cord, as cord draws it.
func (d boardDrawn) call(c *canvas, lc laidCall) {
	call := d.calls[Plug{Session: lc.session, Account: lc.account}]
	drawCall(c, lc.cord.from, call, d.laid.columns.callColumns, d.lines.m, d.lines.m.Now)
	d.cord(c, lc, call.Activity.Works())
}

// cord draws lc's cord from its plug to its jack, ◉, in accent.key while its
// session works, else dimmed toward the canvas, drawn light where the look
// can't dim it; marked as its call's, for what travels it to move frames on;
// and what travels it, as the frame's Traffic tells.
func (d boardDrawn) cord(c *canvas, lc laidCall, works bool) {
	path := lc.cord.path(d.laid.columns.jacks)
	glyphs, k := d.f.Look.cordDrawn(works), cordInk(works)
	last := len(path) - 1
	for i, p := range path {
		c.text(p.x, p.y, lc.cord.glyph(p, i, last, glyphs), k)
	}
	plug := Plug{Session: lc.session, Account: lc.account}
	c.corded(path, plug)
	travel(c, path, theme.AccentKey, d.f.Traffic.On(plug.Session, plug.Account))
}

// key draws the key, the Overview's, on its two rows, where the layout gives
// them, a cell after the lines' column: even pace's on the first; the cap's,
// on the bars the layout draws, and the marks the accounts carry on the
// second, in the first of their forms that fits the width, as firstFit has
// it. A row that would pass the last column starts at column 0. With
// no lines, it has nothing to key, and draws none.
func (d boardDrawn) key(c *canvas) {
	if d.laid.keyAt < 0 || len(d.laid.lines) == 0 {
		return
	}
	m := d.lines.m
	b := blockOf(m, d.f.Width, d.f.Look, ByAccount, d.lines.windows, d.lines.names)
	b.marks = keyMarks(d.lines.names)
	pace, forms := b.keyParts(d.lines.barDrawn())
	for i, l := range []line{pace, firstFit(d.f.Width, keyForms(keyGap(d.f.Width), forms)...)} {
		x := d.laid.columns.lines + 1
		if x+l.width() > d.f.Width {
			x = 0
		}
		c.line(x, d.laid.keyAt+i, l.fit(d.f.Width))
	}
}

// chosen picks out the call chosen, where one is, on bg.selection over its
// own columns alone, from the row's first to the one before its plug, where
// its cord begins, their glyphs and colours as they are: its cord, and the
// lines and bars its row runs on to, are left as drawn.
func (d boardDrawn) chosen(c *canvas) {
	for _, lc := range d.laid.calls {
		if (Plug{Session: lc.session, Account: lc.account}) == d.f.SessionsTab.Chosen {
			c.surface(0, lc.cord.from, lc.cord.plug, hue{token: theme.BgSelection})
		}
	}
}

// boardShown is what of the Switchboard a view shows, scrolled: from its row
// first, rows rows of it, lead rows down the view; and what's out of view,
// over, said on the view's first row, over the rows shown, and under, on the
// row over the key line, each nothing where it says none.
type boardShown struct {
	first, lead, rows int
	over, under       line
}

// shownAt is what of the Switchboard the view shows, scrolled offset down:
// all of it, from its first row, but at step 5, where its lines scroll with
// their calls, each account's rows as laidLine has them. There, where every
// account fits the view, under the row saying how many are above where any
// are, it scrolls an account at a time, as accountsShown has it; else a row
// at a time, as rowsShown has it.
func (d boardDrawn) shownAt(offset int) boardShown {
	switch {
	case d.laid.step < compactedStep || len(d.laid.lines) == 0:
		return boardShown{rows: d.f.viewRows()}
	case d.byRows():
		return d.rowsShown(offset)
	}
	return d.accountsShown(offset)
}

// byRows reports whether the Switchboard scrolls a row at a time: at step 5,
// where an account's rows don't fit the view, under the row saying how many
// are above where any are, so an account at a time would cut its calls.
func (d boardDrawn) byRows() bool {
	if d.laid.step < compactedStep {
		return false
	}
	rows := d.f.viewRows()
	for i, ln := range d.laid.lines {
		if ln.tall()+min(i, 1) > rows {
			return true
		}
	}
	return false
}

// accountsShown is what of the Switchboard the view shows, scrolled offset
// accounts down, no further than mostScrolled: the accounts from that one
// that show whole, each from its first row to its last, and no part of the
// next, so no call is ever cut; scrolled down, how many accounts are above,
// on the view's first row; and how many are below, on the row over the key
// line, as usage says them.
func (d boardDrawn) accountsShown(offset int) boardShown {
	o := min(max(offset, 0), d.mostScrolled())
	from := d.laid.lines[o]
	s := boardShown{first: from.first, lead: min(o, 1)}
	n := d.wholeFrom(o, d.f.viewRows()-s.lead)
	s.rows = d.laid.lines[o+n-1].end() - s.first
	s.over = outOfView(o, 0, d.f.Width, usageOut)
	s.under = outOfView(0, len(d.laid.lines)-o-n, d.f.Width, usageOut)
	return s
}

// rowsShown is what of the Switchboard the view shows scrolled offset rows
// down, as rowsPaged pages it, and how many of its rows are out of view,
// above and below, on the row over the key line, as USAGE says it.
func (d boardDrawn) rowsShown(offset int) boardShown {
	paged := d.paged()
	from, to := paged.shown(offset)
	return boardShown{first: from, rows: to - from, under: outOfView(from, paged.content-to, d.f.Width, usageRowsOut)}
}

// paged is how the Switchboard's rows page in the view's, a row at a time.
func (d boardDrawn) paged() rowsPaged {
	return rowsPaged{rows: d.f.viewRows(), content: d.laid.rows}
}

// wholeFrom is how many accounts show whole from the o-th, from its first
// row, in rows rows.
func (d boardDrawn) wholeFrom(o, rows int) int {
	first, n := d.laid.lines[o].first, 0
	for _, ln := range d.laid.lines[o:] {
		if ln.end()-first > rows {
			break
		}
		n++
	}
	return n
}

// mostScrolled is the fewest accounts the Switchboard scrolls past to show
// its last account whole, at step 5, under the row saying how many are
// above.
func (d boardDrawn) mostScrolled() int {
	rows, n := d.f.viewRows(), len(d.laid.lines)
	for o := range n {
		if d.wholeFrom(o, rows-min(o, 1)) == n-o {
			return o
		}
	}
	return n - 1
}

// scrolling is how far the Switchboard scrolls, scrolled to offset: none
// but at step 5; there, a row at a time, where byRows says so, as paged has
// it; else in accounts, as far as shows its last account whole, as many at
// once as show whole from the offset, as accountsShown has them.
func (d boardDrawn) scrolling(offset int) Scrolling {
	switch {
	case d.laid.step < compactedStep || len(d.laid.lines) == 0:
		return Scrolling{}
	case d.byRows():
		return d.paged().scrolling(offset)
	}
	most := d.mostScrolled()
	o := min(max(offset, 0), most)
	return Scrolling{Most: most, Page: d.wholeFrom(o, d.f.viewRows()-min(o, 1))}
}

// targets are the calls the view shows, each a session's on an account, over
// its cells from its id to its plug, which a click chooses, then opens.
func (d boardDrawn) targets() []Target {
	s, top := d.shownAt(d.f.Scroll), d.f.top()
	var targets []Target
	for _, lc := range d.laid.calls {
		if row := lc.cord.from - s.first; row >= 0 && row < s.rows {
			targets = append(targets, Target{
				Y: top + s.lead + row, Width: d.laid.columns.plug + 1, On: OnSession, Session: lc.session, Account: lc.account,
			})
		}
	}
	return targets
}

// filled are how much of each bar the lines draw they fill, as Filled has
// them: of the windows the layout's step draws, but none drawn empty.
func (d boardDrawn) filled() map[Ref]float64 {
	drawn := d.lines.windowsDrawn()
	return windowsFilled(d.lines.m, func(w status.WindowNow) bool {
		return slices.Contains(drawn, w.Key) && !drawsEmpty(w.Kind)
	})
}
