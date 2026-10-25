package views_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// worked is the summary of the day with the given date as version 6 writes
// it: work's requests of Claude Opus 5.5 upstream, by the local hour, as
// given.
func worked(date string, hours map[int]int) ledger.Summary {
	return ledger.Summary{Version: 6, Day: date, Accounts: []ledger.AccountDay{account("work", byHour(hours))}}
}

// byHour is a model's day of Claude Opus 5.5 whose requests upstream came in
// the local hours given, as many in each as given.
func byHour(hours map[int]int) ledger.ModelDay {
	var counted [24]int
	upstream := 0
	for hour, n := range hours {
		counted[hour] = n
		upstream += n
	}
	return ledger.ModelDay{Model: "claude-opus-5-5", Upstream: upstream, Hours: &counted}
}

// unknown is the summary of the day with the given date as version 5 left
// it, its lines gone: requests upstream, never counted by the hour.
func unknown(date string) ledger.Summary {
	return ledger.Summary{Version: 5, Day: date, Accounts: []ledger.AccountDay{account("work", sent("claude-opus-5-5", 5, ""))}}
}

// hoursOf is WHEN YOU WORK's page of the days given, at now, weeks starting
// on the day given.
func hoursOf(days []ledger.Summary, weekStarts time.Weekday) views.HoursPage {
	in := input(days)
	in.WeekStarts = weekStarts
	return views.NewPast(in, days).Hours()
}

// rowOn is the page's row of the weekday given.
func rowOn(t *testing.T, page views.HoursPage, weekday time.Weekday) views.HoursDay {
	t.Helper()
	i := slices.IndexFunc(page.Days, func(d views.HoursDay) bool { return d.Weekday == weekday })
	if i < 0 {
		t.Fatalf("the page has no row of %v", weekday)
	}
	return page.Days[i]
}

// The grid counts the 28 days ending yesterday, never today nor a day
// before them, each weekday's hours divided by its days known; and says the
// whole weeks those it counts span, to the last of them.
func TestTheHoursCountTheWeeksBeforeToday(t *testing.T) {
	tests := []struct {
		name        string
		days        []ledger.Summary
		weeks       int
		description string
		mondays     int
		monday9     float64
		wednesdays  int
		wednesday14 float64
	}{
		{
			name: "four whole weeks known",
			days: daysFrom("2026-09-01",
				worked("2026-09-07", map[int]int{9: 100}), worked("2026-09-08", map[int]int{9: 100}),
				worked("2026-09-09", map[int]int{14: 8}),
				worked("2026-09-14", map[int]int{9: 8}), worked("2026-09-21", map[int]int{9: 8}),
				worked("2026-09-28", map[int]int{9: 8}), worked("2026-10-05", map[int]int{9: 8}),
				worked("2026-10-07", map[int]int{9: 100})),
			weeks: 4, description: "the 4 weeks to Tue 6 Oct", mondays: 4, monday9: 8, wednesdays: 4, wednesday14: 2,
		},
		{
			name: "two weeks known, the ledger begun within them",
			days: daysFrom("2026-09-23",
				worked("2026-09-28", map[int]int{9: 8}), worked("2026-10-05", map[int]int{9: 4})),
			weeks: 2, description: "the 2 weeks to Tue 6 Oct", mondays: 2, monday9: 6, wednesdays: 2,
		},
		{
			name:  "ten days known, a part week counted whole",
			days:  daysFrom("2026-09-27", worked("2026-09-28", map[int]int{9: 8}), worked("2026-10-05", map[int]int{9: 4})),
			weeks: 2, description: "the 2 weeks to Tue 6 Oct", mondays: 2, monday9: 6, wednesdays: 1,
		},
		{
			name: "scattered days unknown, narrowing their weekdays' divisors alone",
			days: daysFrom("2026-09-01",
				unknown("2026-09-09"), unknown("2026-09-14"), worked("2026-09-21", map[int]int{9: 8}),
				unknown("2026-09-28"), worked("2026-10-05", map[int]int{9: 4})),
			weeks: 4, description: "the 4 weeks to Tue 6 Oct", mondays: 2, monday9: 6, wednesdays: 3,
		},
		{
			name: "yesterday unknown, said to the last day counted",
			days: daysFrom("2026-09-01",
				worked("2026-09-14", map[int]int{9: 8}), worked("2026-10-05", map[int]int{9: 4}), unknown("2026-10-06")),
			weeks: 4, description: "the 4 weeks to Mon 5 Oct", mondays: 4, monday9: 3, wednesdays: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := hoursOf(tt.days, time.Monday)
			if page.Weeks != tt.weeks || page.Description != tt.description {
				t.Errorf("the weeks are %d, %q, want %d, %q", page.Weeks, page.Description, tt.weeks, tt.description)
			}
			monday := rowOn(t, page, time.Monday)
			if monday.Known != tt.mondays || monday.Hours[9].Requests != tt.monday9 {
				t.Errorf("Monday is known on %d days, 09:00 at %v, want %d, %v", monday.Known, monday.Hours[9].Requests, tt.mondays, tt.monday9)
			}
			wednesday := rowOn(t, page, time.Wednesday)
			if wednesday.Known != tt.wednesdays || wednesday.Hours[14].Requests != tt.wednesday14 {
				t.Errorf("Wednesday, the first day's weekday, is known on %d days, 14:00 at %v, want %d, %v",
					wednesday.Known, wednesday.Hours[14].Requests, tt.wednesdays, tt.wednesday14)
			}
			for _, weekday := range []time.Weekday{time.Tuesday, time.Wednesday} {
				if got := rowOn(t, page, weekday).Hours[9].Requests; got != 0 {
					t.Errorf("%v's 09:00 is at %v, want 0: today and the days before the four weeks never count", weekday, got)
				}
			}
		})
	}
}

