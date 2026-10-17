package dashboard

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// seen is a cell as a terminal shows it: its glyph, its colours, and
// whether it's bold.
type seen struct {
	glyph  rune
	fg, bg shade
	bold   bool
}

// String reads as a failure names the cell.
func (s seen) String() string {
	weight := ""
	if s.bold {
		weight = " bold"
	}
	return fmt.Sprintf("%q %s on %s%s", s.glyph, s.fg, s.bg, weight)
}

// shade is a colour SGR sets in 24 bits, or none, where it leaves the
// terminal's own.
type shade struct {
	r, g, b uint8
	set     bool
}

// shadeOf is c as SGR sets it: none where c is nil.
func shadeOf(c color.Color) shade {
	if c == nil {
		return shade{}
	}
	r, g, b, _ := c.RGBA()
	return shade{r: uint8(r >> 8), g: uint8(g >> 8), b: uint8(b >> 8), set: true}
}

// String reads as a hex colour, or "default" where none is set.
func (s shade) String() string {
	if !s.set {
		return "default"
	}
	return fmt.Sprintf("#%02X%02X%02X", s.r, s.g, s.b)
}

// near reports whether s and o are one colour, a channel one apart at most,
// as the frames' generator rounds a half to even, where Go's mixing rounds
// it away from zero.
func (s shade) near(o shade) bool {
	apart := func(x, y uint8) bool { return max(x, y)-min(x, y) <= 1 }
	return s.set == o.set && apart(s.r, o.r) && apart(s.g, o.g) && apart(s.b, o.b)
}

// or is s, or o where s isn't set.
func (s shade) or(o shade) shade {
	if s.set {
		return s
	}
	return o
}

// screenOf reads rows of ANSI into the cells a terminal shows, a glyph a
// cell, each in the pen SGR left set, which carries on from cell to cell and
// row to row until it's reset. It fails the test on anything it would read
// otherwise than a terminal shows it: an escape that isn't SGR, an SGR
// attribute but bold, a colour that isn't 24-bit, or a glyph not a cell
// wide.
func screenOf(t *testing.T, rows []string) [][]seen {
	t.Helper()
	cells := make([][]seen, len(rows))
	var pen seen
	for y, row := range rows {
		for i := 0; i < len(row); {
			r, size := utf8.DecodeRuneInString(row[i:])
			if r == '\x1b' {
				size = pen.apply(t, row[i:])
			} else {
				if w := ansi.StringWidth(string(r)); w != 1 {
					t.Fatalf("row %d: %q is %d cells wide, where a cell is one", y+1, r, w)
				}
				pen.glyph = r
				cells[y] = append(cells[y], pen)
			}
			i += size
		}
	}
	return cells
}

// apply sets the pen as the SGR sequence s starts with says, and returns the
// sequence's length in bytes.
func (s *seen) apply(t *testing.T, sgr string) int {
	t.Helper()
	end := strings.IndexByte(sgr, 'm')
	if !strings.HasPrefix(sgr, "\x1b[") || end < 0 {
		t.Fatalf("escape %q isn't SGR", sgr[:min(len(sgr), 8)])
	}
	codes := strings.Split(sgr[2:end], ";")
	for i := 0; i < len(codes); i++ {
		switch codes[i] {
		case "", "0":
			*s = seen{}
		case "1":
			s.bold = true
		case "22":
			s.bold = false
		case "39":
			s.fg = shade{}
		case "49":
			s.bg = shade{}
		case "38", "48":
			if len(codes) < i+5 || codes[i+1] != "2" {
				t.Fatalf("SGR %q: its colour isn't 24-bit", sgr[:end+1])
			}
			c := shade{r: channel(t, codes[i+2]), g: channel(t, codes[i+3]), b: channel(t, codes[i+4]), set: true}
			if codes[i] == "38" {
				s.fg = c
			} else {
				s.bg = c
			}
			i += 4
		default:
			t.Fatalf("SGR %q: attribute %s isn't read", sgr[:end+1], codes[i])
		}
	}
	return end + 1
}

