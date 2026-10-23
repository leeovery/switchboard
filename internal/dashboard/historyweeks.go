package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Weeks' cells across the frame: a week's, its first day's widest and its
// peak's, weeksApart after the one before, a table's peak right-aligned in
// peakCells; and the verdicts', verdictGap after the last week.
const (
	weekColumnCells = 6
	weeksApart      = 2
	peakCells       = 5
	verdictGap      = 1
)

// Weeks' column shape: a block an account, blockRows apart, each week a
// column columnCells wide and columnRows tall, its peak under it centred in
// the week's cells; each row of a column mixed columnMix from the canvas
// toward its colour, this week's thisWeekMix.
const (
	blockRows   = 6
	columnCells = 5
	columnRows  = 3
	columnMix   = 0.75
	thisWeekMix = 0.45
)

// everyAccount names the row, or the block, of every account's peaks.
const everyAccount = "all accounts"

// The rows of Weeks' table, from the view's top: the weeks' first days',
// every account's, and the first account's.
const (
	weeksHeadRow  = 4
	weeksAllRow   = 6
	weeksFirstRow = 8
)

// historyWeeks draws Weeks from row top: once a whole week is recorded, the
// capacity verdict, then the table or the columns, as v shapes it, their
// accounts scrolled as the frame has them, and the legend on the view's last
// row; in its narrow form, the verdict and the table's latest weeks that fit,
// what's out of view on the view's last row. Where it scrolls whole, every
// account, drawn whole and scrolled by rows over the legend. Till a whole
// week is recorded, it says there aren't enough weeks yet; and before its
// page is built, it's blank.
func (f Frame) historyWeeks(c *canvas, top int) {
	p := f.History.Weeks
	switch {
	case p == nil:
		return
	case p.Capacity.Weeks == 0:
		c.line(0, top, line{{views.NotEnoughWeeks, dimInk}}.fit(f.edge()))
		return
	}
	w := f.weeksDrawn(*p)
	foot := top + f.viewRows() - 1
	if f.weeksWhole(w) {
		body := newCanvas(f.Width, w.rows())
		w.draw(body, f.Look, 0, w.page.Accounts)
		f.pasteWhole(c, body, top, f.weeksPaged(w))
	} else {
		shown, above, below := f.weeksShown(w)
		w.draw(c, f.Look, top, shown)
		w.accountsOut(c, foot-w.footRows(), above, below)
	}
	if !w.narrow {
		w.put(c, 0, foot, w.legend()...)
	}
}

// draw draws Weeks in its shape from row top, the accounts given shown: in
// its narrow form, the verdict and the table; else as v shapes it, the table
// or the columns.
func (w weeksDrawn) draw(c *canvas, look Look, top int, shown []views.WeeksRow) {
	switch {
	case w.narrow:
		w.narrowly(c, top, shown)
	case w.columns:
		w.inColumns(c, look, top, shown)
	default:
		w.asATable(c, top, shown)
	}
}

// weeksWhole reports whether Weeks, drawn as given, scrolls whole: on a
// frame shorter than History's, or where it would show none of its
// accounts.
func (f Frame) weeksWhole(w weeksDrawn) bool {
	return f.shortOfHistory() || f.weeksPage(w) == 0 && len(w.page.Accounts) > 0
}

// footRows is how many rows Weeks keeps at the view's foot: the legend's,
// but in its narrow form, which has none.
func (w weeksDrawn) footRows() int {
	if w.narrow {
		return 0
	}
	return 1
}

// rows is how many rows Weeks takes drawn whole, every account shown, from
// its top: in its narrow form, the verdict's, then the table's; in columns,
// a block every account, the weeks' first days under the last; and as a
// table, to its last account's row.
func (w weeksDrawn) rows() int {
	n := len(w.page.Accounts)
	switch {
	case w.narrow:
		return len(w.narrowVerdict()) + 1 + weeksFirstRow - weeksHeadRow + n
	case w.columns:
		return blockRows*n + columnRows + 2
	}
	return weeksFirstRow + n
}

// weeksPaged is how Weeks, drawn whole, as given, pages in the frame's view,
// over its foot, as heads holds its heading.
func (f Frame) weeksPaged(w weeksDrawn) rowsPaged {
	return f.wholePaged(w.rows(), w.footRows(), w.heads())
}

// heads are Weeks' headings, drawn whole, each over the first row under it:
// the table's weeks' first days, over every account's peaks, or with one
// account, its own; none in columns, whose blocks are named on their first
// rows, nor with no account under them.
func (w weeksDrawn) heads() []headed {
	if w.columns || len(w.page.Accounts) == 0 {
		return nil
	}
	at := weeksHeadRow
	if w.narrow {
		at = len(w.narrowVerdict()) + 1
	}
	first := weeksFirstRow
	if w.several() {
		first = weeksAllRow
	}
	return []headed{{at: at, first: at + first - weeksHeadRow}}
}

