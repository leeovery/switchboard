package views_test

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// threeAccounts are work, personal and side, as Accounts' frames configure
// them.
var threeAccounts = config.Accounts{{ID: "work", Plan: "ultra"}, {ID: "personal", Primary: true, Plan: "ultra"}, {ID: "side", Plan: "ultra"}}

// accountsPageOf is Accounts' page over this week of the days given, the
// accounts configured as given, now Wednesday 7 October.
func accountsPageOf(days []ledger.Summary, configured config.Accounts) views.AccountsPage {
	in := input(days)
	in.Accounts = configured
	return views.NewPast(in, days).Accounts(views.PeriodThisWeek)
}

// shareRead reads a share as a whole percentage: "—" where it isn't known.
func shareRead(share *float64) string {
	if share == nil {
		return "—"
	}
	return prose.Percent(*share)
}

// peaksRead reads weeks' peaks as Detail's WEEKS draws them: whole
// percentages, this week's with … after it, and — where a peak isn't known.
func peaksRead(peaks []views.WeekPeak) string {
	read := make([]string, len(peaks))
	for i, p := range peaks {
		switch {
		case p.Share == nil:
			read[i] = "—"
		case p.Open:
			read[i] = strconv.Itoa(prose.WholePercent(*p.Share)) + "…"
		default:
			read[i] = strconv.Itoa(prose.WholePercent(*p.Share))
		}
	}
	return strings.Join(read, " ")
}

// verdictRead reads a verdict, its limits over its weeks and its share left
// on average, as "ends most weeks with room 2/6 23%": "0 weeks" over none.
func verdictRead(v views.AccountVerdict) string {
	if v.Weeks == 0 {
		return "0 weeks"
	}
	return v.Verdict + " " + factsRead(v)
}

// factsRead reads a verdict's facts alone, its limits over its weeks and its
// share left on average, as "2/6 23%": "0 weeks" over none.
func factsRead(v views.AccountVerdict) string {
	if v.Weeks == 0 {
		return "0 weeks"
	}
	return fmt.Sprintf("%d/%d %s", v.Limits, v.Weeks, shareRead(v.LeftAtReset))
}

// weeksJudged are the weeks Weeks' own page shows, whose whole weeks its
// verdict is over: this one and the seven before it.
const weeksJudged = 8

// lastOf are the last n of the things given, every one where there are
// fewer.
func lastOf[T any](all []T, n int) []T {
	return all[max(len(all)-n, 0):]
}

// Detail's WEEKS and facts are the frames': each account's last six weeks'
// peaks, this week's so far, and Weeks' facts on the last eight, Weeks' own,
// over their 7 whole weeks, its limits and what it left at a reset, on
// average; every account's, the mean of their peaks, the weeks some account
// was short and what every account's weeks left. The page holds the weeks
// of the last 12 months, the 52 to this one.
func TestDetailsWeeksAndFactsAreTheFrames(t *testing.T) {
	page := framesPage()
	tests := []struct {
		name         string
		row          views.AccountsRow
		peaks, facts string
	}{
		{name: "work", row: page.Rows[0], peaks: "100 94 100 81 100 34…", facts: "3/7 9%"},
		{name: "personal", row: page.Rows[1], peaks: "58 77 69 90 85 66…", facts: "0/7 27%"},
		{name: "side", row: page.Rows[2], peaks: "41 30 52 47 60 78…", facts: "0/7 59%"},
		{name: "all accounts", row: page.All, peaks: "66 67 74 73 82 59…", facts: "3/7 32%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := peaksRead(lastOf(tt.row.Peaks, 6)); got != tt.peaks {
				t.Errorf("%s's weeks are %s, want %s", tt.name, got, tt.peaks)
			}
			if got := factsRead(tt.row.Facts(weeksJudged)); got != tt.facts {
				t.Errorf("%s's facts are %s, want %s", tt.name, got, tt.facts)
			}
			if got := tt.row.Facts(weeksJudged); got.All != (tt.name == "all accounts") {
				t.Errorf("%s's facts are every account's: %v", tt.name, got.All)
			}
		})
	}
	if want := []string{"2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28", "2026-10-05"}; !slices.Equal(lastOf(page.Weeks, 6), want) {
		t.Errorf("the last six weeks are %v, want %v", lastOf(page.Weeks, 6), want)
	}
	if len(page.Weeks) != 52 || page.Weeks[0] != "2025-10-13" || len(page.Rows[0].Peaks) != 52 {
		t.Errorf("the page holds %d weeks from %s, %d peaks, want the 52 from 13 Oct 2025", len(page.Weeks), page.Weeks[0], len(page.Rows[0].Peaks))
	}
}

