package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// threeConfigured are three accounts as a config gives them, the first
// labelled apart from its id.
var threeConfigured = config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal"}, {ID: "side"}}

// What the preferences keep of History's views reads back as it was kept:
// each by its name; the Year's all use as all, a version by its name and an
// account by its id; never a calendar year stepped back to.
func TestHistorysChoicesAreKeptByNameAndReadBack(t *testing.T) {
	tests := []struct {
		name string
		page HistoryPage
		kept theme.HistoryPrefs
	}{
		{
			name: "as History opens",
			kept: theme.HistoryPrefs{View: "year", Year: "all", Weeks: "table", Tokens: theme.TokensPrefs{Period: "week", Split: "account"}},
		},
		{
			name: "every choice made",
			page: HistoryPage{
				Shown: HistoryTokens, YearAsk: views.YearAsk{Version: "Opus 5.5"}, WeeksShape: WeeksColumns,
				TokensAsk: views.TokensAsk{Period: views.TokensByMonth}, TokensSplit: TokensByAccountAndModel,
			},
			kept: theme.HistoryPrefs{View: "tokens", Year: "Opus 5.5", Weeks: "columns", Tokens: theme.TokensPrefs{Period: "month", Split: "account+model"}},
		},
		{
			name: "an account, by its id",
			page: HistoryPage{Shown: HistoryDays, YearAsk: views.YearAsk{Account: "work"}, TokensAsk: views.TokensAsk{Period: views.TokensByDay}, TokensSplit: TokensUnsplit},
			kept: theme.HistoryPrefs{View: "days", Year: "work", Weeks: "table", Tokens: theme.TokensPrefs{Period: "day", Split: "none"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.page.Kept(); got != tt.kept {
				t.Errorf("Kept() = %+v, want %+v", got, tt.kept)
			}
			back := HistoryKept(tt.kept, threeConfigured)
			if back.Shown != tt.page.Shown || back.YearAsk != tt.page.YearAsk || back.WeeksShape != tt.page.WeeksShape ||
				back.TokensAsk != tt.page.TokensAsk || back.TokensSplit != tt.page.TokensSplit {
				t.Errorf("HistoryKept(%+v) = %+v, want %+v", tt.kept, back, tt.page)
			}
		})
	}
	stepped := HistoryPage{YearAsk: views.YearAsk{Year: 2025}}
	if got := stepped.Kept(); got.Year != "all" || got.View != "year" {
		t.Errorf("Kept() of a calendar year stepped back to = %+v, want all use, the calendar year left out", got)
	}
}

// Names the dashboard doesn't know, and none, as preferences kept before
// History's were, read as History opens; an account kept that's the one of
// one configured alone shows all use, as v doesn't cycle through it; and one
// no longer configured is taken for a version's name, for the watch to find
// none used, and show all use.
func TestHistorysChoicesKeptThatNameNothingReadAsItOpens(t *testing.T) {
	kept := theme.HistoryPrefs{View: "months", Weeks: "bars", Tokens: theme.TokensPrefs{Period: "year", Split: "session"}}
	for _, kept := range []theme.HistoryPrefs{kept, {}} {
		if got := HistoryKept(kept, threeConfigured); got.Shown != HistoryYear || got.YearAsk != (views.YearAsk{}) || got.WeeksShape != WeeksTable ||
			got.TokensAsk.Period != views.TokensByWeek || got.TokensSplit != TokensByAccount {
			t.Errorf("HistoryKept(%+v) = %+v, want History as it opens", kept, got)
		}
	}
	tests := []struct {
		accounts config.Accounts
		want     views.YearAsk
	}{
		{accounts: threeConfigured[:1], want: views.YearAsk{}},
		{accounts: threeConfigured[1:], want: views.YearAsk{Version: "work"}},
	}
	for _, tt := range tests {
		if got := HistoryKept(theme.HistoryPrefs{Year: "work"}, tt.accounts).YearAsk; got != tt.want {
			t.Errorf("HistoryKept() of work among %v shows %+v, want %+v", tt.accounts.IDs(), got, tt.want)
		}
	}
}

// [ and ] move between the sub-tabs, round; a click picks one, but the one
// shown.
func TestHistorysSubTabsTurnRound(t *testing.T) {
	p := HistoryPage{}
	for _, step := range []struct {
		by   int
		want HistoryView
	}{{-1, HistoryTokens}, {-1, HistoryDays}, {1, HistoryTokens}, {1, HistoryYear}, {1, HistoryWeeks}} {
		p, _ = p.Turned(step.by)
		if p.Shown != step.want {
			t.Errorf("turned %d to %d, want %d", step.by, p.Shown, step.want)
		}
	}
	if _, ok := p.Showing(int(HistoryWeeks)); ok {
		t.Error("Showing() the sub-tab shown reports a change, want none")
	}
	if got, ok := p.Showing(int(HistoryDays)); !ok || got.Shown != HistoryDays {
		t.Errorf("Showing(Days) = %d, %v; want Days shown", got.Shown, ok)
	}
	if _, ok := p.Showing(len(historyViews)); ok {
		t.Error("Showing() a sub-tab there isn't reports a change, want none")
	}
}

// < steps the Year from the last 12 months to this calendar year, then back
// a year at a time to the earliest, and > forward again to the last 12
// months; on Tokens, < steps to a shorter period and > to a longer, to a day
// and a month at the ends; and Weeks and Days have no period to step.
func TestHistorysPeriodsStep(t *testing.T) {
	year := HistoryPage{}
	var years []int
	for _, by := range []int{-1, -1, -1, -1, 1, 1, 1, 1} {
		var ok bool
		if year, ok = year.Stepped(by, 2024, 2026); ok {
			years = append(years, year.YearAsk.Year)
		}
	}
	if want := []int{2026, 2025, 2024, 2025, 2026, 0}; !slices.Equal(years, want) {
		t.Errorf("the Year stepped through %v, want %v", years, want)
	}

	tokens := HistoryPage{Shown: HistoryTokens}
	var periods []string
	for _, by := range []int{-1, -1, 1, 1, 1} {
		var ok bool
		if tokens, ok = tokens.Stepped(by, 2024, 2026); ok {
			periods = append(periods, tokens.Period())
		}
	}
	if want := []string{"by day", "by week", "by month"}; !slices.Equal(periods, want) {
		t.Errorf("Tokens stepped through %q, want %q", periods, want)
	}

	for _, shown := range []HistoryView{HistoryWeeks, HistoryDays} {
		if _, ok := (HistoryPage{Shown: shown}).Stepped(1, 2024, 2026); ok {
			t.Errorf("sub-tab %d steps a period, want none", shown)
		}
	}
}

// v cycles the Year from all use through each version its page lists, then
// each account, and back to all; Weeks between a table and columns; and
// Tokens' split; but on Days, or the Year before its page is built, nothing.
func TestVCyclesWhatEachSubTabShows(t *testing.T) {
	year := HistoryPage{Accounts: threeConfigured, Read: true, Year: &views.YearPage{Versions: []string{"Opus 5.5", "Sonnet 5"}, Accounts: []string{"work", "side"}}}
	var shown []string
	for range 6 {
		year, _ = year.Cycled()
		shown = append(shown, year.Split())
	}
	if want := []string{"Opus 5.5", "Sonnet 5", "Work", "side", "all", "Opus 5.5"}; !slices.Equal(shown, want) {
		t.Errorf("v cycled the Year through %q, want %q", shown, want)
	}

	weeks, tokens := HistoryPage{Shown: HistoryWeeks}, HistoryPage{Shown: HistoryTokens}
	var shapes, splits []string
	for range 4 {
		weeks, _ = weeks.Cycled()
		tokens, _ = tokens.Cycled()
		shapes, splits = append(shapes, weeks.Split()), append(splits, tokens.Split())
	}
	if want := []string{"columns", "table", "columns", "table"}; !slices.Equal(shapes, want) {
		t.Errorf("v cycled Weeks through %q, want %q", shapes, want)
	}
	if want := []string{"by model", "by account and model", "none", "by account"}; !slices.Equal(splits, want) {
		t.Errorf("v cycled Tokens through %q, want %q", splits, want)
	}

	for _, p := range []HistoryPage{{Shown: HistoryDays}, {}, {Year: &views.YearPage{}}} {
		if _, ok := p.Cycled(); ok {
			t.Errorf("v cycled %+v, want nothing", p)
		}
	}
}

// ← and → step the Year shown for one account between the accounts
// configured, round; and do nothing else.
func TestArrowsStepTheYearShownForOneAccount(t *testing.T) {
	p := HistoryPage{Accounts: threeConfigured, YearAsk: views.YearAsk{Account: "work", Year: 2025}}
	var stepped []string
	for _, by := range []int{-1, -1, 1, 1, 1} {
		p, _ = p.Moved(by)
		stepped = append(stepped, p.YearAsk.Account)
	}
	if want := []string{"side", "personal", "side", "work", "personal"}; !slices.Equal(stepped, want) || p.YearAsk.Year != 2025 {
		t.Errorf("the arrows stepped through %q, in %d, want %q, in 2025", stepped, p.YearAsk.Year, want)
	}
	for _, p := range []HistoryPage{
		{Accounts: threeConfigured},
		{Accounts: threeConfigured, YearAsk: views.YearAsk{Version: "Opus 5.5"}},
		{Accounts: threeConfigured, Shown: HistoryWeeks, YearAsk: views.YearAsk{Account: "work"}},
	} {
		if _, ok := p.Moved(1); ok {
			t.Errorf("→ moved %+v, want nothing", p)
		}
	}
}

// The key line names each sub-tab's period and what v has it show, as the
// heading names them: Days has neither, Weeks no period, and the Year no
// version till the last 12 months are read.
func TestHistorysPeriodsAndSplitsAsTheKeyLineNamesThem(t *testing.T) {
	tests := []struct {
		page          HistoryPage
		period, split string
	}{
		{page: HistoryPage{}, period: "last 12 months", split: "all"},
		{page: HistoryPage{Read: true, YearAsk: views.YearAsk{Year: 2025, Version: "Opus 5.5"}}, period: "2025", split: "Opus 5.5"},
		{page: HistoryPage{YearAsk: views.YearAsk{Version: "Opus 5.5"}}, period: "last 12 months", split: "all"},
		{page: HistoryPage{Shown: HistoryWeeks}, split: "table"},
		{page: HistoryPage{Shown: HistoryDays}},
		{page: HistoryPage{Shown: HistoryTokens, TokensSplit: TokensByModel}, period: "by week", split: "by model"},
	}
	for _, tt := range tests {
		if period, split := tt.page.Period(), tt.page.Split(); period != tt.period || split != tt.split {
			t.Errorf("sub-tab %d's period is %q and its split %q, want %q and %q", tt.page.Shown, period, split, tt.period, tt.split)
		}
	}
}

// v names what it has the sub-tab shown show only where that changes what
// the frame draws: on Weeks, where both shapes take their wide form, as the
// columns do from 126 columns, or before its page is built; never where one
// alone does, as the table does from 100 columns, which is drawn whichever v
// chose, nor on a phone, whose form is neither. Tokens' splits show on a
// phone too.
func TestVNamesWhatItShowsOnlyWhereItChangesWhatsDrawn(t *testing.T) {
	weeks := threeWeeks()
	tests := []struct {
		name  string
		page  HistoryPage
		width int
		want  string
	}{
		{name: "Weeks' table at 160", page: HistoryPage{Shown: HistoryWeeks, Weeks: &weeks}, width: 160, want: "table"},
		{name: "Weeks' columns at 126", page: HistoryPage{Shown: HistoryWeeks, WeeksShape: WeeksColumns, Weeks: &weeks}, width: 126, want: "columns"},
		{name: "Weeks' table at 125", page: HistoryPage{Shown: HistoryWeeks, Weeks: &weeks}, width: 125},
		{name: "Weeks' columns at 125", page: HistoryPage{Shown: HistoryWeeks, WeeksShape: WeeksColumns, Weeks: &weeks}, width: 125},
		{name: "Weeks' columns at 100", page: HistoryPage{Shown: HistoryWeeks, WeeksShape: WeeksColumns, Weeks: &weeks}, width: 100},
		{name: "Weeks on a phone of 99", page: HistoryPage{Shown: HistoryWeeks, Weeks: &weeks}, width: 99},
		{name: "Weeks' columns on a phone", page: HistoryPage{Shown: HistoryWeeks, WeeksShape: WeeksColumns, Weeks: &weeks}, width: 52},
		{name: "Weeks unbuilt at 160", page: HistoryPage{Shown: HistoryWeeks}, width: 160, want: "table"},
		{name: "Weeks unbuilt on a phone", page: HistoryPage{Shown: HistoryWeeks}, width: 52},
		{name: "Tokens on a phone", page: HistoryPage{Shown: HistoryTokens, TokensSplit: TokensByModel}, width: 52, want: "by model"},
	}
	for _, tt := range tests {
		f := frameOf(tt.width, 37)
		f.View, f.History = History, tt.page
		if got := f.Split(); got != tt.want {
			t.Errorf("%s, v names %q, want %q", tt.name, got, tt.want)
		}
	}
}

// Where Weeks' columns don't fit but its table does, from 100 columns to
// 125, the table is drawn whichever shape v chose, never the phone form,
// which the frame keeps for where neither fits.
func TestWeeksDrawsTheShapeThatFitsWhereVsDoesnt(t *testing.T) {
	for _, width := range []int{100, 125} {
		table := textOf(historyDrawn(weeksFrame(threeWeeks(), WeeksTable, width, 40, Look{})).rows(Look{}))
		columns := textOf(historyDrawn(weeksFrame(threeWeeks(), WeeksColumns, width, 40, Look{})).rows(Look{}))
		if !slices.Equal(columns, table) {
			t.Errorf("at %d columns, Weeks with its columns chosen reads\n%s\nwant its table, as with the table chosen\n%s", width, strings.Join(columns, "\n"), strings.Join(table, "\n"))
		}
	}
}
