package dashboard

import "math"

// blocks fill a cell by eighths, from none of it to all of it.
var blocks = [...]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

const (
	trackCell = "░"
	// noMarker leaves a marker off a bar.
	noMarker = -1
)

var (
	// paceMarker marks where even use across a window would put it.
	paceMarker = span{"┃", markerInk}
	// reserveMarker marks where an account's reserve starts.
	reserveMarker = span{"╎", warningInk}
)

// bar draws fraction as a bar width cells long, a span a cell: filled in
// eighths of a cell, each filled cell coloured by how far along the ramp it
// sits. A fraction over 1 fills the bar.
func bar(fraction float64, width int) line {
	filled := int(math.Round(min(max(fraction, 0), 1) * float64(8*width)))
	l := make(line, width)
	for i := range l {
		if eighths := filled - 8*i; eighths > 0 {
			l[i] = span{blocks[min(eighths, 8)], ink{color: rampAt(along(i, width))}}
		} else {
			l[i] = span{trackCell, trackInk}
		}
	}
	return l
}

// mark draws marker over the cell of a bar, which is a span a cell, at cell.
// A cell off the bar, as noMarker is, leaves the bar as it is.
func (l line) mark(cell int, marker span) line {
	if 0 <= cell && cell < len(l) {
		l[cell] = marker
	}
	return l
}

// along is how far along a bar width cells long its cell i sits, from 0 at
// the first cell to 1 at the last.
func along(i, width int) float64 {
	if width < 2 {
		return 0
	}
	return float64(i) / float64(width-1)
}

// cellAt is the cell of a bar width cells long that fraction of the bar ends
// in, such as the share of its window that has passed, where use would reach
// if it kept pace.
func cellAt(fraction float64, width int) int {
	return min(int(fraction*float64(width)), width-1)
}

// reserveCell is the cell of a bar width cells long where the reserve starts,
// at 1 − reserve of the window, or noMarker without a reserve.
func reserveCell(reserve float64, width int) int {
	if reserve <= 0 {
		return noMarker
	}
	return cellAt(1-reserve, width)
}
