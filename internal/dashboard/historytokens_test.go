package dashboard

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// byKind are tokens by kind, their total the four.
func byKind(input, output, cacheWrite, cacheRead int) views.Tokens {
	return views.Tokens{Input: input, Output: output, CacheWrite: cacheWrite, CacheRead: cacheRead, Total: input + output + cacheWrite + cacheRead}
}

// worthIn is worth of the dollars given, what it names unpriced left out.
func worthIn(n float64, unpriced ...string) views.Worth {
	return views.Worth{Amount: usd(n), Unpriced: unpriced}
}

// againstAt is worth ratio times what the plans cost, cost dollars.
func againstAt(ratio, cost float64) views.Against {
	return views.Against{Cost: usd(cost), Ratio: &ratio}
}

// versionTokens is a model version's tokens and worth, by its id, on the accounts
// given.
func versionTokens(id string, t views.Tokens, w views.Worth, on ...string) views.ModelTokens {
	return views.ModelTokens{Model: id, On: on, Tokens: t, Worth: w}
}

// pastTokens is Tokens' page as the views give it of three accounts by week:
// this week, so far, its worth part unpriced, and the two before it, the
// last under its price.
func pastTokens() *views.TokensPage {
	opus, sonnet := "claude-opus-5-5", "claude-sonnet-5-5"
	return &views.TokensPage{PricesAsOf: "2026-10-01", Periods: []views.TokenPeriod{
		{
			First: "2026-09-28", Versions: 2, SoFar: true,
			Tokens: byKind(4_200_000, 2_900_000, 38_000_000, 615_000_000), Worth: worthIn(635, "claude-opus-9"), Against: againstAt(4.6, 138),
			ByAccount: []views.AccountTokens{
				{Account: "work", Tokens: byKind(1_200_000, 800_000, 11_000_000, 178_000_000), Worth: worthIn(184), ByModel: []views.ModelTokens{
					versionTokens(opus, byKind(1_000_000, 600_000, 9_000_000, 150_000_000), worthIn(150)),
					versionTokens(sonnet, byKind(200_000, 200_000, 2_000_000, 28_000_000), worthIn(34)),
				}},
				{Account: "personal", Tokens: byKind(1_600_000, 1_100_000, 14_200_000, 241_000_000), Worth: worthIn(212), ByModel: []views.ModelTokens{
					versionTokens(sonnet, byKind(1_600_000, 1_100_000, 14_200_000, 241_000_000), worthIn(212)),
				}},
				{Account: "side", Tokens: byKind(1_400_000, 1_000_000, 12_800_000, 196_000_000), Worth: worthIn(239, "claude-opus-9"), ByModel: []views.ModelTokens{
					versionTokens(opus, byKind(1_400_000, 1_000_000, 12_800_000, 196_000_000), worthIn(239, "claude-opus-9")),
				}},
			},
			ByModel: []views.ModelTokens{
				versionTokens(opus, byKind(2_400_000, 1_600_000, 21_800_000, 346_000_000), worthIn(389, "claude-opus-9"), "side", "work"),
				versionTokens(sonnet, byKind(1_800_000, 1_300_000, 16_200_000, 269_000_000), worthIn(246), "personal", "work"),
			},
		},
		{
			First: "2026-09-21", Versions: 1,
			Tokens: byKind(6_700_000, 4_600_000, 60_500_000, 985_000_000), Worth: worthIn(998), Against: againstAt(7.2, 138),
			ByAccount: []views.AccountTokens{
				{Account: "work", Tokens: byKind(6_200_000, 4_200_000, 55_600_000, 909_600_000), Worth: worthIn(906), ByModel: []views.ModelTokens{
					versionTokens(opus, byKind(6_200_000, 4_200_000, 55_600_000, 909_600_000), worthIn(906)),
				}},
				{Account: "side", Tokens: byKind(500_000, 400_000, 4_900_000, 75_400_000), Worth: worthIn(92), ByModel: []views.ModelTokens{
					versionTokens(opus, byKind(500_000, 400_000, 4_900_000, 75_400_000), worthIn(92)),
				}},
			},
			ByModel: []views.ModelTokens{versionTokens(opus, byKind(6_700_000, 4_600_000, 60_500_000, 985_000_000), worthIn(998), "work", "side")},
		},
		{
			First: "2026-09-14", Versions: 1,
			Tokens: byKind(12, 845, 3_120, 182_340), Worth: worthIn(0.08), Against: againstAt(0.4, 138),
			ByAccount: []views.AccountTokens{
				{Account: "work", Tokens: byKind(12, 845, 3_120, 182_340), Worth: worthIn(0.08), ByModel: []views.ModelTokens{
					versionTokens(opus, byKind(12, 845, 3_120, 182_340), worthIn(0.08)),
				}},
			},
			ByModel: []views.ModelTokens{versionTokens(opus, byKind(12, 845, 3_120, 182_340), worthIn(0.08), "work")},
		},
	}}
}