// framesPage is Accounts' page over this week of the frames' weeks: work's,
// personal's and side's last seven, and this week's so far, now Wednesday 7
// October.
func framesPage() views.AccountsPage {
	days := peaking(map[string][]float64{
		"work":     {0.72, 0.88, 1, 0.94, 1, 0.81, 1},
		"personal": {0.64, 0.70, 0.58, 0.77, 0.69, 0.90, 0.85},
		"side":     {0.22, 0.35, 0.41, 0.30, 0.52, 0.47, 0.60},
	})
	days[len(days)-1] = day("2026-10-07",
		windowsDay{highest: map[string]float64{"7d": 0.66}}.of("personal"),
		windowsDay{highest: map[string]float64{"7d": 0.78}}.of("side"),
		windowsDay{highest: map[string]float64{"7d": 0.34}}.of("work"))
	return accountsPageOf(days, threeAccounts)
}

// Weeks' facts on the last weeks asked for are on the whole weeks among
// them alone, this week never one: the last four are the three before this
// one; this week alone is none; and more than the page holds are every one.
func TestFactsAreOnTheWholeWeeksAmongTheLastAsked(t *testing.T) {
	page := framesPage()
	tests := []struct {
		name  string
		row   views.AccountsRow
		weeks int
		want  string
	}{
		{"work's last four", page.Rows[0], 4, "2/3 6%"},
		{"personal's last four", page.Rows[1], 4, "0/3 19%"},
		{"every account's last four", page.All, 4, "2/3 24%"},
		{"work's this week alone", page.Rows[0], 1, "0 weeks"},
		{"work's every week", page.Rows[0], 100, "3/7 9%"},
	}
	for _, tt := range tests {
		if got := factsRead(tt.row.Facts(tt.weeks)); got != tt.want {
			t.Errorf("%s: the facts are %s, want %s", tt.name, got, tt.want)
		}
	}
}

// Detail's weeks section reaches over the weeks that begin in the period,
// this one last, but never fewer than six: six at today, this week and 30
// days, 12 at 12 weeks and 52 at 12 months; and so it does at the fewest
// whatever the day, before any page is read.
func TestDetailReachesOverThePeriodsWeeksAtLeastSix(t *testing.T) {
	want := map[views.Period]int{views.PeriodToday: 6, views.PeriodThisWeek: 6, views.PeriodThirtyDays: 6, views.PeriodTwelveWeeks: 12, views.PeriodTwelveMonths: 52}
	for _, period := range views.Periods() {
		days := daysFrom("2026-10-07")
		in := input(days)
		page := views.NewPast(in, days).Accounts(period)
		if page.Reaches != want[period] || period.WeeksLeast() != want[period] {
			t.Errorf("at %s, Detail reaches over %d weeks, at the fewest %d, want %d", period.Name(), page.Reaches, period.WeeksLeast(), want[period])
		}
	}
}

// Each account's row carries its label as the config gives it, whatever its
// id; every account's carries none.
func TestEachRowCarriesItsAccountsLabel(t *testing.T) {
	labelled := config.Accounts{{ID: "acct-1", Label: "work"}, {ID: "acct-2", Label: "side"}, {ID: "acct-3"}}
	page := accountsPageOf(daysFrom("2026-10-07"), labelled)
	var got []string
	for _, row := range append(page.Rows, page.All) {
		got = append(got, row.Totals.Account+":"+row.Label)
	}
	if want := []string{"acct-1:work", "acct-2:side", "acct-3:", ":"}; !slices.Equal(got, want) {
		t.Errorf("the rows are of %q, want %q", got, want)
	}
}