// weeksDrawn is Weeks' page as the frame draws it: in the shape v gives it,
// where its wide form fits; else in the other, where that does, as v has no
// shape to change to there; else in its narrow form.
func (f Frame) weeksDrawn(p views.WeeksPage) weeksDrawn {
	chosen := f.History.WeeksShape == WeeksColumns
	for _, columns := range []bool{chosen, !chosen} {
		if f.weeksWideFits(columns) {
			return f.weeksIn(p, false, columns)
		}
	}
	return f.weeksIn(p, true, false)
}

// weeksWideFits reports whether Weeks' wide form, in columns or as a table,
// fits the frame, as flex lays its parts short of the edge: its label
// column, a week and the verdict at their leasts, and its legend whole on one
// row, none of them hanging on the data, so neither does the form.
func (f Frame) weeksWideFits(columns bool) bool {
	verdict := verdictPart(columns)
	_, fits := flex.Lay(f.edge(), weeksParts(namesColumn(nameGap), 1, &verdict)...)
	return fits && partsWidth(weeksDrawn{columns: columns}.legend()) <= f.edge()
}

// weeksIn is Weeks' page in the frame, in its narrow form, or wide, in
// columns or as a table, its row's parts laid out as weeksParts declares
// them.
func (f Frame) weeksIn(p views.WeeksPage, narrow, columns bool) weeksDrawn {
	w := weeksDrawn{page: p, narrow: narrow, columns: columns, edge: f.edge()}
	var verdict *flex.Part
	if !narrow {
		verdict = new(verdictPart(columns))
	}
	spans, fits := flex.Lay(w.edge, weeksParts(w.labelsColumn(), len(p.Weeks), verdict)...)
	w.laid = weeksLaid{labels: spans[0], weeks: spansShown(spans[1:1+len(p.Weeks)], fits)}
	if verdict != nil {
		w.laid.verdict = spans[len(spans)-1]
	}
	return w
}

// weeksParts are a row of Weeks' parts across the frame: the label column
// given; the weeks given, the oldest first, each but the last with
// weeksApart after it, which give way in that order, the last never; and in
// the wide form, the verdict given, verdictGap after the last week. The
// verdict narrows once every week but the last has given way, and the label
// column once the verdict has, so a week leaves before a verdict is cut, and
// a verdict is cut before a name.
func weeksParts(labels flex.Part, weeks int, verdict *flex.Part) []flex.Part {
	labels.Narrows = weeks + 1
	parts := []flex.Part{labels}
	for i := range weeks {
		week := flex.Part{Least: weekColumnCells, Natural: weekColumnCells}
		if i < weeks-1 {
			week = flex.Part{Least: weekColumnCells + weeksApart, Natural: weekColumnCells + weeksApart, Leaves: i + 1}
		}
		parts = append(parts, week)
	}
	if verdict != nil {
		v := *verdict
		v.Narrows = max(weeks, 1)
		parts = append(parts, v)
	}
	return parts
}

// verdictPart is the column of an account's verdict, in columns or as a
// table: its natural the widest the verdict is worded, as widestVerdict has
// it, its least the widest verdict alone, so a verdict is never cut to less
// than its word.
func verdictPart(columns bool) flex.Part {
	least := 0
	for _, verdict := range accountVerdicts {
		least = max(least, ansi.StringWidth(verdict))
	}
	return flex.Part{Least: least, Natural: widestVerdict(columns), Gap: verdictGap}
}

// accountVerdicts are the verdicts Weeks gives an account.
var accountVerdicts = []string{views.CouldGo, views.BarelyUsed, views.HitsItsLimit, views.PlentyToSpare, views.EndsWithRoom}

// widestVerdict is the widest an account's verdict is worded, in columns or
// as a table: of every verdict, with few accounts and with many, its counts
// of views.VerdictWeeks, its shares 100%, the widest they're bounded by. What
// else is beside the weeks, every account's row's verdict, and in columns the
// capacity verdict's rows, isn't counted: it's held to the edge.
func widestVerdict(columns bool) int {
	widest, whole, weeks := 0, 1.0, views.VerdictWeeks
	for _, drop := range []string{"", "any"} {
		w := weeksDrawn{page: views.WeeksPage{Capacity: views.Capacity{Weeks: weeks, Drop: drop}}, columns: columns}
		for _, verdict := range accountVerdicts {
			row := views.WeeksRow{Verdict: verdict, Weeks: weeks, Limits: weeks, Used: &whole, LeftAtReset: &whole, WithoutIt: &weeks}
			widest = max(widest, w.verdictWidth(row))
		}
	}
	return widest
}

