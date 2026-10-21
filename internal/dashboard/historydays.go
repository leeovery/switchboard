package dashboard

import (
	"math"
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
	c.line(0, l.top+byModelRow, groupHead("BY MODEL", byModelSays, l.phone).fit(f.edge()))
	for i, s := range p.ByModel {
		hue := familyHue(s.Of)
		named := familyDot(s.Of, secondaryInk)
		f.daysStrip(c, l, l.familyRow(i), func(room int) line { return named.fit(room) }, shareOfUse(s), func(level int) ink { return stepInk(hue, level) }, s.Days)
	}
	c.line(0, l.byAccount, groupHead("BY ACCOUNT", byAccountSays, l.phone).fit(f.edge()))
	from, to := l.shown()
	for i, s := range p.ByAccount[from:to] {
		f.daysStrip(c, l, l.accountRow(i), func(room int) line { return historyAccount(labelled(s.Label, s.Of), s.Primary, room) }, f.limitsHit(s), dayInk, s.Days)
	}
	if above, below := from, len(p.ByAccount)-to; above+below > 0 {
		c.line(0, l.scrollRow, outOfView(above, below, f.edge(), historyAccountsOut...))
	}
}

// daysLayout is Days' page laid out in a frame: in its phone form, where
// each strip's name takes the row over it; the row it starts at; where a
// strip's row lays its parts, as daysRow has them, and the first of the
// page's days it shows; the rows BY ACCOUNT, its first strip and what's out
// of view are on, its key's first, and how many rows the key takes; how many
// accounts show at once, and how far they're scrolled; and whether it's laid
// out whole, from row 0, to scroll by rows.
type daysLayout struct {
	page       views.DaysPage
	phone      bool
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
// a phone; the weeks shown, the oldest first; and its count, on a phone on
// the row of its name.
type daysRow struct {
	labels flex.Span
	weeks  []flex.Span
	count  flex.Span
}

// layOutDays lays out Days of the page given from row top: wide where it
// fits, else in its phone form.
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
// as daysParts declares them, in its phone form under each strip's name, its
// key two rows, else beside it, its key a row; the key on the view's last
// rows, and what's out of view of BY ACCOUNT on the row over them. On a
// frame shorter than History's, or where BY ACCOUNT would show none of its
// accounts, it's laid out whole, every account shown, to scroll by rows.
func (f Frame) daysLaidOut(p views.DaysPage, top int, phone bool) daysLayout {
	l := daysLayout{page: p, phone: phone, top: top, row: f.daysRowOf(p, phone), keyRows: 1}
	if l.phone {
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
// in its phone form or wide, as flex lays them short of the edge, but where
// even the last week alone doesn't fit, only one whose days end short of it;
// on a phone, its count as phoneCount lays it.
func (f Frame) daysRowOf(p views.DaysPage, phone bool) daysRow {
	spans, _ := flex.Lay(f.edge(), daysParts(p, phone)...)
	if phone {
		weeks := f.daysWeeksShown(spans)
		return daysRow{weeks: weeks, count: f.phoneCount(weeks)}
	}
	last := len(spans) - 1
	return daysRow{labels: spans[0], weeks: f.daysWeeksShown(spans[1:last]), count: spans[last]}
}

// daysWeeksShown are the weeks of those laid out given that didn't give way,
// and whose days end short of the edge.
func (f Frame) daysWeeksShown(weeks []flex.Span) []flex.Span {
	return slices.DeleteFunc(slices.Clone(weeks), func(week flex.Span) bool {
		return week.Left || week.At+week.Size-1 > f.edge()
	})
}

// phoneCount is where a phone lays a strip's count on the row of its name,
// a cell after the name, which takes what's left, at least phoneNameLeast:
// ending where the last of the weeks given ends its days, so it reads with
// its strip; or where that leaves the name too few cells, at the edge.
func (f Frame) phoneCount(weeks []flex.Span) flex.Span {
	end := 0
	if len(weeks) > 0 {
		last := weeks[len(weeks)-1]
		end = last.At + last.Size - 1
	}
	count := daysCount()
	count.Gap = 1
	parts := []flex.Part{{Least: phoneNameLeast, Natural: phoneNameLeast, Grows: true}, count}
	s, fits := flex.Lay(end, parts...)
	if !fits {
		s, _ = flex.Lay(f.edge(), parts...)
	}
	return s[1]
}

// phoneNameLeast are the fewest cells a phone keeps for a strip's name
// before its count: its first letter and an ellipsis.
const phoneNameLeast = 2

// daysParts are a strip's row's parts across the frame, of the page given:
// each of its weeks, the oldest first, which give way in that order, the
// last never; and wide, its label column before them, as daysLabels has it,
// and its count after them, daysCountGap on.
func daysParts(p views.DaysPage, phone bool) []flex.Part {
	weeks := make([]flex.Part, len(p.Weeks))
	for i := range weeks {
		weeks[i] = flex.Part{Least: weekCells, Natural: weekCells}
		if i < len(weeks)-1 {
			weeks[i].Leaves = i + 1
		}
	}
	if phone {
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

// daysLabels is Days' label column, its gap after it: its widest name and
// nameGap, a family's or any account's, not only those shown, so the column
// holds still as BY ACCOUNT scrolls; narrowing only in the turn after the
// last of the weeks given could give way, so names read whole before a week
// is added.
func daysLabels(p views.DaysPage, weeks int) flex.Part {
	widest := 0
	for _, s := range p.ByModel {
		widest = max(widest, familyDot(s.Of, secondaryInk).width())
	}
	for _, s := range p.ByAccount {
		widest = max(widest, historyAccount(labelled(s.Label, s.Of), s.Primary, math.MaxInt).width())
	}
	return flex.Part{Least: leastRunwayLabels, Natural: widest + nameGap, Most: mostRunwayLabels, Narrows: weeks}
}

// placeAccounts places BY ACCOUNT, three rows under the last family's
// strip, and its first strip a stride under it.
func (l *daysLayout) placeAccounts() {
	l.byAccount = l.familyRow(len(l.page.ByModel)-1) + byAccountGap
	l.accountsAt = l.byAccount + stripStride
}

// rows is how many rows Days takes laid out whole, from its top: to BY
// ACCOUNT's last strip, its days on the row under its name on a phone, or
// to its heading, where it has none.
func (l daysLayout) rows() int {
	n := len(l.page.ByAccount)
	switch {
	case n == 0:
		return l.byAccount + 1
	case l.phone:
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
// any; and on a phone, each strip's name over its days.
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

// named is the strip on row y's name over its days, on a phone, where they
// take the row under it: none otherwise.
func (l daysLayout) named(y int) []headed {
	if !l.phone {
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

// daysStrip draws a strip along row y, or on a phone, its name and count on
// row y and the strip under it: its name at column 0, as name gives it in
// the label column's cells less nameGap, or on a phone those before its
// count less one, short of the edge; each of its days shown, in the ink
// shade gives at the day's level, those to come ··; and its count, its
// number a figure, cut short of the edge.
func (f Frame) daysStrip(c *canvas, l daysLayout, y int, name func(room int) line, count []part, shade func(level int) ink, days []views.StripDay) {
	strip, room := y, l.row.labels.Size-nameGap
	if l.phone {
		strip, room = y+1, min(l.row.count.At-1, f.edge())
	}
	c.line(0, y, name(room))
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
// and what they are; a day it hit a 5-hour limit; and the days to come. On a
// phone, the shades take a row of their own, with nothing said of them.
func (f Frame) daysKey(c *canvas, l daysLayout) {
	shades, marks := f.daysKeyParts()
	if l.phone {
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
