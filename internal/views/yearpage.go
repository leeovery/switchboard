package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// YearAsk is what the Year is asked to show: the last 12 months, or a
// calendar year; and all use, or a version's alone, or an account's.
type YearAsk struct {
	// Year is the calendar year shown, as 2025: 0 for the last 12 months.
	Year int
	// Version is the version shown alone, by its name as the version table
	// gives it, as Opus 5.5, and Account the account, by its id: neither for
	// all use.
	Version, Account string
}

// YearPage is the Year as the dashboard draws it: its grid of days, a column
// a week, and the cuts between their shades; the months' names over it;
// LAST 12 MONTHS' numbers, or a calendar year's; BY MONTH's rows; and what v
// cycles through.
type YearPage struct {
	// Weeks are the grid's columns, oldest first, each a week from its first
	// day, as week_starts sets it.
	Weeks []GridWeek
	// Cuts are the cuts between the levels, as quarterCuts gives them, in
	// requests a day; KeyCuts are them as the key gives them, rounded to 50.
	Cuts, KeyCuts []int
	// Months are the months' names over the grid, oldest first.
	Months []MonthLabel
	// Numbers are the period's numbers.
	Numbers YearNumbers
	// ByMonth are BY MONTH's rows, oldest first; and Before, filtered to a
	// version, the row for the months before it was first used: nil where
	// it's none.
	ByMonth []MonthRow
	Before  *MonthsBefore
	// Families are the families BY MONTH's bars are split by, those whose
	// requests were worth anything in its months, in the version table's
	// order: none filtered to a version, whose bars are its own.
	Families []string
	// Version is the version shown alone, as the version table names it,
	// with its family and its shade, or by its name and family where the
	// table doesn't: the zero Version for all use, or an account's.
	Version ledger.Version
	// Versions are the versions v cycles through, by their names, in the
	// version table's order, and Accounts the accounts, by their ids, in the
	// config's order.
	Versions, Accounts []string
}

// GridWeek is a column of the Year's grid: a week, by its first day's date,
// and its seven days, from its first.
type GridWeek struct {
	First string
	Days  []GridDay
}

// GridDay is a day of the Year's grid: its requests and its level, and
// whether it's left out, as a day still to come is, or one outside the
// calendar year shown.
type GridDay struct {
	YearDay
	LeftOut bool
}

// MonthLabel is a month's name over the Year's grid: the month, as Month
// names it, 2026-10; the column of the week it's over, by its index among
// the grid's weeks; and whether it's the current month.
type MonthLabel struct {
	Month   string
	Week    int
	Current bool
}

// YearNumbers are the numbers beside the Year's grid: its requests, its
// sessions, each counted once, the limits hit, by window, what it was worth,
// what the plans cost and the worth against it, its busiest day and its
// longest run of days with any request. Filtered to a version, Share is its
// share of all use's worth and FirstUsed the day it was first used, by its
// date.
type YearNumbers struct {
	Requests int
	Sessions int
	Limits   map[string]int
	Worth
	Against
	BusiestDay string
	LongestRun int
	Share      *float64
	FirstUsed  string
}

// MonthRow is a row of BY MONTH: the month, as Month counts it; filtered to a
// version, Share, the version's share of the month's worth; From, the day the
// ledger began recording, or the version was first used, where that was
// part-way through the month, by its date; and SoFar, set for the month so
// far.
type MonthRow struct {
	Month
	Share *float64
	From  string
	SoFar bool
}

// MonthsBefore is the row, filtered to a version, for the months before it
// was first used: the first and last of them, as Month names them, and the
// version of its family used then, by its name: "" for none.
type MonthsBefore struct {
	From, To string
	Then     string
}

// The Year's grid and its key.
const (
	// yearWeeks are the weeks the grid shows of the last 12 months: this
	// week and the 52 before it.
	yearWeeks = 53
	// labelDays are how many of a month's days a week holds for the month's
	// name to sit over it, the first such week; and labelWeeks the fewest
	// weeks apart two months' names sit, as a name runs past the week after
	// its own.
	labelDays  = 4
	labelWeeks = 2
	// keyRounding is what the key rounds the cuts to, in requests a day.
	keyRounding = 50
)

