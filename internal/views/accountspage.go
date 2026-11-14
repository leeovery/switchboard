package views

import (
	"maps"
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
)

// AccountsPage is Accounts as the dashboard draws it, Compare and Detail
// alike: each configured account's row and every account's, their totals and
// plans over the period; their weeks of the last 12 months, a day at a time
// and a week at a time, with Weeks' facts on any number of the last of them,
// for Detail to show as many as fit; and, for Compare's WEEKS and WORTH,
// their weeks over the period's reach and their worth charted over the
// period. It's built of the days held once
// for each period, never a frame at a time, and stood at each look's moment
// as At stands it.
type AccountsPage struct {
	// Period is the period the rows' totals and plans are over, and Recorded
	// is set where its days record anything of any account: a request, a
	// limit, or a minute at a cap or a limit. A period with nothing recorded,
	// as on a fresh install, shows none of its figures.
	Period   Period
	Recorded bool
	// Weeks are the first days' dates of the weeks Detail may show, oldest
	// first, this week last: as many as Compare reaches over at 12 months,
	// whatever the period, as heldWeeks has them; ToCome how many of this
	// week's days are still to come; and Reaches how many of the last of
	// them Detail's weeks section reaches over at the fewest, as detailWeeks
	// counts them, those that don't fit counted out of view.
	Weeks   []string
	ToCome  int
	Reaches int
	// Reach are the columns Compare's WEEKS draws over the period, oldest
	// first, as reached has them: a week each, those that begin in the
	// period, this one last, as reachWeeks counts them; or of the last 12
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
	// its worth against it, as History counts and prices them: the plan
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
	// ended, "" where it didn't, whatever the period: nil, and "", where none
	// of its weeks ended among the last resetWeeks weeks' days and the week
	// before, and of every account's.
	WeekLeft *float64
	RanOut   string
	// Days are the days of the page's weeks, from the first, those to come
	// among them: each at the level, on its own scale over them all, of the
	// points of its week it used that day, marked where it hit a 5-hour
	// limit. Of every account's, each day is every account's points
	// together, marked where any hit one.
	Days []StripDay
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
	// bucket it's charted in, as worthCharts has them, today's by the day
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

// resetWeeks are the weeks an account's last reset is looked for in, as
// WeekLeft has it: this one and the five before it.
const resetWeeks = 6

// heldWeeks is how many weeks the page holds for Detail, today being the
// day given, as dayfile.Civil gives a day, weeks starting on the day given:
// as many as Compare reaches over at the last 12 months, the most any
// period reaches over.
func heldWeeks(today time.Time, starts time.Weekday) int {
	return PeriodTwelveMonths.reachWeeks(today, starts)
}

// Accounts is Accounts' page over the period given, as AccountsPage has it,
// of the days the Past holds at its Now.
func (p Past) Accounts(period Period) AccountsPage {
	in := p.in
	today := dayfile.CivilOf(in.Now)
	first := period.first(today, in.WeekStarts)
	before, days := splitAt(p.days, first.Format(time.DateOnly))
	_, before = splitAt(before, first.AddDate(0, 0, -lookBack).Format(time.DateOnly))
	totals := accountTotalsOf(before, days, in.Accounts, p.versions())
	known := plans{accounts: in.Accounts, table: in.Prices}
	firstDate := first.Format(time.DateOnly)
	costs := plansOf(days, known, pricedOver(firstDate, period.last(today, in.WeekStarts).Format(time.DateOnly), p.counted(firstDate), period.share()))
	_, weeks := splitAt(p.days, weeksTo(today, in.WeekStarts, heldWeeks(today, in.WeekStarts)).AddDate(0, 0, -lookBack).Format(time.DateOnly))
	page := AccountsPage{Period: period, Reaches: period.detailWeeks(today, in.WeekStarts), Rows: make([]AccountsRow, len(in.Accounts)), PricesAsOf: in.Prices.AsOf, in: in, days: weeks}
	worths := worthsByDay(days, in.Accounts)
	for x, a := range in.Accounts {
		plan, _ := known.of(a.ID)
		page.Rows[x] = AccountsRow{Label: a.Label, Totals: totals.Accounts[x], Plan: costs[x], PlanName: plan.Name,
			WorthBy: worthCharts(period, in.Now, in.WeekStarts, worths[x], nil)}
	}
	all := len(in.Accounts)
	page.All = AccountsRow{Totals: totals.Accounts[all], Plan: costs[all], WorthBy: worthCharts(period, in.Now, in.WeekStarts, worths[all], nil), all: true}
	page.Versions = p.versions().named(page.All.Totals.ByModel)
	page.Recorded = page.All.Totals.recorded()
	page.byDay()
	page.weeked(in, weeks)
	return page
}

// recorded reports whether the totals record anything: a request, a limit,
// or a minute at a cap or a limit.
func (t AccountTotals) recorded() bool {
	minutes := func(m *int) bool { return m != nil && *m > 0 }
	return t.Requests > 0 || len(t.Limits) > 0 || minutes(t.MinutesAtCap) || minutes(t.MinutesAtLimit)
}

// byDay sets BY DAY's days of each row, of the weeks the page holds to its
// Now, and how many of this week's are still to come.
func (page *AccountsPage) byDay() {
	in := page.in
	w := weekDaysOf(page.days, in.Now, in.WeekStarts, heldWeeks(dayfile.CivilOf(in.Now), in.WeekStarts))
	page.ToCome = len(w.days) - 1 - w.today
	every, anyLimits := make([]float64, len(w.days)), make([]int, len(w.days))
	for x, a := range in.Accounts {
		rises, limits := w.risesOf(a.ID, in.WeekWindow)
		page.Rows[x].Days = onItsOwnScale(rises, limits)
		for d := range rises {
			every[d] += rises[d]
			anyLimits[d] += limits[d]
		}
	}
	page.All.Days = onItsOwnScale(every, anyLimits)
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
// holds, and each account's left at its last reset, as lastEnded tells its
// week's of the last resetWeeks weeks' days and the week before them; and
// the page's reach and each row's peaks in it, as reached has them, of
// Weeks over the reach's weeks, those the page holds where they're as many.
func (page *AccountsPage) weeked(in HistoryInput, days []Day) {
	today := dayfile.CivilOf(in.Now)
	past := Past{in: in, days: days}
	held, reach := heldWeeks(today, in.WeekStarts), page.Period.reachWeeks(today, in.WeekStarts)
	weeks := past.weeksOver(held)
	reaching := weeks
	if reach != held {
		reaching = past.weeksOver(reach)
	}
	page.reached(in, reaching)
	page.Weeks = weeks.Weeks
	_, ended := splitAt(days, weeksTo(today, in.WeekStarts, resetWeeks).AddDate(0, 0, -lookBack).Format(time.DateOnly))
	for x, a := range in.Accounts {
		row := &page.Rows[x]
		row.Peaks, row.told = weeks.Accounts[x].Peaks, weeks.Accounts[x].told
		row.WeekLeft, row.RanOut = nil, ""
		if run, ok := lastEnded(accountDays(ended, a.ID), in.WeekWindow); ok {
			row.WeekLeft, row.RanOut = &run.left, run.ranOut
		}
	}
	page.All.Peaks, page.All.told = weeks.All.Peaks, weeks.All.told
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
