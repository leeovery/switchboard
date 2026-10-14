package capture

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// cell is a terminal's cell as a frame draws it: its glyph, in the pen it's
// drawn with.
type cell struct {
	glyph rune
	pen
}

// String reads as a failure names the cell.
func (c cell) String() string {
	weight := ""
	if c.bold {
		weight = " bold"
	}
	return fmt.Sprintf("%q %s on %s%s", c.glyph, c.fg, c.bg, weight)
}

// pen is what a glyph is drawn in: its colours, and whether it's bold.
type pen struct {
	fg, bg colour
	bold   bool
}

// colour is a cell's 24-bit colour, or none where the frame leaves it to the
// terminal.
type colour struct {
	r, g, b uint8
	painted bool
}

// rgb is the colour painted r, g, b.
func rgb(r, g, b uint8) colour {
	return colour{r: r, g: g, b: b, painted: true}
}

// String reads as a hex colour, or "default" where none is painted.
func (c colour) String() string {
	if !c.painted {
		return "default"
	}
	return fmt.Sprintf("#%02X%02X%02X", c.r, c.g, c.b)
}

// near reports whether c and d are one colour, but for a channel one apart
// at most: the frames' generators round a half to even, where Go's mixing
// rounds it away from zero.
func (c colour) near(d colour) bool {
	if c.painted != d.painted {
		return false
	}
	return apart(c.r, d.r) <= 1 && apart(c.g, d.g) <= 1 && apart(c.b, d.b) <= 1
}

// apart is how far x and y are apart.
func apart(x, y uint8) uint8 {
	return max(x, y) - min(x, y)
}

// or is c, or e where c isn't painted.
func (c colour) or(e colour) colour {
	if c.painted {
		return c
	}
	return e
}

// grid is a frame's cells, a row of them a line.
type grid [][]cell

// text is the grid's glyphs, a line a row, right-trimmed as a frame's .txt
// gives them.
func (g grid) text() []string {
	lines := make([]string, len(g))
	for i, row := range g {
		var line strings.Builder
		for _, c := range row {
			line.WriteRune(c.glyph)
		}
		lines[i] = strings.TrimRight(line.String(), " ")
	}
	return lines
}

// parse reads a frame's ANSI into its cells as a terminal draws them, one
// glyph a cell, each in the pen its SGR left set, carried from one cell to
// the next and across lines until reset. A newline ends a row, and the
// frame's last may end in one or not. Anything else, an escape that isn't
// SGR, an SGR attribute but bold, a colour that isn't 24-bit, a control
// character or a glyph not one cell wide, is an error, rather than read
// otherwise than a terminal shows it.
func parse(frame string) (grid, error) {
	g := grid{nil}
	var p pen
	for i := 0; i < len(frame); {
		r, size := utf8.DecodeRuneInString(frame[i:])
		var err error
		switch {
		case r == '\x1b':
			size, err = p.apply(frame[i:])
		case r == '\n':
			g = append(g, nil)
		case r == utf8.RuneError && size == 1:
			err = errors.New("not UTF-8")
		case r < ' ' || r == '\x7f':
			err = fmt.Errorf("control character %q", r)
		case ansi.StringWidth(string(r)) != 1:
			err = fmt.Errorf("%q is %d cells wide, where a cell is one", r, ansi.StringWidth(string(r)))
		default:
			g[len(g)-1] = append(g[len(g)-1], cell{glyph: r, pen: p})
		}
		if err != nil {
			return nil, fmt.Errorf("row %d, column %d: %w", len(g), len(g[len(g)-1])+1, err)
		}
		i += size
	}
	if len(g[len(g)-1]) == 0 {
		g = g[:len(g)-1]
	}
	return g, nil
}