// YearFrom is the first day the Year asked for at now reads, weeks starting
// on the day given, at its local midnight: the earlier of its grid's first
// day and its period's, from which the ledger's days are read for it.
func YearFrom(ask YearAsk, now time.Time, weekStarts time.Weekday) time.Time {
	first, _ := dayfile.Civil(yearSpanOf(ask.Year, dayfile.CivilOf(now), weekStarts).earliest())
	return time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.Local)
}

// year is the Year's page asked for, of the days held.
func (p Past) year(ask YearAsk) YearPage {
	v := p.versions()
	span := yearSpanOf(ask.Year, dayfile.CivilOf(p.in.Now), p.in.WeekStarts)
	filter := yearFilter{YearAsk: ask, v: v}
	drawn := yearDays(filter.days(between(p.days, span.earliest(), span.gridLast())))
	y := yearView{
		ask: ask, span: span, v: v, plans: shownPlans(ask.Account, p.in.Accounts, p.in.Prices),
		all: between(p.days, span.first, span.last), shown: between(drawn, span.first, span.last), began: p.began(),
	}
	cuts := quarterCuts(y.shown.active())
	page := YearPage{
		Weeks: span.weeksOf(drawn.requests(), cuts), Cuts: cuts, KeyCuts: keyCuts(cuts), Months: span.labels(),
		Versions: y.all.versions(v), Accounts: cycledAccounts(p.in.Accounts),
	}
	if ask.Version == "" {
		page.Numbers, page.ByMonth = y.numbers(), y.months()
		page.Families = y.families(page.ByMonth)
		return page
	}
	first := y.shown.firstUsed()
	page.Version = y.version()
	page.Numbers, page.ByMonth, page.Before = y.versionNumbers(first), y.versionMonths(first), y.before(first, page.Version.Family)
	return page
}

// yearSpan is what of the days the Year shows, by their dates: its period,
// the days its numbers and BY MONTH count, from first to last, today or a
// calendar year's 31 December; end, the day the period's plans are priced
// to, a period still running being priced whole; its grid, weeks weeks from
// the one beginning on grid, of which it draws the days from from to to, but
// those after today; and today.
type yearSpan struct {
	first, last, end string
	grid             time.Time
	weeks            int
	from, to         string
	today            string
}

// yearSpanOf is the span of the last 12 months, to the day with the date
// today, of 0, or else of the calendar year given, weeks starting on the day
// given: the last 12 months' from the day after today's date a year back,
// their grid this week and the 52 before it, every day of it drawn, as the
// first week's days before the 12 months are; a calendar year's from 1
// January to 31 December, their grid from the week of the first to the week
// of the last, the days outside the year left out.
func yearSpanOf(year int, today time.Time, weekStarts time.Weekday) yearSpan {
	s := yearSpan{today: today.Format(time.DateOnly)}
	if year == 0 {
		s.first, s.last, s.end = yearAfter(today).Format(time.DateOnly), s.today, s.today
		s.grid, s.weeks = weeksTo(today, weekStarts, yearWeeks), yearWeeks
		s.from, s.to = s.gridFirst(), s.gridLast()
		return s
	}
	jan1, dec31 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	s.first, s.end = jan1.Format(time.DateOnly), dec31.Format(time.DateOnly)
	s.last = min(s.end, s.today)
	s.grid = weekOf(jan1, weekStarts)
	s.weeks = s.weekHolding(dec31) + 1
	s.from, s.to = s.first, s.end
	return s
}

// yearAfter is the first of the 12 months to today: the day after its date a
// year back, 1 March where today is 29 February.
func yearAfter(today time.Time) time.Time {
	back := today.AddDate(-1, 0, 0)
	if back.Day() != today.Day() {
		// A 29 February a year back, in a year without one, is taken to 1
		// March: already the day after the 28th.
		return back
	}
	return back.AddDate(0, 0, 1)
}