// The versions any account used in the period are listed in the version
// table's order, each as the table names and shades it, its ids with it, so
// each row's shares can be found by it; one the table doesn't name, by its
// id and its family alone.
func TestThePeriodsVersionsAreTheTablesInItsOrder(t *testing.T) {
	days := daysFrom("2026-10-05", day("2026-10-06",
		account("side", sent("claude-haiku-4-5", 1, inputUsage), sent("claude-opus-9", 1, inputUsage)),
		account("work", sent("claude-opus-5-5", 1, inputUsage)),
	))
	page := accountsPageOf(days, workAndSide)
	var got []string
	for _, v := range page.Versions {
		got = append(got, fmt.Sprintf("%s/%s/%d/%s", v.Name, v.Family, v.Shade, strings.Join(v.IDs, ",")))
	}
	want := []string{
		"Opus 5.5/opus/1/claude-opus-5-5",
		"Haiku 4.5/haiku/0/claude-haiku-4-5-20251001,claude-haiku-4-5",
		"opus-9/opus/0/claude-opus-9",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the period's versions are %q, want %q", got, want)
	}
}

// The page's worth is priced as of the price table's date, as help gives it,
// with nothing recorded as with days.
func TestThePagesPricesAreAsOfThePriceTablesDate(t *testing.T) {
	for _, days := range [][]ledger.Summary{nil, daysFrom("2026-10-07")} {
		if got := accountsPageOf(days, threeAccounts).PricesAsOf; got != ledger.Pricing.AsOf {
			t.Errorf("of %d days, the page's prices are as of %q, want %q", len(days), got, ledger.Pricing.AsOf)
		}
	}
}

// BY DAY shades each account's days on its own scale, the quarters of its
// own active days, where Days' BY ACCOUNT shades every account on one: so
// side, which used a tenth of what work did each day, is shaded as work is.
// Every account's days are their points together, on a scale of their own,
// marked where any hit a 5-hour limit.
func TestByDayShadesEachAccountOnItsOwnScale(t *testing.T) {
	rises := []float64{0.04, 0.08, 0.12, 0.16}
	var given []ledger.Summary
	for i, rise := range rises {
		date := on("2026-10-05").AddDate(0, 0, -7*5+i).Format(time.DateOnly)
		var limits []ledger.Limit
		if i == 1 {
			limits = []ledger.Limit{limit("5h", date, "15:00")}
		}
		given = append(given, day(date, rose("work", map[string]float64{"7d": rise}, limits...), rose("side", map[string]float64{"7d": rise / 10})))
	}
	page := accountsPageOf(daysFrom("2026-08-24", given...), workAndSide)

	levels := func(days []views.StripDay) []int {
		var got []int
		for _, d := range lastOf(days, 7*6)[:4] {
			got = append(got, d.Level)
		}
		return got
	}
	if work, side := levels(page.Rows[0].Days), levels(page.Rows[1].Days); !slices.Equal(work, []int{1, 2, 3, 4}) || !slices.Equal(side, work) {
		t.Errorf("work's first days are at levels %v and side's %v, want each 1, 2, 3, 4 on its own scale", work, side)
	}
	if got := levels(page.All.Days); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("every account's first days are at levels %v, want 1, 2, 3, 4", got)
	}
	marked := func(days []views.StripDay) []bool {
		var got []bool
		for _, d := range lastOf(days, 7*6)[:4] {
			got = append(got, d.Limit)
		}
		return got
	}
	if want := []bool{false, true, false, false}; !slices.Equal(marked(page.Rows[0].Days), want) || !slices.Equal(marked(page.All.Days), want) ||
		slices.Contains(marked(page.Rows[1].Days), true) {
		t.Errorf("the 5-hour limits are marked on work's %v, side's %v and every account's %v, want work's second day and every account's",
			marked(page.Rows[0].Days), marked(page.Rows[1].Days), marked(page.All.Days))
	}
	// Days' eight weeks begin two before BY DAY's six.
	shared := daysOf(daysFrom("2026-08-24", given...)).ByAccount[1].Days[7*2:]
	if got := levels(shared); !slices.Equal(got, []int{1, 1, 2, 2}) {
		t.Errorf("Days' BY ACCOUNT shades side's first days at levels %v, want 1, 1, 2, 2, every account on one scale", got)
	}
}

