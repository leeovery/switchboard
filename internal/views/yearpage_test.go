package views_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionsOf are a model's requests, its sessions those with the ids given,
// as a summary of version 4 names them.
func sessionsOf(m ledger.ModelDay, ids ...string) ledger.ModelDay {
	m.Sessions, m.SessionIDs = len(ids), ids
	return m
}

// usedBy is an account's day of the models given, its sessions theirs, each
// once.
func usedBy(id string, models ...ledger.ModelDay) ledger.AccountDay {
	a := account(id, models...)
	for _, m := range models {
		a.SessionIDs = append(a.SessionIDs, m.SessionIDs...)
	}
	slices.Sort(a.SessionIDs)
	a.SessionIDs = slices.Compact(a.SessionIDs)
	a.Sessions = len(a.SessionIDs)
	return a
}

// withLimits is the account's day, on which it reached the limits of the
// windows given.
func withLimits(a ledger.AccountDay, windows ...string) ledger.AccountDay {
	for _, w := range windows {
		a.Limits = append(a.Limits, ledger.Limit{Window: w, At: now})
	}
	return a
}

// overTheYear are the days from Monday 6 October 2025, the grid's first, to
// today, Wednesday 7 October 2026, the last 12 months from the 8th:
//   - the 6th, before them, work's 900 requests of Opus 5;
//   - the 8th, work's Opus 5, in session A, and its 5-hour limit;
//   - 2 March, of a summary of version 3, which names its accounts' sessions
//     but not its models': work's Opus 5.5, of sessions B and C, and side's
//     Sonnet 5.5, of D;
//   - yesterday, work's Opus 5.5, of B and E, and both its limits, and
//     side's Haiku 4.5, of D;
//   - today, work's Opus 5.5, of E, and a request no account answered.
var overTheYear = daysFrom("2025-10-06",
	day("2025-10-06", usedBy("work", sessionsOf(sent("claude-opus-5", 900, inputUsage), sessionA))),
	day("2025-10-08", withLimits(usedBy("work", sessionsOf(sent("claude-opus-5", 10, inputUsage), sessionA)), "5h")),
	ledger.Summary{Version: 3, Day: "2026-03-02", Accounts: []ledger.AccountDay{
		{Account: "side", Models: []ledger.ModelDay{{Model: "claude-sonnet-5-5", Upstream: 4, Sessions: 1, Usage: []byte(inputUsage)}}, Sessions: 1, SessionIDs: []string{sessionD}},
		{Account: "work", Models: []ledger.ModelDay{{Model: "claude-opus-5-5", Upstream: 2, Sessions: 2, Usage: []byte(inputUsage)}}, Sessions: 2, SessionIDs: []string{sessionB, sessionC}},
	}},
	day("2026-10-06",
		usedBy("side", sessionsOf(sent("claude-haiku-4-5", 1, inputUsage), sessionD)),
		withLimits(usedBy("work", sessionsOf(sent("claude-opus-5-5", 3, inputUsage), sessionB, sessionE)), "5h", "7d")),
	day("2026-10-07",
		account("", ledger.ModelDay{Model: "claude-opus-5-5", Unsent: 1}),
		usedBy("work", sessionsOf(sent("claude-opus-5-5", 2, inputUsage), sessionE))),
)

// yearOf is the Year's page asked for of the days given, at now.
func yearOf(days []ledger.Summary, ask views.YearAsk) views.YearPage {
	return views.NewPast(input(days), days).Year(ask)
}

// gridOf is the grid's weeks, a string each, a day a character: its level,
// or - where it's left out.
func gridOf(weeks []views.GridWeek) []string {
	grid := make([]string, len(weeks))
	for i, w := range weeks {
		var b strings.Builder
		for _, d := range w.Days {
			if d.LeftOut {
				b.WriteByte('-')
				continue
			}
			fmt.Fprint(&b, d.Level)
		}
		grid[i] = w.First + " " + b.String()
	}
	return grid
}

