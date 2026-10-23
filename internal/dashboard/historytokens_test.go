package dashboard

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
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

// knownOf is a, every account's, over the plans of known accounts of the
// accounts there are.
func knownOf(a views.Against, known, accounts int) views.Against {
	a.Plans = views.PlansKnown{Known: known, Of: accounts}
	return a
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
	return &views.TokensPage{PricesAsOf: "2026-10-01", PeriodCost: usd(138), Periods: []views.TokenPeriod{
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
// as history-tokens-160 lays them out; narrower, as its columns leave, one
// at a time.
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
				"week            models                 input      output cache write    cache read         total  worth     against its price",
				"",
				"28 Sep          2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
				"└ work          2 models                1.2M        0.8M       11.0M          178M          191M  $184",
				"  └ opus-5-5                            1.0M        0.6M        9.0M          150M          161M  $150",
				"  └ sonnet-5-5                          0.2M        0.2M        2.0M         28.0M         30.4M  $34",
				"└ personal      1 model                 1.6M        1.1M       14.2M          241M          258M  $212",
				"  └ sonnet-5-5                          1.6M        1.1M       14.2M          241M          258M  $212",
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
			name: "at 108 columns, models gone", width: 108, height: 37, split: TokensByAccount,
			want: []string{
				"week             input      output cache write    cache read         total  worth     against its price",
				"",
				"28 Sep            4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
				"└ work            1.2M        0.8M       11.0M          178M          191M  $184",
			},
		},
		{
			name: "at 73 columns, against its price and cache read beside total and worth", width: 73, height: 37, split: TokensByAccount,
			want: []string{
				"week            cache read         total  worth     against its price",
				"",
				"28 Sep                615M          660M  $635+     4.6× its $138 week",
				"└ work                178M          191M  $184",
			},
		},
		{
			name: "on a phone, by account", width: 52, height: 37, split: TokensByAccount,
			want: []string{
				"week            cache read         total  worth",
				"",
				"28 Sep                615M          660M  $635+",
				"└ work                178M          191M  $184",
				"└ personal            241M          258M  $212",
				"└ side                196M          211M  $239+",
				"",
				"21 Sep                985M         1.06B  $998",
				"└ work                910M          976M  $906",
				"└ side               75.4M         81.2M  $92",
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

// The footnote keeps the view's last rows while two are left for the periods
// over it, a period's first and the line saying what's below; where they
// wouldn't be, on a short terminal, the narrower the sooner as it wraps, it
// goes, rather than reach for the heads, and the periods take its rows. With
// a row alone for them, the row shows a period, not the line.
func TestTokensFootnoteGoesWhereItWouldLeaveThePeriodsNoRow(t *testing.T) {
	tests := []struct {
		width, height int
		noted         bool
	}{
		{width: 160, height: 14, noted: true},
		{width: 160, height: 13},
		{width: 52, height: 17, noted: true},
		{width: 52, height: 16},
		{width: 50, height: 15},
		{width: 40, height: 13},
		{width: 22, height: 15},
		{width: 160, height: 11},
	}
	for _, tt := range tests {
		f := tokensFrame(tt.width, tt.height, TokensByAccount)
		rows := tokensRowsOf(f)
		noted := slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, "worth: ") })
		if !strings.HasPrefix(rows[0], "week") || !strings.HasPrefix(rows[2], "28 Sep") || noted != tt.noted {
			t.Errorf("at %d×%d, Tokens reads\n%s\nwant its heads, then 28 Sep's row, the footnote shown: %t", tt.width, tt.height, strings.Join(rows, "\n"), tt.noted)
		}
	}
}

// Where a period doesn't fit whole in the rows its periods have, Tokens shows
// them a row at a time under its heads, over its footnote: the row after the
// page says how many rows are out of view, above and below, so a period cut
// short is never counted as shown; a period's row, or an account's, never
// ends a page without the first row under it; and scrolled to the end, the
// last row shows. Where every period fits, they scroll a period at a time.
func TestTokensScrollsARowAtATimeWhereAPeriodDoesntFit(t *testing.T) {
	f := tokensFrame(160, 20, TokensByAccountAndModel)
	tests := []struct {
		name   string
		scroll int
		want   Scrolling
		shown  []string
		says   string
	}{
		{
			name: "at the top, side left for the next page with its first version", want: Scrolling{Most: 11, Page: 6},
			shown: []string{"28 Sep", "└ work", "  └ opus-5-5", "  └ sonnet-5-5", "└ personal", "  └ sonnet-5-5", ""},
			says:  "▼ 12 more below   ·   j/k to scroll",
		},
		{
			name: "a row down", scroll: 1, want: Scrolling{Most: 11, Page: 7},
			shown: []string{"└ work", "  └ opus-5-5", "  └ sonnet-5-5", "└ personal", "  └ sonnet-5-5", "└ side", "  └ opus-5-5"},
			says:  "▲ 1 more above   ·   ▼ 10 more below   ·   j/k to scroll",
		},
		{
			name: "as far as it goes, the last row shown, a page up going 6 to show the row over it", scroll: 11, want: Scrolling{Most: 11, Page: 7, Back: 6},
			shown: []string{"  └ opus-5-5", "└ side", "  └ opus-5-5", "", "14 Sep", "└ work", "  └ opus-5-5"},
			says:  "▲ 11 more above   ·   j/k to scroll",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			if got := f.Scrolling(threeRouted(), now); got != tt.want || !f.ScrollsRows(threeRouted(), now) {
				t.Errorf("Tokens scrolls %+v, a row at a time: %t; want %+v, a row at a time", got, f.ScrollsRows(threeRouted(), now), tt.want)
			}
			rows := tokensRowsOf(f)
			last := len(rows) - 1
			if !strings.HasPrefix(rows[0], "week ") || !strings.HasPrefix(rows[last], "worth: ") || rows[last-1] != "" {
				t.Errorf("Tokens reads\n%s\nwant its heads held at the top, and the footnote at the foot, a blank row over it", strings.Join(rows, "\n"))
			}
			for i, prefix := range tt.shown {
				if row := rows[2+i]; prefix == "" && row != "" || !strings.HasPrefix(row, prefix) {
					t.Errorf("the page's row %d reads %q, want it to start %q", i, row, prefix)
				}
			}
			if says := rows[2+len(tt.shown)]; says != tt.says {
				t.Errorf("the row after the page reads %q, want %q", says, tt.says)
			}
		})
	}
	if tall := tokensFrame(160, 37, TokensByAccountAndModel); tall.ScrollsRows(threeRouted(), now) {
		t.Error("as tall as the frames, where every period fits, Tokens scrolls a row at a time, want a period at a time")
	}
}

