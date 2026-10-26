package dashboard

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// comparedIDs are the ids, shortest first, of the versions Compare's frames
// list, in the version table's order: Opus 5.5, Opus 5, Sonnet 5.5, Sonnet
// 5, Fable 5.1 and Haiku 4.5.
var comparedIDs = []string{"claude-opus-5-5", "claude-opus-5", "claude-sonnet-5-5", "claude-sonnet-5", "claude-fable-5-1", "claude-haiku-4-5"}

// comparedVersions are those versions, as the version table names and
// shades them.
func comparedVersions(t *testing.T) []ledger.Version {
	t.Helper()
	versions := make([]ledger.Version, len(comparedIDs))
	for i, id := range comparedIDs {
		v, ok := ledger.Pricing.Version(id)
		if !ok {
			t.Fatalf("the version table names no %s", id)
		}
		versions[i] = v
	}
	return versions
}

// compareFramed is a row of Compare's frames: an account's, or every
// account's, as accountsFramed has it; the requests and sessions it served;
// its share of use of each of comparedIDs, as a percentage, -1 where it used
// none; and where its plan isn't Max 20x over a week, $46, its plan's name,
// its price a month, and what it costs over the period.
type compareFramed struct {
	accountsFramed
	requests, sessions int
	shares             []int
	plan               string
	price, cost        float64
}

// row is the row as the views give it, each version's worth its share of a
// hundred dollars.
func (f compareFramed) row() views.AccountsRow {
	row := f.accountsFramed.row()
	row.Totals.Requests, row.Totals.Sessions = f.requests, f.sessions
	for i, share := range f.shares {
		if share >= 0 {
			row.Totals.ByModel = append(row.Totals.ByModel, views.ModelShare{
				Model: comparedIDs[i], Share: new(float64(share) / 100), Amount: usd(float64(share)),
			})
		}
	}
	if f.plan != "" {
		row.PlanName = f.plan
	}
	if f.price > 0 {
		row.Plan.Price = usd(f.price)
	}
	if f.cost > 0 {
		row.Plan.Cost = usd(f.cost)
	}
	return row
}

// comparedPage is Accounts' page over the period given of the rows given,
// the last every account's, using each of the versions given.
func comparedPage(period views.AccountsPeriod, versions []ledger.Version, rows ...compareFramed) views.AccountsPage {
	page := views.AccountsPage{Period: period, Recorded: true, Versions: versions}
	for _, f := range rows[:len(rows)-1] {
		page.Rows = append(page.Rows, f.row())
	}
	page.All = rows[len(rows)-1].row()
	countPlans(&page)
	return page
}

// The rows of accounts-compare-160 and accounts-compare-bad-hour-160, this
// week, and the first three columns' of accounts-compare-ten-160.
var (
	compareWork     = compareFramed{accountsFramed: framedWork, requests: 1204, sessions: 9, shares: []int{55, 7, 28, 3, 5, 2}}
	comparePersonal = compareFramed{accountsFramed: framedPersonal, requests: 2330, sessions: 6, shares: []int{29, -1, 60, 7, -1, 4}}
	compareSide     = compareFramed{accountsFramed: framedSide, requests: 1876, sessions: 11, shares: []int{82, -1, 16, -1, 1, 1}}
	compareAll      = compareFramed{accountsFramed: framedAll, requests: 5410, sessions: 19, shares: []int{57, 2, 33, 3, 2, 3}}
)

// tenRow is an account's row of accounts-compare-ten-160 past its first
// three, on Max 20x.
func tenRow(id string, requests, sessions, off int, limits map[string]int, atCap int, left, worth, ratio float64, shares []int) compareFramed {
	return compareFramed{
		id: id, off: off, limits: limits, atCap: atCap, left: new(left), worth: worth, ratio: ratio, planKnown: true,
		requests: requests, sessions: sessions, shares: shares,
	}
}

// tenRows are the rows of accounts-compare-ten-160: the first three as
// accounts-compare-160's; client; spare, on Max 5x; lab; team, extra, ops
// and studio, out of view; and every account's, on ten plans.
func tenRows() []compareFramed {
	spare := tenRow("spare", 96, 1, 0, nil, 0, 0.91, 9, 0.4, []int{12, -1, 74, 6, -1, 8})
	spare.plan, spare.price, spare.cost = "Max 5x", 100, 23
	all := compareFramed{
		on: 10, limits: map[string]int{"5h": 7}, atCap: 9*60 + 57, worth: 1342, ratio: 3.4, planKnown: true,
		requests: 12598, sessions: 51, shares: []int{53, 4, 35, 4, 1, 3}, price: 1700, cost: 392,
	}
	return []compareFramed{
		compareWork, comparePersonal, compareSide,
		tenRow("client", 1410, 5, 0, nil, 0, 0.22, 151, 3.3, []int{44, 4, 43, 4, -1, 5}),
		spare,
		tenRow("lab", 1102, 4, 1, map[string]int{"5h": 1}, 35, 0.30, 133, 2.9, []int{64, 6, 25, -1, 3, 2}),
		tenRow("team", 900, 4, 0, nil, 0, 0.1, 100, 2.2, []int{50, -1, 50, -1, -1, -1}),
		tenRow("extra", 400, 3, 0, nil, 0, 0.5, 40, 0.9, []int{50, -1, 50, -1, -1, -1}),
		tenRow("ops", 300, 2, 0, nil, 0, 0.6, 30, 0.7, []int{50, -1, 50, -1, -1, -1}),
		tenRow("studio", 900, 4, 0, nil, 0, 0.4, 95, 2.1, []int{50, -1, 50, -1, -1, -1}),
		all,
	}
}

