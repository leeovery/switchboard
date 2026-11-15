package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// WeeksPage is Weeks as the dashboard draws it: the eight weeks to this one,
// by their first days; each account's peaks in them and its verdict, and
// every account's; and the capacity verdict over the last 7 whole weeks.
type WeeksPage struct {
	// Weeks are the eight weeks' first days' dates, oldest first, this week
	// last.
	Weeks []string
	// Accounts are each configured account's row, in the config's order,
	// and All every account's, each week the mean of their peaks.
	Accounts []WeeksRow
	All      WeeksRow
	Capacity Capacity
	// Window is the key of the window a week long the peaks are of, as
	// internal/claude names it.
	Window string
	// ShortEach are the accounts, by id, in the config's order, that hit
	// their limit in each week Capacity counts short, where the same ones
	// did each time: none where they differed, or none was short.
	ShortEach []string
	// SameWeeks is set where the replay without Capacity's Drop finds short
	// the very weeks short as the accounts stand, and some are; and
	// NoneShortWithout where it finds none short that weren't already. Each
	// is unset where none is named to drop.
	SameWeeks, NoneShortWithout bool
	// DropPrice is what the plan of Capacity's Drop costs a month, as of
	// today: nil where its plan isn't known, or none is named to drop.
	DropPrice *ledger.Picodollars

	// past is what the page was built of, for At to build it afresh.
	past Past
}

// At is the page as it stands at the moment m, of the document of its
// accounts: built afresh of the days as they stand at m, as Past's filedAt
// files them, where they stand otherwise than as it was built, so its weeks
// are Accounts' at the same moment; else as it was built.
func (page WeeksPage) At(m status.Moment) WeeksPage {
	stood, ok := page.past.filedAt(m)
	if !ok {
		return page
	}
	return stood.weeksPage()
}

// Until is when the page, as At stands it at the moment m, next stands
// otherwise, as Past's standsUntil has it: till then, At stands it alike at
// every moment of m's document.
func (page WeeksPage) Until(m status.Moment) time.Time {
	return page.past.standsUntil(m)
}

// WeeksRow is an account's row of Weeks, or every account's: its verdict, as
// Capacity gives it, and its peak in each of the eight weeks, as Weeks lists
// them. Of an account, its verdict's AccountName names it, and Primary
// marks the primary; and Used is what it used of its weeks its
// verdict is over, their peaks' mean: nil where none is known.
type WeeksRow struct {
	AccountVerdict
	Primary bool
	Used    *float64
	Peaks   []WeekPeak

	// told is what each of the page's weeks tells of it, as its peaks are
	// placed, as Weeks' facts sum them: none of a week not whole.
	told []told
}

// WeekPeak is how full an account's week got: Share, its peak, from 0 to 1,
// nil where it isn't known; Open, set for this week's, its highest so far;
// and AtCap, set where its peak reached its cap, as capOf has it, or of every
// account's, where each one's whose peak is known did. This week's so far
// and weeks not whole are marked too, though Weeks counts a limit in whole
// weeks that have ended alone.
type WeekPeak struct {
	Share *float64
	Open  bool
	AtCap bool
}

// PeakSoFar marks a peak so far, of a week still running: after it, or
// under its column, where Compare's WEEKS charts it.
const PeakSoFar = "…"

// Peak says a week's peak, share, as a whole percentage, as "60%"; and of a
// week still running, open, as its peak so far, as "34%…".
func Peak(share float64, open bool) string {
	if open {
		return prose.Percent(share) + PeakSoFar
	}
	return prose.Percent(share)
}

// weeksPage is Weeks' page: the eight weeks to this one, as weeksOver has
// them.
func (p Past) weeksPage() WeeksPage {
	return p.weeksOver(weeksShown)
}

