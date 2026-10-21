package dashboard

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