// tokensFrame is a frame the size given showing Tokens of pastTokens, split
// as given.
func tokensFrame(width, height int, split TokensSplit) Frame {
	f := frameOf(width, height)
	f.View = History
	f.History = HistoryPage{Shown: HistoryTokens, TokensSplit: split, Read: true, Tokens: pastTokens()}
	return f
}

// tokensRowsOf are the rows of Tokens the frame draws, from its heads to the
// view's last row.
func tokensRowsOf(f Frame) []string {
	rows := f.Draw(threeRouted(), now)
	return rows[f.top() : f.top()+f.viewRows()]
}

// Tokens draws its heads, then each period, newest first, a blank row
// between them, its rows as split, and its footnote on the view's last row,
// as history-tokens-160 lays them out; a phone shows each row's total and
// worth alone.
func TestTokensDrawsEachPeriodAsSplit(t *testing.T) {
	const (
		heads = "week          models                 input      output cache write    cache read         total  worth     against its price"
		note  = "worth: what the same tokens would cost through the API, at its prices as of 1 Oct 2026"
	)
	tests := []struct {
		name          string
		width, height int
		split         TokensSplit
		want          []string
	}{
		{
			name: "by account, at 160 columns", width: 160, height: 37, split: TokensByAccount,
			want: []string{
				heads,
				"",
				"28 Sep        2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
				"└ work        2 models                1.2M        0.8M       11.0M          178M          191M  $184",
				"└ personal    1 model                 1.6M        1.1M       14.2M          241M          258M  $212",
				"└ side        1 model                 1.4M        1.0M       12.8M          196M          211M  $239+",
				"",
				"21 Sep        1 model                 6.7M        4.6M       60.5M          985M         1.06B  $998      7.2× its $138 week",
				"└ work        1 model                 6.2M        4.2M       55.6M          910M          976M  $906",
				"└ side        1 model                 0.5M        0.4M        4.9M         75.4M         81.2M  $92",
				"",
				"14 Sep        1 model                 0.0M        0.0M        0.0M          0.2M          0.2M  $0        0.4× its $138 week",
				"└ work        1 model                 0.0M        0.0M        0.0M          0.2M          0.2M  $0",
			},
		},
		{
			name: "by model, its accounts on it", width: 160, height: 37, split: TokensByModel,
			want: []string{
				strings.Replace(heads, "models", "on    ", 1),
				"",
				"28 Sep        2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
				"└ opus-5-5    side work               2.4M        1.6M       21.8M          346M          372M  $389+",
				"└ sonnet-5-5  personal work           1.8M        1.3M       16.2M          269M          288M  $246",
			},
		},
		{
			name: "by account and model, each account's versions nested under it", width: 160, height: 37, split: TokensByAccountAndModel,
			want: []string{
				heads,
				"",
				"28 Sep        2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
				"└ work        2 models                1.2M        0.8M       11.0M          178M          191M  $184",
				"  └ opus-5-5                          1.0M        0.6M        9.0M          150M          161M  $150",
				"  └ sonnet-5-5                        0.2M        0.2M        2.0M         28.0M         30.4M  $34",
				"└ personal    1 model                 1.6M        1.1M       14.2M          241M          258M  $212",
				"  └ sonnet-5-5                        1.6M        1.1M       14.2M          241M          258M  $212",
			},
		},
		{
			name: "unsplit", width: 160, height: 37, split: TokensUnsplit,
			want: []string{
				heads,
				"",
				"28 Sep        2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
				"",
				"21 Sep        1 model                 6.7M        4.6M       60.5M          985M         1.06B  $998      7.2× its $138 week",
				"",
				"14 Sep        1 model                 0.0M        0.0M        0.0M          0.2M          0.2M  $0        0.4× its $138 week",
			},
		},
		{
			name: "on a phone, by account", width: 52, height: 37, split: TokensByAccount,
			want: []string{
				"week                     total  worth",
				"",
				"28 Sep                    660M  $635+",
				"└ work                    191M  $184",
				"└ personal                258M  $212",
				"└ side                    211M  $239+",
				"",
				"21 Sep                   1.06B  $998",
				"└ work                    976M  $906",
				"└ side                   81.2M  $92",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tokensFrame(tt.width, tt.height, tt.split)
			rows := tokensRowsOf(f)
			if got := rows[:len(tt.want)]; !slices.Equal(got, tt.want) {
				t.Errorf("Tokens reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// The footnote, in text.faint on the view's last rows, says what worth is
// and the price table's date; split by model, or by account and model, that
// each version is at its own price. A phone wraps it.
func TestTokensFootnoteSaysWhatWorthIsAndAsOfWhen(t *testing.T) {
	const note = "worth: what the same tokens would cost through the API, at its prices as of 1 Oct 2026"
	tests := []struct {
		name  string
		width int
		split TokensSplit
		want  []string
	}{
		{name: "by account", width: 160, split: TokensByAccount, want: []string{note}},
		{name: "unsplit", width: 160, split: TokensUnsplit, want: []string{note}},
		{name: "by model", width: 160, split: TokensByModel, want: []string{note + ", each version at its own price"}},
		{name: "by account and model", width: 160, split: TokensByAccountAndModel, want: []string{note + ", each version at its own price"}},
		{
			name: "on a phone", width: 52, split: TokensByAccount,
			want: []string{"worth: what the same tokens would cost through the", "API, at its prices as of 1 Oct 2026"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tokensFrame(tt.width, 37, tt.split)
			rows := tokensRowsOf(f)
			if got := rows[len(rows)-len(tt.want):]; !slices.Equal(got, tt.want) {
				t.Errorf("the footnote reads %q, want %q", got, tt.want)
			}
			c := f.page(threeRouted(), now)
			if got := c.at(0, f.top()+f.viewRows()-1).ink; got != faintInk {
				t.Errorf("the footnote is in %+v, want text.faint", got)
			}
		})
	}
}

// Tokens names its periods by their first day by week, their day by day, and
// their month by month, heading the column so, and prices each against its
// plans for the period it is.
func TestTokensNamesEachPeriodAsItIs(t *testing.T) {
	tests := []struct {
		period      views.TokensPeriod
		first, head string
		row         string
	}{
		{period: views.TokensByWeek, first: "2026-09-28", head: "week", row: "28 Sep        2 models"},
		{period: views.TokensByDay, first: "2026-10-01", head: "day", row: "Thu 1 Oct     2 models"},
		{period: views.TokensByMonth, first: "2026-10", head: "month", row: "Oct 2026      2 models"},
	}
	for _, tt := range tests {
		t.Run(tt.head, func(t *testing.T) {
			f := tokensFrame(160, 37, TokensUnsplit)
			f.History.TokensAsk.Period = tt.period
			f.History.Tokens.Periods = f.History.Tokens.Periods[:1]
			f.History.Tokens.Periods[0].First = tt.first
			rows := tokensRowsOf(f)
			if !strings.HasPrefix(rows[0], tt.head+" ") || !strings.HasPrefix(rows[2], tt.row) {
				t.Errorf("Tokens reads %q over %q, want %q over %q", rows[0], rows[2], tt.head, tt.row)
			}
			if want := "4.6× its $138 " + tt.period.Name(); !strings.HasSuffix(rows[2], want) {
				t.Errorf("the period reads %q, want it to end %q", rows[2], want)
			}
		})
	}
}

// Tokens names each account as Weeks and Days do, by its label, else
// its id: its row, split by account, and among those a version ran on, split
// by model; and the requests none answered, no account.
func TestTokensNamesTheAccountsByTheirLabels(t *testing.T) {
	tests := []struct {
		split TokensSplit
		want  []string
	}{
		{split: TokensByAccount, want: []string{"└ Laptop      2 models", "└ personal    1 model", "└ no account  1 model"}},
		{split: TokensByModel, want: []string{"└ opus-5-5    no account Laptop", "└ sonnet-5-5  personal Laptop"}},
	}
	for _, tt := range tests {
		f := tokensFrame(160, 37, tt.split)
		period := &f.History.Tokens.Periods[0]
		period.ByAccount[0].Label = "Laptop"
		period.ByAccount[2].Account = ""
		period.ByModel[0].On = []string{"", "work"}
		rows := tokensRowsOf(f)
		for _, want := range tt.want {
			if !slices.ContainsFunc(rows, func(row string) bool { return strings.HasPrefix(row, want) }) {
				t.Errorf("split %d, Tokens reads\n%s\nwant a row starting %q", tt.split, strings.Join(rows, "\n"), want)
			}
		}
	}
}

// Tokens' inks, as history-tokens-160 draws them: the heads in text.subtle;
// a period bold text.primary, its versions' count text.muted, and against
// its price bold, state.positive at 1× or more and accent.attention under,
// then the price in text.subtle; under it, └ faint, an account
// text.secondary, its versions' count text.subtle, and its counts text.muted;
// a worth's unpriced + faint.
func TestTokensInks(t *testing.T) {
	period, muted, subtle, faint := ink{token: theme.TextPrimary, bold: true}, ink{token: theme.TextMuted}, ink{token: theme.TextSubtle}, ink{token: theme.TextFaint}
	f := tokensFrame(160, 37, TokensByAccount)
	c := f.page(threeRouted(), now)
	top := f.top()
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "a head", x: 0, y: top, want: subtle},
		{name: "a count's head", x: 37, y: top, want: subtle},
		{name: "a period", x: 0, y: top + 2, want: period},
		{name: "a period's versions", x: 14, y: top + 2, want: muted},
		{name: "a period's count", x: 41, y: top + 2, want: period},
		{name: "a period's worth", x: 96, y: top + 2, want: period},
		{name: "a worth's unpriced part", x: 100, y: top + 2, want: faint},
		{name: "against at 1× or more", x: 106, y: top + 2, want: ink{token: theme.StatePositive, bold: true}},
		{name: "its price", x: 112, y: top + 2, want: subtle},
		{name: "against under 1×", x: 106, y: top + 11, want: ink{token: theme.AccentAttention, bold: true}},
		{name: "the branch", x: 0, y: top + 3, want: faint},
		{name: "an account", x: 2, y: top + 3, want: ink{token: theme.TextSecondary}},
		{name: "its versions", x: 14, y: top + 3, want: subtle},
		{name: "its count", x: 41, y: top + 3, want: muted},
		{name: "its worth", x: 96, y: top + 3, want: muted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.at(tt.x, tt.y).ink; got != tt.want {
				t.Errorf("%q at %d is in %+v, want %+v", c.at(tt.x, tt.y).glyph, tt.x, got, tt.want)
			}
		})
	}
}

// Every figure of the period still running brightens as a look changes it,
// each as the number it is, the period's own and each row it's split into,
// its figures alone, never its words: a count's noun, against its price's
// words, or the accounts a version ran on; a period over has none.
func TestTokensPeriodSoFarBrightensItsFigures(t *testing.T) {
	split := func(account, of string) Number { return Number{Kind: NumberTokensSplit, Account: account, Of: of} }
	tests := []struct {
		name  string
		split TokensSplit
		want  map[Number]string
	}{
		{
			name: "the period's own", split: TokensUnsplit,
			want: map[Number]string{
				{Kind: NumberTokens, Of: "models"}: "2", {Kind: NumberTokens, Of: "input"}: "4.2M", {Kind: NumberTokens, Of: "output"}: "2.9M",
				{Kind: NumberTokens, Of: "cache write"}: "38.0M", {Kind: NumberTokens, Of: "cache read"}: "615M", {Kind: NumberTokens, Of: "total"}: "660M",
				{Kind: NumberTokens, Of: "worth"}: "$635+", {Kind: NumberTokens, Of: "against"}: "4.6×$138",
			},
		},
		{
			name: "an account's", split: TokensByAccount,
			want: map[Number]string{split("side", "models"): "1", split("side", "total"): "211M", split("side", "worth"): "$239+"},
		},
		{
			name: "a version's, and the accounts it ran on", split: TokensByModel,
			want: map[Number]string{split("", "claude-opus-5-5 on"): "", split("", "claude-opus-5-5 input"): "2.4M", split("", "claude-opus-5-5 worth"): "$389+"},
		},
		{
			name: "an account's version's", split: TokensByAccountAndModel,
			want: map[Number]string{split("work", "claude-sonnet-5-5 total"): "30.4M", split("work", "claude-sonnet-5-5 worth"): "$34"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokensFrame(160, 37, tt.split).Numbers(threeRouted(), now)
			for n, want := range tt.want {
				if got[n] != want {
					t.Errorf("%+v reads %q, want %q", n, got[n], want)
				}
			}
		})
	}
	numbered := tokensFrame(160, 37, TokensByAccount)
	numbered.History.Tokens.Periods[0].SoFar = false
	for n := range numbered.Numbers(threeRouted(), now) {
		if n.Kind == NumberTokens || n.Kind == NumberTokensSplit {
			t.Errorf("%+v is numbered, want no period over numbered", n)
		}
	}
}

// Where the periods don't all fit, as many as fit whole show, the row after
// the last saying how many more are below; scrolled, a period at a time, the
// later ones show, and no further than the oldest shows.
func TestTokensScrollsAPeriodAtATime(t *testing.T) {
	f := tokensFrame(160, 37, TokensByAccount)
	page := pastTokens()
	for range 3 {
		page.Periods = append(page.Periods, page.Periods[:3]...)
	}
	f.History.Tokens = page // 12 periods of 4, 3 and 2 rows
	top := f.top()
	scrolling := f.tokensScrolling(top)
	tests := []struct {
		name   string
		scroll int
		first  string
		below  string
		page   int
	}{
		{name: "from the newest", scroll: 0, first: "28 Sep", below: "▼ 6 more weeks below   ·   j/k to scroll", page: 6},
		{name: "scrolled a period", scroll: 1, first: "21 Sep", below: "▼ 5 more weeks below   ·   j/k to scroll", page: 6},
		{name: "as far as it goes", scroll: 6, first: "28 Sep", page: 6},
		{name: "past it", scroll: 9, first: "28 Sep", page: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			rows := tokensRowsOf(f)
			if !strings.HasPrefix(rows[2], tt.first) {
				t.Errorf("the first period shown reads %q, want %q", rows[2], tt.first)
			}
			below := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "▼") })
			switch {
			case tt.below == "" && below >= 0:
				t.Errorf("row %d reads %q, want none saying more are below", below, rows[below])
			case tt.below != "" && (below < 0 || rows[below] != tt.below || rows[below-1] == ""):
				t.Errorf("Tokens reads\n%s\nwant %q under the last period shown", strings.Join(rows, "\n"), tt.below)
			}
			if s := scrolling(min(tt.scroll, 6)); s.Most != 6 || s.Page != tt.page {
				t.Errorf("Tokens scrolls %+v, want as far as 6, %d at once", s, tt.page)
			}
		})
	}
}

