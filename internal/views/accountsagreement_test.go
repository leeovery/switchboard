package views_test

import (
	"io/fs"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
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
// its worth against it, the ledger begun part-way through the period or
// not, as both price a young ledger's plans from its first day. THIS WEEK
// on its first day is the one period whose plans history can't be asked
// for, as from its first day history prices today: its plans there are
// Accounts' TODAY's.
func TestAccountsTotalsAndPlansAreHistorysOverThePeriod(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, _ := accountsAt(am)
			history := historySince(am, am.Period.From(am.Now, am.WeekStarts))
			rows := append(slices.Clone(page.Rows), page.All)
			if len(rows) != len(history.Totals.Accounts) || len(rows) != len(history.Plans) {
				t.Fatalf("the page has %d rows, history %d totals and %d plans", len(rows), len(history.Totals.Accounts), len(history.Plans))
			}
			priced := rows
			if isThisWeeksFirstDay(am) {
				today := am
				today.Period = views.LengthToday.Now()
				page, _ := accountsAt(today)
				priced = append(slices.Clone(page.Rows), page.All)
			}
			for i, row := range rows {
				if got, want := jsonOf(t, row.Totals), jsonOf(t, history.Totals.Accounts[i]); got != want {
					t.Errorf("row %d counts\n%s\nhistory counts\n%s", i, got, want)
				}
				if got, want := jsonOf(t, priced[i].Plan), jsonOf(t, history.Plans[i]); got != want {
					t.Errorf("row %d's plan is\n%s\nhistory's\n%s", i, got, want)
				}
			}
		})
	}
}

// A young ledger's plans are priced from its first day, whatever the period
// it began in, on Accounts, history and the Tokens row of the week alike: of
// work's Ultra and side's Pro, $220 a month, from a Wednesday, this week's
// 5 days at 12/365 of a month to its end, the last 30 days' and the last 12
// weeks' the days to today and to this week's end; and from the period's
// first day, whole. A ledger that holds no day yet, as on a fresh install,
// begins today, as ledger.Begins tells the dashboard and a read of its days
// alike: this week's 3 days from the Friday.
func TestAYoungLedgersPlansArePricedFromItsFirstDayEverywhere(t *testing.T) {
	friday := on("2026-10-09").Add(15 * time.Hour)
	tests := []struct {
		name  string
		began string
		// empty is a ledger holding no day, its days those a read of it
		// gives, where it begins as ledger.Begins says.
		empty  bool
		period views.Period
		want   string
	}{
		{name: "this week, the ledger holding no day yet", began: "2026-10-09", empty: true, period: views.LengthThisWeek.Now(),
			want: "21.698630136986"},
		{name: "this week, begun on its Wednesday", began: "2026-10-07", period: views.LengthThisWeek.Now(), want: "36.164383561644"},
		{name: "this week, begun on its first day, whole", began: "2026-10-05", period: views.LengthThisWeek.Now(), want: "50.769230769231"},
		{name: "today, begun today, whole", began: "2026-10-09", period: views.LengthToday.Now(), want: "7.232876712328"},
		{name: "the last 30 days, begun on the Wednesday, its 3 days to today", began: "2026-10-07", period: views.LengthThirtyDays.Now(),
			want: "21.698630136986"},
		{name: "the last 12 weeks, begun a month before, its days to this week's end", began: "2026-09-09", period: views.LengthTwelveWeeks.Now(),
			want: "238.684931506849"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysTo(tt.began, friday, day(tt.began, account("work", sent("claude-opus-5-5", 1, inputUsage))))
			began := on(tt.began)
			if tt.empty {
				days = daysTo(tt.began, friday)
				began, _, _ = ledger.Begins(time.Time{}, fs.ErrNotExist, friday)
			}
			in := input(days)
			in.Now, in.Began = friday, began
			in.From = tt.period.From(friday, in.WeekStarts)
			got := map[string]string{
				"Accounts": jsonOf(t, views.NewPast(in, days).Accounts(tt.period).All.Plan.Cost),
				"history":  jsonOf(t, views.NewHistory(in).Plans[len(in.Accounts)].Cost),
			}
			want := map[string]string{"Accounts": tt.want, "history": tt.want}
			if tt.period == views.LengthThisWeek.Now() {
				tokens := views.NewHistory(in).Tokens
				got["Tokens"], want["Tokens"] = jsonOf(t, tokens[len(tokens)-1].Cost), tt.want
			}
			if !maps.Equal(got, want) {
				t.Errorf("the ledger begun %s, the plans cost %v, want %v", tt.began, got, want)
			}
		})
	}
}

