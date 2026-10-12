package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

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
// the eight.
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

// historyDays draws Days from row top: the weeks over the days, BY MODEL's
// strips and BY ACCOUNT's, its accounts scrolled as the frame has them, and
// the key on the view's last row; or, where nothing is recorded in its eight
// weeks, nothing recorded yet.
func (f Frame) historyDays(c *canvas, top int) {
	p := f.History.Days
	if p == nil || !p.Recorded() {
		c.line(0, top, line{{"nothing recorded yet", dimInk}})
		return
	}
	l := f.layOutDays(*p, top)
	f.daysWeeks(c, l)
	c.line(0, top+byModelRow, groupHead("BY MODEL", byModelSays, l.phone))
	for i, s := range p.ByModel {
		hue := familyHue(s.Of)
		f.daysStrip(c, l, l.familyRow(i), familyDot(s.Of, secondaryInk).fit, shareOfUse(s), func(level int) ink { return stepInk(hue, level) }, s.Days)
	}
	c.line(0, l.byAccount, groupHead("BY ACCOUNT", byAccountSays, l.phone))
	from, to := l.shown()
	for i, s := range p.ByAccount[from:to] {
		f.daysStrip(c, l, l.accountRow(i), func(room int) line { return historyAccount(labelled(s.Label, s.Of), s.Primary, room) }, f.limitsHit(s), dayInk, s.Days)
	}
	if above, below := from, len(p.ByAccount)-to; above+below > 0 {
		c.line(0, l.scrollRow, accountsOutOfView(above, below))
	}
	f.daysKey(c, l)
}

// daysLayout is Days' page laid out in a frame: in its phone form, where
// each strip's name takes the row over it; the row it starts at; the column
// its days start at, and the first of the page's days it shows; the rows BY
// ACCOUNT, its first strip and what's out of view are on, and its key's
// first; and how far its accounts are scrolled.
type daysLayout struct {
	page       views.DaysPage
	phone      bool
	top        int
	x, first   int
	byAccount  int
	accountsAt int
	scrollRow  int
	keyRow     int
	scrolled   int
}

// layOutDays lays out Days of the page given from row top: wide where it
// fits, else in its phone form.
func (f Frame) layOutDays(p views.DaysPage, top int) daysLayout {
	return f.daysLaidOut(p, top, f.phoneForm(func() int { return f.daysReach(p) }))
}

// daysLaidOut lays out Days of the page given from row top: in its phone
// form, the last three weeks under each strip's name, its key two rows; else
// every week from column 14 beside it, its key a row; the key on the view's
// last rows, and what's out of view of BY ACCOUNT on the row over them.
func (f Frame) daysLaidOut(p views.DaysPage, top int, phone bool) daysLayout {
	l := daysLayout{page: p, phone: phone, top: top, x: daysAt}
	keyRows := 1
	if l.phone {
		l.x, l.first, keyRows = 0, (len(p.Weeks)-daysPhoneWeeks)*7, 2
	}
	l.byAccount = l.familyRow(len(p.ByModel)-1) + byAccountGap
	l.accountsAt = l.byAccount + stripStride
	l.keyRow = top + f.viewRows() - keyRows
	l.scrollRow = l.keyRow - 1
	l.scrolled = min(max(f.Scroll, 0), l.scrolling().Most)
	return l
}

// daysReach is the column Days' wide form of the page given ends before,
// where its widest row ends: the weeks' heads, a group's heading, a strip's
// days or its count, whose number is right-aligned, so it ends where its
// words do, or the key. A strip's name is cut short of its days, and what's
// out of view of BY ACCOUNT never runs as far as a phone's width, so they're
// left out.
func (f Frame) daysReach(p views.DaysPage) int {
	l := daysLayout{page: p, x: daysAt}
	end := max(groupHead("BY MODEL", byModelSays, false).width(), groupHead("BY ACCOUNT", byAccountSays, false).width(), f.wideDaysKey().width())
	for i, week := range p.Weeks {
		end = max(end, daysAt+i*weekCells+weekHead(week, i == len(p.Weeks)-1).width())
	}
	for _, s := range p.ByModel {
		end = max(end, l.stripReach(s.Days, shareOfUse(s)))
	}
	for _, s := range p.ByAccount {
		end = max(end, l.stripReach(s.Days, f.limitsHit(s)))
	}
	return end
}

