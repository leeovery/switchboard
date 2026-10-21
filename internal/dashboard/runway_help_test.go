package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayHelping is Runway's help over the frame given, its view shown as r,
// open at width columns, tall enough to show it whole, listing keys.
func runwayHelping(width int, look Look, r RunwayShown, keys ...Key) Card {
	f := frameOf(width, 400)
	f.View, f.Look, f.Runway = Runway, look, r
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

// runwaySaid is what Runway's help says at width columns in the look
// given, its rows run together.
func runwaySaid(width int, look Look) string {
	return strings.Join(plainLines(runwayHelping(width, look, RunwayShown{Window: views.RunwayFiveHour}).body), " ")
}

// runwayHelpGlyphs are the glyphs Runway's help explains under the part
// headed heading, in the look given, its view shown as r.
func runwayHelpGlyphs(look Look, r RunwayShown, heading string) []glyph {
	parts := Frame{View: Runway, Look: look, Runway: r}.runwayHelp()
	if i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading }); i >= 0 {
		return parts[i].glyphs
	}
	return nil
}

func TestRunwayHelpListsItsKeysThenEachPartInTheDesignsOrder(t *testing.T) {
	keys := []Key{tabViews, {Key: "< >", Does: "window"}, {Key: "←→", Does: "day"}, {Key: "t", Does: "themes"}, {Key: "R", Does: "refresh"}, helpQuit[0], helpQuit[1]}
	card := runwayHelping(160, Look{}, RunwayShown{Window: views.RunwayFiveHour}, keys...)
	if card.title != "HELP · RUNWAY" {
		t.Errorf("help is titled %q, want HELP · RUNWAY", card.title)
	}
	if got := plainLines(card.body[:min(2, len(card.body))]); !slices.Equal(got, []string{"tab views   < > window   ←→ day   t themes   R refresh   ? help   q quit", ""}) {
		t.Errorf("help opens %q, want its keys, then a blank row", got)
	}
	if got, want := helpHeadings(card), []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP"}; !slices.Equal(got, want) {
		t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", got, want)
	}
}

// One account has no pool's lane, so help leaves the pool out, as the page
// does; lanes not built yet are taken as the pool's.
func TestRunwayHelpLeavesThePoolOutWithOneAccount(t *testing.T) {
	tests := []struct {
		name  string
		lanes *views.Lanes
		want  []string
	}{
		{name: "one account", lanes: &views.Lanes{Accounts: []views.Lane{{ID: "work"}}}, want: []string{"LANES", "STEPPING BACK", "COLUMNS", "COMING UP"}},
		{name: "two accounts", lanes: &views.Lanes{Accounts: []views.Lane{{ID: "work"}, {ID: "personal"}}}, want: []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP"}},
		{name: "lanes not built yet", want: []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := helpHeadings(runwayHelping(160, Look{}, RunwayShown{Window: views.RunwayFiveHour, Lanes: tt.lanes}, tabViews)); !slices.Equal(got, tt.want) {
				t.Errorf("help explains %q, want %q", got, tt.want)
			}
		})
	}
}

// Each mark of a lane is explained once, drawn as a lane draws it in each
// look: in colour, room and a run-out alike █, by their colours, and a cap
// and a hold alike ─; without colour, a run-out ▒ and a hold bold, so each
// tells from the other by its glyph or its weight.
func TestRunwayHelpDrawsEachMarkAsALaneDrawsIt(t *testing.T) {
	room := line{{"█", ink{token: theme.VizRamp1}}}
	capped := line{{"─", ink{token: theme.AccentAttention}}}
	reset := line{{"│", ink{token: theme.TextSubtle}}}
	coloured := []line{room, {{"█", ink{token: theme.AccentAttention}}}, capped, {{"─", ink{token: theme.StateDestructive}}}, reset}
	tests := []struct {
		name string
		look Look
		want []line
	}{
		{name: "a look that blends", look: Screen(builtin(t, "nord")), want: coloured},
		{name: "the terminal's colours", look: Screen(builtin(t, "terminal")), want: coloured},
		{
			name: "no colour", look: NoColour(),
			want: []line{room, {{"▒", ink{token: theme.AccentAttention}}}, capped, {{"─", ink{token: theme.StateDestructive, bold: true}}}, reset},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []line
			for _, g := range runwayHelpGlyphs(tt.look, RunwayShown{Window: views.RunwayFiveHour}, "LANES") {
				got = append(got, g.drawn)
			}
			if !slices.EqualFunc(got, tt.want, func(a, b line) bool { return slices.Equal(a, b) }) {
				t.Errorf("the lanes' help draws %v, want %v", got, tt.want)
			}
		})
	}
}

// A refusal and no usable token hold an account back from every request, as
// a limit does, and a lane draws them alike; nothing known is blank.
func TestALaneDrawsEveryHoldFromEveryRequestAlike(t *testing.T) {
	for _, look := range []Look{Screen(builtin(t, "nord")), NoColour()} {
		limited := look.laneMark(views.CellLimited)
		for _, c := range []views.Cell{views.CellRefused, views.CellTokenless} {
			if got := look.laneMark(c); got != limited {
				t.Errorf("a lane draws cell %d as %v, want %v, as a limit", c, got, limited)
			}
		}
		if got := look.laneMark(views.CellUnknown); got != spaces(1) {
			t.Errorf("a lane draws a column nothing is known of as %v, want it blank", got)
		}
	}
}

