package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Weeks' columns: the weeks' from peaksFrom, peaksApart apart, a table's
// peak right-aligned in peakCells; the verdicts' at verdictsAt; a name cut
// to the cells before the weeks, less one; and on a phone, the last
// weeksPhoneWeeks of them.
const (
	peaksFrom       = 14
	peaksApart      = 8
	peakCells       = 5
	verdictsAt      = 82
	weeksNameCells  = peaksFrom - 1
	weeksPhoneWeeks = 4
)

// Weeks' column shape: a block an account, blockRows apart, each week a
// column columnCells wide and columnRows tall, its peak under it centred in
// peakLabelCells; each row of a column mixed columnMix from the canvas
// toward its colour, this week's thisWeekMix.
const (
	blockRows      = 6
	columnCells    = 5
	columnRows     = 3
	peakLabelCells = 6
	columnMix      = 0.75
	thisWeekMix    = 0.45
)

// The rows of Weeks' table, from the view's top: the weeks' first days',
// every account's, and the first account's.
const (
	weeksHeadRow  = 4
	weeksAllRow   = 6
	weeksFirstRow = 8
)

// historyWeeks draws Weeks from row top: once a whole week is recorded, the
// capacity verdict, then the table or the columns, as v shapes it, and the
// legend on the view's last row; in its phone form, the verdict and the
// table's last four weeks. Till then, it says there aren't enough weeks yet;
// and before its page is built, it's blank.
func (f Frame) historyWeeks(c *canvas, top int) {
	p := f.History.Weeks
	switch {
	case p == nil:
		return
	case p.Capacity.Weeks == 0:
		c.line(0, top, line{{"not enough weeks yet", dimInk}})
		return
	}
	w := f.weeksDrawn(*p)
	foot := top + f.viewRows() - 1
	switch {
	case w.phone:
		f.weeksOnAPhone(c, w, top, foot)
	case w.columns:
		f.weeksColumns(c, w, top, foot)
	default:
		f.weeksTable(c, w, top, foot)
	}
}

// weeksDrawn is Weeks' page as the frame draws it: in the shape v gives it,
// where that fits, else in its phone form.
func (f Frame) weeksDrawn(p views.WeeksPage) weeksDrawn {
	w := weeksDrawn{page: p, columns: f.History.WeeksShape == WeeksColumns, edge: f.edge()}
	w.phone = f.phoneForm(w.reach, w.edge)
	w.columns = w.columns && !w.phone
	return w
}

// reach is the column Weeks' wide form, in its shape, ends before, where its
// widest row ends: its headline, its weeks or its legend. What's beside the
// weeks is cut at the frame's edge, so it's left out; as is what's out of
// view, which takes a form that fits there.
func (w weeksDrawn) reach() int {
	if !w.columns {
		return max(w.legend().width(), w.headline().width(), peaksFrom+peaksApart*(len(w.page.Weeks)-1)+peakCells+1)
	}
	end := w.legend().width()
	for i := range w.page.Weeks {
		end = max(end, peaksFrom+peaksApart*i+w.head(i).width())
	}
	rows := w.page.Accounts
	if w.several() {
		rows = append([]views.WeeksRow{w.page.All}, rows...)
	}
	for _, row := range rows {
		for i, p := range row.Peaks {
			x, label := peaksFrom+peaksApart*i, w.columnPeak(row, p)
			end = max(end, x+columnCells, x+(peakLabelCells-label.width())/2+label.width())
		}
	}
	return end
}

// weeksScrolling is how far Weeks scrolls, in accounts, as the frame draws
// it: zero where its accounts fit, as they always do before its page is
// built or a whole week recorded.
func (f Frame) weeksScrolling() Scrolling {
	p := f.History.Weeks
	if p == nil || p.Capacity.Weeks == 0 || f.cramped() {
		return Scrolling{}
	}
	page := f.weeksPage(f.weeksDrawn(*p))
	return Scrolling{Most: max(len(p.Accounts)-page, 0), Page: page}
}

// weeksScrolls is what of Weeks scrolls, from row y, rows tall: the rows of
// the accounts it shows, from the first's, none before its page is built or
// a whole week recorded.
func (f Frame) weeksScrolls() (y, rows int) {
	p := f.History.Weeks
	if p == nil || p.Capacity.Weeks == 0 {
		return 0, 0
	}
	w := f.weeksDrawn(*p)
	if rows = f.weeksPage(w); w.columns {
		rows *= blockRows
	}
	return f.weeksFirst(w), rows
}

