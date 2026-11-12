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
	// latelyStacked is LATELY under COMING UP, a blank row between, both in
	// one column of runwayComingWidth from the first, or the frame's width
	// where that's narrower.
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
// line's time at its widest, the blanks after it, an account's name at its
// least, its fewest words, as views.OpenAgain has them, and how long ago at
// its widest, agoGap before it.
func runwayLatelyLeast() int {
	return prose.DatedWidth() + len(afterTime) + labelLeast + len(" "+views.OpenAgain) + agoGap + len(prose.AgoWidest())
}

// latelyPlaceOf is where LATELY goes on the frame: beside COMING UP where both
// columns hold a whole line, COMING UP its natural runwayComingWidth and
// LATELY its least; else stacked, where one column holds LATELY's least;
// else nowhere, COMING UP alone.
func (f Frame) latelyPlaceOf() latelyPlace {
	least := runwayLatelyLeast()
	switch {
	case comingWidth(f.Width) >= runwayComingWidth && f.Width-latelyAt(f.Width) >= least:
		return latelySideBySide
	case min(f.Width, runwayComingWidth) >= least:
		return latelyStacked
	}
	return comingAlone
}

// latelyColumn is the column LATELY's lines take in the place given: beside
// COMING UP, the Overview's LATELY column's width from the first; stacked,
// COMING UP's one column.
func (f Frame) latelyColumn(place latelyPlace) flex.Span {
	if place == latelySideBySide {
		return flex.Span{Size: f.Width - latelyAt(f.Width)}
	}
	return f.comingColumns(place)[0]
}

// runwayLately draws LATELY, where the fit gives it a row: its heading, in
// text.subtle, and under it, on the rows it's given, the sentences Runway's
// LATELY tells, as runwayLatelyLines lays them; or, where there are none, why
// there are none, as the Overview's LATELY says it.
func (f Frame) runwayLately(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.lately) {
		return
	}
	column := f.latelyColumn(fit.place)
	c.line(column.At, fit.lately, line{{SectionLately.heading(), dimInk}})
	if fit.latelyLines == 0 {
		return
	}
	m, sentences := laid.m, f.Runway.Lately[:min(fit.latelyLines, len(f.Runway.Lately))]
	if len(sentences) == 0 {
		f.quietly(c, m, column.At, fit.lately+1, column.At+column.Size)
		return
	}
	for i, l := range runwayLatelyLines(m, sentences, column.Size) {
		c.line(column.At, fit.lately+1+i, l)
	}
}

// runwayLatelyLines are the sentences given as Runway's LATELY lines at m's
// moment, in cells cells: each its time, in a column as wide as the widest,
// in text.secondary, no mark after it; then its words, as saidLine draws
// them, in its full form where that fits whole beside how long ago it was,
// else its short form, as firstFit has it; and how long ago, as prose.Ago
// says it, ending at the last cell, in text.subtle.
func runwayLatelyLines(m status.Moment, sentences []views.Sentence, cells int) []line {
	names, column := namesOf(m), 0
	for _, s := range sentences {
		column = max(column, len(prose.Dated(m.Now, s.At)))
	}
	lines := make([]line, len(sentences))
	for i, s := range sentences {
		lead := line{{padded(prose.Dated(m.Now, s.At), column) + afterTime, secondaryInk}}
		led := func(said views.Said) line { return slices.Concat(lead, saidLine(m, names, said)) }
		ago := line{{prose.Ago(m.Now, s.At), dimInk}}
		words := firstFit(cells-ago.width()-agoGap, led(s.Full), led(s.Short))
		lines[i] = slices.Concat(words, line{spaces(cells - words.width() - ago.width())}, ago)
	}
	return lines
}
