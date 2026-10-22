package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/views"
)

// comparedAccounts are n accounts configured, by short ids alone, or, where
// long is set, each labelled 24 cells, the most a name column takes.
func comparedAccounts(n int, long bool) config.Accounts {
	accounts := make(config.Accounts, n)
	for i := range accounts {
		accounts[i] = config.Account{ID: fmt.Sprintf("a%d", i)}
		if long {
			accounts[i].Label = fmt.Sprintf("research-consulting-%04d", i)
		}
	}
	return accounts
}

// compareWidths are the widths Compare's form is held to.
const compareWidthsFrom, compareWidthsTo = 40, 220

// Compare is drawn wide from the width where its wide form's parts fit at
// their leasts, and turned narrower, switching once as the width grows: the
// labels' column, 24, and its gap, 2, then the first two accounts' columns
// and all accounts', which never give way, each at least 19 cells, or the
// widest head and a cell: from 83 with short names, from 107 with names of
// 24 cells, 110 with ten of them, as "10 " is a cell more; with one account,
// a column fewer. The rest of the accounts' columns give way, so how many
// there are past two moves nothing.
func TestCompareIsWideFromWhereItsLeastsFit(t *testing.T) {
	tests := []struct {
		name     string
		accounts config.Accounts
		from     int
	}{
		{name: "one, short", accounts: comparedAccounts(1, false), from: 64},
		{name: "one, long", accounts: comparedAccounts(1, true), from: 80},
		{name: "two, short", accounts: comparedAccounts(2, false), from: 83},
		{name: "three, short", accounts: comparedAccounts(3, false), from: 83},
		{name: "three, as the frames configure them", accounts: threeFramed, from: 83},
		{name: "ten, as the frames configure them", accounts: tenFramed, from: 83},
		{name: "ten, short", accounts: comparedAccounts(10, false), from: 83},
		{name: "two, long", accounts: comparedAccounts(2, true), from: 107},
		{name: "nine, long", accounts: comparedAccounts(9, true), from: 107},
		{name: "ten, long", accounts: comparedAccounts(10, true), from: 110},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(0, 37)
			f.View, f.Accounts = Accounts, AccountsShown{Accounts: tt.accounts}
			for f.Width = compareWidthsFrom; f.Width <= compareWidthsTo; f.Width++ {
				if got, want := f.CompareWide(), f.Width >= tt.from; got != want {
					t.Errorf("at %d columns, wide %v, want %v", f.Width, got, want)
				}
			}
		})
	}
}

// Compare's form never hangs on the data: it's the same before the period's
// page is read and after, over every period, as its parts are the config's
// alone.
func TestComparesFormNeverHangsOnTheData(t *testing.T) {
	versions := comparedVersions(t)
	pages := []*views.AccountsPage{nil}
	for _, period := range []views.AccountsPeriod{views.AccountsToday, views.AccountsThisWeek, views.AccountsThirtyDays, views.AccountsTwelveWeeks} {
		pages = append(pages, new(comparedPage(period, versions, compareWork, comparePersonal, compareSide, compareAll)))
	}
	doc := usageWorld(extraOff)
	f := compareFrame(t, threeFramed, views.AccountsPage{})
	for f.Width = compareWidthsFrom; f.Width <= compareWidthsTo; f.Width++ {
		var forms []bool
		for _, page := range pages {
			f.Accounts.Page = page
			forms = append(forms, f.buildAccountsForm(doc, routedAt).scrolls != nil)
		}
		if slices.Contains(forms, !forms[0]) {
			t.Errorf("at %d columns, drawn wide %v as the page is read and the period changes", f.Width, forms)
		}
	}
}

// What's drawn, its focus told by help and its accounts told by the heading
// agree with CompareWide at every width: wide, a column an account under
// their heads, its focus in help, and the heading saying which it shows of
// more than fit; turned, a row an account, its groups headed down the left,
// no focus in help and nothing in the heading.
func TestComparesDrawingHelpAndHeadingAgreeWithItsForm(t *testing.T) {
	const focus = "← and → move the focus along the columns"
	for _, configured := range []config.Accounts{threeFramed, tenFramed, comparedAccounts(10, true)} {
		f := compareFrame(t, configured, views.AccountsPage{})
		f.Accounts.Page, f.Look = nil, Look{}
		doc := usageWorld(extraOff)
		for f.Width = compareWidthsFrom; f.Width <= compareWidthsTo; f.Width++ {
			wide := f.CompareWide()
			drawn := strings.Join(accountsDrawn(f, doc), "\n")
			if got := strings.Contains(drawn, "0 all accounts"); got != wide {
				t.Errorf("%d accounts at %d columns: wide %v, but its heads drawn %v, in\n%s", len(configured), f.Width, wide, got, drawn)
			}
			if got := strings.Contains(strings.Join(plainLines(f.compareHelp().says), " "), focus); got != wide {
				t.Errorf("%d accounts at %d columns: wide %v, but help tells of its focus %v", len(configured), f.Width, wide, got)
			}
			if got := f.compareState() != nil; got && !wide {
				t.Errorf("%d accounts at %d columns: turned, but the heading says %q", len(configured), f.Width, plainLines(f.compareState()))
			}
		}
	}
}
