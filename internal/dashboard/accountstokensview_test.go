package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// periodTokens is Accounts' Tokens' page over this week, as the views give
// it: its total, the week's own figures, split as pastTokens' week is, then
// three days of it, newest first, today's so far, each set against a day's
// $20.
func periodTokens() *views.TokensPage {
	past := pastTokens()
	total := past.Periods[0]
	days := slices.Clone(past.Periods)
	for i, first := range []string{"2026-10-01", "2026-09-30", "2026-09-29"} {
		days[i].First, days[i].Against = first, againstAt(float64(i+2), 20)
	}
	return &views.TokensPage{PricesAsOf: past.PricesAsOf, PeriodCost: usd(20), Periods: days, Total: &total}
}

// accountsTokensFrame is a frame the size given showing Accounts' Tokens of
// the page given over the period given, split as given.
func accountsTokensFrame(width, height int, period views.Period, split TokensSplit, page *views.TokensPage) Frame {
	f := frameOf(width, height)
	f.View, f.Period = Accounts, period
	f.Accounts = AccountsShown{Shown: AccountsTokens, Accounts: threeConfigured, TokensSplit: split, Tokens: page}
	return f
}

// accountsTokensRows are the rows of Accounts' Tokens the frame draws, from
// its heads to the view's last row.
func accountsTokensRows(f Frame) []string {
	rows := f.Draw(threeRouted(), now)
	return rows[f.top() : f.top()+f.viewRows()]
}

