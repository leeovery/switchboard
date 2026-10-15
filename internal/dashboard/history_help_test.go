package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// historyHelping is History's help in the look given, open at width
// columns, tall enough to show it whole, listing keys.
func historyHelping(width int, look Look, keys ...Key) Card {
	f := frameOf(width, 400)
	f.View, f.Look = History, look
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

// historySaid is what History's help says, its rows run together.
func historySaid() string {
	return strings.Join(plainLines(historyHelping(160, Look{}).body), " ")
}

// historyHelpGlyphs are the glyphs History's help explains under the part
// headed heading, in the look given.
func historyHelpGlyphs(look Look, heading string) []glyph {
	parts := Frame{View: History, Look: look}.historyHelp()
	if i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading }); i >= 0 {
		return parts[i].glyphs
	}
	return nil
}

func TestHistoryHelpListsItsKeysThenEachPartInTheDesignsOrder(t *testing.T) {
	card := historyHelping(160, Look{}, tabViews, helpQuit[0], helpQuit[1])
	if card.title != "HELP · HISTORY" {
		t.Errorf("help is titled %q, want HELP · HISTORY", card.title)
	}
	if got := plainLines(card.body[:2]); !slices.Equal(got, []string{"tab views   ? help   q quit", ""}) {
		t.Errorf("help opens %q, want its keys, then a blank row", got)
	}
	var headings []string
	for i, row := range card.body[1:] {
		if len(card.body[i]) == 0 && len(row) == 1 && row[0].ink == dimInk {
			headings = append(headings, row[0].text)
		}
	}
	if want := []string{"YEAR", "BY MONTH", "WEEKS", "DAYS", "TOKENS"}; !slices.Equal(headings, want) {
		t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", headings, want)
	}
}

func TestHistoryHelpDrawsTheFiveShadesAsTheYearDrawsADay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		look   Look
		square func(level int) string
	}{
		{"a look that blends, a square in each shade", Screen(builtin(t, "nord")), func(int) string { return "■" }},
		{"the terminal's colours, each shade's density", Screen(builtin(t, "terminal")), shadeGlyph},
		{"no colour, each shade's density", NoColour(), shadeGlyph},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want []line
			for level := range shadeLevels {
				want = append(want, line{{tc.square(level), dayInk(level)}})
			}
			var got []line
			for _, g := range historyHelpGlyphs(tc.look, "YEAR") {
				got = append(got, g.drawn)
			}
			if !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
				t.Errorf("the Year's help draws its shades %v, want %v, level 0 to 4", got, want)
			}
		})
	}
}

func TestHistoryHelpKeysEachMarkAsThePageDrawsIt(t *testing.T) {
	bar := line{
		{"▅", ink{token: theme.VizFamily1}}, {"▅", ink{token: theme.VizFamily2}},
		{"▅", ink{token: theme.VizFamily3}}, {"▅", ink{token: theme.VizFamily4}},
	}
	for _, tc := range []struct {
		heading string
		drawn   line
	}{
		{"BY MONTH", bar},
		{"BY MONTH", line{{"—", faintInk}}},
		{"WEEKS", line{{"100%", ink{token: theme.StateDestructive, bold: true}}}},
		{"WEEKS", line{{"34%…", ink{token: theme.TextSecondary, bold: true}}}},
		{"WEEKS", line{{"▁▁▁▁▁", faintInk}}},
		{"WEEKS", line{{"—", faintInk}}},
		{"DAYS", line{{"··", faintInk}}},
	} {
		t.Run(tc.heading+" "+tc.drawn.plain(), func(t *testing.T) {
			glyphs := historyHelpGlyphs(Look{}, tc.heading)
			if !slices.ContainsFunc(glyphs, func(g glyph) bool { return slices.Equal(g.drawn, tc.drawn) }) {
				t.Errorf("%s's help explains %v, want %v among them", tc.heading, glyphs, tc.drawn)
			}
		})
	}
}