// tenFramed are the ten accounts accounts-compare-ten-160 configures.
var tenFramed = config.Accounts{
	{ID: "work"}, {ID: "personal", Primary: true}, {ID: "side"}, {ID: "client"}, {ID: "spare"},
	{ID: "lab"}, {ID: "team"}, {ID: "extra"}, {ID: "ops"}, {ID: "studio"},
}

// tenCompared is the auto world with seven accounts more, as
// accounts-compare-ten-160 draws it: client, spare and lab, its 5-hour
// window under pressure; then team, at its 5-hour limit, extra, ops and
// studio, their weeks used as every account's week is 45% used. Each week
// resets as the frame places its even pace.
func tenCompared(d *status.Document) {
	more := []struct {
		id    string
		week  float64
		reset time.Time
	}{
		{"client", 0.47, routedClock(4, 18, 0)}, {"spare", 0.05, routedClock(6, 17, 0)}, {"lab", 0.58, routedClock(5, 10, 0)},
		{"team", 0.40, routedClock(5, 10, 0)}, {"extra", 0.40, routedClock(5, 10, 0)}, {"ops", 0.40, routedClock(5, 10, 0)}, {"studio", 0.42, routedClock(5, 10, 0)},
	}
	for _, a := range more {
		d.Accounts = append(d.Accounts, usageAccount(a.id, 0.2, routedClock(1, 18, 0), a.week, 0, a.reset))
	}
	account("lab", func(a *status.Account) {
		runsOut := routedClock(1, 16, 5)
		rate := 0.8 / runsOut.Sub(routedAt).Hours()
		a.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: routedAt.Add(-score.Recent)}}
		a.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: routedAt.Add(-score.Recent), RunsOut: runsOut, Under: true}
	})(d)
	account("team", func(a *status.Account) {
		a.Windows[0] = quota.Window{Key: "5h", Label: "Session", Utilization: 1, Status: quota.StatusRejected, ResetsAt: routedClock(1, 15, 30)}
		a.Limit = status.Limit{Windows: []string{"5h"}, Until: routedClock(1, 15, 30)}
	})(d)
}

// threeFramed are work, personal, the primary, and side, as Accounts'
// frames configure them.
var threeFramed = config.Accounts{{ID: "work"}, {ID: "personal", Primary: true}, {ID: "side"}}

// compareFrame is a frame of Compare, 160 columns by 37, in nord, of the
// accounts configured and the period's page given.
func compareFrame(t *testing.T, configured config.Accounts, page views.AccountsPage) Frame {
	f := frameOf(160, 37)
	f.View, f.Look, f.WeekWindow, f.VersionNameCells = Accounts, Screen(builtin(t, "nord")), "7d", views.VersionNameCells(ledger.Pricing)
	f.Accounts = AccountsShown{Period: page.Period, Accounts: configured, Page: &page}
	return f
}

// extraOff has every account's extra usage read off, as Compare's frames
// have it.
func extraOff(d *status.Document) {
	for i := range d.Accounts {
		d.Accounts[i].Extra = quota.ExtraUsage{Status: quota.StatusRejected}
	}
}

// threeCompared is Compare of the auto world's three accounts this week, as
// text alone, of a terminal the size given.
func threeCompared(t *testing.T, width, height int) (Frame, status.Document) {
	f := compareFrame(t, threeFramed, comparedPage(views.AccountsThisWeek, comparedVersions(t), compareWork, comparePersonal, compareSide, compareAll))
	f.Width, f.Height, f.Look = width, height, Look{}
	return f, usageWorld(extraOff)
}

// tenOnCompare is Compare of ten accounts this week, as text alone, of a
// terminal the size given, its columns from the first shown given.
func tenOnCompare(t *testing.T, width, height, first int) (Frame, status.Document) {
	f := compareFrame(t, tenFramed, comparedPage(views.AccountsThisWeek, comparedVersions(t), tenRows()...))
	f.Width, f.Height, f.Look = width, height, Look{}
	f.Accounts.First = first
	return f, usageWorld(tenCompared, extraOff)
}

// columnOf is the column the text given starts at along row, -1 where it
// isn't there.
func columnOf(row, text string) int {
	before, _, found := strings.Cut(row, text)
	if !found {
		return -1
	}
	return ansi.StringWidth(before)
}

// As many accounts show as fit 19 cells each, less all accounts', at least
// one and no more than there are, from column 26, all accounts' last; the
// columns share the room alike, the odd cells all accounts' and then the
// first's; past the first shown, those before it are out of view.
func TestCompareShowsAColumnAnAccountAsManyAsFitAllAccountsLast(t *testing.T) {
	three := func(width int) func(t *testing.T) (Frame, status.Document) {
		return func(t *testing.T) (Frame, status.Document) { return threeCompared(t, width, 37) }
	}
	ten := func(width, first int) func(t *testing.T) (Frame, status.Document) {
		return func(t *testing.T) (Frame, status.Document) { return tenOnCompare(t, width, 37, first) }
	}
	tests := []struct {
		name  string
		frame func(t *testing.T) (Frame, status.Document)
		want  map[string]int
	}{
		{name: "three at 200, 43 each, 2 over", frame: three(200), want: map[string]int{"1 work": 26, "2 personal": 70, "3 side": 113, "0 all accounts": 156}},
		{name: "three at 122, 24 each", frame: three(122), want: map[string]int{"1 work": 26, "2 personal": 50, "3 side": 74, "0 all accounts": 98}},
		{name: "three at 100: two shown, 24 each, 2 over", frame: three(100), want: map[string]int{"1 work": 26, "2 personal": 51, "3 side": -1, "0 all accounts": 75}},
		{name: "ten at 200: eight shown, 19 each, 3 over", frame: ten(200, 0), want: map[string]int{"1 work": 26, "2 personal": 46, "3 side": 66, "8 extra": 161, "9 ops": -1, "0 all accounts": 180}},
		{name: "ten at 160, from the fourth", frame: ten(160, 3), want: map[string]int{"3 side": -1, "4 client": 26, "9 ops": 121, "10 studio": -1, "0 all accounts": 140}},
		{name: "ten at 160, from further than leaves six", frame: ten(160, 8), want: map[string]int{"4 client": -1, "5 spare": 26, "10 studio": 121, "0 all accounts": 140}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := tt.frame(t)
			heads := f.Draw(doc, routedAt)[6]
			for text, want := range tt.want {
				if got := columnOf(heads, text); got != want {
					t.Errorf("%q is at column %d, want %d, in %q", text, got, want, heads)
				}
			}
		})
	}
}

