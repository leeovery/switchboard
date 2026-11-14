package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
)

// reachLeast are the fewest weeks Compare's WEEKS reaches over, whatever the
// period.
const reachLeast = 8

// reachWeeks is how many weeks Compare's WEEKS reaches over at the period,
// today being the day given, as dayfile.Civil gives a day, weeks starting on
// the day given: the weeks that begin in the period, to this one, but never
// fewer than reachLeast. So eight at today, this week and the last 30 days,
// 12 at the last 12 weeks, and 52 at the last 12 months.
func (p Period) reachWeeks(today time.Time, starts time.Weekday) int {
	first := p.first(today, starts)
	begun := weekOf(first, starts)
	if begun.Before(first) {
		begun = begun.AddDate(0, 0, 7)
	}
	return max(int(weekOf(today, starts).Sub(begun)/(7*24*time.Hour))+1, reachLeast)
}

// reached sets the page's reach and each row's peaks in it and Weeks'
// verdict on it, as weeks gives them, Weeks over the weeks the reach is of,
// at in: a column a week, as weekBuckets names them; or of the last 12
// months, a column a month, the months they fall in, as monthBuckets names
// them, each its fullest week's.
func (page *AccountsPage) reached(in HistoryInput, weeks WeeksPage) {
	today := dayfile.CivilOf(in.Now)
	first, _ := dayfile.Civil(weeks.Weeks[0])
	page.Reach = weekBuckets(first, today, in.WeekStarts)
	columns := func(peaks []WeekPeak) []WeekPeak { return peaks }
	if page.Period == PeriodTwelveMonths {
		weekly := page.Reach
		page.Reach = monthBuckets(page.Period.first(today, in.WeekStarts), today)
		columns = func(peaks []WeekPeak) []WeekPeak { return fullestIn(page.Reach, weekly, peaks) }
	}
	for x := range page.Rows {
		page.Rows[x].Reached, page.Rows[x].Reaching = columns(weeks.Accounts[x].Peaks), weeks.Accounts[x].AccountVerdict
	}
	page.All.Reached, page.All.Reaching = columns(weeks.All.Peaks), weeks.All.AccountVerdict
}

// fullestIn are the peaks of the months given, each its fullest week's of
// the weeks given, by their first days, peaks holding each week's: of the
// weeks that begin in it, the one whose peak is known and highest, the
// first of any that tie; unknown in a month none of whose weeks' is known.
func fullestIn(months, weeks []Bucket, peaks []WeekPeak) []WeekPeak {
	fullest := make([]WeekPeak, len(months))
	m := 0
	for i, w := range weeks {
		for m+1 < len(months) && !w.Start.Before(months[m+1].Start) {
			m++
		}
		if p := peaks[i]; p.Share != nil && (fullest[m].Share == nil || *p.Share > *fullest[m].Share) {
			fullest[m] = p
		}
	}
	return fullest
}

// worthsByDay are what each configured account's requests were worth each
// of days, in the config's order, then every account's, those no account
// answered among them, as Accounts' totals count them.
func worthsByDay(days []Day, accounts config.Accounts) []dayWorths {
	worths := make([]dayWorths, len(accounts)+1)
	place := make(map[string]int, len(accounts))
	for x, a := range accounts {
		worths[x], place[a.ID] = dayWorths{}, x
	}
	all := len(accounts)
	worths[all] = dayWorths{}
	for _, d := range days {
		for _, a := range d.Accounts {
			var w Worth
			for _, m := range a.Models {
				w.addModel(m)
			}
			worths[all].add(d.Day, w)
			if x, ok := place[a.Account]; ok {
				worths[x].add(d.Day, w)
			}
		}
	}
	return worths
}

// Hourly is the page with today's worth charted by the hour, where its
// period is today and d's fold of the tail is of today: each row's, what
// the requests the fold holds that went to its account were worth by the
// local hour each arrived in, those of no session among them, as the day's
// summary counts them, and all accounts', every one's, those none answered
// among them, priced at today's prices, as Projects charts a project's
// today, worthCharts' one rule. Else it's the page as it is, its today by
// the day.
func (p AccountsPage) Hourly(d SessionDays) AccountsPage {
	today := dayfile.DateOf(p.in.Now)
	if p.Period != PeriodToday || d.today.Date != today {
		return p
	}
	byAccount, all := d.held.worthByHour(pricing{prices: p.in.Prices, date: today})
	p.Rows = slices.Clone(p.Rows)
	for x, a := range p.in.Accounts {
		hours := byAccount[a.ID]
		p.Rows[x].WorthBy = worthCharts(PeriodToday, p.in.Now, p.in.WeekStarts, nil, &hours)
	}
	p.All.WorthBy = worthCharts(PeriodToday, p.in.Now, p.in.WeekStarts, nil, &all)
	return p
}

// worthByHour is what the fold's requests were worth by the local hour each
// arrived in, its sessions' and those of none, priced as price prices them:
// by the account each went to, and every account's together.
func (d daySessions) worthByHour(price pricing) (byAccount map[string][24]Worth, all [24]Worth) {
	byAccount = make(map[string][24]Worth)
	take := func(hours accountHours) {
		for account, on := range hours {
			sums := byAccount[account]
			for h := range on {
				w := price.worth(on[h])
				sums[h].add(w)
				all[h].add(w)
			}
			byAccount[account] = sums
		}
	}
	for _, s := range d.held {
		take(s.hours)
	}
	take(d.loose)
	return byAccount, all
}
