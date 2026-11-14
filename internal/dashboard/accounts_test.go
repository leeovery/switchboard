package dashboard

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// What the preferences keep of Accounts reads back as it was kept: the
// sub-tab and Tokens' split, by their names; never what's chosen in it,
// which is the run's alone.
func TestAccountsChoicesAreKeptByNameAndReadBack(t *testing.T) {
	tests := []struct {
		name  string
		shown AccountsShown
		kept  theme.AccountsPrefs
	}{
		{name: "as Accounts opens", kept: theme.AccountsPrefs{View: "compare", Split: "account"}},
		{name: "Detail", shown: AccountsShown{Shown: AccountsDetail}, kept: theme.AccountsPrefs{View: "detail", Split: "account"}},
		{name: "Tokens, by model", shown: AccountsShown{Shown: AccountsTokens, TokensSplit: TokensByModel}, kept: theme.AccountsPrefs{View: "tokens", Split: "model"}},
		{name: "Compare, not split", shown: AccountsShown{TokensSplit: TokensUnsplit}, kept: theme.AccountsPrefs{View: "compare", Split: "none"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chosen := tt.shown
			chosen.Accounts, chosen.Focused, chosen.Focus, chosen.First = threeConfigured, true, "side", 1
			chosen.Chosen, chosen.ListScroll, chosen.InSessions, chosen.Selected = "work", 2, true, "d28c"
			if got := chosen.Kept(); got != tt.kept {
				t.Errorf("Kept() = %+v, want %+v", got, tt.kept)
			}
			if got := AccountsKept(tt.kept); !reflect.DeepEqual(got, tt.shown) {
				t.Errorf("AccountsKept(%+v) = %+v, want %+v", tt.kept, got, tt.shown)
			}
		})
	}
}

// What the preferences kept that names nothing Accounts knows, as a later
// build's, shows as Accounts opens: Compare.
func TestAccountsChoicesKeptThatNameNothingReadAsItOpens(t *testing.T) {
	for _, kept := range []theme.AccountsPrefs{{View: "chart"}, {View: "Detail"}, {Period: "30-days"}, {Split: "by model"}} {
		if got := AccountsKept(kept); !reflect.DeepEqual(got, AccountsShown{}) {
			t.Errorf("AccountsKept(%+v) = %+v, want Compare", kept, got)
		}
	}
}

// With one account, Compare goes, whatever was kept: Detail shows it in its
// place, the one there is, whatever was chosen, and Tokens stays. With more,
// the sub-tab shown is drawn, and Detail shows the account chosen, or all
// accounts.
func TestOneAccountIsShownInDetailAlone(t *testing.T) {
	tests := []struct {
		name        string
		shown       AccountsShown
		wantView    AccountsView
		wantAccount string
	}{
		{name: "Compare, of three", shown: AccountsShown{Accounts: threeConfigured}, wantView: AccountsCompare},
		{name: "Detail, of three, all accounts", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured}, wantView: AccountsDetail},
		{name: "Detail, of three, one chosen", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured, Chosen: "side"}, wantView: AccountsDetail, wantAccount: "side"},
		{name: "Compare kept, of one", shown: AccountsShown{Accounts: threeConfigured[:1]}, wantView: AccountsDetail, wantAccount: "work"},
		{name: "Detail, of one, all accounts chosen", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured[:1]}, wantView: AccountsDetail, wantAccount: "work"},
		{name: "Tokens, of one", shown: AccountsShown{Shown: AccountsTokens, Accounts: threeConfigured[:1]}, wantView: AccountsTokens, wantAccount: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.shown.View(); got != tt.wantView {
				t.Errorf("View() = %d, want %d", got, tt.wantView)
			}
			if got := tt.shown.Account(); got != tt.wantAccount {
				t.Errorf("Account() = %q, want %q", got, tt.wantAccount)
			}
		})
	}
}