// weeksOver is Weeks' page over the n weeks to this one: the accounts' own
// weeks placed under them as Weeks places them, the resets of the days
// lookBack before them telling the week still running as they begin; and
// the capacity verdict over those of them that are whole. Weeks' own page
// is of eight; Compare's WEEKS reaches over as many as its period has.
func (p Past) weeksOver(n int) WeeksPage {
	in := p.in
	today := dayfile.CivilOf(in.Now)
	this, first := weekOf(today, in.WeekStarts), weeksTo(today, in.WeekStarts, n)
	before, days := splitAt(p.days, first.Format(time.DateOnly))
	_, before = splitAt(before, first.AddDate(0, 0, -lookBack).Format(time.DateOnly))
	placed := weeklyOf(in.WeekWindow, in.WeekStarts, in.Now).placedWeeks(before, days, in.Accounts)
	isWhole := wholeWeek(days, this)
	pl := plans{accounts: in.Accounts, table: in.Prices}
	j := judgingOf(placed, isWhole, in.Accounts, pl, in.WeekWindow)
	page := WeeksPage{
		Weeks:    weekDates(first, n),
		Capacity: j.capacity(),
		Window:   in.WeekWindow,
		past:     p,
	}
	byWeek := make(map[string]Week, len(placed))
	for _, w := range weeksOf(placed, in.Accounts) {
		byWeek[w.Week] = w
	}
	tells := page.toldOf(placed, isWhole, j.r)
	for x, a := range in.Accounts {
		row := WeeksRow{AccountVerdict: page.Capacity.Accounts[x], Primary: a.Primary, Peaks: page.peaksOf(byWeek, x), told: tells[x]}
		if rec := j.records[x]; rec.weeks > 0 {
			used := meanOf(rec.used, rec.weeks)
			row.Used = &used
		}
		page.Accounts = append(page.Accounts, row)
	}
	all := len(in.Accounts)
	page.All = WeeksRow{AccountVerdict: page.Capacity.Accounts[all], Peaks: page.peaksOf(byWeek, all), told: tells[all]}
	page.judge(j.r, pl, dayfile.DateOf(in.Now))
	return page
}

// toldOf is what each of the page's weeks tells of each account, in the
// config's order, then of every account together, of the weeks placed that
// isWhole reports are whole, as the replay r judges them: of an account, its
// record in it, as recordOf has it; of every account, what toldOf tells of
// a week the replay judges. A week not whole, or not placed, tells nothing.
func (page WeeksPage) toldOf(placed []placedWeek, isWhole func(week string) bool, r replay) [][]told {
	accounts := len(r.caps)
	byWeek := make(map[string]placedWeek, len(placed))
	for _, w := range placed {
		byWeek[w.week] = w
	}
	tells := make([][]told, accounts+1)
	for x := range tells {
		tells[x] = make([]told, len(page.Weeks))
	}
	for i, date := range page.Weeks {
		w, ok := byWeek[date]
		if !ok || !isWhole(date) {
			continue
		}
		for x := range accounts {
			tells[x][i] = w.recordOf(x, r.caps[x], page.Window).told()
		}
		if week, judged := w.judged(accounts, page.Window); judged {
			tells[accounts][i] = r.toldOf(week)
		}
	}
	return tells
}

// peaksOf are the peaks of the account with index x among the weeks' own,
// or of every account's, where x is past theirs, in each of the page's
// weeks: unknown in a week the days don't span.
func (page WeeksPage) peaksOf(byWeek map[string]Week, x int) []WeekPeak {
	peaks := make([]WeekPeak, len(page.Weeks))
	for i, date := range page.Weeks {
		w, ok := byWeek[date]
		if !ok || x >= len(w.Accounts) {
			continue
		}
		a := w.Accounts[x]
		if share, known := a.Peaks[page.Window]; known {
			peaks[i].Share, peaks[i].AtCap = &share, page.atCap(w, x)
		}
		peaks[i].Open = slices.Contains(a.Open, page.Window)
	}
	return peaks
}

// atCap reports whether the account with index x hit its limit in the week,
// its peak at its cap, or where x is past theirs, whether every account
// whose peak is known in it did.
func (page WeeksPage) atCap(w Week, x int) bool {
	accounts := page.past.in.Accounts
	reached := func(j int) bool {
		share, known := w.Accounts[j].Peaks[page.Window]
		return !known || millionthsOf(share) >= capOf(accounts[j])
	}
	if x < len(accounts) {
		return reached(x)
	}
	for j := range accounts {
		if !reached(j) {
			return false
		}
	}
	return true
}

// judge gives what the verdict says beside its counts, of the replay r of
// the accounts on the plans pl: who was short each time; and where one is
// named to drop, how the weeks short without it compare with now's, and its
// plan's price on the day with the date today.
func (page *WeeksPage) judge(r replay, pl plans, today string) {
	for _, x := range r.shortEach() {
		page.ShortEach = append(page.ShortEach, pl.accounts[x].ID)
	}
	drop := slices.IndexFunc(pl.accounts, func(a config.Account) bool { return a.ID == page.Capacity.Drop })
	if drop < 0 {
		return
	}
	page.SameWeeks, page.NoneShortWithout = r.without(drop)
	if plan, ok := pl.of(page.Capacity.Drop); ok {
		price := plan.Price(today)
		page.DropPrice = &price
	}
}
