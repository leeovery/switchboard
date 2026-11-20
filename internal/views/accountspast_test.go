package views_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// pastWeeks are work's and side's days to framesNow's, each account's week
// resetting at 00:30 each Monday: the 110 weeks before this one, work's at
// its limit in the week of each month's first Monday, reached on its
// Friday, and at 60% in the rest, side's a tenth and a hundredth for each
// day of the month its Monday is; this week's so far, work's 34% and side's
// 78%; a request of side's each Wednesday; and work's week risen 20% on
// Tuesday 22 September, a 5-hour limit hit that day, and 10% on Tuesday 29
// September.
func pastWeeks() []ledger.Summary {
	const weeks = 110
	peaks := map[string][]float64{"work": make([]float64, weeks), "side": make([]float64, weeks)}
	var limited []string
	for i := range weeks {
		monday := on("2026-09-21").AddDate(0, 0, -7*(weeks-1-i))
		peaks["work"][i], peaks["side"][i] = 0.6, 0.1+0.01*float64(monday.Day())
		if monday.Day() <= 7 {
			peaks["work"][i] = 1
			limited = append(limited, monday.AddDate(0, 0, 4).Format(time.DateOnly))
		}
	}
	risen := map[string]windowsDay{
		"2026-09-22": {rise: map[string]float64{"7d": 0.2}, limits: []ledger.Limit{limit("5h", "2026-09-22", "15:00")}},
		"2026-09-29": {rise: map[string]float64{"7d": 0.1}},
	}
	days := peakingTo(framesNow, peaks)
	for i, d := range days {
		rose, ok := risen[d.Day]
		switch {
		case ok:
			days[i].Accounts = append(days[i].Accounts, rose.of("work"))
		case slices.Contains(limited, d.Day):
			days[i].Accounts = append(days[i].Accounts, windowsDay{limits: []ledger.Limit{limit("7d", d.Day, "18:00")}}.of("work"))
		case on(d.Day).Weekday() == time.Wednesday:
			days[i].Accounts = append(days[i].Accounts, account("side", sent("claude-opus-5-5", 1, inputUsage)))
		}
	}
	days[len(days)-1] = day("2026-10-01",
		windowsDay{highest: map[string]float64{"7d": 0.34}}.of("work"),
		windowsDay{highest: map[string]float64{"7d": 0.78}}.of("side"))
	return days
}

// pastOfWeeks is the Past of the days given, of work and side, at
// framesNow, weeks starting on the day given.
func pastOfWeeks(days []ledger.Summary, starts time.Weekday) views.Past {
	in := input(days)
	in.Now, in.Accounts, in.WeekStarts = framesNow, workAndSide, starts
	return views.NewPast(in, days)
}

// pastAt is the period of the length given at framesNow, stepped back by
// the keys given, as pressed has it, weeks starting on the day given.
func pastAt(t *testing.T, length views.Length, keys string, starts time.Weekday) views.Period {
	t.Helper()
	return pressed(t, length, keys, framesNow, starts, time.Time{})
}

// isOpen reports whether a peak is a week's so far.
func isOpen(p views.WeekPeak) bool {
	return p.Open
}