// A period taller than the view shows alone, cut short over the row saying
// how many more are below, and never over the footnote.
func TestAPeriodTallerThanTheViewIsCutShort(t *testing.T) {
	f := tokensFrame(160, 20, TokensByAccountAndModel)
	rows := tokensRowsOf(f)
	last := len(rows) - 1
	if !strings.HasPrefix(rows[last], "worth: ") || rows[last-1] != "" || rows[last-2] != "▼ 2 more weeks below   ·   j/k to scroll" {
		t.Errorf("Tokens reads\n%s\nwant the period cut short over ▼ and the footnote", strings.Join(rows, "\n"))
	}
}

// With nothing recorded, Tokens says so, in text.subtle, at the view's top.
func TestTokensOfNothingRecordedSaysSo(t *testing.T) {
	for _, page := range []*views.TokensPage{nil, {PricesAsOf: "2026-10-01"}} {
		f := tokensFrame(160, 37, TokensByAccount)
		f.History.Tokens = page
		c := f.page(threeRouted(), now)
		rows := c.rows(Look{})
		if got := rows[f.top()]; got != "nothing recorded yet" || c.at(0, f.top()).ink != dimInk {
			t.Errorf("Tokens of %+v reads %q in %+v, want nothing recorded yet, in text.subtle", page, got, c.at(0, f.top()).ink)
		}
		for i, row := range rows[f.top()+1 : f.top()+f.viewRows()] {
			if row != "" {
				t.Errorf("row %d of Tokens of %+v reads %q, want nothing under the words", i+1, page, row)
			}
		}
	}
}

