package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Days' cells: a day's; a week's, its days and the cell after them; the gap
// after the last week's, so a strip's count starts four cells after its last
// day; and the count's number's, right-aligned, its words after.
const (
	swatchCells    = 2
	weekCells      = 7*swatchCells + 1
	daysCountGap   = 2
	daysCountCells = 4
)

// Days' rows from its top: the weeks' first days, then this week's days'
// initials; BY MODEL, and its first strip, a row between them; each strip
// two rows on from the one before; and BY ACCOUNT, three rows on from the
// last family's.
const (
	byModelRow    = 2
	firstStripRow = 4
	stripStride   = 2
	byAccountGap  = 3
)

// daysAccounts are as many of Days' accounts as show at once, past which BY
// ACCOUNT scrolls within itself.
const daysAccounts = 6

// The words Days says: after BY MODEL and BY ACCOUNT, what each shows; under
// the shades, of a day it hit a 5-hour limit and of the days to come.
const (
	byModelSays    = "every account, weighed at API prices, on one scale"
	byAccountSays  = "the share of its week each day used, on one scale"
	daysShadesNote = "each shade a quarter of the active days"
	daysLimitNote  = "a day it hit a 5-hour limit"
	daysToComeNote = "days to come"
	toCome         = "··"
	limitMark      = "▪"
)

// The words after a strip's count: a family's share of use; and the 5-hour
// limits an account hit, the one, or any other count, which is the widest.
const (
	ofUse          = " of use"
	fiveHourLimit  = " 5-hour limit"
	fiveHourLimits = " 5-hour limits"
)

// historyDays draws Days from row top: the weeks over the days, BY MODEL's
// strips and BY ACCOUNT's, its accounts scrolled as the frame has them, and
// the key on the view's last row; where it scrolls whole, every account,
// drawn whole and scrolled by rows over the key, as paged pages it. Where
// nothing is recorded in its eight weeks, it says so.
func (f Frame) historyDays(c *canvas, top int) {
	p := f.History.Days
	if p == nil || !p.Recorded() {
		c.line(0, top, line{{"nothing recorded yet", dimInk}}.fit(f.edge()))
		return
	}
	l := f.layOutDays(*p, top)
	if l.whole {
		body := newCanvas(f.Width, l.rows())
		f.daysStrips(body, l)
		f.pasteWhole(c, body, top, l.paged(f))
	} else {
		f.daysStrips(c, l)
	}
	f.daysKey(c, l)
}

// daysStrips draws Days but its key, as laid out: the weeks over the days,
// BY MODEL's strips, and BY ACCOUNT's shown, with what's out of view.
func (f Frame) daysStrips(c *canvas, l daysLayout) {
	p := l.page
	f.daysWeeks(c, l)
	c.line(0, l.top+byModelRow, groupHead("BY MODEL", byModelSays, l.narrow).fit(f.edge()))
	for i, s := range p.ByModel {
		hue := familyHue(s.Of)
		named := familyDot(s.Of, secondaryInk)
		f.daysStrip(c, l, l.familyRow(i), func(room int) line { return named.fit(room) }, shareOfUse(s), func(level int) ink { return stepInk(hue, level) }, s.Days)
	}
	c.line(0, l.byAccount, groupHead("BY ACCOUNT", byAccountSays, l.narrow).fit(f.edge()))
	from, to := l.shown()
	for i, s := range p.ByAccount[from:to] {
		f.daysStrip(c, l, l.accountRow(i), func(room int) line { return historyAccount(labelled(s.Label, s.Of), s.Primary, room) }, f.limitsHit(s), dayInk, s.Days)
	}
	if above, below := from, len(p.ByAccount)-to; above+below > 0 {
		c.line(0, l.scrollRow, outOfView(above, below, f.edge(), historyAccountsOut...))
	}
}

// daysLayout is Days' page laid out in a frame: in its narrow form, where
// each strip's name takes the row over it; the row it starts at; where a
// strip's row lays its parts, as daysRow has them, and the first of the
// page's days it shows; the rows BY ACCOUNT, its first strip and what's out
// of view are on, its key's first, and how many rows the key takes; how many
// accounts show at once, and how far they're scrolled; and whether it's laid
// out whole, from row 0, to scroll by rows.
type daysLayout struct {
	page       views.DaysPage
	narrow     bool
	top        int
	row        daysRow
	first      int
	byAccount  int
	accountsAt int
	scrollRow  int
	keyRow     int
	keyRows    int
	most       int
	scrolled   int
	whole      bool
}

