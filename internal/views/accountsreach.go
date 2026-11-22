package views

import (
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/score"
)

// reachLeast are the fewest weeks Compare's WEEKS reaches over, whatever the
// period, and detailLeast the fewest Detail's weeks section does.
const (
	reachLeast  = 8
	detailLeast = 6
)

// reachWeeks is how many weeks Compare's WEEKS reaches over at the period,
// today being the day given, as dayfile.Civil gives a day, weeks starting on
// the day given: the weeks that begin in the period, to its last week, as
// begunWeeks counts them, but never fewer than reachLeast. So eight at
// today, this week and the last 30 days, 12 at the last 12 weeks, and 52 at
// the last 12 months.
func (p Period) reachWeeks(today time.Time, starts time.Weekday) int {
	return max(p.begunWeeks(today, starts), reachLeast)
}

// begunWeeks is how many weeks begin in the period, today being the day
// given, as dayfile.Civil gives a day, weeks starting on the day given: from
// the first that begins on its first day or after, to its last week, this
// one at now, which it counts whether it begins in the period or not.
func (p Period) begunWeeks(today time.Time, starts time.Weekday) int {
	s := p.spanAt(today, starts)
	begun := weekOf(s.first, starts)
	if begun.Before(s.first) {
		begun = begun.AddDate(0, 0, 7)
	}
	return int(s.lastWeek(starts).Sub(begun)/(7*24*time.Hour)) + 1
}

// through is the last day of the span that's come, as dayfile.Civil gives a
// day: its last, but today where its last is still to come, as a week still
// running's is.
func (s periodSpan) through() time.Time {
	if s.last.After(s.today) {
		return s.today
	}
	return s.last
}

// lastWeek is the first day of the span's last week, as dayfile.Civil gives
// a day, weeks starting on the day given: this week's at now. Every count of
// the period's weeks ends at it.
func (s periodSpan) lastWeek(starts time.Weekday) time.Time {
	return weekOf(s.last, starts)
}

// wholeTo is the last day of the last whole week of the span gone, as
// dayfile.Civil gives a day, weeks starting on the day given: its last
// week's, where that's before this one; the week before this one's, where
// its last week is this one, still running; zero of a span at now, whose
// weeks run to now.
func (s periodSpan) wholeTo(starts time.Weekday) time.Time {
	if !s.gone {
		return time.Time{}
	}
	return score.Earlier(s.lastWeek(starts).AddDate(0, 0, 6), weekOf(s.today, starts).AddDate(0, 0, -1))
}

// detailWeeks is how many weeks Detail's weeks section reaches over at the
// period at the fewest, today being the day given, as dayfile.Civil gives a
// day, weeks starting on the day given: the weeks that begin in the period,
// as begunWeeks counts them, but never fewer than detailLeast. So six at
// today, this week and the last 30 days, 12 at the last 12 weeks, and 52 or
// 53 at the last 12 months. It shows more where more fit.
func (p Period) detailWeeks(today time.Time, starts time.Weekday) int {
	return max(p.begunWeeks(today, starts), detailLeast)
}

// WeeksLeast is how many weeks Detail's weeks section reaches over a period
// of the length at the fewest, as detailWeeks has it, whatever the day and
// the day weeks start on: the fewest of any day of a week, as each falls on
// another day of its week, of a year without a 29 February. So six at today,
// this week and the last 30 days, 12 at the last 12 weeks, and 52 at the
// last 12 months. It's known before any page is read, for Detail to lay its
// weeks out by, so they hold still as its page is read.
func (l Length) WeeksLeast() int {
	least := math.MaxInt
	for d := range 7 {
		day := time.Date(2027, time.March, 1+d, 0, 0, 0, 0, time.UTC)
		least = min(least, l.Now().detailWeeks(day, time.Monday))
	}
	return least
}

// reached sets the page's reach and each row's peaks in it and Weeks'
// verdict on it, as weeks gives them, Weeks over the weeks the reach is of,
// to the last week of the period's span s, at in: a column a week, as
// weekBuckets names them; or of 12 months, a column a month, the months
// they touch, as monthBuckets names them, each as full as its fullest week
// got, at its cap where any of its weeks was, as fullestIn has them.
func (page *AccountsPage) reached(in HistoryInput, weeks WeeksPage, s periodSpan) {
	first, _ := dayfile.Civil(weeks.Weeks[0])
	page.Reach = weekBuckets(first, s.through(), in.WeekStarts)
	columns := func(peaks []WeekPeak) []WeekPeak { return peaks }
	if page.Period.length == LengthTwelveMonths {
		weekly := page.Reach
		page.Reach = monthBuckets(s.first, s.through())
		columns = func(peaks []WeekPeak) []WeekPeak { return fullestIn(page.Reach, weekly, peaks) }
	}
	for x := range page.Rows {
		page.Rows[x].Reached, page.Rows[x].Reaching = columns(weeks.Accounts[x].Peaks), weeks.Accounts[x].AccountVerdict
	}
	page.All.Reached, page.All.Reaching = columns(weeks.All.Peaks), weeks.All.AccountVerdict
}

// fullestIn are the peaks of the months given, of the weeks given, by their
// first days, peaks holding each week's: of the weeks that begin in a month,
// the share and Open of the one whose peak is known and highest, the first
// of any that tie; unknown in a month none of whose weeks' is known. A month
// is at its cap where any of its weeks is, and isn't open where a week other
// than its fullest is, so a short week is marked though this week so far is
// fuller.
func fullestIn(months, weeks []Bucket, peaks []WeekPeak) []WeekPeak {
	fullest, capped := make([]WeekPeak, len(months)), make([]int, len(months))
	m := 0
	for i, w := range weeks {
		for m+1 < len(months) && !w.Start.Before(months[m+1].Start) {
			m++
		}
		if p := peaks[i]; p.Share != nil && (fullest[m].Share == nil || *p.Share > *fullest[m].Share) {
			fullest[m] = p
		}
		if peaks[i].AtCap {
			capped[m]++
		}
	}
	for m := range fullest {
		if capped[m] > 1 || capped[m] == 1 && !fullest[m].AtCap {
			fullest[m].Open, fullest[m].AtCap = false, true
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

// Hourly is the page with its day's worth charted by the hour, where its
// period is the day the fold given is of, its hours known, as DayFold has
// them, the fold Period's HoursFold picks of the day's read and the tail's,
// as worthCharts charts a day's hours: each row's, what the requests
// the fold holds that went to its account were worth by the local hour each
// arrived in, those of no session among them, as the day's summary counts
// them, and all accounts', every one's, those none answered among them,
// priced at today's prices, as the days are, as Projects charts a project's
// day, worthCharts' one rule. Else it's the page as it is, its days by the
// day, as where it's today and the fold is of the day before, it's a day
// gone and the fold is today's, or the day's lines are no longer kept.
func (p AccountsPage) Hourly(f DayFold) AccountsPage {
	on := f.hoursOn()
	if !p.Period.hourly(on, p.in.Now, p.in.WeekStarts) {
		return p
	}
	byAccount, all := f.held.worthByHour(pricing{prices: p.in.Prices, date: dayfile.DateOf(p.in.Now)})
	p.Rows = slices.Clone(p.Rows)
	for x, a := range p.in.Accounts {
		p.Rows[x].WorthBy = worthCharts(p.Period, p.in.Now, p.in.WeekStarts, p.in.Began, nil, &hourWorths{date: on, worth: byAccount[a.ID]})
	}
	p.All.WorthBy = worthCharts(p.Period, p.in.Now, p.in.WeekStarts, p.in.Began, nil, &hourWorths{date: on, worth: all})
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
