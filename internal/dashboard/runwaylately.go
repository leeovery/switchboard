package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// latelyPlace is where Runway's LATELY goes, as latelyPlaceOf has it.
type latelyPlace int

const (
	// comingAlone is COMING UP alone, as comingColumns lays it, no LATELY.
	comingAlone latelyPlace = iota
	// latelyStacked is LATELY under COMING UP, a blank row between, from the
	// first column: COMING UP in the columns comingColumns lays, LATELY as
	// wide as they span, or both in one column, as wide as stackedWidth has
	// it, where COMING UP takes one.
	latelyStacked
	// latelySideBySide is LATELY from the first column, as wide as the
	// Overview's LATELY column, and COMING UP as wide as the Overview's
	// COMING UP column, ending at the frame's edge.
	latelySideBySide
)

// latelyLeastLines are the fewest lines LATELY shows stacked under COMING UP.
const latelyLeastLines = 2

// agoGap is the fewest blank cells between a LATELY line's words and how long
// ago it was.
const agoGap = 2

// runwayLatelyLeast is the fewest cells a column of Runway's LATELY takes: a
// line's time at its widest, as prose.Befell says it, the blanks after it,
// an account's name at its least, as nameLeast has it, its fewest words, as
// views.OpenAgain has them, and how long ago at its widest, agoGap before it.
func runwayLatelyLeast() int {
	return prose.BefellWidth() + len(afterTime) + nameLeast() + len(" "+views.OpenAgain) + agoGap + len(prose.AgoWidest())
}

// latelyColumn is a column of Runway's LATELY, as flex lays it: a whole line
// at the least, as runwayLatelyLeast has it, growing past it.
func latelyColumn() flex.Part {
	least := runwayLatelyLeast()
	return flex.Part{Least: least, Natural: least, Grows: true}
}

// latelyPlaceOf is where LATELY goes on the frame, as flex lays its column
// and COMING UP's: beside COMING UP where the Overview's COMING UP column,
// comingWidth wide, holds comingColumn, and its LATELY column, from latelyAt
// to the edge, holds latelyColumn; else stacked, where the frame's width
// holds latelyColumn; else nowhere, COMING UP alone.
func (f Frame) latelyPlaceOf() latelyPlace {
	lately := latelyColumn()
	_, comingBeside := flex.Lay(comingWidth(f.Width), comingColumn)
	_, latelyBeside := flex.Lay(f.Width-latelyAt(f.Width), lately)
	_, latelyUnder := flex.Lay(f.Width, lately)
	switch {
	case comingBeside && latelyBeside:
		return latelySideBySide
	case latelyUnder:
		return latelyStacked
	}
	return comingAlone
}

// runwayColumns are the columns COMING UP's lines and LATELY's take at the
// moment m, where the fit places LATELY: beside COMING UP, LATELY as wide as
// the Overview's LATELY column from the first, and COMING UP as wide as the
// Overview's COMING UP column, ending at the frame's edge; stacked, COMING
// UP's as comingColumns lays them, and LATELY from the first, as wide as
// stackedWidth has it, or as COMING UP spans where that's wider, COMING
// UP's one column as wide as LATELY where it has one; else COMING UP's as
// comingColumns lays them, and LATELY none.
func (f Frame) runwayColumns(m status.Moment, fit runwayFit) (coming []flex.Span, lately flex.Span) {
	switch fit.place {
	case latelySideBySide:
		width := comingWidth(f.Width)
		return []flex.Span{{At: f.Width - width, Size: width}}, flex.Span{Size: f.Width - latelyAt(f.Width)}
	case latelyStacked:
		coming = f.comingColumns()
		last := coming[len(coming)-1]
		lately = flex.Span{Size: max(f.stackedWidth(m, fit), last.At+last.Size)}
		if len(coming) == 1 {
			coming[0] = lately
		}
		return coming, lately
	}
	return f.comingColumns(), flex.Span{}
}