// A count of tokens is in millions to one place under 100 million, then in
// whole millions, and from a billion, in billions to two places.
func TestTokenCounts(t *testing.T) {
	for n, want := range map[int]string{
		0: "0.0M", 12: "0.0M", 800_000: "0.8M", 4_249_999: "4.2M", 75_400_000: "75.4M", 99_949_999: "99.9M",
		99_950_000: "100M", 615_000_000: "615M", 999_499_999: "999M", 999_500_000: "1.00B", 1_056_800_000: "1.06B",
	} {
		if got := tokenCount(n); got != want {
			t.Errorf("%d tokens read %q, want %q", n, got, want)
		}
	}
}

// A row's worth, and in against its price the ratio and the price, are drawn
// whole or as an ellipsis, never cut part-way, as $12,3… would read as a
// smaller sum, while words may cut, unpriced among them: a phone's worth, at
// the frame's width; wide, a worth a cell short of against its price, and
// against its price at the narrowest the wide form is drawn, its price
// filling it, and a cell wider. Only a figure drawn whole is numbered, never
// an ellipsis or the words.
func TestTokensFiguresAreDrawnWholeOrAsAnEllipsis(t *testing.T) {
	unpriced := views.Worth{Unpriced: []string{"claude-opus-9"}}
	tests := []struct {
		name           string
		width          int
		worth          views.Worth
		want           string
		numbered       string
		againstNumbers string
	}{
		{name: "a phone's worth", width: 38, worth: worthIn(12345), want: "28 Sep                    660M  …"},
		{name: "a phone's worth, fitting", width: 39, worth: worthIn(12345), want: "28 Sep                    660M  $12,345", numbered: "$12,345"},
		{name: "a phone's worth, unpriced", width: 38, worth: unpriced, want: "28 Sep                    660M  unpri…"},
		{name: "a worth short of against its price", width: 160, worth: worthIn(1234567), want: "660M  …         14.6× its $12,345 week", againstNumbers: "14.6×$12,345"},
		{
			name: "against its price, its price filling it", width: 123, worth: worthIn(12345),
			want: "660M  $12,345   14.6× its $12,345", numbered: "$12,345", againstNumbers: "14.6×$12,345",
		},
		{
			name: "against its price, its words cut", width: 124, worth: worthIn(12345),
			want: "660M  $12,345   14.6× its $12,345…", numbered: "$12,345", againstNumbers: "14.6×$12,345",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tokensFrame(tt.width, 37, TokensUnsplit)
			f.History.Tokens.Periods[0].Worth, f.History.Tokens.Periods[0].Against = tt.worth, againstAt(14.6, 12345)
			if got := tokensRowsOf(f)[2]; !strings.HasSuffix(got, tt.want) {
				t.Errorf("the period reads %q, want it to end %q", got, tt.want)
			}
			numbers := f.Numbers(threeRouted(), now)
			if got := numbers[Number{Kind: NumberTokens, Of: "worth"}]; got != tt.numbered {
				t.Errorf("its worth is numbered %q, want %q", got, tt.numbered)
			}
			if got := numbers[Number{Kind: NumberTokens, Of: "against"}]; got != tt.againstNumbers {
				t.Errorf("against its price is numbered %q, want %q", got, tt.againstNumbers)
			}
		})
	}
}

