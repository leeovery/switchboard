package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheGlyphLineExplainsTheGlyphs(t *testing.T) {
	f := frameOf(160, 40)
	f.Look = Screen(builtin(t, "nord"))
	got := f.glyphLine(nil)
	if want := "KEY      ▆▆ room left   ⠂⠄⡀ heading   ✕ runs out   ⠠ ⠠ reserve   │   ████ used   ██ heading   ┃ even pace   ╎ reserve   ● session, lit while busy"; got.plain() != want {
		t.Errorf("the glyph line reads\n%q\nwant\n%q", got.plain(), want)
	}
	inks := make(map[string]ink)
	for _, s := range got {
		if _, ok := inks[s.text]; !ok {
			inks[s.text] = s.ink
		}
	}
	for _, tt := range []struct {
		glyph string
		want  ink
	}{
		{glyph: "KEY", want: labelInk},
		{glyph: "▆▆", want: ink{token: theme.StatePositive, fade: projectionFade}},
		{glyph: "⠂⠄⡀", want: ink{token: theme.StatePositive}},
		{glyph: "✕", want: outInk},
		{glyph: "⠠ ⠠", want: ink{token: theme.VizReserve, fade: reserveFade}},
		{glyph: "██", want: ink{ramp: true, at: 1.0 / 3, fade: projectionFade}},
		{glyph: "┃", want: ink{token: theme.VizPace, bold: true}},
		{glyph: "╎", want: reserveInk},
		{glyph: "●", want: positiveInk},
		{glyph: " room left", want: mutedInk},
		{glyph: groupRule, want: faintInk},
	} {
		if inks[tt.glyph] != tt.want {
			t.Errorf("%q is drawn in %+v, want %+v, as the view draws it", tt.glyph, inks[tt.glyph], tt.want)
		}
	}
	var used []ink
	for _, s := range got {
		if s.text == "█" {
			used = append(used, s.ink)
		}
	}
	if want := []ink{{ramp: true}, {ramp: true, at: 1.0 / 3}, {ramp: true, at: 2.0 / 3}, {ramp: true, at: 1}}; !slices.Equal(used, want) {
		t.Errorf("used is drawn in %+v, want the ramp's stops, %+v", used, want)
	}
}

func TestTheGlyphLineExplainsTheGlyphsOfTheChartsStyle(t *testing.T) {
	tests := []struct {
		style  Chart
		want   string
		glyphs map[string]ink
	}{
		{
			style: BurnRate,
			want:  "KEY      ▃▅ use per 10 min   ▆▆ too fast to last   ⠂⠂⠂ fastest that lasts   │   ████ used",
			glyphs: map[string]ink{
				"▃▅":  {token: theme.StatePositive, fade: projectionFade},
				"▆▆":  fastInk,
				"⠂⠂⠂": lastingInk,
			},
		},
		{
			style: Hourglass,
			want:  "KEY      ▀▀ room left   ▄▄ used   ▐▌ its recent rate, falling while busy   │   ████ used",
			glyphs: map[string]ink{
				"▀▀": {token: theme.StatePositive, fade: projectionFade},
				"▄▄": {token: theme.StatePositive, fade: projectionFade},
				"▐▌": {token: theme.StatePositive},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.style.Name(), func(t *testing.T) {
			f := frameOf(160, 40)
			f.Look, f.Chart = Screen(builtin(t, "nord")), tt.style
			got := f.glyphLine(nil)
			if !strings.HasPrefix(got.plain(), tt.want) || !strings.HasSuffix(got.plain(), "● session, lit while busy") {
				t.Errorf("the glyph line reads\n%q\nwant it to start\n%q\nand go on to the bars' glyphs", got.plain(), tt.want)
			}
			if !f.glyphsFit(nil) {
				t.Errorf("the glyph line is %d cells wide, want it to fit 160 columns", got.width())
			}
			for _, s := range got {
				if want, ok := tt.glyphs[s.text]; ok && s.ink != want {
					t.Errorf("%q is drawn in %+v, want %+v, as the chart draws it", s.text, s.ink, want)
				}
			}
		})
	}
}

func TestTheGlyphLineDrawsABarsProjectionInShadeWhereTheLookCantFadeIt(t *testing.T) {
	for _, look := range []Look{NoColour(), Screen(builtin(t, theme.Terminal))} {
		f := frameOf(160, 40)
		f.Look = look
		if got := f.glyphLine(nil).plain(); !strings.Contains(got, "   ▒▒ heading   ") {
			t.Errorf("the glyph line reads %q, want the bars' heading in shade", got)
		}
	}
}

func TestTheGlyphLinesGlyphsLineUpWithTheLabelsUnderTheCards(t *testing.T) {
	coming := []strip{{label: "COMING UP"}, {label: recentLabel}}
	if got := frameOf(160, 40).glyphLine(coming).plain(); !strings.HasPrefix(got, "KEY         ▆▆ room left") {
		t.Errorf("under COMING UP, the glyph line reads %q, want its glyphs in COMING UP's column", got)
	}
}

// helped is a frame of three accounts at 160×34, drawn as text alone, with
