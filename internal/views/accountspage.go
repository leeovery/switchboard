package views

import (
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
)

// AccountsPage is Accounts as the dashboard draws it, Compare and Detail
// alike: each configured account's row and every account's, their totals and
// plans over the period, and their last six weeks, a day at a time and a week
// at a time, with Weeks' verdict on them. It's built of the days held once
// for each period, never a frame at a time, and stood at each look's moment
// as At stands it.
type AccountsPage struct {
	// Period is the period the rows' totals and plans are over, and Recorded
	// is set where its days record anything of any account: a request, a
	// limit, or a minute at a cap or a limit. A period with nothing recorded,
	// as on a fresh install, shows none of its figures.
	Period   AccountsPeriod
	Recorded bool
	// Weeks are the six weeks' first days' dates, oldest first, this week
	// last, and ToCome how many of this week's days are still to come.
	Weeks  []string
	ToCome int
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

	// in and days are what the six weeks are worked out from, for At to
	// work them out afresh: the input the page was built of, and the days
	// from lookBack before the first of the eight weeks Weeks judges.
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
	// PlanName names its plan as the price table does, as Max 20x: "" where
	// its plan isn't known, and of every account's.
	PlanName string
	// WeekLeft is the share its week had left as it last reset, the use read
	// just before that reset taken from the whole, and RanOut the date of the
	// local day it last reached its week's limit in the week that reset
	// ended, "" where it didn't, whatever the period: nil, and "", where none
	// of its weeks ended among the six weeks' days and the week before, and
	// of every account's.
	WeekLeft *float64
	RanOut   string
	// Days are the six weeks' days, from the first, those to come among
	// them: each at the level, on its own scale, of the points of its week
	// it used that day, marked where it hit a 5-hour limit. Of every
	// account's, each day is every account's points together, marked where
	// any hit one.
	Days []StripDay
	// Peaks are its six weeks' peaks, as Weeks places its own weeks under
	// them, this week's so far, and of every account's, the mean of theirs;
	// and Verdict is Weeks' verdict on it, over the last 7 whole weeks, or of
	// every account's, on them together.
	Peaks   []WeekPeak
	Verdict AccountVerdict
}

// accountsWeeks are the weeks Accounts' BY DAY and WEEKS show: this one, so
// far, and the five before it.
const accountsWeeks = 6

// Accounts is Accounts' page over the period given, as AccountsPage has it,
// of the days the Past holds at its Now.
func (p Past) Accounts(period AccountsPeriod) AccountsPage {
	in := p.in
	today := dayfile.CivilOf(in.Now)
	first := period.first(today, in.WeekStarts)
	before, days := splitAt(p.days, first.Format(time.DateOnly))
	_, before = splitAt(before, first.AddDate(0, 0, -lookBack).Format(time.DateOnly))
	totals := accountTotalsOf(before, days, in.Accounts, p.versions())
	known := plans{accounts: in.Accounts, table: in.Prices}
	firstDate := first.Format(time.DateOnly)
	costs := plansOf(days, known, pricedOver(firstDate, period.last(today, in.WeekStarts).Format(time.DateOnly), p.counted(firstDate), period.share()))
	_, weeks := splitAt(p.days, weeksTo(today, in.WeekStarts, weeksShown).AddDate(0, 0, -lookBack).Format(time.DateOnly))
	page := AccountsPage{Period: period, Rows: make([]AccountsRow, len(in.Accounts)), PricesAsOf: in.Prices.AsOf, in: in, days: weeks}
	for x, a := range in.Accounts {
		plan, _ := known.of(a.ID)
		page.Rows[x] = AccountsRow{Label: a.Label, Totals: totals.Accounts[x], Plan: costs[x], PlanName: plan.Name}
	}
	all := len(in.Accounts)
	page.All = AccountsRow{Totals: totals.Accounts[all], Plan: costs[all]}
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

// byDay sets BY DAY's days of each row, of the six weeks to the page's Now,
// and how many of this week's are still to come.
func (page *AccountsPage) byDay() {
	in := page.in
	w := weekDaysOf(page.days, in.Now, in.WeekStarts, accountsWeeks)
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

// weeked sets the page's six weeks and each row's peaks and verdict, as
// Weeks gives them of days at in's Now, and each account's left at its last
// reset, as lastEnded tells its week's of the six weeks' days and the week
// before them.
func (page *AccountsPage) weeked(in HistoryInput, days []Day) {
	weeks := Past{in: in, days: days}.weeksPage()
	shown := len(weeks.Weeks) - accountsWeeks
	page.Weeks = weeks.Weeks[shown:]
	_, ended := splitAt(days, weeksTo(dayfile.CivilOf(in.Now), in.WeekStarts, accountsWeeks).AddDate(0, 0, -lookBack).Format(time.DateOnly))
	for x, a := range in.Accounts {
		row := &page.Rows[x]
		row.Peaks, row.Verdict = weeks.Accounts[x].Peaks[shown:], weeks.Accounts[x].AccountVerdict
		row.WeekLeft, row.RanOut = nil, ""
		if run, ok := lastEnded(accountDays(ended, a.ID), in.WeekWindow); ok {
			row.WeekLeft, row.RanOut = &run.left, run.ranOut
		}
	}
	page.All.Peaks, page.All.Verdict = weeks.All.Peaks[shown:], weeks.All.AccountVerdict
}

// Ends are the accounts whose weeks end with the most left at a reset, on
// average, and with the least, as Detail's verdict on every account names
// them: of those whose verdict knows it, the first configured of any that
// tie, the one both where only one knows it. It reports false where none
// does.
func (p AccountsPage) Ends() (most, least AccountsRow, ok bool) {
	for _, row := range p.Rows {
		left := row.Verdict.LeftAtReset
		switch {
		case left == nil:
		case !ok:
			most, least, ok = row, row, true
		case *left > *most.Verdict.LeftAtReset:
			most = row
		case *left < *least.Verdict.LeftAtReset:
			least = row
		}
	}
	return most, least, ok
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