// daysRow is where a strip's row lays its parts: its label column, none on
// the narrow form; the cells its name is drawn in, the label column's less nameGap,
// or in the narrow form, those before its count; the weeks shown, the oldest first;
// and its count, in the narrow form on the row of its name.
type daysRow struct {
	labels flex.Span
	name   flex.Span
	weeks  []flex.Span
	count  flex.Span
}

// layOutDays lays out Days of the page given from row top: wide where it
// fits, else in its narrow form.
func (f Frame) layOutDays(p views.DaysPage, top int) daysLayout {
	return f.daysLaidOut(p, top, !f.daysWideFits(p))
}

// daysWideFits reports whether Days' wide form of the page fits the frame,
// as flex lays its rows short of the edge: a strip's, a week at least
// beside its label column, and the key, whole on one row, its widest, so
// the form never hangs on the data.
func (f Frame) daysWideFits(p views.DaysPage) bool {
	key := f.wideDaysKey().width()
	_, strips := flex.Lay(f.edge(), daysParts(p, false)...)
	_, keyed := flex.Lay(f.edge(), flex.Part{Least: key, Natural: key})
	return strips && keyed
}

// daysLaidOut lays out Days of the page given from row top: its strips' rows
// as daysParts declares them, in its narrow form under each strip's name, its
// key two rows, else beside it, its key a row; the key on the view's last
// rows, and what's out of view of BY ACCOUNT on the row over them. On a
// frame shorter than History's, or where BY ACCOUNT would show none of its
// accounts, it's laid out whole, every account shown, to scroll by rows.
func (f Frame) daysLaidOut(p views.DaysPage, top int, narrow bool) daysLayout {
	l := daysLayout{page: p, narrow: narrow, top: top, row: f.daysRowOf(p, narrow), keyRows: 1}
	if l.narrow {
		l.keyRows = 2
	}
	l.first = max(len(p.Weeks)-len(l.row.weeks), 0) * 7
	l.keyRow = top + f.viewRows() - l.keyRows
	l.scrollRow = l.keyRow - 1
	l.placeAccounts()
	l.most = min(max((l.scrollRow-2-l.accountsAt)/stripStride+1, 0), daysAccounts)
	if f.shortOfHistory() || l.most == 0 && len(p.ByAccount) > 0 {
		l.whole, l.top = true, 0
		l.placeAccounts()
		l.most = len(p.ByAccount)
		return l
	}
	l.scrolled = min(max(f.Scroll, 0), l.scrolling().Most)
	return l
}

// daysRowOf is where a strip's row of the page lays its parts in the frame,
// in its narrow form or wide, as flex lays them short of the edge: the weeks
// that didn't give way, none where even the last alone doesn't fit; in the
// narrow form, in a cell more, as nothing follows the blank after the last week's
// days, and its name and count as narrowNamed lays them.
func (f Frame) daysRowOf(p views.DaysPage, narrow bool) daysRow {
	if narrow {
		spans, fits := flex.Lay(f.edge()+1, daysParts(p, true)...)
		weeks := spansShown(spans, fits)
		name, count := f.narrowNamed(weeks)
		return daysRow{name: name, weeks: weeks, count: count}
	}
	spans, fits := flex.Lay(f.edge(), daysParts(p, false)...)
	last := len(spans) - 1
	return daysRow{
		labels: spans[0], name: flex.Span{Size: spans[0].Size - nameGap},
		weeks: spansShown(spans[1:last], fits), count: spans[last],
	}
}

// spansShown are the spans of the parts laid out given that didn't give
// way, where they fit; none where they don't.
func spansShown(spans []flex.Span, fit bool) []flex.Span {
	if !fit {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(spans), func(s flex.Span) bool { return s.Left })
}

