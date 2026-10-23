package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// The cells of a line's parts as its steps draw them: a window's label, the
// first's a cell in, and at step 3, the second's beside it, room for
// "5-hour"; and at step 4, its week's bar's.
const (
	firstLabelCells  = windowLabelCells + 1
	secondLabelCells = 7
	oneRowBar        = 14
)

// oneRowStep is the compaction step from which a line takes one row.
const oneRowStep = 4

// leastStanding is the fewest cells a line's state words take: the widest of
// the short leads standingLeads has, ● under pressure, so a name narrows
// before any lead is cut, and the name column holds still as states change.
// A model's own lead, as Fable wk at its cap, takes its window's label and
// may be wider: it's cut where the names leave it no more.
const leastStanding = 16

// The parts of a line's row its name heads, in nameParts' order.
const (
	lineNamePart = iota
	lineMeterPart
	lineWordsPart
)

// The parts of a line's window row across its width, in lineParts' order.
const (
	lineBesidePart = iota
	lineBarsPastPart
	lineBarsPart
	lineSaidPart
)

// lineParts are the parts of a line's window row across its width, as flex
// lays them: its label, percentage and gaps, beside cells; its bars, those
// past leastWideBar and those up to it each narrowing in a turn of its own;
// and its words, which give way to usage's short form. A part that keeps its
// cells sets its least and natural alike. They aren't in the screen's order:
// the label, percentage and gaps together come before the bars they sit
// among.
func lineParts(beside int) []flex.Part {
	return []flex.Part{
		lineBesidePart:   {Least: beside, Natural: beside},
		lineBarsPastPart: {Natural: lineBar - leastWideBar, Narrows: barsToLeastWide},
		lineBarsPart:     {Least: leastBar, Natural: leastWideBar, Narrows: barsToLeast},
		lineSaidPart:     {Least: lineWords, Natural: lineWords, Leaves: wordsShortened, Kept: lineShortWords},
	}
}

// stepThreeLabels are the cells of the labels of step 3's two windows, side
// by side.
var stepThreeLabels = []int{firstLabelCells, secondLabelCells}

// widestLines are the columns the lines take at their widest, their bars
// and words whole: narrower, step 4's state words take the short form.
const widestLines = lineBeside + lineBar + lineWords

// outOfPoolSpan ends the name row of an account the global pin leaves out
// of the pool, in text.faint, cross and words.
var outOfPoolSpan = span{"✕ out of the pool", faintInk}

// linesDrawn are the Switchboard's lines of a moment as its form draws them,
// laid out as laid lays them: each account's name as the Overview names it,
// faint where it's out of the pool, in a name column laid as nameColumn lays
// it; and the windows usage shows of it.
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
	d := linesDrawn{f: f, m: m, laid: laid, names: overviewNames(m), windows: f.listedWindows(m), beside: f.besideWindows(m)}
	d.column = d.nameColumn()
	return d
}

// nameColumn is the cells of the lines' name column, laid across the lines'
// width as nameParts declares a row's parts.
func (d linesDrawn) nameColumn() int {
	s, _ := flex.Lay(d.width(), d.nameParts()...)
	return s[lineNamePart].Size
}

