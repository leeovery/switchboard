package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
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
	return strings.Join(plainLines(runwayHelping(width, look, RunwayShown{}).body), " ")
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
	keys := []Key{tabViews, {Key: "←→", Does: "week"}, {Key: "t", Does: "themes"}, {Key: "R", Does: "refresh"}, helpQuit[0], helpQuit[1]}
	card := runwayHelping(160, Look{}, RunwayShown{}, keys...)
	if card.title != "HELP · RUNWAY" {
		t.Errorf("help is titled %q, want HELP · RUNWAY", card.title)
	}
	if got := plainLines(card.body[:min(2, len(card.body))]); !slices.Equal(got, []string{"tab views   ←→ week   t themes   R refresh   ? help   q quit", ""}) {
		t.Errorf("help opens %q, want its keys, then a blank row", got)
	}
	if got, want := helpHeadings(card), []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP", "LATELY"}; !slices.Equal(got, want) {
		t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", got, want)
	}
}

// pinnedToOneUnread are Runway's lanes as views builds them over the
// auto world the frames were drawn of, the global pin naming side, nothing
// read of it yet: a pool's lane of one.
func pinnedToOneUnread() *views.Lanes {
	doc := framesDoc()
	doc.Accounts = slices.Clone(doc.Accounts)
	for i, a := range doc.Accounts {
		if a.ID == "side" {
			doc.Accounts[i] = status.Account{ID: a.ID, Label: a.Label, TokenSet: true}
		}
	}
	doc.Pin = status.Pin{Accounts: []string{"side"}, Since: routedAt.Add(-time.Hour)}
	m := doc.WorkedOut(claudeLike).At(routedAt, claudeLike)
	lanes := views.Lanes{}.Look(views.LanesFrom{Moment: m, Timeline: views.LiveTimeline(routedAt, 136)})
	return &lanes
}

