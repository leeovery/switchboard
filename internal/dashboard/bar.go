package dashboard

import "math"

// blocks fill a cell by eighths, from none of it to all of it.
var blocks = [...]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

const (
	trackCell  = "░"
	markerCell = "┃"
	// noMarker leaves the pace marker off a bar.
	noMarker = -1
)

// bar draws fraction as a bar width cells long: filled in eighths of a cell,
// each filled cell coloured by how far along the ramp it sits, with the pace
// marker drawn over the cell at marker. A fraction over 1 fills the bar.
func bar(fraction float64, marker, width int) line {
	filled := int(math.Round(min(max(fraction, 0), 1) * float64(8*width)))
	l := make(line, width)
	for i := range l {
		if eighths := filled - 8*i; eighths > 0 {
			l[i] = span{blocks[min(eighths, 8)], ink{color: rampAt(along(i, width))}}
		} else {
			l[i] = span{trackCell, trackInk}
		}
	}
	if 0 <= marker && marker < width {
		l[marker] = span{markerCell, markerInk}
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

// paceCell is the cell of a bar width cells long that marks elapsed, the share
// of its window that has passed: where use would reach if it kept pace.
func paceCell(elapsed float64, width int) int {
	return min(int(elapsed*float64(width)), width-1)
}
