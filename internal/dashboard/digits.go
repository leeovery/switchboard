package dashboard

import (
	"fmt"
	"time"
)

// figures are the big digits' characters, each five pixels tall and as wide
// as its rows, a pixel lit where a row has a 1: the digits, the per cent sign
// and the colon a timer reads by.
var figures = map[rune][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "001", "001", "001"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	'%': {"101", "001", "010", "100", "101"},
	':': {"0", "1", "0", "1", "0"},
}

// bigRows are the cells big digits stand in: three rows, two pixels to a
// cell, the last row's lower half blank.
const bigRows = 3

// big draws text in big digits from x along rows y to y+2, a blank column
// between characters, its pixels in half blocks, ▀▄█, in the ink k, and
// returns the column after its last character. A character it has no figure
// for isn't drawn.
func big(c *canvas, x, y int, text string, k ink) int {
	end := x
	for _, r := range text {
		figure, ok := figures[r]
		if !ok {
			continue
		}
		width := len(figure[0])
		for row := range bigRows {
			for col := range width {
				if glyph := halves(lit(figure, 2*row, col), lit(figure, 2*row+1, col)); glyph != "" {
					c.text(x+col, y+row, glyph, k)
				}
			}
		}
		end = x + width
		x = end + 1
	}
	return end
}

// lit reports whether the figure's pixel in the given row and column is lit:
// none is past its last row.
func lit(figure [5]string, row, col int) bool {
	return row < len(figure) && figure[row][col] == '1'
}

// halves is the half block that lights a cell's upper half, its lower, or
// both, or "" for neither.
func halves(upper, lower bool) string {
	switch {
	case upper && lower:
		return "█"
	case upper:
		return "▀"
	case lower:
		return "▄"
	default:
		return ""
	}
}

// timer counts down from now to t as big digits read it: in hours and
// minutes, such as "1:12"; within its last ten minutes, in minutes and
// seconds, such as "07:42"; and "0:00" once t has come.
func timer(now, t time.Time) string {
	left := max(t.Sub(now), 0)
	if inSeconds(now, t) {
		return fmt.Sprintf("%02d:%02d", int(left/time.Minute), int(left%time.Minute/time.Second))
	}
	return fmt.Sprintf("%d:%02d", int(left/time.Hour), int(left%time.Hour/time.Minute))
}