// On a week's first day, history from today prices a day, as Accounts'
// today does, the shorter of the two periods that begin then, so a day asked
// for is never priced a week; Accounts' THIS WEEK prices the week. Their
// totals are alike.
func TestOnAWeeksFirstDayHistoryFromTodayIsAccountsTodayNotThisWeek(t *testing.T) {
	am := accountsMoment(t, onceItsReFiled)
	from := views.LengthToday.Now().From(am.Now, am.WeekStarts)
	if !from.Equal(views.LengthThisWeek.Now().From(am.Now, am.WeekStarts)) {
		t.Fatalf("on %s, today and this week begin apart", am.Now.Weekday())
	}
	history := historySince(am, from)
	cost := map[string]string{"history": jsonOf(t, history.Plans[len(history.Plans)-1].Cost)}
	for _, period := range []views.Period{views.LengthToday.Now(), views.LengthThisWeek.Now()} {
		am.Period = period
		page, _ := accountsAt(am)
		if got, want := jsonOf(t, page.All.Totals), jsonOf(t, history.Totals.Accounts[len(history.Totals.Accounts)-1]); got != want {
			t.Errorf("every account's %s counts\n%s\nhistory's\n%s", period.Length().Name(), got, want)
		}
		cost[period.Length().Name()] = jsonOf(t, page.All.Plan.Cost)
	}
	want := map[string]string{"history": "13.150684931506", "today": "13.150684931506", "this week": "92.307692307692"}
	if !maps.Equal(cost, want) {
		t.Errorf("on %s, the plans cost %v, want history from today a day, as Accounts' today, %v", am.Now.Weekday(), cost, want)
	}
}

// isThisWeeksFirstDay reports whether am is of THIS WEEK on its first day,
// whose plans history prices as today's.
func isThisWeeksFirstDay(am viewstest.AccountsMoment) bool {
	return am.Period == views.LengthThisWeek.Now() && am.Period.From(am.Now, am.WeekStarts).Equal(views.LengthToday.Now().From(am.Now, am.WeekStarts))
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
			m := am.Doc.At(am.Now, claude.Policy())
			running := views.RunningOf(m, views.ListLive{Running: am.Sessions, Stream: am.Stream}, views.HeldSessions{})
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
			m := am.Doc.At(am.Now, claude.Policy())
			told := make(map[int]bool)
			for _, s := range views.Lately(m, len(m.Events), views.SessionDirs{}) {
				told[s.ID] = true
			}
			for _, e := range m.Events {
				if e.Kind == status.EventMoved {
					told[e.ID] = true
				}
			}
			for _, id := range append(am.Accounts.IDs(), "") {
				for _, s := range views.AccountLately(m, id, len(m.Events), views.SessionDirs{}) {
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

// At each of Accounts' hard moments, Detail's WEEKS, its facts and what
// each account had left at its last reset are history's, each as it stands
// then: Accounts' page built of the days as they were read, as the watch
// builds it, and stood at the moment; history of the days the ledger holds
// then, from the first of Detail's last eight weeks. Over those eight, each
// row's peaks are history's and its facts history's verdict's, and its left
// at its last reset history's, where history gives one.
func TestDetailsWeeksAreHistorysAtTheMoment(t *testing.T) {
	const judged = 8
	for _, am := range viewstest.AccountsMoments() {
		t.Run(am.Name, func(t *testing.T) {
			page, _ := accountsAt(am)
			shown := len(page.Weeks) - judged
			if shown < 0 {
				t.Fatalf("Detail's weeks are %v, want %d at least", page.Weeks, judged)
			}
			weeks := page.Weeks[shown:]
			history := heldHistory(am, weeks[0])
			for i, row := range append(slices.Clone(page.Rows), page.All) {
				if peaks, theirs := peaksRead(row.Peaks[shown:]), peaksRead(historysPeaks(history, i, weeks)); peaks != theirs {
					t.Errorf("row %d's peaks are %s, history's %s", i, peaks, theirs)
				}
				if facts, theirs := factsRead(row.Facts(judged)), factsRead(history.Capacity.Accounts[i]); facts != theirs {
					t.Errorf("row %d's facts are %s, history's %s", i, facts, theirs)
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