// stripReach is the column a wide strip of the days given ends before: its
// last day's, or its count's.
func (l daysLayout) stripReach(days []views.StripDay, count line) int {
	end := countAt(count) + count.width()
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
// over the blank row above what's out of view, at most six.
func (l daysLayout) scrolling() Scrolling {
	fit := min(max((l.scrollRow-2-l.accountsAt)/stripStride+1, 0), daysAccounts)
	accounts := len(l.page.ByAccount)
	return Scrolling{Most: max(accounts-fit, 0), Page: min(fit, accounts)}
}

// shown are the accounts BY ACCOUNT shows, as far as it's scrolled, by their
// indexes: from the first up to to.
func (l daysLayout) shown() (from, to int) {
	return l.scrolled, l.scrolled + l.scrolling().Page
}

// daysScrolling is how far Days' BY ACCOUNT scrolls, by accounts: zero where
// they fit, or there's nothing to draw.
func (f Frame) daysScrolling() Scrolling {
	p := f.History.Days
	if p == nil || !p.Recorded() || f.cramped() {
		return Scrolling{}
	}
	return f.layOutDays(*p, f.top()).scrolling()
}

// daysScrolls is what of Days scrolls, from row y, rows tall: BY ACCOUNT's
// strips, from the first's to the row over what's out of view, none where
// there's nothing to draw.
func (f Frame) daysScrolls() (y, rows int) {
	p := f.History.Days
	if p == nil || !p.Recorded() {
		return 0, 0
	}
	l := f.layOutDays(*p, f.top())
	return l.accountsAt, l.scrollRow - l.accountsAt
}

// daysWeeks draws the weeks shown along Days' top row, each headed over it
// by its first day; and on the row under it, over this week's days, their
// initials, in text.faint.
func (f Frame) daysWeeks(c *canvas, l daysLayout) {
	weeks := l.page.Weeks[l.first/7:]
	for i, week := range weeks {
		x, this := l.x+i*weekCells, i == len(weeks)-1
		c.line(x, l.top, weekHead(week, this))
		if day, ok := civilDay(week); ok && this {
			initials(c, day, x, l.top+1)
		}
	}
}

// initials draws the initials of the seven days from the one given, a day
// every swatchCells from x along row y, in text.faint.
func initials(c *canvas, from time.Time, x, y int) {
	for d := range 7 {
		c.text(x+d*swatchCells, y, dayInitial(from.AddDate(0, 0, d).Weekday()), faintInk)
	}
}

// daysStrip draws a strip along row y, or on a phone, its name on row y and
// the strip under it: its name at column 0, as name gives it in the cells
// that leave a blank one before what follows it; each of its days shown, in
// the ink shade gives at the day's level, those to come ··; and its count, at
// column 136, or on a phone at the right.
func (f Frame) daysStrip(c *canvas, l daysLayout, y int, name func(room int) line, count line, shade func(level int) ink, days []views.StripDay) {
	strip, room := y, l.x-1
	if l.phone {
		strip, room = y+1, c.right(f.edge(), y, count)-1
	} else {
		c.line(countAt(count), y, count)
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
func countAt(count line) int {
	return daysCount + daysCountCells - count[:1].width()
}

// swatch is a day swatchCells wide in its shade's ink, k, at its level: spaces
// on its colour; or where the look blends nothing, its level's glyph, in its
// colour. A day it hit a 5-hour limit has a red ▪ in its first cell.
func (f Frame) swatch(k ink, level int, limited bool) line {
	glyph, drawn := " ", ink{on: k.hue()}
	if !f.Look.blends() {
		glyph, drawn = shadeGlyph(level), k
	}
	if limited {
		return line{{limitMark, ink{token: theme.StateDestructive, on: drawn.on}}, {strings.Repeat(glyph, swatchCells-1), drawn}}
	}
	return line{{strings.Repeat(glyph, swatchCells), drawn}}
}

// shareOfUse is a family's share of the eight weeks' worth, bold
// text.secondary, or — where none of it is priced, then of use.
func shareOfUse(s views.DaysStrip) line {
	share := shareSaid(s.Share, strongInk).numbered(Number{Kind: NumberShareOfUse, Of: s.Of})
	return append(share, span{" of use", dimInk})
}

// limitsHit is how many 5-hour limits an account hit in the eight weeks,
// bold state.destructive, or text.faint at none, then 5-hour limits.
func (f Frame) limitsHit(s views.DaysStrip) line {
	count, noun := exhaustedInk, " 5-hour limits"
	switch s.Limits {
	case 0:
		count = ink{token: theme.TextFaint, bold: true}
	case 1:
		noun = " 5-hour limit"
	}
	return line{{strconv.Itoa(s.Limits), count.numbered(Number{Kind: NumberLimitsHit, Account: s.Of, Of: f.Policy.Started})}, {noun, dimInk}}
}

// daysKey draws Days' key from its row: the five shades from less to more,
// and what they are; a day it hit a 5-hour limit; and the days to come. On a
// phone, the shades take a row of their own, with nothing said of them.
func (f Frame) daysKey(c *canvas, l daysLayout) {
	shades, marks := f.daysKeyParts()
	if l.phone {
		c.line(0, l.keyRow, shades)
		c.line(0, l.keyRow+1, marks)
		return
	}
	c.line(0, l.keyRow, f.wideDaysKey())
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
