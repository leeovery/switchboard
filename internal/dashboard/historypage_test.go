package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
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

// Shown for one version, or one account, the Year's heading says so after
// [ ], in text.faint, its v bold accent.key, the rule from a cell after it, as
// history-year-opus-160 draws it: an account by its label, and a version once
// the last 12 months are read, as one kept is checked against those they
// used. Nothing's said of all use, nor on another sub-tab; nor of a version
// till then, nor of an account kept that's taken for one, as one no longer
// configured is, or that's the one of one alone, which shows all use.
func TestHistorysHeadingSaysWhatTheYearIsShownForAlone(t *testing.T) {
	const named = "HISTORY    Year  Weeks  Days  Tokens  [ ]   "
	nothing := "HISTORY    Year  Weeks  Days  Tokens  [ ]  " + rule(97) + "  < last 12 months >"
	labelled := config.Accounts{{ID: "work", Label: "Work"}, {ID: "side"}}
	tests := []struct {
		name string
		page HistoryPage
		want string
		// v is the column v is drawn at.
		v int
	}{
		{
			name: "a version", page: HistoryPage{Read: true, YearAsk: views.YearAsk{Version: "Opus 5.5"}}, v: 77,
			want: named + "Opus 5.5 only, every account  ·  v for all, a model or an account " + rule(30) + "  < last 12 months >",
		},
		{name: "a version, till the last 12 months are read", page: HistoryPage{YearAsk: views.YearAsk{Version: "Opus 5.5"}}, want: nothing},
		{name: "an account kept since removed, till then", page: HistoryKept(theme.HistoryPrefs{Year: "personal"}, labelled), want: nothing},
		{name: "the account kept of one alone", page: HistoryKept(theme.HistoryPrefs{Year: "work"}, labelled[:1]), want: nothing},
		{
			name: "an account, by its label", page: HistoryPage{YearAsk: views.YearAsk{Account: "work"}, Accounts: labelled}, v: 71,
			want: named + "Work only, every model  ·  v for all, a model or an account " + rule(36) + "  < last 12 months >",
		},
		{
			name: "an account the config labels with its id", page: HistoryPage{YearAsk: views.YearAsk{Account: "side", Year: 2025}, Accounts: labelled}, v: 71,
			want: named + "side only, every model  ·  v for all, a model or an account " + rule(46) + "  < 2025 >",
		},
		{
			name: "on Weeks, nothing", page: HistoryPage{Shown: HistoryWeeks, YearAsk: views.YearAsk{Version: "Opus 5.5"}},
			want: "HISTORY    Year  Weeks  Days  Tokens  [ ]  " + rule(104) + "  eight weeks",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 37)
			f.View, f.History = History, tt.page
			c := f.page(threeRouted(), now)
			if got := c.rows(Look{})[4]; got != tt.want {
				t.Errorf("row 5 reads\n%q\nwant\n%q", got, tt.want)
			}
			if tt.v == 0 {
				return
			}
			for x, want := range map[int]ink{44: faintInk, tt.v - 1: faintInk, tt.v: keyInk, tt.v + 1: faintInk} {
				if got := c.at(x, 4).ink; got != want {
					t.Errorf("column %d is in %+v, want %+v", x, got, want)
				}
			}
		})
	}
}

// On a phone, History's heading sets its sub-tabs tight after its name, its
// period on the row under them, at the right, and the view starts two rows
// on, as history-phone-year-52 draws it.
func TestHistorysHeadingOnAPhone(t *testing.T) {
	f := frameOf(52, 35)
	f.View = History
	rows := f.page(threeRouted(), now).rows(Look{})

	if got, want := rows[5], "HISTORY  Year Weeks Days Tokens"; strings.TrimRight(got, " ") != want {
		t.Errorf("row 6 reads %q, want %q", got, want)
	}
	if got, want := rows[6], blanks(34)+"< last 12 months >"; got != want {
		t.Errorf("row 7 reads %q, want %q", got, want)
	}
	if top := f.top(); top != 8 {
		t.Errorf("the view starts at row %d, want row 9", top+1)
	}
}

// History scrolls as its sub-tab shown does, whatever the router's document
// holds, as it's drawn of the ledger's days alone: the Year a row at a time,
// as far as it runs past the view, Weeks and Days an account at a time, and
// Tokens a period at a time; and the wheel scrolls it over what of it
// scrolls alone: the Year's view, the accounts Weeks shows, BY ACCOUNT, and
// the periods Tokens shows.
func TestHistoryScrollsAsItsSubTabShownDoes(t *testing.T) {
	tokens := tokensFrame(160, 37, TokensByAccount)
	for range 3 {
		tokens.History.Tokens.Periods = append(tokens.History.Tokens.Periods, pastTokens().Periods...)
	}
	tests := []struct {
		name string
		f    Frame
		want Scrolling
		rows bool
		// over is a row of what scrolls, and past one of the view's that
		// doesn't.
		over, past int
	}{
		{name: "the Year, below 37 rows", f: yearFrame(t, 160, 30, views.YearAsk{}, plainYear()), want: Scrolling{Most: 6, Page: 22}, rows: true, over: 6, past: 28},
		{name: "the Year, at 37 rows", f: yearFrame(t, 160, 37, views.YearAsk{}, plainYear()), want: Scrolling{Page: 29}, rows: true, over: 34, past: 35},
		{name: "Weeks", f: weeksFrame(manyWeeks(25), WeeksTable, 160, 37, Look{}), want: Scrolling{Most: 6, Page: 19}, over: 14, past: 6},
		{name: "Days", f: daysFrame(t, daysWorld(tenDays...), 160, 37), want: Scrolling{Most: 4, Page: 6}, over: 21, past: 12},
		{name: "Tokens", f: tokens, want: Scrolling{Most: 6, Page: 6}, over: 8, past: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, doc := range []status.Document{threeRouted(), {}} {
				if got := tt.f.Scrolling(doc, now); got != tt.want {
					t.Errorf("of %d accounts, it scrolls %+v, want %+v", len(doc.Accounts), got, tt.want)
				}
				if got := tt.f.Under(doc, now, 10, tt.over); got != UnderView {
					t.Errorf("row %d is under %v, want what scrolls", tt.over+1, got)
				}
				if got := tt.f.Under(doc, now, 10, tt.past); got != UnderNothing {
					t.Errorf("row %d is under %v, want nothing that scrolls", tt.past+1, got)
				}
			}
			if got := tt.f.ScrollsRows(); got != tt.rows {
				t.Errorf("ScrollsRows() = %v, want %v", got, tt.rows)
			}
		})
	}
}