// BY DAY is the page's weeks to this one, those that begin in the last 12
// months, 53 on the Monday a year back is one and 52 after, a day at a
// time, its days to come counted, as many as are left of this week on each
// of its days; a week of no use is its days at level 0.
func TestByDayIsThePagesWeeksToThisOneItsDaysToComeCounted(t *testing.T) {
	for i := range 7 {
		now := on("2026-10-05").AddDate(0, 0, i).Add(13 * time.Hour)
		days := daysTo("2026-10-01", now, day("2026-10-01", rose("work", map[string]float64{"7d": 0.1})))
		in := input(days)
		in.Now = now
		page := views.NewPast(in, days).Accounts(views.PeriodThisWeek)
		weeks, first := 52, "2025-10-13"
		if i == 0 {
			weeks, first = 53, "2025-10-06"
		}
		t.Run(now.Weekday().String(), func(t *testing.T) {
			if page.ToCome != 6-i || len(page.Rows[0].Days) != 7*len(page.Weeks) || len(page.Weeks) != weeks || page.Weeks[0] != first ||
				page.Weeks[len(page.Weeks)-1] != "2026-10-05" {
				t.Errorf("on %v, %d of %d days are to come, of %d weeks from %s, want %d of %d weeks' from %s to 5 Oct",
					now.Weekday(), page.ToCome, len(page.Rows[0].Days), len(page.Weeks), page.Weeks[0], 6-i, weeks, first)
			}
			lastSix := lastOf(page.Rows[0].Days, 7*6)
			if slices.ContainsFunc(lastSix[:7*4], func(d views.StripDay) bool { return d.Level > 0 }) {
				t.Errorf("the weeks of no use are at levels %v, want 0", lastSix[:7*4])
			}
		})
	}
}

// A period records anything where any account's days in it record a
// request, a limit, or a minute at its cap or at a limit: windows read alone,
// and minutes read as none, record nothing.
func TestAPeriodRecordsARequestALimitOrAMinuteAtACapOrALimit(t *testing.T) {
	minutes := func(atCap, atLimit int) ledger.AccountDay {
		return ledger.AccountDay{Account: "side", MinutesAtCap: new(atCap), MinutesAtLimit: new(atLimit)}
	}
	tests := []struct {
		name     string
		day      ledger.AccountDay
		recorded bool
	}{
		{name: "windows read alone", day: windowsDay{highest: map[string]float64{"7d": 0.4}}.of("side")},
		{name: "minutes read as none", day: minutes(0, 0)},
		{name: "a request", day: account("side", sent("claude-opus-5-5", 1, inputUsage)), recorded: true},
		{name: "a limit", day: windowsDay{limits: []ledger.Limit{limit("5h", "2026-10-07", "09:00")}}.of("side"), recorded: true},
		{name: "a minute at its cap", day: minutes(1, 0), recorded: true},
		{name: "a minute at a limit", day: minutes(0, 1), recorded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := accountsPageOf(daysFrom("2026-10-05", day("2026-10-07", tt.day)), workAndSide)
			if page.Recorded != tt.recorded {
				t.Errorf("the period records anything: %v, want %v", page.Recorded, tt.recorded)
			}
		})
	}
}

// Left at its last reset is what an account's week had left as it last
// reset, whatever the period, and the day its week ran out where it reached
// its week's limit then: none of every account's, and none where none of
// its weeks ended in the six weeks.
func TestLeftAtItsLastResetIsItsLastWeeksWhateverThePeriod(t *testing.T) {
	days := daysFrom("2026-09-20",
		day("2026-09-23", windowsDay{limits: []ledger.Limit{limit("7d", "2026-09-23", "18:00"), limit("5h", "2026-09-27", "12:00")}}.of("work")),
		day("2026-09-28", windowsDay{resets: []ledger.Reset{reset("7d", "2026-09-28", "00:30", 1)}}.of("work"),
			windowsDay{resets: []ledger.Reset{reset("7d", "2026-09-28", "10:00", 0.85)}}.of("side")))
	in := input(days)
	in.Accounts = workAndSide
	for _, period := range []views.Period{views.PeriodToday, views.PeriodThisWeek, views.PeriodTwelveWeeks, views.PeriodTwelveMonths} {
		t.Run(period.Name(), func(t *testing.T) {
			page := views.NewPast(in, days).Accounts(period)
			work, side := page.Rows[0], page.Rows[1]
			if shareRead(work.WeekLeft) != "0%" || work.RanOut != "2026-09-23" || shareRead(side.WeekLeft) != "15%" || side.RanOut != "" ||
				page.All.WeekLeft != nil || page.All.RanOut != "" {
				t.Errorf("work left %s, ran out %q; side %s, %q; every account %v, %q: want 0%% on 23 Sep, 15%%, and none",
					shareRead(work.WeekLeft), work.RanOut, shareRead(side.WeekLeft), side.RanOut, page.All.WeekLeft, page.All.RanOut)
			}
		})
	}
}

