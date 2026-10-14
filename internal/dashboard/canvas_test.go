package dashboard

import (
	"image/color"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
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
			c.surface(1, 0, 10, hue{token: theme.BgAttention, fade: tt.fade})

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
		c.surface(0, 0, 2, hue{token: theme.BgSelection, fade: tt.fade})
		if got := strings.Contains(c.rows(look)[0], "\x1b[7m"); got != tt.want {
			t.Errorf("faded %v, the surface shows: %v, want %v: %q", tt.fade, got, tt.want, c.rows(look)[0])
		}
	}
}

func TestWithoutColourASurfaceShowsOnlyOnBgSelectionAndThereReversed(t *testing.T) {
	for tok := theme.TextPrimary; tok.String() != ""; tok++ {
		c := newCanvas(4, 1)
		c.text(0, 0, "ab", ink{})
		c.surface(0, 0, 2, hue{token: tok})
		want := "ab"
		if tok == theme.BgSelection {
			want = "\x1b[7mab\x1b[m"
		}
		if got := c.rows(NoColour())[0]; got != want {
			t.Errorf("on %s, drew %q, want %q", tok, got, want)
		}
	}
	for _, tt := range []struct {
		name string
		on   hue
		draw func(*canvas)
	}{
		{name: "a bar's ramp", on: hue{ramp: true, at: 0.5}},
		{name: "bg.selection faded halfway", on: hue{token: theme.BgSelection, fade: 0.5}},
		{name: "bg.selection behind a card", on: hue{token: theme.BgSelection}, draw: func(c *canvas) { c.fade() }},
	} {
		c := newCanvas(4, 1)
		c.text(0, 0, "ab", ink{})
		c.surface(0, 0, 2, tt.on)
		if tt.draw != nil {
			tt.draw(c)
		}
		if got := c.rows(NoColour())[0]; got != "ab" {
			t.Errorf("%s, drew %q, want %q, on no surface", tt.name, got, "ab")
		}
	}
	if got := NoColour().Canvas(); got != nil {
		t.Errorf("Canvas() = %v, want none painted", got)
	}
}

func TestEachSelectedRowIsReversedWithoutColourAndOnBgSelectionInColour(t *testing.T) {
	nord, terminal := Screen(builtin(t, "nord")), Screen(builtin(t, theme.Terminal))
	selection := hue{token: theme.BgSelection}
	cursor := routedCard(func(r *RoutingCard) { r.Cursor = 2 })
	card, _ := drawnLit(Frame{Width: 160, Height: 37}, cursor)
	phoneCard, _ := drawnLit(Frame{Width: 52, Height: 40}, cursor)
	focused := frameOf(160, 60)
	focused.Sections, focused.Focus = Sections{}.Toggled(SectionGraphs), "personal"
	_, usage := usageBody(focused, threeRouted(), now, 11)
	titled := func(width int) *canvas {
		f := Frame{Width: width, Height: 2, View: Sessions}
		c := newCanvas(f.Width, f.Height)
		f.title(c, now)
		return c
	}
	tests := []struct {
		name string
		c    *canvas
		// rows are the rows on bg.selection.
		rows []int
	}{
		{name: "the routing card's cursor", c: card, rows: []int{15}},
		{name: "on a phone, the cursor's account's both lines", c: phoneCard, rows: []int{16, 17}},
		{name: "the Overview's focus", c: usage, rows: []int{4}},
		{name: "the title's tab", c: titled(160), rows: []int{0}},
		{name: "on a phone, the title's tab", c: titled(52), rows: []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var selected []int
			for y, row := range tt.c.cells {
				lit := false
				for x, here := range row {
					k := here.ink
					if k.on != selection {
						if NoColour().reversed(k) {
							t.Errorf("cell %d of row %d is reversed under NO_COLOR, on %+v, want only bg.selection reversed", x+1, y+1, k.on)
						}
						continue
					}
					lit = true
					if !NoColour().reversed(k) {
						t.Errorf("cell %d of row %d isn't reversed under NO_COLOR, as the terminal theme draws bg.selection", x+1, y+1)
					}
					if nord.reversed(k) || !sameColor(nord.surface(k), nord.theme.Colour(theme.BgSelection)) {
						t.Errorf("in nord, cell %d of row %d is on %v, reversed %v, want bg.selection's colour", x+1, y+1, nord.surface(k), nord.reversed(k))
					}
					if !terminal.reversed(k) {
						t.Errorf("in the terminal theme, cell %d of row %d isn't reversed", x+1, y+1)
					}
				}
				if lit {
					selected = append(selected, y)
				}
			}
			if !slices.Equal(selected, tt.rows) {
				t.Errorf("rows %v are on bg.selection, want %v", selected, tt.rows)
			}
			if got := reversedRows(tt.c.rows(NoColour())); !slices.Equal(got, tt.rows) {
				t.Errorf("under NO_COLOR, rows %v are drawn reversed, want %v, those on bg.selection", got, tt.rows)
			}
		})
	}
}

