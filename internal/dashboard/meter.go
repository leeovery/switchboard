package dashboard

import (
	"math"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The parts of a window's bar line on a card: its label, in a column of at
// least labelColumn cells with the gap after it; its bar; its use,
// right-aligned in useColumn cells with the gap before it; and in brief where
// it's heading, in whitherColumn cells with the gap before it.
const (
	labelColumn   = 9
	useColumn     = 5
	whitherColumn = 12
	// barLine is the fewest cells a bar line takes, as a 50-column card's
	// inside, and barLinesGap the blank cells between bar lines side by side
	// on a card wide enough for more than one to a row.
	barLine     = 44
	barLinesGap = 4
)

const (
	// rampSteps are the steps a bar's cells take along the ramp: a cell's
	// colour is the ramp's at the nearest eighth of the way along.
	rampSteps = 8
	// projectionFade fades a bar's projection, and a chart's level, halfway
	// into the canvas.
	projectionFade = 0.5
	// shadeCell stands for a bar's projection in a look that can't fade it.
	shadeCell = "▒"
)

// meter draws a window as it stands at now as a bar line, width cells from x
// along row y, its label labelled cells wide: its label, short; a bar of its
// use, its projection beyond, where even pace would be, and where its
// account's reserve starts, at reserve of it, none for none; its use; and in
// brief where it's heading.
func (f Frame) meter(c *canvas, s standing, reserve float64, now time.Time, x, y, width, labelled int) {
	cells := width - labelled - useColumn - whitherColumn
	c.line(x, y, line{{truncate(status.Short(s.window.Label), labelled-1), textInk}})
	if cells < 1 {
		return
	}
	f.bar(c, s, reserve, now, x+labelled, y, cells)
	c.right(x+labelled+cells+useColumn, y, line{s.use(titleInk)})
	c.line(x+labelled+cells+useColumn+1, y, line{s.whither(now, "")}.fit(whitherColumn-1))
}

// bar draws a window standing as s at now as a bar cells long from x along
// row y: its use, as eased has it, its projection beyond, where even pace
// would be, and where its account's reserve, reserve of it, starts.
func (f Frame) bar(c *canvas, s standing, reserve float64, now time.Time, x, y, cells int) {
	share, ok := s.projected()
	if !ok {
		share = 0
	}
	f.fill(c, x, y, cells, f.eased(s.account, s.window), share, reserveCell(reserve, cells), s.paceCell(now, cells))
}

// eased is how much of the account with the given id's window w its bar
// fills: as far as the bars have eased to its reading, while they ease, else
// as read.
func (f Frame) eased(id string, w quota.Window) float64 {
	if used, ok := f.Eased[Ref{Account: id, Window: w.Key}]; ok {
		return used
	}
	return w.Utilization
}

// fill draws a bar cells long from x along row y: used filled in eighths, each
// filled cell coloured by where it sits along the ramp, to the nearest
// eighth; heading's share beyond it, where that's more, in the ramp's colours
// faded halfway, or where the look can't fade them, in shade; the empty
// cells' track; and over them, the reserve's mark at the cell reserve, and
// the pace marker at the cell pace, noMarker leaving either off.
func (f Frame) fill(c *canvas, x, y, cells int, used, heading float64, reserve, pace int) {
	filled := eighths(used, cells)
	projected := max(eighths(heading, cells), filled)
	fades := f.Look.blends()
	for i := range cells {
		at := math.Round(along(i, cells)*rampSteps) / rampSteps
		solid := hue{ramp: true, at: at}
		faint := hue{ramp: true, at: at, fade: projectionFade}
		switch from := 8 * i; {
		case filled >= from+8:
			c.text(x+i, y, blocks[8], ink{ramp: true, at: at})
		case filled > from && projected > filled && fades:
			c.text(x+i, y, blocks[filled-from], ink{ramp: true, at: at, on: faint})
		case filled > from:
			c.text(x+i, y, blocks[filled-from], ink{ramp: true, at: at})
		case projected > from && fades:
			c.text(x+i, y, blocks[min(projected-from, 8)], ink{ramp: true, at: at, fade: projectionFade})
		case projected > from:
			c.text(x+i, y, shadeCell, ink{ramp: true, at: at})
		default:
			c.text(x+i, y, trackCell, trackInk)
		}
		under := coveredBy(i, filled, projected, solid, faint)
		if i == reserve {
			c.text(x+i, y, reserveMarker.text, notched(under, fades))
		}
		if i == pace {
			c.text(x+i, y, paceMarker.text, ink{token: theme.VizPace, bold: true, on: under})
		}
	}
}

// eighths is how many eighths of a cell share fills of a bar cells long, a
// share over 1 filling it.
func eighths(share float64, cells int) int {
	return int(math.Round(min(max(share, 0), 1) * float64(8*cells)))
}

// coveredBy is the colour a bar's cell i is covered by, as a marker over it
// takes for its surface: solid where its fill covers it, faint where its
// projection does, or where its fill ends in it and its projection goes on;
// the zero hue, the canvas, where the track shows, or a part-filled cell
// ends the bar's colour.
func coveredBy(i, filled, projected int, solid, faint hue) hue {
	switch from := 8 * i; {
	case filled >= from+8:
		return solid
	case filled > from && projected > filled:
		return faint
	case filled <= from && projected >= from+8:
		return faint
	default:
		return hue{}
	}
}

// notched is how the reserve's mark is drawn on a cell covered by under: a
// notch, in the canvas's colour, where the cell's colour shows; else in
// viz.reserve, on the canvas.
func notched(under hue, fades bool) ink {
	if under == (hue{}) || (under.fade >= 0.5 && !fades) {
		return reserveInk
	}
	return ink{token: theme.Canvas, on: under}
}