// Left at its last reset, and the day the week ran out, are of a week that
// ended among the six weeks' days and the week before them, the days from
// Monday 24 August: an account idle since one ended before, in the two weeks
// before those that Weeks judges too, or the days read before them, has
// neither, as Compare says "—".
func TestLeftAtItsLastResetIsOfAWeekEndedInTheSixWeeksOrTheWeekBefore(t *testing.T) {
	tests := []struct {
		name, ended, left, ranOut string
	}{
		{name: "in the days read before the eight weeks", ended: "2026-08-12", left: "—"},
		{name: "in the first of the eight weeks", ended: "2026-08-20", left: "—"},
		{name: "in the week before the six", ended: "2026-08-27", left: "0%", ranOut: "2026-08-26"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran := on(tt.ended).AddDate(0, 0, -1).Format(time.DateOnly)
			days := daysFrom("2026-08-10",
				day(ran, windowsDay{limits: []ledger.Limit{limit("7d", ran, "18:00")}}.of("work")),
				day(tt.ended, windowsDay{resets: []ledger.Reset{reset("7d", tt.ended, "10:00", 1)}}.of("work")))
			work := accountsPageOf(days, workAndSide).Rows[0]
			if got := shareRead(work.WeekLeft); got != tt.left || work.RanOut != tt.ranOut {
				t.Errorf("work, idle since its week ended on %s, left %s, ran out %q; want %s, %q", tt.ended, got, work.RanOut, tt.left, tt.ranOut)
			}
		})
	}
}

// The widest name a version is given is the version table's widest, or the
// id of a model the table prices and doesn't name, claude- taken off, as one
// the config prices.
func TestTheWidestVersionNameIsOfEveryModelTheTablePrices(t *testing.T) {
	pricing := func(ids ...string) ledger.Table {
		table := ledger.Pricing
		table.Models = append(slices.Clone(ledger.Pricing.Models), ledger.Model{IDs: ids})
		return table
	}
	tests := []struct {
		name  string
		table ledger.Table
		want  int
	}{
		{name: "an empty table, none", table: ledger.Table{}, want: 0},
		{name: "the version table's, Sonnet 5.5", table: ledger.Pricing, want: len("Sonnet 5.5")},
		{
			name: "a model priced and named by the version table, by its name, not its id",
			table: ledger.Table{
				Models:   []ledger.Model{{IDs: []string{"claude-a-long-dated-id-20990101"}}},
				Versions: []ledger.Version{{IDs: []string{"claude-a-long-dated-id-20990101"}, Name: "Short"}},
			},
			want: len("Short"),
		},
		{name: "a model the config prices, by its id", table: pricing("claude-sonnet-4-20250514"), want: len("sonnet-4-20250514")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.VersionNameCells(tt.table); got != tt.want {
				t.Errorf("the widest version name is %d cells, want %d", got, tt.want)
			}
		})
	}
}

// The page's limited windows are every window any row records a limit of,
// every account's among them, each once.
func TestThePagesLimitedWindowsAreEveryRowsLimits(t *testing.T) {
	page := views.AccountsPage{
		Rows: []views.AccountsRow{
			{Totals: views.AccountTotals{Limits: map[string]int{"5h": 2, "7d_oi": 1}}},
			{Totals: views.AccountTotals{Limits: map[string]int{"7d": 1}}},
		},
		All: views.AccountsRow{Totals: views.AccountTotals{Limits: map[string]int{"5h": 2, "7d": 1, "1d": 1}}},
	}
	if got, want := page.LimitedWindows(), []string{"1d", "5h", "7d", "7d_oi"}; !slices.Equal(got, want) {
		t.Errorf("the page's limited windows are %v, want %v", got, want)
	}
}