// Of a short period before a tall one, 14 Sep's then 28 Sep's of seven
// accounts and seven versions, by account and by model, from 14 rows, where
// the footnote shows, to 37: Tokens never shows fewer of its periods' rows a
// row taller, going on a row at a time past where each period fits while a
// period at a time would show fewer.
func TestTokensNeverShowsFewerRowsAsTheTerminalGrows(t *testing.T) {
	page := pastTokens()
	short, tall := page.Periods[2], page.Periods[0]
	tall.ByAccount = slices.Repeat(tall.ByAccount, 3)[:7]
	tall.ByModel = slices.Repeat(tall.ByModel, 4)[:7]
	page.Periods = []views.TokenPeriod{short, tall}
	for _, tt := range []struct {
		name  string
		split TokensSplit
	}{
		{name: "by account", split: TokensByAccount},
		{name: "by model", split: TokensByModel},
	} {
		t.Run(tt.name, func(t *testing.T) {
			was := 0
			for height := 14; height <= 37; height++ {
				f := tokensFrame(160, height, tt.split)
				f.History.Tokens = page
				if shown := periodRowsShown(tokensRowsOf(f)); shown < was {
					t.Errorf("at %d rows, Tokens shows %d of its periods' rows, fewer than the %d a row shorter", height, shown, was)
				} else {
					was = shown
				}
			}
		})
	}
}

