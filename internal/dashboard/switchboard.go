package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
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
	return sessionsForm{draw: d.draw, scrolling: d.scrolling, targets: d.targets, filled: d.filled}
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
// scrolled as the frame has it, as shownAt has it: where accounts are out of
// view above, how many, over the rest, and where they are below, how many,
// on the row over the key line, each right-aligned, as usage says them.
func (d boardDrawn) draw(c *canvas, top int) {
	s := d.shownAt(d.f.Scroll)
	if s.above > 0 {
		c.right(d.f.Width, top, outOfView(s.above, 0, d.f.Width, usageOut))
	}
	c.paste(d.board(), s.first, top+s.lead, s.rows)
	if s.below > 0 {
		c.right(d.f.Width, d.f.Height-underView, outOfView(0, s.below, d.f.Width, usageOut))
	}
}

// noSessionsRunning is what the Switchboard's first call's row says while
// none runs, as while no router lists any.
const noSessionsRunning = "no sessions running"

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

// call draws the call lc along its row: its session's id, cut to 4, bold
// text.primary while it works, else text.muted, and its directory, cut to
// the cells the columns give it, in text.muted; what it's doing, as doing
// says it; where it came from, as from says it, while the move that brought
// it is its latest; and its cord, as cord draws it.
func (d boardDrawn) call(c *canvas, lc laidCall) {
	call := d.calls[Plug{Session: lc.session, Account: lc.account}]
	k, y := d.laid.columns, lc.cord.from
	works := call.Activity.Works()
	id := mutedInk
	if works {
		id = titleInk
	}
	x := c.text(0, y, status.ShownID(lc.session), id)
	if dir := status.DirName(call.Dir, k.dir); dir != "" {
		c.text(x, y, " "+dir, mutedInk)
	}
	c.line(k.doing, y, doing(call.Activity, d.lines.m.Now))
	if call.From != "" {
		c.line(k.from, y, d.from(call.From))
	}
	d.cord(c, lc, works)
}

// doing is what a call's session is doing at now, as a session's page says
// it: ↑ asking or ↓ answering, bold accent.key; refused, bold
// state.destructive; throttled, text.subtle; else idle, text.faint.
func doing(a views.Activity, now time.Time) line {
	k := faintInk
	switch a.Doing {
	case views.Asking, views.Answering:
		k = keyInk
	case views.Refused:
		k = exhaustedInk
	case views.Throttled:
		k = dimInk
	}
	return line{{a.Said(now), k}}
}

// from is where a call came from, the account with the given id it left:
// ▸ in accent.key, from in text.muted, where the columns keep it, and the
// account's name, cut to the cells they give it, in text.secondary.
func (d boardDrawn) from(id string) line {
	k := d.laid.columns
	words := " from "
	if k.fromShort {
		words = " "
	}
	return line{{"▸", keyMarkInk}, {words, mutedInk}, {prose.Cut(named(d.lines.m, id), k.account), secondaryInk}}
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
// second. A row that would pass the last column starts at column 0.
func (d boardDrawn) key(c *canvas) {
	if d.laid.keyAt < 0 {
		return
	}
	m := d.lines.m
	b := blockOf(m, d.f.Width, d.f.Look, ByAccount, d.lines.windows, d.lines.names)
	b.marks = keyMarks(d.lines.names, false)
	pace, after := b.keyParts(d.lines.barDrawn())
	for i, l := range []line{pace, joined(keyGap(d.f.Width), after...)} {
		x := d.laid.columns.lines + 1
		if x+l.width() > d.f.Width {
			x = 0
		}
		c.line(x, d.laid.keyAt+i, l.fit(d.f.Width))
	}
}

// chosen picks out the call chosen, where one is, on bg.selection across the
// width, its glyphs and colours, its cord's too, as they are.
func (d boardDrawn) chosen(c *canvas) {
	for _, lc := range d.laid.calls {
		if (Plug{Session: lc.session, Account: lc.account}) == d.f.SessionsTab.Chosen {
			c.surface(0, lc.cord.from, d.f.Width, hue{token: theme.BgSelection})
		}
	}
}

// boardShown is what of the Switchboard a view shows, scrolled: from its row
// first, rows rows of it, lead rows down the view, under the row saying how
// many accounts are above; and how many accounts are out of view above and
// below.
type boardShown struct {
	first, lead, rows int
	above, below      int
}

// shownAt is what of the Switchboard the view shows, scrolled offset
// accounts down: all of it, from its first row, but at step 5, where its
// lines scroll an account at a time, each account's first row its line's or
// its group's, whichever is higher, its calls moving with it. Scrolled down,
// the view's first row says how many are above; and how many are below that
// it doesn't show whole is said on the row over the key line.
func (d boardDrawn) shownAt(offset int) boardShown {
	rows := d.f.viewRows()
	if d.laid.step < compactedStep || len(d.laid.lines) == 0 {
		return boardShown{rows: rows}
	}
	o := min(max(offset, 0), d.mostScrolled())
	s := boardShown{first: d.laid.lines[o].first, rows: rows, above: o}
	if o > 0 {
		s.lead, s.rows = 1, rows-1
	}
	s.below = len(d.laid.lines) - o - d.wholeFrom(o, s.first, s.rows)
	return s
}

// wholeFrom is how many accounts' lines show whole from the o-th, the view
// showing rows rows from row first.
func (d boardDrawn) wholeFrom(o, first, rows int) int {
	n := 0
	for _, ln := range d.laid.lines[o:] {
		if ln.top+ln.rows-first > rows {
			break
		}
		n++
	}
	return n
}

// mostScrolled is the fewest accounts the Switchboard scrolls past to show
// its last line whole, at step 5.
func (d boardDrawn) mostScrolled() int {
	rows := d.f.viewRows()
	n := len(d.laid.lines)
	for o := range n {
		lead := min(o, 1)
		if d.wholeFrom(o, d.laid.lines[o].first, rows-lead) == n-o {
			return o
		}
	}
	return n - 1
}

// scrolling is how far the Switchboard scrolls, scrolled to offset, in
// accounts: none but at step 5, where it goes as far as shows its last line
// whole, as many accounts at once as it shows whole there.
func (d boardDrawn) scrolling(offset int) Scrolling {
	if d.laid.step < compactedStep || len(d.laid.lines) == 0 {
		return Scrolling{}
	}
	s := d.shownAt(offset)
	return Scrolling{Most: d.mostScrolled(), Page: d.wholeFrom(s.above, s.first, s.rows)}
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
