package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// Each period begins at the start of its first local day: today at
// midnight; this week on the day weeks start on; the last 30 days 29 days
// before today; the last 12 weeks 11 weeks before this one's first; the last
// 12 months on the day after today's date a year back.
func TestEachPeriodBeginsOnItsFirstDay(t *testing.T) {
	thursday := time.Date(2026, 10, 1, 14, 42, 7, 0, time.Local)
	tests := []struct {
		name   string
		period views.Period
		now    time.Time
		starts time.Weekday
		want   time.Time
	}{
		{name: "today", period: views.PeriodToday, now: thursday, starts: time.Monday, want: on("2026-10-01")},
		{name: "this week from Monday", period: views.PeriodThisWeek, now: thursday, starts: time.Monday, want: on("2026-09-28")},
		{name: "this week from Sunday", period: views.PeriodThisWeek, now: thursday, starts: time.Sunday, want: on("2026-09-27")},
		{name: "30 days", period: views.PeriodThirtyDays, now: thursday, starts: time.Monday, want: on("2026-09-02")},
		{name: "12 weeks from Monday", period: views.PeriodTwelveWeeks, now: thursday, starts: time.Monday, want: on("2026-07-13")},
		{name: "12 weeks from Sunday", period: views.PeriodTwelveWeeks, now: thursday, starts: time.Sunday, want: on("2026-07-12")},
		{name: "today at a second to midnight", period: views.PeriodToday, now: on("2026-10-02").Add(-time.Second), starts: time.Monday, want: on("2026-10-01")},
		{name: "today at a second after midnight", period: views.PeriodToday, now: on("2026-10-02").Add(time.Second), starts: time.Monday, want: on("2026-10-02")},
		{name: "this week at midnight on its first day", period: views.PeriodThisWeek, now: on("2026-10-05"), starts: time.Monday, want: on("2026-10-05")},
		{name: "this week a second before its first day", period: views.PeriodThisWeek, now: on("2026-10-05").Add(-time.Second), starts: time.Monday, want: on("2026-09-28")},
		{name: "12 months", period: views.PeriodTwelveMonths, now: thursday, starts: time.Monday, want: on("2025-10-02")},
		{name: "12 months at a second after midnight", period: views.PeriodTwelveMonths, now: on("2026-10-02").Add(time.Second), starts: time.Monday, want: on("2025-10-03")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.period.From(tt.now, tt.starts); !got.Equal(tt.want) {
				t.Errorf("%q.From(%v, %v) = %v, want %v", tt.period, tt.now, tt.starts, got, tt.want)
			}
		})
	}
}

// Each period has its name, as the heading and the key line give it, THIS
// WEEK's title naming it, and the unit against its price names it by.
func TestEachPeriodIsNamedTitledAndPriced(t *testing.T) {
	tests := []struct {
		period            views.Period
		name, title, unit string
	}{
		{period: views.PeriodToday, name: "today", title: "TODAY", unit: "day"},
		{period: views.PeriodThisWeek, name: "this week", title: "THIS WEEK", unit: "week"},
		{period: views.PeriodThirtyDays, name: "30 days", title: "LAST 30 DAYS", unit: "month"},
		{period: views.PeriodTwelveWeeks, name: "12 weeks", title: "LAST 12 WEEKS", unit: "12 weeks"},
		{period: views.PeriodTwelveMonths, name: "12 months", title: "LAST 12 MONTHS", unit: "12 months"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := [3]string{tt.period.Name(), tt.period.Title(), tt.period.Unit()}; got != [3]string{tt.name, tt.title, tt.unit} {
				t.Errorf("%q is named, titled and priced %q, want %q", tt.period, got, [3]string{tt.name, tt.title, tt.unit})
			}
			if !tt.period.Known() {
				t.Errorf("%q isn't known", tt.period)
			}
		})
	}
	if views.Period("a fortnight").Known() {
		t.Error("a period read back that isn't one is known")
	}
	if got, want := len(views.Periods()), len(tests); got != want {
		t.Errorf("there are %d periods, want %d, each named here", got, want)
	}
}

// < steps to a shorter period and > to a longer, today, this week, 30 days,
// 12 weeks and 12 months in turn, each stopping at its end, where the period
// stands.
func TestThePeriodStepsShorterAndLongerStoppingAtTheEnds(t *testing.T) {
	tests := []struct {
		period          views.Period
		shorter, longer views.Period
		shortened       bool
		lengthened      bool
	}{
		{period: views.PeriodToday, shorter: views.PeriodToday, longer: views.PeriodThisWeek, lengthened: true},
		{period: views.PeriodThisWeek, shorter: views.PeriodToday, shortened: true, longer: views.PeriodThirtyDays, lengthened: true},
		{period: views.PeriodThirtyDays, shorter: views.PeriodThisWeek, shortened: true, longer: views.PeriodTwelveWeeks, lengthened: true},
		{period: views.PeriodTwelveWeeks, shorter: views.PeriodThirtyDays, shortened: true, longer: views.PeriodTwelveMonths, lengthened: true},
		{period: views.PeriodTwelveMonths, shorter: views.PeriodTwelveWeeks, shortened: true, longer: views.PeriodTwelveMonths},
	}
	for _, tt := range tests {
		t.Run(tt.period.Name(), func(t *testing.T) {
			if got, ok := tt.period.Stepped(-1); got != tt.shorter || ok != tt.shortened {
				t.Errorf("%q.Stepped(-1) = %q, %v, want %q, %v", tt.period, got, ok, tt.shorter, tt.shortened)
			}
			if got, ok := tt.period.Stepped(1); got != tt.longer || ok != tt.lengthened {
				t.Errorf("%q.Stepped(1) = %q, %v, want %q, %v", tt.period, got, ok, tt.longer, tt.lengthened)
			}
		})
	}
}

