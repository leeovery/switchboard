package theme_test

import (
	"image/color"
	"math"
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheBuiltIns(t *testing.T) {
	var slugs []string
	for _, b := range theme.Builtins() {
		slugs = append(slugs, b.Slug)
	}
	if want := []string{"amber", "exchange", "nord", "terminal", "tokyo-night", "tokyo-night-day"}; !slices.Equal(slugs, want) {
		t.Errorf("Builtins() = %q, want %q", slugs, want)
	}
	for _, slug := range []string{theme.DefaultLight, theme.DefaultDark, theme.Terminal} {
		if _, ok := theme.Builtin(slug); !ok {
			t.Errorf("Builtin(%q) found none", slug)
		}
	}
	if _, ok := theme.Builtin("solarized"); ok {
		t.Error("Builtin(solarized) found one, want none")
	}
}

func TestEveryBuiltInGivesEveryToken(t *testing.T) {
	for _, b := range theme.Builtins() {
		t.Run(b.Slug, func(t *testing.T) {
			for tok := theme.TextPrimary; tok <= theme.VizSeries6; tok++ {
				if b.Colour(tok) == nil && !leftToTheTerminal(b, tok) {
					t.Errorf("%s has no %s", b.Slug, tok)
				}
			}
		})
	}
}

// leftToTheTerminal reports whether the terminal theme leaves tok as the
// terminal has it: its text in its own foreground, on its own background.
func leftToTheTerminal(b theme.Theme, tok theme.Token) bool {
	if b.Slug != theme.Terminal {
		return false
	}
	return slices.Contains([]theme.Token{
		theme.TextPrimary, theme.TextSecondary, theme.TextTertiary, theme.TextOnSelection,
		theme.Canvas, theme.BgAttention, theme.BgSubtle, theme.VizPace, theme.VizSeries5,
	}, tok)
}

func TestNordIsPortalsWithTheDashboardsOwnBars(t *testing.T) {
	nord, _ := theme.Builtin("nord")
	for tok, want := range map[theme.Token]string{
		theme.TextPrimary:      "#ECEFF4",
		theme.TextTertiary:     "#D8DEE9",
		theme.TextMuted:        "#939EB2",
		theme.StatePositive:    "#A7C492",
		theme.StateDestructive: "#DD8188",
		theme.Canvas:           "#2E3440",
		theme.Border:           "#4C566A",
		theme.AccentMode:       "#88C0D0",
		theme.AccentPrimary:    "#B48EAD",
		theme.VizRamp1:         "#A3BE8C",
		theme.VizRamp2:         "#EBCB8B",
		theme.VizRamp3:         "#D08770",
		theme.VizRamp4:         "#BF616A",
		theme.VizTrack:         "#4C566A",
		theme.VizPace:          "#ECEFF4",
		theme.VizReserve:       "#81A1C1",
		theme.VizSeries1:       "#81A1C1",
	} {
		if got := hex(nord.Colour(tok)); got != want {
			t.Errorf("nord's %s = %s, want %s", tok, got, want)
		}
	}
}

func TestTheBuiltInsAsTheirSheetsDrawThem(t *testing.T) {
	tests := []struct {
		slug string
		want map[theme.Token]string
	}{
		{slug: "tokyo-night", want: map[theme.Token]string{
			theme.Canvas: "#0B0C14", theme.TextPrimary: "#C0CAF5", theme.AccentAttention: "#FF9E64",
			theme.VizRamp1: "#9ECE6A", theme.VizRamp2: "#E0AF68", theme.VizRamp3: "#FF9E64", theme.VizRamp4: "#F7768E",
		}},
		{slug: "tokyo-night-day", want: map[theme.Token]string{
			theme.Canvas: "#E1E2E7", theme.TextPrimary: "#2E3C64", theme.Border: "#C9CDDB",
			theme.VizRamp1: "#587539", theme.VizRamp2: "#8C6C3E", theme.VizRamp3: "#B15C00", theme.VizRamp4: "#BD2545", theme.VizTrack: "#C9CDDB",
		}},
		{slug: "amber", want: map[theme.Token]string{
			theme.Canvas: "#0E0B06", theme.TextPrimary: "#FFD27A", theme.StateDestructive: "#FF5A36",
			theme.VizRamp1: "#B07A00", theme.VizRamp4: "#FF5A36", theme.VizTrack: "#2A1E05", theme.VizPace: "#FFE7B0", theme.VizReserve: "#FFD27A",
		}},
		{slug: "exchange", want: map[theme.Token]string{
			theme.Canvas: "#1B1714", theme.TextPrimary: "#F4EEDF", theme.AccentKey: "#C9A44C",
			theme.VizRamp3: "#E07A3F", theme.VizReserve: "#4F86C6", theme.VizTrack: "#5A4D3E", theme.VizPace: "#F4EEDF",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			b, _ := theme.Builtin(tt.slug)
			for tok, want := range tt.want {
				if got := hex(b.Colour(tok)); got != want {
					t.Errorf("%s's %s = %s, want %s", tt.slug, tok, got, want)
				}
			}
			for n := 1; n <= 6; n++ {
				if b.Series(n) == b.Colour(theme.StateDestructive) {
					t.Errorf("%s's series %d is its state.destructive, which a limit or a refusal is drawn in", tt.slug, n)
				}
			}
		})
	}
}

func TestEveryBuiltInsTextReadsOnItsSurfaces(t *testing.T) {
	for _, b := range theme.Builtins() {
		if !b.Paints() {
			continue
		}
		for _, pair := range [][2]theme.Token{{theme.TextPrimary, theme.Canvas}, {theme.TextOnSelection, theme.BgSelection}} {
			if ratio := contrast(b.Colour(pair[0]), b.Colour(pair[1])); ratio < 4.5 {
				t.Errorf("%s's %s on its %s contrasts %.2f to 1, want 4.5 at least, as text must", b.Slug, pair[0], pair[1], ratio)
			}
		}
	}
}

// contrast is the WCAG contrast ratio of a colour against another.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// luminance is a colour's relative luminance, as WCAG works it out.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	linear := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

func TestTheDefaultPairIsALightAndADarkTheme(t *testing.T) {
	light, _ := theme.Builtin(theme.DefaultLight)
	dark, _ := theme.Builtin(theme.DefaultDark)
	if theme.Dark(light.Colour(theme.Canvas)) {
		t.Errorf("the light default, %s, has a dark canvas", light.Slug)
	}
	if !theme.Dark(dark.Colour(theme.Canvas)) {
		t.Errorf("the dark default, %s, has a light canvas", dark.Slug)
	}
}

func TestTheTerminalThemeIsTheTerminalsOwn(t *testing.T) {
	term, _ := theme.Builtin(theme.Terminal)
	if term.Paints() {
		t.Error("the terminal theme paints its canvas, want the terminal's own background left as it is")
	}
	for tok := theme.TextPrimary; tok <= theme.VizSeries6; tok++ {
		c := term.Colour(tok)
		if _, basic := c.(ansi.BasicColor); c != nil && !basic {
			t.Errorf("the terminal theme's %s is %#v, want one of the terminal's own sixteen colours, or its own", tok, c)
		}
	}
	for _, b := range theme.Builtins() {
		if b.Slug != theme.Terminal && !b.Paints() {
			t.Errorf("%s paints no canvas, want only the terminal theme to leave it", b.Slug)
		}
	}
}

func TestDark(t *testing.T) {
	tests := []struct {
		name       string
		background color.Color
		want       bool
	}{
		{name: "no answer, taken for dark", want: true},
		{name: "black", background: color.Black, want: true},
		{name: "white", background: color.White},
		{name: "nord's canvas", background: color.RGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xff}, want: true},
		{name: "tokyo night day's canvas", background: color.RGBA{R: 0xE1, G: 0xE2, B: 0xE7, A: 0xff}},
		{name: "a mid grey, just dark", background: color.RGBA{R: 0x7F, G: 0x7F, B: 0x7F, A: 0xff}, want: true},
		{name: "a mid grey, just light", background: color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}},
		{name: "saturated blue, its lightness half", background: color.RGBA{B: 0xff, A: 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := theme.Dark(tt.background); got != tt.want {
				t.Errorf("Dark(%v) = %v, want %v", tt.background, got, tt.want)
			}
		})
	}
}

func TestMix(t *testing.T) {
	green, red := color.RGBA{R: 0xA3, G: 0xBE, B: 0x8C, A: 0xff}, color.RGBA{R: 0xBF, G: 0x61, B: 0x6A, A: 0xff}
	tests := []struct {
		t    float64
		want string
	}{
		{t: 0, want: "#A3BE8C"},
		{t: 1, want: "#BF616A"},
		{t: 0.5, want: "#B1907B"},
	}
	for _, tt := range tests {
		if got := hex(theme.Mix(green, red, tt.t)); got != tt.want {
			t.Errorf("Mix(green, red, %v) = %s, want %s", tt.t, got, tt.want)
		}
	}
}
