package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// graphsHelping is Graphs' help in the look given, open at width columns,
// tall enough to show it whole, listing keys.
func graphsHelping(width int, look Look, keys ...Key) Card {
	f := frameOf(width, 400)
	f.View, f.Look = Graphs, look
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

// graphsSaid is what Graphs' help says at 160 columns, its rows run
// together.
func graphsSaid() string {
	return strings.Join(plainLines(graphsHelping(160, Look{}).body), " ")
}

// graphsHelpGlyphs are the glyphs Graphs' help explains under the part
// headed heading, in the look given.
func graphsHelpGlyphs(look Look, heading string) []glyph {
	parts := Frame{View: Graphs, Look: look}.graphsHelp()
	if i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading }); i >= 0 {
		return parts[i].glyphs
	}
	return nil
}

// drawnGlyphs are the glyphs as they're drawn.
func drawnGlyphs(glyphs []glyph) []line {
	drawn := make([]line, len(glyphs))
	for i, g := range glyphs {
		drawn[i] = g.drawn
	}
	return drawn
}

func TestGraphsHelpListsItsKeysThenEachSectionInThePagesOrder(t *testing.T) {
	keys := []Key{
		tabViews, {Key: "w", Does: "week"}, {Key: "f", Does: "5-hour"}, {Key: "o", Does: "pool"}, {Key: "g", Does: "glance"},
		{Key: "h", Does: "when"}, {Key: "j k", Does: "scroll"}, {Key: "PgUp PgDn", Does: "page"}, pin,
		{Key: "t", Does: "themes"}, {Key: "R", Does: "refresh"}, helpQuit[0], helpQuit[1],
	}
	card := graphsHelping(160, Look{}, keys...)
	if card.title != "HELP · GRAPHS" {
		t.Errorf("help is titled %q, want HELP · GRAPHS", card.title)
	}
	opens := []string{
		"tab views   w week   f 5-hour   o pool   g glance   h when   j k scroll   PgUp PgDn page   p pin   t themes   R refresh",
		"? help   q quit",
		"",
	}
	if got := plainLines(card.body[:len(opens)]); !slices.Equal(got, opens) {
		t.Errorf("help opens\n%s\nwant its keys, then a blank row\n%s", strings.Join(got, "\n"), strings.Join(opens, "\n"))
	}
	if got, want := helpHeadings(card), []string{"WEEK AND 5-HOUR", "POOL", "GLANCE", "WHEN YOU WORK", "ROOM"}; !slices.Equal(got, want) {
		t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", got, want)
	}
}

// A card's marks are explained as v0.1's legend draws them, so help and the
// page's foot never draw one apart.
func TestGraphsHelpDrawsACardsMarksAsTheLegendDoes(t *testing.T) {
	glyphs := drawnGlyphs(graphsHelpGlyphs(Look{}, "WEEK AND 5-HOUR"))
	for _, item := range burnLegend(400, true) {
		for _, s := range item {
			if s.ink == mutedInk || strings.TrimSpace(s.text) == "" {
				continue
			}
			if !slices.ContainsFunc(glyphs, func(g line) bool { return slices.Equal(g, line{s}) }) {
				t.Errorf("the cards' help draws %v, want the legend's %v among them", glyphs, s)
			}
		}
	}
	if now := (line{{nowLine, faintInk}}); !slices.ContainsFunc(glyphs, func(g line) bool { return slices.Equal(g, now) }) {
		t.Errorf("the cards' help draws %v, want now's %v among them", glyphs, now)
	}
}

// In colour, the room left is keyed in each ink a window's state gives it,
// as a card fills it; without colour, which can't tell them apart, in the
// lasting window's alone.
func TestGraphsHelpKeysAWindowsInkOnlyInColour(t *testing.T) {
	rooms := func(look Look) []theme.Token {
		var tokens []theme.Token
		for _, g := range drawnGlyphs(graphsHelpGlyphs(look, "WEEK AND 5-HOUR")) {
			if g.plain() == "▆▆" {
				tokens = append(tokens, g[0].ink.token)
			}
		}
		return tokens
	}
	for _, tt := range []struct {
		name string
		look Look
		want []theme.Token
	}{
		{name: "a look that blends", look: Screen(builtin(t, "nord")), want: []theme.Token{theme.VizRamp1, theme.AccentAttention, theme.StateDestructive}},
		{name: "the terminal's colours", look: Screen(builtin(t, "terminal")), want: []theme.Token{theme.VizRamp1, theme.AccentAttention, theme.StateDestructive}},
		{name: "no colour", look: NoColour(), want: []theme.Token{theme.VizRamp1}},
	} {
		if got := rooms(tt.look); !slices.Equal(got, tt.want) {
			t.Errorf("in %s, help keys the room left in %v, want %v", tt.name, got, tt.want)
		}
	}
}

