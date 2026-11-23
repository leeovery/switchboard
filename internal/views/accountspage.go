package views

import (
	"maps"
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
)

// AccountsPage is Accounts as the dashboard draws it, Compare and Detail
// alike: each configured account's row and every account's, their totals and
// plans over the period; their weeks of the 12 months to its last week, a
// day at a time and a week at a time, with Weeks' facts on any number of the
// last of them, for Detail to show as many as fit; and, for Compare's WEEKS
// and WORTH, their weeks over the period's reach and their worth charted
// over the period. A period gone counts its own days alone, and tells its
// weeks as the weeks to this one tell them, to its last. It's built of the
// days held once for each period, never a frame at a time, and stood at
// each look's moment as At stands it.
type AccountsPage struct {
	// Period is the period the rows' totals and plans are over, and Recorded
	// is set where its days record anything of any account: a request, a
	// limit, or a minute at a cap or a limit. A period with nothing recorded,
	// as on a fresh install, shows none of its figures.
	Period   Period
	Recorded bool
	// Weeks are the first days' dates of the weeks Detail may show, oldest
	// first, the period's last week last, this one at now: as many as
	// Compare reaches over at the 12 months to the period's last day, or to
	// today where that's earlier, whatever its length, as heldWeeks has
	// them; ToCome how many of this week's days are still to come, none where
	// the weeks end before it; ThisWeek set where they end at this week, as
	// at now and in a period gone whose last week holds today, the weeks'
	// sections drawing it as now's; and Reaches how many of the last of them
	// Detail's weeks section reaches over at the fewest, as detailWeeks
	// counts them, those that don't fit counted out of view. WholeTo is, of
	// a period gone, the last day of the last whole week they hold, as
	// dayfile.Civil gives a day, Detail's weeks section naming them by it,
	// as "the eight weeks to Sun 27 Sep", as wholeTo has it and WeeksShown
	// and LimitHitOver say it; zero at now.
	Weeks    []string
	ToCome   int
	ThisWeek bool
	Reaches  int
	WholeTo  time.Time
	// Reach are the columns Compare's WEEKS draws over the period, oldest
	// first, as reached has them: a week each, those that begin in the
	// period, its last week last, as reachWeeks counts them; or of 12
	// months, a month each.
	Reach []Bucket
	// Rows are each configured account's, in the config's order, and All
	// every account's.
	Rows []AccountsRow
	All  AccountsRow
	// Versions are the model versions any account used in the period, in
	// the version table's order, as every account's ByModel lists them, each
	// as named names it.
	Versions []ledger.Version
	// PricesAsOf is the date of the day the price table worth is priced by
	// was read, as help gives it.
	PricesAsOf string
	// NoLongerHere is how many accounts with no row of their own the
	// period's days name, whose work All's totals count: those removed, and
	// those moved to another pool, as noLongerHere counts them.
	NoLongerHere int

	// in and days are what the weeks are worked out from, for At to work
	// them out afresh: the input the page was built of, and the days from
	// lookBack before the first of the weeks Detail may show, which hold the
	// reach's.
	in   HistoryInput
	days []Day
}

// AccountsRow is an account's row of Accounts, or every account's.
type AccountsRow struct {
	// Label is its label, as the config gives it: "" of every account's.
	Label string
	// Totals are its totals over the period, and Plan its plan over it and
	// its worth against it, as history counts and prices them: the plan
	// priced whole for the period, whenever the account began recording, so
	// an account added part-way shows what was recorded of it against the
	// whole period's price; but from the ledger's first day where it began
	// part-way through the period, as there's no worth for the days before.
	Totals AccountTotals
	Plan   PlanCost
	// PlanName names its plan as the price table does, as Ultra: "" where
	// its plan isn't known, and of every account's.
	PlanName string
	// WeekLeft is the share its week had left as it last reset, the use read
	// just before that reset taken from the whole, and RanOut the date of the
	// local day it last reached its week's limit in the week that reset
	// ended, "" where it didn't, whatever the period's length; of a period
	// gone, as its last week reset, as leftAt has it, but where its last
	// week is this week, as at now: nil, and "", where none of its weeks
	// ended among the days of the resetWeeks weeks to the period's last and
	// the week before them, and of every account's. RanOut isn't for
	// display: RanOutOn names that day, as Accounts says it, by its weekday
	// at now, as "Fri"; in a period gone by its date, as "30 Sep", since an
	// own week placed under its last week can run out after its last day,
	// which a weekday would name as one in it.
	WeekLeft *float64
	RanOut   string
	RanOutOn string
	// Days are the days of the page's weeks, from the first, those to come
	// among them, each as DayUsed has it, for ShadedDays to shade those BY DAY
	// draws.
	Days []DayUsed
	// Peaks are its peaks in the page's weeks, as Weeks places its own weeks
	// under them, this week's so far, and of every account's, the mean of
	// theirs, whose facts Facts says.
	Peaks []WeekPeak
	// Reached are its peaks in each of the reach's columns: a week's, as
	// Peaks has them, or a month's, its fullest week's, as fullestIn has
	// it; and Reaching is Weeks' verdict on the whole weeks among them, whose
	// facts Compare's WEEKS says.
	Reached  []WeekPeak
	Reaching AccountVerdict
	// WorthBy is what its requests were worth over the period in each
	// bucket it's charted in, as worthCharts has them, a day's by the day
	// till Hourly charts it by the hour, for Compare's WORTH to take the
	// finest that fits.
	WorthBy WorthCharts

	// told is what each of the page's weeks tells of it, as Weeks' facts sum
	// them, and all is set of every account's.
	told []told
	all  bool
}