// Over the last 12 months, Accounts counts the days the Year counts as LAST
// 12 MONTHS, and prices them alike, 12 months of plans whole, or from the
// ledger's first day where it began part-way through them: every account's
// requests, worth and against its price are the Year's, the day before the
// 12 months left out of both. Work's Ultra and side's Pro cost $220 a month.
func TestAccountsOverTwelveMonthsAreTheYearsLastTwelveMonths(t *testing.T) {
	requests := []ledger.Summary{
		day("2025-10-07", account("work", sent("claude-opus-5-5", 5, inputUsage))),
		day("2025-10-08", account("work", sent("claude-opus-5-5", 2, inputUsage))),
		day("2026-03-16", account("side", sent("claude-opus-5-5", 3, inputUsage))),
		day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage))),
	}
	tests := []struct {
		name, began    string
		requests       int
		worth, cost    string
		worthOverCosts float64
	}{
		{name: "the ledger begun before them, whole", began: "2025-09-01", requests: 6, worth: "$12", cost: "$2,640", worthOverCosts: 12.0 / 2640},
		{name: "the ledger begun part-way through them, from its first day", began: "2026-03-16", requests: 4, worth: "$8",
			cost: "$1,490", worthOverCosts: 8 / (220 * 12 * 206.0 / 365)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysTo(tt.began, now, requests...)
			in := input(days)
			in.Began = on(tt.began)
			past := views.NewPast(in, days)
			all, year := past.Accounts(views.PeriodTwelveMonths).All, past.Year(views.YearAsk{}).Numbers
			if all.Totals.Requests != tt.requests || all.Totals.Amount.Dollars() != tt.worth || all.Plan.Cost.GroupedDollars() != tt.cost {
				t.Errorf("over 12 months, every account's %d requests are worth %s against %s, want %d worth %s against %s",
					all.Totals.Requests, all.Totals.Amount.Dollars(), all.Plan.Cost.GroupedDollars(), tt.requests, tt.worth, tt.cost)
			}
			if all.Plan.Ratio == nil || math.Abs(*all.Plan.Ratio-tt.worthOverCosts) > 1e-9 {
				t.Errorf("over 12 months, every account's worth is %v× its plans' cost, want %v×", all.Plan.Ratio, tt.worthOverCosts)
			}
			if got, want := jsonOf(t, all.Plan.Against), jsonOf(t, year.Against); got != want {
				t.Errorf("over 12 months, every account's against its price is\n%s\nthe Year's\n%s", got, want)
			}
			if all.Totals.Requests != year.Requests || jsonOf(t, all.Totals.Worth) != jsonOf(t, year.Worth) {
				t.Errorf("over 12 months, every account counts %d requests worth %s, the Year %d worth %s",
					all.Totals.Requests, jsonOf(t, all.Totals.Worth), year.Requests, jsonOf(t, year.Worth))
			}
		})
	}
}

// The days read for the last 12 months reach back the week before any
// period's first day, so Accounts' page over the last 12 months, built of
// them, counts an account idle since its week reset on their first day as
// history --since does, from the days before them too: the week it ran out
// on Friday 3 October 2025, and had none left as it reset.
func TestTwelveMonthsOfTheDaysReadTellARunBegunBeforeThem(t *testing.T) {
	days := daysFrom("2025-09-01",
		day("2025-10-03", windowsDay{limits: []ledger.Limit{limit("7d", "2025-10-03", "18:00")}}.of("work")),
		day("2025-10-08", windowsDay{resets: []ledger.Reset{reset("7d", "2025-10-08", "00:30", 1)}}.of("work")))
	read := views.DaysFrom(views.YearAsk{}, now, time.Monday).Format(time.DateOnly)
	held := slices.DeleteFunc(slices.Clone(days), func(s ledger.Summary) bool { return s.Day < read })
	in := input(days)
	work := views.NewPast(in, held).Accounts(views.PeriodTwelveMonths).Rows[0].Totals
	in.From = views.PeriodTwelveMonths.From(now, time.Monday)
	if got, want := jsonOf(t, work), jsonOf(t, views.NewHistory(in).Totals.Accounts[0]); got != want {
		t.Errorf("over 12 months of the days read from %s, work counts\n%s\nhistory --since\n%s", read, got, want)
	}
	if left, ok := work.LeftAtReset["7d"]; work.RanOut["7d"] != "2025-10-03" || !ok || left != 0 {
		t.Errorf("work's week ran out %q, %v left at its reset; want 3 Oct 2025, none", work.RanOut["7d"], work.LeftAtReset)
	}
}