// Where there's no pool's lane, as with one account, or two lanes no pool's
// lane is drawn under, help leaves the pool out, as the page does; lanes not
// built yet are taken as the pool's.
func TestRunwayHelpLeavesThePoolOutWithNoPoolsLane(t *testing.T) {
	tests := []struct {
		name  string
		lanes *views.Lanes
		want  []string
	}{
		{name: "one account", lanes: lanesOf(1), want: []string{"LANES", "STEPPING BACK", "COLUMNS", "COMING UP", "LATELY"}},
		{name: "two accounts", lanes: lanesOf(2), want: []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP", "LATELY"}},
		{name: "a pin of one not read yet, a pool's lane of one", lanes: pinnedToOneUnread(), want: []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP", "LATELY"}},
		{name: "two lanes, no pool's lane", lanes: &views.Lanes{Accounts: []views.Lane{{ID: "work"}, {ID: "side"}}}, want: []string{"LANES", "STEPPING BACK", "COLUMNS", "COMING UP", "LATELY"}},
		{name: "lanes not built yet", want: []string{"LANES", "POOL", "STEPPING BACK", "COLUMNS", "COMING UP", "LATELY"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := helpHeadings(runwayHelping(160, Look{}, RunwayShown{Lanes: tt.lanes}, tabViews)); !slices.Equal(got, tt.want) {
				t.Errorf("help explains %q, want %q", got, tt.want)
			}
		})
	}
}

// Each mark of a lane is explained once, drawn as a lane draws it in each
// look: in colour, room and a run-out alike █, by their colours, and a cap
// and a hold alike ━; without colour, a run-out ▒ and a hold bold, so each
// tells from the other by its glyph or its weight.
func TestRunwayHelpDrawsEachMarkAsALaneDrawsIt(t *testing.T) {
	room := line{{"█", ink{token: theme.VizRamp1}}}
	capped := line{{"━", ink{token: theme.AccentAttention}}}
	reset := line{{"│", ink{token: theme.TextSubtle}}}
	coloured := []line{room, {{"█", ink{token: theme.AccentAttention}}}, capped, {{"━", ink{token: theme.StateDestructive}}}, reset}
	tests := []struct {
		name string
		look Look
		want []line
	}{
		{name: "a look that blends", look: Screen(builtin(t, "nord")), want: coloured},
		{name: "the terminal's colours", look: Screen(builtin(t, "terminal")), want: coloured},
		{
			name: "no colour", look: NoColour(),
			want: []line{room, {{"▒", ink{token: theme.AccentAttention}}}, capped, {{"━", ink{token: theme.StateDestructive, bold: true}}}, reset},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []line
			for _, g := range runwayHelpGlyphs(tt.look, RunwayShown{}, "LANES") {
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

// │ is explained as the week's reset, as the legend keys it.
func TestRunwayHelpExplainsTheResetOfTheWeek(t *testing.T) {
	const want = "where its week resets"
	glyphs := runwayHelpGlyphs(Look{}, RunwayShown{}, "LANES")
	if !slices.ContainsFunc(glyphs, func(g glyph) bool { return g.drawn.plain() == "│" && g.means == want }) {
		t.Errorf("the lanes' help explains %v, want │ %q", glyphs, want)
	}
}

func TestRunwayHelpSaysWhatItsRulesAre(t *testing.T) {
	said := runwaySaid(160, Look{})
	for _, tc := range []struct{ what, want string }{
		{"a column by its middle", "drawn by how the account stood at the column's middle"},
		{"the windows that count", "Every window every model shares counts, a model's own window not"},
		{"the week from a day before now", "A lane an account, across the week from a day before now"},
		{"the use under a name", "its use of the week"},
		{"the labels collapsed", "a collapses the labels, to give the lanes more room"},
		{"a collapsed lane", "the lanes start at the left edge, with no names or marks"},
		{"a collapsed use", "under the start of each its week's use, as 66%"},
		{"a collapsed use out of the pool", "faint where the pool leaves its account out"},
		{"the words after a collapsed use", "The words under a lane start two cells after its use where they'd otherwise meet it"},
		{"the pool's room collapsed", "without Σ pool, under the lane's start, of how many of the pool's accounts have the week, as 1.2/3"},
		{"a stale account", "read 14:20 where its last read failed; and not read before its first"},
		{"a refused token's use faded", "and the use of one whose token is refused"},
		{"a run of stretches", "a stretch says lifts 19:00, when its own hold lifts, and only the last says back"},
		{"a time as the week resets", "Where room returns as the week resets, a cap's or a run-out's end says resets Sun 21:00 in place of back"},
		{"a week's reset", "Else resets Sun 21:00 where its week reaches no limit, or room all week"},
		{"the held words", "at its cap, at its limit, its token refused, as capped 14:31 and out 13:58"},
		{"a projection", "out ~16:05 where it runs out, or caps ~15:45 where it reaches its cap, ~ marking a time projected at its pace"},
		{"words shifted to fit", "shift left until they do, never over the words before, and drop parts from the end where even that can't fit them"},
		{"the past from the readings history", "from the readings history, each by the last reading of each window"},
		{"a column not read", "A column with no reading of its account by then is blank"},
		{"a reserve set now redraws past caps", "change a reserve, and the caps of days gone are drawn again by it"},
		{"the lanes scrolling", "the lanes scroll with j and k"},
		{"the pool's columns", "filled to how many of the pool's accounts have room then"},
		{"the pool's columns at none", "where none has room, ▂ along its foot, in a limit's red"},
		{"the pool's least fill", "▃ at the least where any has room"},
		{"the legend at the foot", "The legend sits at the foot, over the key line"},
		{"what's out of view", "what's out of view said beside the pool's room"},
		{"LATELY giving way first", "As rows run short, LATELY under COMING UP goes first; then an account takes two rows"},
		{"the legend and COMING UP giving way", "then the legend gives way, then COMING UP, then the blank row under the axis; only then do the lanes scroll"},
		{"COMING UP's room", "It sits under the pool's lane, or at the right beside LATELY, and takes the rows left"},
		{"COMING UP cut", "where its lines don't all fit, its last says how many more"},
		{"what counts without room", "An account at its cap or its limit counts without room"},
		{"the pool's accounts", "The pool is every account on auto, and the pin's while one holds"},
		{"the pool's room", "Under Σ pool, the pool's room in the week"},
		{"the pool pinned", "as pinned says after its room where it fits"},
		{"stepping back a week", "steps back, first to this week from its first day, as week_starts begins weeks, then a week at a time"},
		{"stepping forward", "from this week to the live view"},
		{"how far back", "reaches back as far as the day summaries, which are kept for good"},
		{"past the readings' keep", "draws its lanes from its summaries' limits and resets alone"},
		{"a week stepped back to", "no now, nothing dimmed, no COMING UP, no LATELY and no room line"},
		{"a step back never kept", "A step back is never kept, so Runway opens live"},
		{"COMING UP's lines", "an account back from its cap, its limit or its refused token, whichever lifts last"},
		{"COMING UP's resets", "each other reset of a window every model shares"},
		{"no prime", "No prime is listed"},
		{"LATELY's events", "a cap, a limit, a refused token, coming under pressure and room again, the sessions each moved said in its line"},
		{"no session alone", "No session's start or move shows alone"},
		{"how long ago", "Each line ends with how long ago it was, as 11m ago or 1d 4h ago, its words in full where they fit beside it"},
		{"LATELY in the rows left", "It takes only the rows the lanes and COMING UP leave"},
		{"LATELY beside COMING UP", "LATELY sits at the left and COMING UP at the right"},
		{"LATELY stacked", "else under COMING UP, where its heading and two lines fit: COMING UP in its two columns where they fit, LATELY across the frame under them, or else the two sharing one column, as wide as their lines need to read whole, from 72 to the frame's"},
		{"LATELY gone", "else it's gone, the first to give way as rows run short"},
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

// How long a column is follows the width, as the lanes keep the week's span:
// at 160 columns, the lanes' 148 after a label column of 12, ≈ 68 minutes;
// at 52, the lanes' 40 after the same column, 252 exactly, with no ≈.
func TestRunwayHelpSaysHowLongAColumnIsAtTheWidth(t *testing.T) {
	tests := []struct {
		width int
		week  string
	}{
		{width: 160, week: "≈ 68 minutes"},
		{width: 216, week: "≈ 49 minutes"},
		{width: 52, week: "252 minutes"},
	}
	for _, tt := range tests {
		want := "at this width, a column is " + tt.week + ", as the legend says"
		if said := runwaySaid(tt.width, Look{}); !strings.Contains(said, want) {
			t.Errorf("at %d columns, help reads\n%s\nwant it to say %q", tt.width, said, want)
		}
	}
}

func TestRunwayHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, width := range []int{216, 160, 100, 52} {
		card := runwayHelping(width, Look{}, RunwayShown{}, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(row.plain(), prose.Ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}