// A period gone tells its weeks as the weeks to this one tell them, the
// trailing window of the weeks at now: Detail's weeks, BY DAY and Compare's
// WEEKS end at its last week, each as now's over as many weeks more as it's
// back, those more dropped, so every week is whole, at its peak as the reset
// after it filed it, none so far, none of its days to come, and its weeks
// not ending at this week: a day a week back or more among them, as Sunday
// 27 September, and weeks starting on Sunday as on Monday.
func TestAPeriodGonesWeeksAreTheTrailingWindowOfTheWeeksAtNow(t *testing.T) {
	days := pastWeeks()
	for _, starts := range []time.Weekday{time.Monday, time.Sunday} {
		past := pastOfWeeks(days, starts)
		for _, length := range views.Lengths() {
			for _, keys := range []string{"<", "<<", "<<<", "<<<<"} {
				period := pastAt(t, length, keys, starts)
				back := period.SpanOn(framesNow, starts).WeeksBack
				if back == 0 {
					continue
				}
				t.Run(starts.String()+" "+length.Name()+" "+keys, func(t *testing.T) {
					page := past.Accounts(period)
					held, reached := past.WeeksOver(len(page.Weeks)+back), past.WeeksOver(len(page.Reach)+back)
					if got, want := page.Weeks, held.Weeks[:len(page.Weeks)]; !slices.Equal(got, want) {
						t.Errorf("the page's weeks are %v, want those at now, but the last %d, %v", got, back, want)
					}
					nows, reachedNow := append(slices.Clone(held.Accounts), held.All), append(slices.Clone(reached.Accounts), reached.All)
					for i, row := range append(slices.Clone(page.Rows), page.All) {
						if got, want := row.Peaks, nows[i].Peaks[:len(row.Peaks)]; !reflect.DeepEqual(got, want) {
							t.Errorf("row %d's peaks are %s, want those at now, %s", i, peaksRead(got), peaksRead(want))
						}
						if length != views.LengthTwelveMonths {
							if got, want := row.Reached, reachedNow[i].Peaks[:len(row.Reached)]; !reflect.DeepEqual(got, want) {
								t.Errorf("row %d's reach is %s, want the weeks at now's, %s", i, peaksRead(got), peaksRead(want))
							}
						}
						if slices.ContainsFunc(row.Peaks, isOpen) || slices.ContainsFunc(row.Reached, isOpen) {
							t.Errorf("row %d has a week so far: %s, reached %s", i, peaksRead(row.Peaks), peaksRead(row.Reached))
						}
						if len(row.Days) != 7*len(page.Weeks) {
							t.Errorf("row %d has %d days, want %d weeks' %d", i, len(row.Days), len(page.Weeks), 7*len(page.Weeks))
						}
					}
					if page.ToCome != 0 || page.ThisWeek {
						t.Errorf("%d days are to come, the weeks end at this week %v, want none to come, and not", page.ToCome, page.ThisWeek)
					}
					if len(page.Weeks) < page.Reaches || length != views.LengthTwelveMonths && len(page.Weeks) < len(page.Reach) {
						t.Errorf("the page holds %d weeks, want every one of the %d Detail reaches over and the %d of Compare's reach", len(page.Weeks), page.Reaches, len(page.Reach))
					}
				})
			}
		}
	}
}

