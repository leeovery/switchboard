package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
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
}

// WeeksRow is an account's row of Weeks, or every account's: its verdict, as
// Capacity gives it, and its peak in each of the eight weeks, as Weeks lists
// them. Of an account, Label is its label, as the config gives it, and
// Primary marks the primary; and Used is what it used of its weeks its
// verdict is over, their peaks' mean: nil where none is known.
type WeeksRow struct {
	AccountVerdict
	Label   string
	Primary bool
	Used    *float64
	Peaks   []WeekPeak
}

// WeekPeak is how full an account's week got: Share, its peak, from 0 to 1,
// nil where it isn't known; and Open, set for this week's, its highest so
// far.
type WeekPeak struct {
	Share *float64
	Open  bool
}

// peakSoFar marks a peak so far, of a week still running, after it.
const peakSoFar = "…"

// Peak says a week's peak, share, as a whole percentage, as "60%"; and of a
// week still running, open, as its peak so far, as "34%…".
func Peak(share float64, open bool) string {
	if open {
		return prose.Percent(share) + peakSoFar
	}
	return prose.Percent(share)
}

// weeksPage is Weeks' page: the eight weeks to this one, the accounts' own
// weeks placed under them as Weeks places them, the resets of the days
// lookBack before them telling the week still running as they begin; and
// the capacity verdict over those of them that are whole.
func (p Past) weeksPage() WeeksPage {
	in := p.in
	today := dateOf(in.Now)
	this, first := weekOf(today, in.WeekStarts), weeksTo(today, in.WeekStarts, weeksShown)
	before, days := splitAt(p.days, first.Format(time.DateOnly))
	_, before = splitAt(before, first.AddDate(0, 0, -lookBack).Format(time.DateOnly))
	placed := weeklyOf(in.WeekWindow, in.WeekStarts, in.Now).placedWeeks(before, days, in.Accounts)
	isWhole := wholeWeek(days, this)
	pl := plans{accounts: in.Accounts, table: in.Prices}
	j := judgingOf(placed, isWhole, in.Accounts, pl, in.WeekWindow)
	page := WeeksPage{
		Weeks:    weekDates(first, weeksShown),
		Capacity: j.capacity(),
		Window:   in.WeekWindow,
	}
	byWeek := make(map[string]Week, len(placed))
	for _, w := range weeksOf(placed, in.Accounts) {
		byWeek[w.Week] = w
	}
	for x, a := range in.Accounts {
		row := WeeksRow{AccountVerdict: page.Capacity.Accounts[x], Label: a.Label, Primary: a.Primary, Peaks: page.peaksOf(byWeek, x)}
		if rec := j.records[x]; rec.weeks > 0 {
			used := meanOf(rec.used, rec.weeks)
			row.Used = &used
		}
		page.Accounts = append(page.Accounts, row)
	}
	page.All = WeeksRow{AccountVerdict: page.Capacity.Accounts[len(in.Accounts)], Peaks: page.peaksOf(byWeek, len(in.Accounts))}
	page.judge(j.r, pl, today.Format(time.DateOnly))
	return page
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
			peaks[i].Share = &share
		}
		peaks[i].Open = slices.Contains(a.Open, page.Window)
	}
	return peaks
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