// day is the date of day i of the grid's week w.
func (s yearSpan) day(w, i int) string {
	return s.grid.AddDate(0, 0, 7*w+i).Format(time.DateOnly)
}

// weekHolding is the index among the grid's weeks of the one holding the day
// given.
func (s yearSpan) weekHolding(day time.Time) int {
	return int(day.Sub(s.grid) / (7 * 24 * time.Hour))
}

// gridFirst and gridLast are the dates of the grid's first day and its last.
func (s yearSpan) gridFirst() string { return s.day(0, 0) }
func (s yearSpan) gridLast() string  { return s.day(s.weeks-1, 6) }

// earliest is the date of the first day the Year reads: the earlier of its
// grid's first and its period's, as the 12 months begin before the grid on a
// week's first day where they hold a 29 February.
func (s yearSpan) earliest() string {
	return min(s.first, s.gridFirst())
}

// drawn reports whether the grid draws the day with the given date, but for
// its being still to come.
func (s yearSpan) drawn(day string) bool {
	return day >= s.from && day <= s.to
}

// leftOut reports whether the grid leaves the day with the given date out:
// one it doesn't draw, or one still to come.
func (s yearSpan) leftOut(day string) bool {
	return !s.drawn(day) || day > s.today
}

// weeksOf are the grid's weeks of the days' requests, by their dates, each
// day at its level, as shade gives it of cuts: a day it holds none of at
// level 0, and one it leaves out with no requests or level.
func (s yearSpan) weeksOf(requests map[string]int, cuts []int) []GridWeek {
	weeks := make([]GridWeek, s.weeks)
	for w := range weeks {
		days := make([]GridDay, 7)
		for i := range days {
			day := YearDay{Day: s.day(w, i)}
			if s.leftOut(day.Day) {
				days[i] = GridDay{YearDay: day, LeftOut: true}
				continue
			}
			day.Requests = requests[day.Day]
			day.Level = shade(day.Requests, cuts)
			days[i] = GridDay{YearDay: day}
		}
		weeks[w] = GridWeek{First: days[0].Day, Days: days}
	}
	return weeks
}

// labels are the months' names over the grid, oldest first: each over the
// first week that holds labelDays of its days the grid draws, days still to
// come among them, today's month the current one; and today's month, where
// no week holds as many of its days yet, over the week holding its 1st, the
// grid's last, 3 weeks or more after the month before's name. A name a week
// before the next would run into it, and is left out: only the month the
// grid begins part-way through, its last days in the first week, can be.
func (s yearSpan) labels() []MonthLabel {
	var labels []MonthLabel
	labelled := make(map[string]bool)
	for w := range s.weeks {
		held := make(map[string]int)
		for i := range 7 {
			day := s.day(w, i)
			month, _ := monthOf(day)
			if !s.drawn(day) || labelled[month] {
				continue
			}
			if held[month]++; held[month] == labelDays {
				labelled[month] = true
				labels = append(labels, MonthLabel{Month: month, Week: w, Current: s.running(month)})
			}
		}
	}
	if current, ok := s.currentLabel(); ok && !labelled[current.Month] {
		labels = append(labels, current)
	}
	if len(labels) > 1 && labels[1].Week-labels[0].Week < labelWeeks {
		return labels[1:]
	}
	return labels
}

// currentLabel is today's month's name over the week holding its 1st,
// reporting false where the grid doesn't draw that day.
func (s yearSpan) currentLabel() (MonthLabel, bool) {
	month, _ := monthOf(s.today)
	first, _ := dayfile.MonthStart(month)
	if !s.drawn(first.Format(time.DateOnly)) {
		return MonthLabel{}, false
	}
	return MonthLabel{Month: month, Week: s.weekHolding(first), Current: true}, true
}