// A day it hit a 5-hour limit is drawn as Days' key draws it, in each look:
// a red ▪ on a viz.day.2 swatch, or, where the look blends nothing, before
// that shade's density.
func TestHistoryHelpDrawsADayAtALimitAsDaysDoes(t *testing.T) {
	day2 := dayInk(2)
	glyphed := line{{"▪", ink{token: theme.StateDestructive}}, {shadeGlyph(2), day2}}
	for _, tc := range []struct {
		name string
		look Look
		want line
	}{
		{"a look that blends", Screen(builtin(t, "nord")), line{{"▪", ink{token: theme.StateDestructive, on: day2.hue()}}, {" ", ink{on: day2.hue()}}}},
		{"the terminal's colours", Screen(builtin(t, "terminal")), glyphed},
		{"no colour", NoColour(), glyphed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			glyphs := historyHelpGlyphs(tc.look, "DAYS")
			if !slices.ContainsFunc(glyphs, func(g glyph) bool { return slices.Equal(g.drawn, tc.want) }) {
				t.Errorf("Days' help explains %v, want %v among them", glyphs, tc.want)
			}
		})
	}
}

func TestHistoryHelpSaysWhatEachVerdictMeans(t *testing.T) {
	said := historySaid()
	for _, verdict := range []string{
		views.OneMoreThanYouNeed + ", where",
		views.OneTooFew + ", where",
		"; too few, where",
		"about right.",
		"hits its limit most weeks, in",
		"plenty to spare most weeks, 40%",
		"ends most weeks with room.",
		"Could go, where",
		"barely used, 75%",
	} {
		if !strings.Contains(said, verdict) {
			t.Errorf("help reads\n%s\nwant it to say %q", said, verdict)
		}
	}
}

func TestHistoryHelpSaysWhatItsFiguresAre(t *testing.T) {
	said := historySaid()
	for _, tc := range []struct{ what, want string }{
		{"the Year shades requests", "shaded by how many requests it made, never by its worth"},
		{"each shade a quarter of the active days", "the active days are split into quarters"},
		{"BY MONTH's bars", "a month's worth against the largest month's"},
		{"against what the plans cost", "what the plans cost that month and its worth against it"},
		{"a week's peak", "the use read just before the reset that ended it"},
		{"short", "short where some account hit its limit"},
		{"the replay", "with one more, of the plan most of the accounts have"},
		{"Days' two scales", "two scales of their own"},
		{"Tokens' columns", "Its columns are ccusage's"},
		{"the price table's date", "as of the date the footnote gives"},
	} {
		if !strings.Contains(said, tc.want) {
			t.Errorf("help reads\n%s\nwant it to explain %s, %q", said, tc.what, tc.want)
		}
	}
}

// Help says how each view whose scroll can count rows or things scrolls,
// and that it goes back to its top as its scroll turns from one to the
// other: the Overview's USAGE, History's Weeks, Days and Tokens; and that
// the Year, a row at a time, says what's out of view.
func TestHelpSaysAViewGoesBackToItsTopAsItsScrollTurns(t *testing.T) {
	saying := func(parts []helpPart, heading string) string {
		i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading })
		if i < 0 {
			t.Fatalf("help explains no %s", heading)
		}
		var said []string
		for _, l := range parts[i].says {
			said = append(said, l.plain())
		}
		return strings.Join(said, " ")
	}
	history := Frame{View: History}.historyHelp()
	tests := []struct {
		heading, said string
		want          []string
	}{
		{heading: "USAGE", said: saying(overviewHelp(), SectionUsage.heading()), want: []string{"an account at a time, or a row at a time", "it goes back to its top"}},
		{heading: "YEAR", said: saying(history, "YEAR"), want: []string{"scrolls a row at a time", "how many rows are out of view above and below"}},
		{heading: "WEEKS", said: saying(history, "WEEKS"), want: []string{"an account at a time", "shorter than 37 rows", "it goes back to its top"}},
		{heading: "DAYS", said: saying(history, "DAYS"), want: []string{"BY ACCOUNT scrolls an account at a time", "shorter than 37 rows", "it goes back to its top"}},
		{heading: "TOKENS", said: saying(history, "TOKENS"), want: []string{"a period at a time, or a row at a time", "they go back to their top"}},
	}
	for _, tt := range tests {
		for _, want := range tt.want {
			if !strings.Contains(tt.said, want) {
				t.Errorf("help on %s reads %q, want %q", tt.heading, tt.said, want)
			}
		}
	}
}

func TestHistoryHelpFitsTheCardAtEveryWidth(t *testing.T) {
	const thisWeek = "34%…"
	for _, width := range []int{216, 160, 100, 52} {
		card := historyHelping(width, Look{}, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(strings.ReplaceAll(row.plain(), thisWeek, ""), prose.Ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}
