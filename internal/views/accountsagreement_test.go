package views_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// At each of Accounts' hard moments, the totals and plans Compare and THIS
// WEEK read off a row are history's over the same period, every one of
// them, of each account and of every account: its requests, its limits, its
// sessions served and its worth among them, each window's last run, told
// from the days before the period too, and what its plan cost over the
// period and its worth against it.
func TestAccountsTotalsAndPlansAreHistorysOverThePeriod(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, _ := accountsAt(am)
			in := accountsIn(am)
			in.From, in.Period = am.Period.From(am.Now, am.WeekStarts), &am.Period
			history := views.NewHistory(in)
			rows := append(slices.Clone(page.Rows), page.All)
			if len(rows) != len(history.Totals.Accounts) || len(rows) != len(history.Plans) {
				t.Fatalf("the page has %d rows, history %d totals and %d plans", len(rows), len(history.Totals.Accounts), len(history.Plans))
			}
			for i, row := range rows {
				if got, want := jsonOf(t, row.Totals), jsonOf(t, history.Totals.Accounts[i]); got != want {
					t.Errorf("row %d counts\n%s\nhistory counts\n%s", i, got, want)
				}
				if got, want := jsonOf(t, row.Plan), jsonOf(t, history.Plans[i]); got != want {
					t.Errorf("row %d's plan is\n%s\nhistory's\n%s", i, got, want)
				}
			}
		})
	}
}

// Every account's sessions served count each session once, however many
// accounts it ran on, so they're never more than the accounts' summed.
func TestEveryAccountsSessionsServedAreNoMoreThanTheAccountsSummed(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, _ := accountsAt(am)
			sum := 0
			for _, row := range page.Rows {
				sum += row.Totals.Sessions
			}
			if page.All.Totals.Sessions > sum {
				t.Errorf("every account served %d sessions, more than the accounts' %d summed", page.All.Totals.Sessions, sum)
			}
		})
	}
}

// Detail's SESSIONS of each account are the rows RunningOf lists on it, in
// its order, as many as ROUTING counts on it, and together every row
// RunningOf lists.
func TestDetailsSessionsAreRunningOfsOnTheAccountAndROUTINGsCount(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			m := am.Doc.At(am.Now, claude.Policy)
			running := views.RunningOf(m, am.Sessions, am.Stream)
			counts := views.RunningCounts(running)
			var all []views.Running
			for _, a := range m.Accounts {
				on := views.RunningOn(running, a.ID)
				if len(on) != counts[a.ID] {
					t.Errorf("SESSIONS lists %d sessions on %s, ROUTING counts %d", len(on), a.ID, counts[a.ID])
				}
				all = append(all, on...)
			}
			if !slices.EqualFunc(all, running, func(a, b views.Running) bool { return a.Account == b.Account && a.Session == b.Session }) {
				t.Errorf("SESSIONS lists %v over the accounts, want RunningOf's %v", all, running)
			}
		})
	}
}

// Detail's LATELY, of each account and of every account, tells only events
// of the document's that the Overview's LATELY tells of, a move among them
// whether or not it's folded there, and never one that names no account.
func TestDetailsLATELYTellsOnlyEventsLATELYTells(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			m := am.Doc.At(am.Now, claude.Policy)
			told := make(map[int]bool)
			for _, s := range views.Lately(m, len(m.Events)) {
				told[s.ID] = true
			}
			for _, e := range m.Events {
				if e.Kind == status.EventMoved {
					told[e.ID] = true
				}
			}
			for _, id := range append(am.Accounts.IDs(), "") {
				for _, s := range views.AccountLately(m, id, len(m.Events)) {
					if !told[s.ID] {
						t.Errorf("%q's LATELY tells event %d, %q, which LATELY doesn't", id, s.ID, s.Full)
					}
					if slices.Contains([]string{status.EventRestart, status.EventHealth, status.EventAuto}, s.Kind) {
						t.Errorf("%q's LATELY tells %q, of an event that names no account", id, s.Full)
					}
				}
			}
		})
	}
}

// Detail's WEEKS and verdict are Weeks', of History, over the same days, as
// the page is built: its six weeks the last six of Weeks' eight, each row's
// peaks and verdict theirs.
func TestDetailsWeeksAreHistoryWeeksLastSix(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			past := views.NewPast(accountsIn(am), am.Days)
			built, weeks := past.Accounts(am.Period), past.Weeks()
			shown := len(weeks.Weeks) - len(built.Weeks)
			if shown < 0 || !slices.Equal(built.Weeks, weeks.Weeks[shown:]) {
				t.Fatalf("Detail's weeks are %v, Weeks' %v", built.Weeks, weeks.Weeks)
			}
			rows := append(slices.Clone(weeks.Accounts), weeks.All)
			for i, row := range append(slices.Clone(built.Rows), built.All) {
				if got, want := peaksRead(row.Peaks), peaksRead(rows[i].Peaks[shown:]); got != want {
					t.Errorf("row %d's peaks are %s, Weeks' %s", i, got, want)
				}
				if got, want := verdictRead(row.Verdict), verdictRead(rows[i].AccountVerdict); got != want {
					t.Errorf("row %d's verdict is %s, Weeks' %s", i, got, want)
				}
			}
		})
	}
}
