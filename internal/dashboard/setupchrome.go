package dashboard

import (
	"strconv"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/setup"
)

// setupTabsAfter is the blank cells between setup's name and its first tab.
const setupTabsAfter = 4

// The columns a page sets out from: what it says, and a fact's mark; a
// fact's label; and what's said of it.
const (
	setupIndent  = 2
	factLabelAt  = 5
	factDetailAt = 26
)

var (
	setupName = line{{"setup", titleInk}}
	// setupApart sets the status line's parts apart.
	setupApart = span{"  ·  ", faintInk}
)

// title draws the title row, width cells wide: the dashboard's name, then
// setup's, then its steps as tabs, a cell apart, and ending at the last
// column, the clock, as the dashboard's title row has it. It reports whether
// the name and the tabs fit, as clocked has it.
func (s Setup) title(c *canvas, width int) bool {
	c.line(0, 0, pill)
	x := c.line(pill.width()+titleGap, 0, setupName) + setupTabsAfter
	for i, step := range SetupSteps() {
		if i > 0 {
			x++
		}
		x = c.line(x, 0, s.tab(step))
	}
	return clocked(c, width, x, s.Now)
}

// tab is the step's tab, its number and name with a space either side, Done
// with no number: picked out where it's the step showing, muted where it's
// passed through, else subtle. A step passed through has a bold ✓ in
// state.positive in its number's place.
func (s Setup) tab(step SetupStep) line {
	k := tabInk
	switch {
	case step == s.Step:
		k = shownTabInk
	case s.passed(step):
		k = mutedInk
	}
	if !step.numbered() {
		return line{{" " + step.Title() + " ", k}}
	}
	number := span{strconv.Itoa(int(step) + 1), k}
	if s.passed(step) {
		tick := boldPositiveInk
		tick.on = k.on
		number = span{"✓", tick}
	}
	return line{{" ", k}, number, {" " + step.Title() + " ", k}}
}

// statusLine draws the status line along row y, width cells wide: its
// parts, setupApart between each, cut where they don't fit.
func (s Setup) statusLine(c *canvas, y, width int) {
	parts := make([]line, len(s.Status))
	for i, p := range s.Status {
		parts[i] = p.line()
	}
	c.line(0, y, joined(setupApart, parts...).fit(width))
}

// line is the part as the status line says it: a part that's up, ● in
// state.positive, and one that's not up yet, ○ in text.subtle, each before
// its words in text.secondary; a part told alone, its words muted.
func (p SetupPart) line() line {
	switch p.Standing {
	case SetupUp:
		return line{{"● ", positiveInk}, {p.Words, secondaryInk}}
	case SetupDown:
		return line{{"○ ", dimInk}, {p.Words, secondaryInk}}
	}
	return line{{p.Words, mutedInk}}
}

// setupPage is a step's page, under the status line: its heading, and
// what's said at the heading's right, where anything is; what the step is
// for, where it says, in the lines the page breaks it into; what it found
// and did, a fact a row; and its keys, at the key line's left, and onward,
// at its right.
type setupPage struct {
	name, tail   string
	purpose      []line
	facts        []setup.Said
	keys, onward []Key
}

// underHeading is the rows from a page's heading row to its first under it.
const underHeading = 2

// draw draws the page from row top, rows tall and width cells wide, and
// says how far it scrolls: its heading row, as the dashboard heads a
// section; and from underHeading rows on, what's under it, as content draws
// it, scrolled scroll rows down, as paged pages it.
func (p setupPage) draw(c *canvas, top, rows, width, scroll int) Scrolling {
	h := heading{name: p.name}
	if p.tail != "" {
		h.right = line{{p.tail, mutedInk}}
	}
	for _, piece := range h.pieces(top, width, false) {
		c.line(piece.x, piece.y, piece.line)
	}
	whole := p.content(width)
	paged := rowsPaged{rows: rows - underHeading, content: whole.height(), says: true}
	paged.paste(c, whole, top+underHeading, scroll, width, historyRowsOut...)
	return paged.scrolling(scroll)
}

// content is what's under the page's heading, drawn whole width cells wide:
// from setupIndent, what the step is for, each of its lines wrapped further
// where it's wider than the page; then a row on, its facts, a blank row
// between each.
func (p setupPage) content(width int) *canvas {
	c := growing(width)
	y := 0
	for _, said := range p.purpose {
		for _, l := range said.wrap(width - setupIndent) {
			c.line(setupIndent, y, l)
			y++
		}
	}
	if p.purpose != nil {
		y++
	}
	for _, said := range p.facts {
		y = fact(c, y, width, said) + 1
	}
	return c
}

// The parts of a fact's row, as factParts lays them out.
const (
	factIndent = iota
	factMark
	factLabel
	factDetail
)

// factParts are a fact's row's parts, across width cells: setupIndent
// cells, its mark, a cell, then two cells on, its label, cut to end a cell
// before factDetailAt, and what's said of it, taking the rest.
func factParts(width int) []flex.Span {
	label := factDetailAt - factLabelAt - 1
	spans, _ := flex.Lay(width, []flex.Part{
		factIndent: {Least: setupIndent, Natural: setupIndent},
		factMark:   {Least: 1, Natural: 1},
		factLabel:  {Least: label, Natural: label, Gap: factLabelAt - setupIndent - 1},
		factDetail: {Least: 1, Grows: true, Gap: 1},
	}...)
	return spans
}

// fact draws what a step said of one thing along row y, width cells wide,
// as factParts lays it out: its mark, as markOf has it; its label, in
// text.secondary; and what's said of it, muted, wrapped onto the rows under
// it. It returns the row after the last it drew on.
func fact(c *canvas, y, width int, said setup.Said) int {
	spans := factParts(width)
	c.line(spans[factMark].At, y, line{markOf(said.Mark)})
	label, detail := spans[factLabel], spans[factDetail]
	c.line(label.At, y, line{{said.Label, secondaryInk}}.fit(label.Size))
	next := y + 1
	for i, l := range (line{{said.Detail, mutedInk}}).wrap(detail.Size) {
		c.line(detail.At, y+i, l)
		next = y + i + 1
	}
	return next
}

// markOf is the mark of how what a step found stands, bold: ✓ in
// state.positive for done; ! in accent.attention for done but for something
// the user must do; ✗ in state.destructive for failed.
func markOf(m setup.Mark) span {
	switch m {
	case setup.Warning:
		return span{"!", alertInk}
	case setup.Failed:
		return span{"✗", exhaustedInk}
	}
	return span{"✓", boldPositiveInk}
}

// onward are the keys at the right of a page's key line: enter, doing what
// does says, as next: Priming, then q quit.
func onward(does string) []Key {
	return []Key{{Key: "enter", Does: does}, Quit}
}

// keyLine draws the page's key line along row y, width cells wide: its
// onward keys ending at the last column, and from the first, as many of its
// own keys as fit whole, keysApart clear of them; each key's words as
// Key.words has them, keysApart apart.
func (p setupPage) keyLine(c *canvas, y, width int) {
	room := width
	if right := keyRow(p.onward); right != nil {
		room = c.right(width, y, right) - keysApart
	}
	x := 0
	for i, k := range p.keys {
		gap := 0
		if i > 0 {
			gap = keysApart
		}
		words := k.words()
		if x+gap+words.width() > room {
			return
		}
		x = c.line(x+gap, y, words)
	}
}