// Accounts' heading row names it and its sub-tabs, Compare, Detail and
// Tokens, the one drawn picked out, and [ ] after them; with one account,
// Detail and Tokens, Compare gone; and at its right its period between <
// and >, which the frames draw before the sweep of keys as p and the period.
func TestAccountsHeadingRow(t *testing.T) {
	const named = "ACCOUNTS    Compare  Detail  Tokens  [ ]  "
	tests := []struct {
		name   string
		shown  AccountsShown
		period views.Period
		picked int
		other  int
		want   string
	}{
		{name: "Compare, this week", shown: AccountsShown{Accounts: threeConfigured}, picked: 11, other: 20, want: named + rule(103) + "  < this week >"},
		{name: "Detail, today", shown: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured}, period: views.PeriodToday, picked: 20, other: 28, want: named + rule(107) + "  < today >"},
		{name: "Tokens, this week", shown: AccountsShown{Shown: AccountsTokens, Accounts: threeConfigured}, picked: 28, other: 11, want: named + rule(103) + "  < this week >"},
		{name: "the last 30 days", shown: AccountsShown{Accounts: threeConfigured}, period: views.PeriodThirtyDays, picked: 11, other: 20, want: named + rule(105) + "  < 30 days >"},
		{name: "the last 12 weeks", shown: AccountsShown{Accounts: threeConfigured}, period: views.PeriodTwelveWeeks, picked: 11, other: 20, want: named + rule(104) + "  < 12 weeks >"},
		{name: "the last 12 months", shown: AccountsShown{Accounts: threeConfigured}, period: views.PeriodTwelveMonths, picked: 11, other: 20, want: named + rule(103) + "  < 12 months >"},
		{name: "one account, Compare gone", shown: AccountsShown{Accounts: threeConfigured[:1]}, picked: 11, other: 19, want: "ACCOUNTS    Detail  Tokens  [ ]  " + rule(112) + "  < this week >"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 37)
			f.View, f.Accounts, f.Period = Accounts, tt.shown, tt.period
			c := f.page(threeRouted(), now)
			if got := c.rows(Look{})[4]; got != tt.want {
				t.Errorf("row 5 reads\n%q\nwant\n%q", got, tt.want)
			}
			if got := c.at(tt.picked, 4).ink; got != shownSubTabInk {
				t.Errorf("the sub-tab drawn, at column %d, is in %+v, want it picked out", tt.picked, got)
			}
			if tt.other < 0 {
				return
			}
			if got := c.at(tt.other, 4).ink; got != tabInk {
				t.Errorf("the other sub-tab, at column %d, is in %+v, want %+v", tt.other, got, tabInk)
			}
		})
	}
}

// On a phone, Accounts' heading sets its sub-tabs two cells after its name, its
// period on the row under them, at the right, and the view starts two rows
// on, as accounts-phone-compare-52 draws it.
func TestAccountsHeadingOnAPhone(t *testing.T) {
	f := frameOf(52, 48)
	f.View, f.Accounts = Accounts, AccountsShown{Accounts: threeConfigured}
	rows := f.page(threeRouted(), now).rows(Look{})

	if got, want := rows[6], "ACCOUNTS   Compare  Detail  Tokens"; strings.TrimRight(got, " ") != want {
		t.Errorf("row 7 reads %q, want %q", got, want)
	}
	if got, want := rows[7], blanks(39)+"< this week >"; got != want {
		t.Errorf("row 8 reads %q, want %q", got, want)
	}
	if top := f.top(); top != 9 {
		t.Errorf("the view starts at row %d, want row 10", top+1)
	}
}

// Accounts' form is built once a frame, as the Overview's sections are:
// every look a frame Built of a moment takes, its page, how it scrolls, its
// click targets and its bars filled, shares one drawing of Compare's body,
// Detail's panel or its stack, naming the windows no more often than
// the page alone does; and its bars filled draw nothing.
func TestAccountsFormIsBuiltOnceAFrame(t *testing.T) {
	doc, page := phoneDoc(), phonePage(t)
	tests := []struct {
		name string
		f    Frame
	}{
		{name: "Compare", f: compareFrame(t, threeFramed, *page)},
		{name: "Detail", f: detailFrame(Screen(builtin(t, "nord")), threeFramed, "work", "", page, nil)},
		{name: "turned Compare", f: phoneFrame(t, 48, AccountsCompare, "", page)},
		{name: "stacked Detail", f: phoneFrame(t, 41, AccountsDetail, "work", page)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alone, drawn := namingCounted(tt.f)
			alone.Built(doc, routedAt).Draw(doc, routedAt)
			if *drawn == 0 {
				t.Fatal("the page names no window, so counting tells nothing")
			}
			f, looked := namingCounted(tt.f)
			built := f.Built(doc, routedAt)
			built.Draw(doc, routedAt)
			built.Scrolling(doc, routedAt)
			built.Targets(doc, routedAt)
			built.Filled(doc, routedAt)
			if *looked != *drawn {
				t.Errorf("the frame's looks named windows %d times, want %d, as its page alone does", *looked, *drawn)
			}
			f, filled := namingCounted(tt.f)
			f.Built(doc, routedAt).Filled(doc, routedAt)
			if *filled != 0 {
				t.Errorf("its bars filled named windows %d times, want none: drawing nothing", *filled)
			}
		})
	}
}

