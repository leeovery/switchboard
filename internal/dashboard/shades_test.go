package dashboard

import (
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
// viz.version.6; one of a family with no hue of its own, as Mythos, in
// text.muted with it.
func TestAVersionIsDrawnInItsShade(t *testing.T) {
	tests := []struct {
		version ledger.Version
		want    theme.Token
	}{
		{version: ledger.Version{Name: "Opus 5.5", Family: "opus", Shade: 1}, want: theme.VizVersion1},
		{version: ledger.Version{Name: "Sonnet 5", Family: "sonnet", Shade: 4}, want: theme.VizVersion4},
		{version: ledger.Version{Name: "Haiku 5.5", Family: "haiku", Shade: 6}, want: theme.VizVersion6},
		{version: ledger.Version{Name: "Mythos 5.1", Family: "mythos"}, want: theme.TextMuted},
	}
	for _, tt := range tests {
		if got := versionHue(tt.version); got != (hue{token: tt.want}) {
			t.Errorf("versionHue(%s) = %+v, want %s", tt.version.Name, got, tt.want)
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