// running reports whether the month, as Month names it, is today's.
func (s yearSpan) running(month string) bool {
	return strings.HasPrefix(s.today, month+"-")
}

// partWay is the day with the given date where the period's days held, or a
// version's use, began on it: after the period's first day, in the month
// given, as Month names it, and part-way through it. It's "" otherwise.
func (s yearSpan) partWay(month, began string) string {
	if began <= s.first || !strings.HasPrefix(began, month+"-") || strings.HasSuffix(began, "-01") {
		return ""
	}
	return began
}

// keyCuts are the cuts as the key gives them, each rounded to the nearest
// keyRounding, but one under it, kept as it is, as a key's bound of none
// would say nothing.
func keyCuts(cuts []int) []int {
	rounded := make([]int, len(cuts))
	for i, cut := range cuts {
		rounded[i] = cut
		if cut >= keyRounding {
			rounded[i] = (cut + keyRounding/2) / keyRounding * keyRounding
		}
	}
	return rounded
}

// yearFilter is what of a day's use the Year shows, as asked: every
// account's; an account's alone; or a version's alone, every account's, by
// its name as the version table gives it.
type yearFilter struct {
	YearAsk
	v versions
}

// days are the days, each narrowed to the use shown.
func (f yearFilter) days(days []Day) []Day {
	if f.Account == "" && f.Version == "" {
		return days
	}
	narrowed := make([]Day, len(days))
	for i, d := range days {
		priced := ledger.Priced{Summary: d.Summary}
		for _, a := range d.Accounts {
			if kept, ok := f.account(a); ok {
				priced.Accounts = append(priced.Accounts, kept)
			}
		}
		narrowed[i] = f.v.day(priced)
	}
	return narrowed
}

// account is what's shown of an account's day, reporting false where it's
// none of it: the whole of the account shown; or of a version shown, the
// account's requests of it alone, with no limits, which are the account's,
// and the sessions they were of: those named, by their ids, each once, and
// besides them, the count of each model's day that names none, as one of a
// summary from before version 4, which can't be counted distinct.
func (f yearFilter) account(a ledger.PricedAccount) (ledger.PricedAccount, bool) {
	if f.Version == "" {
		return a, a.Account == f.Account
	}
	kept := ledger.PricedAccount{Account: a.Account}
	for _, m := range a.Models {
		if m.Model == "" || f.v.table.VersionName(m.Model) != f.Version {
			continue
		}
		kept.Models = append(kept.Models, m)
		if m.SessionIDs == nil {
			kept.Sessions += m.Sessions
			continue
		}
		kept.SessionIDs = append(kept.SessionIDs, m.SessionIDs...)
	}
	if kept.SessionIDs != nil {
		slices.Sort(kept.SessionIDs)
		kept.SessionIDs = slices.Compact(kept.SessionIDs)
	}
	kept.Sessions += len(kept.SessionIDs)
	return kept, len(kept.Models) > 0
}

// shownPlans are the plans of the accounts the Year shows: every configured
// account's, or the one with the given id's alone, none where it's none of
// them.
func shownPlans(account string, accounts config.Accounts, table ledger.Table) plans {
	if account == "" {
		return plans{accounts: accounts, table: table}
	}
	i := slices.IndexFunc(accounts, func(a config.Account) bool { return a.ID == account })
	if i < 0 {
		return plans{table: table}
	}
	return plans{accounts: accounts[i : i+1], table: table}
}

// cycledAccounts are the accounts v cycles through, by their ids, in the
// config's order: none of one alone, which would only repeat all use.
func cycledAccounts(accounts config.Accounts) []string {
	if len(accounts) < 2 {
		return nil
	}
	return accounts.IDs()
}

// yearDays are days of the Year, in order.
type yearDays []Day

// between are those of days, in order, from the one with the date first to
// the one with the date last.
func between(days []Day, first, last string) yearDays {
	var within yearDays
	for _, d := range days {
		if d.Day >= first && d.Day <= last {
			within = append(within, d)
		}
	}
	return within
}