// verdictWidth is how many cells the row's verdict takes beside its weeks:
// in the table, its one row; in columns, the widest of its rows.
func (w weeksDrawn) verdictWidth(row views.WeeksRow) int {
	if !w.columns {
		return partsWidth(w.tableVerdict(row))
	}
	widest := 0
	for _, l := range w.accountAside(row) {
		widest = max(widest, partsWidth(l))
	}
	return widest
}

// labelsColumn is the label column, nameGap after it, as namesColumn lays
// it out of the names it holds: every account's row's, or any account's,
// its mark beside it, not only those shown, so the column holds still as
// Weeks scrolls.
func (w weeksDrawn) labelsColumn() flex.Part {
	names := []columnName{{label: ansi.StringWidth(everyAccount)}}
	for _, row := range w.page.Accounts {
		names = append(names, historyNamed(w.named(row.Account), row.Primary))
	}
	return namesColumn(nameGap, names...)
}

// weeksScrolling is how far Weeks scrolls, as the frame draws it, scrolled
// offset down: by rows, where it scrolls whole; else in accounts, zero where
// they fit, as they always do before its page is built or a whole week
// recorded.
func (f Frame) weeksScrolling(offset int) Scrolling {
	p := f.History.Weeks
	if p == nil || p.Capacity.Weeks == 0 || f.cramped() {
		return Scrolling{}
	}
	w := f.weeksDrawn(*p)
	if f.weeksWhole(w) {
		return f.weeksPaged(w).scrolling(offset)
	}
	page := f.weeksPage(w)
	return Scrolling{Most: max(len(p.Accounts)-page, 0), Page: page}
}

// weeksScrolls is what of Weeks scrolls, from row y, rows tall: where it
// scrolls whole, the rows over its foot; else the rows of the accounts it
// shows, from the first's; none before its page is built or a whole week
// recorded.
func (f Frame) weeksScrolls() (y, rows int) {
	p := f.History.Weeks
	if p == nil || p.Capacity.Weeks == 0 {
		return 0, 0
	}
	w := f.weeksDrawn(*p)
	if f.weeksWhole(w) {
		return f.top(), f.weeksPaged(w).page()
	}
	if rows = f.weeksPage(w); w.columns {
		rows *= blockRows
	}
	return f.weeksFirst(w), rows
}

// weeksFirst is the row of the first account Weeks shows: under the table's
// every account's, after the verdict, or in its narrow form, after the
// verdict's rows; or in columns, the first block after every account's.
func (f Frame) weeksFirst(w weeksDrawn) int {
	top := f.top()
	switch {
	case w.narrow:
		return top + len(w.narrowVerdict()) + 1 + weeksFirstRow - weeksHeadRow
	case w.columns:
		return top + blockRows
	}
	return top + weeksFirstRow
}

// weeksPage is how many accounts Weeks shows at once, from the first's row:
// in the table, those that fit to the one over the scroll line, over the
// legend; in columns, the blocks that fit, the last one's dates under it;
// and in its narrow form, those that fit to the scroll line, the view's last
// row. It's none where none fits.
func (f Frame) weeksPage(w weeksDrawn) int {
	first, foot := f.weeksFirst(w), f.top()+f.viewRows()-1
	switch {
	case w.narrow:
		return max(foot-first, 0)
	case w.columns:
		return max((foot-first)/blockRows, 0)
	}
	return max(foot-1-first, 0)
}

// weeksShown is the accounts of the page Weeks shows, scrolled as the frame
// has it, page of them at once; and how many more there are above them, and
// below.
func (f Frame) weeksShown(w weeksDrawn) (shown []views.WeeksRow, above, below int) {
	page := f.weeksPage(w)
	at := min(max(f.Scroll, 0), max(len(w.page.Accounts)-page, 0))
	end := min(at+page, len(w.page.Accounts))
	return w.page.Accounts[at:end], at, len(w.page.Accounts) - end
}

// accountsOut draws, on row y, how many more accounts there are above those
// shown, and below, where there are any, as Days says it, ending before the
// edge.
func (w weeksDrawn) accountsOut(c *canvas, y, above, below int) {
	if above+below > 0 {
		c.line(0, y, outOfView(above, below, w.edge, historyAccountsOut...))
	}
}

