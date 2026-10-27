package setuptoken

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// maxRows is how many rows a screen keeps, scrollback and all: setup-token
// draws a few dozen, so this is only a bound on a program that draws without
// end.
const maxRows = 1000

// screen is what a terminal shows of what's drawn to it, as far as reading
// setup-token's drawing needs: its text, a row each, as the cursor moves,
// erases and wraps it. Colours, links and modes are passed over, and each
// character takes a column.
type screen struct {
	width, height int
	// rows holds the rows scrolled off the top, then the visible ones from
	// rows[top], each as long as what's been drawn on it.
	rows [][]rune
	top  int
	// row and col are the cursor's: row indexes rows. wrap says the last
	// character drawn filled the row, so the next starts the row below.
	row, col int
	wrap     bool
	// saved is the cursor as it was last saved, its row counted from top.
	saved [2]int
	ansi  *ansi.Parser
}

// newScreen returns an empty screen, width columns wide and height rows high.
func newScreen(width, height int) *screen {
	s := &screen{width: width, height: height, ansi: ansi.NewParser()}
	s.ansi.SetHandler(ansi.Handler{
		Print:     s.print,
		Execute:   s.execute,
		HandleCsi: s.csi,
		HandleEsc: s.esc,
	})
	return s
}

// write draws what was written to the terminal.
func (s *screen) write(drawn []byte) {
	for _, b := range drawn {
		s.ansi.Advance(b)
	}
}

// drawing returns the screen's text, scrollback first, a line a row, without
// the spaces that end each, nor the blank rows that begin and end it.
func (s *screen) drawing() drawing {
	lines := make(drawing, len(s.rows))
	for i, row := range s.rows {
		lines[i] = strings.TrimRight(string(row), " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	first := slices.IndexFunc(lines, func(line string) bool { return line != "" })
	if first < 0 {
		return nil
	}
	return lines[first:]
}

func (s *screen) print(r rune) {
	if s.wrap {
		s.col = 0
		s.lineFeed()
	}
	s.put(r)
	if s.col == s.width-1 {
		s.wrap = true
		return
	}
	s.col++
}

func (s *screen) execute(b byte) {
	switch b {
	case ansi.CR:
		s.moveTo(s.row, 0)
	case ansi.LF, ansi.VT, ansi.FF:
		s.lineFeed()
	case ansi.BS:
		s.moveTo(s.row, s.col-1)
	case ansi.HT:
		s.moveTo(s.row, (s.col/8+1)*8)
	}
}

// csi follows a control sequence, with n its first parameter: 1 where it has
// none, as the moves take it.
func (s *screen) csi(cmd ansi.Cmd, params ansi.Params) {
	if cmd.Prefix() != 0 || cmd.Intermediate() != 0 {
		return
	}
	n, _, _ := params.Param(0, 1)
	switch cmd.Final() {
	case 'A':
		s.moveTo(s.row-n, s.col)
	case 'B', 'e':
		s.moveTo(s.row+n, s.col)
	case 'C', 'a':
		s.moveTo(s.row, s.col+n)
	case 'D':
		s.moveTo(s.row, s.col-n)
	case 'E':
		s.moveTo(s.row+n, 0)
	case 'F':
		s.moveTo(s.row-n, 0)
	case 'G', '`':
		s.moveTo(s.row, n-1)
	case 'd':
		s.moveTo(s.top+n-1, s.col)
	case 'H', 'f':
		col, _, _ := params.Param(1, 1)
		s.moveTo(s.top+n-1, col-1)
	case 'J':
		mode, _, _ := params.Param(0, 0)
		s.eraseDisplay(mode)
	case 'K':
		mode, _, _ := params.Param(0, 0)
		s.eraseLine(mode)
	case 'X':
		s.erase(s.row, s.col, s.col+n)
	case 's':
		s.save()
	case 'u':
		s.restore()
	}
}

func (s *screen) esc(cmd ansi.Cmd) {
	if cmd.Intermediate() != 0 {
		return
	}
	switch cmd.Final() {
	case '7':
		s.save()
	case '8':
		s.restore()
	case 'D':
		s.lineFeed()
	case 'E':
		s.col = 0
		s.lineFeed()
	case 'M':
		s.moveTo(s.row-1, s.col)
	case 'c':
		s.reset()
	}
}

// reset empties the screen, as a terminal's full reset does.
func (s *screen) reset() {
	s.rows, s.top, s.row, s.col, s.wrap, s.saved = nil, 0, 0, 0, false, [2]int{}
}

// lineFeed moves the cursor down a row, scrolling the screen up a row at its
// bottom.
func (s *screen) lineFeed() {
	s.wrap = false
	if s.row == s.top+s.height-1 {
		s.top++
	}
	s.row++
}

// moveTo moves the cursor to row and col, kept on the visible screen.
func (s *screen) moveTo(row, col int) {
	s.wrap = false
	s.row = min(max(row, s.top), s.top+s.height-1)
	s.col = min(max(col, 0), s.width-1)
}

func (s *screen) save() {
	s.saved = [2]int{s.row - s.top, s.col}
}

func (s *screen) restore() {
	s.moveTo(s.top+s.saved[0], s.saved[1])
}

// put draws r at the cursor.
func (s *screen) put(r rune) {
	for len(s.rows) <= s.row {
		s.rows = append(s.rows, nil)
	}
	s.trim()
	row := s.rows[s.row]
	for len(row) <= s.col {
		row = append(row, ' ')
	}
	row[s.col] = r
	s.rows[s.row] = row
}

// eraseDisplay erases, by mode, the visible screen from the cursor to its end
// (0), from its start to the cursor (1), or all of it (2), or the rows
// scrolled off it (3).
func (s *screen) eraseDisplay(mode int) {
	bottom := s.top + s.height
	switch mode {
	case 0:
		s.erase(s.row, s.col, s.width)
		s.eraseRows(s.row+1, bottom)
	case 1:
		s.eraseRows(s.top, s.row)
		s.erase(s.row, 0, s.col+1)
	case 2:
		s.eraseRows(s.top, bottom)
	case 3:
		s.rows = slices.Delete(s.rows, 0, min(s.top, len(s.rows)))
		s.row -= s.top
		s.top = 0
	}
}

// eraseLine erases, by mode, the cursor's row from the cursor to its end (0),
// from its start to the cursor (1), or all of it (2).
func (s *screen) eraseLine(mode int) {
	switch mode {
	case 0:
		s.erase(s.row, s.col, s.width)
	case 1:
		s.erase(s.row, 0, s.col+1)
	case 2:
		s.erase(s.row, 0, s.width)
	}
}

// eraseRows erases the rows from first up to end.
func (s *screen) eraseRows(first, end int) {
	for row := first; row < min(end, len(s.rows)); row++ {
		s.rows[row] = nil
	}
}

// erase erases the columns of row from first up to end.
func (s *screen) erase(row, first, end int) {
	if row >= len(s.rows) {
		return
	}
	cells := s.rows[row]
	if end >= len(cells) {
		s.rows[row] = cells[:min(first, len(cells))]
		return
	}
	for col := first; col < end; col++ {
		cells[col] = ' '
	}
}

// trim drops the oldest rows scrolled off the screen past maxRows.
func (s *screen) trim() {
	over := len(s.rows) - maxRows
	if over <= 0 || over > s.top {
		return
	}
	s.rows = slices.Delete(s.rows, 0, over)
	s.top -= over
	s.row -= over
}