// stackedWidth is the width COMING UP and LATELY share stacked at the moment
// m, as fitted: what their lines need to read whole, never under
// runwayComingWidth, or the frame's width where that's narrower, nor over
// the frame's width. A line of COMING UP needs its words and its countdown,
// as comingLines draws them; a line of LATELY shown, its short form beside
// how long ago it was, as runwayLatelyLines lays it.
func (f Frame) stackedWidth(m status.Moment, fit runwayFit) int {
	need, things := runwayComingWidth, f.runwayHappenings(m)
	column := whenColumn(things, m.Now)
	for _, h := range things {
		need = max(need, h.said(m.Now, column).width()+comingTail)
	}
	for _, l := range runwayLatelyParts(m, f.latelyShown(fit)) {
		need = max(need, l.least())
	}
	return min(need, f.Width)
}

// latelyShown are the sentences Runway's LATELY shows as fitted: as many as
// it has rows for.
func (f Frame) latelyShown(fit runwayFit) []views.Sentence {
	return f.Runway.Lately[:min(max(fit.latelyLines, 0), len(f.Runway.Lately))]
}

// runwayLately draws LATELY, where the fit gives it a row: its heading, in
// text.subtle, and under it, on the rows it's given, the sentences Runway's
// LATELY tells, as runwayLatelyLines lays them, in its column as laid out;
// or, where there are none, why there are none, as the Overview's LATELY
// says it.
func (f Frame) runwayLately(c *canvas, laid runwayLaid) {
	fit, column := laid.fit, laid.lately
	if !fit.draws(fit.lately) {
		return
	}
	c.line(column.At, fit.lately, line{{SectionLately.heading(), dimInk}})
	if fit.latelyLines == 0 {
		return
	}
	m, sentences := laid.m, f.latelyShown(fit)
	if len(sentences) == 0 {
		f.quietly(c, m, column.At, fit.lately+1, column.At+column.Size)
		return
	}
	for i, l := range runwayLatelyLines(m, sentences, column.Size) {
		c.line(column.At, fit.lately+1+i, l)
	}
}

// latelyParts are a line of Runway's LATELY in parts: its words in full and
// short, each led by its time, and how long ago it was.
type latelyParts struct {
	full, short, ago line
}

// least is the fewest cells the line reads whole in: its short form, agoGap
// and how long ago.
func (l latelyParts) least() int {
	return l.short.width() + agoGap + l.ago.width()
}

// runwayLatelyParts are the sentences given as Runway's LATELY lines at m's
// moment, in parts: each its time, as prose.Befell says it, in a column as
// wide as the widest shown, in text.secondary, no mark after it; then its
// words, as saidLine draws them, in full and short; and how long ago, as
// prose.Ago says it, in text.subtle.
func runwayLatelyParts(m status.Moment, sentences []views.Sentence) []latelyParts {
	names, column := namesOf(m), 0
	for _, s := range sentences {
		column = max(column, len(prose.Befell(m.Now, s.At)))
	}
	parts := make([]latelyParts, len(sentences))
	for i, s := range sentences {
		lead := line{{padded(prose.Befell(m.Now, s.At), column) + afterTime, secondaryInk}}
		led := func(said views.Said) line { return slices.Concat(lead, saidLine(m, names, said)) }
		parts[i] = latelyParts{full: led(s.Full), short: led(s.Short), ago: line{{prose.Ago(m.Now, s.At), dimInk}}}
	}
	return parts
}

// runwayLatelyLines are the sentences given as Runway's LATELY lines at m's
// moment, in cells cells, as runwayLatelyParts has them: its words in full
// where they fit whole beside how long ago it was, else short, as firstFit
// has it; and how long ago, ending at the last cell.
func runwayLatelyLines(m status.Moment, sentences []views.Sentence, cells int) []line {
	parts := runwayLatelyParts(m, sentences)
	lines := make([]line, len(parts))
	for i, l := range parts {
		words := firstFit(cells-l.ago.width()-agoGap, l.full, l.short)
		lines[i] = slices.Concat(words, line{spaces(cells - words.width() - l.ago.width())}, l.ago)
	}
	return lines
}
