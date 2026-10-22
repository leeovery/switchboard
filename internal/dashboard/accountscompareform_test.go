package dashboard

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
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
// 24 cells, 110 with ten of them, as "10 " is a cell more. The rest of the
// accounts' columns give way, so how many there are past two moves nothing.
// With fewer than two there's nothing to compare, and it's never wide.
func TestCompareIsWideFromWhereItsLeastsFit(t *testing.T) {
	never := compareWidthsTo + 1
	tests := []struct {
		name     string
		accounts config.Accounts
		from     int
	}{
		{name: "none", from: never},
		{name: "one, short", accounts: comparedAccounts(1, false), from: never},
		{name: "one, long", accounts: comparedAccounts(1, true), from: never},
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

// With no accounts configured, Compare draws nothing, and help tells of no
// focus at any width.
func TestComparesHelpTellsOfNoFocusWithNoAccounts(t *testing.T) {
	const focus = "← and → move the focus along the columns"
	for width := compareWidthsFrom; width <= compareWidthsTo; width++ {
		if said := strings.Join(plainLines(accountsHelping(width, AccountsShown{}, Look{}).body), " "); strings.Contains(said, focus) {
			t.Errorf("with no accounts at %d columns, help reads\n%s\nwant no %q", width, said, focus)
		}
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

// comparedWorld is a world Compare is drawn of: the accounts configured,
// the rows of its page, every account's last, and its document.
type comparedWorld struct {
	name       string
	configured config.Accounts
	rows       []compareFramed
	doc        status.Document
}

// comparedWorlds are the worlds of Compare's frames at their widest
// figures: ten accounts, work's extra usage on and its limits hit by two
// windows, as over a month; the first two, three and nine of them; and
// three, side's plan not known.
func comparedWorlds() []comparedWorld {
	ten := tenRows()
	ten[0].limits = map[string]int{"5h": 19, "7d": 2}
	doc := usageWorld(tenCompared, extraOff)
	account("work", func(a *status.Account) { a.Extra = quota.ExtraUsage{Status: quota.StatusAllowed} })(&doc)
	all := ten[len(ten)-1]
	worlds := []comparedWorld{{name: "ten", configured: tenFramed, rows: ten, doc: doc}}
	for _, n := range []int{2, 3, 9} {
		worlds = append(worlds, comparedWorld{
			name: fmt.Sprintf("%d of them", n), configured: tenFramed[:n], rows: append(slices.Clone(ten[:n]), all), doc: doc,
		})
	}
	unknown := compareSide
	unknown.planKnown = false
	return append(worlds, comparedWorld{
		name: "side's plan not known", configured: threeFramed,
		rows: []compareFramed{compareWork, comparePersonal, unknown, compareAll}, doc: usageWorld(extraOff),
	})
}

// formTaken is the form a row's value takes in a column the cells given, by
// its place among its forms, the fullest 0.
type formTaken struct{ cells, form int }

// Wherever Compare is drawn wide, in every world and over every period, each
// row's value takes the fullest of its forms its column holds, and never a
// figure cut: the form it takes fits its column whole, and a wider column
// never gives it a shorter form. Its form switches once as the frame widens,
// at the one width over every period.
func TestComparesValuesCutNoFigureAtAnyWidth(t *testing.T) {
	versions := comparedVersions(t)
	for _, w := range comparedWorlds() {
		t.Run(w.name, func(t *testing.T) {
			doc := w.doc.WorkedOut(claudeLike)
			from := map[views.AccountsPeriod]int{}
			for _, period := range views.AccountsPeriods() {
				f := compareFrame(t, w.configured, comparedPage(period, versions, w.rows...))
				f.Look = Look{}
				taken := map[string][]formTaken{}
				for f.Width = compareWidthsFrom; f.Width <= compareWidthsTo; f.Width++ {
					if !f.CompareWide() {
						if wide, ok := from[period]; ok {
							t.Errorf("%s: turned at %d columns, though wide from %d", period.Name(), f.Width, wide)
						}
						continue
					}
					if _, ok := from[period]; !ok {
						from[period] = f.Width
					}
					cp := f.comparing(doc, routedAt)
					for _, g := range cp.groups() {
						for _, r := range g.rows {
							for _, col := range cp.columns() {
								forms, of := r.said(col), r.label+" of "+cmp.Or(col.id, "all accounts")
								i := slices.IndexFunc(forms, func(form []part) bool { return partsWidth(form) <= col.width })
								if i < 0 {
									t.Errorf("%s at %d columns: %s is cut, its shortest %q in %d cells",
										period.Name(), f.Width, of, plainLines([]line{fitParts(200, forms.shortest()...)}), col.width)
									continue
								}
								taken[of] = append(taken[of], formTaken{cells: col.width, form: i})
							}
						}
					}
				}
				for of, took := range taken {
					slices.SortStableFunc(took, func(a, b formTaken) int { return a.cells - b.cells })
					for j := 1; j < len(took); j++ {
						if took[j].form > took[j-1].form {
							t.Errorf("%s: %s takes form %d in %d cells, shorter than form %d in %d",
								period.Name(), of, took[j].form, took[j].cells, took[j-1].form, took[j-1].cells)
						}
					}
				}
			}
			if widths := slices.Compact(slices.Sorted(maps.Values(from))); len(widths) != 1 {
				t.Errorf("wide from %v as the period changes, want one width", from)
			}
		})
	}
}
