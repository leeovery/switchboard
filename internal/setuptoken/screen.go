package setuptoken

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/x/ansi"
)

// maxRows is how many rows a screen keeps, scrollback and all: setup-token
// draws a few dozen, so this is only a bound on a program that draws without
// end.
const maxRows = 1000

// screen is what a terminal shows of what's drawn to it, as far as reading
// setup-token's drawing needs: its text, a row each, as the cursor moves,
// erases, inserts, deletes, scrolls and wraps it, and when each character was
// written. Colours, links and modes are passed over, and each character takes
// a column.
type screen struct {
	width, height int
	// rows holds the rows scrolled off the top, then the visible ones from
	// rows[top], each as long as what's been drawn on it. writes counts the
	// characters drawn, which is when each cell was written.
	rows   [][]cell
	writes int
	top    int
	// margins are the first and last of the visible rows that scroll, from
	// 0.
	margins [2]int
	// row and col are the cursor's: row indexes rows. wrap says the last
	// character drawn filled the row, so the next starts the row below.
	row, col int
	wrap     bool
	// last is the last character drawn, which a repeat draws again.
	last rune
	// saved is the cursor as it was last saved, its row counted from top.
	saved [2]int
	ansi  *ansi.Parser
}

// cell is a character on the screen, and when it was written, as the screen
// counts the characters drawn.
type cell struct {
	r       rune
	written int
}

// blankCell is a cell nothing has written to, or something has erased.
var blankCell = cell{r: ' '}

