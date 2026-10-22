package dashboard

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// The panel's width, its edges included, as its two sides were signed off
// at, the least natural it has; and the rows of the frame it leaves spare at
// its tallest, past which it scrolls, the cursor's row kept in view.
const (
	panelSignedOff = 124
	panelSpare     = 2
)

// The panel's columns, from its content's start, where they differ by the
// table it's of: where ONLY starts; and within ONLY, where today's
// sessions start.
var (
	onlyAt  = [...]int{LogRequests: 40, LogEvents: 58}
	todayAt = [...]int{LogRequests: 12, LogEvents: 1}
)

// The panel's other columns and counts: within ONLY, where a row's choices
// start after its label, how far apart its ticks are, and how many of
// today's sessions show at once; within COLUMNS, how far apart its columns
// are, how many columns each of its first two holds, and where its third,
// of the rest, is; within Events' kinds, where today's count of a kind ends
// and what it is starts.
const (
	choicesAt      = 10
	ticksApart     = keysApart
	sessionsInView = 6
	columnsApart   = 13
	columnsDown    = 6
	columnsRestAt  = 2 * columnsApart
	kindCountEnd   = 19
	kindMeansAt    = 21
)

// pane is a part of the panel's body laid out: words put at a row and a
// column each, and the ticks among them, where they're placed.
type pane struct {
	puts  []put
	spots []placedTick
}

// put is words put at a row and a column of a pane.
type put struct {
	row, x int
	words  line
}

// placedTick is a tick as it's placed: the side it's on, what it ticks, its
// row and its column, and the cells its words take, at its widest.
type placedTick struct {
	side      int
	on        tickOn
	row, x, w int
}

// put puts words at row and column x.
func (p *pane) put(row, x int, words line) {
	p.puts = append(p.puts, put{row: row, x: x, words: words})
}

// tick puts the tick t at row and column x, placing it as taking cells, or
// its words', where they're more.
func (p *pane) tick(row, x int, t tick, cells int) {
	words := t.words()
	p.put(row, x, words)
	p.spots = append(p.spots, placedTick{on: t.on, row: row, x: x, w: max(cells, words.width())})
}

// beside puts o in p from row and column x, its ticks on the side given.
func (p *pane) beside(o pane, row, x, side int) {
	for _, u := range o.puts {
		p.put(u.row+row, u.x+x, u.words)
	}
	for _, s := range o.spots {
		s.side, s.row, s.x = side, s.row+row, s.x+x
		p.spots = append(p.spots, s)
	}
}

// height is how many rows the pane takes.
func (p pane) height() int {
	n := 0
	for _, u := range p.puts {
		n = max(n, u.row+1)
	}
	return n
}

// rows are the pane's rows, each its words in the order of their columns.
func (p pane) rows() []line {
	rows := make([]line, p.height())
	puts := slices.SortedStableFunc(slices.Values(p.puts), func(a, b put) int { return cmp.Or(a.row-b.row, a.x-b.x) })
	for _, u := range puts {
		rows[u.row] = padTo(rows[u.row], u.x, u.words)
	}
	return rows
}

// card is the panel on the frame, titled as the table it's of: in two
// sides, where bothFit says they fit, else the side the cursor's on alone;
// the tick the cursor's on lit a cell either side; as tall as its body, up
// to the frame's height less panelSpare, past which it shows the rows
// around the cursor's.
func (p LogPanel) card(f Frame) Card {
	return p.cardOf(p.laidOut(f))
}

// cardOf is the panel's card, width cells wide, of its body: shown rows of
// it, from the row from.
func (p LogPanel) cardOf(width int, body pane, from, shown int) Card {
	rows := body.rows()
	k := Card{
		title: "LOG · " + strings.ToUpper(logViews[p.Of]) + " · WHAT IT SHOWS", width: width,
		body: rows[from : from+shown], keys: panelKeysIn(width - 2*cardInset),
		scroll: cardScroll{above: from, below: len(rows) - from - shown},
	}
	if s, ok := p.lit(body); ok && s.row >= from && s.row < from+shown {
		k.lit = bodyRows{first: s.row - from, n: 1, x: s.x, cells: s.w + 2}
	}
	return k
}

// laidOut is the panel as the frame lays it out: its width; its body; and
// from which row of it, and how many, it shows.
func (p LogPanel) laidOut(f Frame) (width int, body pane, from, shown int) {
	width = f.cardWidth(p.natural())
	body = p.body(width-2*cardInset, p.bothFit(width-2*cardInset))
	rows := body.height()
	shown = min(max(f.bodyRoom(panelSpare, true), 0), rows)
	if s, ok := p.lit(body); ok {
		from = min(max(s.row+1-shown, 0), rows-shown)
	}
	return width, body, from, shown
}