// narrowNamed is where the narrow form lays a strip's name and its count on the row
// over its days: the count ending where the last of the weeks given ends its
// days, so it reads with its strip, the name a cell before it taking what's
// left, at least narrowNameLeast; or where that doesn't fit, the count ending
// at the edge, and cut there where even that doesn't.
func (f Frame) narrowNamed(weeks []flex.Span) (name, count flex.Span) {
	end := 0
	if len(weeks) > 0 {
		last := weeks[len(weeks)-1]
		end = last.At + last.Size - 1
	}
	counted := daysCount()
	counted.Gap = 1
	parts := []flex.Part{{Least: narrowNameLeast, Natural: narrowNameLeast, Grows: true}, counted}
	s, fits := flex.Lay(end, parts...)
	if !fits {
		s, _ = flex.Lay(f.edge(), parts...)
	}
	s[0].Size = min(s[0].Size, max(f.edge(), 0))
	return s[0], s[1]
}

// narrowNameLeast are the fewest cells the narrow form keeps for a strip's name
// before its count: two of the name as drawn, its marks among them, as w…
// and ●…, or for the primary its mark alone, " ◆".
const narrowNameLeast = 2

// daysParts are a strip's row's parts across the frame, of the page given:
// each of its weeks, the oldest first, which give way in that order, the
// last never; and wide, its label column before them, as daysLabels has it,
// and its count after them, daysCountGap on.
func daysParts(p views.DaysPage, narrow bool) []flex.Part {
	weeks := make([]flex.Part, len(p.Weeks))
	for i := range weeks {
		weeks[i] = flex.Part{Least: weekCells, Natural: weekCells}
		if i < len(weeks)-1 {
			weeks[i].Leaves = i + 1
		}
	}
	if narrow {
		return weeks
	}
	count := daysCount()
	count.Gap = daysCountGap
	return slices.Concat([]flex.Part{daysLabels(p, len(weeks))}, weeks, []flex.Part{count})
}

// daysCount is a strip's count, as wide as the widest, so its columns never
// hang on the data: its number, then its words, a family's share of use or
// an account's 5-hour limits.
func daysCount() flex.Part {
	cells := daysCountCells + max(ansi.StringWidth(ofUse), ansi.StringWidth(fiveHourLimits))
	return flex.Part{Least: cells, Natural: cells}
}

// daysLabels is Days' label column, nameGap after it, as namesColumn lays
// it out: a family's name, its dot among it, or any account's, its mark
// beside it, not only those shown, so the column holds still as BY ACCOUNT
// scrolls; narrowing only in the turn after the last of the weeks given
// could give way, so names read whole before a week is added.
func daysLabels(p views.DaysPage, weeks int) flex.Part {
	var names []columnName
	for _, s := range p.ByModel {
		names = append(names, columnName{label: familyDot(s.Of, secondaryInk).width()})
	}
	for _, s := range p.ByAccount {
		names = append(names, historyNamed(labelled(s.Label, s.Of), s.Primary))
	}
	labels := namesColumn(nameGap, names...)
	labels.Narrows = weeks
	return labels
}

// placeAccounts places BY ACCOUNT, three rows under the last family's
// strip, and its first strip a stride under it.
func (l *daysLayout) placeAccounts() {
	l.byAccount = l.familyRow(len(l.page.ByModel)-1) + byAccountGap
	l.accountsAt = l.byAccount + stripStride
}

// rows is how many rows Days takes laid out whole, from its top: to BY
// ACCOUNT's last strip, its days on the row under its name in the narrow form, or
// to its heading, where it has none.
func (l daysLayout) rows() int {
	n := len(l.page.ByAccount)
	switch {
	case n == 0:
		return l.byAccount + 1
	case l.narrow:
		return l.accountRow(n-1) + 2
	}
	return l.accountRow(n-1) + 1
}

// paged is how Days, laid out whole, pages in the frame's view, over its
// key, as heads holds its headings.
func (l daysLayout) paged(f Frame) rowsPaged {
	return f.wholePaged(l.rows(), l.keyRows, l.heads())
}

// heads are Days' headings, as it's laid out, each over the first row under
// it: BY MODEL's and BY ACCOUNT's over their first strips, where each has
// any; and in the narrow form, each strip's name over its days.
func (l daysLayout) heads() []headed {
	var heads []headed
	if len(l.page.ByModel) > 0 {
		heads = append(heads, headed{at: l.top + byModelRow, first: l.familyRow(0)})
	}
	for i := range l.page.ByModel {
		heads = append(heads, l.named(l.familyRow(i))...)
	}
	if len(l.page.ByAccount) > 0 {
		heads = append(heads, headed{at: l.byAccount, first: l.accountsAt})
	}
	for i := range l.page.ByAccount {
		heads = append(heads, l.named(l.accountRow(i))...)
	}
	return heads
}