// asATable draws Weeks' table from row top: the verdict, then the weeks'
// first days, every account's peaks, and each shown account's, its verdict
// at the right.
func (w weeksDrawn) asATable(c *canvas, top int, shown []views.WeeksRow) {
	w.put(c, 0, top, w.headline()...)
	w.put(c, 0, top+1, joinedParts(span{"   ·   ", dimInk}, slices.Concat([][]part{w.asNow()}, w.replays())...)...)
	w.put(c, 0, top+2, w.thirdRow()...)
	w.table(c, top+weeksHeadRow, shown)
}

// table draws the table's weeks shown from its heads' row y: their first
// days, every account's peaks, and the peaks of the accounts shown; and in
// the wide form, the verdicts after them.
func (w weeksDrawn) table(c *canvas, y int, shown []views.WeeksRow) {
	w.weekHeads(c, y)
	all, first, aside := y+weeksAllRow-weeksHeadRow, y+weeksFirstRow-weeksHeadRow, w.laid.verdict.At
	if !w.narrow {
		w.put(c, aside, y, slices.Concat([]part{wordsIn("over the last ", faintInk)}, countedParts(w.page.Capacity.Weeks, "week", faintInk))...)
	}
	if w.several() {
		c.line(0, all, w.everyAccount())
		w.tablePeaks(c, all, w.page.All)
	}
	if w.several() && !w.narrow {
		w.put(c, aside, all, w.allShort()...)
	}
	for i, row := range shown {
		c.line(0, first+i, w.name(row))
		w.tablePeaks(c, first+i, row)
		if !w.narrow {
			w.put(c, aside, first+i, w.tableVerdict(row)...)
		}
	}
}

// inColumns draws Weeks' columns from row top: a block for every account,
// then one each shown account, each week's peak a column, with the verdict,
// or the account's, beside it; and the weeks' first days under the last.
func (w weeksDrawn) inColumns(c *canvas, look Look, top int, shown []views.WeeksRow) {
	aside := w.laid.verdict.At
	if w.several() {
		c.line(0, top, w.everyAccount())
		w.block(c, look, top, w.page.All)
	}
	for i, l := range w.allAside() {
		w.put(c, aside, top+i, l...)
	}
	y := top
	for _, row := range shown {
		y += blockRows
		c.line(0, y, w.name(row))
		w.block(c, look, y, row)
		for i, l := range w.accountAside(row) {
			w.put(c, aside, y+i, l...)
		}
	}
	first := w.firstShown()
	for i, week := range w.laid.weeks {
		w.put(c, week.At, y+columnRows+1, figurePart(w.head(first+i), Number{}))
	}
}

// narrowly draws Weeks in the narrow form from row top: the verdict, a part a row,
// wrapped to the width; then the table's weeks that fit, with no verdicts
// beside it, the accounts given shown.
func (w weeksDrawn) narrowly(c *canvas, top int, shown []views.WeeksRow) {
	verdict := w.narrowVerdict()
	for i, l := range verdict {
		c.line(0, top+i, l)
	}
	w.table(c, top+len(verdict)+1, shown)
}

// weeksDrawn is Weeks' page as it's drawn: in its narrow form, in columns, or
// as a table, its row's parts laid out, its lines cut short before the
// column edge.
type weeksDrawn struct {
	page            views.WeeksPage
	narrow, columns bool
	edge            int
	laid            weeksLaid
}

// weeksLaid is where a row of Weeks lays its parts: its label column; the
// weeks shown, the latest of the page's that fit, the oldest first; and in
// the wide form, the verdicts' column.
type weeksLaid struct {
	labels  flex.Span
	weeks   []flex.Span
	verdict flex.Span
}

// firstShown is the index of the page's first week shown.
func (w weeksDrawn) firstShown() int {
	return len(w.page.Weeks) - len(w.laid.weeks)
}

// nameCells are the cells an account's name is drawn in: the label column's
// less nameGap; or where no week shows, the row's, to the edge.
func (w weeksDrawn) nameCells() int {
	if len(w.laid.weeks) == 0 {
		return max(w.edge, 0)
	}
	return min(w.laid.labels.Size-nameGap, max(w.edge, 0))
}

// put draws the parts from x along row y, cut as fitParts cuts them to end
// before the edge.
func (w weeksDrawn) put(c *canvas, x, y int, parts ...part) {
	c.line(x, y, fitParts(w.edge-x, parts...))
}

// several reports whether there's more than one account: with one, every
// account's row goes, as it would only repeat that account's.
func (w weeksDrawn) several() bool {
	return len(w.page.Accounts) > 1
}

// many reports whether Weeks takes its form for many accounts: where the
// verdict names one to drop.
func (w weeksDrawn) many() bool {
	return w.page.Capacity.Drop != ""
}

