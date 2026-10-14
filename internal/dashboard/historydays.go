package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Days' columns: where its days start, a day's cells and a week's, its days'
// and the cell after them; and where a strip's count, its share of use or
// its 5-hour limits, starts, right-aligned in daysCountCells.
const (
	daysAt         = 14
	swatchCells    = 2
	weekCells      = 7*swatchCells + 1
	daysCount      = 136
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

// Days' accounts, as many as show at once, past which BY ACCOUNT scrolls
// within itself; and daysPhoneWeeks, the weeks a phone shows, the last of
// the eight, or as many of them as fit.
const (
	daysAccounts   = 6
	daysPhoneWeeks = 3
)

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
// drawn whole and scrolled by rows over the key. Where nothing is recorded
// in its eight weeks, it says so.
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
		f.pasteWhole(c, body, top, l.keyRows)
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
		f.daysStrip(c, l, l.familyRow(i), familyDot(s.Of, secondaryInk).fit, shareOfUse(s), func(level int) ink { return stepInk(hue, level) }, s.Days)
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
// each strip's name takes the row over it; the row it starts at; the column
// its days start at, and the first of the page's days it shows; the rows BY
// ACCOUNT, its first strip and what's out of view are on, its key's first,
// and how many rows the key takes; how many accounts show at once, and how
// far they're scrolled; and whether it's laid out whole, from row 0, to
// scroll by rows.
type daysLayout struct {
	page       views.DaysPage
	phone      bool
	top        int
	x, first   int
	byAccount  int
	accountsAt int
	scrollRow  int
	keyRow     int
	keyRows    int
	most       int
	scrolled   int
	whole      bool
}

// layOutDays lays out Days of the page given from row top: wide where it
// fits, else in its phone form.
func (f Frame) layOutDays(p views.DaysPage, top int) daysLayout {
	return f.daysLaidOut(p, top, f.phoneForm(func() int { return f.daysReach(p) }))
}

// daysLaidOut lays out Days of the page given from row top: in its phone
// form, the last three weeks, or as many as fit short of the edge, under
// each strip's name, its key two rows; else every week from column 14
// beside it, its key a row; the key on the view's last rows, and what's out
// of view of BY ACCOUNT on the row over them. On a frame shorter than
// History's, or where BY ACCOUNT would show none of its accounts, it's laid
// out whole, every account shown, to scroll by rows.
func (f Frame) daysLaidOut(p views.DaysPage, top int, phone bool) daysLayout {
	l := daysLayout{page: p, phone: phone, top: top, x: daysAt, keyRows: 1}
	if l.phone {
		weeks := min(daysPhoneWeeks, max((f.edge()+1)/weekCells, 0))
		l.x, l.first, l.keyRows = 0, max(len(p.Weeks)-weeks, 0)*7, 2
	}
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

// daysReach is the width Days' wide form of the page given takes: where its
// widest row ends, and the margin after it, as Days ends its rows at the
// edge, as its scroll line does.
func (f Frame) daysReach(p views.DaysPage) int {
	return f.daysWidest(p) + margin
}

// daysWidest is the column Days' wide form of the page given ends before,
// where its widest row ends: the weeks' heads, a group's heading, a strip's
// days or its count, whose number is right-aligned, so it ends where its
// words do, at the widest they're drawn, so the form never hangs on the
// count, or the key. A strip's name is cut short of its days, and what's out
// of view of BY ACCOUNT fits the width it's drawn at, so they're left out.
func (f Frame) daysWidest(p views.DaysPage) int {
	l := daysLayout{page: p, x: daysAt}
	end := max(groupHead("BY MODEL", byModelSays, false).width(), groupHead("BY ACCOUNT", byAccountSays, false).width(), f.wideDaysKey().width())
	for i, week := range p.Weeks {
		end = max(end, daysAt+i*weekCells+weekHead(week, i == len(p.Weeks)-1).width())
	}
	for _, s := range p.ByModel {
		end = max(end, l.stripReach(s.Days, ofUse))
	}
	for _, s := range p.ByAccount {
		end = max(end, l.stripReach(s.Days, fiveHourLimits))
	}
	return end
}

// stripReach is the column a wide strip of the days given ends before: its
// last day's, or its count's, its words those given.
func (l daysLayout) stripReach(days []views.StripDay, words string) int {
	end := daysCount + daysCountCells + ansi.StringWidth(words)
	if len(days) > 0 {
		end = max(end, l.dayAt(len(days)-1)+swatchCells)
	}
	return end
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

// daysScrolling is how far Days scrolls: laid out whole, by rows; else BY
// ACCOUNT, by accounts; zero where they fit, or there's nothing to draw.
func (f Frame) daysScrolling() Scrolling {
	p := f.History.Days
	if p == nil || !p.Recorded() || f.cramped() {
		return Scrolling{}
	}
	l := f.layOutDays(*p, f.top())
	if l.whole {
		return f.scrolledWhole(l.rows(), l.keyRows)
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
		return top, f.scrolledWhole(l.rows(), l.keyRows).Page
	}
	return l.accountsAt, l.scrollRow - l.accountsAt
}

// daysWeeks draws the weeks shown along Days' top row, each headed over it
// by its first day; and on the row under it, over this week's days, their
// initials, in text.faint.
func (f Frame) daysWeeks(c *canvas, l daysLayout) {
	weeks := l.page.Weeks[l.first/7:]
	for i, week := range weeks {
		x, this := l.x+i*weekCells, i == len(weeks)-1
		c.line(x, l.top, fitParts(weekCells-1, figurePart(weekHead(week, this), Number{})))
		if day, ok := dayfile.Civil(week); ok && this {
			initials(c, day, x, l.top+1)
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

// daysStrip draws a strip along row y, or on a phone, its name on row y and
// the strip under it: its name at column 0, as name gives it in the cells
// that leave a blank one before what follows it; each of its days shown, in
// the ink shade gives at the day's level, those to come ··; and its count,
// its number a figure, at column 136, or on a phone at the right, cut short
// of the edge.
func (f Frame) daysStrip(c *canvas, l daysLayout, y int, name func(room int) line, count []part, shade func(level int) ink, days []views.StripDay) {
	strip, room := y, l.x-1
	if l.phone {
		counted := fitParts(f.edge(), count...)
		strip, room = y+1, c.right(f.edge(), y, counted)-1
	} else {
		c.line(countAt(count), y, fitParts(f.edge()-countAt(count), count...))
	}
	c.line(0, y, name(room))
	coming := len(days) - l.page.ToCome
	for i := l.first; i < len(days); i++ {
		x := l.dayAt(i)
		if i >= coming {
			c.text(x, strip, toCome, faintInk)
			continue
		}
		c.line(x, strip, f.swatch(shade(days[i].Level), days[i].Level, days[i].Limit))
	}
}

// dayAt is the column the page's day with index i is drawn at, a week's days
// side by side and the weeks a cell apart.
func (l daysLayout) dayAt(i int) int {
	return l.x + (i-l.first)/7*weekCells + (i-l.first)%7*swatchCells
}

// countAt is the column a wide strip's count starts at: its first part, the
// number, right-aligned in daysCountCells from column 136.
func countAt(count []part) int {
	return daysCount + daysCountCells - partsWidth(count[:1])
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
