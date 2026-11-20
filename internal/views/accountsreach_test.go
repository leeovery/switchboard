package views_test

import (
	"fmt"
	"iter"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/readings"
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
		{views.LengthToday.Now(), 8, "17 Aug", "5 Oct", "Aug", "2026-08-17", "2026-10-05"},
		{views.LengthThisWeek.Now(), 8, "17 Aug", "5 Oct", "Aug", "2026-08-17", "2026-10-05"},
		{views.LengthThirtyDays.Now(), 8, "17 Aug", "5 Oct", "Aug", "2026-08-17", "2026-10-05"},
		{views.LengthTwelveWeeks.Now(), 12, "20 Jul", "5 Oct", "Jul", "2026-07-20", "2026-10-05"},
		{views.LengthTwelveMonths.Now(), 13, "Oct", "Oct", "Oct", "2025-10-01", "2026-10-01"},
	}
	for _, tt := range tests {
		t.Run(tt.period.Length().Name(), func(t *testing.T) {
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
	page, weeks := past.Accounts(views.LengthThisWeek.Now()), past.Weeks()
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
	if work := reachingPage(views.LengthTwelveWeeks.Now()).Rows[0].Reaching; work.Weeks != 11 || work.Limits != 2 {
		t.Errorf("work's facts over 12 weeks are %s, want its limit hit in 2 of 11 whole weeks", verdictRead(work))
	}
	months := reachingPage(views.LengthTwelveMonths.Now())
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

// At 12 months a month is as full as its fullest week, at its cap where any
// of its weeks is: September's weeks are work at its limit, side and
// personal at 40%, then all three at 80%, 75% and 75%, its fullest.
func TestAMonthAt12MonthsIsAtItsCapWhereAnyOfItsWeeksIs(t *testing.T) {
	days := peaking(map[string][]float64{"work": {1, 0.8}, "side": {0.4, 0.75}, "personal": {0.4, 0.75}})
	in := input(days)
	in.Accounts = config.Accounts{{ID: "work"}, {ID: "side"}, {ID: "personal"}}
	page := views.NewPast(in, days).Accounts(views.LengthTwelveMonths.Now())
	september := slices.IndexFunc(page.Reach, func(b views.Bucket) bool { return b.Start.Format(time.DateOnly) == "2026-09-01" })
	if september < 0 {
		t.Fatalf("the reach %v has no September", page.Reach)
	}
	if got := page.All.Reached[september]; got.Share == nil || prose.Percent(*got.Share) != "77%" || !got.AtCap {
		t.Errorf("every account's September is %+v, want its fullest week, 77%%, at its cap", got)
	}
}

// At 12 months the month this week falls in is as full as this week so far,
// its fullest, but marked at its cap, not open, where another of its weeks
// was short: it's 20 October, and October's first week was whole and short,
// work at its limit and the rest at 30%; this week so far, all three at 60%.
func TestTheMonthThisWeekFallsInIsMarkedWhereAnotherOfItsWeeksWasShort(t *testing.T) {
	today := on("2026-10-20").Add(14 * time.Hour)
	ended := func(date string, peaks ...float64) ledger.Summary {
		var accounts []ledger.AccountDay
		for x, id := range []string{"work", "side", "personal"} {
			accounts = append(accounts, windowsDay{resets: []ledger.Reset{reset("7d", date, "00:30", peaks[x])}}.of(id))
		}
		return day(date, accounts...)
	}
	soFar := windowsDay{highest: map[string]float64{"7d": 0.6}}
	days := daysTo("2026-09-28", today,
		ended("2026-10-05", 0.2, 0.2, 0.2),
		ended("2026-10-12", 1, 0.3, 0.3),
		ended("2026-10-19", 0.4, 0.4, 0.4),
		day("2026-10-20", soFar.of("work"), soFar.of("side"), soFar.of("personal")),
	)
	in := input(days)
	in.Now, in.Accounts = today, config.Accounts{{ID: "work"}, {ID: "side"}, {ID: "personal"}}
	page := views.NewPast(in, days).Accounts(views.LengthTwelveMonths.Now())
	october := slices.IndexFunc(page.Reach, func(b views.Bucket) bool { return b.Start.Format(time.DateOnly) == "2026-10-01" })
	if october < 0 {
		t.Fatalf("the reach %v has no October", page.Reach)
	}
	if got := page.All.Reached[october]; got.Share == nil || prose.Percent(*got.Share) != "60%" || got.Open || !got.AtCap {
		t.Errorf("every account's October is %+v, want this week so far, 60%%, not open, at its cap", got)
	}
	if side := page.Rows[1].Reached[october]; side.Share == nil || prose.Percent(*side.Share) != "60%" || !side.Open || side.AtCap {
		t.Errorf("side's October is %+v, want this week so far, 60%%, open, none of its weeks at its cap", side)
	}
	if work := page.Rows[0].Reached[october]; work.Share == nil || *work.Share != 1 || !work.AtCap {
		t.Errorf("work's October is %+v, want its fullest week, at its limit", work)
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
		{views.LengthToday.Now(), []chart{{views.ByDay, 1, "Wed"}}},
		{views.LengthThisWeek.Now(), []chart{{views.ByDay, 7, "Wed"}}},
		{views.LengthThirtyDays.Now(), []chart{{views.ByDay, 30, "7 Oct"}, {views.ByWeek, 5, "week of 5 Oct"}}},
		{views.LengthTwelveWeeks.Now(), []chart{{views.ByDay, 84, "7 Oct"}, {views.ByWeek, 12, "week of 5 Oct"}}},
		{views.LengthTwelveMonths.Now(), []chart{{views.ByWeek, 53, "week of 5 Oct"}, {views.ByMonth, 13, "Oct 2026"}}},
	}
	for _, tt := range tests {
		t.Run(tt.period.Length().Name(), func(t *testing.T) {
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

// chartFrom is a chart of WORTH BY as where it begins tells it: its unit,
// how many buckets it has, and the date its first starts on.
type chartFrom struct {
	unit    views.BucketUnit
	buckets int
	first   string
}

// chartsFrom are the charts given, as chartFrom has them.
func chartsFrom(charts views.WorthCharts) []chartFrom {
	var from []chartFrom
	for _, by := range charts {
		from = append(from, chartFrom{by.Unit, len(by.Buckets), by.Buckets[0].Start.Format(time.DateOnly)})
	}
	return from
}

// Where the work began part-way through the period, as a young pool's did,
// each account's worth is charted from the day it began, no bucket before
// it as a day of none, its first week the one that day falls in; where it
// began before the period, the whole period is charted.
func TestAnAccountsWorthIsChartedFromTheDayTheWorkBegan(t *testing.T) {
	days := daysFrom("2026-10-02",
		day("2026-10-05", account("work", sent("claude-opus-5-5", 1, inputUsage))),
		day("2026-10-07", account("work", sent("claude-opus-5-5", 3, opusUsage))),
	)
	tests := []struct {
		name   string
		began  string
		period views.Period
		want   []chartFrom
	}{
		{"begun on Friday, over 30 days", "2026-10-02", views.LengthThirtyDays.Now(), []chartFrom{{views.ByDay, 6, "2026-10-02"}, {views.ByWeek, 2, "2026-09-28"}}},
		{"begun on Friday, over 12 months", "2026-10-02", views.LengthTwelveMonths.Now(), []chartFrom{{views.ByWeek, 2, "2026-09-28"}, {views.ByMonth, 1, "2026-10-01"}}},
		{"begun on Friday, over this week", "2026-10-02", views.LengthThisWeek.Now(), []chartFrom{{views.ByDay, 7, "2026-10-05"}}},
		{"begun in January, over 30 days", "2026-01-01", views.LengthThirtyDays.Now(), []chartFrom{{views.ByDay, 30, "2026-09-08"}, {views.ByWeek, 5, "2026-09-07"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(days)
			in.Accounts, in.Began = workAndSide, on(tt.began)
			page := views.NewPast(in, days).Accounts(tt.period)
			for _, row := range append(slices.Clone(page.Rows), page.All) {
				if got := chartsFrom(row.WorthBy); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("%q's worth is charted as %+v, want %+v", row.Totals.Account, got, tt.want)
				}
			}
		})
	}
}

// Accounts' today, fed the tail's fold of it, charts each account's worth by
// the hour its requests arrived in, and by two hours, as Projects charts a
// project's, a request of no session among them, all accounts' every
// account's, a session's request answered by no account among them; a fold
// of another day, or any other period, leaves the page by the day, as the
// summaries have it.
func TestAccountsTodayIsChartedByTheHourOfTheTailsFold(t *testing.T) {
	lines := slices.Concat(
		projectAsks(repoSession, "/code/switchboard", "", "work", times(october(7, 9, 0), 2)...),
		projectAsks(repoSession, "/code/switchboard", "", "side", october(7, 9, 30)),
		projectAsks(ownSession, "/code/own/api", "", "side", times(october(7, 12, 0), 3)...),
		projectAsks(ownSession, "/code/own/api", "", "work", october(7, 13, 0)),
		projectAsks(ownSession, "/code/own/api", "", "", october(7, 13, 5)),
		projectAsks("", "/code/switchboard", "", "work", october(7, 10, 30)),
	)
	in := input(nil)
	in.Accounts = workAndSide
	byDay := views.NewPast(in, nil).Accounts(views.LengthToday.Now())
	page := byDay.Hourly(heldDays(now, lines...).TodayFold())
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
			{views.ByHour, 24, 10, map[int]views.Worth{9: hoursWorth(2), 10: hoursWorth(1), 13: hoursWorth(1)}, "09:00"},
			{views.ByTwoHours, 12, 5, map[int]views.Worth{4: hoursWorth(2), 5: hoursWorth(1), 6: hoursWorth(1)}, "08:00–10:00"},
		}},
		{"side", page.Rows[1], []chart{
			{views.ByHour, 24, 10, map[int]views.Worth{9: hoursWorth(1), 12: hoursWorth(3)}, "12:00"},
			{views.ByTwoHours, 12, 5, map[int]views.Worth{4: hoursWorth(1), 6: hoursWorth(3)}, "12:00–14:00"},
		}},
		{"all accounts", page.All, []chart{
			{views.ByHour, 24, 10, map[int]views.Worth{9: hoursWorth(3), 10: hoursWorth(1), 12: hoursWorth(3), 13: hoursWorth(2)}, "09:00"},
			{views.ByTwoHours, 12, 5, map[int]views.Worth{4: hoursWorth(3), 5: hoursWorth(1), 6: hoursWorth(5)}, "12:00–14:00"},
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
	if got := byDay.Hourly(yesterday.TodayFold()); !reflect.DeepEqual(charts(got), charts(byDay)) {
		t.Errorf("fed yesterday's fold, today's page is charted %+v, want it by the day, %+v", charts(got), charts(byDay))
	}
	week := views.NewPast(in, nil).Accounts(views.LengthThisWeek.Now())
	if got := week.Hourly(heldDays(now, lines...).TodayFold()); !reflect.DeepEqual(charts(got), charts(week)) {
		t.Errorf("fed today's fold, this week's page is charted %+v, want it by the day, %+v", charts(got), charts(week))
	}
	back, _ := views.LengthToday.Now().Back(in.Now, in.WeekStarts, time.Time{})
	gone := views.NewPast(in, nil).Accounts(back)
	if got := gone.Hourly(heldDays(now, lines...).TodayFold()); !reflect.DeepEqual(charts(got), charts(gone)) {
		t.Errorf("fed today's fold, yesterday's page is charted %+v, want it by the day, %+v", charts(got), charts(gone))
	}
}

// Today's hours count every request, as the day's summary does, those of no
// session among them, so each row's hours sum to its worth, all accounts'
// to theirs.
func TestAccountsTodaysHoursSumToItsWorth(t *testing.T) {
	lines := slices.Concat(
		projectAsks(repoSession, "/code/switchboard", "", "work", times(october(7, 9, 0), 2)...),
		projectAsks("", "/code/switchboard", "", "work", october(7, 10, 0)),
		projectAsks(ownSession, "/code/own/api", "", "side", october(7, 12, 0)),
	)
	var held []ledger.Line
	for _, l := range lines {
		held = append(held, l.Line)
	}
	none := func(time.Time, time.Time) iter.Seq[readings.Reading] { return func(func(readings.Reading) bool) {} }
	summary, err := ledger.Summarise("2026-10-07", slices.Values(held), none, nil, ledger.Caps{}, now)
	if err != nil {
		t.Fatal(err)
	}
	days := []ledger.Summary{summary}
	in := input(days)
	in.Accounts = workAndSide
	page := views.NewPast(in, days).Accounts(views.LengthToday.Now()).Hourly(heldDays(now, lines...).TodayFold())
	for _, row := range append(slices.Clone(page.Rows), page.All) {
		var sum views.Worth
		for _, b := range row.WorthBy[0].Buckets {
			sum = sumOf(sum, b.Worth)
		}
		if row.WorthBy[0].Unit != views.ByHour || !reflect.DeepEqual(sum, row.Totals.Worth) {
			t.Errorf("%q's hours by %d sum to %+v, want its worth, %+v", row.Totals.Account, row.WorthBy[0].Unit, sum, row.Totals.Worth)
		}
	}
	if got, want := page.Rows[0].Totals.Worth, hoursWorth(3); !reflect.DeepEqual(got, want) {
		t.Errorf("work's worth is %+v, want its three requests', %+v, the one of no session among them", got, want)
	}
}

// A request of no session that a later read of the tail gives again counts
// once in today's hours, as a session's does.
func TestARequestOfNoSessionGivenAgainCountsOnce(t *testing.T) {
	line := projectAsks("", "/code/switchboard", "", "work", october(7, 10, 0))
	days := views.ReadSessionDays(ledgerOfLines(), now.AddDate(0, 0, -1), now, views.PoolRead{}).
		Tailed(line, true, nil, nil, now, views.PoolRead{}).Tailed(line, false, nil, nil, now, views.PoolRead{})
	in := input(nil)
	in.Accounts = workAndSide
	page := views.NewPast(in, nil).Accounts(views.LengthToday.Now()).Hourly(days.TodayFold())
	if got := page.Rows[0].WorthBy[0].Buckets[10].Worth; !reflect.DeepEqual(got, hoursWorth(1)) {
		t.Errorf("given twice, work's 10:00 is worth %+v, want the request's once, %+v", got, hoursWorth(1))
	}
}