// apply sets the pen as the SGR sequence s starts with says, and returns the
// sequence's length in bytes.
func (p *pen) apply(s string) (int, error) {
	if !strings.HasPrefix(s, "\x1b[") {
		return 0, fmt.Errorf("escape %q isn't SGR", s[:min(len(s), 2)])
	}
	end := strings.IndexFunc(s[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
	if end < 0 {
		return 0, fmt.Errorf("escape %q never ends", s)
	}
	if s[2+end] != 'm' {
		return 0, fmt.Errorf("escape %q isn't SGR", s[:3+end])
	}
	return 3 + end, p.set(s[2 : 2+end])
}

// set sets the pen as the SGR parameters given say, in turn.
func (p *pen) set(params string) error {
	codes := strings.Split(params, ";")
	for i := 0; i < len(codes); i++ {
		n, err := attribute(codes[i])
		if err != nil {
			return fmt.Errorf("SGR %q: %w", params, err)
		}
		switch n {
		case 0:
			*p = pen{}
		case 1:
			p.bold = true
		case 22:
			p.bold = false
		case 39:
			p.fg = colour{}
		case 49:
			p.bg = colour{}
		case 38, 48:
			c, err := truecolour(codes[i+1:])
			if err != nil {
				return fmt.Errorf("SGR %q: %w", params, err)
			}
			if n == 38 {
				p.fg = c
			} else {
				p.bg = c
			}
			i += 4
		default:
			return fmt.Errorf("SGR %q: attribute %d isn't read", params, n)
		}
	}
	return nil
}

// attribute is the SGR parameter given, an empty one 0, as a terminal reads
// it.
func attribute(code string) (int, error) {
	if code == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return 0, fmt.Errorf("parameter %q isn't a number", code)
	}
	return n, nil
}

// errColour is a colour SGR gives otherwise than in 24 bits.
var errColour = errors.New("its colour isn't 24-bit")

// truecolour is the colour the SGR parameters after 38 or 48 give: 2, then
// its red, green and blue.
func truecolour(codes []string) (colour, error) {
	if len(codes) < 4 || codes[0] != "2" {
		return colour{}, errColour
	}
	var channels [3]uint8
	for i, code := range codes[1:4] {
		n, err := strconv.ParseUint(code, 10, 8)
		if err != nil {
			return colour{}, errColour
		}
		channels[i] = uint8(n)
	}
	return rgb(channels[0], channels[1], channels[2]), nil
}

// span is cells of a frame's row: its columns from and to, or the whole row
// where neither is given. Rows and columns count from 1, as a frame's .txt
// reads in an editor.
type span struct {
	row, from, to int
}

// String reads as the README lists a span.
func (s span) String() string {
	if s.from == 0 {
		return fmt.Sprintf("row %d", s.row)
	}
	return fmt.Sprintf("row %d, columns %d to %d", s.row, s.from, s.to)
}

// covers reports whether the span covers the cell at row and column.
func (s span) covers(row, col int) bool {
	return row == s.row && (s.from == 0 || s.from <= col && col <= s.to)
}

// difference is a cell, at its row and column counted from 1, that a fixture
// draws otherwise than its frame.
type difference struct {
	row, col     int
	frame, drawn cell
}

// compare lists the cells drawn differs from frame in, passing over those a
// span of over covers. A cell whose background isn't painted is the canvas's,
// as a terminal of that background shows it; a colour's channel may be one
// apart (see colour.near); and a blank's foreground and weight aren't
// compared, as nothing shows them. A drawn row shorter than the frame's is
// filled to its width with blank, unpainted cells, as a terminal shows what
// wasn't written, since usage's printout writes nothing past a line's end.
// The grids must have as many rows, and no drawn row may be wider.
func compare(frame, drawn grid, canvas colour, over []span) ([]difference, error) {
	if len(drawn) != len(frame) {
		return nil, fmt.Errorf("drawn %d rows, where the frame has %d", len(drawn), len(frame))
	}
	var diffs []difference
	for r := range frame {
		if len(drawn[r]) > len(frame[r]) {
			return nil, fmt.Errorf("row %d drawn %d columns, where the frame has %d", r+1, len(drawn[r]), len(frame[r]))
		}
		row := filled(drawn[r], len(frame[r]))
		for c := range frame[r] {
			if passed(over, r+1, c+1) || alike(frame[r][c], row[c], canvas) {
				continue
			}
			diffs = append(diffs, difference{row: r + 1, col: c + 1, frame: frame[r][c], drawn: row[c]})
		}
	}
	return diffs, nil
}

// filled is row, with blank, unpainted cells after it to width, which is no
// less than its own.
func filled(row []cell, width int) []cell {
	return slices.Concat(row, slices.Repeat([]cell{{glyph: ' '}}, width-len(row)))
}

// passed reports whether a span of over covers the cell at row and column.
func passed(over []span, row, col int) bool {
	for _, s := range over {
		if s.covers(row, col) {
			return true
		}
	}
	return false
}

// alike reports whether cells a and b look the same on a terminal whose
// background is canvas.
func alike(a, b cell, canvas colour) bool {
	if a.glyph != b.glyph || !a.bg.or(canvas).near(b.bg.or(canvas)) {
		return false
	}
	return a.glyph == ' ' || a.bold == b.bold && a.fg.near(b.fg)
}

// shown is how many cells a failure lists before it counts the rest.
const shown = 20

// describe says where a fixture draws otherwise than its frame, stem: a line
// a cell, for the first shown of them, then how many more.
func describe(stem string, diffs []difference) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s draws %d cells otherwise than its frame:", stem, len(diffs))
	for _, d := range diffs[:min(len(diffs), shown)] {
		fmt.Fprintf(&b, "\n  row %d, column %d: the frame's %s, drawn %s", d.row, d.col, d.frame, d.drawn)
	}
	if more := len(diffs) - shown; more > 0 {
		fmt.Fprintf(&b, "\n  and %d more", more)
	}
	return b.String()
}