// natural is the panel's natural width, its edges included: the width its
// two sides were signed off at, or wider where its parts at their naturals,
// as a long configured name has them, need more, cardInset in from either
// edge.
func (p LogPanel) natural() int {
	s, _ := flex.Lay(math.MaxInt, p.sideParts()...)
	last := s[len(s)-1]
	return max(panelSignedOff, last.At+last.Size+2*cardInset)
}

// bothFit reports whether the panel's two sides fit content cells, as flex
// lays them at their leasts.
func (p LogPanel) bothFit(content int) bool {
	_, fits := flex.Lay(content, p.sideParts()...)
	return fits
}

// sideParts are the parts of the panel's two sides across a row: the left
// side, to where ONLY starts; and ONLY, as wide as today's sessions' rows,
// its widest part.
func (p LogPanel) sideParts() []flex.Part {
	return append([]flex.Part{{Least: onlyAt[p.Of], Natural: onlyAt[p.Of]}}, p.sessionParts()...)
}

// lit is the tick the cursor's on, as it's placed in body.
func (p LogPanel) lit(body pane) (placedTick, bool) {
	ticks := p.sideTicks(p.side)
	if len(ticks) == 0 {
		return placedTick{}, false
	}
	on := ticks[p.cursor(ticks)].on
	i := slices.IndexFunc(body.spots, func(s placedTick) bool { return s.side == p.side && s.on == on })
	if i < 0 {
		return placedTick{}, false
	}
	return body.spots[i], true
}

// Clicked is the panel on the frame f with the tick at x along row y
// ticked, as space ticks it, the cursor moved to it: false where no tick is
// there.
func (p LogPanel) Clicked(f Frame, x, y int) (LogPanel, bool) {
	width, body, from, shown := p.laidOut(f)
	left, top, _, _ := p.cardOf(width, body, from, shown).box(f.Width, f.Height)
	col, row := x-left-cardInset, y-top-2+from
	if row < from || row >= from+shown {
		return p, false
	}
	for _, s := range body.spots {
		if s.row == row && s.x-1 <= col && col <= s.x+s.w {
			p.side, p.on[s.side] = s.side, s.on
			return p.Ticked(), true
		}
	}
	return p, false
}

// body is the panel's body, width cells wide: what it's for; then its two
// sides, where both is set, else the cursor's alone; and what it would show.
func (p LogPanel) body(width int, both bool) pane {
	var b pane
	row := putLines(&b, 0, p.purpose().wrap(width)) + 1
	switch {
	case both:
		b.beside(p.leftPane(), row, 0, 0)
		b.beside(p.onlyPane(width-onlyAt[p.Of]), row, onlyAt[p.Of], 1)
	case p.side == 0:
		b.beside(p.leftPane(), row, 0, 0)
	default:
		b.beside(p.onlyPane(width), row, 0, 1)
	}
	putLines(&b, b.height()+1, p.shows().wrap(width))
	return b
}

// putLines puts lines in b a row each from row, giving the row after them.
func putLines(b *pane, row int, lines []line) int {
	for _, l := range lines {
		b.put(row, 0, l)
		row++
	}
	return row
}

// purpose is what the panel is for, in text.secondary: of Requests', with
// how many lines today has.
func (p LogPanel) purpose() line {
	if p.Of == LogEvents {
		return line{{"Pick which events it shows.", secondaryInk}}
	}
	return line{{"Pick the columns it shows, and which of today's " + prose.Counted(p.Lines.Lines(), "line") + ".", secondaryInk}}
}

// shows is what the panel would show, in text.secondary, its figures bold
// text.primary: of Requests, how many of today's lines, in how many
// columns; of Events, how many of today's events, all of them where it
// hides none, and where there are none, that there are none yet, in
// text.faint.
func (p LogPanel) shows() line {
	said := line{{"It shows ", secondaryInk}}
	if p.Of == LogRequests {
		return slices.Concat(said,
			line{{prose.Grouped(p.Lines.Shown), titleInk}, {" of ", secondaryInk}},
			countedSaid(p.Lines.Lines(), "line", titleInk, secondaryInk),
			line{{", in ", secondaryInk}},
			countedSaid(p.Columns.Count(), "column", titleInk, secondaryInk),
			line{{".", secondaryInk}},
		)
	}
	switch c := p.Events; {
	case c.All == 0:
		return line{{"It shows today's events: none yet.", faintInk}}
	case c.Shown == c.All:
		return slices.Concat(said, line{{"all ", secondaryInk}, {prose.Grouped(c.All), titleInk}, {" of today's events.", secondaryInk}})
	default:
		return slices.Concat(said,
			line{{prose.Grouped(c.Shown), titleInk}, {" of today's ", secondaryInk}},
			countedSaid(c.All, "event", titleInk, secondaryInk),
			line{{".", secondaryInk}},
		)
	}
}