// Facts are Weeks' facts on the whole weeks among the last n of the row's,
// as Weeks' verdict counts them, its words unsaid: how many there are, how
// many it hit its limit in, or of every account's, how many some account
// was short in, and what they left at a reset, on average, where it's
// known. Every one of its weeks, where n is more.
func (r AccountsRow) Facts(n int) AccountVerdict {
	var t told
	for _, week := range r.told[max(len(r.told)-n, 0):] {
		t = t.plus(week)
	}
	v := t.facts()
	v.All = r.all
	return v
}

// DayUsed is a day of an account's: Used, the points of its week it used
// that day, and Limits, the 5-hour limits it hit that day; of every
// account's, theirs together.
type DayUsed struct {
	Used   float64
	Limits int
}

// ShadedDays are the days given as BY DAY shades them: each at the level of its
// use on those days' own scale, the quarters of their active days, marked
// where it hit a 5-hour limit, so the days BY DAY draws are shaded against
// each other alone, never against days out of view.
func ShadedDays(days []DayUsed) []StripDay {
	used, limits := make([]float64, len(days)), make([]int, len(days))
	for i, day := range days {
		used[i], limits[i] = day.Used, day.Limits
	}
	return onItsOwnScale(used, limits)
}

// resetWeeks are the weeks an account's last reset is looked for in, as
// WeekLeft has it: the period's last week, this one at now, and the five
// before it.
const resetWeeks = 6

// heldWeeks is how many weeks the page holds for Detail over the span, to
// its last week, weeks starting on the day given: as many as Compare
// reaches over at the 12 months to the last day of it that's come, as
// through has it, the most any period ending then reaches over.
func (s periodSpan) heldWeeks(starts time.Weekday) int {
	return LengthTwelveMonths.Now().reachWeeks(s.through(), starts)
}

// heldFrom is the first day of the first of the weeks the page holds for
// Detail over the span, as heldWeeks counts them to its last week, weeks
// starting on the day given, as dayfile.Civil gives a day.
func (s periodSpan) heldFrom(starts time.Weekday) time.Time {
	return weeksTo(s.lastWeek(starts), starts, s.heldWeeks(starts))
}

// AccountsFrom is when the ledger's days an Accounts page of the period
// reads at now begin, weeks starting on the day given, at the start of the
// first local day, as From has a day: lookBack days before the earlier of
// the period's first day, so its totals know the run that day began in, and
// the first of the weeks the page holds back from its last week, as
// heldFrom has it, so each is told by the reset that began it. The page
// reads every day from there to today, a period gone's too, as the resets
// after its last week file its weeks' peaks. A period gone reaches as far
// back from its last day as one at now does from today.
func (p Period) AccountsFrom(now time.Time, starts time.Weekday) time.Time {
	s := p.spanAt(dayfile.CivilOf(now), starts)
	first := score.Earlier(s.first, s.heldFrom(starts))
	start, _, _ := dayfile.Day(first.AddDate(0, 0, -lookBack).Format(time.DateOnly))
	return start
}