// │ is explained as the view shown's window's reset, as the legend keys it.
func TestRunwayHelpExplainsTheResetOfTheViewsWindow(t *testing.T) {
	for _, tt := range []struct {
		window views.RunwayWindow
		want   string
	}{
		{window: views.RunwayFiveHour, want: "where its 5-hour resets"},
		{window: views.RunwayWeek, want: "where its week resets"},
	} {
		glyphs := runwayHelpGlyphs(Look{}, RunwayShown{Window: tt.window}, "LANES")
		if !slices.ContainsFunc(glyphs, func(g glyph) bool { return g.drawn.plain() == "│" && g.means == tt.want }) {
			t.Errorf("in the %q view, the lanes' help explains %v, want │ %q", tt.window, glyphs, tt.want)
		}
	}
}

func TestRunwayHelpSaysWhatItsRulesAre(t *testing.T) {
	said := runwaySaid(160, Look{})
	for _, tc := range []struct{ what, want string }{
		{"a column by its middle", "drawn by how the account stood at the column's middle"},
		{"the windows that count", "Every window every model shares counts, a model's own window not"},
		{"the 5-hour leaving out a week running out", "the 5-hour view leaves out a week running out"},
		{"the use under a name", "its use of the view's window"},
		{"the labels collapsed", "a collapses the labels, to give the lanes more room"},
		{"a collapsed label", "its number and its marks alone, as 2 ◆, and under them its use, as 66%"},
		{"the pool's room collapsed", "under Σ, of how many accounts the pool is, as 1.2/3"},
		{"a stale account", "read 14:20 where its last read failed; and not read before its first"},
		{"a refused token's use faded", "and the use of one whose token is refused"},
		{"a run of stretches", "a stretch says lifts 19:00, when its own hold lifts, and only the last says back"},
		{"a time as the view's window resets", "A cap's or a run-out's time adds as its 5-hour resets"},
		{"a projection", "~ marking a time projected at its pace"},
		{"the past from the readings history", "from the readings history, each by the last reading of each window"},
		{"a column not read", "A column with no reading of its account by then is blank"},
		{"a reserve set now redraws past caps", "change a reserve, and the caps of days gone are drawn again by it"},
		{"the lanes scrolling", "the lanes scroll with j and k"},
		{"the pool's columns", "filled to how many of the pool's accounts have room then"},
		{"the pool's columns at none", "where none has room, ─ along its foot, as a lane at its limit draws it"},
		{"what counts without room", "An account at its cap or its limit counts without room"},
		{"the pool's accounts", "The pool is every account on auto, and the pin's while one holds"},
		{"the pool's room", "Under Σ pool, the pool's room in the view's window"},
		{"the pool pinned", "as pinned says after its room where it fits"},
		{"stepping back a day", "first to today from its midnight, then a day at a time"},
		{"stepping back a week", "first to this week from its first day, as week_starts begins weeks"},
		{"stepping forward", "from today, or this week, to the live view"},
		{"the other window", "< and > show the other window's live view"},
		{"how far back", "reaches back as far as the day summaries, which are kept for good"},
		{"past the readings' keep", "draws its lanes from its summary's limits and resets alone"},
		{"a day stepped back to", "no now, nothing dimmed, no COMING UP and no room line"},
		{"a step back never kept", "a step back never is, so Runway opens live"},
		{"COMING UP's lines", "an account back from its cap, its limit or its refused token, whichever lifts last"},
		{"COMING UP in the week", "the week view adds each week's run-out and reset"},
		{"no prime", "No prime is listed"},
	} {
		if !strings.Contains(said, tc.want) {
			t.Errorf("help reads\n%s\nwant it to explain %s, %q", said, tc.what, tc.want)
		}
	}
}

// Help says the past is dimmed only where the look dims it, as the legend
// does.
func TestRunwayHelpSaysThePastIsDimmedWhereTheLookBlends(t *testing.T) {
	const dimmed = "and everything before it dimmed"
	for _, tt := range []struct {
		name string
		look Look
		want bool
	}{
		{name: "a look that blends", look: Screen(builtin(t, "nord")), want: true},
		{name: "the terminal's colours", look: Screen(builtin(t, "terminal"))},
		{name: "no colour", look: NoColour()},
	} {
		if got := strings.Contains(runwaySaid(160, tt.look), dimmed); got != tt.want {
			t.Errorf("in %s, help says the past is dimmed: %t, want %t", tt.name, got, tt.want)
		}
	}
}

// How long a column is follows the width, as the lanes keep their span: at
// 160 columns, the lanes' 146 after a label column of 14, ≈ 9.3 minutes,
// ≈ 69 and ≈ 9.9; at 52, the lanes' 38 after the same column, about 36
// minutes.
func TestRunwayHelpSaysHowLongAColumnIsAtTheWidth(t *testing.T) {
	tests := []struct {
		width               int
		fiveHour, week, day string
	}{
		{width: 160, fiveHour: "≈ 9.3 minutes", week: "≈ 69 minutes", day: "≈ 9.9 minutes"},
		{width: 216, fiveHour: "≈ 6.7 minutes", week: "≈ 50 minutes", day: "≈ 7.1 minutes"},
		{width: 52, fiveHour: "≈ 36 minutes", week: "≈ 265 minutes", day: "≈ 38 minutes"},
	}
	for _, tt := range tests {
		want := "a column is " + tt.fiveHour + " in the 5-hour view, " + tt.week + " in the week's and " + tt.day + " on a day stepped back to"
		if said := runwaySaid(tt.width, Look{}); !strings.Contains(said, want) {
			t.Errorf("at %d columns, help reads\n%s\nwant it to say %q", tt.width, said, want)
		}
	}
}

func TestRunwayHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, width := range []int{216, 160, 100, 52} {
		card := runwayHelping(width, Look{}, RunwayShown{Window: views.RunwayFiveHour}, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(row.plain(), prose.Ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}