// Against its price is the ratio as History writes it, then the price, to
// the dollar, its thousands set apart, the period named, the ratio and the
// price its figures; a faint dash where no plan is known.
func TestAgainstItsPriceAsTokensWritesIt(t *testing.T) {
	said := func(ratio span, price, unit string) []part {
		return []part{
			figurePart(line{ratio}, Number{}), wordsPart(line{{" its ", dimInk}}),
			figurePart(line{{price, dimInk}}, Number{}), wordsPart(line{{" " + unit, dimInk}}),
		}
	}
	dash := []part{figurePart(line{noFigure}, Number{})}
	tests := []struct {
		name string
		got  []part
		want []part
	}{
		{name: "no plan known", got: againstItsPrice(views.Against{}, "week"), want: dash},
		{name: "plans that cost nothing", got: againstItsPrice(views.Against{Cost: new(ledger.Picodollars)}, "month"), want: dash},
		{name: "a week's", got: againstItsPrice(againstAt(4.6, 138), "week"), want: said(span{"4.6×", boldPositiveInk}, "$138", "week")},
		{name: "a month's, short of it", got: againstItsPrice(againstAt(0.94, 600), "month"), want: said(span{"0.9×", alertInk}, "$600", "month")},
		{name: "a month's of a thousand or more", got: againstItsPrice(againstAt(1.2, 1800), "month"), want: said(span{"1.2×", boldPositiveInk}, "$1,800", "month")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("reads %+v, want %+v", tt.got, tt.want)
			}
		})
	}
}

