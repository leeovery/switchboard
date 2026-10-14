package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// The cells of a line's parts as its steps draw them: a window's label, the
// first's a cell in, and at step 3, the second's beside it; and at step 4,
// its 5-hour bar's.
const (
	firstLabelCells  = windowLabelCells + 1
	secondLabelCells = 5
	oneRowBar        = 14
)

// stepThreeLabels are the cells of the labels of step 3's two windows, side
// by side.
var stepThreeLabels = []int{firstLabelCells, secondLabelCells}

// widestLines are the columns the lines take at their widest, from 145
// columns: narrower, step 4's state words take the short form.
const widestLines = lineBeside + lineBar + lineWords

// outOfPoolSpan ends the name row of an account the global pin leaves out
// of the pool, in text.faint, cross and words.
var outOfPoolSpan = span{"✕ out of the pool", faintInk}

// linesDrawn are the Switchboard's lines of a moment as its form draws them,
// laid out as laid lays them: each account's name as the Overview names it,
// faint where it's out of the pool, in a name column fitted to them as usage
// fits it at the frame's width; and the windows usage shows of it.
type linesDrawn struct {
	f       Frame
	m       status.Moment
	laid    boardLayout
	names   []accountName
	column  int
	windows []shownWindow
	beside  []shownWindow
}

// linesDrawn are the Switchboard's lines of m, laid out as laid lays them.
func (f Frame) linesDrawn(m status.Moment, laid boardLayout) linesDrawn {
	names := overviewNames(m)
	return linesDrawn{
		f: f, m: m, laid: laid, names: names, column: nameColumn(names, f.Width),
		windows: usageWindows(m, nil, f.windowNamer(m)), beside: f.besideWindows(m),
	}
}

// draw draws each account's line on c from the row the layout gives it.
func (d linesDrawn) draw(c *canvas) {
	for _, ln := range d.laid.lines {
		if i := slices.IndexFunc(d.m.Accounts, func(a status.AccountNow) bool { return a.ID == ln.account }); i >= 0 {
			d.line(c, d.m.Accounts[i], d.names[i], ln.top)
		}
	}
}

// x is the column the lines start at.
func (d linesDrawn) x() int {
	return d.laid.columns.lines
}

// width is the lines' columns, from the column they start at to the edge.
func (d linesDrawn) width() int {
	return d.f.Width - d.x()
}

// line draws account a's line, named as given, from row y, at the step the
// layout takes: at steps 1 and 2, its name row, then a row a window, a blank
// row between at step 1; at step 3, its name row, then its 5-hour and its
// week side by side; and from step 4, all on one row.
func (d linesDrawn) line(c *canvas, a status.AccountNow, n accountName, y int) {
	switch d.laid.step {
	case 1, 2:
		apart := 2
		if d.laid.step == 2 {
			apart = 1
		}
		d.nameRow(c, a, n, y)
		for j, w := range d.windowsShown() {
			d.window(c, a, w, y+1+j*apart)
		}
	case 3:
		d.nameRow(c, a, n, y)
		d.sideBySide(c, a, y+1)
	default:
		d.oneRow(c, a, n, y)
	}
}

// nameRow draws a's name row along row y: its number, name and marks, then
// from the name column, its state in ROUTING's words, ending where the pin's
// mark needs the room, as stands has them.
func (d linesDrawn) nameRow(c *canvas, a status.AccountNow, n accountName, y int) {
	c.line(d.x(), y, n.line(d.column))
	d.stands(c, a, d.x()+d.column, y, d.laid.columns.shortWords)
}

// stands draws how a stands from x along row y, in ROUTING's words, or
// short, their short form, cut by whole clauses to end wordsGap cells short
// of the pin's mark, which ends the row at the last column: ● pinned on an
// account the global pin names, ✕ out of the pool, faint, on one it leaves
// out, as pinMark has it; picked out while its state is newly changed, as
// the Overview's ROUTING picks it out.
func (d linesDrawn) stands(c *canvas, a status.AccountNow, x, y int, short bool) {
	end := d.f.Width
	if mark := pinMark(a); mark != nil {
		end = c.right(d.f.Width, y, mark) - wordsGap
	}
	drawn := c.line(x, y, standingWords(a.Routing, short, end-x))
	d.f.cueStanding(c, a.ID, x, y, drawn-x)
}

// pinMark is what ends account a's name row on the Switchboard, as the global
// pin has it: ● pinned, in accent.key, on an account it names; ✕ out of the
// pool, cross and words in text.faint, on one it leaves out; else nothing.
func pinMark(a status.AccountNow) line {
	switch {
	case a.Pinned:
		return line{pinnedSpan}
	case a.OutOfPool:
		return line{outOfPoolSpan}
	}
	return nil
}

// window draws a's window w along row y, as usage --by account draws it
// narrow: its label a cell in, then its gauge, its bar the cells the columns
// give it, its words in their form, ending a cell short of the edge.
func (d linesDrawn) window(c *canvas, a status.AccountNow, w shownWindow, y int) {
	x := d.x() + 1
	c.line(x, y, windowLabel(w, windowLabelCells))
	d.wordsBlock().gauge(c, d.gauge(a, w.key), x+firstLabelCells, y, d.laid.columns.bar, d.f.Width-afterWords)
}