// WHEN YOU WORK's shades are drawn as its grid draws an hour at each level,
// in each look.
func TestGraphsHelpDrawsTheFiveShadesAsTheGridDrawsAnHour(t *testing.T) {
	for _, tc := range []struct {
		name string
		look Look
		tile func(level int) string
	}{
		{"a look that blends, a tile in each shade", Screen(builtin(t, "nord")), func(int) string { return hourTile }},
		{"the terminal's colours, each shade's density", Screen(builtin(t, "terminal")), shadeGlyph},
		{"no colour, each shade's density", NoColour(), shadeGlyph},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want []line
			for level := range shadeLevels {
				want = append(want, line{{tc.tile(level), dayInk(level)}})
			}
			glyphs := graphsHelpGlyphs(tc.look, "WHEN YOU WORK")
			if got := drawnGlyphs(glyphs); !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
				t.Errorf("WHEN YOU WORK's help draws its shades %v, want %v, level 0 to 4", got, want)
			}
			if len(glyphs) > 1 && (glyphs[0].means != "an hour with nothing on it" || glyphs[1].means != "the quietest quarter of the active hours") {
				t.Errorf("WHEN YOU WORK's help says its shades mean %q and %q, want them of hours", glyphs[0].means, glyphs[1].means)
			}
		})
	}
}

func TestGraphsHelpSaysWhatItsRulesAre(t *testing.T) {
	said := graphsSaid()
	for _, tc := range []struct{ what, want string }{
		{"a card's span", "its room over its own window, from the window's start to its reset"},
		{"each card's own clock", "each card keeps its own clock"},
		{"the room to now", "each column to the room its middle had"},
		{"a card's room", "the room it has left, as 66% left"},
		{"Runway's words", "then what Runway says under its lane in that window's view"},
		{"words giving way", "whole parts going from the end where they don't fit"},
		{"what changes the words", "a hold shorter than one of its columns can go unsaid; a wider terminal, or Runway's labels collapsed, gives finer columns"},
		{"why the line is straight", "straight, as it holds the rate the window's been used at lately to its reset"},
		{"where it runs out", "where the window runs out, its ✕ there"},
		{"the floor after", "then runs along the floor it meets, its cap or empty, to its reset"},
		{"✕ at a hold's start", "✕ marks the column the hold began, where that's told"},
		{"the reserve", "Where the account's reserve holds, so not while a pin spends it"},
		{"the window's ink", "The window's ink is its state's, as Runway's lanes colour one, each window by its own"},
		{"an account not read", "has Runway's words for it and its chart empty but for now's │"},
		{"an account the router lacks", "its label alone, numbered after the router's last"},
		{"faded and faint", "a stale one is faded, and one out of the pool faint"},
		{"the pool's line", "the pool's room in each window, the week's then the 5-hour's"},
		{"the pool's line with one account", "With one account there's no pool, and no line"},
		{"POOL's span", "The pool's room over Runway's week view, from a day before now for a week"},
		{"POOL's stack", "stacked from the foot, in the config's order, each in its own colour"},
		{"POOL's hues", "The first 4 accounts each have a colour of their own; a fifth on takes the colour four before it"},
		{"POOL's heading", "from now, faint, where it's heading at its recent rate"},
		{"POOL's top cell", "the top cell is the colour of the account with most of it"},
		{"POOL without colour", "Without colour the accounts can't be told apart, so POOL draws the pool's room alone"},
		{"GLANCE's sparkline", "A sparkline of 16 cells is the window's room from its start to its reset"},
		{"GLANCE's words", "then switchboard usage's words for the window"},
		{"GLANCE's pool", "from its accounts' earliest start to their latest reset"},
		{"GLANCE narrowing", "its words take switchboard usage's short forms, then its names narrow"},
		{"GLANCE beside", "It sits beside WHEN YOU WORK wherever both fit whole"},
		{"WHEN YOU WORK's rows", "a row a weekday, from the first day of the week, as week_starts begins weeks"},
		{"the weeks it counts", "summed over the four whole weeks before today and divided by the days of that weekday known"},
		{"fewer weeks", "or fewer where fewer are known; with none, nothing recorded yet"},
		{"the requests it counts", "those that spent quota and went upstream, each in the local hour it arrived in"},
		{"the clocks changing", "an hour the clocks skip has none that day, and one they repeat counts both"},
		{"its shades", "The shades are History's"},
		{"the toggles", "w, f, o, g and h show and hide WEEK, 5-HOUR, POOL, GLANCE and WHEN YOU WORK, at any size"},
		{"the toggles kept", "they're kept for next time"},
		{"rows to spare", "Rows to spare go first to each card's words, a row of their own, then to the charts"},
		{"the tallest chart", "up to 19 rows"},
		{"scrolling", "the page scrolls whole, a row at a time, with j and k"},
		{"the legend held", "the legend stays at the foot"},
		{"the layout held", "The layout changes only on a key, a resize or the config, never as the data does"},
		{"the clock", "The charts move on with the clock"},
	} {
		if !strings.Contains(said, tc.want) {
			t.Errorf("help reads\n%s\nwant it to explain %s, %q", said, tc.what, tc.want)
		}
	}
}

func TestGraphsHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, look := range []Look{Look{}, Screen(builtin(t, "nord"))} {
		for _, width := range []int{216, 160, 100, 52} {
			card := graphsHelping(width, look, tabViews, helpQuit[0], helpQuit[1])
			inside := card.width - 2*cardInset
			for i, row := range card.body {
				if row.width() > inside || strings.Contains(row.plain(), prose.Ellipsis) {
					t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
				}
			}
		}
	}
}
