package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// Days' columns: where its days start, a day's cells and a week's, its days'
// and the cell after them; and where a strip's count, its share of use or
// its 5-hour limits, starts, right-aligned in countCells.
const (
	daysAt      = 14
	swatchCells = 2
	weekCells   = 7*swatchCells + 1
	daysCount   = 136
	countCells  = 4
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
// within itself; and the weeks a phone shows, the last of the eight.
const (
	daysAccounts = 6
	phoneWeeks   = 3
)

// The words Days says: under the shades, of a day it hit a 5-hour limit and
// of the days to come; and how to scroll to the accounts out of view.
const (
	daysShadesNote = "each shade a quarter of the active days"
	daysLimitNote  = "a day it hit a 5-hour limit"
	daysToComeNote = "days to come"
	daysToScroll   = " or the wheel to scroll them"
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
	c.line(0, top+byModelRow, groupHead("BY MODEL", "every account, weighed at API prices, on one scale", l.phone))
	for i, s := range p.ByModel {
		hue := familyHue(s.Of)
		f.daysStrip(c, l, l.familyRow(i), familyName(s.Of), shareOfUse(s), func(level int) ink { return stepInk(hue, level) }, s.Days)
	}
	c.line(0, l.byAccount, groupHead("BY ACCOUNT", "the share of its week each day used, on one scale", l.phone))
	from, to := l.shown()
	for i, s := range p.ByAccount[from:to] {
		f.daysStrip(c, l, l.accountRow(i), line{{status.Clean(s.Of), titleInk}}, f.limitsHit(s), dayInk, s.Days)
	}
	if above, below := from, len(p.ByAccount)-to; above+below > 0 {
		c.line(0, l.scrollRow, accountsOutOfView(above, below))
	}
	f.daysKey(c, l)
}

// daysLayout is Days' page laid out in a frame: on a phone, where each
// strip's name takes the row over it; the row it starts at; the column its
// days start at, and the first of the page's days it shows; the rows BY
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

// layOutDays lays out Days of the page given from row top: on a phone, the
// last three weeks under each strip's name, its key two rows; else every
// week from column 14 beside it, its key a row; the key on the view's last
// rows, and what's out of view of BY ACCOUNT on the row over them.
func (f Frame) layOutDays(p views.DaysPage, top int) daysLayout {
	l := daysLayout{page: p, phone: f.phone(), top: top, x: daysAt}
	keyRows := 1
	if l.phone {
		l.x, l.first, keyRows = 0, (len(p.Weeks)-phoneWeeks)*7, 2
	}
	l.byAccount = l.familyRow(len(p.ByModel)-1) + byAccountGap
	l.accountsAt = l.byAccount + stripStride
	l.keyRow = top + f.viewRows() - keyRows
	l.scrollRow = l.keyRow - 1
	l.scrolled = min(max(f.Scroll, 0), l.scrolling().Most)
	return l
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

// groupHead is a group's heading, in text.subtle, and after it, but on a
// phone, what's on one scale, in text.faint.
func groupHead(name, scale string, phone bool) line {
	if phone {
		return line{{name, dimInk}}
	}
	return line{{name, dimInk}, {"  " + scale, faintInk}}
}

// daysWeeks draws the weeks shown along Days' top row: each week's first day
// over it, in text.faint, this week's in bold text.secondary; and on the row
// under it, over this week's days, their initials, in text.faint.
func (f Frame) daysWeeks(c *canvas, l daysLayout) {
	weeks := l.page.Weeks[l.first/7:]
	for i, week := range weeks {
		day, err := time.Parse(time.DateOnly, week)
		if err != nil {
			continue
		}
		x, k := l.x+i*weekCells, faintInk
		if i == len(weeks)-1 {
			k = strongInk
			initials(c, day, x, l.top+1)
		}
		c.text(x, l.top, day.Format("2 Jan"), k)
	}
}

// initials draws the initials of the seven days from the one given, a day
// every swatchCells from x along row y, in text.faint.
func initials(c *canvas, from time.Time, x, y int) {
	for d := range 7 {
		c.text(x+d*swatchCells, y, from.AddDate(0, 0, d).Weekday().String()[:1], faintInk)
	}
}

// daysStrip draws a strip along row y, or on a phone, its name on row y and
// the strip under it: its name at column 0, cut to leave a blank cell before
// what follows it; each of its days shown, in the ink shade gives at the
// day's level, those to come ··; and its count, at column 136, or on a phone
// at the right.
func (f Frame) daysStrip(c *canvas, l daysLayout, y int, name, count line, shade func(level int) ink, days []views.StripDay) {
	strip, room := y, l.x-1
	if l.phone {
		strip, room = y+1, c.right(f.edge(), y, count)-1
	} else {
		c.line(daysCount+countCells-count[:1].width(), y, count)
	}
	c.line(0, y, name.fit(room))
	coming := len(days) - l.page.ToCome
	for i := l.first; i < len(days); i++ {
		x := l.x + (i-l.first)/7*weekCells + (i-l.first)%7*swatchCells
		if i >= coming {
			c.text(x, strip, toCome, faintInk)
			continue
		}
		c.line(x, strip, f.swatch(shade(days[i].Level), days[i].Level, days[i].Limit))
	}
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
	share := span{"—", faintInk}
	if s.Share != nil {
		share = span{status.Percent(*s.Share), strongInk}
	}
	return line{share.numbered(Number{Kind: NumberShareOfUse, Of: s.Of}), {" of use", dimInk}}
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

// accountsOutOfView says how many of BY ACCOUNT's accounts are out of view
// above and below, and how to scroll to them: "▼ 4 more accounts below   ·
// j/k or the wheel to scroll them".
func accountsOutOfView(above, below int) line {
	var l line
	if above > 0 {
		l = line{{"▲", keyMarkInk}, {" " + moreAccounts(above) + " above", mutedInk}, {"   ·   ", faintInk}}
	}
	if below > 0 {
		return append(l, moreBelow(below, "account", daysToScroll)...)
	}
	return append(l, span{"j/k", keyInk}, span{daysToScroll, mutedInk})
}

// daysKey draws Days' key from its row: the five shades from less to more,
// and what they are; a day it hit a 5-hour limit; and the days to come. On a
// phone, the shades take a row of their own, with nothing said of them.
func (f Frame) daysKey(c *canvas, l daysLayout) {
	shades := line{{"less ", dimInk}}
	for level := range shadeLevels {
		if level > 0 {
			shades = append(shades, span{" ", ink{}})
		}
		shades = append(shades, f.swatch(dayInk(level), level, false)...)
	}
	shades = append(shades, span{" more", dimInk})
	marks := slices.Concat(f.swatch(dayInk(2), 2, true), line{{" " + daysLimitNote, dimInk}, {"   ", ink{}}, {toCome, faintInk}, {" " + daysToComeNote, dimInk}})
	if l.phone {
		c.line(0, l.keyRow, shades)
		c.line(0, l.keyRow+1, marks)
		return
	}
	c.line(0, l.keyRow, slices.Concat(shades, line{{"  " + daysShadesNote, faintInk}, {"    ", ink{}}}, marks))
}

// familyName is a family's name as Days shows it, its dot in its hue, then
// its name, its first letter capital: ● Opus.
func familyName(family string) line {
	name := family
	if first, size := utf8.DecodeRuneInString(family); size > 0 {
		name = string(unicode.ToUpper(first)) + family[size:]
	}
	return line{{"●", familyHue(family).ink()}, {" " + status.Clean(name), secondaryInk}}
}
