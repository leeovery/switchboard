package dashboard

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

// barIn is b drawn cells long in look, on a canvas of its own.
func barIn(b usageBar, look Look, cells int) *canvas {
	c := newCanvas(cells, 1)
	b.draw(c, look, 0, 0, cells)
	return c
}

// glyphs are the glyphs of the canvas's first row.
func glyphs(c *canvas) string {
	var s strings.Builder
	for _, here := range c.cells[0] {
		s.WriteString(here.glyph)
	}
	return s.String()
}

// themed is the look usage prints the built-in theme with the given slug in.
func themed(t *testing.T, slug string) Look {
	t.Helper()
	th, ok := theme.Builtin(slug)
	if !ok {
		t.Fatalf("no built-in theme %s", slug)
	}
	return Print(th, nil)
}

func TestABarFillsInEighthsOfACellAHalfToEven(t *testing.T) {
	tests := []struct {
		name  string
		used  float64
		cells int
		want  string
	}{
		{name: "half", used: 0.5, cells: 8, want: "████    "},
		{name: "2.5 eighths, to 2", used: 2.5 / 128, cells: 16, want: "▎" + strings.Repeat(" ", 15)},
		{name: "3.5 eighths, to 4", used: 3.5 / 128, cells: 16, want: "▌" + strings.Repeat(" ", 15)},
		{name: "a part-filled cell", used: 0.39, cells: 16, want: "██████▎" + strings.Repeat(" ", 9)},
		{name: "over 100% fills it", used: 1.3, cells: 4, want: "████"},
		{name: "below 0 fills nothing", used: -0.2, cells: 4, want: "    "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := glyphs(barIn(usageBar{used: tt.used, capAt: 1}, Look{}, tt.cells)); got != tt.want {
				t.Errorf("used %v over %d cells is %q, want %q", tt.used, tt.cells, got, tt.want)
			}
		})
	}
}

func TestABarsCellsStepAlongTheRampAHalfToEven(t *testing.T) {
	tests := []struct {
		i, cells int
		want     float64
	}{
		{i: 0, cells: 16, want: 0}, {i: 1, cells: 16, want: 1.0 / 8}, {i: 15, cells: 16, want: 1},
		{i: 1, cells: 17, want: 0}, {i: 3, cells: 17, want: 2.0 / 8}, {i: 5, cells: 17, want: 2.0 / 8}, {i: 7, cells: 17, want: 4.0 / 8},
		{i: 0, cells: 1, want: 0},
	}
	for _, tt := range tests {
		if got := rampStep(tt.i, tt.cells); got != tt.want {
			t.Errorf("cell %d of %d sits %v along the ramp, want %v", tt.i, tt.cells, got, tt.want)
		}
	}
	nord := themed(t, "nord")
	c := barIn(usageBar{used: 1, capAt: 1}, nord, 17)
	if got, want := c.at(1, 0).ink.on, (hue{ramp: true, at: 0}); got != want {
		t.Errorf("a full bar's second cell of 17 is on %+v, want the ramp's first step", got)
	}
}

func TestTheCapsMarkSitsOnItsCellsOwnBackground(t *testing.T) {
	nord := themed(t, "nord")
	tests := []struct {
		name  string
		bar   usageBar
		cells int
		at    int
		on    hue
	}{
		{name: "on the fill", bar: usageBar{used: 0.5, capAt: 0.25}, cells: 16, at: 4, on: hue{ramp: true, at: rampStep(4, 16)}},
		{name: "on the track", bar: usageBar{used: 0.5, capAt: 0.75}, cells: 16, at: 12, on: hue{token: theme.VizTrack}},
		{name: "on the cell the fill ends in", bar: usageBar{used: 99.0 / 128, capAt: 0.75}, cells: 16, at: 12, on: hue{token: theme.VizTrack}},
		{name: "at 85.5 of 90 cells, at 86", bar: usageBar{used: 0.2, capAt: 0.95}, cells: 90, at: 86, on: hue{token: theme.VizTrack}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := barIn(tt.bar, nord, tt.cells)
			here := c.at(tt.at, 0)
			if want := (ink{token: theme.VizReserve, on: tt.on}); here.glyph != capMark || here.ink != want {
				t.Errorf("cell %d is %q in %+v, want %q in %+v", tt.at, here.glyph, here.ink, capMark, want)
			}
			if n := strings.Count(glyphs(c), capMark); n != 1 {
				t.Errorf("the bar has %d cap's marks, want 1", n)
			}
		})
	}
	if got := glyphs(barIn(usageBar{used: 0.5, capAt: 1}, nord, 16)); strings.Contains(got, capMark) {
		t.Errorf("a bar without a cap reads %q, want no cap's mark", got)
	}
}

