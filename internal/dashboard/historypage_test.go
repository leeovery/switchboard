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

// On a phone, History's heading sets its sub-tabs two cells after its name, its
// period on the row under them, at the right, and the view starts two rows
// on, as history-phone-year-52 draws it.
func TestHistorysHeadingOnAPhone(t *testing.T) {
	f := frameOf(52, 35)
	f.View = History
	rows := f.page(threeRouted(), now).rows(Look{})

	if got, want := rows[5], "HISTORY   Year  Weeks  Days  Tokens"; strings.TrimRight(got, " ") != want {
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
		{name: "the Year, below 37 rows", f: yearFrame(t, besideFrom, 30, views.YearAsk{}, plainYear()), want: Scrolling{Most: 7, Page: 21}, rows: true, over: 6, past: 28},
		{name: "the Year, at 37 rows", f: yearFrame(t, besideFrom, 37, views.YearAsk{}, plainYear()), want: Scrolling{Page: 29}, rows: true, over: 34, past: 35},
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
			if got := tt.f.ScrollsRows(status.Document{}, now); got != tt.rows {
				t.Errorf("ScrollsRows() = %v, want %v", got, tt.rows)
			}
		})
	}
}

// Scrolled, the Year shows its rows from as far down as it's scrolled, cut
// a row short of the view's foot, its last row saying how many rows are out
// of view above and below, as every view scrolled whole says it (the owner's
// call, 15 October 2026), the rows under it left to the key line; scrolled
// past its end, as far as it goes; at its top, nothing above.
func TestAScrolledYearShowsItsRowsFromWhereItsScrolledTo(t *testing.T) {
	whole := textOf(historyDrawn(yearFrame(t, besideFrom, 37, views.YearAsk{}, plainYear())).rows(Look{}))[6:]
	tests := []struct {
		scroll, from int
		out          string
	}{
		{scroll: 0, from: 0, out: "▼ 7 more below   ·   j/k to scroll"},
		{scroll: 4, from: 4, out: "▲ 4 more above   ·   ▼ 3 more below   ·   j/k to scroll"},
		{scroll: 99, from: 7, out: "▲ 7 more above   ·   j/k to scroll"},
	}
	for _, tt := range tests {
		f := yearFrame(t, besideFrom, 30, views.YearAsk{}, plainYear())
		f.Scroll = tt.scroll
		rows := textOf(historyDrawn(f).rows(Look{}))
		if !slices.Equal(rows[6:27], whole[tt.from:tt.from+21]) || rows[27] != tt.out || rows[28] != "" || rows[29] != "" {
			t.Errorf("scrolled %d, the Year reads\n%s\nwant its rows from row %d of\n%s\nthen %q", tt.scroll, strings.Join(rows, "\n"), tt.from, strings.Join(whole, "\n"), tt.out)
		}
	}
}

// History's scrolling, asked at an offset, is how far its view scrolls
// scrolled there, whatever the frame has it scrolled to: the Year's, and
// Weeks' and Days' drawn whole, whose page ends over a heading it would show
// without its first row, so how many rows it shows changes as it scrolls.
func TestHistoryScrollingIsAsFarAsItScrollsAtTheOffsetAsked(t *testing.T) {
	tests := []struct {
		name string
		f    Frame
	}{
		{name: "the Year", f: yearFrame(t, besideFrom, 21, views.YearAsk{}, plainYear())},
		{name: "Weeks drawn whole", f: weeksFrame(tenWeeks(), WeeksTable, 160, 16, Look{})},
		{name: "Days drawn whole", f: daysFrame(t, daysWorld(tenDays...), 160, 24)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{}
			asked := tt.f.At(doc, now).scrollingAt(doc, now)
			pages := map[int]bool{}
			for offset := range tt.f.Scrolling(doc, now).Most + 1 {
				scrolled := tt.f
				scrolled.Scroll = offset
				want := scrolled.Scrolling(doc, now)
				pages[want.Page] = true
				if got := asked(offset); got != want {
					t.Errorf("asked at %d, it scrolls %+v, want %+v, as scrolled there", offset, got, want)
				}
			}
			if len(pages) < 2 {
				t.Errorf("it shows %v rows at every offset, want a page that changes as it scrolls, to tell offsets apart", pages)
			}
		})
	}
}

// A Year that fits says nothing of what's out of view: 37 rows tall with
// its numbers beside the grid, and on a phone 61 rows tall.
func TestAYearThatFitsSaysNothingOutOfView(t *testing.T) {
	for _, size := range []struct{ width, height int }{{besideFrom, 37}, {73, 61}} {
		rows := textOf(historyDrawn(yearFrame(t, size.width, size.height, views.YearAsk{}, plainYear())).rows(Look{}))
		if i := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "more below") }); i >= 0 {
			t.Errorf("at %d × %d, row %d reads %q, want nothing out of view", size.width, size.height, i+1, rows[i])
		}
	}
}

