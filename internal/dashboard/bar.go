package dashboard

// blocks fill a cell by eighths, from none of it to all of it.
var blocks = [...]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

const (
	trackCell = "░"
	// noMarker leaves a marker off a bar.
	noMarker = -1
)

var (
	// paceMarker marks where even use across a window would put it.
	paceMarker = span{"┃", paceInk}
	// reserveMarker marks where an account's reserve starts.
	reserveMarker = span{"╎", reserveInk}
)

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