// Where Compare shows fewer accounts than there are, the heading says which,
// that ←→ moves along them and that all accounts stays put, dropping its
// last clause where it hasn't the room, as at 100 columns and at 83, the
// narrowest ten accounts' Compare is drawn wide at, its period on a row of
// its own under a phone's chrome; where it shows every account, nothing.
func TestComparesHeadingSaysWhichAccountsItShowsOfMoreThanFit(t *testing.T) {
	tests := []struct {
		name  string
		frame func(t *testing.T) (Frame, status.Document)
		want  string
	}{
		{name: "every account shown", frame: func(t *testing.T) (Frame, status.Document) { return threeCompared(t, 160, 37) }},
		{
			name:  "ten from the fourth",
			frame: func(t *testing.T) (Frame, status.Document) { return tenOnCompare(t, 160, 37, 3) },
			want:  "accounts 4–9 of 10  ·  ←→ moves along them; all accounts stays put",
		},
		{
			name:  "three at 100, its last clause dropped",
			frame: func(t *testing.T) (Frame, status.Document) { return threeCompared(t, 100, 37) },
			want:  "accounts 1–2 of 3  ·  ←→ moves along them",
		},
		{
			name:  "ten at 100 from the ninth, its last clause dropped",
			frame: func(t *testing.T) (Frame, status.Document) { return tenOnCompare(t, 100, 37, 8) },
			want:  "accounts 9–10 of 10  ·  ←→ moves along them",
		},
		{
			name:  "ten at 83, the narrowest wide, from the ninth",
			frame: func(t *testing.T) (Frame, status.Document) { return tenOnCompare(t, 83, 37, 8) },
			want:  "accounts 9–10 of 10  ·  ←→ moves along them",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := tt.frame(t)
			heading := f.Draw(doc, routedAt)[f.headingY()]
			_, state, _ := strings.Cut(heading, "Detail")
			state, _, _ = strings.Cut(state, "<")
			if got := strings.Trim(state, "─ []"); got != tt.want {
				t.Errorf("the heading's state is %q, want %q, in %q", got, tt.want, heading)
			}
		})
	}
}

// The columns scroll to show the account the focus moves to, no further than
// they must, all accounts showing wherever they're scrolled.
func TestComparesColumnsScrollToShowTheFocusNoFurtherThanTheyMust(t *testing.T) {
	tests := []struct {
		name  string
		first int
		id    string
		want  int
	}{
		{name: "one shown", id: "lab", want: 0},
		{name: "one past the last shown", id: "team", want: 1},
		{name: "the last", id: "studio", want: 4},
		{name: "one before the first shown", first: 4, id: "work", want: 0},
		{name: "all accounts", first: 2, id: "", want: 2},
		{name: "one the config doesn't hold", first: 2, id: "gone", want: 2},
		{name: "from further than they go", first: 9, id: "side", want: 2},
		{name: "all accounts, from further than they go", first: 9, id: "", want: 4},
		{name: "the last, from further than they go", first: 9, id: "studio", want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := tenOnCompare(t, 160, 37, tt.first)
			if got := f.CompareFirst(tt.id); got != tt.want {
				t.Errorf("CompareFirst(%q) from %d = %d, want %d", tt.id, tt.first, got, tt.want)
			}
		})
	}
}

// Each head is cut to its column less a cell: a name past the most a column
// holds whole with …, keeping its number and its marks. An account the config
// holds and the document doesn't yet, as one just added in place of the
// tenth, which the router still holds, stands not read, its week empty and
// its extra usage a dash, numbered after the document's ten.
func TestComparesHeadsKeepToTheirColumns(t *testing.T) {
	configured := append(slices.Clone(tenFramed[:9]), config.Account{ID: "added"})
	f, doc := tenOnCompare(t, 160, 37, 4)
	f.Accounts.Accounts = configured
	rows := f.Draw(doc.WorkedOut(claudeLike), routedAt)
	tests := []struct {
		row, column int
		want        string
	}{
		{row: 6, column: 121, want: "11 added           "},
		{row: 7, column: 121, want: "○ not read         "},
		{row: 9, column: 121, want: "              —    "},
		{row: 31, column: 121, want: "—                  "},
	}
	for _, tt := range tests {
		if got := ansi.Cut(rows[tt.row], tt.column, tt.column+ansi.StringWidth(tt.want)); got != tt.want {
			t.Errorf("row %d from column %d reads %q, want %q", tt.row, tt.column, got, tt.want)
		}
	}
}