// barDrawn is the cells of the bars the lines draw at the layout's step: the
// columns' at steps 1 and 2; at step 3, what the lines' width leaves each of
// two side by side; and from step 4, oneRowBar.
func (d linesDrawn) barDrawn() int {
	switch d.laid.step {
	case 1, 2:
		return d.laid.columns.bar
	case 3:
		return sideBySideBar(d.width(), stepThreeLabels)
	}
	return oneRowBar
}

// sideBySide draws a's 5-hour and week along row y, as windowsBeside draws
// them, a cell in, the two bars sharing what the lines' width leaves them,
// ending at the last column.
func (d linesDrawn) sideBySide(c *canvas, a status.AccountNow, y int) {
	shown := d.windowsShown()
	gauges := make([]gauge, len(shown))
	for i, w := range shown {
		gauges[i] = d.gauge(a, w.key)
	}
	windowsBeside(c, d.f.Look, d.x()+1, y, d.barDrawn(), stepThreeLabels, shown, gauges)
}

// besideWindows are the windows drawn side by side of m, as step 3 and a
// lane draw them: the first two every model shares, its 5-hour and its
// week, as the policy m was judged by has them, in Claude's order, each
// named as the frame names it.
func (f Frame) besideWindows(m status.Moment) []shownWindow {
	keys := slices.SortedFunc(slices.Values(m.Policy().Shared), quota.CompareKeys)
	shown := usageWindows(m, keys, f.windowNamer(m))
	return shown[:min(len(shown), len(stepThreeLabels))]
}

// sideBySideCells are the cells windows side by side take besides their bars,
// their labels of the cells given: a cell in, each label, and after its bar,
// a cell and its percentage, groupGap cells between each.
func sideBySideCells(labels []int) int {
	cells := 1 + (len(labels)-1)*groupGap
	for _, l := range labels {
		cells += l + 1 + percentCells
	}
	return cells
}

// sideBySideBar is the cells of each bar of windows side by side across width
// cells, their labels of the cells given: what the rest leaves them, shared.
func sideBySideBar(width int, labels []int) int {
	return max((width-sideBySideCells(labels))/len(labels), 0)
}

// windowsBeside draws the windows given, each by its gauge, side by side
// along row y from x: each its label, cut a cell short of the cells labels
// gives it, in text.subtle; then its bar, bar cells long, a cell and its
// percentage, with no words, as meter draws them; the next groupGap cells
// on.
func windowsBeside(c *canvas, look Look, x, y, bar int, labels []int, windows []shownWindow, gauges []gauge) {
	for i, g := range gauges[:min(len(gauges), len(windows), len(labels))] {
		c.line(x, y, windowLabel(windows[i], labels[i]-1))
		x = meter(c, look, g, x+labels[i], y, bar) + groupGap
	}
}

// oneRow draws a's line on row y alone: its number, name and marks, cut to
// the name column; from it, its 5-hour bar, oneRowBar cells, a cell and its
// percentage; and afterBar cells on from its bar, how it stands, short where
// the lines are narrower than their widest, as stands has it.
func (d linesDrawn) oneRow(c *canvas, a status.AccountNow, n accountName, y int) {
	x := d.x() + d.column
	c.line(d.x(), y, n.line(d.column))
	for _, w := range d.windowsShown() {
		meter(c, d.f.Look, d.gauge(a, w.key), x, y, oneRowBar)
	}
	d.stands(c, a, x+oneRowBar+afterBar, y, d.width() < widestLines)
}

// windowLabel is the window w's name, short, cut to the cells given, in
// text.subtle.
func windowLabel(w shownWindow, cells int) line {
	return line{{prose.Cut(status.Short(w.name), cells), dimInk}}
}

// gauge is a's window with the given key as the lines draw it, as the
// Overview's USAGE draws it: faded where a is out of the pool, eased and
// cued as the frame has it.
func (d linesDrawn) gauge(a status.AccountNow, key string) gauge {
	return d.f.overviewGauge(accountGauge(a, key, d.m.Now), a.OutOfPool)
}

// meter draws g's bar in the look, bar cells long from x along row y,
// marked as its window's, then a cell and its percentage, with no words, and
// returns the column after it.
func meter(c *canvas, look Look, g gauge, x, y, bar int) int {
	g.bar.draw(c, look, x, y, bar)
	c.barred(x, y, bar, g.of)
	end := x + bar + 1 + percentCells
	c.right(end, y, g.percentage())
	return end
}

// wordsBlock is the block a line's windows are drawn by, in the frame's look,
// their words in full, or in usage's short form where the columns say, as
// the narrowing table gives them: never as a block of the lines' own width
// would pick them, as their words go short at 117 columns, where usage's
// own rule would take them short under 140. A block of no width draws
// usage's short words.
func (d linesDrawn) wordsBlock() usageBlock {
	b := usageBlock{look: d.f.Look, width: fullWords}
	if d.laid.columns.shortWords {
		b.width = 0
	}
	return b
}

// windowsShown are the windows a line draws at the layout's step: every
// window usage shows at steps 1 and 2; at step 3, its 5-hour and its week,
// as besideWindows has them; and from step 4, its 5-hour alone.
func (d linesDrawn) windowsShown() []shownWindow {
	switch d.laid.step {
	case 1, 2:
		return d.windows
	case 3:
		return d.beside
	}
	return d.windows[:min(len(d.windows), 1)]
}

// windowsDrawn are the keys of the windows a line draws at the layout's step,
// as windowsShown has them.
func (d linesDrawn) windowsDrawn() []string {
	var keys []string
	for _, w := range d.windowsShown() {
		keys = append(keys, w.key)
	}
	return keys
}