// periodRowsShown are how many of Tokens' rows, under its heads, show a
// period's: neither blank, nor saying what's out of view, nor the footnote.
func periodRowsShown(rows []string) int {
	n := 0
	for _, row := range rows[min(2, len(rows)):] {
		if row != "" && !strings.HasPrefix(row, "worth: ") && !strings.Contains(row, "more below") && !strings.Contains(row, "more above") {
			n++
		}
	}
	return n
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

// A row's worth, and in against its price the ratio and the price, are drawn
// whole or as an ellipsis, never cut part-way, as $12,3… would read as a
// smaller sum, while words may cut, unpriced among them: a worth, to the
// frame's width where against its price has left; else a worth a cell short
// of against its price, and against its price, which ends at the frame's
// width, its words filling it, and a cell narrower, gone. Only a figure drawn
// whole is numbered, never
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
		{name: "a worth to the frame's width", width: 27, worth: worthIn(12345), want: "28 Sep         660M  …"},
		{name: "a worth to the frame's width, fitting", width: 28, worth: worthIn(12345), want: "28 Sep         660M  $12,345", numbered: "$12,345"},
		{name: "a worth to the frame's width, unpriced", width: 27, worth: unpriced, want: "28 Sep         660M  unpri…"},
		{name: "a worth short of against its price", width: 160, worth: worthIn(1234567), want: "660M  …         14.6× its $12,345 week", againstNumbers: "14.6×$12,345"},
		{
			name: "against its price, its words filling it", width: 74, worth: worthIn(12345),
			want: "660M  $12,345   14.6× its $12,345 week", numbered: "$12,345", againstNumbers: "14.6×$12,345",
		},
		{
			name: "against its price gone a cell narrower, never cut", width: 73, worth: worthIn(12345),
			want: "660M  $12,345", numbered: "$12,345",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tokensFrame(tt.width, 37, TokensUnsplit)
			f.History.Tokens.PeriodCost = usd(12345)
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
		{name: "no plan known", got: againstItsPrice(views.Against{}, "week", 54), want: dash},
		{name: "plans that cost nothing", got: againstItsPrice(views.Against{Cost: new(ledger.Picodollars)}, "month", 54), want: dash},
		{name: "a week's", got: againstItsPrice(againstAt(4.6, 138), "week", 54), want: said(span{"4.6×", boldPositiveInk}, "$138", "week")},
		{name: "a month's, short of it", got: againstItsPrice(againstAt(0.94, 600), "month", 54), want: said(span{"0.9×", alertInk}, "$600", "month")},
		{name: "a month's of a thousand or more", got: againstItsPrice(againstAt(1.2, 1800), "month", 54), want: said(span{"1.2×", boldPositiveInk}, "$1,800", "month")},
		{name: "every account's, over the plans known", got: againstItsPrice(knownOf(againstAt(4.4, 92), 2, 3), "week", 54),
			want: append(said(span{"4.4×", boldPositiveInk}, "$92", "week"), wordsIn(" · 2 of 3 plans known", dimInk))},
		{name: "every account's, over the plans known, short where that doesn't fit", got: againstItsPrice(knownOf(againstAt(4.4, 92), 2, 3), "week", 37),
			want: append(said(span{"4.4×", boldPositiveInk}, "$92", "week"), wordsIn(" · 2 of 3 plans", dimInk))},
		{name: "every account's, over the plans known, compact where neither fits", got: againstItsPrice(knownOf(againstAt(4.4, 92), 2, 3), "week", 30),
			want: append(said(span{"4.4×", boldPositiveInk}, "$92", "week"), wordsIn(" · 2/3", dimInk))},
		{name: "every account's, over the plans known, the ratio alone where none fits", got: againstItsPrice(knownOf(againstAt(4.4, 92), 2, 3), "week", 22),
			want: said(span{"4.4×", boldPositiveInk}, "$92", "week")},
		{name: "every account's, every plan known", got: againstItsPrice(knownOf(againstAt(4.4, 92), 3, 3), "week", 54),
			want: said(span{"4.4×", boldPositiveInk}, "$92", "week")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("reads %+v, want %+v", tt.got, tt.want)
			}
		})
	}
}