// The heading says which accounts Compare shows by their numbers, the
// first's and the last's, where those run on one by one: where the config
// and the router agree, as their places in the config; where the router
// still holds the first of ten the config no longer has, from 2. Where the
// numbers shown skip one, as side's 3 after work's 1 with personal removed
// and client added, it counts them instead.
func TestComparesHeadingSaysWhichAccountsItShowsByTheirNumbers(t *testing.T) {
	ten := usageWorld(tenCompared, extraOff)
	tests := []struct {
		name       string
		configured config.Accounts
		doc        status.Document
		width      int
		first      int
		want       func(shown int) string
	}{
		{
			name: "the config and the router agree", configured: tenFramed, doc: ten, width: 160, first: 4,
			want: func(shown int) string { return fmt.Sprintf("accounts 5–%d of 10", 4+shown) },
		},
		{
			name: "the first removed, still the router's", configured: tenFramed[1:], doc: ten, width: 160,
			want: func(shown int) string { return fmt.Sprintf("accounts 2–%d of 9", 1+shown) },
		},
		{
			name: "one removed and one added", configured: configured("work", "side", "client"), doc: usageWorld(extraOff), width: 90,
			want: func(shown int) string { return fmt.Sprintf("%d of 3 accounts", shown) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 37)
			f.View, f.WeekWindow, f.Accounts = Accounts, "7d", AccountsShown{Accounts: tt.configured, First: tt.first}
			held := f.At(tt.doc, routedAt)
			shown := held.compareShows()
			if shown < 2 || shown >= len(tt.configured) {
				t.Fatalf("Compare shows %d of %d accounts at %d columns; want some, not all, for its heading to say which", shown, len(tt.configured), tt.width)
			}
			if want := tt.want(shown); rowWith(textOf(f.Draw(tt.doc, routedAt)), want) < 0 {
				t.Errorf("no row says %q", want)
			}
		})
	}
}

// A name is at most 24 cells, its marks among them, however wide its column:
// one past it is cut with …, keeping its number and marks, as a phone's is.
func TestComparesNamesAreCutToTheMostHoweverWideTheirColumn(t *testing.T) {
	const long = "personal-account-of-mine-and-then-some"
	tests := []struct {
		name  string
		frame func(t *testing.T) (Frame, status.Document)
	}{
		{name: "ten at 160", frame: func(t *testing.T) (Frame, status.Document) { return tenOnCompare(t, 160, 37, 0) }},
		{name: "three at 220", frame: func(t *testing.T) (Frame, status.Document) { return threeCompared(t, 220, 37) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := tt.frame(t)
			f.Accounts.Accounts = slices.Clone(f.Accounts.Accounts)
			f.Accounts.Accounts[1].Label = long
			account("personal", func(a *status.Account) { a.Label = long })(&doc)
			heads := f.Draw(doc.WorkedOut(claudeLike), routedAt)[6]
			if at := columnOf(heads, "2 personal-account-of-m… ◆ "); at < 0 {
				t.Errorf("personal's head isn't cut to 24 cells with its marks, in %q", heads)
			}
		})
	}
}

// A column is as wide as the widest account's head and a cell, its name
// whole at most 24 cells, where that's more than 19, so no name is cut where
// another account's column would show it whole: fewer accounts show, and
// their columns share what's left.
func TestComparesColumnsHoldTheWidestNameWhole(t *testing.T) {
	long := map[string]string{"work": "work-team-plan", "personal": "personal-research", "side": "side-hustle-consulting"}
	tests := []struct {
		name  string
		width int
		want  map[string]int
	}{
		{name: "at 108, two shown", width: 108, want: map[string]int{"1 work-team-plan": 26, "2 personal-research ◆": 53, "3 side": -1, "0 all accounts": 80}},
		{name: "at 138, every one shown", width: 138, want: map[string]int{"1 work-team-plan": 26, "2 personal-research ◆": 54, "3 side-hustle-consulting": 82, "0 all accounts": 110}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, tt.width, 37)
			f.Accounts.Accounts = slices.Clone(threeFramed)
			for i, a := range f.Accounts.Accounts {
				f.Accounts.Accounts[i].Label = long[a.ID]
				account(a.ID, func(s *status.Account) { s.Label = long[s.ID] })(&doc)
			}
			heads := f.Draw(doc, routedAt)[6]
			for text, want := range tt.want {
				if got := columnOf(heads, text); got != want {
					t.Errorf("%q is at column %d, want %d, in %q", text, got, want, heads)
				}
			}
		})
	}
}

// All accounts' head counts the configured accounts' states, as their
// columns stand: one the document lacks yet as not read, and one the
// document holds and the config doesn't, not at all.
func TestAllAccountsCountsTheConfiguredAccountsStates(t *testing.T) {
	tests := []struct {
		name       string
		configured config.Accounts
		want       string
	}{
		{name: "one added the document lacks", configured: append(slices.Clone(threeFramed), config.Account{ID: "added"}), want: "2 open · 1 at cap · 1 not read"},
		{name: "one the config no longer holds", configured: threeFramed[1:], want: "2 open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, 200, 37)
			f.Accounts.Accounts = tt.configured
			rows := f.Draw(doc, routedAt)
			x := columnOf(rows[6], "0 all accounts")
			if got := strings.TrimSpace(ansi.Cut(rows[7], x, f.Width)); got != tt.want {
				t.Errorf("all accounts' states read %q, want %q", got, tt.want)
			}
		})
	}
}