// At Thursday 1 October, the weeks' peaks and facts are each period's:
// this week, to its own, so far; last week and the week of 14 September,
// to theirs, each whole, and Sunday 27 September, a day of last week, to
// last week; work's facts on Weeks' eight, side's on six, as the weeks they
// end at have them.
func TestAPeriodGonesWeeksAndFactsEndAtItsLastWeek(t *testing.T) {
	past := pastOfWeeks(pastWeeks(), time.Monday)
	tests := []struct {
		length               views.Length
		keys                 string
		weeks, work, side    string
		workFacts, sideFacts string
		toCome               int
		thisWeek             bool
	}{
		{length: views.LengthThisWeek, keys: "", weeks: "24 Aug … 28 Sep", work: "60 60 100 60 60 34…", side: "34 41 17 24 31 78…",
			workFacts: "1/7 34%", sideFacts: "0/5 71%", toCome: 3, thisWeek: true},
		{length: views.LengthThisWeek, keys: "<", weeks: "17 Aug … 21 Sep", work: "60 60 60 100 60 60", side: "27 34 41 17 24 31",
			workFacts: "2/8 30%", sideFacts: "0/6 71%"},
		{length: views.LengthThisWeek, keys: "<<", weeks: "10 Aug … 14 Sep", work: "60 60 60 60 100 60", side: "20 27 34 41 17 24",
			workFacts: "2/8 30%", sideFacts: "0/6 73%"},
		{length: views.LengthToday, keys: "<<<<", weeks: "17 Aug … 21 Sep", work: "60 60 60 100 60 60", side: "27 34 41 17 24 31",
			workFacts: "2/8 30%", sideFacts: "0/6 71%"},
	}
	for _, tt := range tests {
		period := pastAt(t, tt.length, tt.keys, time.Monday)
		t.Run(views.DialOf(period, framesNow, time.Monday, time.Time{}).Name(), func(t *testing.T) {
			page := past.Accounts(period)
			six := lastOf(page.Weeks, 6)
			got := fmt.Sprintf("%s … %s | %s | %s | %s | %s | %d %v", prose.DayMonth(civilDate(six[0])), prose.DayMonth(civilDate(six[5])),
				peaksRead(lastOf(page.Rows[0].Peaks, 6)), peaksRead(lastOf(page.Rows[1].Peaks, 6)),
				factsRead(page.Rows[0].Facts(8)), factsRead(page.Rows[1].Facts(6)), page.ToCome, page.ThisWeek)
			want := fmt.Sprintf("%s | %s | %s | %s | %s | %d %v", tt.weeks, tt.work, tt.side, tt.workFacts, tt.sideFacts, tt.toCome, tt.thisWeek)
			if got != want {
				t.Errorf("the weeks are\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// BY DAY of a period gone holds its weeks' days as they were and none after
// them: last week's, and Sunday 27 September's, hold work's week risen 20%
// on Tuesday 22 September, a 5-hour limit hit that day, but not the 10% it
// rose on the 29th, which now's holds, its days after today still to come;
// every account's are theirs together.
func TestByDayOfAPeriodGoneHoldsItsWeeksDays(t *testing.T) {
	past := pastOfWeeks(pastWeeks(), time.Monday)
	quiet, on22, on29 := views.DayUsed{}, views.DayUsed{Used: 0.2, Limits: 1}, views.DayUsed{Used: 0.1}
	lastWeek := []views.DayUsed{quiet, on22, quiet, quiet, quiet, quiet, quiet}
	tests := []struct {
		length views.Length
		keys   string
		weeks  []views.DayUsed
		toCome int
	}{
		{views.LengthThisWeek, "", slices.Concat(lastWeek, []views.DayUsed{quiet, on29, quiet, quiet, quiet, quiet, quiet}), 3},
		{views.LengthThisWeek, "<", slices.Concat(make([]views.DayUsed, 7), lastWeek), 0},
		{views.LengthToday, "<<<<", slices.Concat(make([]views.DayUsed, 7), lastWeek), 0},
	}
	for _, tt := range tests {
		period := pastAt(t, tt.length, tt.keys, time.Monday)
		t.Run(views.DialOf(period, framesNow, time.Monday, time.Time{}).Name(), func(t *testing.T) {
			page := past.Accounts(period)
			for _, row := range []views.AccountsRow{page.Rows[0], page.All} {
				if got := lastOf(row.Days, 14); !reflect.DeepEqual(got, tt.weeks) || page.ToCome != tt.toCome {
					t.Errorf("%q's last two weeks' days are %v, %d to come; want %v, %d", row.Totals.Account, got, page.ToCome, tt.weeks, tt.toCome)
				}
			}
		})
	}
}

// A period gone whose last week is this week, a day of it before today's,
// draws this week as now does: Detail's weeks, BY DAY and Compare's WEEKS
// are now's, this week so far and its days to come among them; but its
// worth marks no bucket as now's.
func TestAPeriodGoneWithinThisWeekDrawsThisWeekAsNow(t *testing.T) {
	past := pastOfWeeks(pastWeeks(), time.Monday)
	today := past.Accounts(views.LengthToday.Now())
	weeks := func(page views.AccountsPage) string {
		rows := append(slices.Clone(page.Rows), page.All)
		return jsonOf(t, []any{page.Weeks, page.ToCome, page.ThisWeek, page.Reaches, page.Reach, rows[0].Peaks, rows[0].Reached, rows[0].Days,
			rows[1].Peaks, rows[1].Reached, rows[2].Peaks, rows[2].Reached, rows[0].WeekLeft, rows[1].WeekLeft})
	}
	for _, keys := range []string{"<", "<<"} {
		period := pastAt(t, views.LengthToday, keys, time.Monday)
		t.Run(views.DialOf(period, framesNow, time.Monday, time.Time{}).Name(), func(t *testing.T) {
			page := past.Accounts(period)
			if got, want := weeks(page), weeks(today); got != want {
				t.Errorf("the weeks are\n%s\nwant today's\n%s", got, want)
			}
			if !page.ThisWeek || page.ToCome != 3 || !page.Rows[0].Peaks[len(page.Rows[0].Peaks)-1].Open {
				t.Errorf("this week ends the weeks %v, %d days to come, work's last %+v; want this week's, 3 to come, so far",
					page.ThisWeek, page.ToCome, page.Rows[0].Peaks[len(page.Rows[0].Peaks)-1])
			}
			if by := page.Rows[0].WorthBy[0]; by.Now() != -1 || !by.Gone {
				t.Errorf("work's worth marks bucket %d as now's, gone %v, want none, gone", by.Now(), by.Gone)
			}
		})
	}
}

// A period gone's reach ends at its last week: as many weeks as now's at
// its length, the weeks that begin in it but never fewer than eight; or at
// 12 months, a column a month, the 13 months the period touches, its last
// a month no week has begun in yet. 12 months back, side's months are each
// as full as its fullest week, the week of its last Monday, none so far;
// work's facts are on the 52 whole weeks beginning in it, its limit hit in
// 12, the weeks of each month's first Monday.
func TestAPeriodGonesReachEndsAtItsLastWeek(t *testing.T) {
	past := pastOfWeeks(pastWeeks(), time.Monday)
	tests := []struct {
		length                       views.Length
		keys                         string
		columns                      int
		first, last, firstAt, lastAt string
	}{
		{views.LengthToday, "<<", 8, "10 Aug", "28 Sep", "2026-08-10", "2026-09-28"},
		{views.LengthThisWeek, "<", 8, "3 Aug", "21 Sep", "2026-08-03", "2026-09-21"},
		{views.LengthThisWeek, "<<", 8, "27 Jul", "14 Sep", "2026-07-27", "2026-09-14"},
		{views.LengthThirtyDays, "<", 8, "13 Jul", "31 Aug", "2026-07-13", "2026-08-31"},
		{views.LengthTwelveWeeks, "<", 12, "20 Apr", "6 Jul", "2026-04-20", "2026-07-06"},
		{views.LengthTwelveMonths, "<", 13, "Oct", "Oct", "2024-10-01", "2025-10-01"},
	}
	for _, tt := range tests {
		period := pastAt(t, tt.length, tt.keys, time.Monday)
		t.Run(views.DialOf(period, framesNow, time.Monday, time.Time{}).Name(), func(t *testing.T) {
			reach := past.Accounts(period).Reach
			got := fmt.Sprintf("%d %s %s %s %s", len(reach), reach[0].Label, reach[len(reach)-1].Label,
				reach[0].Start.Format(time.DateOnly), reach[len(reach)-1].Start.Format(time.DateOnly))
			if want := fmt.Sprintf("%d %s %s %s %s", tt.columns, tt.first, tt.last, tt.firstAt, tt.lastAt); got != want {
				t.Errorf("the reach is %s, want %s", got, want)
			}
		})
	}
	page := past.Accounts(pastAt(t, views.LengthTwelveMonths, "<", time.Monday))
	if got, want := peaksRead(page.Rows[1].Reached), "38 35 40 37 34 41 38 36 40 38 35 39 —"; got != want {
		t.Errorf("12 months back, side's months are %s, want each its last Monday's week, %s", got, want)
	}
	if work := page.Rows[0].Reaching; work.Weeks != 52 || work.Limits != 12 {
		t.Errorf("12 months back, work's facts are %s, want its limit hit in 12 of 52 whole weeks", verdictRead(work))
	}
	if page.ThisWeek {
		t.Error("12 months back, the weeks end at this week")
	}
}

// A period gone's worth is charted over its own days, its buckets summing
// to its worth, marking none as now's and counting none as to come,
// whatever its length; at now, each chart marks its own, as ever.
func TestAPeriodGonesWorthMarksNoBucketAsNows(t *testing.T) {
	past := pastOfWeeks(pastWeeks(), time.Monday)
	for _, length := range views.Lengths() {
		for _, keys := range []string{"", "<", "<<"} {
			period := pastAt(t, length, keys, time.Monday)
			gone := keys != ""
			t.Run(length.Name()+" "+keys, func(t *testing.T) {
				page := past.Accounts(period)
				span := period.SpanOn(framesNow, time.Monday)
				for _, row := range append(slices.Clone(page.Rows), page.All) {
					for _, by := range row.WorthBy {
						first, last := by.Buckets[0].Start.Format(time.DateOnly), by.Buckets[len(by.Buckets)-1].Start.Format(time.DateOnly)
						if by.Gone != gone || gone && (by.Now() != -1 || by.ToCome != 0) || !gone && by.Now() < 0 || last > span.Last {
							t.Errorf("%q's chart by %d, %s to %s, is gone %v, its now %d, %d to come; want gone %v, to %s",
								row.Totals.Account, by.Unit, first, last, by.Gone, by.Now(), by.ToCome, gone, span.Last)
						}
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
}

// A day's page fed the fold of that day's lines charts its worth by that
// day's hours, a day gone's too, none of them now's or to come; fed another
// day's, it's charted by the day. Projects' page of a day gone, of its
// session that day, handed today's hours, charts it by the day, as they're
// not its own.
func TestADaysHoursChartThatDayAlone(t *testing.T) {
	in := input(nil)
	in.Accounts = workAndSide
	yesterday, _ := views.LengthToday.Now().Back(now, time.Monday, time.Time{})
	page := views.NewPast(in, nil).Accounts(yesterday)
	itsFold := heldDays(october(6, 23, 0), projectAsks(repoSession, "/code/switchboard", "", "work", october(6, 22, 0))...)
	todays := heldDays(now, projectAsks(repoSession, "/code/switchboard", "", "work", october(6, 22, 0), october(7, 9, 0))...)
	by := page.Hourly(itsFold.TodayFold()).Rows[0].WorthBy[0]
	if by.Unit != views.ByHour || dayfile.DateOf(by.Buckets[0].Start) != "2026-10-06" || by.ToCome != 0 || by.Now() != -1 ||
		!reflect.DeepEqual(by.Buckets[22].Worth, hoursWorth(1)) {
		t.Errorf("fed its fold, yesterday's work is charted by %d from %v, %d to come, now %d, 22:00 worth %+v; want by the hour of 6 Oct, none now's, 22:00's request",
			by.Unit, by.Buckets[0].Start, by.ToCome, by.Now(), by.Buckets[22].Worth)
	}
	if got := chartsFrom(page.Hourly(todays.TodayFold()).Rows[0].WorthBy); !reflect.DeepEqual(got, []chartFrom{{views.ByDay, 1, "2026-10-06"}}) {
		t.Errorf("fed today's fold, yesterday's work is charted %+v, want by the day", got)
	}
	project, _ := todays.Projects(projectsIn(yesterday, now)).Project("/code/switchboard")
	if got := chartsFrom(project.WorthBy); !reflect.DeepEqual(got, []chartFrom{{views.ByDay, 1, "2026-10-06"}}) || project.WorthBy[0].Now() != -1 {
		t.Errorf("handed today's hours, the project's yesterday is charted %+v, now %d; want by the day, none now's", got, project.WorthBy[0].Now())
	}
}

// resettingWeeks are four accounts' days to framesNow's: work's week
// resetting at 00:30 each Monday from 7 September, its limit reached on
// the 11th and the 24th; side's at 10:00 each Thursday from 10 September,
// its limit reached on the 30th; spare's, idle but for one that reset on
// Monday 3 August; and personal's at 09:00 each Saturday from 5 September,
// each own week placed under the calendar week after the one it began in,
// with weeks from Monday.
func resettingWeeks() []ledger.Summary {
	ended := func(id, date, clock string, before float64) ledger.AccountDay {
		return windowsDay{resets: []ledger.Reset{reset("7d", date, clock, before)}}.of(id)
	}
	limited := func(id, date string) ledger.AccountDay {
		return windowsDay{limits: []ledger.Limit{limit("7d", date, "18:00")}}.of(id)
	}
	return daysTo("2026-07-27", framesNow,
		day("2026-08-03", ended("spare", "2026-08-03", "10:00", 0.5)),
		day("2026-09-05", ended("personal", "2026-09-05", "09:00", 0.5)),
		day("2026-09-07", ended("work", "2026-09-07", "00:30", 0.5)),
		day("2026-09-10", ended("side", "2026-09-10", "10:00", 0.4)),
		day("2026-09-11", limited("work", "2026-09-11")),
		day("2026-09-12", ended("personal", "2026-09-12", "09:00", 0.8)),
		day("2026-09-14", ended("work", "2026-09-14", "00:30", 1)),
		day("2026-09-17", ended("side", "2026-09-17", "10:00", 0.6)),
		day("2026-09-19", ended("personal", "2026-09-19", "09:00", 0.65)),
		day("2026-09-21", ended("work", "2026-09-21", "00:30", 0.7)),
		day("2026-09-24", limited("work", "2026-09-24"), ended("side", "2026-09-24", "10:00", 0.9)),
		day("2026-09-26", ended("personal", "2026-09-26", "09:00", 0.45)),
		day("2026-09-28", ended("work", "2026-09-28", "00:30", 1)),
		day("2026-09-30", limited("side", "2026-09-30")),
		day("2026-10-01", ended("side", "2026-10-01", "10:00", 1)),
	)
}

// Left at its last reset, of a period gone, is what the week had left as
// it reset at the end of the period's last week, the reset that ended the
// account's own week placed under it, though that came after the period's
// last day, as work's on the Monday after and side's on the Thursday, or
// came in it, as personal's, its own week placed under the week after the
// one it began in: so of a period gone it's what the last WEEKS column
// left, where that column's peak is known. At now, it's as the week last
// reset. Each is looked for in the six weeks to the period's last week and
// the week before them: spare's of 3 August is in those of the week of 14
// September, and of the week before, but not in last week's or now's. With
// weeks from Sunday, side's own week from Thursday 24 September is placed
// under this week, so last week's is the one before.
func TestLeftAtItsLastResetOfAPeriodGoneIsAsItsLastWeekReset(t *testing.T) {
	days := resettingWeeks()
	tests := []struct {
		starts                            time.Weekday
		keys, work, side, spare, personal string
	}{
		{time.Monday, "", "0% ran out 2026-09-24", "0% ran out 2026-09-30", "—", "55%"},
		{time.Monday, "<", "0% ran out 2026-09-24", "0% ran out 2026-09-30", "—", "55%"},
		{time.Monday, "<<", "30%", "10%", "50%", "35%"},
		{time.Monday, "<<<", "0% ran out 2026-09-11", "40%", "50%", "20%"},
		{time.Sunday, "<", "0% ran out 2026-09-24", "10%", "—", "55%"},
	}
	left := func(row views.AccountsRow) string {
		if row.RanOut == "" {
			return shareRead(row.WeekLeft)
		}
		return shareRead(row.WeekLeft) + " ran out " + row.RanOut
	}
	for _, tt := range tests {
		in := input(days)
		in.Now, in.WeekStarts = framesNow, tt.starts
		in.Accounts = config.Accounts{{ID: "work"}, {ID: "side"}, {ID: "spare"}, {ID: "personal"}}
		period := pastAt(t, views.LengthThisWeek, tt.keys, tt.starts)
		t.Run(tt.starts.String()+" "+views.DialOf(period, framesNow, tt.starts, time.Time{}).Name(), func(t *testing.T) {
			page := views.NewPast(in, days).Accounts(period)
			got := [5]string{left(page.Rows[0]), left(page.Rows[1]), left(page.Rows[2]), left(page.Rows[3]), shareRead(page.All.WeekLeft)}
			if want := [5]string{tt.work, tt.side, tt.spare, tt.personal, "—"}; got != want {
				t.Errorf("work, side, spare, personal and every account left %q, want %q", got, want)
			}
			for _, row := range page.Rows {
				if last := row.Peaks[len(row.Peaks)-1]; tt.keys != "" && last.Share != nil && shareRead(row.WeekLeft) != prose.Percent(1-*last.Share) {
					t.Errorf("%q left %s, want what its last week's peak, %s, left", row.Totals.Account, shareRead(row.WeekLeft), prose.Percent(*last.Share))
				}
			}
		})
	}
}

// An Accounts page reads the days from lookBack before the earlier of its
// period's first day and the first of the weeks it holds, as many as 12
// months to its last week reach over: at now, those of the 12 months to
// today; gone, those to its last day, its last week's last of a week
// length.
func TestAccountsFromIsTheFirstDayAPeriodsPageReads(t *testing.T) {
	tests := []struct {
		length views.Length
		keys   string
		want   string
	}{
		{views.LengthToday, "", "2025-09-29"},
		{views.LengthToday, "<", "2025-09-29"},
		{views.LengthThisWeek, "", "2025-09-29"},
		{views.LengthThisWeek, "<", "2025-09-22"},
		{views.LengthThisWeek, "<<", "2025-09-15"},
		{views.LengthThirtyDays, "", "2025-09-29"},
		{views.LengthThirtyDays, "<", "2025-09-01"},
		{views.LengthTwelveWeeks, "", "2025-09-29"},
		{views.LengthTwelveWeeks, "<", "2025-07-07"},
		{views.LengthTwelveMonths, "", "2025-09-25"},
		{views.LengthTwelveMonths, "<", "2024-09-25"},
	}
	for _, tt := range tests {
		period := pastAt(t, tt.length, tt.keys, time.Monday)
		t.Run(views.DialOf(period, framesNow, time.Monday, time.Time{}).Name(), func(t *testing.T) {
			from := period.AccountsFrom(framesNow, time.Monday)
			if got := from.Format(time.DateOnly); got != tt.want || !from.Equal(on(tt.want)) {
				t.Errorf("the page reads from %v, want the start of %s", from, tt.want)
			}
		})
	}
}

// An Accounts page built of the days from AccountsFrom is the page built of
// every day, at now and gone, of every length.
func TestAnAccountsPageNeedsNoDayBeforeAccountsFrom(t *testing.T) {
	days := pastWeeks()
	every := pastOfWeeks(days, time.Monday)
	for _, length := range views.Lengths() {
		for _, keys := range []string{"", "<", "<<"} {
			period := pastAt(t, length, keys, time.Monday)
			t.Run(length.Name()+" "+keys, func(t *testing.T) {
				from := dayfile.DateOf(period.AccountsFrom(framesNow, time.Monday))
				read := slices.DeleteFunc(slices.Clone(days), func(s ledger.Summary) bool { return s.Day < from })
				if got, want := jsonOf(t, pastOfWeeks(read, time.Monday).Accounts(period)), jsonOf(t, every.Accounts(period)); got != want {
					t.Errorf("built of the days from %s, the page is\n%s\nwant it as built of every day\n%s", from, got, want)
				}
			})
		}
	}
}

// At now, the days read for the last 12 months, as DaysFrom has them, hold
// every length's Accounts page, on any day, weeks starting on any day.
func TestTheDaysReadAtNowHoldEveryLengthsAccountsPage(t *testing.T) {
	for d := range 14 {
		at := framesNow.AddDate(0, 0, d)
		for starts := time.Sunday; starts <= time.Saturday; starts++ {
			read := views.DaysFrom(0, at, starts)
			for _, length := range views.Lengths() {
				if from := length.Now().AccountsFrom(at, starts); from.Before(read) {
					t.Errorf("on %s, weeks from %s, %s's page reads from %s, before the days read from %s",
						at.Format(time.DateOnly), starts, length.Name(), from.Format(time.DateOnly), read.Format(time.DateOnly))
				}
			}
		}
	}
}

// Stood at a moment, a period gone files the reset its document reads and
// the days don't hold yet, as now does: last week, at 21:00:01 on Monday 5
// October, before the router reads work's week again, has it ended at its
// limit, as once the reset is filed, and as it isn't at 20:59:59.
func TestAPeriodGoneStoodAtAMomentFilesTheResetThatEndedItsLastWeek(t *testing.T) {
	lastWeek := func(name string) views.AccountsPage {
		am := accountsMoment(t, name)
		am.Period = pressed(t, views.LengthThisWeek, "<", am.Now, am.WeekStarts, time.Time{})
		page, _ := accountsAt(am)
		return page
	}
	before, unread, filed := lastWeek(aSecondBefore), lastWeek(beforeTheRead), lastWeek(onceItsReFiled)
	if got, want := weeksSaid(unread), weeksSaid(filed); got != want {
		t.Errorf("before the reread, last week says\n%s\nwant it as once the reset is filed\n%s", got, want)
	}
	if got, was := weeksSaid(unread), weeksSaid(before); got == was {
		t.Errorf("at 21:00:01, last week says as at 20:59:59\n%s\nwant it changed by the reset", got)
	}
	if work := unread.Rows[0]; shareRead(work.WeekLeft) != "0%" || work.Peaks[len(work.Peaks)-1].Open {
		t.Errorf("at 21:00:01, work's last week left %s, its peak %+v; want 0%%, whole", shareRead(work.WeekLeft), work.Peaks[len(work.Peaks)-1])
	}
}

// At each of Accounts' hard moments, a period gone's totals, of every
// length a step back and two, are history's over the same days: asked for
// from its first day, the ledger holding the days to its last, at the
// moment, so priced alike, each window's last run told from the days before
// it too. What its plans cost is history's asked for on its last day, a
// period gone priced to its last day, as now's is.
func TestAPeriodGonesTotalsAndPlansAreHistorysOverItsDays(t *testing.T) {
	for _, am := range viewstest.AccountsMoments() {
		for _, length := range views.Lengths() {
			for _, keys := range []string{"<", "<<"} {
				am.Period = pressed(t, length, keys, am.Now, am.WeekStarts, time.Time{})
				t.Run(am.Name+"/"+length.Name()+" "+keys, func(t *testing.T) {
					page, _ := accountsAt(am)
					last := am.Period.SpanOn(am.Now, am.WeekStarts).Last
					in := accountsIn(am)
					in.From = am.Period.From(am.Now, am.WeekStarts)
					in.Ledger = ledgerOf(slices.DeleteFunc(slices.Clone(am.Days), func(s ledger.Summary) bool { return s.Day > last }))
					counted := views.NewHistory(in)
					in.Now = on(last).Add(12 * time.Hour)
					priced := views.NewHistory(in)
					for i, row := range append(slices.Clone(page.Rows), page.All) {
						if got, want := jsonOf(t, row.Totals), jsonOf(t, counted.Totals.Accounts[i]); got != want {
							t.Errorf("row %d counts\n%s\nhistory counts\n%s", i, got, want)
						}
						if got, want := jsonOf(t, []any{row.Plan.Price, row.Plan.Cost}), jsonOf(t, []any{priced.Plans[i].Price, priced.Plans[i].Cost}); got != want {
							t.Errorf("row %d's plan costs %s, history's %s", i, got, want)
						}
					}
				})
			}
		}
	}
}