// Scrolled, the Year shows its rows from as far down as it's scrolled, cut
// at the view's foot, the rows under it left to the key line; scrolled past
// its end, as far as it goes.
func TestAScrolledYearShowsItsRowsFromWhereItsScrolledTo(t *testing.T) {
	whole := textOf(historyDrawn(yearFrame(t, 160, 37, views.YearAsk{}, plainYear())).rows(Look{}))[6:]
	for _, tt := range []struct{ scroll, from int }{{scroll: 4, from: 4}, {scroll: 99, from: 6}} {
		f := yearFrame(t, 160, 30, views.YearAsk{}, plainYear())
		f.Scroll = tt.scroll
		rows := textOf(historyDrawn(f).rows(Look{}))
		if !slices.Equal(rows[6:28], whole[tt.from:tt.from+22]) || rows[28] != "" || rows[29] != "" {
			t.Errorf("scrolled %d, the Year reads\n%s\nwant its rows from row %d of\n%s", tt.scroll, strings.Join(rows, "\n"), tt.from, strings.Join(whole, "\n"))
		}
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

// What's out of view of a view's accounts, above and below, is said in the
// longest of its forms that fits the width given: the design's, as
// history-days-ten-160 draws it, ▲ and ▼ in accent.key, the words muted, the
// · between them faint, and j/k bold accent.key; spaced as USAGE's scroll
// line is; without how to scroll; and as USAGE's rows say it; and where
// none fits, as the last, one count: below where any are, cut short where it
// doesn't fit rather than above, else above.
func TestWhatsOutOfViewOfTheAccountsIsSaidInTheLongestFormThatFits(t *testing.T) {
	key, words, apart, keys := ink{token: theme.AccentKey}, ink{token: theme.TextMuted}, ink{token: theme.TextFaint}, ink{token: theme.AccentKey, bold: true}
	tests := []struct {
		above, below, width int
		want                line
	}{
		{below: 4, width: 160, want: line{{"▼", key}, {" 4 more accounts below", words}, {"   ·   ", apart}, {"j/k", keys}, {" or the wheel to scroll them", words}}},
		{above: 2, width: 160, want: line{{"▲", key}, {" 2 more accounts above", words}, {"   ·   ", apart}, {"j/k", keys}, {" or the wheel to scroll them", words}}},
		{
			above: 2, below: 4, width: 91,
			want: line{
				{"▲", key}, {" 2 more accounts above", words}, {"   ·   ", apart},
				{"▼", key}, {" 4 more accounts below", words}, {"   ·   ", apart}, {"j/k", keys}, {" or the wheel to scroll them", words},
			},
		},
		{below: 4, width: 60, want: line{{"▼", key}, {" 4 more accounts below", words}, {" · ", apart}, {"j/k", keys}, {" or wheel to scroll", words}}},
		{
			above: 2, below: 4, width: 90,
			want: line{
				{"▲", key}, {" 2 more accounts above", words}, {" · ", apart},
				{"▼", key}, {" 4 more accounts below", words}, {" · ", apart}, {"j/k", keys}, {" or wheel to scroll", words},
			},
		},
		{above: 2, below: 4, width: 51, want: line{{"▲", key}, {" 2 more accounts above", words}, {" · ", apart}, {"▼", key}, {" 4 more accounts below", words}}},
		{above: 2, below: 4, width: 48, want: line{{"▲", key}, {" 2 more accounts", words}, {" · ", apart}, {"▼", key}, {" 4 more accounts", words}}},
		{above: 2, below: 4, width: 20, want: line{{"▼", key}, {" 4 more accounts", words}}},
		{above: 2, below: 12, width: 17, want: line{{"▼", key}, {" 12 more accoun…", words}}},
		{above: 2, below: 4, width: 12, want: line{{"▼", key}, {" 4 more ac…", words}}},
		{above: 2, width: 12, want: line{{"▲", key}, {" 2 more ac…", words}}},
	}
	for _, tt := range tests {
		got := accountsOutOfView(tt.above, tt.below, tt.width)
		if !slices.Equal(got, tt.want) || got.width() > tt.width {
			t.Errorf("%d above and %d below, in %d cells, the line reads %+v, want %+v", tt.above, tt.below, tt.width, got, tt.want)
		}
	}
}
