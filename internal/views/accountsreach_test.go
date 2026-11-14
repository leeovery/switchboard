package views_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// yearOfPeaks are 60 weeks of work's and side's own weeks, each ending as
// its calendar week does, the last on Monday 5 October, now's: work's at
// its limit in the week of each month's first Monday, at 60% in the rest;
// side's a tenth and a hundredth for each day of the month its Monday is,
// so each month's fullest is its last; and this week's so far, work's 34%
// and side's 78%.
func yearOfPeaks() []ledger.Summary {
	peaks := map[string][]float64{"work": make([]float64, 60), "side": make([]float64, 60)}
	for i := range 60 {
		monday := on("2026-09-28").AddDate(0, 0, -7*(59-i))
		peaks["work"][i] = 0.6
		if monday.Day() <= 7 {
			peaks["work"][i] = 1
		}
		peaks["side"][i] = 0.1 + 0.01*float64(monday.Day())
	}
	days := peaking(peaks)
	days[len(days)-2] = day("2026-10-06",
		windowsDay{highest: map[string]float64{"7d": 0.34}}.of("work"),
		windowsDay{highest: map[string]float64{"7d": 0.78}}.of("side"))
	return days
}

// reachingPage is Accounts' page over the period given of yearOfPeaks, of
// work and side.
func reachingPage(period views.Period) views.AccountsPage {
	days := yearOfPeaks()
	in := input(days)
	in.Accounts = workAndSide
	return views.NewPast(in, days).Accounts(period)
}

// Compare's WEEKS reaches over the weeks that begin in the period, this one
// last, but never fewer than eight: a column a week, named on the axis
// where its month starts; and at 12 months, a column a month, the months
// the period touches, each named.
func TestCompareReachesOverThePeriodsWeeks(t *testing.T) {
	tests := []struct {
		period                  views.Period
		columns                 int
		first, last, firstAxis  string
		firstStart, lastStarted string
	}{
		{views.PeriodToday, 8, "17 Aug", "5 Oct", "Aug", "2026-08-17", "2026-10-05"},
		{views.PeriodThisWeek, 8, "17 Aug", "5 Oct", "Aug", "2026-08-17", "2026-10-05"},
		{views.PeriodThirtyDays, 8, "17 Aug", "5 Oct", "Aug", "2026-08-17", "2026-10-05"},
		{views.PeriodTwelveWeeks, 12, "20 Jul", "5 Oct", "Jul", "2026-07-20", "2026-10-05"},
		{views.PeriodTwelveMonths, 13, "Oct", "Oct", "Oct", "2025-10-01", "2026-10-01"},
	}
	for _, tt := range tests {
		t.Run(tt.period.Name(), func(t *testing.T) {
			page := reachingPage(tt.period)
			reach := page.Reach
			got := fmt.Sprintf("%d %s %s %s %s %s", len(reach), reach[0].Label, reach[len(reach)-1].Label, reach[0].Axis,
				reach[0].Start.Format(time.DateOnly), reach[len(reach)-1].Start.Format(time.DateOnly))
			want := fmt.Sprintf("%d %s %s %s %s %s", tt.columns, tt.first, tt.last, tt.firstAxis, tt.firstStart, tt.lastStarted)
			if got != want {
				t.Errorf("the reach is %s, want %s", got, want)
			}
			for _, row := range append(slices.Clone(page.Rows), page.All) {
				if len(row.Reached) != len(reach) {
					t.Errorf("%q has %d peaks in the reach, want one a column, %d", row.Totals.Account, len(row.Reached), len(reach))
				}
			}
		})
	}
}

// Each row's peaks in the reach, and the facts WEEKS says under them, are
// Weeks' over the reach's weeks, never worked out again: over this week's
// eight, Weeks' own page's, peaks and verdicts alike; this week's column
// so far, still running; every account's peaks the mean of theirs, and its
// limits the weeks some account was short.
func TestTheReachsPeaksAndFactsAreWeeks(t *testing.T) {
	days := yearOfPeaks()
	in := input(days)
	in.Accounts = workAndSide
	past := views.NewPast(in, days)
	page, weeks := past.Accounts(views.PeriodThisWeek), past.Weeks()
	rows := append(slices.Clone(page.Rows), page.All)
	weekly := append(slices.Clone(weeks.Accounts), weeks.All)
	for i, row := range rows {
		if !reflect.DeepEqual(row.Reached, weekly[i].Peaks) {
			t.Errorf("%q's peaks in the reach are %s, want Weeks', %s", row.Totals.Account, peaksRead(row.Reached), peaksRead(weekly[i].Peaks))
		}
		if !reflect.DeepEqual(row.Reaching, weekly[i].AccountVerdict) {
			t.Errorf("%q's facts are %s, want Weeks' verdict, %s", row.Totals.Account, verdictRead(row.Reaching), verdictRead(weekly[i].AccountVerdict))
		}
	}
	if last := page.Rows[0].Reached[7]; !last.Open {
		t.Errorf("work's last column is %+v, want this week's, still running", last)
	}
	if got, want := page.All.Reaching.Limits, weeks.Capacity.Short; !page.All.Reaching.All || got != want {
		t.Errorf("all accounts were short on %d weeks, want Weeks' short, %d", got, want)
	}
}

