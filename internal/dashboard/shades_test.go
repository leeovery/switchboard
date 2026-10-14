package dashboard

import (
	"fmt"
	"image/color"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
)

// A day is drawn in its level's shade, viz.day.0 for one with nothing on it
// to viz.day.4 for the busiest quarter; and where it takes a hue in four
// steps, viz.day.0 still at level 0, and the hue itself the third step.
func TestADayIsDrawnInItsLevelsShade(t *testing.T) {
	for level, want := range []theme.Token{theme.VizDay0, theme.VizDay1, theme.VizDay2, theme.VizDay3, theme.VizDay4} {
		if got := dayInk(level); got != (ink{token: want}) {
			t.Errorf("dayInk(%d) = %+v, want %s", level, got, want)
		}
	}
	look := Screen(builtin(t, "nord"))
	opus := familyHue("opus")
	if got := stepInk(opus, 0); got != dayInk(0) {
		t.Errorf("stepInk(opus, 0) = %+v, want viz.day.0, as dayInk(0) is %+v", got, dayInk(0))
	}
	if got, want := look.colour(stepInk(opus, 3)), look.colour(ink{token: theme.VizFamily1}); !sameColor(got, want) {
		t.Errorf("Opus's third step is %v, want its hue itself, %v", got, want)
	}
}

// A hue's four steps go from viz.day.0 40% and then 70% of the way to it, then
// the hue itself, then the hue mixed 55% toward white, as nord's frame of the
// Year filtered to Opus 5.5 draws them; or toward black on a light canvas. They
// take the same colours drawn on, as Days' swatches are.
func TestAHuesStepsGoFromTheEmptyDaysShadeToTheHueAndPast(t *testing.T) {
	tests := []struct {
		slug string
		hue  hue
		want [4]color.Color
	}{
		{slug: "nord", hue: familyHue("opus"), want: [4]color.Color{rgb(0x495B76), rgb(0x546E91), rgb(0x5E81AC), rgb(0xB7C6DA)}},
		{slug: "nord", hue: familyHue("fable"), want: [4]color.Color{rgb(0x6B6076), rgb(0x907792), rgb(0xB48EAD), rgb(0xDDCCDA)}},
		{slug: "tokyo-night-day", hue: hue{token: theme.AccentKey}, want: [4]color.Color{rgb(0x90A4D6), rgb(0x5F80D0), rgb(0x2D5CCA), rgb(0x14295B)}},
	}
	for _, tt := range tests {
		look := Screen(builtin(t, tt.slug))
		for i, want := range tt.want {
			level := i + 1
			if got := look.colour(stepInk(tt.hue, level)); !sameColor(got, want) {
				t.Errorf("%s: step %d of %s is %s, want %s", tt.slug, level, tt.hue.token, hexOf(got), hexOf(want))
			}
			if got := look.surface(ink{on: stepInk(tt.hue, level).hue()}); !sameColor(got, want) {
				t.Errorf("%s: step %d of %s drawn on is %s, want %s", tt.slug, level, tt.hue.token, hexOf(got), hexOf(want))
			}
		}
	}
}

// The terminal theme mixes nothing: a hue's steps are the hue itself, a
// glyph's density telling them apart.
func TestTheTerminalThemeDrawsEveryStepInTheHueItself(t *testing.T) {
	look := Screen(builtin(t, theme.Terminal))
	want := look.colour(ink{token: theme.VizFamily1})
	for level := 1; level < shadeLevels; level++ {
		if got := look.colour(stepInk(familyHue("opus"), level)); got != want {
			t.Errorf("step %d of Opus's hue is %v, want the hue itself, %v", level, got, want)
		}
	}
}

// A family takes its hue, Opus, Sonnet, Fable and Haiku in viz.family.1 to
// viz.family.4; one with none of its own, as Mythos, or one the version table
// doesn't know, text.muted.
func TestAFamilyIsDrawnInItsHue(t *testing.T) {
	for family, want := range map[string]theme.Token{
		"opus": theme.VizFamily1, "sonnet": theme.VizFamily2, "fable": theme.VizFamily3, "haiku": theme.VizFamily4,
		"mythos": theme.TextMuted, "claude-unknown-9": theme.TextMuted,
	} {
		if got := familyHue(family); got != (hue{token: want}) {
			t.Errorf("familyHue(%q) = %+v, want %s", family, got, want)
		}
	}
}