func TestEvenPacesMarkIsDrawnLastInBoldOnNoBackground(t *testing.T) {
	nord := themed(t, "nord")
	pace := func(p float64) *float64 { return &p }
	tests := []struct {
		name  string
		bar   usageBar
		cells int
		at    int
	}{
		{name: "over the fill, at 8.5 of 16 cells, at 8", bar: usageBar{used: 0.9, capAt: 1, pace: pace(8.5 / 16)}, cells: 16, at: 8},
		{name: "over the cap's mark", bar: usageBar{used: 0.2, capAt: 0.75, pace: pace(0.75)}, cells: 16, at: 12},
		{name: "never past the last cell", bar: usageBar{used: 0.2, capAt: 1, pace: pace(1)}, cells: 16, at: 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			here := barIn(tt.bar, nord, tt.cells).at(tt.at, 0)
			if want := (ink{token: theme.VizPace, bold: true}); here.glyph != paceMark || here.ink != want {
				t.Errorf("cell %d is %q in %+v, want %q in %+v", tt.at, here.glyph, here.ink, paceMark, want)
			}
		})
	}
}

func TestAnEmptyBarIsItsTrackAlone(t *testing.T) {
	half := 0.5
	got := glyphs(barIn(usageBar{used: 0.6, capAt: 0.9, pace: &half, empty: true}, Look{}, 10))
	if got != strings.Repeat(" ", 10) {
		t.Errorf("an empty bar reads %q, want its track alone, no fill or marks", got)
	}
}

func TestAStaleBarKeeps35PercentOfItsColourAndNothingBold(t *testing.T) {
	half := 0.5
	c := barIn(usageBar{used: 0.33, capAt: 0.9, pace: &half, stale: true}, themed(t, "nord"), 10)
	for x := range 10 {
		k := c.at(x, 0).ink
		if k.fade != staleFade || k.bold || !k.dim || (k.on != (hue{}) && k.on.fade != staleFade) {
			t.Errorf("cell %d is drawn in %+v, want it and its surface faded 65%% toward the canvas, dim, not bold", x, k)
		}
	}
}

func TestABarOutOfThePoolFadesAsAStaleOneAndTheTwoCompound(t *testing.T) {
	half := 0.5
	tests := []struct {
		name       string
		stale, out bool
		// kept is how much of its colour the bar keeps.
		kept float64
	}{
		{name: "neither", kept: 1},
		{name: "stale", stale: true, kept: 0.35},
		{name: "out of the pool", out: true, kept: 0.35},
		{name: "both, 35% of 35%", stale: true, out: true, kept: 0.35 * 0.35},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := barIn(usageBar{used: 0.33, capAt: 0.9, pace: &half, stale: tt.stale, out: tt.out}, themed(t, "nord"), 10)
			faded := tt.kept < 1
			for x := range 10 {
				k := c.at(x, 0).ink
				if math.Abs(1-k.fade-tt.kept) > 1e-9 || k.dim != faded || (faded && k.bold) || (k.on != (hue{}) && math.Abs(1-k.on.fade-tt.kept) > 1e-9) {
					t.Errorf("cell %d is drawn in %+v, want it and its surface keeping %v of its colour, dim and not bold where faded", x, k, tt.kept)
				}
			}
		})
	}
}

func TestABarEasingFillsAsFarAsItHasEased(t *testing.T) {
	eased := 0.25
	bar := usageBar{used: 0.75, capAt: 1, eased: &eased}
	if got, want := glyphs(barIn(bar, Look{}, 8)), "██      "; got != want {
		t.Errorf("easing at 25%% to 75%%, the bar reads %q, want %q", got, want)
	}
	if got := bar.percentage().plain(); got != "75%" {
		t.Errorf("easing, the percentage reads %q, want the reading, 75%%", got)
	}
}

func TestABarWithoutColourIsDrawnInBlocks(t *testing.T) {
	half := 0.5
	bar := usageBar{used: 0.33, capAt: 0.9, pace: &half}
	row := barIn(bar, NoColour(), 10).rows(NoColour())[0]
	if got, want := ansi.Strip(row), "███▎ ┃   ╎"; got != want {
		t.Errorf("without colour the bar reads %q, want its fill in blocks, its track as spaces and its marks", got)
	}
	if strings.Contains(row, "38;") || strings.Contains(row, "48;") {
		t.Errorf("without colour the bar is drawn %q, want no colour", row)
	}
	if !strings.Contains(row, "\x1b[1m┃") {
		t.Errorf("without colour the bar is drawn %q, want even pace's mark bold", row)
	}
}

func TestTheTerminalThemesBarIsDrawnInGlyphsItsTrackFaint(t *testing.T) {
	look := themed(t, theme.Terminal)
	half := 0.5
	c := barIn(usageBar{used: 0.33, capAt: 0.9, pace: &half}, look, 10)
	if got, want := glyphs(c), "███▎░┃░░░╎"; got != want {
		t.Errorf("in the terminal theme the bar reads %q, want %q", got, want)
	}
	fill, track := c.at(0, 0).ink, c.at(4, 0).ink
	if !fill.ramp || look.faint(fill) || fill.on != (hue{}) {
		t.Errorf("its fill is drawn in %+v, want the ramp's colour, on no surface, not faint", fill)
	}
	if !look.faint(track) {
		t.Errorf("its track is drawn in %+v, want it faint", track)
	}
	stale := barIn(usageBar{used: 0.33, capAt: 0.9, pace: &half, stale: true}, look, 10)
	for x := range 10 {
		if k := stale.at(x, 0).ink; !look.faint(k) {
			t.Errorf("stale, cell %d is drawn in %+v, want it faint, as the theme fades nothing", x, k)
		}
	}
}