// Over 12 weeks and 12 months, the facts are over the whole weeks the reach
// is of, the 11 and 51 before this one; and at 12 months each month's column
// is its fullest week's of those that begin in it, the month this week
// began in holding it so far, where it's the only one.
func TestTheReachAt12WeeksAnd12MonthsIsTheirWeeks(t *testing.T) {
	if work := reachingPage(views.PeriodTwelveWeeks).Rows[0].Reaching; work.Weeks != 11 || work.Limits != 2 {
		t.Errorf("work's facts over 12 weeks are %s, want its limit hit in 2 of 11 whole weeks", verdictRead(work))
	}
	months := reachingPage(views.PeriodTwelveMonths)
	if work := months.Rows[0].Reaching; work.Weeks != 51 || work.Limits != 11 {
		t.Errorf("work's facts over 12 months are %s, want its limit hit in 11 of 51 whole weeks", verdictRead(work))
	}
	if got, want := peaksRead(months.Rows[0].Reached), "60 100 100 100 100 100 100 100 100 100 100 100 34…"; got != want {
		t.Errorf("work's months are %s, want each its fullest week, %s", got, want)
	}
	if got, want := peaksRead(months.Rows[1].Reached), "37 34 39 36 33 40 37 35 39 37 41 38 78…"; got != want {
		t.Errorf("side's months are %s, want each its last week, its fullest, %s", got, want)
	}
}

// What an account's requests were worth is charted in the buckets Projects
// charts a project's in, the finest first, but today's by the day, till
// Hourly charts it by the hour, as the summaries hold no hours: this week's
// by day, 30 days' and 12 weeks' by day and by week, 12 months' by week and
// by month, its peak month named with its year where the period touches
// two of its name; each chart's buckets summing to the row's worth, every
// account's to theirs.
func TestAnAccountsWorthIsChartedAsProjectsChartAProjects(t *testing.T) {
	days := daysFrom("2025-09-01",
		day("2025-11-03", account("work", sent("claude-opus-5-5", 2, inputUsage))),
		day("2026-08-20", account("side", sent("claude-opus-5-5", 1, inputUsage))),
		day("2026-10-05", account("work", sent("claude-opus-5-5", 1, inputUsage))),
		day("2026-10-07", account("work", sent("claude-opus-5-5", 3, opusUsage)), account("side", sent("claude-opus-5-5", 1, inputUsage))),
	)
	in := input(days)
	in.Accounts = workAndSide
	past := views.NewPast(in, days)
	type chart struct {
		unit    views.BucketUnit
		buckets int
		peak    string
	}
	tests := []struct {
		period views.Period
		work   []chart
	}{
		{views.PeriodToday, []chart{{views.ByDay, 1, "Wed"}}},
		{views.PeriodThisWeek, []chart{{views.ByDay, 7, "Wed"}}},
		{views.PeriodThirtyDays, []chart{{views.ByDay, 30, "7 Oct"}, {views.ByWeek, 5, "week of 5 Oct"}}},
		{views.PeriodTwelveWeeks, []chart{{views.ByDay, 84, "7 Oct"}, {views.ByWeek, 12, "week of 5 Oct"}}},
		{views.PeriodTwelveMonths, []chart{{views.ByWeek, 53, "week of 5 Oct"}, {views.ByMonth, 13, "Oct 2026"}}},
	}
	for _, tt := range tests {
		t.Run(tt.period.Name(), func(t *testing.T) {
			page := past.Accounts(tt.period)
			var got []chart
			for _, by := range page.Rows[0].WorthBy {
				got = append(got, chart{by.Unit, len(by.Buckets), by.PeakNamed()})
			}
			if !reflect.DeepEqual(got, tt.work) {
				t.Errorf("work's worth is charted as %+v, want %+v", got, tt.work)
			}
			for _, row := range append(slices.Clone(page.Rows), page.All) {
				for _, by := range row.WorthBy {
					var sum views.Worth
					for _, b := range by.Buckets {
						sum = sumOf(sum, b.Worth)
					}
					if !reflect.DeepEqual(sum, row.Totals.Worth) {
						t.Errorf("%q's buckets by %d sum to %+v, want its worth, %+v", row.Totals.Account, by.Unit, sum, row.Totals.Worth)
					}
				}
			}
		})
	}
}

