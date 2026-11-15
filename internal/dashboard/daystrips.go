package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The strips' cells: a day's; a week's, its days and the cell after them; the gap
// after the last week's, so a strip's count starts four cells after its last
// day; and the count's number's, right-aligned, its words after.
const (
	swatchCells    = 2
	weekCells      = 7*swatchCells + 1
	daysCountGap   = 2
	daysCountCells = 4
)

// stripStride is how many rows on from the one before each strip is.
const stripStride = 2

// The words the strips say: after MODELS BY DAY, what it shows; under the
// shades, of a day it hit a 5-hour limit, as Accounts' BY DAY marks one, and
// of the days to come.
const (
	byModelSays    = "every account, weighed at API prices, on one scale"
	daysShadesNote = "each shade a quarter of the active days"
	daysLimitNote  = "a day it hit a 5-hour limit"
	daysToComeNote = "days to come"
	toCome         = "··"
	limitMark      = "▪"
)

// The words after a strip's count, a family's share of use; and
// designedCountWords, the widest the count's column was reckoned at in the
// signed-off design, when the strips counted an account's 5-hour limits too.
// The column keeps that width so the strips lie where the design drew them:
// sized by ofUse alone, the narrow form's shares move 7 cells right, as on
// trends-year-73x61.
const (
	ofUse              = " of use"
	designedCountWords = " 5-hour limits"
)

// familyStrips draws BY MODEL's strips, as laid out: a family's each, its
// dot and name, its days in steps of its colour, and its share of use.
func (f Frame) familyStrips(c *canvas, l daysLayout) {
	for i, s := range l.page.ByModel {
		hue := familyHue(s.Of)
		named := familyDot(s.Of, secondaryInk)
		f.daysStrip(c, l, l.familyRow(i), func(room int) line { return named.fit(room) }, shareOfUse(s), func(level int) ink { return stepInk(hue, level) }, s.Days)
	}
}

// daysLayout is Days' page laid out in a frame: in its narrow form, where
// each strip's name takes the row over it; the row it starts at; where a
// strip's row lays its parts, as daysRow has them, and the first of the
// page's days it shows; how many rows under its top BY MODEL's first strip
// is; and its key's first row, and how many rows the key takes.
type daysLayout struct {
	page    views.DaysPage
	narrow  bool
	top     int
	row     daysRow
	first   int
	strips  int
	keyRow  int
	keyRows int
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

// daysWideFits reports whether Days' wide form of the page fits the frame,
// as flex lays its rows short of the edge: a strip's, a week at least
// beside its label column, and the key given, whole on one row, its widest,
// so the form never hangs on the data.
func (f Frame) daysWideFits(p views.DaysPage, key line) bool {
	keyed := key.width()
	_, strips := flex.Lay(f.edge(), daysParts(p, false)...)
	_, held := flex.Lay(f.edge(), flex.Part{Least: keyed, Natural: keyed})
	return strips && held
}

// daysStripped is Days of the page given laid out in the frame from row
// top, in its narrow form where narrow says, else wide: where a strip's row
// lays its parts, the first day its weeks shown begin with, and BY MODEL's
// first strip strips rows under its top.
func (f Frame) daysStripped(p views.DaysPage, top int, narrow bool, strips int) daysLayout {
	l := daysLayout{page: p, narrow: narrow, top: top, row: f.daysRowOf(p, narrow), strips: strips}
	l.first = max(len(p.Weeks)-len(l.row.weeks), 0) * 7
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

// daysCount is a strip's count, as wide as the design reckoned it, so its
// columns never hang on the data: its number, then its words, a family's
// share of use.
func daysCount() flex.Part {
	cells := daysCountCells + max(ansi.StringWidth(ofUse), ansi.StringWidth(designedCountWords))
	return flex.Part{Least: cells, Natural: cells}
}

// daysLabels is the strips' label column, nameGap after it, as namesColumn
// lays it out: a family's name, its dot among it; narrowing only in the turn
// after the last of the weeks given could give way, so names read whole
// before a week is added.
func daysLabels(p views.DaysPage, weeks int) flex.Part {
	var names []columnName
	for _, s := range p.ByModel {
		names = append(names, columnName{label: familyDot(s.Of, secondaryInk).width()})
	}
	labels := namesColumn(nameGap, names...)
	labels.Narrows = weeks
	return labels
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
	return l.top + l.strips + stripStride*i
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
// the cells the row lays it out in; each of its days shown, in the ink shade
// gives at the day's level, those to come ··; and its count, its number a
// figure, cut short of the edge.
func (f Frame) daysStrip(c *canvas, l daysLayout, y int, name func(room int) line, count []part, shade func(level int) ink, days []views.StripDay) {
	strip := l.stripRow(y)
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

// stripRow is the row the days of the strip named on row y are on: y, or in
// the narrow form, the row under it.
func (l daysLayout) stripRow(y int) int {
	if l.narrow {
		return y + 1
	}
	return y
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
// first of two, its only one at one.
func (f Frame) daySwatch(k ink, level int, limited bool, cells int) line {
	glyph, drawn := " ", ink{on: k.hue()}
	if !f.Look.blends() {
		glyph, drawn = shadeGlyph(level), k
	}
	if !limited {
		return line{{strings.Repeat(glyph, cells), drawn}}
	}
	before := (cells - 1) / 2
	marked := line{{limitMark, ink{token: theme.StateDestructive, on: drawn.on}}}
	if after := cells - 1 - before; after > 0 {
		marked = append(marked, span{strings.Repeat(glyph, after), drawn})
	}
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

// daysShades are the five shades of Days' key, from less to more.
func (f Frame) daysShades() line {
	shades := line{{"less ", dimInk}}
	for level := range shadeLevels {
		if level > 0 {
			shades = append(shades, span{" ", ink{}})
		}
		shades = append(shades, f.swatch(dayInk(level), level, false)...)
	}
	return append(shades, span{" more", dimInk})
}

// daysShadesSaid are the shades of Days' key, and what they are.
func (f Frame) daysShadesSaid() line {
	return append(f.daysShades(), span{"  " + daysShadesNote, faintInk})
}

// daysToCome is the mark of Days' key for the days to come, and what it
// means.
func daysToCome() line {
	return line{{toCome, faintInk}, {" " + daysToComeNote, dimInk}}
}
