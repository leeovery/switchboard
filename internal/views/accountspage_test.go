package views_test

import (
	"fmt"
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
var threeAccounts = config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "personal", Primary: true, Plan: "max20x"}, {ID: "side", Plan: "max20x"}}

// accountsPageOf is Accounts' page over this week of the days given, the
// accounts configured as given, now Wednesday 7 October.
func accountsPageOf(days []ledger.Summary, configured config.Accounts) views.AccountsPage {
	in := input(days)
	in.Accounts = configured
	return views.NewPast(in, days).Accounts(views.AccountsThisWeek)
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
	return fmt.Sprintf("%s %d/%d %s", v.Verdict, v.Limits, v.Weeks, shareRead(v.LeftAtReset))
}

// Detail's WEEKS and verdicts are the frames': each account's last six
// weeks' peaks, this week's so far, and its verdict over the last 7 whole
// weeks, its limits and what it left at a reset, on average; every
// account's, the mean of their peaks, and room to spare together, naming
// side as the one ending weeks with the most left and work the least.
func TestDetailsWeeksAndVerdictsAreTheFrames(t *testing.T) {
	days := peaking(map[string][]float64{
		"work":     {0.72, 0.88, 1, 0.94, 1, 0.81, 1},
		"personal": {0.64, 0.70, 0.58, 0.77, 0.69, 0.90, 0.85},
		"side":     {0.22, 0.35, 0.41, 0.30, 0.52, 0.47, 0.60},
	})
	days[len(days)-1] = day("2026-10-07",
		windowsDay{highest: map[string]float64{"7d": 0.66}}.of("personal"),
		windowsDay{highest: map[string]float64{"7d": 0.78}}.of("side"),
		windowsDay{highest: map[string]float64{"7d": 0.34}}.of("work"))
	page := accountsPageOf(days, threeAccounts)

	tests := []struct {
		name           string
		row            views.AccountsRow
		peaks, verdict string
	}{
		{name: "work", row: page.Rows[0], peaks: "100 94 100 81 100 34…", verdict: views.HitsItsLimit + " 3/7 9%"},
		{name: "personal", row: page.Rows[1], peaks: "58 77 69 90 85 66…", verdict: views.EndsWithRoom + " 0/7 27%"},
		{name: "side", row: page.Rows[2], peaks: "41 30 52 47 60 78…", verdict: views.PlentyToSpare + " 0/7 59%"},
		{name: "all accounts", row: page.All, peaks: "66 67 74 73 82 59…", verdict: views.RoomToSpare + " 3/7 32%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := peaksRead(tt.row.Peaks); got != tt.peaks {
				t.Errorf("%s's weeks are %s, want %s", tt.name, got, tt.peaks)
			}
			if got := verdictRead(tt.row.Verdict); got != tt.verdict {
				t.Errorf("%s's verdict is %s, want %s", tt.name, got, tt.verdict)
			}
		})
	}
	if want := []string{"2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28", "2026-10-05"}; !slices.Equal(page.Weeks, want) {
		t.Errorf("the six weeks are %v, want %v", page.Weeks, want)
	}
	if most, least, ok := page.Ends(); !ok || most.Totals.Account != "side" || least.Totals.Account != "work" {
		t.Errorf("weeks end with the most left on %q and the least on %q (%v), want side and work", most.Totals.Account, least.Totals.Account, ok)
	}
}

// Where one account's weeks are known, it ends weeks with both the most and
// the least left; where none's are, none does.
func TestTheEndsOfOneAccountsWeeksAreItsOwn(t *testing.T) {
	days := peaking(map[string][]float64{"work": {0.5, 0.6, 0.7}})
	page := accountsPageOf(days, workAndSide)
	if most, least, ok := page.Ends(); !ok || most.Totals.Account != "work" || least.Totals.Account != "work" {
		t.Errorf("the ends are %q and %q (%v), want work both", most.Totals.Account, least.Totals.Account, ok)
	}
	if _, _, ok := accountsPageOf(daysFrom("2026-10-07"), workAndSide).Ends(); ok {
		t.Error("with no week known, the ends are known")
	}
}

// Of the accounts that tie, the first configured ends weeks with the most
// left, and with the least.
func TestTheFirstConfiguredEndsWeeksOfThoseThatTie(t *testing.T) {
	days := peaking(map[string][]float64{"work": {0.5, 0.5}, "side": {0.5, 0.5}, "spare": {0.5, 0.5}})
	most, least, _ := accountsPageOf(days, accounts).Ends()
	if most.Totals.Account != "work" || least.Totals.Account != "work" {
		t.Errorf("of three that tie, the ends are %q and %q, want work, the first configured, both", most.Totals.Account, least.Totals.Account)
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
		for _, d := range days[:4] {
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
		for _, d := range days[:4] {
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

// BY DAY is the six weeks to this one, a day at a time, its days to come
// counted, as many as are left of this week on each of its days; a week of
// no use is its days at level 0.
func TestByDayIsTheSixWeeksToThisOneItsDaysToComeCounted(t *testing.T) {
	for i := range 7 {
		now := on("2026-10-05").AddDate(0, 0, i).Add(13 * time.Hour)
		days := daysTo("2026-10-01", now, day("2026-10-01", rose("work", map[string]float64{"7d": 0.1})))
		in := input(days)
		in.Now = now
		page := views.NewPast(in, days).Accounts(views.AccountsThisWeek)
		t.Run(now.Weekday().String(), func(t *testing.T) {
			if page.ToCome != 6-i || len(page.Rows[0].Days) != 42 || page.Weeks[0] != "2026-08-31" {
				t.Errorf("on %v, %d of %d days are to come, from %s, want %d of 42, from 31 Aug", now.Weekday(), page.ToCome, len(page.Rows[0].Days), page.Weeks[0], 6-i)
			}
			if slices.ContainsFunc(page.Rows[0].Days[:7*4], func(d views.StripDay) bool { return d.Level > 0 }) {
				t.Errorf("the weeks of no use are at levels %v, want 0", page.Rows[0].Days[:7*4])
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
	for _, period := range []views.AccountsPeriod{views.AccountsToday, views.AccountsThisWeek, views.AccountsTwelveWeeks} {
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
