package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// Each period begins at the start of its first local day: today at
// midnight; this week on the day weeks start on; the last 30 days 29 days
// before today; the last 12 weeks 11 weeks before this one's first.
func TestEachPeriodBeginsOnItsFirstDay(t *testing.T) {
	thursday := time.Date(2026, 10, 1, 14, 42, 7, 0, time.Local)
	tests := []struct {
		name   string
		period views.AccountsPeriod
		now    time.Time
		starts time.Weekday
		want   time.Time
	}{
		{name: "today", period: views.AccountsToday, now: thursday, starts: time.Monday, want: on("2026-10-01")},
		{name: "this week from Monday", period: views.AccountsThisWeek, now: thursday, starts: time.Monday, want: on("2026-09-28")},
		{name: "this week from Sunday", period: views.AccountsThisWeek, now: thursday, starts: time.Sunday, want: on("2026-09-27")},
		{name: "30 days", period: views.AccountsThirtyDays, now: thursday, starts: time.Monday, want: on("2026-09-02")},
		{name: "12 weeks from Monday", period: views.AccountsTwelveWeeks, now: thursday, starts: time.Monday, want: on("2026-07-13")},
		{name: "12 weeks from Sunday", period: views.AccountsTwelveWeeks, now: thursday, starts: time.Sunday, want: on("2026-07-12")},
		{name: "today at a second to midnight", period: views.AccountsToday, now: on("2026-10-02").Add(-time.Second), starts: time.Monday, want: on("2026-10-01")},
		{name: "today at a second after midnight", period: views.AccountsToday, now: on("2026-10-02").Add(time.Second), starts: time.Monday, want: on("2026-10-02")},
		{name: "this week at midnight on its first day", period: views.AccountsThisWeek, now: on("2026-10-05"), starts: time.Monday, want: on("2026-10-05")},
		{name: "this week a second before its first day", period: views.AccountsThisWeek, now: on("2026-10-05").Add(-time.Second), starts: time.Monday, want: on("2026-09-28")},
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
		period            views.AccountsPeriod
		name, title, unit string
	}{
		{period: views.AccountsToday, name: "today", title: "TODAY", unit: "day"},
		{period: views.AccountsThisWeek, name: "this week", title: "THIS WEEK", unit: "week"},
		{period: views.AccountsThirtyDays, name: "30 days", title: "LAST 30 DAYS", unit: "month"},
		{period: views.AccountsTwelveWeeks, name: "12 weeks", title: "LAST 12 WEEKS", unit: "12 weeks"},
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
	if views.AccountsPeriod("a fortnight").Known() {
		t.Error("a period read back that isn't one is known")
	}
}

// < steps to a shorter period and > to a longer, today, this week, 30 days
// and 12 weeks in turn, each stopping at its end, where the period stands.
func TestThePeriodStepsShorterAndLongerStoppingAtTheEnds(t *testing.T) {
	tests := []struct {
		period          views.AccountsPeriod
		shorter, longer views.AccountsPeriod
		shortened       bool
		lengthened      bool
	}{
		{period: views.AccountsToday, shorter: views.AccountsToday, longer: views.AccountsThisWeek, lengthened: true},
		{period: views.AccountsThisWeek, shorter: views.AccountsToday, shortened: true, longer: views.AccountsThirtyDays, lengthened: true},
		{period: views.AccountsThirtyDays, shorter: views.AccountsThisWeek, shortened: true, longer: views.AccountsTwelveWeeks, lengthened: true},
		{period: views.AccountsTwelveWeeks, shorter: views.AccountsThirtyDays, shortened: true, longer: views.AccountsTwelveWeeks},
	}
	for _, tt := range tests {
		t.Run(tt.period.Name(), func(t *testing.T) {
			if got, ok := tt.period.Shorter(); got != tt.shorter || ok != tt.shortened {
				t.Errorf("%q.Shorter() = %q, %v, want %q, %v", tt.period, got, ok, tt.shorter, tt.shortened)
			}
			if got, ok := tt.period.Longer(); got != tt.longer || ok != tt.lengthened {
				t.Errorf("%q.Longer() = %q, %v, want %q, %v", tt.period, got, ok, tt.longer, tt.lengthened)
			}
		})
	}
}

// A period's plans are priced whole, while it's still running, whatever day
// of it it is: today a day's price, this week a week's, even on its first
// day, 30 days a month's and 12 weeks twelve weeks'.
func TestAPeriodsPlansArePricedWhole(t *testing.T) {
	max20x, _ := ledger.Pricing.Plan("max20x")
	monthly := max20x.Price("2026-10-05")
	tests := []struct {
		name   string
		period views.AccountsPeriod
		now    time.Time
		want   string
	}{
		{name: "today", period: views.AccountsToday, now: on("2026-10-05").Add(time.Hour), want: "$7"},
		{name: "this week on its first day", period: views.AccountsThisWeek, now: on("2026-10-05").Add(time.Hour), want: "$46"},
		{name: "this week on its last day", period: views.AccountsThisWeek, now: on("2026-10-11").Add(time.Hour), want: "$46"},
		{name: "30 days", period: views.AccountsThirtyDays, now: on("2026-10-05").Add(time.Hour), want: "$200"},
		{name: "12 weeks", period: views.AccountsTwelveWeeks, now: on("2026-10-05").Add(time.Hour), want: "$554"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysTo("2026-07-01", tt.now)
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
// Accounts shows, twelve weeks, a 5-hour window's at most 403 times, a week's
// 12, a day's 84; a key that names no length counts as an hour's window.
func TestLimitsAreHitAtMostOnceAWindowOverTheLongestPeriod(t *testing.T) {
	for key, want := range map[string]int{"5h": 403, "7d": 12, "7d_oi": 12, "1d": 84, "session": 2016} {
		if got := views.LimitsHitMost(key); got != want {
			t.Errorf("LimitsHitMost(%q) = %d, want %d", key, got, want)
		}
	}
}