// reversedRows are the rows, by index, drawn with an SGR that reverses.
func reversedRows(rows []string) []int {
	var reversed []int
	for y, row := range rows {
		for _, sgr := range regexp.MustCompile(`\x1b\[([0-9;]*)m`).FindAllStringSubmatch(row, -1) {
			if slices.Contains(strings.Split(sgr[1], ";"), "7") {
				reversed = append(reversed, y)
				break
			}
		}
	}
	return reversed
}

func TestThePageBehindACardIsFadedAsItsFrameDrawsIt(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	doc, sessions := routedWorld(false)
	f := Frame{
		Width: 160, Height: 37, Look: nord, View: Overview, Policy: claudeLike, Sessions: sessions,
		Card: RoutingCard{Rows: RoutingRows(doc.At(routedAt, claudeLike), sessions, views.Stream{})},
	}

	heldTo(t, "routing-auto-160", reference(t, "routing-auto-160"), screenOf(t, f.Draw(doc, routedAt)), region{top: 0, bottom: 2, left: 0, right: 159}, shadeOf(nord.Canvas()), nil, nil)
}

func TestEveryCellOfThePageBehindACardIsFadedAndNothingInItBold(t *testing.T) {
	f := frameOf(160, 40)
	f.Look = Screen(builtin(t, "nord"))
	doc := threeRouted()
	page := screenOf(t, f.Draw(doc, now))
	f.Card = RoutingCard{Rows: RoutingRows(doc.At(now, claudeLike), nil, views.Stream{})}
	behind := screenOf(t, f.Draw(doc, now))

	k := f.Card.card(f)
	height := len(k.body) + chrome(true)
	x, y := (f.Width-k.width)/2, (f.Height-height)/2
	canvas := f.Look.Canvas()
	faded := func(s shade) shade {
		if !s.set {
			return s
		}
		return shadeOf(theme.Mix(color.RGBA{R: s.r, G: s.g, B: s.b, A: 0xff}, canvas, behindFade))
	}
	for row := range page {
		for col, was := range page[row] {
			if row >= y && row < y+height && col >= x && col < x+k.width {
				continue
			}
			want := seen{glyph: was.glyph, fg: faded(was.fg), bg: faded(was.bg.or(shadeOf(canvas)))}
			got := behind[row][col]
			if want.glyph == ' ' {
				want.fg, got.fg = shade{}, shade{}
			}
			if got != want {
				t.Errorf("row %d, column %d is %s, want %s: the page's %s faded 80%% into the canvas, not bold", row+1, col+1, got, want, was)
			}
		}
	}
	if title := behind[y][x+3]; title.fg != shadeOf(f.Look.theme.Colour(theme.TextSubtle)) {
		t.Errorf("the card's title is %s, want it in text.subtle, not faded", title)
	}
}