func TestParseReadsSGRAsATerminalDoes(t *testing.T) {
	ink, canvas, other := rgb(216, 222, 233), rgb(46, 52, 64), rgb(1, 2, 3)
	tests := []struct {
		name, frame string
		want        grid
	}{
		{
			name:  "the generators', every cell painted, reset before each",
			frame: "\x1b[0;1;38;2;216;222;233;48;2;46;52;64mA\x1b[0;38;2;1;2;3;48;2;46;52;64mb \x1b[0m",
			want:  grid{slices.Concat(inked("A", pen{ink, canvas, true}), inked("b ", pen{other, canvas, false}))},
		},
		{
			name:  "Lip Gloss's, a run in one pen, then reset by an empty SGR",
			frame: "\x1b[1;38;2;216;222;233;48;2;46;52;64mAb\x1b[m c",
			want:  grid{slices.Concat(inked("Ab", pen{ink, canvas, true}), inked(" c", pen{}))},
		},
		{
			name:  "each attribute carried, until changed alone",
			frame: "\x1b[1mA\x1b[38;2;1;2;3mB\x1b[22mC\x1b[48;2;46;52;64mD\x1b[39mE\x1b[49mF",
			want: grid{slices.Concat(
				inked("A", pen{bold: true}), inked("B", pen{fg: other, bold: true}), inked("C", pen{fg: other}),
				inked("D", pen{other, canvas, false}), inked("E", pen{bg: canvas}), inked("F", pen{}),
			)},
		},
		{
			name:  "carried across lines",
			frame: "\x1b[1mA\nB\x1b[0m\nC",
			want:  grid{inked("A", pen{bold: true}), inked("B", pen{bold: true}), inked("C", pen{})},
		},
		{
			name:  "a last newline, ending the last row",
			frame: "A\n\nB\n",
			want:  grid{inked("A", pen{}), nil, inked("B", pen{})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.frame)
			if err != nil {
				t.Fatal(err)
			}
			if !equal(got, tt.want) {
				t.Errorf("parse(%q) = %v, want %v", tt.frame, got, tt.want)
			}
		})
	}
}

func TestParseRefusesWhatItWouldReadOtherwiseThanATerminal(t *testing.T) {
	tests := []struct {
		name, frame, want string
	}{
		{name: "faint", frame: "\x1b[2mA", want: `row 1, column 1: SGR "2": attribute 2 isn't read`},
		{name: "underlined", frame: "AB\x1b[4mC", want: `row 1, column 3: SGR "4": attribute 4 isn't read`},
		{name: "reversed", frame: "\x1b[7mA", want: "attribute 7 isn't read"},
		{name: "a palette's colour", frame: "\x1b[38;5;1mA", want: "its colour isn't 24-bit"},
		{name: "a colour cut short", frame: "\x1b[48;2;1;2mA", want: "its colour isn't 24-bit"},
		{name: "a channel past 255", frame: "\x1b[38;2;1;2;256mA", want: "its colour isn't 24-bit"},
		{name: "a colour's parts apart by colons", frame: "\x1b[38:2::1:2:3mA", want: `parameter "38:2::1:2:3" isn't a number`},
		{name: "an escape that isn't SGR", frame: "A\n\x1b[2JB", want: `row 2, column 1: escape "\x1b[2J" isn't SGR`},
		{name: "a hyperlink", frame: "\x1b]8;;x\x1b\\A", want: `escape "\x1b]" isn't SGR`},
		{name: "an escape never ended", frame: "A\x1b[1", want: "never ends"},
		{name: "a tab", frame: "A\tB", want: `row 1, column 2: control character '\t'`},
		{name: "a carriage return", frame: "A\r\nB", want: `control character '\r'`},
		{name: "a wide glyph", frame: "A世", want: "row 1, column 2: '世' is 2 cells wide, where a cell is one"},
		{name: "bytes that aren't UTF-8", frame: "A\xff", want: "not UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := parse(tt.frame)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parse(%q) = %v, error %v, want an error with %q", tt.frame, g, err, tt.want)
			}
		})
	}
}