// All accounts' extra usage counts every configured account, as its states
// counted do: one the document lacks yet among them, not read, so with work's
// on, it's on for 1 of 4, in Compare's column, in Detail's THIS WEEK and in a
// phone's.
func TestAllAccountsExtraUsageCountsTheConfiguredAccounts(t *testing.T) {
	added := append(slices.Clone(threeFramed), config.Account{ID: "added"})
	workOn := func(d *status.Document) { d.Accounts[0].Extra = quota.ExtraUsage{Status: quota.StatusAllowed} }
	after := func(row, label string) string {
		_, said, _ := strings.Cut(row, label)
		return strings.TrimSpace(said)
	}
	const want = "on for 1 of 4"

	compare, _ := threeCompared(t, 200, 37)
	compare.Accounts.Accounts = added
	rows := textOf(compare.Draw(usageWorld(extraOff, workOn), routedAt))
	x := columnOf(rows[6], "0 all accounts")
	if got := strings.TrimSpace(ansi.Cut(rows[rowWith(rows, "extra usage")], x, compare.Width)); got != want {
		t.Errorf("Compare's all accounts' extra usage reads %q, want %q", got, want)
	}

	detail := detailFrame(Look{}, added, "", "", detailsPage(t), nil)
	rows = textOf(detail.Draw(usageWorld(extraOff, workOn), routedAt))
	if got := after(rows[rowWith(rows, "extra use")], "extra use"); got != want {
		t.Errorf("Detail's all accounts' extra use reads %q, want %q", got, want)
	}

	phone := phoneText(t, 52, 60, AccountsDetail, "", phonePage(t), nil)
	phone.Accounts.Accounts = added
	doc := phoneDoc()
	workOn(&doc)
	rows = textOf(phone.Draw(doc, routedAt))
	if got := after(rows[rowWith(rows, "extra use")], "extra use"); got != want {
		t.Errorf("a phone's all accounts' extra use reads %q, want %q", got, want)
	}
}

// Once ← or → has put the focus on a column, its heads are on bg.selection
// across its width less a cell; none is before.
func TestComparesFocusedColumnsHeadsAreSelected(t *testing.T) {
	tests := []struct {
		name          string
		focused       bool
		focus         string
		first, length int
	}{
		{name: "none focused"},
		{name: "personal", focused: true, focus: "personal", first: 60, length: 32},
		{name: "all accounts", focused: true, focus: "", first: 126, length: 33},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, 160, 37)
			f.Accounts.Focused, f.Accounts.Focus = tt.focused, tt.focus
			c := f.At(doc, routedAt).page(doc, routedAt)
			for y := 5; y <= 8; y++ {
				for x := range f.Width {
					selected := c.at(x, y).ink.on.token == theme.BgSelection
					if want := (y == 6 || y == 7) && x >= tt.first && x < tt.first+tt.length; selected != want {
						t.Errorf("row %d, column %d on bg.selection: %v, want %v", y, x, selected, want)
					}
				}
			}
		})
	}
}

// A value takes the fullest of its forms that fits its column less a cell,
// its words giving way and never a figure: the limits by their windows'
// short names, extra usage on alone, against its price its ratio alone; all
// accounts' to the frame's edge, its full forms whole.
func TestComparesValuesTakeTheFullestFormTheirColumnsHold(t *testing.T) {
	f, doc := tenOnCompare(t, 122, 37, 0)
	account("work", func(a *status.Account) { a.Extra = quota.ExtraUsage{Status: quota.StatusAllowed} })(&doc)
	f.Accounts.Page.Rows[0].Totals.Limits = map[string]int{"5h": 2, "7d": 1}
	rows := f.Draw(doc.WorkedOut(claudeLike), routedAt)
	tests := []struct {
		row, column int
		want        string
	}{
		{row: 25, column: 26, want: "week ×1 · 5h ×2    n"},
		{row: 31, column: 26, want: "on                 o"},
		{row: 31, column: 102, want: "on for 1 of 10"},
		{row: 30, column: 102, want: "$1,700/mo · 10 plans"},
		{row: 33, column: 26, want: "4.0× its $46 week  "},
		{row: 33, column: 102, want: "3.4× its $392 week"},
	}
	for _, tt := range tests {
		if got := ansi.Cut(rows[tt.row], tt.column, tt.column+ansi.StringWidth(tt.want)); got != tt.want {
			t.Errorf("row %d from column %d reads %q, want %q", tt.row, tt.column, got, tt.want)
		}
	}
}

// trimmed are the rows given, from the first to the last, without the
// blanks at their ends.
func trimmed(rows []string, first, last int) []string {
	shown := make([]string, 0, last-first+1)
	for _, row := range rows[first : last+1] {
		shown = append(shown, strings.TrimRight(row, " "))
	}
	return shown
}

// In a terminal too short for its rows, Compare holds its heads and scrolls
// the rest under them a row at a time, saying over the key line how many are
// out of view, and how to scroll to them.
func TestCompareScrollsItsRowsUnderItsHeadsHeld(t *testing.T) {
	tests := []struct {
		name   string
		scroll int
		want   map[int]string
	}{
		{
			name: "at the top",
			want: map[int]string{6: "                          1 work", 9: "WEEK USED", 26: "  at cap or limit", 27: "▼ 7 more below · j/k"},
		},
		{
			name: "scrolled to the foot", scroll: 7,
			want: map[int]string{6: "                          1 work", 9: "  ● Fable 5.1", 26: "  against its price", 27: "▲ 7 more above · j/k"},
		},
		{
			name: "past the foot, as far as it goes", scroll: 12,
			want: map[int]string{9: "  ● Fable 5.1", 27: "▲ 7 more above · j/k"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, 160, 29)
			f.Scroll = tt.scroll
			rows := f.Draw(doc, routedAt)
			for y, want := range tt.want {
				if !strings.HasPrefix(rows[y], want) {
					t.Errorf("row %d is %q, want it to start %q", y, rows[y], want)
				}
			}
		})
	}
	f, doc := threeCompared(t, 160, 29)
	if got, want := f.Scrolling(doc, routedAt), (Scrolling{Most: 7, Page: 18}); got != want {
		t.Errorf("Compare at 29 rows scrolls %+v, want %+v", got, want)
	}
	if x, y, width, rows := f.At(doc, routedAt).scrolls(doc, routedAt); x != 0 || y != 9 || width != 160 || rows != 18 {
		t.Errorf("what scrolls is %d rows from column %d along row %d, %d wide, want 18 from column 0 along row 9, 160 wide", rows, x, y, width)
	}
	f.Height = 37
	if rows := f.Draw(doc, routedAt); strings.TrimSpace(rows[35]) != "" {
		t.Errorf("with room for every row, the row over the key line is %q, want it blank", rows[35])
	}
}