// models are the days' models' days, every account's, in order.
func (days yearDays) models() []ledger.PricedModel {
	var models []ledger.PricedModel
	for _, d := range days {
		for _, a := range d.Accounts {
			models = append(models, a.Models...)
		}
	}
	return models
}

// requests are each day's requests, by its date.
func (days yearDays) requests() map[string]int {
	requests := make(map[string]int, len(days))
	for _, d := range days {
		requests[d.Day] = d.requests()
	}
	return requests
}

// active are the requests of each day that had any, in order.
func (days yearDays) active() []int {
	var active []int
	for _, d := range days {
		if n := d.requests(); n > 0 {
			active = append(active, n)
		}
	}
	return active
}

// worth is what the days' requests were worth.
func (days yearDays) worth() Worth {
	var w Worth
	for _, m := range days.models() {
		w.addModel(m)
	}
	return w
}

// firstUsed is the date of the first day with any request: "" where none
// has.
func (days yearDays) firstUsed() string {
	if i := slices.IndexFunc(days, func(d Day) bool { return d.requests() > 0 }); i >= 0 {
		return days[i].Day
	}
	return ""
}

// versions are the versions the days' requests were of, by their names, in
// the version table's order, then those it doesn't name, by theirs.
func (days yearDays) versions(v versions) []string {
	ranks := make(map[string]int)
	for _, m := range days.models() {
		if m.Model != "" && m.Requests() > 0 {
			_, rank := v.of(m.Model)
			ranks[v.table.VersionName(m.Model)] = rank
		}
	}
	return slices.SortedFunc(maps.Keys(ranks), func(a, b string) int { return cmp.Or(cmp.Compare(ranks[a], ranks[b]), cmp.Compare(a, b)) })
}

// yearView is the Year's period as asked for: what it is, the plans of the
// accounts shown, the period's days, every account's use and as narrowed to
// what's shown, and the date the ledger began recording on, where it tells
// it: "" where it doesn't.
type yearView struct {
	ask   YearAsk
	span  yearSpan
	v     versions
	plans plans
	all   yearDays
	shown yearDays
	began string
}

// totals are the shown days' totals, as Accounts counts every account's:
// each session once, of the version shown, those of its requests.
func (y yearView) totals() AccountTotals {
	return accountTotalsOf(nil, y.shown, nil, y.v).Accounts[0]
}

// numbers are the period's numbers of all use, or an account's: what its
// plans cost, and the worth against it, among them.
func (y yearView) numbers() YearNumbers {
	t := y.totals()
	return YearNumbers{
		Requests: t.Requests, Sessions: t.Sessions, Limits: t.Limits, Worth: t.Worth, Against: y.against(),
		BusiestDay: t.BusiestDay, LongestRun: t.LongestRun,
	}
}

// versionNumbers are the period's numbers of the version shown: its share
// of all use's worth, and the day with the date first it was first used,
// among them.
func (y yearView) versionNumbers(first string) YearNumbers {
	t := y.totals()
	return YearNumbers{
		Requests: t.Requests, Sessions: t.Sessions, Worth: t.Worth, Share: shareOf(t.Worth, y.all.worth().amount()),
		BusiestDay: t.BusiestDay, LongestRun: t.LongestRun, FirstUsed: first,
	}
}

// against is what the shown accounts' plans cost over the period, a day
// each, from the later of its first day and the day the ledger began to its
// end, and the worth against it: none where the ledger began after it.
func (y yearView) against() Against {
	from := max(y.span.first, y.began)
	start, _ := dayfile.Civil(from)
	end, _ := dayfile.Civil(y.span.end)
	if end.Before(start) {
		return Against{}
	}
	return y.plans.against(accountWorths(y.shown), from, days(int(end.Sub(start)/(24*time.Hour))+1))
}