// nameParts are the parts of a line's row its name heads, as flex lays them:
// the name column, its names' labels a column of names as Runway's, at least
// labelLeast, at its natural the widest, as labelColumnCells counts them,
// with their numbers, their marks and nameGap; from oneRowStep, where the
// line takes one row, its week's bar, oneRowBar cells, and afterBar on to its
// words; and its words, at least leastStanding, which take the rest. So a
// name is whole wherever it fits beside them, and narrows toward its least
// before the words' lead is cut.
func (d linesDrawn) nameParts() []flex.Part {
	least, natural := 0, 0
	for _, n := range d.names {
		least = max(least, n.cells(labelLeast)+nameGap)
		natural = max(natural, n.cells(labelColumnCells(n.label))+nameGap)
	}
	meter := 0
	if d.laid.step >= oneRowStep {
		meter = oneRowBar + afterBar
	}
	return []flex.Part{
		lineNamePart:  {Least: least, Natural: natural},
		lineMeterPart: {Least: meter, Natural: meter},
		lineWordsPart: {Least: leastStanding, Grows: true},
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
// row between at step 1; at step 3, its name row, then its week and its
// 5-hour side by side; and from step 4, all on one row.
func (d linesDrawn) line(c *canvas, a status.AccountNow, n accountName, y int) {
	switch d.laid.step {
	case 1, 2:
		d.nameRow(c, a, n, y, d.laid.columns.shortWords)
		d.stacked(c, a, y+1)
	case 3:
		d.nameRow(c, a, n, y, d.laid.columns.shortWords)
		d.sideBySide(c, a, y+1)
	default:
		d.oneRow(c, a, n, y)
	}
}

// nameRow draws a's name row along row y: its number, name and marks, then
// from the name column, its state in ROUTING's words, or short, their short
// form, ending where the pin's mark needs the room, as stands has them.
func (d linesDrawn) nameRow(c *canvas, a status.AccountNow, n accountName, y int, short bool) {
	c.line(d.x(), y, n.line(d.column))
	d.stands(c, a, d.x()+d.column, y, short)
}

// stacked draws a's windows from row y, as steps 1 and 2 draw them, a row a
// window, as window draws it, a blank row between at step 1.
func (d linesDrawn) stacked(c *canvas, a status.AccountNow, y int) {
	apart := 2
	if d.laid.step == 2 {
		apart = 1
	}
	for j, w := range d.windowsShown() {
		d.window(c, a, w, y+j*apart)
	}
}

// stands draws how a stands from x along row y, in ROUTING's words, or
// short, their short form, cut by whole clauses to end wordsGap cells short
// of the pin's mark, which ends the row at the last column: ● pinned on an
// account the global pin names, ✕ out of the pool, faint, on one it leaves
// out, as pinMark has it, where pinFits has it room, the words then not
// saying the pin again, else not drawn; picked out while its state is newly
// changed, as the Overview's ROUTING picks it out.
func (d linesDrawn) stands(c *canvas, a status.AccountNow, x, y int, short bool) {
	end := d.f.Width
	mark, said := pinShown(a, x, end, short)
	if mark != nil {
		end = c.right(end, y, mark) - wordsGap
	}
	drawn := c.line(x, y, standingWords(said, short, end-x))
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

// pinShown is a's pin mark, as pinMark has it, where pinFits has it room
// ending a row at column end, else nil; and how a stands in words beside it,
// as saidBeside has it, so the pin is said once, by the mark where it's drawn.
func pinShown(a status.AccountNow, x, end int, short bool) (line, status.Routing) {
	mark := pinMark(a)
	if mark == nil || !pinFits(a, mark, x, end, short) {
		return nil, a.Routing
	}
	return mark, saidBeside(a.Routing, a.Pinned)
}

// pinFits reports whether a's pin mark, ending a row at column end, leaves
// the words from x of how a stands, or short, their short form, their first
// clause whole, wordsGap cells short of it: the words say how a stands
// first, and the mark gives way to them.
func pinFits(a status.AccountNow, mark line, x, end int, short bool) bool {
	return end-mark.width()-wordsGap-x >= standingLeads(a.Routing, short).width()
}

// window draws a's window w along row y, as usage --by account draws it
// narrow: its label a cell in, then its gauge, its bar and its words the
// cells the columns give them, its words in their form.
func (d linesDrawn) window(c *canvas, a status.AccountNow, w shownWindow, y int) {
	x := d.x() + 1
	bar := x + firstLabelCells
	c.line(x, y, windowLabel(w, windowLabelCells))
	d.wordsBlock().gauge(c, d.gauge(a, w.key), bar, y, d.laid.columns.bar, bar+d.laid.columns.bar+afterBar+d.laid.columns.words)
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

// sideBySide draws a's week and 5-hour along row y, as windowsBeside draws
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
// lane draw them: the first two every model shares, its week and its
// 5-hour, as m's SharedWindows has them, each named as the frame names it.
func (f Frame) besideWindows(m status.Moment) []shownWindow {
	shown := usageWindows(m.SharedWindows(), f.windowNamer(m))
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
// the name column; from it, its week's bar, oneRowBar cells, a cell and its
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

// drawKey draws the lines' key, the Overview's, on its two rows from row y,
// from column 0, each fit to the frame's width: even pace's on the first,
// and the marks the accounts carry on the second, in the first of their
// forms that fits the width, as keyForms and firstFit have them; never the
// cap's, as no bar of the Switchboard draws one.
func (d linesDrawn) drawKey(c *canvas, y int) {
	b := blockOf(d.m, d.f.Width, d.f.Look, ByAccount, d.windows, d.names).uncapped()
	b.marks = keyMarks(d.names)
	pace, forms := b.keyParts(d.barDrawn())
	for i, l := range []line{pace, firstFit(d.f.Width, keyForms(forms)...)} {
		c.line(0, y+i, l.fit(d.f.Width))
	}
}

// windowLabel is the window w's name, short, cut to the cells given, in
// text.subtle.
func windowLabel(w shownWindow, cells int) line {
	return line{{prose.Cut(status.Short(w.name), cells), dimInk}}
}

// gauge is a's window with the given key as the lines draw it, as the
// Overview's USAGE draws it, uncapped: faded where a is out of the pool,
// eased and cued as the frame has it.
func (d linesDrawn) gauge(a status.AccountNow, key string) gauge {
	return d.f.overviewGauge(accountGauge(a, key, d.m.Now).uncapped(), a.OutOfPool)
}

// uncapped is the block as the Switchboard draws it, as uncapped has each of
// its gauges: its key names no cap.
func (b usageBlock) uncapped() usageBlock {
	rows := make([]usageRow, len(b.rows))
	for i, r := range b.rows {
		r.cap, r.gauges = "", slices.Clone(r.gauges)
		for j, g := range r.gauges {
			r.gauges[j] = g.uncapped()
		}
		rows[i] = r
	}
	b.rows = rows
	return b
}

// uncapped is the gauge as the Switchboard draws it: its bar with no cap's
// mark, at any width, as its line's state words say the cap.
func (g gauge) uncapped() gauge {
	g.bar.capAt = 1
	return g
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
// the board's turns take them: never as a block of the lines' own width
// would pick them, as their words go short in their own turn, not by usage's
// rule of width. A block of no width draws usage's short words.
func (d linesDrawn) wordsBlock() usageBlock {
	b := usageBlock{look: d.f.Look, width: fullWords}
	if d.laid.columns.shortWords {
		b.width = 0
	}
	return b
}

// windowsShown are the windows a line draws at the layout's step: every
// window usage shows at steps 1 and 2; at step 3, its week and its 5-hour,
// as besideWindows has them; and from step 4, the first usage shows alone,
// its week.
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