// Narrower than the parts that stay at their leasts, Tokens cuts each column
// at the frame's width: a period's name and its counts are figures, drawn
// whole or as an ellipsis, numbered only where drawn whole, a count's
// ellipsis at the edge; the heads are words, cut; and the footnote's date, a
// figure, is never broken across its rows.
func TestTokensCutsEachColumnAtTheWidth(t *testing.T) {
	total := Number{Kind: NumberTokens, Of: "total"}
	tests := []struct {
		name     string
		width    int
		row      int
		want     string
		numbered string
	}{
		{name: "a count's head, cut", width: 17, row: 0, want: "week          to…"},
		{name: "a count, an ellipsis", width: 17, row: 2, want: "28 Sep          …"},
		{name: "a count, whole", width: 19, row: 2, want: "28 Sep         660M", numbered: "660M"},
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

// tokensLeaving are the parts of Tokens that leave, in the order they leave
// as the frame narrows.
var tokensLeaving = []int{tokensOf, tokensCacheWrite, tokensInput, tokensOutput, tokensAgainst, tokensCacheRead}

// tokensSplits are every way Tokens splits its periods.
var tokensSplits = []TokensSplit{TokensByAccount, TokensByModel, TokensByAccountAndModel, TokensUnsplit}

// longTokensAccounts are accounts configured with long labels: 14, 17 and 22
// cells.
var longTokensAccounts = config.Accounts{
	{ID: "work", Label: "work-team-plan"}, {ID: "personal", Label: "personal-research"}, {ID: "side", Label: "side-hustle-consulting"},
}

// tokensLeft is how many of tokensLeaving have left at the columns, and
// whether they left in that order, each only after those before it.
func tokensLeft(cols tokensColumns) (int, bool) {
	left := 0
	for _, part := range tokensLeaving {
		if cols.shows(part) {
			break
		}
		left++
	}
	for _, part := range tokensLeaving[left:] {
		if !cols.shows(part) {
			return left, false
		}
	}
	return left, true
}

// Tokens' columns leave one at a time as the frame narrows, never all at
// once: models first, then cache write, input, output, against its price and
// cache read, total and worth staying. The names hold their natural while any
// of those shows, and narrow only after: a wider frame never shows fewer
// columns, nor a shorter name. So it holds from 220 columns to 40, of short
// names and long, in every split.
func TestTokensColumnsLeaveOneAtATime(t *testing.T) {
	for _, split := range tokensSplits {
		for names, accounts := range map[string]config.Accounts{"short": nil, "long": longTokensAccounts} {
			f := tokensFrame(220, 37, split)
			f.History.Accounts = accounts
			natural := f.tokensNamesNatural()
			wasLeft, wasName := 0, natural
			for width := 220; width >= 40; width-- {
				f.Width = width
				cols := f.tokensColumns()
				left, inOrder := tokensLeft(cols)
				name := cols.spans[tokensName].Size
				switch {
				case !inOrder:
					t.Fatalf("%s names, split %d, at %d columns, the columns left out of order: %+v", names, split, width, cols.spans)
				case left > wasLeft+1:
					t.Errorf("%s names, split %d, at %d columns, %d columns left at once", names, split, width, left-wasLeft)
				case name > wasName:
					t.Errorf("%s names, split %d, at %d columns, the names take %d cells, more than %d a column wider", names, split, width, name, wasName)
				case left < len(tokensLeaving) && name != natural:
					t.Errorf("%s names, split %d, at %d columns, the names take %d cells with a detail column showing, want their natural %d", names, split, width, name, natural)
				case !cols.shows(tokensTotal) || !cols.shows(tokensWorth):
					t.Errorf("%s names, split %d, at %d columns, total or worth has left", names, split, width)
				}
				wasLeft, wasName = left, name
			}
		}
	}
}

// Tokens' columns at the owner's widths, of the labels configured: every
// column at 160; models gone at 120 and 108, of short names, and cache write
// too at 108 of long; at 73, cache read and against its price beside total
// and worth, of short names, and cache read alone of long; and at 52, cache
// read alone.
func TestTokensShowsTheColumnsThatFit(t *testing.T) {
	tests := []struct {
		width    int
		accounts config.Accounts
		left     int
	}{
		{width: 160, left: 0},
		{width: 160, accounts: longTokensAccounts, left: 0},
		{width: 120, left: 1},
		{width: 108, left: 1},
		{width: 108, accounts: longTokensAccounts, left: 2},
		{width: 73, left: 4},
		{width: 73, accounts: longTokensAccounts, left: 5},
		{width: 52, left: 5},
	}
	for _, tt := range tests {
		f := tokensFrame(tt.width, 37, TokensByAccount)
		f.History.Accounts = tt.accounts
		if left, _ := tokensLeft(f.tokensColumns()); left != tt.left {
			t.Errorf("at %d columns, of %d labels configured, %d columns have left, want %d", tt.width, len(tt.accounts), left, tt.left)
		}
	}
}

// The names' natural is of what the split names and what's configured, never
// of the rows shown, so the columns never move with the data: at least
// tokensNamesCells, which holds the periods' names and the versions'; by
// account, the widest label configured, else its id, after its branch, at
// labelMost at the most; by model, or unsplit, no account's, as no row names
// one; and by account and model, both, a version's nested id among them. A
// label the rows carry that isn't configured changes nothing.
func TestTokensNamesNaturalIsOfWhatTheSplitNames(t *testing.T) {
	tests := []struct {
		name     string
		split    TokensSplit
		accounts config.Accounts
		want     int
	}{
		{name: "none configured", split: TokensByAccount, want: tokensNamesCells},
		{name: "short labels", split: TokensByAccount, accounts: config.Accounts{{ID: "work"}, {ID: "personal"}}, want: tokensNamesCells},
		{name: "a long label", split: TokensByAccount, accounts: longTokensAccounts, want: 24},
		{name: "an id where there's no label", split: TokensByAccount, accounts: config.Accounts{{ID: "the-research-account"}}, want: 22},
		{name: "past labelMost", split: TokensByAccount, accounts: config.Accounts{{ID: "work", Label: strings.Repeat("w", 40)}}, want: 2 + labelMost},
		{name: "by model, of long labels", split: TokensByModel, accounts: longTokensAccounts, want: tokensNamesCells},
		{name: "unsplit, of long labels", split: TokensUnsplit, accounts: longTokensAccounts, want: tokensNamesCells},
		{name: "nested versions", split: TokensByAccountAndModel, want: tokensNamesCells + 2},
		{name: "by account and model, of long labels", split: TokensByAccountAndModel, accounts: longTokensAccounts, want: 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tokensFrame(160, 37, tt.split)
			f.History.Accounts = tt.accounts
			if got := f.tokensColumns().spans[tokensName].Size; got != tt.want {
				t.Errorf("the names take %d cells, want %d", got, tt.want)
			}
			f.History.Tokens.Periods[0].ByAccount[0].Label = strings.Repeat("x", 30)
			if got := f.tokensColumns().spans[tokensName].Size; got != tt.want {
				t.Errorf("of a label the rows carry, the names take %d cells, want %d still", got, tt.want)
			}
		})
	}
}

// A label configured long reads whole in its column, at every width the
// owner uses, as the names come before the detail columns.
func TestTokensDrawsALongLabelWhole(t *testing.T) {
	for _, width := range []int{73, 108, 109, 138, 173} {
		f := tokensFrame(width, 37, TokensByAccount)
		f.History.Accounts = longTokensAccounts
		f.History.Tokens.Periods[0].ByAccount[2].Label = "side-hustle-consulting"
		rows := tokensRowsOf(f)
		if !slices.ContainsFunc(rows, func(row string) bool { return strings.HasPrefix(row, "└ side-hustle-consulting  ") }) {
			t.Errorf("at %d columns, Tokens reads\n%s\nwant └ side-hustle-consulting whole", width, strings.Join(rows, "\n"))
		}
	}
}

// Split by model, the version's accounts read whole where there's room: on
// takes its natural at 200 columns, every account configured a space apart,
// before against its price grows; and narrows to its least, 18, before any
// column leaves.
func TestTokensOnTakesTheAccountsConfigured(t *testing.T) {
	accounts := append(slices.Clone(longTokensAccounts), config.Account{ID: "lab"})
	f := tokensFrame(200, 37, TokensByModel)
	f.History.Accounts = accounts
	period := &f.History.Tokens.Periods[0]
	period.ByModel[0].On = []string{"work", "personal", "side", "lab"}
	for i, a := range accounts[:3] {
		period.ByAccount[i].Account, period.ByAccount[i].Label = a.ID, a.Label
	}
	const on = "work-team-plan personal-research side-hustle-consulting lab"
	rows := tokensRowsOf(f)
	if !strings.HasPrefix(rows[3], "└ opus-5-5    "+on+"  ") {
		t.Errorf("at 200 columns, the version reads %q, want it on %q whole", rows[3], on)
	}
	if got := f.tokensColumns().spans[tokensOf].Size; got != len(on) {
		t.Errorf("on takes %d cells, want its natural %d", got, len(on))
	}
	for width := 200; width >= 100; width-- {
		f.Width = width
		cols := f.tokensColumns()
		if left, _ := tokensLeft(cols); left > 0 && cols.spans[tokensOf].Size > tokensOfCells {
			t.Errorf("at %d columns, a column left with on at %d cells, past its least", width, cols.spans[tokensOf].Size)
		}
	}
}

// Where every count but total has left, total sits a cell after the names,
// the cells cache read kept between them leaving with it, so the names have
// all but total's and worth's: from 30 columns to 60, of short names and
// long, they take their natural or every cell but those.
func TestTokensTotalSitsACellAfterTheNamesAlone(t *testing.T) {
	beside := tokensTotalGap + 10 + tokensWorthGap + tokensWorthCells
	for _, accounts := range []config.Accounts{nil, longTokensAccounts} {
		f := tokensFrame(60, 37, TokensByAccount)
		f.History.Accounts = accounts
		natural := f.tokensNamesNatural()
		for width := 30; width <= 60; width++ {
			f.Width = width
			cols := f.tokensColumns()
			if cols.shows(tokensCacheRead) {
				continue
			}
			if got, want := cols.spans[tokensName].Size, min(natural, width-beside); got != want {
				t.Errorf("of %d labels configured, at %d columns, the names take %d cells, want %d", len(accounts), width, got, want)
			}
		}
	}
}

// Against its price is never cut: its least is its words at their widest
// for what the plans configured cost over a whole period, the ratio at
// 99.9×, so where they don't fit, it leaves. From 220 columns to 40, by week
// and by month, each period's reads whole, its unit last, or not at all; and
// at the narrowest frame it shows in, the widest it's reckoned at would end
// at the frame's edge.
func TestTokensAgainstItsPriceIsNeverCut(t *testing.T) {
	tests := []struct {
		period views.TokensPeriod
		cost   float64
		widest string
	}{
		{period: views.TokensByWeek, cost: 138, widest: "99.9× its $138 week"},
		{period: views.TokensByMonth, cost: 600, widest: "99.9× its $600 month"},
	}
	for _, tt := range tests {
		f := tokensFrame(220, 37, TokensUnsplit)
		f.History.TokensAsk.Period = tt.period
		f.History.Tokens.PeriodCost = usd(tt.cost)
		for i := range f.History.Tokens.Periods {
			f.History.Tokens.Periods[i].Against = againstAt(*f.History.Tokens.Periods[i].Ratio, tt.cost)
		}
		narrowest := 0
		for width := 220; width >= 40; width-- {
			f.Width = width
			for _, row := range tokensRowsOf(f) {
				if strings.Contains(row, "× its") && !strings.HasSuffix(row, " "+tt.period.Name()) {
					t.Errorf("by %s at %d columns, a period reads %q, want against its price whole", tt.period.Name(), width, row)
				}
			}
			if f.tokensColumns().shows(tokensAgainst) {
				narrowest = width
			}
		}
		f.Width = narrowest
		if at := f.tokensColumns().at(tokensAgainst); at+ansi.StringWidth(tt.widest) != narrowest {
			t.Errorf("by %s, against its price shows from %d columns, at %d, want it where %q ends at the edge", tt.period.Name(), narrowest, at, tt.widest)
		}
	}
}

// A name in on is at labelMost cells at the most, as a column of names has
// it, so at on's natural every name reads as the natural reckons it: a label
// past labelMost cut to it with an ellipsis, the names after it whole.
func TestTokensOnCutsEachNameAtLabelMost(t *testing.T) {
	long := strings.Repeat("r", 30)
	f := tokensFrame(220, 37, TokensByModel)
	f.History.Accounts = config.Accounts{{ID: "work", Label: long}, {ID: "side"}}
	period := &f.History.Tokens.Periods[0]
	period.ByAccount[0].Label = long
	period.ByModel[0].On = []string{"work", "side"}
	on := strings.Repeat("r", labelMost-1) + "… side"
	if got := f.tokensColumns().spans[tokensOf].Size; got != ansi.StringWidth(on) {
		t.Errorf("on takes %d cells, want its natural %d", got, ansi.StringWidth(on))
	}
	if row := tokensRowsOf(f)[3]; !strings.HasPrefix(row, "└ opus-5-5    "+on+" ") {
		t.Errorf("the version reads %q, want it on %q", row, on)
	}
}

// Where the split's column doesn't name accounts, by account, by account and
// model, or unsplit, many accounts configured never widen it: it's
// tokensOfCells at every width it shows.
func TestTokensOfIsItsLeastWhereItNamesNoAccounts(t *testing.T) {
	for _, split := range []TokensSplit{TokensByAccount, TokensByAccountAndModel, TokensUnsplit} {
		f := tokensFrame(220, 37, split)
		f.History.Accounts = tenConfigured()
		if got := f.tokensColumns().spans[tokensOf].Size; got != tokensOfCells {
			t.Errorf("split %d, of ten accounts, at 220 columns the split's column takes %d cells, want %d", split, got, tokensOfCells)
		}
	}
}

// A ratio of 100× or more, as a day's worth often is against what its plans
// cost a day, is written whole, so against its price still fits its least
// and is never cut: by day, unsplit and by model on ten accounts, from 220
// columns to 40, the newest day's reads whole, or not at all.
// Against its price's least is reckoned at tokensRatioMost, so no ratio short
// of 10000× is written wider than it.
func TestTokensRatioMostIsTheWidestARatioIsWritten(t *testing.T) {
	most := ansi.StringWidth(prose.Ratio(tokensRatioMost))
	for tenths := range 99995 {
		x := float64(tenths) / 10
		if got := ansi.StringWidth(prose.Ratio(x)); got > most {
			t.Fatalf("a ratio of %v reads %q, %d cells, wider than tokensRatioMost's %d", x, prose.Ratio(x), got, most)
		}
	}
}

func TestTokensAgainstItsPriceOfAHundredfoldIsNeverCut(t *testing.T) {
	for _, split := range []TokensSplit{TokensUnsplit, TokensByModel} {
		for ratio, said := range map[float64]string{123.4: "123× its $7 day", 1234.5: "1234× its $7 day"} {
			f := tokensFrame(220, 37, split)
			f.History.TokensAsk.Period = views.TokensByDay
			f.History.Accounts = tenConfigured()
			f.History.Tokens.PeriodCost = usd(7)
			period := &f.History.Tokens.Periods[0]
			period.Against = againstAt(ratio, 7)
			period.ByModel[0].On = f.History.Accounts.IDs()
			shown := 0
			for width := 220; width >= 40; width-- {
				f.Width = width
				row := tokensRowsOf(f)[2]
				switch {
				case strings.HasSuffix(row, said):
					shown++
				case strings.Contains(row, "×"):
					t.Errorf("split %d at %d columns, a ratio of %v reads %q, want it to end %q whole", split, width, ratio, row, said)
				}
			}
			if shown == 0 {
				t.Errorf("split %d, a ratio of %v never shows %q", split, ratio, said)
			}
		}
	}
}

// Nothing Tokens draws is left to the canvas to clip: in every split, at
// any width, scrolled, it draws nothing past the frame's width.
func TestTokensDrawsNothingPastTheWidth(t *testing.T) {
	for _, split := range tokensSplits {
		for width := 1; width <= 160; width++ {
			f := tokensFrame(width, 40, split)
			f.Scroll = 2
			historyPastItsEdge(t, fmt.Sprintf("Tokens split %d", split), f, f.Width)
		}
	}
}