// A Year too tall for the view shows a heading with its first row under it,
// or ends its page over the heading: BY MONTH with its heads and first
// month, and its numbers' heading, under the key, with its first figures.
func TestAScrolledYearShowsNoHeadingWithoutItsFirstRow(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		heading       string
		want          Scrolling
	}{
		{name: "BY MONTH, its heading the page's last row", width: besideFrom, height: 21, heading: "BY MONTH", want: Scrolling{Most: 16, Page: 11}},
		{name: "BY MONTH, its heads the page's last row", width: besideFrom, height: 22, heading: "BY MONTH", want: Scrolling{Most: 15, Page: 11}},
		{name: "a phone's numbers, their heading the page's last row", width: 52, height: 23, heading: "LAST 12 MONTHS", want: Scrolling{Most: 21, Page: 11}},
	}
	for _, tt := range tests {
		f := yearFrame(t, tt.width, tt.height, views.YearAsk{}, plainYear())
		rows := textOf(historyDrawn(f).rows(Look{}))
		if got := f.Scrolling(status.Document{}, now); got != tt.want || rowWith(rows, tt.heading) >= 0 {
			t.Errorf("%s: the Year scrolls %+v, want %+v, and reads\n%s\nwant no %s", tt.name, got, tt.want, strings.Join(rows, "\n"), tt.heading)
		}
		f = yearFrame(t, tt.width, tt.height+2, views.YearAsk{}, plainYear())
		if rows := textOf(historyDrawn(f).rows(Look{})); rowWith(rows, tt.heading) < 0 {
			t.Errorf("%s: two rows taller, the Year reads\n%s\nwant %s with its first row", tt.name, strings.Join(rows, "\n"), tt.heading)
		}
	}
}

// What's out of view of Tokens' periods says how many more are below, each
// counted one or many, and how to scroll to them: ▼ in accent.key, the words
// muted, the · between them faint, and j/k bold accent.key; cut to the width,
// its count a figure, drawn whole with ▼, its words cut, else ▼ alone.
func TestWhatsOutOfViewOfTokensPeriodsSaysHowToScrollToThem(t *testing.T) {
	key, words, apart, keys := ink{token: theme.AccentKey}, ink{token: theme.TextMuted}, ink{token: theme.TextFaint}, ink{token: theme.AccentKey, bold: true}
	tests := []struct {
		name     string
		n, width int
		noun     string
		want     line
	}{
		{
			name: "Tokens' weeks, as history-tokens-160 draws them", n: 3, width: 160, noun: "week",
			want: line{{"▼", key}, {" 3 more weeks below", words}, {"   ·   ", apart}, {"j/k", keys}, {" to scroll", words}},
		},
		{
			name: "one", n: 1, width: 160, noun: "month",
			want: line{{"▼", key}, {" 1 more month below", words}, {"   ·   ", apart}, {"j/k", keys}, {" to scroll", words}},
		},
		{name: "its words cut", n: 12, width: 12, noun: "week", want: line{{"▼", key}, {" 12 more w…", words}}},
		{name: "its count filling it", n: 12, width: 4, noun: "week", want: line{{"▼", key}, {" 12", words}}},
		{name: "its count a cell short", n: 12, width: 3, noun: "week", want: line{{"▼", key}}},
		{name: "no room", n: 12, noun: "week"},
	}
	for _, tt := range tests {
		if got := outOfView(0, tt.n, tt.width, periodsOut(tt.noun)); !slices.Equal(got.inked(), tt.want) {
			t.Errorf("%s: %d more %ss below, in %d cells, the line reads %q, %+v, want %q, %+v", tt.name, tt.n, tt.noun, tt.width, got.plain(), got, tt.want.plain(), tt.want)
		}
	}
}

// What's out of view of a view's accounts, above and below, is said in the
// longest of its forms that fits the width given: the design's, as
// history-days-ten-160 draws it, ▲ and ▼ in accent.key, the words muted, the
// · between them faint, and j/k bold accent.key; spaced as USAGE's scroll
// line is; without how to scroll; and as USAGE's rows say it; and where
// none fits, as the last, one count: below where any are, cut short where it
// doesn't fit rather than above, else above; its count a figure, drawn whole
// with its mark, its words cut, and where that doesn't fit, the mark alone.
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
		{above: 2, below: 12, width: 5, want: line{{"▼", key}, {" 12…", words}}},
		{above: 2, below: 12, width: 4, want: line{{"▼", key}, {" 12", words}}},
		{above: 2, below: 12, width: 3, want: line{{"▼", key}}},
		{above: 2, below: 12, width: 0},
	}
	for _, tt := range tests {
		got := outOfView(tt.above, tt.below, tt.width, historyAccountsOut...)
		if !slices.Equal(got.inked(), tt.want) || got.width() > tt.width {
			t.Errorf("%d above and %d below, in %d cells, the line reads %q, %+v, want %q, %+v", tt.above, tt.below, tt.width, got.plain(), got, tt.want.plain(), tt.want)
		}
	}
}