// Narrower than a phone, Tokens cuts each column at the frame's width: a
// period's name and its counts are figures, drawn whole or as an ellipsis,
// numbered only where drawn whole, a count's ellipsis at the edge; the heads
// are words, cut; and the footnote's date, a figure, is never broken across
// its rows.
func TestTokensCutsEachColumnAtTheWidth(t *testing.T) {
	total := Number{Kind: NumberTokens, Of: "total"}
	tests := []struct {
		name     string
		width    int
		row      int
		want     string
		numbered string
	}{
		{name: "a count's head, cut", width: 28, row: 0, want: "week                     to…"},
		{name: "a count, an ellipsis", width: 28, row: 2, want: "28 Sep                     …"},
		{name: "a count, whole", width: 30, row: 2, want: "28 Sep                    660M", numbered: "660M"},
		{name: "a period's name, an ellipsis", width: 5, row: 2, want: "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tokensFrame(tt.width, 37, TokensUnsplit)
			if got := tokensRowsOf(f)[tt.row]; got != tt.want {
				t.Errorf("at %d columns, row %d reads %q, want %q", tt.width, tt.row, got, tt.want)
			}
			if got := f.Numbers(threeRouted(), now)[total]; got != tt.numbered {
				t.Errorf("at %d columns, its total is numbered %q, want %q", tt.width, got, tt.numbered)
			}
		})
	}
	rows := tokensRowsOf(tokensFrame(40, 37, TokensUnsplit))
	if got := rows[len(rows)-1]; got != "1 Oct 2026" {
		t.Errorf("at 40 columns, the footnote's last row reads %q, want the date whole, %q", got, "1 Oct 2026")
	}
}
