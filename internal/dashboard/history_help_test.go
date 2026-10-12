package dashboard

import (
	"slices"
	"strings"
	"testing"

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
		{"DAYS", line{{"▪ ", ink{token: theme.StateDestructive, on: hue{token: theme.VizDay2}}}}},
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

func TestHistoryHelpFitsTheCardAtEveryWidth(t *testing.T) {
	const thisWeek = "34%…"
	for _, width := range []int{216, 160, 100, 52} {
		card := historyHelping(width, Look{}, tabViews, helpQuit[0], helpQuit[1])
		inside := card.width - 2*cardInset
		for i, row := range card.body {
			if row.width() > inside || strings.Contains(strings.ReplaceAll(row.plain(), thisWeek, ""), ellipsis) {
				t.Errorf("at %d columns, help's row %d is %q, %d cells; want it whole within %d", width, i+1, row.plain(), row.width(), inside)
			}
		}
	}
}