// headline is the verdict's headline, as views.Headline says it, bold, as
// "three is about right", its count a figure: in accent.attention where there
// are too few, else in state.positive.
func (w weeksDrawn) headline() []part {
	k := boldPositiveInk
	if v := w.page.Capacity.Verdict; v == views.OneTooFew || v == views.TooFew {
		k = alertInk
	}
	return saidParts(views.Headline(len(w.page.Accounts), w.page.Capacity.Verdict), k)
}

// asNow is how many of the last whole weeks were short as the accounts
// stand, as views.ShortOfTheLastWeeks says it, and who was short each time,
// where the same were: "as now: short on 3 of the last 7 weeks, each time
// work alone".
func (w weeksDrawn) asNow() []part {
	c := w.page.Capacity
	k := w.countInk(c.Short)
	l := slices.Concat([]part{wordsIn(views.ReplayAsNow, dimInk)}, saidParts(views.ShortOfTheLastWeeks(c.Short, c.Weeks), k))
	names := make([]string, len(w.page.ShortEach))
	for i, id := range w.page.ShortEach {
		names[i] = w.named(id)
	}
	switch len(names) {
	case 0:
	case 1:
		l = append(l, wordsIn(", each time "+names[0]+" alone", mutedInk))
	default:
		l = append(l, wordsIn(", "+prose.List(names)+" each time", mutedInk))
	}
	return l
}

// replays are what the replays with one account fewer and one more find,
// "with one fewer: short on 6 of 7" and "with one more: short on none":
// where one can be dropped, one fewer's saying where its weeks are the
// same, and with two fewer in one more's place; with one account, whether a
// second would have cleared the weeks short.
func (w weeksDrawn) replays() [][]part {
	c := w.page.Capacity
	switch {
	case c.WithOneFewer == nil && c.Short == 0:
		return nil
	case c.WithOneFewer == nil && c.WithOneMore == 0:
		return [][]part{{wordsIn("a second would have cleared them", mutedInk)}}
	case c.WithOneFewer == nil:
		return [][]part{w.replay(views.ReplayASecond, c.WithOneMore, "")}
	}
	same := ""
	if w.page.SameWeeks {
		same = ", the same weeks"
	}
	fewer := w.replay(views.ReplayOneFewer, *c.WithOneFewer, same)
	if c.WithTwoFewer != nil {
		return [][]part{fewer, w.replay(views.ReplayTwoFewer, *c.WithTwoFewer, "")}
	}
	return [][]part{fewer, w.replay(views.ReplayOneMore, c.WithOneMore, "")}
}

// replay is a replay's label and the weeks it finds short, n of those
// judged, and what follows them.
func (w weeksDrawn) replay(label string, n int, after string) []part {
	k := w.countInk(n)
	return slices.Concat([]part{wordsIn(label, dimInk)}, w.shortOn(n, k), []part{wordsIn(after, k)})
}

// countInk is the ink of a count of weeks short, n of them: text.muted, or
// bold accent.attention for the worst of the counts the verdict shows, where
// any is short.
func (w weeksDrawn) countInk(n int) ink {
	if n > 0 && n == slices.Max(w.counts()) {
		return alertInk
	}
	return mutedInk
}

// counts are the counts of weeks short the verdict shows: as now, and as
// each replay it shows finds.
func (w weeksDrawn) counts() []int {
	c := w.page.Capacity
	counts := []int{c.Short}
	switch {
	case c.WithOneFewer == nil:
		return append(counts, c.WithOneMore)
	case c.WithTwoFewer != nil:
		return append(counts, *c.WithOneFewer, *c.WithTwoFewer)
	}
	return append(counts, *c.WithOneFewer, c.WithOneMore)
}

// estimate says the verdict is an estimate, as few accounts' form words it,
// or many's.
func (w weeksDrawn) estimate() []part {
	if w.many() {
		return []part{wordsIn(views.EstimateOfReplays, faintInk)}
	}
	return []part{wordsIn(views.EstimateOfOneFewerOrMore, faintInk)}
}