// channel is a colour's channel as SGR gives it.
func channel(t *testing.T, code string) uint8 {
	t.Helper()
	n, err := strconv.ParseUint(code, 10, 8)
	if err != nil {
		t.Fatalf("channel %q: %v", code, err)
	}
	return uint8(n)
}

// reference is the cells of the signed-off frame with the stem given, as a
// terminal shows its ANSI.
func reference(t *testing.T, stem string) [][]seen {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "vhs", "reference", stem+".ansi"))
	if err != nil {
		t.Fatal(err)
	}
	return screenOf(t, strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"))
}

// region is the cells of a frame a test holds to it: from row top to
// bottom and column left to right, counted from 0.
type region struct {
	top, bottom, left, right int
}

// spot is cells along a frame's row, from column first to last, counted
// from 0.
type spot struct {
	row, first, last int
}

// covers reports whether any of the spots covers the cell at row and col.
func covers(spots []spot, row, col int) bool {
	for _, s := range spots {
		if s.row == row && s.first <= col && col <= s.last {
			return true
		}
	}
	return false
}

// aRowUp is drawn read a row down, its row y as row y+1, so what the design
// has since drawn a row up from where a frame drew it is held to the frame's
// rows, as Runway's pool's room and all under it are, the rule over the
// pool's lane and its words under it gone (the owner's call, 17 October
// 2026).
func aRowUp(drawn [][]seen) [][]seen {
	return append([][]seen{nil}, drawn...)
}

// redrawn is the frame with the glyph was read as is along the rows given,
// counted from 0, its cells' colours and weight as it has them: where the
// design has since drawn the glyph otherwise, and the frame still holds the
// rest of each cell.
func redrawn(frame [][]seen, was, is rune, rows ...int) [][]seen {
	cells := make([][]seen, len(frame))
	for y, row := range frame {
		cells[y] = slices.Clone(row)
		if !slices.Contains(rows, y) {
			continue
		}
		for x := range cells[y] {
			if cells[y][x].glyph == was {
				cells[y][x].glyph = is
			}
		}
	}
	return cells
}

// heldTo holds the cells drawn in the region to the frame's, as a terminal
// whose background is canvas shows them: their glyphs and backgrounds, and
// their foregrounds and weight, but a blank's, which nothing shows. It
// passes over the cells whole covers, and the foregrounds of those
// foreground covers, where the frame draws them otherwise than the design.
func heldTo(t *testing.T, stem string, frame, drawn [][]seen, in region, canvas shade, whole, foreground []spot) {
	t.Helper()
	var unlike []string
	for y := in.top; y <= in.bottom; y++ {
		for x := in.left; x <= in.right; x++ {
			want, got := seenAt(frame, x, y), seenAt(drawn, x, y)
			glyph := want.glyph == got.glyph && want.bg.or(canvas).near(got.bg.or(canvas))
			ink := want.glyph == ' ' || want.bold == got.bold && (want.fg.near(got.fg) || covers(foreground, y, x))
			if alike := glyph && ink; !alike && !covers(whole, y, x) {
				unlike = append(unlike, fmt.Sprintf("row %d, column %d: the frame's %s, drawn %s", y+1, x+1, want, got))
			}
		}
	}
	if len(unlike) > 0 {
		t.Errorf("drew %d cells otherwise than %s:\n  %s", len(unlike), stem, strings.Join(unlike[:min(len(unlike), 20)], "\n  "))
	}
}

// seenAt is the cell at x along row y, or a blank one where none was drawn,
// as a terminal shows what wasn't written.
func seenAt(cells [][]seen, x, y int) seen {
	if y >= len(cells) || x >= len(cells[y]) {
		return seen{glyph: ' '}
	}
	return cells[y][x]
}