// Accounts is Accounts' page over the period given, as AccountsPage has it,
// of the days the Past holds at its Now: its totals, plans and worth of the
// period's days alone, from its first to its last.
func (p Past) Accounts(period Period) AccountsPage {
	in := p.in
	span := period.spanAt(dayfile.CivilOf(in.Now), in.WeekStarts)
	first := span.first
	before, days := splitAt(p.days, first.Format(time.DateOnly))
	_, before = splitAt(before, first.AddDate(0, 0, -lookBack).Format(time.DateOnly))
	days, _ = splitAt(days, span.last.AddDate(0, 0, 1).Format(time.DateOnly))
	totals := accountTotalsOf(before, days, in.Accounts, nil, p.versions())
	known := plans{accounts: in.Accounts, table: in.Prices}
	costs := plansOf(days, known, pricedOver(first.Format(time.DateOnly), span.last.Format(time.DateOnly), p.counted(first).Format(time.DateOnly), period.length.share()))
	_, weeks := splitAt(p.days, span.heldFrom(in.WeekStarts).AddDate(0, 0, -lookBack).Format(time.DateOnly))
	page := AccountsPage{Period: period, Rows: make([]AccountsRow, len(in.Accounts)), PricesAsOf: in.Prices.AsOf, NoLongerHere: in.noLongerHere(days), in: in, days: weeks}
	worths := worthsByDay(days, in.Accounts)
	for x, a := range in.Accounts {
		plan, _ := known.of(a.ID)
		page.Rows[x] = AccountsRow{Label: a.Label, Totals: totals.Accounts[x], Plan: costs[x], PlanName: plan.Name,
			WorthBy: worthCharts(period, in.Now, in.WeekStarts, in.Began, worths[x], nil)}
	}
	all := len(in.Accounts)
	page.All = AccountsRow{Totals: totals.Accounts[all], Plan: costs[all], WorthBy: worthCharts(period, in.Now, in.WeekStarts, in.Began, worths[all], nil), all: true}
	page.Versions = p.versions().named(page.All.Totals.ByModel)
	page.Recorded = page.All.Totals.recorded()
	page.stand(in, weeks)
	return page
}

// stand sets all the page tells of its weeks, of the input and days given,
// as they stand at in's Now, to the period's last week, as its span that
// day has it: how many Detail reaches over, as detailWeeks counts them;
// whether they end at this week, and the last day of their last whole week,
// as wholeTo has it; and over the weeks the page holds, as heldWeeks counts
// them, BY DAY's days, as byDay has them, and their peaks, facts and
// resets, as weeked has them. So a day that changes either count, or the
// weeks' end, changes all they tell alike.
func (page *AccountsPage) stand(in HistoryInput, days []Day) {
	s := page.Period.spanAt(dayfile.CivilOf(in.Now), in.WeekStarts)
	page.Reaches = page.Period.detailWeeks(s.today, in.WeekStarts)
	page.ThisWeek, page.WholeTo = s.weeksBack == 0, s.wholeTo(in.WeekStarts)
	page.byDay(in, days, s)
	page.weeked(in, days, s)
}

// recorded reports whether the totals record anything: a request, a limit,
// or a minute at a cap or a limit.
func (t AccountTotals) recorded() bool {
	minutes := func(m *int) bool { return m != nil && *m > 0 }
	return t.Requests > 0 || len(t.Limits) > 0 || minutes(t.MinutesAtCap) || minutes(t.MinutesAtLimit)
}

// byDay sets BY DAY's days of each row, of the days given over the weeks
// the page holds to the last week of the period's span s at in's Now, and
// how many of this week's are still to come, none where they end before it.
func (page *AccountsPage) byDay(in HistoryInput, days []Day, s periodSpan) {
	w := weekDaysOf(days, in.Now, in.WeekStarts, s.heldWeeks(in.WeekStarts), s.lastWeek(in.WeekStarts))
	page.ToCome = w.toCome()
	every := make([]DayUsed, len(w.days))
	for x, a := range in.Accounts {
		rises, limits := w.risesOf(a.ID, in.WeekWindow)
		days := make([]DayUsed, len(rises))
		for d := range rises {
			days[d] = DayUsed{Used: rises[d], Limits: limits[d]}
			every[d].Used += rises[d]
			every[d].Limits += limits[d]
		}
		page.Rows[x].Days = days
	}
	page.All.Days = every
}

// onItsOwnScale are a strip's days, each at the level its value is shaded at
// on the strip's own scale, the quarters of its own active days, marked
// where its day's limits are any.
func onItsOwnScale(values []float64, limits []int) []StripDay {
	days := levelled(values, quarterCuts(appendActive(nil, values)))
	for d, n := range limits {
		days[d].Limit = n > 0
	}
	return days
}