// namingCounted is the frame naming each window by its key, and how many
// times it has named one.
func namingCounted(f Frame) (Frame, *int) {
	named := new(int)
	f.WindowName = func(key string) string {
		*named++
		return key
	}
	return f, named
}

// Accounts' views, scrolled a row at a time, show each heading with the
// first row under it, or end the page over the heading, at any height and
// however far they're scrolled: Compare's groups; Detail's USAGE, the rule
// over the last six weeks and SESSIONS; and turned and stacked, Compare's and
// Detail's parts, each under its heading. A heading on the page's first row
// stays, as the page has no row over it to end on.
func TestAccountsShowNoHeadingWithoutItsFirstRow(t *testing.T) {
	t.Parallel()
	_, sessions, _ := overviewWorld()
	compare, compareDoc := threeCompared(t, 160, 37)
	detail := detailFrame(Look{}, framedAccounts, "side", "", detailsPage(t), sessions)
	tests := []struct {
		name     string
		f        Frame
		doc      status.Document
		cells    func(row string) string
		headings []string
	}{
		{name: "Compare", f: compare, doc: compareDoc, cells: plain, headings: []string{"ACTIVITY", "LIMITS", "VALUE"}},
		{name: "Detail", f: detail, doc: usageWorld(extraOff, happened(onAuto()...)), cells: panelCells, headings: []string{"USAGE", "┄┄ the last six weeks", "SESSIONS"}},
		{
			name: "turned Compare", f: phoneText(t, 52, 37, AccountsCompare, "", phonePage(t), nil), doc: phoneDoc(), cells: plain,
			headings: []string{"WEEK USED", "BY MODEL", "ACTIVITY", "LIMITS", "VALUE"},
		},
		{
			name: "stacked Detail", f: phoneText(t, 52, 37, AccountsDetail, "side", phonePage(t), sessions), doc: phoneDoc(), cells: plain,
			headings: []string{"USAGE", "THIS WEEK", "SESSIONS", "LATELY"},
		},
	}
	for _, tt := range tests {
		for height := 14; height <= 37; height++ {
			f := tt.f
			f.Height = height
			for scroll := range f.Scrolling(tt.doc, routedAt).Most + 1 {
				f.Scroll = scroll
				rows := textOf(f.Draw(tt.doc, routedAt))
				view := rows[f.top() : f.top()+f.viewRows()]
				if y := headingAlone(view, tt.cells, tt.headings); y > 0 {
					t.Errorf("%s at %d rows, scrolled %d, row %d is a heading without its first row:\n%s", tt.name, height, scroll, f.top()+y+1, strings.Join(rows, "\n"))
				}
			}
		}
	}
}

// plain is a row's text, trimmed at its right.
func plain(row string) string {
	return strings.TrimRight(row, " ")
}

// headingAlone is the first of the view's rows, as cells reads them, that
// leads with one of the headings given with none of the rows under it
// drawn on, -1 where none does.
func headingAlone(view []string, cells func(string) string, headings []string) int {
	for y, row := range view {
		text := strings.TrimLeft(cells(row), " ")
		heading := slices.ContainsFunc(headings, func(h string) bool { return strings.HasPrefix(text, h) })
		if heading && !slices.ContainsFunc(view[y+1:], func(r string) bool { return cells(r) != "" }) {
			return y
		}
	}
	return -1
}
