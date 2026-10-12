package dashboard

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// historyDrawn is the sub-tab of History f shows, drawn alone on a canvas of
// its own, from its view's top.
func historyDrawn(f Frame) *canvas {
	c := newCanvas(f.Width, f.Height)
	f.history(c, f.top())
	return c
}

// History's heading row names it and its sub-tabs, the one shown picked out,
// and at its right the Year's period, the last 12 months or a calendar year,
// and Tokens', between < and >, or eight weeks, which Weeks and Days keep to.
// Till its days are read, nothing is drawn under it.
func TestHistorysHeadingRow(t *testing.T) {
	const named = "HISTORY    Year  Weeks  Days  Tokens  [ ]  "
	tests := []struct {
		name    string
		page    HistoryPage
		shownAt int
		want    string
	}{
		{name: "the Year, over the last 12 months", shownAt: 10, want: named + rule(97) + "  < last 12 months >"},
		{name: "the Year, stepped back to a calendar year", page: HistoryPage{YearAsk: views.YearAsk{Year: 2025}}, shownAt: 10, want: named + rule(107) + "  < 2025 >"},
		{name: "Weeks", page: HistoryPage{Shown: HistoryWeeks}, shownAt: 16, want: named + rule(104) + "  eight weeks"},
		{name: "Days", page: HistoryPage{Shown: HistoryDays}, shownAt: 23, want: named + rule(104) + "  eight weeks"},
		{name: "Tokens, by week", page: HistoryPage{Shown: HistoryTokens}, shownAt: 29, want: named + rule(104) + "  < by week >"},
		{
			name: "Tokens, by day", page: HistoryPage{Shown: HistoryTokens, TokensAsk: views.TokensAsk{Period: views.TokensByDay}}, shownAt: 29,
			want: named + rule(105) + "  < by day >",
		},
		{
			name: "Tokens, by month", page: HistoryPage{Shown: HistoryTokens, TokensAsk: views.TokensAsk{Period: views.TokensByMonth}}, shownAt: 29,
			want: named + rule(103) + "  < by month >",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 37)
			f.View, f.History = History, tt.page
			c := f.page(threeRouted(), now)
			rows := c.rows(Look{})

			if got := rows[4]; got != tt.want {
				t.Errorf("row 5 reads\n%q\nwant\n%q", got, tt.want)
			}
			for x, sub := range []int{10, 16, 23, 29} {
				want := tabInk
				if sub == tt.shownAt {
					want = shownSubTabInk
				}
				if got := c.at(sub, 4).ink; got != want {
					t.Errorf("sub-tab %d is in %+v, want %+v", x, got, want)
				}
			}
			for i, row := range rows[5:36] {
				if row != "" {
					t.Errorf("row %d = %q, want nothing drawn under the heading till the days are read", i+6, row)
				}
			}
		})
	}
}

// What's out of view below says how many more rows there are, each counted
// one or many, and how to scroll to them: ▼ in accent.key, the words muted,
// the · between them faint, and j/k bold accent.key.
func TestMoreBelowSaysWhatsOutOfViewAndHowToScrollToIt(t *testing.T) {
	key, words, apart, keys := ink{token: theme.AccentKey}, ink{token: theme.TextMuted}, ink{token: theme.TextFaint}, ink{token: theme.AccentKey, bold: true}
	tests := []struct {
		name           string
		n              int
		noun, toScroll string
		want           line
	}{
		{
			name: "Days' accounts, as history-days-ten-160 draws them", n: 4, noun: "account", toScroll: " or the wheel to scroll them",
			want: line{{"▼", key}, {" 4 more accounts below", words}, {"   ·   ", apart}, {"j/k", keys}, {" or the wheel to scroll them", words}},
		},
		{
			name: "Tokens' weeks, as history-tokens-160 draws them", n: 3, noun: "week", toScroll: " to scroll",
			want: line{{"▼", key}, {" 3 more weeks below", words}, {"   ·   ", apart}, {"j/k", keys}, {" to scroll", words}},
		},
		{
			name: "one", n: 1, noun: "month", toScroll: " to scroll",
			want: line{{"▼", key}, {" 1 more month below", words}, {"   ·   ", apart}, {"j/k", keys}, {" to scroll", words}},
		},
	}
	for _, tt := range tests {
		if got := moreBelow(tt.n, tt.noun, tt.toScroll); !slices.Equal(got, tt.want) {
			t.Errorf("%s: moreBelow(%d, %q, %q) = %+v, want %+v", tt.name, tt.n, tt.noun, tt.toScroll, got, tt.want)
		}
	}
}