// weeksFirst is the row of the first account Weeks shows: under the table's
// every account's, after the verdict, or in its phone form, after the
// verdict's rows; or in columns, the first block after every account's.
func (f Frame) weeksFirst(w weeksDrawn) int {
	top := f.top()
	switch {
	case w.phone:
		return top + len(w.phoneVerdict()) + 1 + weeksFirstRow - weeksHeadRow
	case w.columns:
		return top + blockRows
	}
	return top + weeksFirstRow
}

// weeksPage is how many accounts Weeks shows at once, from the first's row:
// in the table, those that fit to the one over the scroll line, over the
// legend; in columns, the blocks that fit, the last one's dates under it;
// and in its phone form, those that fit to the scroll line, the view's last
// row. It's one at least.
func (f Frame) weeksPage(w weeksDrawn) int {
	first, foot := f.weeksFirst(w), f.top()+f.viewRows()-1
	switch {
	case w.phone:
		return max(foot-first, 1)
	case w.columns:
		return max((foot-first)/blockRows, 1)
	}
	return max(foot-1-first, 1)
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
		c.line(0, y, accountsOutOfView(above, below, w.edge))
	}
}

// weeksTable draws Weeks' table from row top to the legend on row foot: the
// verdict, then the weeks' first days, every account's peaks, and each
// account's, its verdict at the right.
func (f Frame) weeksTable(c *canvas, w weeksDrawn, top, foot int) {
	c.line(0, top, w.headline())
	w.put(c, 0, top+1, joined(span{"   ·   ", dimInk}, slices.Concat([]line{w.asNow()}, w.replays())...))
	w.put(c, 0, top+2, w.thirdRow())
	shown, above, below := f.weeksShown(w)
	w.table(c, top+weeksHeadRow, len(w.page.Weeks), shown, true)
	w.accountsOut(c, foot-1, above, below)
	c.line(0, foot, w.legend())
}

// table draws the table's last weeks of the page's from its heads' row y:
// their first days, every account's peaks, and the peaks of the accounts
// shown; and where aside is set, the verdicts at the right.
func (w weeksDrawn) table(c *canvas, y, weeks int, shown []views.WeeksRow, aside bool) {
	w.weekHeads(c, y, weeks)
	all, first := y+weeksAllRow-weeksHeadRow, y+weeksFirstRow-weeksHeadRow
	if aside {
		w.put(c, verdictsAt, y, line{{"over the last " + prose.Counted(w.page.Capacity.Weeks, "week"), faintInk}})
	}
	if w.several() {
		c.line(0, all, line{{"all accounts", strongInk}})
		w.tablePeaks(c, all, w.page.All, weeks)
	}
	if w.several() && aside {
		w.put(c, verdictsAt, all, w.allShort())
	}
	for i, row := range shown {
		c.line(0, first+i, w.name(row))
		w.tablePeaks(c, first+i, row, weeks)
		if aside {
			w.put(c, verdictsAt, first+i, w.tableVerdict(row))
		}
	}
}

// weeksColumns draws Weeks' columns from row top to the legend on row foot:
// a block for every account, then one an account, each week's peak a column,
// with the verdict, or the account's, beside it; and the weeks' first days
// under the last.
func (f Frame) weeksColumns(c *canvas, w weeksDrawn, top, foot int) {
	if w.several() {
		c.line(0, top, line{{"all accounts", strongInk}})
		w.block(c, f.Look, top, w.page.All)
	}
	for i, l := range w.allAside() {
		w.put(c, verdictsAt, top+i, l)
	}
	shown, above, below := f.weeksShown(w)
	y := top
	for _, row := range shown {
		y += blockRows
		c.line(0, y, w.name(row))
		w.block(c, f.Look, y, row)
		for i, l := range w.accountAside(row) {
			w.put(c, verdictsAt, y+i, l)
		}
	}
	for i := range w.page.Weeks {
		c.line(peaksFrom+peaksApart*i, y+columnRows+1, w.head(i))
	}
	w.accountsOut(c, foot-1, above, below)
	c.line(0, foot, w.legend())
}

// weeksOnAPhone draws Weeks on a phone from row top: the verdict, a part a
// row, wrapped to the width; then the table's last four weeks, with no
// verdicts beside it, its accounts scrolling to row foot.
func (f Frame) weeksOnAPhone(c *canvas, w weeksDrawn, top, foot int) {
	verdict := w.phoneVerdict()
	for i, l := range verdict {
		c.line(0, top+i, l)
	}
	shown, above, below := f.weeksShown(w)
	w.table(c, top+len(verdict)+1, min(weeksPhoneWeeks, len(w.page.Weeks)), shown, false)
	w.accountsOut(c, foot, above, below)
}