// percentages match a percentage, or the dash said for none.
var percentages = regexp.MustCompile(`\d+%|—`)

// WEEK USED shows the week the frame names the accounts' own, as Accounts'
// page reads it, whatever the policy shares: of every account, and of all
// accounts; and nothing of a week the frame names none.
func TestWEEKUSEDShowsTheWeekTheFrameNames(t *testing.T) {
	tests := []struct {
		name, week string
		want       []string
	}{
		{name: "the accounts' week", week: "7d", want: []string{"34%", "66%", "78%", "59%"}},
		{name: "Fable's week named so", week: "7d_oi", want: []string{"12%", "0%", "4%", "5%"}},
		{name: "none named", want: []string{"—", "—", "—", "—"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, 160, 37)
			f.WeekWindow = tt.week
			if got := percentages.FindAllString(f.Draw(doc, routedAt)[9], -1); !slices.Equal(got, tt.want) {
				t.Errorf("WEEK USED reads %q, want %q", got, tt.want)
			}
		})
	}
}

// Before the period's days are read, Compare draws what's live alone: its
// heads and WEEK USED.
func TestCompareDrawsItsHeadsAndWEEKUSEDTillItsDaysAreRead(t *testing.T) {
	f, doc := threeCompared(t, 160, 37)
	f.Accounts.Page = nil
	rows := f.Draw(doc, routedAt)
	if got := trimmed(rows, 6, 12); !strings.HasPrefix(got[3], "WEEK USED") || got[1] == "" || got[5] != "" || got[6] != "" {
		t.Errorf("Compare before its days are read is\n%s\nwant its heads and WEEK USED alone", strings.Join(got, "\n"))
	}
}

// A period with nothing recorded says a dash in each of its own figures, as
// the owner settled it: on a fresh install, and on a week's first morning
// before any request. The plan, from the config, extra usage, live, and
// left at its last reset, of the six weeks, are no figures of the period,
// and say what they are: on a fresh install no week has ended yet.
func TestAPeriodWithNothingRecordedSaysADashForEachOfItsFigures(t *testing.T) {
	dashes := map[int]string{
		20: "  requests                —                                 —                                —                                —",
		21: "  sessions served         —                                 —                                —                                —",
		22: "  sessions moved          —                                 —                                —                                —",
		25: "  limits hit              —                                 —                                —                                —",
		26: "  at cap or limit         —                                 —                                —                                —",
		32: "  worth at API prices     —                                 —                                —                                —",
		33: "  against its price       —                                 —                                —                                —",
	}
	theirOwn := map[int]string{
		30: "  plan                    Max 20x · $200/mo                 Max 20x · $200/mo                Max 20x · $200/mo                $600/mo · 3 plans",
		31: "  extra usage             off                               off                              off                              off on all",
	}
	tests := []struct {
		name string
		page func(*views.AccountsPage)
		left string
	}{
		{name: "a week's first morning", page: func(*views.AccountsPage) {},
			left: "  left at its last reset  0% (limit Fri)                    15%                              40%                              —"},
		{name: "a fresh install", page: func(p *views.AccountsPage) {
			for i := range p.Rows {
				p.Rows[i].WeekLeft, p.Rows[i].RanOut = nil, ""
			}
		}, left: "  left at its last reset  —                                 —                                —                                —"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, 160, 37)
			f.Accounts.Page.Recorded = false
			tt.page(f.Accounts.Page)
			rows := f.Draw(doc, routedAt)
			want := maps.Clone(dashes)
			maps.Copy(want, theirOwn)
			want[27] = tt.left
			for y, want := range want {
				if got := strings.TrimRight(rows[y], " "); got != want {
					t.Errorf("row %d is\n%q, want\n%q", y, got, want)
				}
			}
		})
	}
}