// The grid counts from the day the ledger began, where that's within the
// four weeks: a day held before it is never a known day of no requests.
func TestTheHoursCountFromTheDayTheLedgerBegan(t *testing.T) {
	tests := []struct {
		began       string
		weeks       int
		description string
		mondays     int
		monday9     float64
	}{
		{began: "2026-09-23", weeks: 2, description: "the 2 weeks to Tue 6 Oct", mondays: 2, monday9: 4},
		{began: "2026-08-01", weeks: 4, description: "the 4 weeks to Tue 6 Oct", mondays: 4, monday9: 2},
	}
	for _, tt := range tests {
		t.Run(tt.began, func(t *testing.T) {
			days := daysFrom("2026-09-01", worked("2026-09-28", map[int]int{9: 8}))
			in := input(days)
			in.Began = on(tt.began)
			page := views.NewPast(in, days).Hours()
			if page.Weeks != tt.weeks || page.Description != tt.description {
				t.Errorf("the weeks are %d, %q, want %d, %q", page.Weeks, page.Description, tt.weeks, tt.description)
			}
			monday := rowOn(t, page, time.Monday)
			if monday.Known != tt.mondays || monday.Hours[9].Requests != tt.monday9 {
				t.Errorf("Monday is known on %d days, 09:00 at %v, want %d, %v", monday.Known, monday.Hours[9].Requests, tt.mondays, tt.monday9)
			}
		})
	}
}

// Where no day of the four weeks is known, the section has no grid: the
// page is empty.
func TestNoDayKnownIsNoGrid(t *testing.T) {
	everyDayUnknown := daysFrom("2026-09-09")
	for i, d := range everyDayUnknown[:len(everyDayUnknown)-1] {
		everyDayUnknown[i] = unknown(d.Day)
	}
	tests := []struct {
		name string
		days []ledger.Summary
	}{
		{name: "the ledger begun today", days: daysFrom("2026-10-07", worked("2026-10-07", map[int]int{9: 30}))},
		{name: "every day unknown", days: everyDayUnknown},
		{name: "no days", days: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if page := hoursOf(tt.days, time.Monday); !reflect.DeepEqual(page, views.HoursPage{}) {
				t.Errorf("the page is %+v, want none", page)
			}
		})
	}
}

// Where days are known but no hour is active, the grid has no cuts and its
// key none to give: both empty, alike.
func TestQuietWeeksHaveNoCuts(t *testing.T) {
	page := hoursOf(daysFrom("2026-09-01"), time.Monday)
	if page.Weeks != 4 || len(page.Days) != 7 {
		t.Fatalf("the page is of %d weeks and %d rows, want 4 and 7", page.Weeks, len(page.Days))
	}
	if page.Cuts == nil || len(page.Cuts) != 0 || page.KeyCuts == nil || len(page.KeyCuts) != 0 {
		t.Errorf("the cuts are %#v and the key's %#v, want both empty", page.Cuts, page.KeyCuts)
	}
}

// The rows run a weekday each, from the first day of the week, as
// week_starts begins weeks.
func TestTheHoursRowsStartTheWeekAsWeekStartsSays(t *testing.T) {
	tests := []struct {
		weekStarts time.Weekday
		want       []string
	}{
		{weekStarts: time.Monday, want: []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}},
		{weekStarts: time.Sunday, want: []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}},
	}
	for _, tt := range tests {
		t.Run(tt.weekStarts.String(), func(t *testing.T) {
			page := hoursOf(daysFrom("2026-09-01", worked("2026-09-14", map[int]int{9: 8})), tt.weekStarts)
			var names []string
			for i, d := range page.Days {
				names = append(names, d.Name)
				if want := (tt.weekStarts + time.Weekday(i)) % 7; d.Weekday != want {
					t.Errorf("row %d is %v, want %v", i, d.Weekday, want)
				}
			}
			if !slices.Equal(names, tt.want) {
				t.Errorf("the rows are %v, want %v", names, tt.want)
			}
		})
	}
}