// weeksDrawn is Weeks' page as it's drawn: in its phone form, in columns, or
// as a table, its lines cut short before the column edge.
type weeksDrawn struct {
	page           views.WeeksPage
	phone, columns bool
	edge           int
}

// put draws the line from x along row y, cut short to end before the edge.
func (w weeksDrawn) put(c *canvas, x, y int, l line) {
	c.line(x, y, l.fit(w.edge-x))
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

// headline is the verdict's headline, bold, as "three is about right": in
// accent.attention where there are too few, else in state.positive.
func (w weeksDrawn) headline() line {
	k := boldPositiveInk
	if v := w.page.Capacity.Verdict; v == views.OneTooFew || v == views.TooFew {
		k = alertInk
	}
	return line{{prose.Number(len(w.page.Accounts)) + " is " + w.page.Capacity.Verdict, k}}
}

// asNow is how many of the last whole weeks were short as the accounts
// stand, and who was short each time, where the same were:
// "as now: short on 3 of the last 7 weeks, each time work alone".
func (w weeksDrawn) asNow() line {
	c := w.page.Capacity
	l := line{
		{"as now: ", dimInk},
		{"short on " + w.shortCount(c.Short) + " of the last " + prose.Counted(c.Weeks, "week"), w.countInk(c.Short)},
	}
	names := make([]string, len(w.page.ShortEach))
	for i, id := range w.page.ShortEach {
		names[i] = w.named(id)
	}
	switch len(names) {
	case 0:
	case 1:
		l = append(l, span{", each time " + names[0] + " alone", mutedInk})
	default:
		l = append(l, span{", " + prose.List(names) + " each time", mutedInk})
	}
	return l
}

// replays are what the replays with one account fewer and one more find,
// "with one fewer: short on 6 of 7" and "with one more: short on none":
// where one can be dropped, one fewer's saying where its weeks are the
// same, and with two fewer in one more's place; with one account, whether a
// second would have cleared the weeks short.
func (w weeksDrawn) replays() []line {
	c := w.page.Capacity
	switch {
	case c.WithOneFewer == nil && c.Short == 0:
		return nil
	case c.WithOneFewer == nil && c.WithOneMore == 0:
		return []line{{{"a second would have cleared them", mutedInk}}}
	case c.WithOneFewer == nil:
		return []line{w.replay("with a second: ", c.WithOneMore, "")}
	}
	same := ""
	if w.page.SameWeeks {
		same = ", the same weeks"
	}
	fewer := w.replay("with one fewer: ", *c.WithOneFewer, same)
	if c.WithTwoFewer != nil {
		return []line{fewer, w.replay("with two fewer: ", *c.WithTwoFewer, "")}
	}
	return []line{fewer, w.replay("with one more: ", c.WithOneMore, "")}
}

// replay is a replay's label and the weeks it finds short, n of those
// judged, and what follows them.
func (w weeksDrawn) replay(label string, n int, after string) line {
	return line{{label, dimInk}, {w.shortOn(n, w.page.Capacity.Weeks) + after, w.countInk(n)}}
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

// shortCount counts weeks short, as "3", or "none".
func (weeksDrawn) shortCount(n int) string {
	if n == 0 {
		return "none"
	}
	return strconv.Itoa(n)
}

// estimate says the verdict is an estimate, as few accounts' form words it,
// or many's.
func (w weeksDrawn) estimate() line {
	if w.many() {
		return line{{"an estimate, from replaying these weeks", faintInk}}
	}
	return line{{"an estimate, from replaying these weeks with one account fewer or more", faintInk}}
}

// dropped names the account to drop and says why:
// "spare is the one to drop: $100/mo, 5% used on average, and nothing ran
// short without it".
func (w weeksDrawn) dropped() line {
	var facts []string
	if p := w.page.DropPrice; p != nil {
		facts = append(facts, p.GroupedDollars()+"/mo")
	}
	if used := w.row(w.page.Capacity.Drop).Used; used != nil {
		facts = append(facts, status.Percent(*used)+" used on average")
	}
	said := strings.Join(facts, ", ")
	switch {
	case w.page.NoneShortWithout && said != "":
		said += ", and nothing ran short without it"
	case w.page.NoneShortWithout:
		said = "nothing ran short without it"
	}
	text := w.named(w.page.Capacity.Drop) + " is the one to drop"
	if said != "" {
		text += ": " + said
	}
	return line{{text, mutedInk}}
}

// thirdRow is the verdict's third row: with many accounts, the one to drop,
// then three spaces and the estimate; with few, the estimate alone.
func (w weeksDrawn) thirdRow() line {
	if w.many() {
		return slices.Concat(w.dropped(), line{{"   ", ink{}}}, w.estimate())
	}
	return w.estimate()
}

// phoneVerdict is the verdict as a phone draws it: a part a row, each
// wrapped to the edge.
func (w weeksDrawn) phoneVerdict() []line {
	parts := slices.Concat([]line{w.headline(), w.asNow()}, w.replays())
	if w.many() {
		parts = append(parts, w.dropped())
	}
	var rows []line
	for _, l := range append(parts, w.estimate()) {
		rows = append(rows, l.wrap(w.edge)...)
	}
	return rows
}

// allAside is what's beside every account's block: the headline, as now,
// the replays three spaces apart, and the estimate.
func (w weeksDrawn) allAside() []line {
	return []line{w.headline(), w.asNow(), joined(span{"   ", ink{}}, w.replays()...), w.estimate()}
}

// allShort is every account's verdict in the table: how many of the weeks
// judged were short, and with many accounts, how many there are.
func (w weeksDrawn) allShort() line {
	c := w.page.Capacity
	l := line{{"short on " + w.shortCount(c.Short) + " of " + prose.Counted(c.Weeks, "week"), strongInk}}
	if w.many() {
		l = append(l, span{"   with " + prose.Number(len(w.page.Accounts)) + " accounts", dimInk})
	}
	return l
}

// verdictInk is the ink of an account's verdict, bold: hits its limit in
// state.destructive, plenty to spare in state.positive, could go and barely
// used in accent.attention, and ends with room in text.secondary.
func (weeksDrawn) verdictInk(verdict string) ink {
	switch verdict {
	case views.HitsItsLimit:
		return exhaustedInk
	case views.PlentyToSpare:
		return boldPositiveInk
	case views.CouldGo, views.BarelyUsed:
		return alertInk
	}
	return strongInk
}

// spared is the verdict of an account that could go or is barely used,
// alone in its cell: "could go: 5% used, no limit ever", or "barely used:
// 80% left at a reset"; reporting false of any other.
func (w weeksDrawn) spared(row views.WeeksRow) (line, bool) {
	var says string
	switch {
	case row.Verdict == views.CouldGo && row.Used != nil:
		says = status.Percent(*row.Used) + " used, no limit ever"
	case row.Verdict == views.BarelyUsed && row.LeftAtReset != nil:
		says = status.Percent(*row.LeftAtReset) + " left at a reset"
	case row.Verdict != views.CouldGo && row.Verdict != views.BarelyUsed:
		return nil, false
	}
	return line{{row.Verdict + ": ", w.verdictInk(row.Verdict)}, {says, mutedInk}}, true
}

// tableVerdict is an account's verdict in the table, bold in its colour,
// then what it left at a reset, and with few accounts, how many weeks the
// rest would have been short without it.
func (w weeksDrawn) tableVerdict(row views.WeeksRow) line {
	if l, ok := w.spared(row); ok || row.Verdict == "" {
		return l
	}
	l := line{{row.Verdict, w.verdictInk(row.Verdict)}}
	switch left := row.LeftAtReset; {
	case left != nil && w.many():
		l = append(l, span{"   " + status.Percent(*left) + " left at a reset", dimInk})
	case left != nil:
		l = append(l, span{"  " + status.Percent(*left) + " left", dimInk})
	}
	if row.WithoutIt != nil && !w.many() {
		l = append(l, span{fmt.Sprintf("  without it: short %d of %d", *row.WithoutIt, w.page.Capacity.Weeks), dimInk})
	}
	return l
}

// accountAside is what's beside an account's block: its verdict; how many
// of its weeks it hit its limit in and what they left at a reset; and how
// many weeks the rest would have been short without it.
func (w weeksDrawn) accountAside(row views.WeeksRow) []line {
	if row.Verdict == "" {
		return nil
	}
	verdict, ok := w.spared(row)
	if !ok {
		verdict = line{{row.Verdict, w.verdictInk(row.Verdict)}}
	}
	record := fmt.Sprintf("limit hit %d of %s", row.Limits, prose.Counted(row.Weeks, "week"))
	if row.LeftAtReset != nil {
		record += " · " + status.Percent(*row.LeftAtReset) + " left at a reset, on average"
	}
	aside := []line{verdict, {{record, mutedInk}}}
	if row.WithoutIt != nil {
		aside = append(aside, line{{"without it, the rest: ", dimInk}, {w.shortOn(*row.WithoutIt, w.page.Capacity.Weeks), mutedInk}})
	}
	return aside
}

// shortOn says how many of the weeks judged were short, n of them:
// "short on 6 of 7", or "short on none".
func (weeksDrawn) shortOn(n, weeks int) string {
	if n == 0 {
		return "short on none"
	}
	return fmt.Sprintf("short on %d of %d", n, weeks)
}

// legend is the key to how the peaks are drawn, each mark as it's drawn and
// the words in text.faint; the columns' led by what short means.
func (w weeksDrawn) legend() line {
	var l line
	if w.columns {
		l = line{{"short: a week some account hit its limit", faintInk}, {"   ·   ", faintInk}}
	}
	return append(l,
		span{"each week at its peak", faintInk}, span{"   ·   ", faintInk},
		span{"100%", exhaustedInk}, span{" it hit its limit", faintInk}, span{"   ·   ", faintInk},
		span{"34%…", w.thisWeekInk()}, span{" this week, so far", faintInk},
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

// name is an account's name and mark at the start of its row, cut short of
// the weeks.
func (w weeksDrawn) name(row views.WeeksRow) line {
	return historyAccount(w.named(row.Account), row.Primary, weeksNameCells)
}

// weekHeads draws the first days of the last weeks of the page's along row
// y, each right-aligned in a cell more than a peak.
func (w weeksDrawn) weekHeads(c *canvas, y, weeks int) {
	first := len(w.page.Weeks) - weeks
	for i := range weeks {
		c.right(peaksFrom+peaksApart*i+peakCells+1, y, w.head(first+i))
	}
}

// head is the page's week with index i, headed by its first day.
func (w weeksDrawn) head(i int) line {
	return weekHead(w.page.Weeks[i], i == len(w.page.Weeks)-1)
}

// tablePeaks draws the row's peaks in the last weeks of the page's along row
// y, each right-aligned in peakCells.
func (w weeksDrawn) tablePeaks(c *canvas, y int, row views.WeeksRow, weeks int) {
	for i, p := range row.Peaks[len(row.Peaks)-weeks:] {
		c.right(peaksFrom+peaksApart*i+peakCells, y, w.peak(row, p))
	}
}

// peak is a week's peak as the table draws it: a percentage, in its level's
// colour as usage colours one, but bold only at 100%, in
// state.destructive; this week's so far, "34%…", bold text.secondary, which
// brightens as it changes; and — in text.faint where it isn't known.
func (w weeksDrawn) peak(row views.WeeksRow, p views.WeekPeak) line {
	if p.Share == nil {
		return line{noFigure}
	}
	text := status.Percent(*p.Share)
	if p.Open {
		return line{{text + "…", w.thisWeekInk()}}.numbered(w.number(row))
	}
	if status.WholePercent(*p.Share) >= 100 {
		return line{{text, exhaustedInk}}
	}
	return line{{text, usedInk(*p.Share, ink{token: theme.VizRamp1})}}
}

// number is the number this week's peak of the row's account is, or of
// every account's: its use of the week window so far.
func (w weeksDrawn) number(row views.WeeksRow) Number {
	return Number{Kind: NumberUsed, Account: row.Account, Of: w.page.Window}
}

// block draws the row's columns from row y, as look draws them: each week's
// column, filled to its peak, and under it the peak, centred.
func (w weeksDrawn) block(c *canvas, look Look, y int, row views.WeeksRow) {
	for i, p := range row.Peaks {
		x := peaksFrom + peaksApart*i
		w.column(c, look, x, y, p)
		label := w.columnPeak(row, p)
		c.line(x+(peakLabelCells-label.width())/2, y+columnRows, label)
	}
}

// columnPeak is a week's peak as the columns draw it under its column:
// text.muted, at 100% bold state.destructive, this week's so far
// text.secondary, which brightens as it changes; and — in text.faint where
// it isn't known.
func (w weeksDrawn) columnPeak(row views.WeeksRow, p views.WeekPeak) line {
	switch {
	case p.Share == nil:
		return line{noFigure}
	case p.Open:
		return line{{status.Percent(*p.Share) + "…", secondaryInk}}.numbered(w.number(row))
	case status.WholePercent(*p.Share) >= 100:
		return line{{status.Percent(*p.Share), exhaustedInk}}
	}
	return line{{status.Percent(*p.Share), mutedInk}}
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
	filled := halfEven(min(float64(status.WholePercent(*p.Share))/100, 1) * columnRows * 8)
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