// countedSaid counts n of the thing a noun names, as views.CountedWords
// counts it, its figure in the ink given and the words after it in rest's.
func countedSaid(n int, noun string, figure, rest ink) line {
	said := views.CountedWords(n, noun)
	return line{{said[0].Text, figure}, {said[1].Text, rest}}
}

// leftPane is the panel's left side: of Requests, COLUMNS; of Events, the
// kinds of event.
func (p LogPanel) leftPane() pane {
	if p.Of == LogEvents {
		return p.eventKindsPane()
	}
	return p.columnsPane()
}

// columnsPane is COLUMNS: its heading, then a tick a column, those shown as
// it opened first, columnsDown to a column in its first two, then the rest,
// shown then hidden, in its third.
func (p LogPanel) columnsPane() pane {
	var pn pane
	pn.put(0, 0, line{{"COLUMNS", dimInk}})
	shown, _ := p.laidColumns()
	paired := min(len(shown), 2*columnsDown)
	for i, t := range p.columnTicks() {
		row, x := i-paired, columnsRestAt
		if i < paired {
			row, x = i%columnsDown, i/columnsDown*columnsApart
		}
		pn.tick(1+row, x, t, 0)
	}
	return pn
}

// eventKindsPane is Events' kinds: its heading, then a row a kind, its tick,
// today's count of it, in text.secondary, text.faint at none, and what it
// is, in text.muted.
func (p LogPanel) eventKindsPane() pane {
	var pn pane
	pn.put(0, 0, line{{"EVENTS", dimInk}})
	pn.put(0, kindCountEnd-len("today"), line{{"today", dimInk}})
	for i, t := range p.eventKindTicks() {
		row, n := 1+i, p.Events.Kinds[t.on.id]
		count := span{prose.Grouped(n), secondaryInk}
		if n == 0 {
			count.ink = faintInk
		}
		pn.tick(row, 0, t, 0)
		pn.put(row, kindCountEnd-line{count}.width(), line{count})
		pn.put(row, kindMeansAt, line{{eventKinds[i].means, mutedInk}})
	}
	return pn
}

// onlyPane is ONLY, width cells wide: its heading; a row a choice, its
// label, then its ticks, a row too long for them wrapping under its label;
// then today's sessions, sessionsInView of them, those around the cursor
// where it's among them, laid out as sessionColumns has them, and a line
// saying how many more there are and how to find one from another day.
func (p LogPanel) onlyPane(width int) pane {
	var pn pane
	pn.put(0, 0, line{{"ONLY", dimInk}})
	row := 1
	for _, r := range p.choiceRows() {
		pn.put(row, 0, line{{r.label, dimInk}})
		pn.put(row, choicesAt, r.words)
		x := choicesAt
		for _, t := range r.ticks {
			w := t.words().width()
			if x > choicesAt && x+w > width {
				row, x = row+1, choicesAt
			}
			pn.tick(row, x, t, 0)
			x += w + ticksApart
		}
		row++
	}
	at := p.sessionColumns(width)
	ticks := p.sessionTicks()
	first, end := p.sessionsShown(ticks)
	for i, t := range ticks[first:end] {
		t.after = p.sessionWords(p.Sessions[first+i], at.account)
		pn.tick(row, at.at, t, at.cells)
		row++
	}
	pn.put(row, at.at, moreToday(len(ticks)-end))
	return pn
}

// sessionsShown are which of today's sessions' ticks show, from first to
// end: the first sessionsInView, or where the cursor's on one past them,
// those up to it.
func (p LogPanel) sessionsShown(ticks []tick) (first, end int) {
	if p.side == 1 {
		if i := slices.IndexFunc(ticks, func(t tick) bool { return t.on == p.on[1] }); i >= sessionsInView {
			first = i + 1 - sessionsInView
		}
	}
	return first, min(first+sessionsInView, len(ticks))
}

// moreToday says how many more of today's sessions there are below those
// shown, where there are any, and that / finds one from another day.
func moreToday(n int) line {
	var said line
	if n > 0 {
		said = line{{"▼", doingInk}, {" " + prose.Grouped(n) + " more today   ", mutedInk}}
	}
	return append(said, span{"/", keyInk}, span{" finds one from another day", mutedInk})
}

// panelKeys are the panel's keys, in its keys row's order.
var panelKeys = []Key{
	{Key: "←→", Does: "side"}, {Key: "↑↓", Does: "move"}, {Key: "space", Does: "tick"},
	{Key: "enter", Does: "apply"}, {Key: "esc", Does: "cancel"},
}

// panelKeysIn are as many of the panel's keys as fit width cells, from the
// first.
func panelKeysIn(width int) []Key {
	n := len(panelKeys)
	for n > 1 && keyRow(panelKeys[:n]).width() > width {
		n--
	}
	return panelKeys[:n]
}
