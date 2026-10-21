package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// threeShown and oneShown are Accounts of three accounts, and of one.
var (
	threeShown = AccountsShown{Accounts: config.Accounts{{ID: "work"}, {ID: "personal"}, {ID: "side"}}}
	oneShown   = AccountsShown{Accounts: config.Accounts{{ID: "work"}}}
)

// accountsHelping is Accounts' help as shown, in the look given, open at
// width columns, tall enough to show it whole, listing keys.
func accountsHelping(width int, shown AccountsShown, look Look, keys ...Key) Card {
	f := frameOf(width, 400)
	f.View, f.Accounts, f.Look = Accounts, shown, look
	f.Card = Help{Keys: keys}
	return f.Card.card(f)
}

// accountsHelpSaid is what Accounts' help as shown says, its rows run
// together.
func accountsHelpSaid(shown AccountsShown) string {
	return strings.Join(plainLines(accountsHelping(160, shown, Look{}).body), " ")
}

// accountsHelpGlyphs are the glyphs Accounts' help explains under the part
// headed heading, in the look given.
func accountsHelpGlyphs(look Look, heading string) []line {
	parts := Frame{View: Accounts, Accounts: threeShown, Look: look}.accountsHelp()
	var drawn []line
	if i := slices.IndexFunc(parts, func(p helpPart) bool { return p.heading == heading }); i >= 0 {
		for _, g := range parts[i].glyphs {
			drawn = append(drawn, g.drawn)
		}
	}
	return drawn
}

// helpHeadings are the parts' headings a help card's body explains, each in
// text.subtle after a blank row.
func helpHeadings(card Card) []string {
	var headings []string
	for i, row := range card.body[1:] {
		if len(card.body[i]) == 0 && len(row) == 1 && row[0].ink == dimInk {
			headings = append(headings, row[0].text)
		}
	}
	return headings
}

func TestAccountsHelpListsItsKeysThenEachPartInTheDesignsOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shown AccountsShown
		want  []string
	}{
		{"three accounts", threeShown, []string{"COMPARE", "VALUE", "ALL ACCOUNTS", "DETAIL"}},
		{"one account, Compare and all accounts gone from the page", oneShown, []string{"VALUE", "DETAIL"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := accountsHelping(160, tc.shown, Look{}, tabViews, helpQuit[0], helpQuit[1])
			if card.title != "HELP · ACCOUNTS" {
				t.Errorf("help is titled %q, want HELP · ACCOUNTS", card.title)
			}
			if got := plainLines(card.body[:2]); !slices.Equal(got, []string{"tab views   ? help   q quit", ""}) {
				t.Errorf("help opens %q, want its keys, then a blank row", got)
			}
			if got := helpHeadings(card); !slices.Equal(got, tc.want) {
				t.Errorf("help explains %q, want %q, each headed in text.subtle after a blank row", got, tc.want)
			}
		})
	}
}

// Each state an account's head says in a word is explained as Compare says
// it, its glyph and word in its state's colour, open's glyph alone in
// state.positive, in the order they're counted in.
func TestAccountsHelpKeysEachStateAsCompareSaysIt(t *testing.T) {
	attention, destructive := ink{token: theme.AccentAttention}, ink{token: theme.StateDestructive}
	want := []line{
		{{"●", ink{token: theme.StatePositive}}, {" open", ink{token: theme.TextSecondary}}},
		{{"● pressure", attention}},
		{{"● at cap", attention}},
		{{"■ limit", destructive}},
		{{"■ refused", destructive}},
		{{"○ not read", ink{token: theme.TextSubtle}}},
	}
	if got := accountsHelpGlyphs(Look{}, "COMPARE"); !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
		t.Errorf("Compare's help explains the states %v, want %v", got, want)
	}
}

// What each state means is said as status says it.
func TestAccountsHelpSaysWhatEachStateMeansAsStatusDoes(t *testing.T) {
	var got, want []string
	for _, g := range statesHelp() {
		got = append(got, g.means)
	}
	for s := status.StateOpen; s <= status.StateNotRead; s++ {
		want = append(want, s.Means())
	}
	if !slices.Equal(got, want) {
		t.Errorf("help says the states mean %q, want %q", got, want)
	}
}

// Worth part unpriced, against its price either side of 1×, and nothing
// recorded are drawn as the page draws them.
func TestAccountsHelpKeysValuesMarksAsThePageDrawsThem(t *testing.T) {
	want := []line{
		{{"$184", ink{token: theme.TextSecondary}}, {"+", ink{token: theme.TextFaint}}},
		{{"4.0×", ink{token: theme.StatePositive, bold: true}}},
		{{"0.6×", ink{token: theme.AccentAttention, bold: true}}},
		{{"—", ink{token: theme.TextFaint}}},
	}
	if got := accountsHelpGlyphs(Look{}, "VALUE"); !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
		t.Errorf("VALUE's help explains %v, want %v", got, want)
	}
}

