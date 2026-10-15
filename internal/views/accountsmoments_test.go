package views_test

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// accountsIn is what Accounts' page is built of at the moment am: its days,
// asked for from the first, where the ledger began, as the watch learns it,
// by the built-in price table, its accounts' weeks Claude's.
func accountsIn(am viewstest.AccountsMoment) views.HistoryInput {
	return views.HistoryInput{
		Ledger: ledgerOf(am.Days), From: on(am.Days[0].Day), Began: on(am.Days[0].Day), Now: am.Now, Prices: ledger.Pricing,
		Accounts: am.Accounts, WeekStarts: am.WeekStarts, WeekWindow: claude.WeekWindow, Family: claude.Provider{}.Family,
	}
}

// accountsAt is Accounts' page at the moment am, as the watch has it: built
// of the moment's days, for its period, as they were read, then stood at its
// moment; and the moment.
func accountsAt(am viewstest.AccountsMoment) (views.AccountsPage, status.Moment) {
	m := am.Doc.At(am.Now, claude.Policy)
	return builtOf(am).At(m), m
}

// builtOf is Accounts' page of the moment am's days, for its period, built
// as they were read, as pastOf has them.
func builtOf(am viewstest.AccountsMoment) views.AccountsPage {
	return pastOf(am).Accounts(am.Period)
}

// pastOf is the Past of the moment am's days, as they were read, at its
// DaysRead, else its Now.
func pastOf(am viewstest.AccountsMoment) views.Past {
	in := accountsIn(am)
	in.Now = cmp.Or(am.DaysRead, am.Now)
	return views.NewPast(in, am.Days)
}

// rowOf is the page's row of the account with the given id, or every
// account's, for "".
func rowOf(t *testing.T, page views.AccountsPage, id string) views.AccountsRow {
	t.Helper()
	if id == "" {
		return page.All
	}
	i := slices.IndexFunc(page.Rows, func(r views.AccountsRow) bool { return r.Totals.Account == id })
	if i < 0 {
		t.Fatalf("the page has no row of %s", id)
	}
	return page.Rows[i]
}

// pageFigures are the page's figures, as viewstest.Figures names them.
func pageFigures(page views.AccountsPage) viewstest.Figures {
	recorded := "no"
	if page.Recorded {
		recorded = "yes"
	}
	return viewstest.Figures{
		viewstest.FigureRecorded: recorded,
		viewstest.FigureWeeks:    strings.Join(page.Weeks, " "),
		viewstest.FigureToCome:   strconv.Itoa(page.ToCome),
	}
}

// rowFigures are the figures of the row of the account with the given id,
// or every account's, of the page and the moment m of the moment am, as
// viewstest.Figures names them.
func rowFigures(t *testing.T, am viewstest.AccountsMoment, page views.AccountsPage, m status.Moment, id string) viewstest.Figures {
	t.Helper()
	row := rowOf(t, page, id)
	totals := row.Totals
	f := viewstest.Figures{
		viewstest.FigureRequests: strconv.Itoa(totals.Requests),
		viewstest.FigureSessions: strconv.Itoa(totals.Sessions),
		viewstest.FigureMoved:    fmt.Sprintf("on %d off %d", totals.MovedOn, totals.MovedOff),
		viewstest.FigureLimits:   limitsRead(totals.Limits),
		viewstest.FigureMinutes:  minutesRead(totals),
		viewstest.FigurePartial:  "",
		viewstest.FigureLeft:     shareRead(row.WeekLeft),
		viewstest.FigureRanOut:   row.RanOut,
		viewstest.FigurePlan:     planRead(row),
		viewstest.FigureWorth:    cmpOrDash(totals.Dollars()),
		viewstest.FigureAgainst:  againstRead(row.Plan.Against),
		viewstest.FigurePeaks:    peaksRead(row.Peaks),
		viewstest.FigureVerdict:  verdictRead(row.Verdict),
		viewstest.FigureWeekUsed: weekUsedRead(m, id),
		viewstest.FigureRunning:  runningRead(m, am, id),
		viewstest.FigureLately:   latelyRead(views.AccountLately(m, id, 3)),
	}
	if len(totals.Partial) > 0 {
		f[viewstest.FigurePartial] = "partial"
	}
	if id == "" {
		known, of := page.All.Plan.Plans.Known, page.All.Plan.Plans.Of
		f[viewstest.FigurePlansOf] = fmt.Sprintf("%d of %d", known, of)
		most, least, _ := page.Ends()
		f[viewstest.FigureMostLeft], f[viewstest.FigureLeastLeft] = most.Totals.Account, least.Totals.Account
	}
	return f
}

