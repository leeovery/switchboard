package dashboard

import (
	"math"
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// blocks fill a cell by eighths, from none of it to all of it.
var blocks = [...]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

const (
	trackCell = "░"
	// paceMark marks where even use across a window would put it.
	paceMark = "┃"
	// capMark marks where an account's cap, its reserve, starts.
	capMark = "╎"
)

// along is how far along a bar width cells long its cell i sits, from 0 at
// the first cell to 1 at the last: a bar of one cell, at 0.
func along(i, width int) float64 {
	return float64(i) / float64(max(width-1, 1))
}

// eighthsBar draws a bar from x along row y, eighths eighths of a cell long,
// its cells split among parts worth the amounts given, in their order, as
// splitCells splits them, each in its hue: its full cells as stackedBar draws
// them; and its last, where it's part-filled, its eighth block in its part's
// hue, on what's under it.
func eighthsBar(c *canvas, look Look, x, y, eighths int, amounts []ledger.Picodollars, hues []hue) {
	cells := (eighths + 7) / 8
	runs := splitCells(amounts, cells)
	part, last := eighths%8, lastRun(runs)
	if part == 0 || last < 0 {
		stackedBar(c, look, x, y, runs, hues)
		return
	}
	runs[last]--
	stackedBar(c, look, x, y, runs, hues)
	c.text(x+cells-1, y, blocks[part], hues[last].ink())
}

// lastRun is the index of the last of the runs with any cells: -1 where
// none has.
func lastRun(runs []int) int {
	for i, n := range slices.Backward(runs) {
		if n > 0 {
			return i
		}
	}
	return -1
}

// barEighths is how many eighths of cells cells a worth of w takes against
// the largest, a half rounded to the even: none of none, else at least one.
func barEighths(w, largest ledger.Picodollars, cells int) int {
	if w <= 0 || largest <= 0 {
		return 0
	}
	return max(1, int(math.RoundToEven(float64(cells*8)*float64(w)/float64(largest))))
}