// A click on a column's heads, its name or its state, across the column less
// a cell, moves the focus to it: all accounts' by "".
func TestAClickOnAColumnsHeadsMovesTheFocusThere(t *testing.T) {
	f, doc := threeCompared(t, 160, 37)
	var got []Target
	for _, target := range f.Targets(doc, routedAt) {
		if target.On == OnColumn {
			got = append(got, target)
		}
	}
	var want []Target
	for _, col := range []struct {
		id        string
		at, cells int
	}{{"work", 26, 34}, {"personal", 60, 33}, {"side", 93, 33}, {"", 126, 34}} {
		for y := 6; y <= 7; y++ {
			want = append(want, Target{X: col.at, Y: y, Width: col.cells - 1, On: OnColumn, Account: col.id})
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("Compare's columns' targets are\n%+v\nwant\n%+v", got, want)
	}
}

// WEEK USED's bars ease after a read, each account's by its id and its
// week's key and all accounts' by the key alone, but one drawn empty, as of
// an account whose token isn't usable; each bar fills as far as it has eased.
func TestWEEKUSEDsBarsEaseAsFarAsTheyHaveEased(t *testing.T) {
	f, doc := threeCompared(t, 160, 37)
	account("side", func(a *status.Account) { a.TokenSet = false })(&doc)
	doc = doc.WorkedOut(claudeLike)
	all := f.Moment(doc, routedAt).All().Windows[0].Used
	want := map[Ref]float64{{Account: "work", Window: "7d"}: 0.34, {Account: "personal", Window: "7d"}: 0.66, {Window: "7d"}: all}
	if got := f.Filled(doc, routedAt); !maps.Equal(got, want) {
		t.Errorf("Compare's bars fill %v, want %v", got, want)
	}
	f.Eased = map[Ref]float64{{Account: "work", Window: "7d"}: 0}
	if row := f.Draw(doc, routedAt)[9]; strings.Join(strings.Fields(ansi.Cut(row, 26, 50)), " ") != "┃ ╎" || !strings.Contains(row, "34%") {
		t.Errorf("work's week eased to nothing reads %q, want its bar empty but for its pace and its cap, its percentage as read", ansi.Cut(row, 26, 55))
	}
}

// Every configured account's week is filled, shown or scrolled out of view,
// as the Overview's are, so a column scrolled into view while its bar eases
// eases from where it stood, not from nothing.
func TestWEEKUSEDFillsEveryAccountsWeekShownOrNot(t *testing.T) {
	f, doc := tenOnCompare(t, 160, 37, 0)
	filled := f.Filled(doc, routedAt)
	for _, a := range tenFramed {
		if _, ok := filled[Ref{Account: a.ID, Window: "7d"}]; !ok {
			t.Errorf("%s's week isn't filled, want every account's, shown or not", a.ID)
		}
	}
	if _, ok := filled[Ref{Window: "7d"}]; !ok || len(filled) != len(tenFramed)+1 {
		t.Errorf("Compare fills %v, want each account's week and all accounts'", filled)
	}
}

// What a look reads brightens as it changes: each account's week's
// percentage, as USAGE's is numbered, and all accounts', a figure of its own,
// and its accounts' states counted; and the period's figures, as a look reads
// today's days, each of an account, or all accounts', "", by its row, the
// figures of a row one number, a version's share of use by its name, but no
// dash or none, which says there's no figure: with nothing recorded, but
// the plan and left at its last reset, which are no figures of the period.
func TestComparesNumbersAreWhatALookReads(t *testing.T) {
	f, doc := threeCompared(t, 160, 37)
	got := f.Numbers(doc, routedAt)
	delete(got, Number{Kind: NumberRouted})
	period := make(map[Number]string)
	maps.Copy(period, got)
	maps.DeleteFunc(got, func(n Number, _ string) bool { return n.Kind == NumberPeriod })
	want := map[Number]string{
		{Kind: NumberUsed, Account: "work", Of: "7d"}:     "34%",
		{Kind: NumberUsed, Account: "personal", Of: "7d"}: "66%",
		{Kind: NumberUsed, Account: "side", Of: "7d"}:     "78%",
		{Kind: NumberAllUsed, Of: "7d"}:                   "59%",
		{Kind: NumberAccountStates, Of: "open"}:           "2 open",
		{Kind: NumberAccountStates, Of: "at cap"}:         "1 at cap",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Compare's numbers but the period's are %v, want %v", got, want)
	}
	for n, want := range map[Number]string{
		{Kind: NumberPeriod, Account: "work", Of: "requests"}:               "1,204",
		{Kind: NumberPeriod, Of: "requests"}:                                "5,410",
		{Kind: NumberPeriod, Account: "side", Of: "sessions moved"}:         "4",
		{Kind: NumberPeriod, Account: "work", Of: "limits hit"}:             "5-hour ×2",
		{Kind: NumberPeriod, Account: "side", Of: "at cap or limit"}:        "52m",
		{Kind: NumberPeriod, Account: "work", Of: "left at its last reset"}: "0%",
		{Kind: NumberPeriod, Account: "work", Of: "worth at API prices"}:    "$184",
		{Kind: NumberPeriod, Of: "against its price"}:                       "4.6×$138",
		{Kind: NumberPeriod, Account: "work", Of: "Opus 5.5 share"}:         "55%",
	} {
		if period[n] != want {
			t.Errorf("%+v reads %q, want %q", n, period[n], want)
		}
	}
	for _, n := range []Number{
		{Kind: NumberPeriod, Account: "personal", Of: "limits hit"},
		{Kind: NumberPeriod, Account: "personal", Of: "at cap or limit"},
		{Kind: NumberPeriod, Of: "left at its last reset"},
		{Kind: NumberPeriod, Account: "personal", Of: "Opus 5 share"},
	} {
		if said, ok := period[n]; ok {
			t.Errorf("%+v is numbered, reading %q, where it says there's none", n, said)
		}
	}

	f.Accounts.Page.Recorded, f.Accounts.Page.Versions = false, nil
	for n, said := range f.Numbers(doc, routedAt) {
		if n.Kind == NumberPeriod && n.Of != "plan" && n.Of != "left at its last reset" {
			t.Errorf("a period with nothing recorded numbers %+v, reading %q, its dash", n, said)
		}
	}
}

// An account's state in a word is cued as it changes, and only then: a
// change ROUTING alone sees, as a session lapsing, cues nothing here. The cue
// shows over the focus, as a warning; and while it fades, the page settles.
func TestAnAccountsStateIsCuedAsItsWordChanges(t *testing.T) {
	tests := []struct {
		name          string
		changed, word map[string]float64
		focused       bool
		cued          bool
	}{
		{name: "its word changed", word: map[string]float64{"work": 0.25}, cued: true},
		{name: "its word changed, its column focused", word: map[string]float64{"work": 0}, focused: true, cued: true},
		{name: "ROUTING's state alone changed", changed: map[string]float64{"work": 0.25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, 160, 37)
			f.Look = Screen(builtin(t, "nord"))
			f.Changed, f.StateChanged = tt.changed, tt.word
			f.Accounts.Focused, f.Accounts.Focus = tt.focused, "work"
			c := f.At(doc, routedAt).page(doc, routedAt)
			for x := 24; x < 60; x++ {
				cued := c.at(x, 7).ink.on.token == theme.BgAttention
				if want := tt.cued && x >= 26 && x < 26+ansi.StringWidth("● at cap"); cued != want {
					t.Errorf("column %d of the states cued: %v, want %v", x, cued, want)
				}
			}
			if settles := f.MaySettle(); settles != (len(tt.changed)+len(tt.word) > 0) {
				t.Errorf("the page may settle: %v, want it to while a cue fades", settles)
			}
		})
	}
}