// Accounts' today, fed the tail's fold of it, charts each account's worth by
// the hour its requests arrived in, and by two hours, as Projects charts a
// project's, all accounts' every account's; a fold of another day, or any
// other period, leaves the page by the day, as the summaries have it.
func TestAccountsTodayIsChartedByTheHourOfTheTailsFold(t *testing.T) {
	lines := slices.Concat(
		projectAsks(repoSession, "/code/switchboard", "", "work", times(october(7, 9, 0), 2)...),
		projectAsks(repoSession, "/code/switchboard", "", "side", october(7, 9, 30)),
		projectAsks(ownSession, "/code/own/api", "", "side", times(october(7, 12, 0), 3)...),
		projectAsks(ownSession, "/code/own/api", "", "work", october(7, 13, 0)),
	)
	in := input(nil)
	in.Accounts = workAndSide
	byDay := views.NewPast(in, nil).Accounts(views.PeriodToday)
	page := byDay.Hourly(heldDays(now, lines...))
	type chart struct {
		unit            views.BucketUnit
		buckets, toCome int
		worths          map[int]views.Worth
		peak            string
	}
	of := func(by views.WorthBy) chart {
		worths := map[int]views.Worth{}
		for i, b := range by.Buckets {
			if b.Amount != nil && *b.Amount > 0 {
				worths[i] = b.Worth
			}
		}
		return chart{by.Unit, len(by.Buckets), by.ToCome, worths, by.PeakNamed()}
	}
	tests := []struct {
		name string
		row  views.AccountsRow
		want []chart
	}{
		{"work", page.Rows[0], []chart{
			{views.ByHour, 24, 10, map[int]views.Worth{9: hoursWorth(2), 13: hoursWorth(1)}, "09:00"},
			{views.ByTwoHours, 12, 5, map[int]views.Worth{4: hoursWorth(2), 6: hoursWorth(1)}, "08:00–10:00"},
		}},
		{"side", page.Rows[1], []chart{
			{views.ByHour, 24, 10, map[int]views.Worth{9: hoursWorth(1), 12: hoursWorth(3)}, "12:00"},
			{views.ByTwoHours, 12, 5, map[int]views.Worth{4: hoursWorth(1), 6: hoursWorth(3)}, "12:00–14:00"},
		}},
		{"all accounts", page.All, []chart{
			{views.ByHour, 24, 10, map[int]views.Worth{9: hoursWorth(3), 12: hoursWorth(3), 13: hoursWorth(1)}, "09:00"},
			{views.ByTwoHours, 12, 5, map[int]views.Worth{4: hoursWorth(3), 6: hoursWorth(4)}, "12:00–14:00"},
		}},
	}
	for _, tt := range tests {
		var got []chart
		for _, by := range tt.row.WorthBy {
			got = append(got, of(by))
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s's today is charted as %+v, want %+v", tt.name, got, tt.want)
		}
	}
	if !reflect.DeepEqual(byDay.Rows[0].Totals, page.Rows[0].Totals) {
		t.Errorf("charted by the hour, work's totals are %+v, want the page's, %+v", page.Rows[0].Totals, byDay.Rows[0].Totals)
	}
	charts := func(page views.AccountsPage) []views.WorthCharts {
		return []views.WorthCharts{page.Rows[0].WorthBy, page.Rows[1].WorthBy, page.All.WorthBy}
	}
	yesterday := heldDays(october(6, 23, 0), projectAsks(repoSession, "/code/switchboard", "", "work", october(6, 22, 0))...)
	if got := byDay.Hourly(yesterday); !reflect.DeepEqual(charts(got), charts(byDay)) {
		t.Errorf("fed yesterday's fold, today's page is charted %+v, want it by the day, %+v", charts(got), charts(byDay))
	}
	week := views.NewPast(in, nil).Accounts(views.PeriodThisWeek)
	if got := week.Hourly(heldDays(now, lines...)); !reflect.DeepEqual(charts(got), charts(week)) {
		t.Errorf("fed today's fold, this week's page is charted %+v, want it by the day, %+v", charts(got), charts(week))
	}
}