func TestEveryFixturesFrameParsesToItsPlainText(t *testing.T) {
	utc := local()
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			frame := frameOf(t, utc, name)
			g, err := parse(frame)
			if err != nil {
				t.Fatalf("parsing the frame: %v", err)
			}
			want := strings.Split(strings.TrimSuffix(ansi.Strip(frame), "\n"), "\n")
			for i := range want {
				want[i] = strings.TrimRight(want[i], " ")
			}
			if got := g.text(); !slices.Equal(got, want) {
				t.Errorf("the frame's cells read\n%s\nwant its plain text\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestCompareHoldsEachCellToItsFrame(t *testing.T) {
	canvas := rgb(46, 52, 64)
	frame, err := parse("\x1b[0;1;38;2;216;222;233;48;2;46;52;64mAb\x1b[0;38;2;1;2;3;48;2;46;52;64m c\n" +
		"\x1b[0;38;2;1;2;3;48;2;180;142;173mde \x1b[0;38;2;1;2;3;48;2;46;52;64mf\x1b[0m")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(g grid)
		over   []span
		// want are the cells found to differ, each as its row and column.
		want [][2]int
	}{
		{name: "the frame itself", change: func(grid) {}},
		{name: "a glyph changed", change: func(g grid) { g[0][1].glyph = 'B' }, want: [][2]int{{1, 2}}},
		{name: "a channel one apart", change: func(g grid) { g[0][0].fg.r++; g[1][0].bg.b-- }},
		{name: "a channel two apart", change: func(g grid) { g[0][3].fg.g += 2 }, want: [][2]int{{1, 4}}},
		{name: "a background two apart", change: func(g grid) { g[1][1].bg.r -= 2 }, want: [][2]int{{2, 2}}},
		{name: "bold changed", change: func(g grid) { g[1][3].bold = true }, want: [][2]int{{2, 4}}},
		{name: "a foreground left to the terminal", change: func(g grid) { g[0][0].fg = colour{} }, want: [][2]int{{1, 1}}},
		{name: "a background unpainted, on the canvas", change: func(g grid) { g[0][1].bg = colour{}; g[1][3].bg = colour{} }},
		{name: "a background unpainted, off the canvas", change: func(g grid) { g[1][0].bg = colour{} }, want: [][2]int{{2, 1}}},
		{name: "a blank's foreground and weight", change: func(g grid) { g[0][2].fg = rgb(9, 9, 9); g[1][2].bold = true }},
		{name: "a blank's background", change: func(g grid) { g[0][2].bg = rgb(9, 9, 9) }, want: [][2]int{{1, 3}}},
		{
			name:   "cells passed over",
			change: func(g grid) { g[0][1].glyph, g[0][2].glyph, g[0][3].glyph = 'x', 'y', 'z' },
			over:   []span{{row: 1, from: 2, to: 3}},
			want:   [][2]int{{1, 4}},
		},
		{
			name:   "a row passed over",
			change: func(g grid) { g[0][0].bold = false; g[1][0].glyph, g[1][3].bold = 'x', true },
			over:   []span{{row: 2}},
			want:   [][2]int{{1, 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := clone(frame)
			tt.change(drawn)
			diffs, err := compare(frame, drawn, canvas, tt.over)
			if err != nil {
				t.Fatal(err)
			}
			var got [][2]int
			for _, d := range diffs {
				got = append(got, [2]int{d.row, d.col})
				if want := (difference{row: d.row, col: d.col, frame: frame[d.row-1][d.col-1], drawn: drawn[d.row-1][d.col-1]}); d != want {
					t.Errorf("row %d, column %d is told as the frame's %s, drawn %s, want the frame's %s, drawn %s",
						d.row, d.col, d.frame, d.drawn, want.frame, want.drawn)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("compare found the cells %v differing, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareFillsAShortRowAsATerminalShowsIt(t *testing.T) {
	canvas, ink, other := rgb(46, 52, 64), rgb(216, 222, 233), rgb(180, 142, 173)
	tests := []struct {
		name  string
		frame []cell
		// want are the columns found to differ, each with the cell drawn
		// there.
		want map[int]cell
	}{
		{name: "the frame blank on the canvas past it", frame: slices.Concat(inked("ab", pen{ink, canvas, true}), inked("  ", pen{ink, canvas, false}))},
		{
			name:  "a glyph in the frame past it",
			frame: slices.Concat(inked("ab", pen{ink, canvas, true}), inked(" c", pen{ink, canvas, false})),
			want:  map[int]cell{4: {glyph: ' '}},
		},
		{
			name:  "another background in the frame past it",
			frame: slices.Concat(inked("ab", pen{ink, canvas, true}), inked(" ", pen{ink, canvas, false}), inked(" ", pen{ink, other, false})),
			want:  map[int]cell{4: {glyph: ' '}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := grid{inked("ab", pen{ink, canvas, true})}
			diffs, err := compare(grid{tt.frame}, drawn, canvas, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := map[int]cell{}
			for _, d := range diffs {
				got[d.col] = d.drawn
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("compare found %v differing, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareRefusesAGridOfAnotherSize(t *testing.T) {
	frame := grid{inked("ab", pen{}), inked("cd", pen{})}
	tests := []struct {
		name  string
		drawn grid
		want  string
	}{
		{name: "a row fewer", drawn: frame[:1], want: "drawn 1 rows, where the frame has 2"},
		{name: "a row more", drawn: append(clone(frame), nil), want: "drawn 3 rows, where the frame has 2"},
		{name: "a row wider", drawn: grid{inked("abc", pen{}), frame[1]}, want: "row 1 drawn 3 columns, where the frame has 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compare(frame, tt.drawn, colour{}, nil); err == nil || err.Error() != tt.want {
				t.Errorf("compare error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestDescribeNamesEachCellThenCountsTheRest(t *testing.T) {
	frame := cell{glyph: 'b', fg: rgb(1, 2, 3), bg: rgb(46, 52, 64)}
	drawn := cell{glyph: 'x', bold: true}
	diffs := make([]difference, shown+3)
	for i := range diffs {
		diffs[i] = difference{row: 2, col: i + 1, frame: frame, drawn: drawn}
	}

	got := strings.Split(describe("usage-160", diffs), "\n")
	want := []string{
		"usage-160 draws 23 cells otherwise than its frame:",
		"  row 2, column 1: the frame's 'b' #010203 on #2E3440, drawn 'x' default on default bold",
	}
	if !slices.Equal(got[:2], want) {
		t.Errorf("describe starts\n%s\nwant\n%s", strings.Join(got[:2], "\n"), strings.Join(want, "\n"))
	}
	if last := got[len(got)-1]; len(got) != shown+2 || last != "  and 3 more" {
		t.Errorf("describe is %d lines, the last %q, want %d, the last %q", len(got), last, shown+2, "  and 3 more")
	}
	if one := describe("usage-160", diffs[:1]); strings.Contains(one, "more") {
		t.Errorf("describe of one cell says %q, want no count of more", one)
	}
}

// inked is a row of text's cells, each drawn in p.
func inked(text string, p pen) []cell {
	var row []cell
	for _, r := range text {
		row = append(row, cell{glyph: r, pen: p})
	}
	return row
}

// equal reports whether a and b are the same cells, row for row.
func equal(a, b grid) bool {
	return slices.EqualFunc(a, b, func(x, y []cell) bool { return slices.Equal(x, y) })
}

// clone is a copy of g, each row its own.
func clone(g grid) grid {
	c := make(grid, len(g))
	for i, row := range g {
		c[i] = slices.Clone(row)
	}
	return c
}