// weeked sets the page's weeks and each row's peaks in them and what each
// tells, as Weeks gives them of days at in's Now, over the weeks the page
// holds to the last week of the period's span s, and each account's left
// at its last reset, as leftAt tells its week's of the resetWeeks weeks'
// days to that one and the week before them; and the page's reach and each
// row's peaks in it, as reached has them, of Weeks over the reach's weeks,
// those the page holds where they're as many.
func (page *AccountsPage) weeked(in HistoryInput, days []Day, s periodSpan) {
	past := Past{in: in, days: days}
	held, reach, last := s.heldWeeks(in.WeekStarts), page.Period.reachWeeks(s.today, in.WeekStarts), s.lastWeek(in.WeekStarts)
	weeks := past.weeksOver(held, last)
	reaching := weeks
	if reach != held {
		reaching = past.weeksOver(reach, last)
	}
	page.reached(in, reaching, s)
	page.Weeks = weeks.Weeks
	_, ended := splitAt(days, weeksTo(last, in.WeekStarts, resetWeeks).AddDate(0, 0, -lookBack).Format(time.DateOnly))
	own := weeklyOf(in.WeekWindow, in.WeekStarts, in.Now)
	for x, a := range in.Accounts {
		row := &page.Rows[x]
		row.Peaks, row.told = weeks.Accounts[x].Peaks, weeks.Accounts[x].told
		row.WeekLeft, row.RanOut, row.RanOutOn = nil, "", ""
		if run, ok := s.leftAt(own, accountDays(ended, a.ID)); ok {
			row.WeekLeft, row.RanOut, row.RanOutOn = &run.left, run.ranOut, s.dayNamed(run.ranOut)
		}
	}
	page.All.Peaks, page.All.told = weeks.All.Peaks, weeks.All.told
}

// leftAt is the run of an account's week, as w tells its own weeks of its
// days, oldest first, that ended last as of the span's last week: where
// that's this week, the last that ended among them, as lastEnded tells it;
// else the one that ended that week, the last of them placed under it or
// before, as Weeks places them, so a period gone tells what the week had
// left as it reset at its end, though that reset came after its last day.
// It reports false where none ended.
func (s periodSpan) leftAt(w weekly, days []ledger.AccountDay) (endedRun, bool) {
	if s.weeksBack == 0 {
		return lastEnded(days, w.key)
	}
	last := s.lastWeek(w.starts).Format(time.DateOnly)
	var ended ownWeek
	found := false
	for _, o := range w.ownWeeksOf(days)[w.key] {
		if w.placedUnder(o) <= last {
			ended, found = o, true
		}
	}
	if !found {
		return endedRun{}, false
	}
	return ranOver(days, w.key, ended.start, ended.end, ended.peak), true
}

// dayNamed names the day given, as dayfile.Civil reads a date, as RanOutOn
// does in the span: by its weekday, as "Fri"; of a span gone, by its date,
// as "30 Sep", as prose.DayMonthAt names it. "" where it isn't a date.
func (s periodSpan) dayNamed(date string) string {
	day, ok := dayfile.Civil(date)
	switch {
	case !ok:
		return ""
	case s.gone:
		return prose.DayMonthAt(s.today, day)
	}
	return prose.Weekday(day.Weekday())
}

// LimitedWindows are the windows any row of the page records a limit of,
// every account's among them, by their keys, each once, sorted.
func (p AccountsPage) LimitedWindows() []string {
	var keys []string
	for _, row := range append(slices.Clone(p.Rows), p.All) {
		keys = append(keys, slices.Collect(maps.Keys(row.Totals.Limits))...)
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// named are the versions of the shares given, in their order, each as the
// version table names and shades it; or, one it doesn't name, by the id its
// share gives it, claude- taken off, and its family, as familyOf reads it.
func (v versions) named(shares []ModelShare) []ledger.Version {
	named := make([]ledger.Version, len(shares))
	for i, s := range shares {
		version, ok := v.table.Version(s.Model)
		if !ok {
			version = ledger.Version{IDs: []string{s.Model}, Name: ledger.ShortModelID(s.Model), Family: v.familyOf(s.Model)}
		}
		named[i] = version
	}
	return named
}

// VersionNameCells are the cells of the widest name the pages built with the
// table give a version of a model it prices, as named names them: a row of
// its version table, or a model it prices and doesn't name, as one the
// config prices, by its id, claude- taken off. A model priced nowhere is
// named by its id too, and may be wider still.
func VersionNameCells(table ledger.Table) int {
	widest := 0
	for _, v := range table.Versions {
		widest = max(widest, ansi.StringWidth(prose.Clean(v.Name)))
	}
	for _, m := range table.Models {
		for _, id := range m.IDs {
			widest = max(widest, ansi.StringWidth(table.VersionName(id)))
		}
	}
	return widest
}