// The last 12 months are the days History's Year counts as LAST 12 MONTHS,
// on any day: from the day after today's date a year back to today, a 29
// February a year back taken to 1 March, whatever day weeks start on.
func TestTwelveMonthsAreTheYearsLastTwelveMonths(t *testing.T) {
	tests := []struct {
		name        string
		now         time.Time
		first, last string
	}{
		{name: "a Thursday", now: on("2026-10-01").Add(14 * time.Hour), first: "2025-10-02", last: "2026-10-01"},
		{name: "in a week begun mid-month", now: on("2026-10-15").Add(9 * time.Hour), first: "2025-10-16", last: "2026-10-15"},
		{name: "on the first of a month", now: on("2027-03-01").Add(time.Hour), first: "2026-03-02", last: "2027-03-01"},
		{name: "on a 29 February", now: on("2028-02-29").Add(12 * time.Hour), first: "2027-03-01", last: "2028-02-29"},
		{name: "on the day after a 29 February", now: on("2028-03-01").Add(12 * time.Hour), first: "2027-03-02", last: "2028-03-01"},
		{name: "on a 28 February, from the 29th a year back", now: on("2029-02-28").Add(12 * time.Hour), first: "2028-02-29", last: "2029-02-28"},
	}
	for _, tt := range tests {
		for _, starts := range []time.Weekday{time.Monday, time.Sunday, time.Wednesday} {
			t.Run(tt.name+" from "+starts.String(), func(t *testing.T) {
				first, last := views.PeriodTwelveMonths.Spans(tt.now, starts)
				yearFirst, yearLast := views.YearsTwelveMonths(tt.now, starts)
				if got, want := [2]string{first, last}, [2]string{tt.first, tt.last}; got != want {
					t.Errorf("12 months at %v run %v, want %v", tt.now, got, want)
				}
				if got, want := [2]string{first, last}, [2]string{yearFirst, yearLast}; got != want {
					t.Errorf("12 months at %v run %v, the Year's %v", tt.now, got, want)
				}
			})
		}
	}
}

// A period's plans are priced whole, while it's still running, whatever day
// of it it is: today a day's price, this week a week's, even on its first
// day, 30 days a month's, 12 weeks twelve weeks' and 12 months twelve
// months'.
func TestAPeriodsPlansArePricedWhole(t *testing.T) {
	ultra, _ := ledger.Pricing.Plan("ultra")
	monthly := ultra.Price("2026-10-05")
	tests := []struct {
		name   string
		period views.Period
		now    time.Time
		want   string
	}{
		{name: "today", period: views.PeriodToday, now: on("2026-10-05").Add(time.Hour), want: "$7"},
		{name: "this week on its first day", period: views.PeriodThisWeek, now: on("2026-10-05").Add(time.Hour), want: "$46"},
		{name: "this week on its last day", period: views.PeriodThisWeek, now: on("2026-10-11").Add(time.Hour), want: "$46"},
		{name: "30 days", period: views.PeriodThirtyDays, now: on("2026-10-05").Add(time.Hour), want: "$200"},
		{name: "12 weeks", period: views.PeriodTwelveWeeks, now: on("2026-10-05").Add(time.Hour), want: "$554"},
		{name: "12 months", period: views.PeriodTwelveMonths, now: on("2026-10-05").Add(time.Hour), want: "$2,400"},
		{name: "12 months holding a 29 February", period: views.PeriodTwelveMonths, now: on("2028-03-01").Add(time.Hour), want: "$2,400"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysTo(tt.now.AddDate(-1, -1, 0).Format(time.DateOnly), tt.now)
			in := input(days)
			in.Now = tt.now
			plan := views.NewPast(in, days).Accounts(tt.period).Rows[0].Plan
			if plan.Price == nil || *plan.Price != monthly || plan.Cost == nil || plan.Cost.GroupedDollars() != tt.want {
				t.Errorf("work's plan over %s costs %v of %v a month, want %s of %v", tt.name, plan.Cost, plan.Price, tt.want, monthly)
			}
		})
	}
}

// A window's limit is hit at most once a window, so over the longest period
// the dial steps to, 12 months, of 366 days where they hold a 29 February, a
// 5-hour window's at most 1,756 times, a week's 52, a day's 366; a key that
// names no length counts as an hour's window.
func TestLimitsAreHitAtMostOnceAWindowOverTheLongestPeriod(t *testing.T) {
	for key, want := range map[string]int{"5h": 1756, "7d": 52, "7d_oi": 52, "1d": 366, "session": 8784} {
		if got := views.LimitsHitMost(key); got != want {
			t.Errorf("LimitsHitMost(%q) = %d, want %d", key, got, want)
		}
	}
}