// limitsRead reads limits by window, as "5h×2 7d×1": "" for none.
func limitsRead(limits map[string]int) string {
	var read []string
	for _, key := range slices.SortedFunc(maps.Keys(limits), quota.CompareKeys) {
		read = append(read, key+"×"+strconv.Itoa(limits[key]))
	}
	return strings.Join(read, " ")
}

// minutesRead reads the minutes at a cap and at a limit, together: "—" where
// none was read.
func minutesRead(t views.AccountTotals) string {
	if t.MinutesAtCap == nil || t.MinutesAtLimit == nil {
		return "—"
	}
	return strconv.Itoa(*t.MinutesAtCap + *t.MinutesAtLimit)
}

// planRead reads a row's plan, by its name, and its price, as "Max 20x
// $200/mo", or every account's, its price alone: "—" where no plan is known.
func planRead(row views.AccountsRow) string {
	switch {
	case row.Plan.Price == nil:
		return cmpOrDash(row.PlanName)
	case row.Plan.All:
		return row.PlanName + row.Plan.Monthly()
	}
	return row.PlanName + " " + row.Plan.Monthly()
}

// againstRead reads a worth against what plans cost, as "4.0× of $46": "—"
// where there's no ratio.
func againstRead(a views.Against) string {
	if a.Ratio == nil || a.Cost == nil {
		return "—"
	}
	return prose.Ratio(*a.Ratio) + " of " + a.Cost.GroupedDollars()
}

// cmpOrDash is said, or "—" where it's "".
func cmpOrDash(said string) string {
	if said == "" {
		return "—"
	}
	return said
}

// weekUsedRead reads the use of the week window of the account with the
// given id at the moment m, as WEEK USED draws it: "" for every account's,
// and where it hasn't one.
func weekUsedRead(m status.Moment, id string) string {
	for _, a := range m.Accounts {
		if a.ID != id {
			continue
		}
		if w, ok := a.Window(claude.WeekWindow); ok {
			return prose.Percent(w.Used)
		}
	}
	return ""
}

// runningRead reads the sessions running on the account with the given id,
// as Detail's SESSIONS lists them, by their shown ids.
func runningRead(m status.Moment, am viewstest.AccountsMoment, id string) string {
	var ids []string
	for _, r := range views.RunningOn(views.RunningOf(m, views.ListLive{Running: am.Sessions, Stream: am.Stream}, views.HeldList{}), id) {
		ids = append(ids, status.ShownID(r.Session))
	}
	return strings.Join(ids, " ")
}

// latelyRead reads LATELY's sentences in full, " | " between them.
func latelyRead(sentences []views.Sentence) string {
	read := make([]string, len(sentences))
	for i, s := range sentences {
		read[i] = s.Full.String()
	}
	return strings.Join(read, " | ")
}

// At each of Accounts' hard moments, the page stood at its moment gives the
// figures wanted of it, of the page and of each row the moment names.
func TestAccountsGivesEachFigureWantedAtEachHardMoment(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, m := accountsAt(am)
			got := pageFigures(page)
			for name, want := range am.Page {
				if got[name] != want {
					t.Errorf("the page's %s is %q, want %q", name, got[name], want)
				}
			}
			for _, id := range slices.Sorted(maps.Keys(am.Want)) {
				got := rowFigures(t, am, page, m, id)
				for _, name := range slices.Sorted(maps.Keys(am.Want[id])) {
					if want := am.Want[id][name]; got[name] != want {
						t.Errorf("%q's %s is %q, want %q", id, name, got[name], want)
					}
				}
			}
		})
	}
}