// A version the version table gives a shade is drawn in it, viz.version.1 to
// viz.version.6; an older one it gives none, as Haiku 4.5, in its family's
// hue mixed 30% toward the canvas; one the table doesn't know, in its
// family's hue, unmixed; and one of a family with no hue of its own, as
// Mythos, in text.muted with it, unmixed.
func TestAVersionIsDrawnInItsShade(t *testing.T) {
	version := func(id string) ledger.Version {
		v, ok := ledger.Pricing.Version(id)
		if !ok {
			t.Fatalf("the version table names no %s", id)
		}
		return v
	}
	tests := []struct {
		version ledger.Version
		want    hue
	}{
		{version: version("claude-opus-5-5"), want: hue{token: theme.VizVersion1}},
		{version: version("claude-opus-5"), want: hue{token: theme.VizVersion2}},
		{version: version("claude-sonnet-5"), want: hue{token: theme.VizVersion4}},
		{version: version("claude-haiku-5-5"), want: hue{token: theme.VizVersion6}},
		{version: version("claude-haiku-4-5"), want: hue{token: theme.VizFamily4, fade: theme.OlderFade}},
		{version: version("claude-opus-4-8"), want: hue{token: theme.VizFamily1, fade: theme.OlderFade}},
		{version: ledger.Version{Name: "opus-6", Family: "opus"}, want: hue{token: theme.VizFamily1}},
		{version: version("claude-mythos-5-1"), want: hue{token: theme.TextMuted}},
		{version: version("claude-mythos-5"), want: hue{token: theme.TextMuted}},
	}
	for _, tt := range tests {
		if got := versionHue(tt.version); got != tt.want {
			t.Errorf("versionHue(%s) = %+v, want %+v", tt.version.Name, got, tt.want)
		}
	}
}

// An older version the version table gives no shade is drawn as the theme
// draws an older one's: its family's hue mixed 30% toward the canvas, and its
// steps from there; or, in the terminal theme, which mixes nothing, faint in
// its family's colour, as the terminal theme draws Opus 5 and Sonnet 5.
func TestAnOlderVersionIsMixedTowardTheCanvasOrFaint(t *testing.T) {
	haiku45, _ := ledger.Pricing.Version("claude-haiku-4-5")
	older := versionHue(haiku45)
	nord := Screen(builtin(t, "nord"))
	mixed := theme.Mix(rgb(0xD8DEE9), rgb(0x2E3440), 0.3)
	if got := nord.colour(older.ink()); !sameColor(got, mixed) {
		t.Errorf("Haiku 4.5 in nord is %s, want Haiku's hue mixed 30%% toward the canvas, %s", hexOf(got), hexOf(mixed))
	}
	if got, want := nord.colour(stepInk(older, 1)), theme.Mix(rgb(0x3B4252), mixed, 0.4); !sameColor(got, want) {
		t.Errorf("Haiku 4.5's first step in nord is %s, want 40%% of the way from viz.day.0 to its shade, %s", hexOf(got), hexOf(want))
	}
	if nord.faint(older.ink()) {
		t.Error("Haiku 4.5 is drawn faint in nord, want it mixed toward the canvas instead")
	}
	term := Screen(builtin(t, theme.Terminal))
	opus5, _ := ledger.Pricing.Version("claude-opus-5")
	opus55, _ := ledger.Pricing.Version("claude-opus-5-5")
	for _, tt := range []struct {
		name  string
		k     ink
		faint bool
		like  theme.Token
	}{
		{name: "Haiku 4.5", k: older.ink(), faint: true, like: theme.VizFamily4},
		{name: "Haiku 4.5's third step", k: stepInk(older, 3), faint: true, like: theme.VizFamily4},
		{name: "Opus 5", k: versionHue(opus5).ink(), faint: true, like: theme.VizFamily1},
		{name: "Opus 5.5", k: versionHue(opus55).ink(), like: theme.VizFamily1},
	} {
		if got := term.faint(tt.k); got != tt.faint {
			t.Errorf("in the terminal theme, %s is drawn faint: %v, want %v", tt.name, got, tt.faint)
		}
		if got, want := term.colour(tt.k), term.colour(ink{token: tt.like}); got != want {
			t.Errorf("in the terminal theme, %s is %v, want its family's colour, %v", tt.name, got, want)
		}
	}
}

// Where the look draws no shades of its own, a day's level is told by a
// glyph's density: · for none, then ░, ▒, ▓ and █.
func TestADaysLevelIsToldByAGlyphsDensity(t *testing.T) {
	for level, want := range []string{"·", "░", "▒", "▓", "█"} {
		if got := shadeGlyph(level); got != want {
			t.Errorf("shadeGlyph(%d) = %q, want %q", level, got, want)
		}
	}
}

// hexOf is c written #RRGGBB, or "none" for no colour.
func hexOf(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