// BY MODEL's bars are the narrowest column less 4, at most 40, and WEEK
// USED's percentage ends where they do, its bar a cell before it.
func TestComparesBarsGrowWithTheirColumnsToTheMost(t *testing.T) {
	tests := []struct {
		name         string
		width, share int
	}{
		{name: "at 138, 28 each", width: 138, share: 24},
		{name: "at 160, 33 at the narrowest", width: 160, share: 29},
		{name: "at 220, 48 at the narrowest, held at the most", width: 220, share: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, doc := threeCompared(t, tt.width, 37)
			rows := f.Draw(doc, routedAt)
			if got := strings.TrimRight(ansi.Cut(rows[11], 26, 26+tt.share+shareSpare), " "); got != strings.Repeat("█", tt.share) {
				t.Errorf("work's BY MODEL bar is %q, want %d cells", got, tt.share)
			}
			if got := columnOf(rows[9], "34%") + len("34%"); got != 26+tt.share {
				t.Errorf("work's week ends at column %d, want %d, with its BY MODEL bar, in %q", got, 26+tt.share, rows[9])
			}
		})
	}
}

// The labels' column holds every label it's drawn with whole, WEEK USED's
// with now, BY MODEL's heading, and the groups' names and their rows'
// labels, and is no wider than the widest of them; the columns start 2
// cells after it, and a version's name past it is cut to it.
func TestComparesLabelsColumnHoldsItsLabelsAndCutsAVersionsName(t *testing.T) {
	f, doc := threeCompared(t, 160, 60)
	f.Accounts.Period = views.AccountsThirtyDays
	f.Accounts.Page.Versions[0].Name = "Opus 5.5 with a name long past the most"
	rows := f.Draw(doc, routedAt)
	labels := compareLabels()
	drawn := make([]string, len(rows))
	for i, row := range rows {
		drawn[i] = strings.TrimRight(ansi.Cut(row, 0, labels), " ")
	}
	want := []string{"WEEK USED  now", "BY MODEL  " + compareByModelSays}
	for _, g := range (compared{page: f.Accounts.Page}).groups() {
		want = append(want, g.name)
		for _, r := range g.rows {
			want = append(want, "  "+r.label)
		}
	}
	widest := 0
	for _, label := range want {
		widest = max(widest, ansi.StringWidth(label))
		if !slices.Contains(drawn, label) {
			t.Errorf("%q isn't drawn whole in the labels' column, %d cells", label, labels)
		}
	}
	if labels != widest {
		t.Errorf("the labels' column is %d cells, want %d, its widest label's", labels, widest)
	}
	if got, want := drawn[12], "  ● "+prose.Cut("Opus 5.5 with a name long past the most", labels-versionAt); got != want {
		t.Errorf("the version's name reads %q, want %q", got, want)
	}
	if got := columnOf(rows[6], "1 work"); got != labels+labelsGap {
		t.Errorf("work's column starts at %d, want %d, 2 cells after the labels'", got, labels+labelsGap)
	}
}

// Without blended colours, BY MODEL's bars are full blocks in each version's
// shade, a column's parts as their shares round, cumulatively.
func TestBYMODELsBarsAreEachVersionsShareOfUse(t *testing.T) {
	f, doc := threeCompared(t, 160, 37)
	row := f.Draw(doc, routedAt)[11]
	for i, x := range []int{26, 60, 93, 126} {
		if got := strings.TrimRight(ansi.Cut(row, x, x+30), " "); got != strings.Repeat("█", 29) {
			t.Errorf("column %d's bar is %q, want 29 cells", i, got)
		}
	}
	if got, want := splitCells([]ledger.Picodollars{29, 0, 60, 7, 0, 4}, 24), []int{7, 0, 14, 2, 0, 1}; !slices.Equal(got, want) {
		t.Errorf("personal's shares split 24 cells %v, want %v", got, want)
	}
	if got := splitCells([]ledger.Picodollars{0, 0}, 24); !slices.Equal(got, []int{0, 0}) {
		t.Errorf("shares worth nothing split 24 cells %v, want none", got)
	}
}

// A version's share of use reads as a whole percentage; one that rounds to
// nothing, <1%; and a dash in text.faint where none of it was used, or none
// priced; each bold where all accounts' is.
func TestAVersionsShareOfUseSaysEachForm(t *testing.T) {
	tests := []struct {
		name  string
		share *float64
		k     ink
		want  line
	}{
		{name: "a share", share: new(0.55), k: secondaryInk, want: line{{"55%", secondaryInk}}},
		{name: "a half percent, to the even", share: new(0.005), k: secondaryInk, want: line{{"<1%", secondaryInk}}},
		{name: "rounding to one", share: new(0.006), k: secondaryInk, want: line{{"1%", secondaryInk}}},
		{name: "none", k: secondaryInk, want: line{noFigure}},
		{name: "all accounts' none", k: strongInk, want: line{{"—", ink{token: theme.TextFaint, bold: true}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitParts(4, shareParts(tt.share, tt.k)...).inked(); !slices.Equal(got, tt.want) {
				t.Errorf("said %+v, want %+v", got, tt.want)
			}
		})
	}
}