// dropped names the account to drop and says why, in text.muted, its price
// and its use figures: "spare is the one to drop: $100/mo, 5% used on
// average, and nothing ran short without it".
func (w weeksDrawn) dropped() []part {
	var facts [][]part
	if p := w.page.DropPrice; p != nil {
		facts = append(facts, []part{figureIn(views.PerMonth(*p), mutedInk)})
	}
	if used := w.row(w.page.Capacity.Drop).Used; used != nil {
		facts = append(facts, []part{figureIn(prose.Percent(*used), mutedInk), wordsIn(" used on average", mutedInk)})
	}
	said := joinedParts(span{", ", mutedInk}, facts...)
	switch {
	case w.page.NoneShortWithout && said != nil:
		said = append(said, wordsIn(", and nothing ran short without it", mutedInk))
	case w.page.NoneShortWithout:
		said = []part{wordsIn("nothing ran short without it", mutedInk)}
	}
	named := []part{wordsIn(views.ToDrop(w.named(w.page.Capacity.Drop)), mutedInk)}
	if said == nil {
		return named
	}
	return slices.Concat(named, []part{wordsIn(": ", mutedInk)}, said)
}

// thirdRow is the verdict's third row: with many accounts, the one to drop,
// then three spaces and the estimate; with few, the estimate alone.
func (w weeksDrawn) thirdRow() []part {
	if w.many() {
		return slices.Concat(w.dropped(), []part{wordsIn("   ", ink{})}, w.estimate())
	}
	return w.estimate()
}

// narrowVerdict is the verdict as the narrow form draws it: a part a row, each
// wrapped to the edge, its figures never broken.
func (w weeksDrawn) narrowVerdict() []line {
	parts := slices.Concat([][]part{w.headline(), w.asNow()}, w.replays())
	if w.many() {
		parts = append(parts, w.dropped())
	}
	var rows []line
	for _, said := range append(parts, w.estimate()) {
		rows = append(rows, wrapParts(w.edge, said...)...)
	}
	return rows
}

// allAside is what's beside every account's block: the headline, as now,
// the replays three spaces apart, and the estimate.
func (w weeksDrawn) allAside() [][]part {
	return [][]part{w.headline(), w.asNow(), joinedParts(span{"   ", ink{}}, w.replays()...), w.estimate()}
}

// allShort is every account's verdict in the table: how many of the weeks
// judged were short, as views.ShortOfWeeks says it, and with many accounts,
// how many there are.
func (w weeksDrawn) allShort() []part {
	c := w.page.Capacity
	l := saidParts(views.ShortOfWeeks(c.Short, c.Weeks), strongInk)
	if w.many() {
		l = append(l, wordsIn("   with ", dimInk), figureIn(prose.Number(len(w.page.Accounts)), dimInk), wordsIn(" accounts", dimInk))
	}
	return l
}

// verdictInk is the ink of an account's verdict, bold: hits its limit in
// state.destructive, plenty to spare in state.positive, could go and barely
// used in accent.attention, and ends with room in text.secondary; and of
// every account's, room to spare together in state.positive and little to
// spare together in accent.attention.
func verdictInk(verdict string) ink {
	switch verdict {
	case views.HitsItsLimit:
		return exhaustedInk
	case views.PlentyToSpare, views.RoomToSpare:
		return boldPositiveInk
	case views.CouldGo, views.BarelyUsed, views.LittleToSpare:
		return alertInk
	}
	return strongInk
}

// spared is the verdict of an account that could go or is barely used,
// alone in its cell, its share a figure: "could go: 5% used, no limit ever",
// or "barely used: 80% left at a reset"; reporting false of any other.
func (w weeksDrawn) spared(row views.WeeksRow) ([]part, bool) {
	var share *float64
	var says string
	switch {
	case row.Verdict == views.CouldGo && row.Used != nil:
		share, says = row.Used, " used, no limit ever"
	case row.Verdict == views.BarelyUsed && row.LeftAtReset != nil:
		share, says = row.LeftAtReset, " left at a reset"
	case row.Verdict != views.CouldGo && row.Verdict != views.BarelyUsed:
		return nil, false
	}
	spared := []part{wordsIn(row.Verdict+": ", verdictInk(row.Verdict))}
	if share != nil {
		spared = append(spared, figureIn(prose.Percent(*share), mutedInk), wordsIn(says, mutedInk))
	}
	return spared, true
}

// tableVerdict is an account's verdict in the table, bold in its colour,
// then what it left at a reset, and with few accounts, how many weeks the
// rest would have been short without it, each count a figure.
func (w weeksDrawn) tableVerdict(row views.WeeksRow) []part {
	if l, ok := w.spared(row); ok || row.Verdict == "" {
		return l
	}
	l := []part{wordsIn(row.Verdict, verdictInk(row.Verdict))}
	switch left := row.LeftAtReset; {
	case left != nil && w.many():
		l = append(l, wordsIn("   ", dimInk), figureIn(prose.Percent(*left), dimInk), wordsIn(" left at a reset", dimInk))
	case left != nil:
		l = append(l, wordsIn("  ", dimInk), figureIn(prose.Percent(*left), dimInk), wordsIn(" left", dimInk))
	}
	if row.WithoutIt != nil && !w.many() {
		l = slices.Concat(l, []part{wordsIn("  without it: short ", dimInk)}, saidParts(views.OutOf(*row.WithoutIt, w.page.Capacity.Weeks), dimInk))
	}
	return l
}