// The grid is the 53 weeks to this week, from the first day of the week a
// year back: the days to come this week left out, and those of the first
// week before the 12 months drawn at their levels, though the cuts are the
// 12 months' active days' alone. A day of no requests is at level 0.
func TestTheYearsGridIsTheFiftyThreeWeeksToThisWeek(t *testing.T) {
	page := yearOf(overTheYear, views.YearAsk{})

	grid := gridOf(page.Weeks)
	if len(grid) != 53 {
		t.Fatalf("the grid has %d weeks, want 53", len(grid))
	}
	// Of 10, 6, 4 and 3 requests, the cuts are those at 1, 2 and 3: 4, 6
	// and 10; 900, the 6th's, is beyond them all.
	if want := []int{4, 6, 10}; !slices.Equal(page.Cuts, want) {
		t.Errorf("the cuts are %v, want %v, the 12 months' alone", page.Cuts, want)
	}
	for _, want := range []string{"2025-10-06 4040000", "2026-03-02 3000000", "2026-09-28 0000000", "2026-10-05 021----"} {
		if !slices.Contains(grid, want) {
			t.Errorf("the grid holds no week %q:\n%s", want, strings.Join(grid, "\n"))
		}
	}
	if first := page.Weeks[0].Days[0]; first.Requests != 900 {
		t.Errorf("the grid's first day has %d requests, want 900, drawn at its level", first.Requests)
	}
}

// A month's name sits over the first week holding 4 or more of its days,
// days still to come among them, today's month the current one: as the
// frames' Thursday 1 October 2026 has them, Oct at 14 to Oct at 118, two
// cells a week from 14.
func TestEachMonthsNameSitsOverTheFirstWeekHoldingFourOfItsDays(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 42, 7, 0, time.Local)
	days := daysTo("2025-09-29", at)
	in := input(days)
	in.Now = at
	page := views.NewPast(in, days).Year(views.YearAsk{})

	var got []string
	for _, l := range page.Months {
		label := fmt.Sprintf("%s@%d", l.Month, 14+2*l.Week)
		if l.Current {
			label += " current"
		}
		got = append(got, label)
	}
	want := []string{
		"2025-10@14", "2025-11@24", "2025-12@32", "2026-01@40", "2026-02@50", "2026-03@58", "2026-04@66",
		"2026-05@76", "2026-06@84", "2026-07@92", "2026-08@102", "2026-09@110", "2026-10@118 current",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the months' names sit at\n%v\nwant\n%v", got, want)
	}
}

// A calendar year's grid runs from the week of 1 January to the week of 31
// December, the days outside it left out, and its numbers count its days
// alone, against its plans priced to its end, though it's still running,
// from the day the ledger began where it began in the year.
func TestACalendarYearsGridIsItsWeeksAndItsNumbersItsDays(t *testing.T) {
	// 1 January 2025 is a Wednesday, and 31 December too; 1 January 2026 is
	// a Thursday.
	sinceLastYear := daysFrom("2024-12-30", busy("2024-12-31", 9), busy("2025-01-01", 7), busy("2025-12-31", 5), busy("2026-01-02", 2))
	tests := []struct {
		name        string
		days        []ledger.Summary
		year        int
		first, last string
		months      string
		requests    int
		cost        string
	}{
		{
			name: "last year", days: sinceLastYear, year: 2025,
			first: "2024-12-30 --40000", last: "2025-12-29 002----", months: "2025-01@0 … 2025-12@48", requests: 12, cost: "2640",
		},
		{
			name: "this year, so far", days: sinceLastYear, year: 2026,
			first: "2025-12-29 ---0400", last: "2026-12-28 -------", months: "2026-01@0 … 2026-12@48", requests: 2, cost: "2640",
		},
		{
			// The days before the ledger began are empty squares, like any
			// other; its plans cost from 6 October to 31 December, 87 ×
			// 12/365 of $220.
			name: "the year the ledger began in", days: overTheYear, year: 2025,
			first: "2024-12-30 --00000", last: "2025-12-29 000----", months: "2025-01@0 … 2025-12@48", requests: 910, cost: "629.260273972603",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := yearOf(tt.days, views.YearAsk{Year: tt.year})

			grid := gridOf(page.Weeks)
			if len(grid) != 53 || grid[0] != tt.first || grid[len(grid)-1] != tt.last {
				t.Errorf("the grid's %d weeks run %q to %q, want 53, %q to %q", len(grid), grid[0], grid[len(grid)-1], tt.first, tt.last)
			}
			first, last := page.Months[0], page.Months[len(page.Months)-1]
			if got := fmt.Sprintf("%s@%d … %s@%d", first.Month, first.Week, last.Month, last.Week); got != tt.months {
				t.Errorf("the months' names run %s, want %s", got, tt.months)
			}
			if page.Numbers.Requests != tt.requests {
				t.Errorf("the year's requests are %d, want %d", page.Numbers.Requests, tt.requests)
			}
			if got := jsonOf(t, page.Numbers.Cost); got != tt.cost {
				t.Errorf("the year's plans cost %s, want %s", got, tt.cost)
			}
		})
	}
}