// BY DAY's five shades, a day at a 5-hour limit and the days to come are
// drawn as Detail draws a day, two cells, in each look; and a week at its
// limit as WEEKS draws its peak.
func TestAccountsHelpDrawsBYDAYAndWEEKSAsDetailDoes(t *testing.T) {
	day2 := dayInk(2)
	for _, tc := range []struct {
		name    string
		look    Look
		day     func(level int) line
		limited line
	}{
		{
			"a look that blends, each day spaces on its shade", Screen(builtin(t, "nord")),
			func(level int) line { return line{{"  ", ink{on: dayInk(level).hue()}}} },
			line{{"▪", ink{token: theme.StateDestructive, on: day2.hue()}}, {" ", ink{on: day2.hue()}}},
		},
		{
			"no colour, each day its shade's density", NoColour(),
			func(level int) line { return line{{strings.Repeat(shadeGlyph(level), 2), dayInk(level)}} },
			line{{"▪", ink{token: theme.StateDestructive}}, {shadeGlyph(2), day2}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want []line
			for level := range shadeLevels {
				want = append(want, tc.day(level))
			}
			want = append(want, tc.limited, line{{"··", ink{token: theme.TextFaint}}}, line{{"100", ink{token: theme.StateDestructive, bold: true}}})
			if got := accountsHelpGlyphs(tc.look, "DETAIL"); !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
				t.Errorf("Detail's help explains\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// BY DAY's key draws a day as many cells wide as Detail draws its days in
// the form it's built in, the limit's ▪ in its middle cell, the first of
// two, and a day to come a · a cell: two with the list, three with one
// account at 160, whose days grow into the width, two at 120, where they
// don't yet, and two where one account's Detail is stacked, as on a phone.
func TestAccountsHelpDrawsADayAsWideAsDetailDrawsItsDays(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	tests := []struct {
		name  string
		shown AccountsShown
		width int
		cells int
	}{
		{name: "three accounts, the list beside the panel", shown: threeShown, width: 160, cells: 2},
		{name: "one account, wide", shown: oneShown, width: 160, cells: 3},
		{name: "one account, its days not yet grown", shown: oneShown, width: 120, cells: 2},
		{name: "one account on a phone", shown: oneShown, width: 52, cells: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var want []line
			for level := range shadeLevels {
				want = append(want, line{{strings.Repeat(" ", tt.cells), ink{on: dayInk(level).hue()}}})
			}
			on := ink{on: dayInk(2).hue()}
			limited := line{{"▪", ink{token: theme.StateDestructive, on: on.on}}, {" ", on}}
			if tt.cells == 3 {
				limited = slices.Concat(line{{" ", on}}, limited)
			}
			want = append(want, limited, line{{strings.Repeat("·", tt.cells), faintInk}})
			doc := usageWorld(extraOff)
			parts := Frame{Width: tt.width, Height: 37, View: Accounts, Accounts: tt.shown, Look: look, Policy: claudeLike}.Built(doc, routedAt).accountsHelp()
			glyphs := parts[len(parts)-1].glyphs
			var got []line
			for _, g := range glyphs[:len(glyphs)-1] {
				got = append(got, g.drawn)
			}
			if !slices.EqualFunc(got, want, func(a, b line) bool { return slices.Equal(a, b) }) {
				t.Errorf("Detail's help draws its days\n%v\nwant\n%v", got, want)
			}
		})
	}
}

// Worth is said to be priced as of the price table's date once the page is
// read, and before, at the price table's prices alone.
func TestAccountsHelpGivesThePriceTablesDate(t *testing.T) {
	read := threeShown
	read.Page = &views.AccountsPage{PricesAsOf: "2026-10-01"}
	for _, tc := range []struct {
		name  string
		shown AccountsShown
		want  string
	}{
		{"the page read", read, "at the price table's prices as of 1 Oct 2026, worked out"},
		{"the page not yet read", threeShown, "at the price table's prices, worked out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if said := accountsHelpSaid(tc.shown); !strings.Contains(said, tc.want) {
				t.Errorf("help reads\n%s\nwant it to say %q", said, tc.want)
			}
		})
	}
}

func TestAccountsHelpSaysWhatItsFiguresAre(t *testing.T) {
	said := accountsHelpSaid(threeShown)
	for _, tc := range []struct{ what, want string }{
		{"a session served once", "each counted once, however many days and accounts it ran on"},
		{"Compare's keys", "← and → move the focus along the columns"},
		{"esc on Compare", "enter opens the account focused in Detail, and esc ends the focus"},
		{"the time at cap or limit", "the time it spent at its cap or at a limit"},
		{"left at its last reset", "what its week had left as it last reset, 100% less the peak of the week that ended last"},
		{"worth at API prices", "what the same tokens would have cost through the API, each version at its own prices"},
		{"why worth is unpriced", "of a model the price table doesn't know or of requests that went upstream without their usage"},
		{"how a period is priced", "a day 12/365 of a month, a week 12/52, 30 days a month, and 12 weeks twelve weeks'"},
		{"a period priced whole", "a period still running priced whole"},
		{"all accounts' plans", "sums the plans it knows, its ratio counting only their accounts' worth"},
		{"what all accounts counts", "Every configured account, always, whatever is pinned, at its limit or refused"},
		{"all accounts' room", "the sum of what each has left of it, to its limit"},
		{"why it isn't the pool", "It isn't the pool, the accounts new sessions can go to now, each to its cap"},
		{"Detail's windows", "every window read as switchboard usage reads them"},
		{"BY DAY's scale", "in History's five shades, on the account's own scale"},
		{"WEEKS' peaks", "the use read just before the reset that ended it, and this week's, its highest so far"},
	} {
		if !strings.Contains(said, tc.want) {
			t.Errorf("help reads\n%s\nwant it to explain %s, %q", said, tc.what, tc.want)
		}
	}
}

// History's help and Accounts' say each verdict at the cuts views judges it
// by, each word as views words it.
func TestHelpSaysEachVerdictAtViewsCuts(t *testing.T) {
	accountsVerdicts := []string{
		fmt.Sprintf("%s, in %d or more of the %d;", views.HitsItsLimit, views.LimitWeeks, views.VerdictWeeks),
		fmt.Sprintf("%s, %d%% or more left at a reset on average;", views.PlentyToSpare, views.PlentyLeft),
		"else " + views.EndsWithRoom + ".",
		fmt.Sprintf("%s%s, where the rest would cover it", strings.ToUpper(views.CouldGo[:1]), views.CouldGo[1:]),
		fmt.Sprintf("used %d%% or less on average;", views.CouldGoUsed),
		fmt.Sprintf("%s, %d%% or more left at a reset on average.", views.BarelyUsed, views.BarelyLeft),
	}
	for _, tc := range []struct {
		name string
		said string
		want []string
	}{
		{"History's", historySaid(), accountsVerdicts},
		{"Accounts'", accountsHelpSaid(threeShown), append(accountsVerdicts,
			fmt.Sprintf("%s where their weeks left %d%% or more at a reset", views.RoomToSpare, views.RoomLeft),
			"else "+views.LittleToSpare+",",
		)},
	} {
		for _, verdict := range tc.want {
			if !strings.Contains(tc.said, verdict) {
				t.Errorf("%s help reads\n%s\nwant it to say %q", tc.name, tc.said, verdict)
			}
		}
	}
}

// With one account, help explains what its Detail draws, THIS WEEK's limits
// by window and its extra usage among them, and says nothing of all
// accounts, which goes.
func TestAccountsHelpOfOneAccountExplainsWhatItDraws(t *testing.T) {
	said := accountsHelpSaid(oneShown)
	for _, want := range []string{
		"the limits it hit, by window",
		"Extra usage is on where Claude bills past the plan's limit",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("help of one account reads\n%s\nwant it to say %q", said, want)
		}
	}
	if strings.Contains(said, "all accounts") {
		t.Errorf("help of one account reads\n%s\nwant nothing of all accounts", said)
	}
}

// Compare's focus is explained where it's drawn, wide; a phone's Compare
// draws none.
func TestAccountsHelpExplainsComparesFocusWhereItsDrawn(t *testing.T) {
	const focus = "← and → move the focus along the columns"
	for _, tc := range []struct {
		width int
		want  bool
	}{{160, true}, {100, true}, {99, false}, {52, false}} {
		said := strings.Join(plainLines(accountsHelping(tc.width, threeShown, Look{}).body), " ")
		if got := strings.Contains(said, focus); got != tc.want {
			t.Errorf("at %d columns, help reads\n%s\nwant %q said %v", tc.width, said, focus, tc.want)
		}
	}
}

// Detail's list, and → and ← between it and SESSIONS, are explained where
// there's a list; with one account, which has none, ↑ and ↓ move through
// SESSIONS alone.
func TestAccountsHelpExplainsTheListWhereThereIsOne(t *testing.T) {
	const list, between = "The list chooses the account the panel shows", "→ takes the choice into them"
	for _, tc := range []struct {
		name     string
		shown    AccountsShown
		wantList bool
	}{
		{"three accounts", threeShown, true},
		{"one account", oneShown, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			said := accountsHelpSaid(tc.shown)
			for _, explained := range []string{list, between} {
				if got := strings.Contains(said, explained); got != tc.wantList {
					t.Errorf("help reads\n%s\nwant %q said %v", said, explained, tc.wantList)
				}
			}
			if !strings.Contains(said, "and enter opens its page") {
				t.Errorf("help reads\n%s\nwant enter on a session explained", said)
			}
		})
	}
}

func TestAccountsHelpFitsTheCardAtEveryWidth(t *testing.T) {
	for _, shown := range []AccountsShown{threeShown, oneShown} {
		for _, width := range []int{216, 160, 100, 52} {
			card := accountsHelping(width, shown, Look{}, tabViews, helpQuit[0], helpQuit[1])
			inside := card.width - 2*cardInset
			for i, row := range card.body {
				if row.width() > inside || strings.Contains(row.plain(), prose.Ellipsis) {
					t.Errorf("at %d columns of %d accounts, help's row %d is %q, %d cells; want it whole within %d",
						width, len(shown.Accounts), i+1, row.plain(), row.width(), inside)
				}
			}
		}
	}
}
