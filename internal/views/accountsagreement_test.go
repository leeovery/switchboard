package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// At each of Accounts' hard moments, the totals and plans Compare and THIS
// WEEK read off a row are history's over the same period, asked for as
// history --since asks, by its first day, every one of them, of each
// account and of every account: its requests, its limits, its sessions
// served and its worth among them, each window's last run, told from the
// days before the period too, and what its plan cost over the period and
// its worth against it. Where the ledger began part-way through the period,
// history prices the plans from its first day, as it prices the days asked
// for from before it, and Accounts the whole period: history's cost less.
func TestAccountsTotalsAndPlansAreHistorysOverThePeriod(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, _ := accountsAt(am)
			from := am.Period.From(am.Now, am.WeekStarts)
			history := historySince(am, from)
			rows := append(slices.Clone(page.Rows), page.All)
			if len(rows) != len(history.Totals.Accounts) || len(rows) != len(history.Plans) {
				t.Fatalf("the page has %d rows, history %d totals and %d plans", len(rows), len(history.Totals.Accounts), len(history.Plans))
			}
			partWay := on(am.Days[0].Day).After(from)
			for i, row := range rows {
				if got, want := jsonOf(t, row.Totals), jsonOf(t, history.Totals.Accounts[i]); got != want {
					t.Errorf("row %d counts\n%s\nhistory counts\n%s", i, got, want)
				}
				switch got, want := jsonOf(t, row.Plan), jsonOf(t, history.Plans[i]); {
				case !partWay && got != want:
					t.Errorf("row %d's plan is\n%s\nhistory's\n%s", i, got, want)
				case partWay && row.Plan.Cost != nil && (history.Plans[i].Cost == nil || *history.Plans[i].Cost >= *row.Plan.Cost):
					t.Errorf("the ledger begun part-way through the period, row %d's plan is\n%s\nhistory's\n%s\nwant history's cost less", i, got, want)
				}
			}
		})
	}
}

// On a week's first day, Accounts' today is the one period history can't be
// asked for by its first day: from it, history prices this week, the longer
// that begins then, Accounts' own to begin with, and Accounts' today a day.
// Their totals are alike.
func TestOnAWeeksFirstDayHistoryFromTodayIsThisWeekNotAccountsToday(t *testing.T) {
	am := accountsMoment(t, onceItsReFiled)
	am.Period = views.AccountsToday
	page, _ := accountsAt(am)
	history := historySince(am, am.Period.From(am.Now, am.WeekStarts))
	if got, want := jsonOf(t, page.All.Totals), jsonOf(t, history.Totals.Accounts[len(history.Totals.Accounts)-1]); got != want {
		t.Errorf("every account's today counts\n%s\nhistory's\n%s", got, want)
	}
	got := []string{jsonOf(t, page.All.Plan.Cost), jsonOf(t, history.Plans[len(history.Plans)-1].Cost)}
	if want := []string{"13.150684931506", "92.307692307692"}; !slices.Equal(got, want) {
		t.Errorf("on %s, Accounts' today and history from today cost %v, want a day and this week, %v", am.Now.Weekday(), got, want)
	}
}

// historySince is history at the moment am, asked for from the time given,
// as history --since asks for it.
func historySince(am viewstest.AccountsMoment, from time.Time) views.History {
	in := accountsIn(am)
	in.From = from
	return views.NewHistory(in)
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
			running := views.RunningOf(m, views.ListLive{Running: am.Sessions, Stream: am.Stream}, views.HeldList{})
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

// At each of Accounts' hard moments, Detail's WEEKS, its verdicts and what
// each account had left at its last reset are History's Weeks' and
// history's, each as it stands then: Accounts' page and Weeks' built of the
// days as they were read, as the watch builds them, and stood at the moment;
// history of the days the ledger holds then, from Weeks' first week. Its six
// weeks are the last six of Weeks' eight, each row's peaks and verdict
// theirs, and its left at its last reset history's, where history gives one.
func TestDetailsWeeksAreHistorysAtTheMoment(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, m := accountsAt(am)
			weeks := pastOf(am).Weeks().At(m)
			shown := len(weeks.Weeks) - len(page.Weeks)
			if shown < 0 || !slices.Equal(page.Weeks, weeks.Weeks[shown:]) {
				t.Fatalf("Detail's weeks are %v, Weeks' %v", page.Weeks, weeks.Weeks)
			}
			history := heldHistory(am, weeks.Weeks[0])
			rows := append(slices.Clone(weeks.Accounts), weeks.All)
			for i, row := range append(slices.Clone(page.Rows), page.All) {
				peaks := []string{peaksRead(row.Peaks), peaksRead(rows[i].Peaks[shown:]), peaksRead(historysPeaks(history, i, page.Weeks))}
				if !allSame(peaks) {
					t.Errorf("row %d's peaks are %s, Weeks' %s, history's %s", i, peaks[0], peaks[1], peaks[2])
				}
				verdicts := []string{verdictRead(row.Verdict), verdictRead(rows[i].AccountVerdict), verdictRead(history.Capacity.Accounts[i])}
				if !allSame(verdicts) {
					t.Errorf("row %d's verdict is %s, Weeks' %s, history's %s", i, verdicts[0], verdicts[1], verdicts[2])
				}
				if i == len(page.Rows) {
					continue
				}
				if left, ok := history.Totals.Accounts[i].LeftAtReset[claude.WeekWindow]; ok && shareRead(row.WeekLeft) != shareRead(&left) {
					t.Errorf("row %d had %s left at its last reset, history %s", i, shareRead(row.WeekLeft), shareRead(&left))
				}
			}
		})
	}
}

// heldHistory is history at the moment am, of the days the ledger holds then
// from the day with the given date.
func heldHistory(am viewstest.AccountsMoment, from string) views.History {
	in := accountsIn(am)
	if am.Held != nil {
		in.Ledger = ledgerOf(am.Held)
	}
	in.From = on(from)
	return views.NewHistory(in)
}

// historysPeaks are history's peaks of its week window of the account with
// index x among its weeks' own, or every account's, past theirs, in the
// weeks with the dates given, as WEEKS has them: unknown in a week it
// doesn't give.
func historysPeaks(h views.History, x int, dates []string) []views.WeekPeak {
	peaks := make([]views.WeekPeak, len(dates))
	for i, date := range dates {
		w := slices.IndexFunc(h.Weeks, func(w views.Week) bool { return w.Week == date })
		if w < 0 {
			continue
		}
		a := h.Weeks[w].Accounts[x]
		if share, ok := a.Peaks[claude.WeekWindow]; ok {
			peaks[i].Share = &share
		}
		peaks[i].Open = slices.Contains(a.Open, claude.WeekWindow)
	}
	return peaks
}

// allSame reports whether each of said is the first.
func allSame(said []string) bool {
	return !slices.ContainsFunc(said, func(s string) bool { return s != said[0] })
}