// named is the strip on row y's name over its days, in the narrow form, where they
// take the row under it: none otherwise.
func (l daysLayout) named(y int) []headed {
	if !l.narrow {
		return nil
	}
	return []headed{{at: y, first: y + 1}}
}

// familyRow is the row BY MODEL's i-th strip is on.
func (l daysLayout) familyRow(i int) int {
	return l.top + firstStripRow + stripStride*i
}

// accountRow is the row the i-th of BY ACCOUNT's strips shown is on.
func (l daysLayout) accountRow(i int) int {
	return l.accountsAt + stripStride*i
}

// scrolling is how far BY ACCOUNT scrolls, by accounts: as many show as fit
// over the blank row above what's out of view, at most six; laid out whole,
// every one.
func (l daysLayout) scrolling() Scrolling {
	accounts := len(l.page.ByAccount)
	return Scrolling{Most: max(accounts-l.most, 0), Page: min(l.most, accounts)}
}

// shown are the accounts BY ACCOUNT shows, as far as it's scrolled, by their
// indexes: from the first up to to.
func (l daysLayout) shown() (from, to int) {
	return l.scrolled, l.scrolled + l.scrolling().Page
}

// daysScrolling is how far Days scrolls, scrolled offset down: laid out
// whole, by rows; else BY ACCOUNT, by accounts; zero where they fit, or
// there's nothing to draw.
func (f Frame) daysScrolling(offset int) Scrolling {
	p := f.History.Days
	if p == nil || !p.Recorded() || f.cramped() {
		return Scrolling{}
	}
	l := f.layOutDays(*p, f.top())
	if l.whole {
		return l.paged(f).scrolling(offset)
	}
	return l.scrolling()
}

// daysScrolls is what of Days scrolls, from row y, rows tall: laid out
// whole, the rows over its key; else BY ACCOUNT's strips, from the first's
// to the row over what's out of view; none where there's nothing to draw.
func (f Frame) daysScrolls() (y, rows int) {
	p := f.History.Days
	if p == nil || !p.Recorded() {
		return 0, 0
	}
	top := f.top()
	l := f.layOutDays(*p, top)
	if l.whole {
		return top, l.paged(f).page()
	}
	return l.accountsAt, l.scrollRow - l.accountsAt
}

// daysWeeks draws the weeks shown along Days' top row, each headed over it
// by its first day, in its days' cells; and on the row under it, over this
// week's days, their initials, in text.faint.
func (f Frame) daysWeeks(c *canvas, l daysLayout) {
	weeks := l.page.Weeks[l.first/7:]
	for i, week := range weeks {
		at, this := l.row.weeks[i], i == len(weeks)-1
		c.line(at.At, l.top, fitParts(at.Size-1, figurePart(weekHead(week, this), Number{})))
		if day, ok := dayfile.Civil(week); ok && this {
			initials(c, day, at.At, l.top+1)
		}
	}
}

// initials draws the initials of the seven days from the one given, a day
// every swatchCells from x along row y, in text.faint.
func initials(c *canvas, from time.Time, x, y int) {
	for d := range 7 {
		c.text(x+d*swatchCells, y, prose.WeekdayInitial(from.AddDate(0, 0, d).Weekday()), faintInk)
	}
}

// daysStrip draws a strip along row y, or in the narrow form, its name and count on
// row y and the strip under it: its name at column 0, as name gives it in
// the cells the row lays it out in; each of its days shown, in the ink
// shade gives at the day's level, those to come ··; and its count, its
// number a figure, cut short of the edge.
func (f Frame) daysStrip(c *canvas, l daysLayout, y int, name func(room int) line, count []part, shade func(level int) ink, days []views.StripDay) {
	strip := y
	if l.narrow {
		strip++
	}
	c.line(0, y, name(l.row.name.Size))
	at := l.row.count.At + daysCountCells - partsWidth(count[:1])
	c.line(at, y, fitParts(f.edge()-at, count...))
	coming := len(days) - l.page.ToCome
	for i := l.first; i < min(len(days), l.first+7*len(l.row.weeks)); i++ {
		x := l.dayAt(i)
		if i >= coming {
			c.text(x, strip, toCome, faintInk)
			continue
		}
		c.line(x, strip, f.swatch(shade(days[i].Level), days[i].Level, days[i].Limit))
	}
}