// accountAside is what's beside an account's block: its verdict; how many
// of its weeks it hit its limit in and what they left at a reset; and how
// many weeks the rest would have been short without it, each count a
// figure.
func (w weeksDrawn) accountAside(row views.WeeksRow) [][]part {
	if row.Verdict == "" {
		return nil
	}
	verdict, ok := w.spared(row)
	if !ok {
		verdict = []part{wordsIn(row.Verdict, verdictInk(row.Verdict))}
	}
	record := saidParts(views.LimitHit(row.Limits, row.Weeks), mutedInk)
	if row.LeftAtReset != nil {
		record = slices.Concat(record, []part{wordsIn(" · ", mutedInk)}, saidParts(views.LeftOnAverage(*row.LeftAtReset), mutedInk))
	}
	aside := [][]part{verdict, record}
	if row.WithoutIt != nil {
		aside = append(aside, slices.Concat([]part{wordsIn(views.WithoutTheRest, dimInk)}, w.shortOn(*row.WithoutIt, mutedInk)))
	}
	return aside
}

// shortOn says how many of the weeks judged were short, n of them, as
// views.ShortOutOf says it, in the ink given: "short on 6 of 7", its counts
// figures, or "short on none".
func (w weeksDrawn) shortOn(n int, k ink) []part {
	return saidParts(views.ShortOutOf(n, w.page.Capacity.Weeks), k)
}

// legend is the key to how the peaks are drawn, each mark as it's drawn, a
// figure, and the words in text.faint; the columns' led by what short
// means.
func (w weeksDrawn) legend() []part {
	var l []part
	if w.columns {
		l = []part{wordsIn("short: a week some account hit its limit", faintInk), wordsIn("   ·   ", faintInk)}
	}
	return append(l,
		wordsIn("each week at its peak", faintInk), wordsIn("   ·   ", faintInk),
		figureIn(views.Peak(1, false), exhaustedInk), wordsIn(" it hit its limit", faintInk), wordsIn("   ·   ", faintInk),
		figureIn(views.Peak(0.34, true), w.thisWeekInk()), wordsIn(" this week, so far", faintInk),
	)
}

// thisWeekInk is the ink of this week's peak so far: text.secondary, bold in
// the table.
func (w weeksDrawn) thisWeekInk() ink {
	if w.columns {
		return secondaryInk
	}
	return strongInk
}

// row is the row of the account with the given id, the zero row where
// there's none.
func (w weeksDrawn) row(id string) views.WeeksRow {
	i := slices.IndexFunc(w.page.Accounts, func(r views.WeeksRow) bool { return r.Account == id })
	if i < 0 {
		return views.WeeksRow{}
	}
	return w.page.Accounts[i]
}

// named is what Weeks calls the account with the given id, as labelled
// names it.
func (w weeksDrawn) named(id string) string {
	return labelled(w.row(id).Label, id)
}

// name is an account's name and mark at the start of its row, cut to the
// cells names are drawn in.
func (w weeksDrawn) name(row views.WeeksRow) line {
	return historyAccount(w.named(row.Account), row.Primary, w.nameCells())
}

// everyAccount names every account's row, or block, bold text.secondary,
// cut to the cells names are drawn in.
func (w weeksDrawn) everyAccount() line {
	return line{{everyAccount, strongInk}}.fit(w.nameCells())
}

// weekHeads draws the first days of the weeks shown along row y, each
// right-aligned in its week's cells.
func (w weeksDrawn) weekHeads(c *canvas, y int) {
	first := w.firstShown()
	for i, week := range w.laid.weeks {
		c.rightFit(week.At+weekColumnCells, w.edge, y, figurePart(w.head(first+i), Number{}))
	}
}

// head is the page's week with index i, headed by its first day.
func (w weeksDrawn) head(i int) line {
	return weekHead(w.page.Weeks[i], i == len(w.page.Weeks)-1)
}

// tablePeaks draws the row's peaks in the weeks shown along row y, each
// right-aligned in peakCells.
func (w weeksDrawn) tablePeaks(c *canvas, y int, row views.WeeksRow) {
	for i, p := range w.peaksShown(row) {
		c.rightFit(w.laid.weeks[i].At+peakCells, w.edge, y, w.peak(row, p))
	}
}