// newScreen returns an empty screen, width columns wide and height rows high.
func newScreen(width, height int) *screen {
	s := &screen{width: width, height: height, margins: [2]int{0, height - 1}, ansi: ansi.NewParser()}
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

// drawing returns the screen's lines, scrollback first, a line a row, without
// the blanks that end each, nor the blank rows that begin and end it. Its
// lines are the screen's own, until it's next written to.
func (s *screen) drawing() drawing {
	lines := make(drawing, len(s.rows))
	for i, row := range s.rows {
		end := len(row)
		for end > 0 && row[end-1].r == ' ' {
			end--
		}
		lines[i] = line(row[:end])
	}
	blank := func(l line) bool { return len(l) == 0 }
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	first := slices.IndexFunc(lines, func(l line) bool { return !blank(l) })
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
	s.last = r
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

// csi follows a control sequence, with n its first parameter as the moves,
// insertions, deletions and scrolls take it: 1 where it's missing or 0.
func (s *screen) csi(cmd ansi.Cmd, params ansi.Params) {
	if cmd.Prefix() == 0 && cmd.Intermediate() == '!' && cmd.Final() == 'p' {
		s.softReset()
		return
	}
	if cmd.Prefix() != 0 || cmd.Intermediate() != 0 {
		return
	}
	n := count(params, 0)
	switch cmd.Final() {
	case 'A':
		s.moveTo(s.up(n), s.col)
	case 'B', 'e':
		s.moveTo(s.down(n), s.col)
	case 'C', 'a':
		s.moveTo(s.row, s.col+n)
	case 'D':
		s.moveTo(s.row, s.col-n)
	case 'E':
		s.moveTo(s.down(n), 0)
	case 'F':
		s.moveTo(s.up(n), 0)
	case 'G', '`':
		s.moveTo(s.row, n-1)
	case 'd':
		s.moveTo(s.top+n-1, s.col)
	case 'H', 'f':
		s.moveTo(s.top+n-1, count(params, 1)-1)
	case 'J':
		mode, _, _ := params.Param(0, 0)
		s.eraseDisplay(mode)
	case 'K':
		mode, _, _ := params.Param(0, 0)
		s.eraseLine(mode)
	case 'X':
		s.erase(s.row, s.col, s.col+n)
	case '@':
		s.insertChars(n)
	case 'P':
		s.deleteChars(n)
	case 'L':
		s.insertLines(n)
	case 'M':
		s.deleteLines(n)
	case 'S':
		s.scrollUp(n)
	case 'T':
		// With more parameters, it's a mouse's tracking, not a scroll.
		if len(params) <= 1 {
			s.scrollDown(n)
		}
	case 'r':
		bottom, _, _ := params.Param(1, 0)
		s.setMargins(n, cmp.Or(bottom, s.height))
	case 'b':
		s.repeat(n)
	case 's':
		s.save()
	case 'u':
		s.restore()
	}
}

// count returns the parameter at i as a count, a row or a column: 1 where
// it's missing or 0, as a terminal takes it.
func count(params ansi.Params, i int) int {
	n, _, _ := params.Param(i, 1)
	return max(n, 1)
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
		s.reverseLineFeed()
	case 'c':
		s.reset()
	}
}

// reset empties the screen, as a terminal's full reset does.
func (s *screen) reset() {
	*s = screen{width: s.width, height: s.height, margins: [2]int{0, s.height - 1}, ansi: s.ansi}
}

// softReset sets the margins to the whole screen, and the cursor saved to its
// home, as a terminal's soft reset does, leaving what's drawn, and the cursor,
// where they are.
func (s *screen) softReset() {
	s.margins, s.saved = [2]int{0, s.height - 1}, [2]int{}
}

// up returns the row n rows above the cursor's, stopping at the top margin
// where the cursor's at or below it, or the screen's top.
func (s *screen) up(n int) int {
	top := s.top
	if s.row >= s.top+s.margins[0] {
		top += s.margins[0]
	}
	return max(s.row-n, top)
}

// down returns the row n rows below the cursor's, stopping at the bottom
// margin where the cursor's at or above it, or the screen's bottom.
func (s *screen) down(n int) int {
	bottom := s.top + s.height - 1
	if s.row <= s.top+s.margins[1] {
		bottom = s.top + s.margins[1]
	}
	return min(s.row+n, bottom)
}

// lineFeed moves the cursor down a row, scrolling the rows between the
// margins up a row at the bottom one.
func (s *screen) lineFeed() {
	s.wrap = false
	switch {
	case s.row == s.top+s.margins[1]:
		s.scrollUp(1)
	case s.row < s.top+s.height-1:
		s.row++
	}
}

// reverseLineFeed moves the cursor up a row, scrolling the rows between the
// margins down a row at the top one.
func (s *screen) reverseLineFeed() {
	if s.row == s.top+s.margins[0] {
		s.scrollDown(1)
		return
	}
	s.moveTo(s.row-1, s.col)
}

// scrollUp scrolls the rows between the margins up n, the cursor where it
// is. Where the top margin is the screen's top, those scrolled off it go into
// the scrollback, as xterm has them, those below the bottom margin staying
// where they are; otherwise they're gone.
func (s *screen) scrollUp(n int) {
	if s.margins[0] != 0 {
		s.shiftUp(s.top+s.margins[0], s.top+s.margins[1], n)
		return
	}
	n = min(n, s.margins[1]+1)
	if below := s.top + s.margins[1] + 1; below < len(s.rows) {
		s.rows = slices.Insert(s.rows, below, make([][]cell, n)...)
	}
	s.top += n
	s.row += n
}

// scrollDown scrolls the rows between the margins down n, the cursor where
// it is, those at the bottom gone.
func (s *screen) scrollDown(n int) {
	s.shiftDown(s.top+s.margins[0], s.top+s.margins[1], n)
}

// setMargins sets the rows that scroll, the first and the last, from 1, and
// takes the cursor home: where they don't make a region of two rows or more,
// nothing changes.
func (s *screen) setMargins(first, last int) {
	if first >= last || last > s.height {
		return
	}
	s.margins = [2]int{first - 1, last - 1}
	s.moveTo(s.top, 0)
}

// insertLines inserts n blank rows at the cursor's, those below it moving
// down, those past the bottom margin gone, where the cursor is between the
// margins.
func (s *screen) insertLines(n int) {
	if s.betweenMargins() {
		s.shiftDown(s.row, s.top+s.margins[1], n)
		s.moveTo(s.row, 0)
	}
}

// deleteLines deletes n rows from the cursor's, those below it moving up,
// blank ones in at the bottom margin, where the cursor is between the
// margins.
func (s *screen) deleteLines(n int) {
	if s.betweenMargins() {
		s.shiftUp(s.row, s.top+s.margins[1], n)
		s.moveTo(s.row, 0)
	}
}

func (s *screen) betweenMargins() bool {
	return s.row >= s.top+s.margins[0] && s.row <= s.top+s.margins[1]
}

// shiftUp moves the rows from first to last up n, those it moves past first
// gone, blank ones in at last.
func (s *screen) shiftUp(first, last, n int) {
	s.ensure(last)
	n = min(n, last-first+1)
	copy(s.rows[first:last+1], s.rows[first+n:last+1])
	s.blank(last-n+1, last)
}

// shiftDown moves the rows from first to last down n, those it moves past
// last gone, blank ones in at first.
func (s *screen) shiftDown(first, last, n int) {
	s.ensure(last)
	n = min(n, last-first+1)
	copy(s.rows[first+n:last+1], s.rows[first:last+1-n])
	s.blank(first, first+n-1)
}

// blank makes the rows from first to last blank.
func (s *screen) blank(first, last int) {
	for row := first; row <= last; row++ {
		s.rows[row] = nil
	}
}

// insertChars inserts n blanks at the cursor, what's after it on its row
// moving right, what passes the row's end gone.
func (s *screen) insertChars(n int) {
	s.wrap = false
	if s.row >= len(s.rows) || s.col >= len(s.rows[s.row]) {
		return
	}
	row := slices.Insert(s.rows[s.row], s.col, slices.Repeat([]cell{blankCell}, min(n, s.width))...)
	s.rows[s.row] = row[:min(len(row), s.width)]
}

// deleteChars deletes n characters from the cursor, what's after them on its
// row moving left.
func (s *screen) deleteChars(n int) {
	s.wrap = false
	if s.row >= len(s.rows) || s.col >= len(s.rows[s.row]) {
		return
	}
	row := s.rows[s.row]
	s.rows[s.row] = slices.Delete(row, s.col, min(s.col+n, len(row)))
}

// repeat draws the last character drawn n times more, as many as fill the
// screen at most.
func (s *screen) repeat(n int) {
	if s.last == 0 {
		return
	}
	for range min(n, s.width*s.height) {
		s.print(s.last)
	}
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

// ensure makes sure there's a row at index row, blank where none was drawn.
func (s *screen) ensure(row int) {
	for len(s.rows) <= row {
		s.rows = append(s.rows, nil)
	}
}

// put draws r at the cursor.
func (s *screen) put(r rune) {
	s.ensure(s.row)
	s.trim()
	row := s.rows[s.row]
	for len(row) <= s.col {
		row = append(row, blankCell)
	}
	s.writes++
	row[s.col] = cell{r: r, written: s.writes}
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
		s.drop(min(s.top, len(s.rows)))
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
	s.blank(first, min(end, len(s.rows))-1)
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
		cells[col] = blankCell
	}
}

// trim drops the oldest rows scrolled off the screen past maxRows.
func (s *screen) trim() {
	over := len(s.rows) - maxRows
	if over <= 0 || over > s.top {
		return
	}
	s.drop(over)
	s.top -= over
	s.row -= over
}

// drop drops the first n rows.
func (s *screen) drop(n int) {
	s.rows = slices.Delete(s.rows, 0, n)
}
