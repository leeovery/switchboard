package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// A week's cells across the frame: its first day's widest and its peak's,
// weeksApart after the one before, a table's peak right-aligned in
// peakCells.
const (
	weekColumnCells = 6
	weeksApart      = 2
	peakCells       = 5
)

// A week's column: columnCells wide, each row mixed columnMix from the
// canvas toward its colour, this week's thisWeekMix.
const (
	columnCells = 5
	columnMix   = 0.75
	thisWeekMix = 0.45
)

// atItsCap keys a peak drawn as one that hit its limit, as the verdicts
// count one, in the help cards.
const atItsCap = "at its cap"

// peakPart is a week's peak as a percentage, a figure: in its level's colour
// as usage colours one, but bold only at its cap, in state.destructive; this
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
	case p.AtCap:
		return figureIn(text, exhaustedInk)
	}
	return figureIn(text, usedInk(*p.Share, ink{token: theme.VizRamp1}))
}

// peakColumn draws a week's column, filled from row bottom up to its peak as
// a week's peak is written, a whole percent, rows tall at a full week, cells wide,
// its top cell in eighths, as look draws its cells: each row in the ramp's
// colour at its middle, mixed from the canvas toward it; a week of no use
// ▁ in text.faint along the bottom; and one whose peak isn't known,
// nothing. Compare's WEEKS and Detail's are drawn so.
func peakColumn(c *canvas, look Look, x, bottom, rows, cells int, p views.WeekPeak) {
	if p.Share == nil {
		return
	}
	if *p.Share <= 0 {
		c.text(x, bottom, strings.Repeat(levels[1], cells), faintInk)
		return
	}
	filled := halfEven(min(float64(prose.WholePercent(*p.Share))/100, 1) * float64(rows) * 8)
	for r := range rows {
		eighths := min(max(filled-8*r, 0), 8)
		if eighths == 0 {
			continue
		}
		glyph, k := weekCell(look, eighths, (float64(r)+0.5)/float64(rows), p.Open)
		c.text(x, bottom-r, strings.Repeat(glyph, cells), k)
	}
}

// weekCell is a cell of a week's column's row filled eighths of it, 1 to
// 8, at t up the ramp, as look draws it, mixed columnMix from the canvas
// toward the ramp's colour there, or this week's thisWeekMix: where it
// blends, a filled cell is a space on that colour and a part-filled one its
// eighth block in it; where it doesn't, the fill is in blocks, █ and the
// eighths, this week's dim.
func weekCell(look Look, eighths int, t float64, thisWeek bool) (string, ink) {
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