// peaksShown are the row's peaks in the weeks shown.
func (w weeksDrawn) peaksShown(row views.WeeksRow) []views.WeekPeak {
	return row.Peaks[max(len(row.Peaks)-len(w.laid.weeks), 0):]
}

// peak is a week's peak as the table draws it, as peakPart has it, this
// week's in its ink, as the number its account's use of the week is.
func (w weeksDrawn) peak(row views.WeeksRow, p views.WeekPeak) part {
	return peakPart(p, w.thisWeekInk(), w.number(row))
}

// peakPart is a week's peak as a percentage, a figure: in its level's colour
// as usage colours one, but bold only at 100%, in state.destructive; this
// week's so far, "34%…", in the ink given, as the number n; and — in
// text.faint where it isn't known.
func peakPart(p views.WeekPeak, open ink, n Number) part {
	if p.Share == nil {
		return figurePart(line{noFigure}, Number{})
	}
	text := views.Peak(*p.Share, p.Open)
	switch {
	case p.Open:
		return figurePart(line{{text, open}}, n)
	case prose.WholePercent(*p.Share) >= 100:
		return figureIn(text, exhaustedInk)
	}
	return figureIn(text, usedInk(*p.Share, ink{token: theme.VizRamp1}))
}

// number is the number this week's peak of the row's account is, or of
// every account's: its use of the week window so far.
func (w weeksDrawn) number(row views.WeeksRow) Number {
	return Number{Kind: NumberUsed, Account: row.Account, Of: w.page.Window}
}

// block draws the row's columns from row y, as look draws them: each week's
// column, filled to its peak, and under it the peak, centred.
func (w weeksDrawn) block(c *canvas, look Look, y int, row views.WeeksRow) {
	for i, p := range w.peaksShown(row) {
		x := w.laid.weeks[i].At
		w.column(c, look, x, y, p)
		label := w.columnPeak(row, p)
		at := x + (weekColumnCells-label.said.width())/2
		w.put(c, at, y+columnRows, label)
	}
}

// columnPeak is a week's peak as the columns draw it under its column, a
// figure: text.muted, at 100% bold state.destructive, this week's so far
// text.secondary, which brightens as it changes; and — in text.faint where
// it isn't known.
func (w weeksDrawn) columnPeak(row views.WeeksRow, p views.WeekPeak) part {
	if p.Share == nil {
		return figurePart(line{noFigure}, Number{})
	}
	text := views.Peak(*p.Share, p.Open)
	switch {
	case p.Open:
		return figurePart(line{{text, secondaryInk}}, w.number(row))
	case prose.WholePercent(*p.Share) >= 100:
		return figureIn(text, exhaustedInk)
	}
	return figureIn(text, mutedInk)
}

// column draws a week's column, filled from the bottom to its peak as it's
// written under it, a whole percent, its top cell in eighths, from row y down
// columnRows, as look draws its cells: each row in the ramp's colour at its
// middle, mixed from the canvas toward it; a week of no use ▁▁▁▁▁ in
// text.faint along the bottom; and one whose peak isn't known, nothing.
func (w weeksDrawn) column(c *canvas, look Look, x, y int, p views.WeekPeak) {
	if p.Share == nil {
		return
	}
	bottom := y + columnRows - 1
	if *p.Share <= 0 {
		c.text(x, bottom, strings.Repeat(levels[1], columnCells), faintInk)
		return
	}
	filled := halfEven(min(float64(prose.WholePercent(*p.Share))/100, 1) * columnRows * 8)
	for r := range columnRows {
		eighths := min(max(filled-8*r, 0), 8)
		if eighths == 0 {
			continue
		}
		glyph, k := w.columnCell(look, eighths, (float64(r)+0.5)/columnRows, p.Open)
		c.text(x, bottom-r, strings.Repeat(glyph, columnCells), k)
	}
}

// columnCell is a cell of a column's row filled eighths of it, 1 to 8, at t
// up the ramp, as look draws it, mixed columnMix from the canvas toward the
// ramp's colour there, or this week's thisWeekMix: where it blends, a
// filled cell is a space on that colour and a part-filled one its eighth
// block in it; where it doesn't, the fill is in blocks, █ and the eighths,
// this week's dim.
func (weeksDrawn) columnCell(look Look, eighths int, t float64, thisWeek bool) (string, ink) {
	fade := 1 - columnMix
	if thisWeek {
		fade = 1 - thisWeekMix
	}
	k := ink{ramp: true, at: t, fade: fade, dim: thisWeek}
	if eighths == 8 && look.blends() {
		return " ", ink{on: k.hue()}
	}
	return levels[eighths], k
}