// months are BY MONTH's rows of all use, or an account's: each against what
// the plans shown cost that month, as monthsOf prices it, the period's first
// month from its first day.
func (y yearView) months() []MonthRow {
	months := monthsOf(y.shown, y.plans, y.v, y.span.first, y.span.today)
	rows := make([]MonthRow, len(months))
	for i, m := range months {
		rows[i] = MonthRow{Month: m, From: y.span.partWay(m.Month, y.began), SoFar: y.span.running(m.Month)}
	}
	return rows
}

// versionMonths are BY MONTH's rows of the version shown: a row each month
// it was used in, with its share of the month's worth, the month it was
// first used in, on the day with the date first, from then.
func (y yearView) versionMonths(first string) []MonthRow {
	whole := make(map[string]ledger.Picodollars)
	for _, s := range spansOf(y.all, monthKey) {
		whole[s.key] = yearDays(s.days).worth().amount()
	}
	var rows []MonthRow
	for _, m := range monthsOf(y.shown, plans{}, y.v, y.span.first, y.span.today) {
		if m.Requests > 0 {
			rows = append(rows, MonthRow{Month: m, Share: shareOf(m.Worth, whole[m.Month]), From: y.span.partWay(m.Month, first), SoFar: y.span.running(m.Month)})
		}
	}
	return rows
}

// before is the row for the months held before the version shown was first
// used, on the day with the date first, every month held where it never
// was, and the version of its family, the one given, most used then: nil
// where it was used from the first month held.
func (y yearView) before(first, family string) *MonthsBefore {
	from, _ := monthOf(max(y.span.first, y.began))
	to, _ := monthOf(y.span.last)
	if first != "" {
		usedIn, _ := monthOf(first)
		used, _ := dayfile.MonthStart(usedIn)
		to = dayfile.MonthOf(used.AddDate(0, -1, 0))
	}
	if to < from {
		return nil
	}
	return &MonthsBefore{From: from, To: to, Then: y.then(first, family)}
}

// then is the version of the family given used most before the day with the
// date first, by requests, then in the version table's order, other than the
// version shown, by its name: "" where none was.
func (y yearView) then(first, family string) string {
	requests, ranks := make(map[string]int), make(map[string]int)
	for _, d := range y.all {
		if first != "" && d.Day >= first {
			break
		}
		for _, m := range (yearDays{d}).models() {
			name := y.v.table.VersionName(m.Model)
			if m.Model == "" || name == y.ask.Version || y.v.familyOf(m.Model) != family || m.Requests() == 0 {
				continue
			}
			requests[name] += m.Requests()
			_, ranks[name] = y.v.of(m.Model)
		}
	}
	if len(requests) == 0 {
		return ""
	}
	return slices.MinFunc(slices.Collect(maps.Keys(requests)), func(a, b string) int {
		return cmp.Or(cmp.Compare(requests[b], requests[a]), cmp.Compare(ranks[a], ranks[b]), cmp.Compare(a, b))
	})
}

// version is the version shown, as the version table names it; or, one it
// doesn't, by its name and its family, as its requests give it.
func (y yearView) version() ledger.Version {
	if i := slices.IndexFunc(y.v.table.Versions, func(v ledger.Version) bool { return v.Name == y.ask.Version }); i >= 0 {
		return y.v.table.Versions[i]
	}
	shown := ledger.Version{Name: y.ask.Version}
	if models := y.shown.models(); len(models) > 0 {
		shown.Family = y.v.familyOf(models[0].Model)
	}
	return shown
}

// families are the families BY MONTH's rows are split by: those whose
// requests were worth anything in any of them, in the version table's order.
func (y yearView) families(rows []MonthRow) []string {
	sums := familySums{}
	for _, row := range rows {
		for _, f := range row.ByFamily {
			if f.amount() > 0 {
				sums.add(f.Family, f.Worth)
			}
		}
	}
	listed := y.v.byFamily(sums)
	names := make([]string, len(listed))
	for i, f := range listed {
		names[i] = f.Family
	}
	return names
}
