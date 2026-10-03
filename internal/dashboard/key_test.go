package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheKeyLineExplainsTheGlyphs(t *testing.T) {
	f := frameOf(160, 40)
	f.Look = Screen(builtin(t, "nord"))
	got := f.keyLine(nil)
	if want := "KEY      ▆▆ room left   ⠂⠄⡀ heading   ✕ runs out   ⠠ ⠠ reserve   │   ████ used   ██ heading   ┃ even pace   ╎ reserve   ● session, lit while busy"; got.plain() != want {
		t.Errorf("the key line reads\n%q\nwant\n%q", got.plain(), want)
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

func TestTheKeyLineExplainsTheGlyphsOfTheChartsStyle(t *testing.T) {
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
			got := f.keyLine(nil)
			if !strings.HasPrefix(got.plain(), tt.want) || !strings.HasSuffix(got.plain(), "● session, lit while busy") {
				t.Errorf("the key line reads\n%q\nwant it to start\n%q\nand go on to the bars' glyphs", got.plain(), tt.want)
			}
			if !f.keyFits(nil) {
				t.Errorf("the key line is %d cells wide, want it to fit 160 columns", got.width())
			}
			for _, s := range got {
				if want, ok := tt.glyphs[s.text]; ok && s.ink != want {
					t.Errorf("%q is drawn in %+v, want %+v, as the chart draws it", s.text, s.ink, want)
				}
			}
		})
	}
}

func TestTheKeyLineDrawsABarsProjectionInShadeWhereTheLookCantFadeIt(t *testing.T) {
	for _, look := range []Look{NoColour(), Screen(builtin(t, theme.Terminal))} {
		f := frameOf(160, 40)
		f.Look = look
		if got := f.keyLine(nil).plain(); !strings.Contains(got, "   ▒▒ heading   ") {
			t.Errorf("the key line reads %q, want the bars' heading in shade", got)
		}
	}
}

func TestTheKeyLinesGlyphsLineUpWithTheLabelsUnderTheCards(t *testing.T) {
	coming := []strip{{label: "COMING UP"}, {label: recentLabel}}
	if got := frameOf(160, 40).keyLine(coming).plain(); !strings.HasPrefix(got, "KEY         ▆▆ room left") {
		t.Errorf("under COMING UP, the key line reads %q, want its glyphs in COMING UP's column", got)
	}
}

// helped is a frame of three accounts at 160×34, drawn as text alone, with
// the help open, listing w and q.
func helped(t *testing.T) Frame {
	t.Helper()
	f := frameOf(160, 34)
	f.Help = []Key{{Key: "w", Does: "cycle the window every card features, now auto"}, {Key: "q", Does: "quit"}}
	return f
}

func TestTheHelpIsDrawnOverTheMiddleOfTheView(t *testing.T) {
	rows := helped(t).Draw(threeRouted(), now)
	want := []string{
		"╭─────────────────────────────────────────────────────────╮",
		"│  ? Keys                                      esc close  │",
		"├─────────────────────────────────────────────────────────┤",
		"│  w      cycle the window every card features, now auto  │",
		"│  q      quit                                            │",
		"├─────────────────────────────────────────────────────────┤",
		"│  ▆▆     room left                                       │",
		"│  ⠂⠄⡀    heading                                         │",
		"│  ✕      runs out                                        │",
		"│  ⠠ ⠠    reserve                                         │",
		"├─────────────────────────────────────────────────────────┤",
		"│  ████   used                                            │",
		"│  ▒▒     heading                                         │",
		"│  ┃      even pace                                       │",
		"│  ╎      reserve                                         │",
		"│  ●      session, lit while busy                         │",
		"╰─────────────────────────────────────────────────────────╯",
	}
	// The panel is 59 cells wide and 17 rows tall, in the middle of 160 by
	// 34: from column 50, row 9.
	for i, w := range want {
		if got := string([]rune(rows[8+i])[50:109]); got != w {
			t.Errorf("row %d of the help reads\n%q\nwant\n%q", i+1, got, w)
		}
	}
	if !strings.HasPrefix(rows[0], "  SWITCHBOARD ") || !strings.HasPrefix(rows[9], " │  ● under pressure ") || !strings.HasSuffix(rows[9], "│  ● open · new sessions come here               │") {
		t.Errorf("rows are\n%s\nwant the view drawn around the help", strings.Join(rows, "\n"))
	}
}

func TestTheHelpIsCutToFitASmallFrame(t *testing.T) {
	for _, size := range [][2]int{{60, 12}, {30, 8}, {4, 3}} {
		f := helped(t)
		f.Width, f.Height = size[0], size[1]
		rows := f.Draw(threeRouted(), now)
		if len(rows) != size[1] {
			t.Errorf("at %d×%d, drew %d rows", size[0], size[1], len(rows))
		}
		for i, row := range rows {
			if got := ansi.StringWidth(row); got > size[0] {
				t.Errorf("at %d×%d, row %d is %d cells wide", size[0], size[1], i+1, got)
			}
		}
	}
	f := helped(t)
	f.Width, f.Height = 60, 12
	if rows := f.Draw(threeRouted(), now); !strings.Contains(rows[0], "╭───") || !strings.Contains(rows[1], "? Keys") || !strings.Contains(rows[1], "esc close") {
		t.Errorf("on a frame too short for it, the help's first rows read\n%s\n%s\nwant its top edge and its title at the top", rows[0], rows[1])
	}
}