// The hours are the summaries', as the clocks gave them: an hour they
// skipped holds none, and one they repeated counts both.
func TestTheHoursAreTheSummariesAcrossAClockChange(t *testing.T) {
	page := hoursOf(daysFrom("2026-09-01",
		worked("2026-09-20", map[int]int{0: 4, 3: 4}),
		worked("2026-09-27", map[int]int{1: 8}),
	), time.Monday)
	sunday := rowOn(t, page, time.Sunday)
	if got := sunday.Hours[2]; got.Requests != 0 || got.Level != 0 {
		t.Errorf("the skipped hour is %+v, want none", got)
	}
	if got := sunday.Hours[1].Requests; got != 2 {
		t.Errorf("the repeated hour is at %v a day, want 2: both its instants' 8 over 4 Sundays", got)
	}
}

// The cuts are of each hour's requests a day, so a weekday known on fewer
// days is shaded by its own average, never by its sum, as if it had four.
func TestAWeekdayKnownOnFewerDaysIsShadedByItsAverage(t *testing.T) {
	page := hoursOf(daysFrom("2026-09-01",
		worked("2026-09-14", map[int]int{9: 4, 10: 1}), worked("2026-09-21", map[int]int{9: 4, 10: 1}),
		worked("2026-09-28", map[int]int{9: 4, 10: 1}), worked("2026-10-05", map[int]int{9: 4, 10: 1}),
		unknown("2026-09-15"), unknown("2026-09-22"), unknown("2026-09-29"), worked("2026-10-06", map[int]int{9: 4}),
	), time.Monday)
	if want := []float64{1, 4, 4}; !slices.Equal(page.Cuts, want) {
		t.Fatalf("the cuts are %v, want %v", page.Cuts, want)
	}
	monday, tuesday := rowOn(t, page, time.Monday), rowOn(t, page, time.Tuesday)
	if tuesday.Known != 1 || tuesday.Hours[9] != monday.Hours[9] {
		t.Errorf("Tuesday, known on %d days, has 09:00 at %+v, want Monday's %+v", tuesday.Known, tuesday.Hours[9], monday.Hours[9])
	}
	if got := monday.Hours[10].Level; got != 2 {
		t.Errorf("Monday's 10:00 is at level %d, want 2", got)
	}
}

// A day counts where every model's hours are known and sum to the requests
// it sent upstream, or where it sent none; else it's left out of its
// weekday's divisor, as a summary edited by hand reads.
func TestADaysHoursCountWhereTheyAreTrue(t *testing.T) {
	short := byHour(map[int]int{9: 3})
	short.Upstream = 5
	negative := byHour(map[int]int{9: 6, 10: -1})
	unsent := ledger.ModelDay{Model: "claude-opus-5-5", Unsent: 3}
	tests := []struct {
		name   string
		models []ledger.ModelDay
		known  int
	}{
		{name: "summing to upstream", models: []ledger.ModelDay{byHour(map[int]int{9: 5})}, known: 4},
		{name: "short of upstream", models: []ledger.ModelDay{short}, known: 3},
		{name: "an hour under none", models: []ledger.ModelDay{negative}, known: 3},
		{name: "never counted", models: []ledger.ModelDay{sent("claude-opus-5-5", 5, "")}, known: 3},
		{name: "never counted, none upstream", models: []ledger.ModelDay{unsent}, known: 4},
		{name: "a model known, another not", models: []ledger.ModelDay{byHour(map[int]int{9: 5}), sent("claude-haiku-4-5", 2, "")}, known: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edited := ledger.Summary{Version: 6, Day: "2026-09-14", Accounts: []ledger.AccountDay{account("work", tt.models...)}}
			page := hoursOf(daysFrom("2026-09-01", edited), time.Monday)
			if got := rowOn(t, page, time.Monday).Known; got != tt.known {
				t.Errorf("Monday is known on %d days, want %d", got, tt.known)
			}
		})
	}
}

// The hours are read, never written: a summary's may be the one held's own.
func TestTheHoursLeaveTheSummariesAsTheyAre(t *testing.T) {
	held := worked("2026-09-14", map[int]int{9: 8, 10: 2})
	was := *held.Accounts[0].Models[0].Hours
	hoursOf(daysFrom("2026-09-01", held, worked("2026-09-21", map[int]int{9: 4})), time.Monday)
	if got := *held.Accounts[0].Models[0].Hours; got != was {
		t.Errorf("the summary's hours are %v, want %v", got, was)
	}
}

// The key gives the cuts rounded to 10, as words.
func TestTheHoursKeyRoundsTheCuts(t *testing.T) {
	page := hoursOf(daysFrom("2026-09-01",
		worked("2026-09-14", map[int]int{9: 372, 10: 192, 11: 48}),
	), time.Monday)
	if want := []float64{12, 48, 93}; !slices.Equal(page.Cuts, want) {
		t.Fatalf("the cuts are %v, want %v", page.Cuts, want)
	}
	if want := []string{"10", "50", "90"}; !slices.Equal(page.KeyCuts, want) {
		t.Errorf("the key's cuts are %v, want %v", page.KeyCuts, want)
	}
}