func TestAFadeBehindACardComesAfterTheCellsOwn(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	nord, canvas := look.theme, look.Canvas()
	c := newCanvas(1, 1)
	c.text(0, 0, "x", ink{token: theme.AccentKey, fade: staleFade, glow: 0.5, on: hue{token: theme.BgAttention, fade: 0.3}, bold: true})
	c.fade()
	faded := c.cells[0][0].ink

	own := theme.Mix(theme.Mix(nord.Colour(theme.AccentKey), nord.Colour(theme.TextPrimary), 0.5), canvas, staleFade)
	if got, want := look.colour(faded), theme.Mix(own, canvas, behindFade); !sameColor(got, want) {
		t.Errorf("its colour is %v, want %v: its glow, then its own fade, then 80%% into the canvas", got, want)
	}
	ownSurface := theme.Mix(nord.Colour(theme.BgAttention), canvas, 0.3)
	if got, want := look.surface(faded), theme.Mix(ownSurface, canvas, behindFade); !sameColor(got, want) {
		t.Errorf("its surface is %v, want %v: its own fade, then 80%% into the canvas", got, want)
	}
	if faded.bold {
		t.Error("it's bold behind the card, want nothing bold")
	}
}

func TestAFadeLeavesTheRowsKeptAsTheyreDrawn(t *testing.T) {
	c := newCanvas(2, 3)
	for y := range 3 {
		c.text(0, y, "ab", ink{token: theme.AccentKey, bold: true})
	}
	c.fade(1)
	for y, row := range c.cells {
		if kept := y == 1; row[0].ink.behind == kept || row[0].ink.bold != kept {
			t.Errorf("row %d is behind the card %v and bold %v, want it %v and %v", y+1, row[0].ink.behind, row[0].ink.bold, !kept, kept)
		}
	}
}

func TestBehindACardALookThatCantBlendDrawsThePageFaint(t *testing.T) {
	look := Screen(builtin(t, "terminal"))
	c := newCanvas(30, 1)
	c.line(0, 0, line{{" Overview ", ink{token: theme.TextPrimary, on: hue{token: theme.BgSelection}, bold: true}}, {"▲ side", nameInk}})
	c.fade()

	for _, sgr := range regexp.MustCompile(`\x1b\[([0-9;]*)m`).FindAllStringSubmatch(c.rows(look)[0], -1) {
		codes := strings.Split(sgr[1], ";")
		if sgr[1] == "" || slices.Equal(codes, []string{"0"}) {
			continue
		}
		if !slices.Contains(codes, "2") || slices.ContainsFunc(codes, func(code string) bool { return code == "1" || code == "7" || code == "48" }) {
			t.Errorf("drew %q behind the card, want it faint, on no surface, nothing bold", sgr[0])
		}
	}
}

func TestBehindACardWithoutColourThePageLosesOnlyItsBold(t *testing.T) {
	c := newCanvas(20, 1)
	c.line(0, 0, line{{"work", titleInk}, spaces(2), {"healthy", secondaryInk}})
	c.fade()

	if got, want := c.rows(NoColour())[0], "work  healthy"; got != want {
		t.Errorf("drew %q, want %q: its words as they were, nothing bold", got, want)
	}
}

// A canvas within bounds, within others, draws only where both hold: an
// inner clip never reaches past the outer.
func TestACanvasWithinBoundsWithinOthersDrawsOnlyWhereBothHold(t *testing.T) {
	c := newCanvas(6, 4)
	inner := c.within(bounds{x: 1, y: 1, width: 4, rows: 2}).within(bounds{x: 0, y: 0, width: 3, rows: 4})
	for y := range 4 {
		inner.text(0, y, "xxxxxx", ink{})
	}
	want := []string{"", " xx", " xx", ""}
	for y, w := range want {
		if got := rowOf(c, y); got != w {
			t.Errorf("row %d reads %q, want %q", y+1, got, w)
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
