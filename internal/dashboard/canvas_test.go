package dashboard

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTextIsDrawnACellAGlyph(t *testing.T) {
	tests := []struct {
		name string
		x    int
		text string
		want string
		// wantEnd is the column text returns: the one after it.
		wantEnd int
	}{
		{name: "from a column", x: 2, text: "abc", want: "  abc", wantEnd: 5},
		{name: "cut by the right edge", x: 6, text: "abcdef", want: "      abcd", wantEnd: 12},
		{name: "cut by the left edge", x: -2, text: "abcdef", want: "cdef", wantEnd: 4},
		{name: "a wide glyph over two cells", x: 1, text: "東京", want: " 東京", wantEnd: 5},
		{name: "a wide glyph the right edge cuts, left blank", x: 7, text: "a東京", want: "       a東", wantEnd: 12},
		{name: "a letter and its accent, one glyph", x: 0, text: "áb", want: "áb", wantEnd: 2},
		{name: "a glyph of no width, not drawn", x: 0, text: "́ab", want: "ab", wantEnd: 2},
		{name: "box drawing and braille, a cell each", x: 0, text: "╭─⠂⠄⡀▆", want: "╭─⠂⠄⡀▆", wantEnd: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(10, 1)
			end := c.text(tt.x, 0, tt.text, ink{})
			if got := c.rows(Look{})[0]; got != tt.want {
				t.Errorf("drew %q, want %q", got, tt.want)
			}
			if end != tt.wantEnd {
				t.Errorf("text() = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

func TestDrawingOverHalfAWideGlyphBlanksItsOtherHalf(t *testing.T) {
	tests := []struct {
		name string
		x    int
		want string
	}{
		{name: "its first half", x: 0, want: "x 京"},
		{name: "its second half", x: 1, want: " x京"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(6, 1)
			c.text(0, 0, "東京", ink{})
			c.text(tt.x, 0, "x", ink{})
			if got := c.rows(Look{})[0]; got != tt.want {
				t.Errorf("drew %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRightEndsALineBeforeTheColumnGiven(t *testing.T) {
	c := newCanvas(14, 1)
	start := c.right(13, 0, line{{"read ", dimInk}, {"4s ago", dimInk}})

	if got, want := c.rows(Look{})[0], "  read 4s ago"; got != want {
		t.Errorf("drew %q, want %q, the last column blank", got, want)
	}
	if start != 2 {
		t.Errorf("right() = %d, want the column it starts at, 2", start)
	}
}

func TestEachRowIsDrawnInItsLook(t *testing.T) {
	nord := builtin(t, "nord")
	c := newCanvas(20, 2)
	c.line(1, 0, line{{"ROUTER", labelInk}, spaces(2), {"healthy", titleInk}})
	c.line(1, 1, line{{"5 sessions · auto", mutedInk}})

	tests := []struct {
		name string
		look Look
		// check checks what each row reads.
		check func(t *testing.T, rows []string)
	}{
		{name: "the zero look, as text alone, ending at its last glyph", look: Look{}, check: func(t *testing.T, rows []string) {
			if want := []string{" ROUTER  healthy", " 5 sessions · auto"}; !slices.Equal(rows, want) {
				t.Errorf("rows = %q, want %q", rows, want)
			}
		}},
		{name: "without colour, bold alone", look: NoColour(), check: func(t *testing.T, rows []string) {
			for _, sgr := range regexp.MustCompile(`\x1b\[[0-9;]*m`).FindAllString(strings.Join(rows, ""), -1) {
				if sgr != "\x1b[1m" && sgr != "\x1b[m" {
					t.Errorf("drew %q, want bold alone", sgr)
				}
			}
			if !strings.Contains(rows[0], "\x1b[1mROUTER") {
				t.Errorf("row 1 = %q, want its label bold", rows[0])
			}
		}},
		{name: "full screen, every cell on the canvas", look: Screen(nord), check: func(t *testing.T, rows []string) {
			for i, row := range rows {
				if got := ansi.StringWidth(row); got != 20 {
					t.Errorf("row %d is %d cells wide, want the canvas's 20", i+1, got)
				}
				for _, run := range unpaintedRuns(row) {
					t.Errorf("row %d has %q off the canvas", i+1, run)
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, c.rows(tt.look))
		})
	}
}

func TestARunOfOneInkIsDrawnAtOnce(t *testing.T) {
	c := newCanvas(30, 1)
	c.line(1, 0, line{{"5 sessions · auto", mutedInk}})

	if row := c.rows(Screen(builtin(t, "nord")))[0]; !strings.Contains(row, "m5 sessions · auto") {
		t.Errorf("drew %q, want the line in one run: the blanks between its words drawn in its ink", row)
	}
}

func TestASurfaceKeepsTheGlyphsOnIt(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	tests := []struct {
		name string
		fade float64
		// want is the surface's colour, as a background's SGR.
		want string
	}{
		{name: "just laid", fade: 0, want: "48;2;61;64;70"},
		{name: "faded halfway into the canvas", fade: 0.5, want: "48;2;54;58;67"},
		{name: "faded away", fade: 1, want: "48;2;46;52;64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCanvas(12, 1)
			c.line(1, 0, line{{"14:41", secondaryInk}})
			c.surface(1, 0, 10, theme.BgAttention, tt.fade)

			row := c.rows(look)[0]
			if got := ansi.Strip(row); got != " 14:41      " {
				t.Errorf("drew %q, want the glyphs as they were", got)
			}
			if !strings.Contains(row, "38;2;229;233;240;"+tt.want+"m14:41") {
				t.Errorf("drew %q, want 14:41 in its colour on %s", row, tt.want)
			}
		})
	}
}

func TestASurfaceShowsInALookThatCantBlendUntilHalfFaded(t *testing.T) {
	look := Screen(builtin(t, "terminal"))
	for _, tt := range []struct {
		fade float64
		want bool
	}{{fade: 0.4, want: true}, {fade: 0.5, want: false}} {
		c := newCanvas(4, 1)
		c.text(0, 0, "ab", ink{})
		c.surface(0, 0, 2, theme.BgSelection, tt.fade)
		if got := strings.Contains(c.rows(look)[0], "\x1b[100"); got != tt.want {
			t.Errorf("faded %v, the surface shows: %v, want %v: %q", tt.fade, got, tt.want, c.rows(look)[0])
		}
	}
}

// unpaintedRuns are the runs of text in row drawn without the canvas, Nord's,
// under them.
func unpaintedRuns(row string) []string {
	var off []string
	for _, run := range regexp.MustCompile(`(\x1b\[[0-9;]*m)([^\x1b]+)`).FindAllStringSubmatch(row, -1) {
		if !strings.Contains(run[1], "48;2;") {
			off = append(off, run[2])
		}
	}
	return off
}