// dayAt is the column the page's day with index i is drawn at, in its week's
// cells, a day every swatchCells.
func (l daysLayout) dayAt(i int) int {
	shown := i - l.first
	return l.row.weeks[shown/7].At + shown%7*swatchCells
}

// swatch is a day swatchCells wide, as daySwatch draws one.
func (f Frame) swatch(k ink, level int, limited bool) line {
	return f.daySwatch(k, level, limited, swatchCells)
}

// daySwatch is a day cells wide in its shade's ink, k, at its level: spaces
// on its colour; or where the look blends nothing, its level's glyph, in its
// colour. A day it hit a 5-hour limit has a red ▪ in its middle cell, the
// first of two.
func (f Frame) daySwatch(k ink, level int, limited bool, cells int) line {
	glyph, drawn := " ", ink{on: k.hue()}
	if !f.Look.blends() {
		glyph, drawn = shadeGlyph(level), k
	}
	if !limited {
		return line{{strings.Repeat(glyph, cells), drawn}}
	}
	before := (cells - 1) / 2
	marked := line{{limitMark, ink{token: theme.StateDestructive, on: drawn.on}}, {strings.Repeat(glyph, cells-1-before), drawn}}
	if before == 0 {
		return marked
	}
	return append(line{{strings.Repeat(glyph, before), drawn}}, marked...)
}

// shareOfUse is a family's share of the eight weeks' worth, a figure, bold
// text.secondary, or — where none of it is priced, then of use.
func shareOfUse(s views.DaysStrip) []part {
	return []part{figurePart(shareSaid(s.Share, strongInk), Number{Kind: NumberShareOfUse, Of: s.Of}), wordsIn(ofUse, dimInk)}
}

// limitsHit is how many 5-hour limits an account hit in the eight weeks, a
// figure, bold state.destructive, or text.faint at none, then 5-hour limits.
func (f Frame) limitsHit(s views.DaysStrip) []part {
	count, noun := exhaustedInk, fiveHourLimits
	switch s.Limits {
	case 0:
		count = ink{token: theme.TextFaint, bold: true}
	case 1:
		noun = fiveHourLimit
	}
	n := Number{Kind: NumberLimitsHit, Account: s.Of, Of: f.Policy.Started}
	return []part{figurePart(line{{strconv.Itoa(s.Limits), count}}, n), wordsIn(noun, dimInk)}
}

// daysKey draws Days' key from its row: the five shades from less to more,
// and what they are; a day it hit a 5-hour limit; and the days to come. In the
// narrow form, the shades take a row of their own, with nothing said of them.
func (f Frame) daysKey(c *canvas, l daysLayout) {
	shades, marks := f.daysKeyParts()
	if l.narrow {
		c.line(0, l.keyRow, shades.fit(f.edge()))
		c.line(0, l.keyRow+1, marks.fit(f.edge()))
		return
	}
	c.line(0, l.keyRow, f.wideDaysKey().fit(f.edge()))
}

// daysKeyParts are the parts of Days' key: the shades, and the marks.
func (f Frame) daysKeyParts() (shades, marks line) {
	shades = line{{"less ", dimInk}}
	for level := range shadeLevels {
		if level > 0 {
			shades = append(shades, span{" ", ink{}})
		}
		shades = append(shades, f.swatch(dayInk(level), level, false)...)
	}
	shades = append(shades, span{" more", dimInk})
	marks = slices.Concat(f.swatch(dayInk(2), 2, true), line{{" " + daysLimitNote, dimInk}, {"   ", ink{}}, {toCome, faintInk}, {" " + daysToComeNote, dimInk}})
	return shades, marks
}

// wideDaysKey is Days' key in a row: the shades, what they are, and the
// marks.
func (f Frame) wideDaysKey() line {
	shades, marks := f.daysKeyParts()
	return slices.Concat(shades, line{{"  " + daysShadesNote, faintInk}, {"    ", ink{}}}, marks)
}