// The period's total leads, split as its rows are, named for the dial's
// period in a row's head's ink and set against the period's price; a rule in
// border.subtle runs under it as far as the heads do, and a blank row under
// that, then the rows, a day each, newest first, each against a day's price,
// as accounts-tokens-173x53 draws them.
func TestAccountsTokensLeadsWithThePeriodsTotalRuledOff(t *testing.T) {
	const heads = "day           models                 input      output cache write    cache read         total  worth     against its price"
	f := accountsTokensFrame(160, 37, views.PeriodThisWeek, TokensByAccount, periodTokens())
	rows := accountsTokensRows(f)
	want := []string{
		heads,
		"",
		"this week     2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 week",
		"└ work        2 models                1.2M        0.8M       11.0M          178M          191M  $184",
		"└ personal    1 model                 1.6M        1.1M       14.2M          241M          258M  $212",
		"└ side        1 model                 1.4M        1.0M       12.8M          196M          211M  $239+",
		rule(len(heads)),
		"",
		"Thu 1 Oct     2 models                4.2M        2.9M       38.0M          615M          660M  $635+     2.0× its $20 day",
		"└ work        2 models                1.2M        0.8M       11.0M          178M          191M  $184",
	}
	if got := rows[:len(want)]; !slices.Equal(got, want) {
		t.Fatalf("Tokens reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	c := f.page(threeRouted(), now)
	top := f.top()
	if got := c.at(0, top+2).ink; got != titleInk {
		t.Errorf("the total's head is in %+v, want a row's head's %+v", got, titleInk)
	}
	for _, x := range []int{0, len(heads) - 1} {
		if got := c.at(x, top+6).ink; got != borderInk {
			t.Errorf("the rule at column %d is in %+v, want border.subtle", x, got)
		}
	}
}

// The rule runs as far as the heads do, whatever shows: to against its
// price's head, or where that's left, to worth's.
func TestAccountsTokensRuleRunsAsFarAsTheHeads(t *testing.T) {
	for _, tt := range []struct {
		width int
		heads string
	}{
		{width: 73, heads: "day             cache read         total  worth     against its price"},
		{width: 52, heads: "day             cache read         total  worth"},
	} {
		rows := accountsTokensRows(accountsTokensFrame(tt.width, 37, views.PeriodThisWeek, TokensByAccount, periodTokens()))
		if rows[0] != tt.heads || rows[6] != rule(len(tt.heads)) {
			t.Errorf("at %d columns, the heads read %q and the rule %q, want the rule as long as %q", tt.width, rows[0], rows[6], tt.heads)
		}
	}
}

// Over today, the period is its total alone: no rule, and no row repeating
// it.
func TestAccountsTokensTodayIsItsTotalAlone(t *testing.T) {
	page := periodTokens()
	page.Periods = nil
	rows := accountsTokensRows(accountsTokensFrame(160, 37, views.PeriodToday, TokensByAccount, page))
	want := []string{
		"",
		"today         2 models                4.2M        2.9M       38.0M          615M          660M  $635+     4.6× its $138 day",
		"└ work        2 models                1.2M        0.8M       11.0M          178M          191M  $184",
		"└ personal    1 model                 1.6M        1.1M       14.2M          241M          258M  $212",
		"└ side        1 model                 1.4M        1.0M       12.8M          196M          211M  $239+",
		"",
		"",
	}
	if got := rows[1 : 1+len(want)]; !slices.Equal(got, want) {
		t.Errorf("Tokens over today reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Against its price names the unit of what it's set against: a row's, a
// day, a week or a month, as the period's rows are; the total's, the
// period's, as Compare names it.
func TestAccountsTokensAgainstItsPriceNamesEachUnit(t *testing.T) {
	tests := []struct {
		period           views.Period
		total, firstRow  string
		head, firstNamed string
	}{
		{period: views.PeriodThisWeek, head: "day", total: "its $138 week", firstNamed: "Thu 1 Oct", firstRow: "its $20 day"},
		{period: views.PeriodThirtyDays, head: "day", total: "its $138 month", firstNamed: "Thu 1 Oct", firstRow: "its $20 day"},
		{period: views.PeriodTwelveWeeks, head: "week", total: "its $138 12 weeks", firstNamed: "1 Oct", firstRow: "its $20 week"},
		{period: views.PeriodTwelveMonths, head: "month", total: "its $138 12 months", firstNamed: "Oct 2026", firstRow: "its $20 month"},
	}
	for _, tt := range tests {
		t.Run(tt.period.Name(), func(t *testing.T) {
			page := periodTokens()
			if tt.period == views.PeriodTwelveMonths {
				page.Periods[0].First = "2026-10"
			}
			rows := accountsTokensRows(accountsTokensFrame(160, 37, tt.period, TokensUnsplit, page))
			if !strings.HasPrefix(rows[0], tt.head+" ") {
				t.Errorf("the heads read %q, want them to lead with %q", rows[0], tt.head)
			}
			if !strings.HasPrefix(rows[2], tt.period.Name()+" ") || !strings.HasSuffix(rows[2], tt.total) {
				t.Errorf("the total reads %q, want it named %q and against %q", rows[2], tt.period.Name(), tt.total)
			}
			if !strings.HasPrefix(rows[5], tt.firstNamed+" ") || !strings.HasSuffix(rows[5], tt.firstRow) {
				t.Errorf("the first row reads %q, want it named %q and against %q", rows[5], tt.firstNamed, tt.firstRow)
			}
		})
	}
}

// v splits each row, the total's too, by account, by model, by both, or not
// at all, as History's Tokens does.
func TestAccountsTokensSplitsEachRowAsVHasIt(t *testing.T) {
	tests := []struct {
		split TokensSplit
		total []string
	}{
		{split: TokensByAccount, total: []string{"this week", "└ work", "└ personal", "└ side"}},
		{split: TokensByModel, total: []string{"this week", "└ opus-5-5", "└ sonnet-5-5"}},
		{split: TokensByAccountAndModel, total: []string{"this week", "└ work", "  └ opus-5-5", "  └ sonnet-5-5", "└ personal", "  └ sonnet-5-5", "└ side", "  └ opus-5-5"}},
		{split: TokensUnsplit, total: []string{"this week"}},
	}
	for _, tt := range tests {
		t.Run(tokensSplitWords[tt.split], func(t *testing.T) {
			rows := accountsTokensRows(accountsTokensFrame(173, 53, views.PeriodThisWeek, tt.split, periodTokens()))
			names := func(from, n int) []string {
				var named []string
				for _, r := range rows[from : from+n] {
					lead := len(r) - len(strings.TrimLeft(r, " "))
					name, _, _ := strings.Cut(r[lead:], "  ")
					named = append(named, r[:lead]+name)
				}
				return named
			}
			n := len(tt.total)
			if got := names(2, n); !slices.Equal(got, tt.total) {
				t.Errorf("the total's rows are %q, want %q", got, tt.total)
			}
			if got := names(2+n+2, n); got[0] != "Thu 1 Oct" || !slices.Equal(got[1:], tt.total[1:]) {
				t.Errorf("Thu 1 Oct's rows are %q, want it split as the total is", got)
			}
		})
	}
}

// The rows fill the view to the footnote, a row at a time, never a page of
// whole rows with blank rows under them: the last shown cut where its split
// rows don't fit, the row after saying how many more are below; scrolled,
// they move a row at a time.
func TestAccountsTokensRowsFillTheViewAndCutTheLast(t *testing.T) {
	f := accountsTokensFrame(160, 18, views.PeriodThisWeek, TokensByAccount, periodTokens())
	rows := accountsTokensRows(f)
	placed, note := f.accountsTokens().placed(f.top())
	last := placed.first - f.top() + placed.rows - 1
	if got := rows[last]; !strings.HasPrefix(got, "▼ ") || !strings.Contains(got, " more below") {
		t.Fatalf("the periods' last row reads %q, want how many more are below, in\n%s", got, strings.Join(rows, "\n"))
	}
	if got := rows[last-1]; strings.TrimSpace(got) == "" {
		t.Errorf("the row over the cut is blank, want the rows to fill to it, in\n%s", strings.Join(rows, "\n"))
	}
	if len(note) == 0 {
		t.Errorf("the footnote went, want it kept under the rows")
	}
	s := f.Scrolling(threeRouted(), now)
	content := (4 + 1) + 1 + 4 + 1 + 3 + 1 + 2 // the total and its rule, then the days, a blank row between each
	if s.Most != content-(placed.rows-1) {
		t.Errorf("Tokens scrolls as far as %d rows, want %d, a row at a time", s.Most, content-(placed.rows-1))
	}
	f.Scroll = 1
	if got := accountsTokensRows(f)[2]; !strings.HasPrefix(got, "└ work") {
		t.Errorf("scrolled a row, the first row shown reads %q, want the total's first split", got)
	}
	if !f.ScrollsRows(threeRouted(), now) {
		t.Error("Accounts' Tokens scrolls a thing at a time, want a row at a time")
	}
}

// The total's figures and today's row's each brighten as the number they
// are, never one for both: the total's of the period Accounts counts over.
func TestAccountsTokensTotalAndTodayAreNumberedApart(t *testing.T) {
	got := accountsTokensFrame(160, 37, views.PeriodThisWeek, TokensByAccount, periodTokens()).Numbers(threeRouted(), now)
	for n, want := range map[Number]string{
		{Kind: NumberPeriod, Of: "tokens total"}:                  "660M",
		{Kind: NumberPeriod, Account: "side", Of: "tokens worth"}: "$239+",
		{Kind: NumberTokens, Of: "total"}:                         "660M",
		{Kind: NumberTokens, Of: "against"}:                       "2.0×$20",
		{Kind: NumberTokensSplit, Account: "side", Of: "worth"}:   "$239+",
	} {
		if got[n] != want {
			t.Errorf("%+v reads %q, want %q", n, got[n], want)
		}
	}
}

// With nothing recorded over the period, Tokens says so; till its page is
// built, it draws nothing.
func TestAccountsTokensOfNothingRecordedSaysSo(t *testing.T) {
	rows := accountsTokensRows(accountsTokensFrame(160, 37, views.PeriodThisWeek, TokensByAccount, &views.TokensPage{}))
	if got := strings.TrimSpace(rows[0]); got != nothingRecorded {
		t.Errorf("Tokens of nothing recorded reads %q, want %q", got, nothingRecorded)
	}
	for i, r := range accountsTokensRows(accountsTokensFrame(160, 37, views.PeriodThisWeek, TokensByAccount, nil)) {
		if strings.TrimSpace(r) != "" {
			t.Errorf("Tokens unbuilt draws %q on its row %d, want nothing", r, i)
		}
	}
}