// The numbers count the 12 months alone, every account's: each session once,
// however many days and accounts it ran on; the limits by window; the worth
// against the plans of the accounts on one known, priced a year; the
// busiest day and the longest run.
func TestTheYearsNumbersCountTheTwelveMonths(t *testing.T) {
	tests := []struct {
		name string
		ask  views.YearAsk
		want string
	}{
		{
			// $5 + $6 + $5 + $4 of 23 requests, against work's $200 and
			// side's $20 a month, over the year's 365 days.
			name: "all use",
			want: `{"Requests":23,"Sessions":5,"Limits":{"5h":2,"7d":1},"worth":20,"cost":2640,"against":0.007575757575757576,` +
				`"BusiestDay":"2025-10-08","LongestRun":2,"Share":null,"FirstUsed":""}`,
		},
		{
			// Work's own, against its own plan.
			name: "an account's alone", ask: views.YearAsk{Account: "work"},
			want: `{"Requests":17,"Sessions":4,"Limits":{"5h":2,"7d":1},"worth":17,"cost":2400,"against":0.007083333333333333,` +
				`"BusiestDay":"2025-10-08","LongestRun":2,"Share":null,"FirstUsed":""}`,
		},
		{
			// Opus 5.5's: its sessions B and E counted once, and 2 March's
			// two, of a summary that names no model's, added up, as they
			// can't be told apart; $12 of the year's $20; no limits, which
			// are an account's, and no plans.
			name: "a version's alone", ask: views.YearAsk{Version: "Opus 5.5"},
			want: `{"Requests":8,"Sessions":4,"Limits":null,"worth":12,"BusiestDay":"2026-10-06","LongestRun":2,"Share":0.6,"FirstUsed":"2026-03-02"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jsonOf(t, yearOf(overTheYear, tt.ask).Numbers); got != tt.want {
				t.Errorf("the numbers are\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// Filtered, the grid shades what's shown alone, its cuts re-scaled to its
// active days.
func TestAFilteredGridShadesWhatsShownAlone(t *testing.T) {
	tests := []struct {
		name  string
		ask   views.YearAsk
		cuts  []int
		weeks []string
	}{
		{name: "a version", ask: views.YearAsk{Version: "Opus 5.5"}, cuts: []int{2, 3, 3}, weeks: []string{"2025-10-06 0000000", "2026-03-02 2000000", "2026-10-05 044----"}},
		{name: "an account", ask: views.YearAsk{Account: "side"}, cuts: []int{1, 4, 4}, weeks: []string{"2025-10-06 0000000", "2026-03-02 4000000", "2026-10-05 020----"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := yearOf(overTheYear, tt.ask)
			if !slices.Equal(page.Cuts, tt.cuts) {
				t.Errorf("the cuts are %v, want %v", page.Cuts, tt.cuts)
			}
			grid := gridOf(page.Weeks)
			for _, want := range tt.weeks {
				if !slices.Contains(grid, want) {
					t.Errorf("the grid holds no week %q", want)
				}
			}
		})
	}
}

// BY MONTH is a row a month, the first of the 12 months priced from their
// first day, the month the ledger began recording in from then, and the
// month so far marked so.
func TestByMonthMarksTheMonthTheLedgerBeganAndTheMonthSoFar(t *testing.T) {
	tests := []struct {
		name string
		days []ledger.Summary
		want []string
	}{
		{
			// The 12 months begin on 8 October 2025: its 24 days of the
			// plans' $220 a month cost 24 × 12/365 of it.
			name: "a ledger from before the 12 months",
			days: overTheYear,
			want: []string{"2025-10 cost 173.58904109589 from  so far false", "2026-10 cost 220 from  so far true"},
		},
		{
			name: "a ledger begun on 15 September",
			days: daysFrom("2026-09-15", busy("2026-09-15", 3)),
			want: []string{"2026-09 cost 220 from 2026-09-15 so far false", "2026-10 cost 220 from  so far true"},
		},
		{
			name: "a ledger begun on the first of a month",
			days: daysFrom("2026-09-01", busy("2026-09-01", 3)),
			want: []string{"2026-09 cost 220 from  so far false", "2026-10 cost 220 from  so far true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := yearOf(tt.days, views.YearAsk{}).ByMonth
			got := []string{monthRowOf(t, rows[0]), monthRowOf(t, rows[len(rows)-1])}
			if !slices.Equal(got, tt.want) {
				t.Errorf("BY MONTH's first and last rows are\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// monthRowOf says what a test asks of a BY MONTH row: its month, its
// plans' cost, from when it's counted and whether it's the month so far.
func monthRowOf(t *testing.T, row views.MonthRow) string {
	t.Helper()
	return fmt.Sprintf("%s cost %s from %s so far %t", row.Month.Month, jsonOf(t, row.Cost), row.From, row.SoFar)
}

// Filtered to a version, BY MONTH is a row a month it was used in, with its
// share of the month's worth, the month it was first used in from then; and
// a row before them for the months before it, naming the version of its
// family used most then.
func TestAVersionsMonthsAreThoseItWasUsedIn(t *testing.T) {
	tests := []struct {
		name    string
		version string
		rows    string
		before  string
	}{
		{
			name: "Opus 5.5, after Opus 5", version: "Opus 5.5",
			rows: `[{"month":"2026-03","requests":2,"worth":4,"by_family":[{"family":"opus","worth":4}],"Share":0.6666666666666666,"From":"2026-03-02","SoFar":false},` +
				`{"month":"2026-10","requests":6,"worth":8,"by_family":[{"family":"opus","worth":8}],"Share":0.8888888888888888,"From":"","SoFar":true}]`,
			before: `{"From":"2025-10","To":"2026-02","Then":"Opus 5"}`,
		},
		{
			name: "Sonnet 5.5, with no other Sonnet before it", version: "Sonnet 5.5",
			rows:   `[{"month":"2026-03","requests":4,"worth":2,"by_family":[{"family":"sonnet","worth":2}],"Share":0.3333333333333333,"From":"2026-03-02","SoFar":false}]`,
			before: `{"From":"2025-10","To":"2026-02","Then":""}`,
		},
		{
			name: "Opus 5, used from the first month", version: "Opus 5",
			rows:   `[{"month":"2025-10","requests":10,"worth":5,"by_family":[{"family":"opus","worth":5}],"Share":1,"From":"","SoFar":false}]`,
			before: `null`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := yearOf(overTheYear, views.YearAsk{Version: tt.version})
			if got := jsonOf(t, page.ByMonth); got != tt.rows {
				t.Errorf("BY MONTH is\n%s\nwant\n%s", got, tt.rows)
			}
			if got := jsonOf(t, page.Before); got != tt.before {
				t.Errorf("the row before is %s, want %s", got, tt.before)
			}
			if page.Families != nil {
				t.Errorf("the families are %v, want none, a version's bars being its own", page.Families)
			}
		})
	}
}

// The version shown is as the version table names it, its shade with it; or,
// one the table doesn't name, by its name and its family alone.
func TestTheVersionShownIsTheTablesWithItsShade(t *testing.T) {
	days := daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage), sent("claude-opus-9", 1, inputUsage))))
	tests := []struct {
		version string
		want    ledger.Version
	}{
		{version: "Opus 5.5", want: ledger.Version{Name: "Opus 5.5", Family: "opus", Shade: 1}},
		{version: "opus-9", want: ledger.Version{Name: "opus-9", Family: "opus"}},
	}
	for _, tt := range tests {
		got := yearOf(days, views.YearAsk{Version: tt.version}).Version
		if got.Name != tt.want.Name || got.Family != tt.want.Family || got.Shade != tt.want.Shade {
			t.Errorf("the version shown is %+v, want %+v", got, tt.want)
		}
	}
}

// v cycles through the versions used in the 12 months, in the version
// table's order, those it doesn't name after them, and the configured
// accounts, in the config's order: none of one alone.
func TestVCyclesTheVersionsUsedAndTheAccounts(t *testing.T) {
	days := daysFrom("2026-10-06",
		day("2026-10-06", account("work", sent("claude-haiku-4-5", 1, inputUsage), sent("claude-opus-9", 1, inputUsage))),
		day("2026-10-07", account("side", sent("claude-sonnet-5-5", 1, inputUsage), ledger.ModelDay{Model: "claude-fable-5-1", Checks: 3}),
			account("work", sent("claude-opus-5-5", 1, inputUsage), sent("claude-opus-4-5-20251101", 1, inputUsage), sent("claude-opus-4-5", 1, inputUsage))))

	page := yearOf(days, views.YearAsk{})
	if want := []string{"Opus 5.5", "Opus 4.5", "Sonnet 5.5", "Haiku 4.5", "opus-9"}; !slices.Equal(page.Versions, want) {
		t.Errorf("v cycles the versions %v, want %v", page.Versions, want)
	}
	if want := []string{"work", "side", "spare"}; !slices.Equal(page.Accounts, want) {
		t.Errorf("v cycles the accounts %v, want %v", page.Accounts, want)
	}
	in := input(days)
	in.Accounts = config.Accounts{{ID: "work", Plan: "max20x"}}
	if alone := views.NewPast(in, days).Year(views.YearAsk{}); alone.Accounts != nil {
		t.Errorf("v cycles the accounts %v of one alone, want none", alone.Accounts)
	}
}

// BY MONTH's bars are split by the families whose requests were worth
// anything in its months, in the version table's order.
func TestTheBarsFamiliesAreThoseOfAnyWorth(t *testing.T) {
	days := daysFrom("2026-09-30",
		day("2026-09-30", account("work", sent("claude-haiku-4-5", 1, inputUsage))),
		day("2026-10-07", account("work", sent("claude-sonnet-5-5", 1, inputUsage), sent("claude-opus-5-5", 1, inputUsage), sent("claude-fable-5-1", 1, ""))))

	if got, want := yearOf(days, views.YearAsk{}).Families, []string{"opus", "sonnet", "haiku"}; !slices.Equal(got, want) {
		t.Errorf("the families are %v, want %v", got, want)
	}
}

// The key rounds each cut to the nearest 50, but one under 50, which it
// keeps as it is.
func TestTheKeyRoundsTheCutsToFifty(t *testing.T) {
	days := daysFrom("2026-10-01", busy("2026-10-01", 30), busy("2026-10-02", 449), busy("2026-10-03", 475), busy("2026-10-04", 1020))
	page := yearOf(days, views.YearAsk{})
	if want := []int{449, 475, 1020}; !slices.Equal(page.Cuts, want) {
		t.Fatalf("the cuts are %v, want %v", page.Cuts, want)
	}
	if want := []int{450, 500, 1000}; !slices.Equal(page.KeyCuts, want) {
		t.Errorf("the key's cuts are %v, want %v", page.KeyCuts, want)
	}
	small := yearOf(daysFrom("2026-10-06", busy("2026-10-06", 3), busy("2026-10-07", 24)), views.YearAsk{})
	if want := []int{3, 24, 24}; !slices.Equal(small.KeyCuts, want) {
		t.Errorf("the key's cuts are %v, want %v, those under 50 as they are", small.KeyCuts, want)
	}
}

// An empty ledger draws the grid at level 0, with no cuts and no numbers.
func TestAnEmptyLedgersYearIsAllLevelZero(t *testing.T) {
	page := yearOf(daysFrom("2026-10-07"), views.YearAsk{})
	for _, w := range page.Weeks {
		for _, d := range w.Days {
			if d.Level != 0 {
				t.Fatalf("%s is at level %d, want 0", d.Day, d.Level)
			}
		}
	}
	if len(page.Cuts) != 0 || page.Numbers.Requests != 0 || page.Numbers.Amount != nil {
		t.Errorf("the cuts are %v, and the numbers %+v, want none", page.Cuts, page.Numbers)
	}
}

// The Year's days are read from its grid's first day: the first of the week a
// year back, or of the week of a calendar year's 1 January.
func TestTheYearIsReadFromItsGridsFirstDay(t *testing.T) {
	tests := []struct {
		name   string
		ask    views.YearAsk
		starts time.Weekday
		want   string
	}{
		{name: "the last 12 months", starts: time.Monday, want: "2025-10-06"},
		{name: "weeks starting on Sunday", starts: time.Sunday, want: "2025-10-05"},
		{name: "a calendar year", ask: views.YearAsk{Year: 2025}, starts: time.Monday, want: "2024-12-30"},
	}
	for _, tt := range tests {
		if got, want := views.YearFrom(tt.ask, now, tt.starts), on(tt.want); !got.Equal(want) {
			t.Errorf("%s: read from %v, want %v", tt.name, got, want)
		}
	}
}
