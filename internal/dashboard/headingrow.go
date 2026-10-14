package dashboard

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/theme"
)

// The blank cells the heading row sets its parts apart by: before the
// sub-tabs and before the state; between the rule and what's either side of
// it, but the state, which the rule starts a cell after; and on a phone,
// before the sub-tabs.
const (
	headingGap = 3
	ruleGap    = 2
	compactGap = 2
)

// shownSubTabInk picks out the sub-tab shown.
var shownSubTabInk = ink{token: theme.TextPrimary, on: hue{token: theme.BgSubtle}, bold: true}

// heading is a tab's heading row, as the tab gives it: its name; its
// sub-tabs, where it has them, which of them is shown, and whether a phone
// sets them tight, as the rule Accounts and History share has it; the state
// the screen can't otherwise show, in text.faint, a key within it in bold
// accent.key, where there's any, and its shorter forms, each a clause fewer,
// for less room; and at its right, its period or window, which < and > step,
// or another line, as the Log's count and its f.
type heading struct {
	name    string
	subTabs []string
	shown   int
	tight   bool
	state   line
	shorter []line
	period  string
	right   line
}

// piece is a part of the heading row as it's laid out: a line, from x along
// row y, and what a click on it does, as a target has it but for where it
// is: nothing where its On is zero.
type piece struct {
	x, y  int
	line  line
	click Target
}

// target is the piece as a click's target, over the cells it's drawn on.
func (p piece) target() Target {
	t := p.click
	t.X, t.Y, t.Width = p.x, p.y, p.line.width()
	return t
}

// heading is the heading row of the tab shown: Accounts', as accountsHeading
// has it; History's, Sessions', Runway's and the Log's, as theirs have them;
// else its name alone.
func (f Frame) heading() heading {
	switch f.View {
	case Accounts:
		return f.accountsHeading()
	case History:
		return f.History.heading()
	case Sessions:
		return f.SessionsTab.heading()
	case Runway:
		return f.Runway.heading()
	case Log:
		return f.Log.heading(f.phone(), f.Width)
	}
	return heading{name: strings.ToUpper(f.View.Title())}
}

// headingRow draws the tab's heading row, as headingPieces lays it out.
func (f Frame) headingRow(c *canvas) {
	for _, p := range f.headingPieces() {
		c.line(p.x, p.y, p.line)
	}
}

// headingPieces are the heading row of the tab shown, laid out under the
// status line: none on the Overview, whose sections head themselves, nor
// under a session's page, which heads itself, nor on a page too short for
// its view, as cramped says.
func (f Frame) headingPieces() []piece {
	if f.View == Overview || f.Session != nil || f.cramped() {
		return nil
	}
	return f.heading().pieces(f.headingY(), f.Width, f.phone())
}

// rows is how many rows the heading takes, on a phone where phone says: two
// where its period takes the row under it, as ownPeriod has it; else one.
func (h heading) rows(phone bool) int {
	if h.ownPeriod(phone) {
		return 2
	}
	return 1
}

// compact reports whether the heading is drawn as a phone draws a tab's
// with sub-tabs: without [ ] or a rule.
func (h heading) compact(phone bool) bool {
	return phone && len(h.subTabs) > 0
}

// tightly reports whether the heading's sub-tabs are set tight, as a phone
// sets them where the heading says to: each its name and a cell after it.
func (h heading) tightly(phone bool) bool {
	return h.tight && h.compact(phone)
}

// ownPeriod reports whether the heading's period takes the row under it, as
// a phone has it where the heading is compact. A tab without sub-tabs, as
// Runway, keeps its rule and its window on its row.
func (h heading) ownPeriod(phone bool) bool {
	return h.compact(phone) && h.period != ""
}

// pieces are the heading's parts, laid out along row y of a frame width
// cells wide, on a phone where phone says: its name and sub-tabs, as named
// lays them out; then three cells on, the state, or the first of its shorter
// forms that fits where the rest leaves it short of room, cut where none
// does. Ending at the last column, its period between < and >, or
// its line, on the row under it where ownPeriod says, and left out where it
// doesn't fit beside the name and the sub-tabs. Between them, the rule, from
// two cells after them, or one after the state, to two cells short of what
// sits at the right, or to the last column where nothing does. A compact
// heading has no rule.
func (h heading) pieces(y, width int, phone bool) []piece {
	var laid laying
	x := h.named(&laid, y, phone)
	right, rightY, end := h.rightPieces(), y, width
	switch {
	case h.ownPeriod(phone):
		rightY++
	case width-ends(right) < x+ruleGap:
		right = nil
	case right != nil:
		end = width - ends(right) - ruleGap
	}
	ruled := x + ruleGap
	if h.state != nil {
		ruled = laid.put(x+headingGap, y, firstFit(end-x-headingGap, slices.Concat([]line{h.state}, h.shorter)...), Target{}) + 1
	}
	if !h.compact(phone) {
		laid.put(ruled, y, line{{rule(end - ruled), borderInk}}, Target{})
	}
	from := width - ends(right)
	for _, p := range right {
		from = laid.put(from, rightY, p.line, p.click)
	}
	return laid
}

// named lays the heading's name out from the first column of row y, then,
// where it has them, its sub-tabs, touching, three cells on, or two where
// it's compact, set tight or not, as the Log's and History's phone frames
// draw them, and a cell on, [ ], but where it's compact, or has one sub-tab
// alone, with none to turn to; and returns the column after them.
func (h heading) named(laid *laying, y int, phone bool) int {
	x := laid.put(0, y, line{{h.name, dimInk}}, Target{})
	if len(h.subTabs) == 0 {
		return x
	}
	gap := headingGap
	if h.compact(phone) {
		gap = compactGap
	}
	x += gap
	for i := range h.subTabs {
		x = laid.put(x, y, h.subTab(i, phone), Target{On: OnSubTab, SubTab: i})
	}
	if h.compact(phone) || len(h.subTabs) == 1 {
		return x
	}
	return laid.put(x+1, y, line{{"[ ]", faintInk}}, Target{})
}

// laying is a heading row as it's being laid out: its pieces so far.
type laying []piece

// put lays l out from x along row y, a click on it doing what click says,
// where it takes any cells, and returns the column after it.
func (l *laying) put(x, y int, ln line, click Target) int {
	if ln.width() > 0 {
		*l = append(*l, piece{x: x, y: y, line: ln, click: click})
	}
	return x + ln.width()
}

// subTab is the heading's sub-tab i, on a phone where phone says, its name
// with a space either side, or after it alone where it's set tight, picked
// out, the space with it, where it's the one shown.
func (h heading) subTab(i int, phone bool) line {
	name := " " + h.subTabs[i] + " "
	if h.tightly(phone) {
		name = h.subTabs[i] + " "
	}
	if i == h.shown {
		return line{{name, shownSubTabInk}}
	}
	return line{{name, tabInk}}
}

// rightPieces are what sits at the heading's right, in order, where they
// are yet to be placed: its period, with < before it and > after, each a
// step back and on; else its line; else nothing.
func (h heading) rightPieces() []piece {
	switch {
	case h.period != "":
		return []piece{
			{line: line{{"<", keyInk}}, click: Target{On: OnStep, Step: -1}},
			{line: line{{" " + h.period + " ", mutedInk}}},
			{line: line{{">", keyInk}}, click: Target{On: OnStep, Step: 1}},
		}
	case h.right != nil:
		return []piece{{line: h.right}}
	default:
		return nil
	}
}

// ends is how many cells the pieces take, laid end to end.
func ends(pieces []piece) int {
	n := 0
	for _, p := range pieces {
		n += p.line.width()
	}
	return n
}
