package capture

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// monthWorths are BY MONTH's months as the frames draw them, from October
// 2025, the first of the last 12 months, to September 2026, each its worth
// in dollars (acct11.MONTHS): each has 8.6 requests a dollar, and a 5-hour
// limit reached for each $52, as the frames work them out.
var monthWorths = []float64{1180, 1340, 1210, 1560, 1720, 1980, 2150, 2330, 2470, 2690, 2860, 3120}

// The rates the frames' months work out their requests and limits at.
const (
	requestsADollar = 8.6
	dollarsALimit   = 52
)

// weekUse is an account's use this week, Monday to now, as Accounts draws it
// (acct3.STATS and acct7.WORTH, and acct4's and acct9's for the accounts of
// the frames of six and ten): its requests and its worth, in dollars; and
// what of it today's lines hold, which the sessions' world gives, the rest
// being the days before today's.
type weekUse struct {
	requests int
	worth    float64
	today    weekToday
}

// weekToday is what today's lines hold of an account's week: its requests,
// and their worth by model version, in dollars.
type weekToday struct {
	requests int
	worths   map[string]float64
}

// worth is what today's lines are worth, in dollars.
func (t weekToday) worth() float64 {
	sum := 0.0
	for _, v := range versionOrder {
		sum += t.worths[v]
	}
	return sum
}

// beforeToday is the account's use this week but what today's lines hold: its
// requests, and their worth, in dollars.
func (u weekUse) beforeToday() (int, float64) {
	return u.requests - u.today.requests, u.worth - u.today.worth()
}

// weekUses are each account's use this week. Today's is what the sessions'
// world's lines make it, to a tenth of a cent: work's 592 requests, $57.08,
// personal's 463, $48.13, and side's 184, $20.74, a request short of what the
// Log counts, as c61b's fifth is still answering at 14:42:07, all of Opus 5.5
// but their side requests, of Haiku 4.5; and none of the seven accounts the
// frames of six and ten add.
var weekUses = map[string]weekUse{
	"work":     {requests: 1204, worth: 184, today: weekToday{592, map[string]float64{opus55: 57.036, haiku45: 0.041}}},
	"personal": {requests: 2330, worth: 212, today: weekToday{463, map[string]float64{opus55: 48.097, haiku45: 0.030}}},
	"side":     {requests: 1876, worth: 239, today: weekToday{184, map[string]float64{opus55: 20.717, haiku45: 0.025}}},
	"client":   {requests: 1410, worth: 151},
	"spare":    {requests: 96, worth: 9},
	"lab":      {requests: 1102, worth: 133},
	"team":     {requests: 1980, worth: 201},
	"extra":    {requests: 310, worth: 31},
	"ops":      {requests: 520, worth: 52},
	"studio":   {requests: 870, worth: 98},
}

// yesterday is Wednesday 30 September as the Log tells of it: its requests,
// its worth, in dollars, and its sessions.
var yesterday = struct {
	requests int
	worth    float64
	sessions int
}{1912, 212, 17}

// The sessions of yesterday's that ran on into today, as the sessions' world
// has them, beside db8a: 4b0e on work; 5e7d on side; and 8a2c, which started
// on personal and moved to side as personal reached its limit.
const (
	id4B0E = "4b0e91c7-2f58-4d36-a0e4-6c1b8d3f7a92"
	id5E7D = "5e7d2a06-b9c4-4f17-8d53-e2a7c0f9b148"
	id8A2C = "8a2c4f19-d06e-4b7a-93c5-1e8f6b2d0a57"
)

// weekSessions are how many sessions each of the accounts the frames of six
// and ten add served this week, as Accounts draws them (acct4.STATS and
// acct9.STATS).
var weekSessions = map[string]int{"client": 5, "spare": 1, "lab": 4, "team": 7, "extra": 2, "ops": 3, "studio": 4}

// requestsASession is how many requests a session's day has, as the days
// before the last 12 months, and the eight weeks of the seven accounts the
// frames of six and ten add, are counted into sessions.
const requestsASession = 126

// The sessions Year draws over the last 12 months, and how many of today's
// the sessions' world has that didn't run on from yesterday: 14, but db8a,
// 4b0e, 5e7d and 8a2c.
const (
	yearSessions      = 1688
	todaysNewSessions = 10
)

// lastTwelveMonths is the first day of the year of the last 12 months to
// today: Thursday 2 October 2025.
const lastTwelveMonths = 3

// three are the accounts of most frames, whose use the year's days hold, the
// seven more of the frames of six and ten having theirs for the eight weeks
// and the week before them alone.
var three = []string{"work", "personal", "side"}

// accountDay is an account's use of a day: the share of its week its use
// rose; its requests, their worth, in dollars, and that worth's share by
// model version; its sessions, by id, and those moved onto it and off it;
// and its 5-hour windows.
type accountDay struct {
	rise     float64
	requests int
	worth    float64
	versions map[string]float64
	sessions []string
	movedOn  int
	movedOff int
	windows  []session
}

// year is the accounts' use over the year, day by day, to yesterday, each
// account's days by its id.
type year struct {
	now  time.Time
	days map[string][]accountDay
}

// newYear is the accounts' use over the year to the day before now's.
func newYear(now time.Time) year {
	y := year{now: now, days: make(map[string][]accountDay)}
	for _, a := range tenAccounts {
		y.days[a.ID] = make([]accountDay, today)
	}
	y.rise()
	y.spend()
	y.limit()
	y.session()
	y.version()
	return y
}

// firstDay is the first day of the year the account with the given id has
// days of: the year's first, of the three, or the week before the eight
// weeks', of the rest.
func firstDay(id string) int {
	if slices.Contains(three, id) {
		return 0
	}
	return eightWeeks - 7
}

// --------------------------------------------------------- rises

// ownWeek is a week of an account's own, from one reset to the next: how full
// it got, or has got so far, still running; the column of the eight weeks it's
// placed under, as Weeks places it, -1 for one before them; and the days of
// the year its use rose in, those whose middle it holds.
type ownWeek struct {
	start, end time.Time
	peak       float64
	open       bool
	column     int
	days       []int
}

// weeksOf are the account with the given id's own weeks, oldest first, from
// the one holding its first day to the one still running: those placed under
// the eight weeks with their peaks as the frames draw them, and those before
// each a share of the eight weeks' mean as its days' use is of theirs.
func (y year) weeksOf(id string) []ownWeek {
	var weeks []ownWeek
	for back := 0; ; back-- {
		end := weekReset(id, y.now, back)
		w := ownWeek{start: weekReset(id, y.now, back-1), end: end, open: end.After(y.now)}
		w.column = columnOf(w)
		for d := firstDay(id); d <= today; d++ {
			if noon := dayAt(y.now, d, 12, 0, 0); !noon.Before(w.start) && noon.Before(w.end) {
				w.days = append(w.days, d)
			}
		}
		weeks = append(weeks, w)
		if len(w.days) == 0 || w.days[0] <= firstDay(id) {
			break
		}
	}
	slices.Reverse(weeks)
	placed := peaks[id]
	var eight []ownWeek
	for i, w := range weeks {
		if w.column >= 0 {
			weeks[i].peak = placed[w.column] / 100
			eight = append(eight, w)
		}
	}
	for i, w := range weeks {
		if w.column < 0 {
			weeks[i].peak = min(meanPeak(id)*busyOver(w.days)/meanBusy(eight), 0.97)
		}
	}
	return weeks
}

// columnOf is the column of the eight weeks an own week is placed under, as
// Weeks places it: the last for the one still running; else that of the
// calendar week of the day it began, where it began up to half a week in,
// or of the next; -1 for one placed before them.
func columnOf(w ownWeek) int {
	if w.open {
		return 7
	}
	began := dayOf(w.start)
	week := began - (began%7+7)%7
	if began-week > 3 {
		week += 7
	}
	column := (week - eightWeeks) / 7
	if week < eightWeeks || column > 6 {
		return -1
	}
	return column
}

// meanPeak is the mean of the account with the given id's eight weeks' peaks
// but the one still running.
func meanPeak(id string) float64 {
	placed, sum := peaks[id], 0.0
	for _, p := range placed[:7] {
		sum += p
	}
	return sum / 700
}

// busyOver is how busy every account was over the days given.
func busyOver(days []int) float64 {
	sum := 0.0
	for _, d := range days {
		sum += busy(d)
	}
	return sum
}

// meanBusy is how busy every account was over the weeks given, those ended,
// on average.
func meanBusy(weeks []ownWeek) float64 {
	sum, n := 0.0, 0
	for _, w := range weeks {
		if !w.open {
			sum, n = sum+busyOver(w.days), n+1
		}
	}
	return sum / float64(n)
}

// rise sets how far each account's week rose on each of its days: its peak
// over its days as busy as the account was on each, a week that reached its
// limit doing so on its Friday, and none after; the week still running
// rising to its use so far by now, today as busy as the part of it gone.
func (y year) rise() {
	for _, a := range tenAccounts {
		days := y.days[a.ID]
		for _, w := range y.weeksOf(a.ID) {
			weights := make(map[int]float64)
			spent := false
			for _, d := range w.days {
				if spent || d >= today && !w.open {
					break
				}
				weights[d] = accountBusy(a.ID, d)
				if d == today {
					weights[d] *= partGone
				}
				spent = w.peak >= 1 && d%7 == 4
			}
			sum := 0.0
			for _, v := range weights {
				sum += v
			}
			for d, v := range weights {
				if d >= firstDay(a.ID) && d < today {
					days[d].rise = w.peak * v / sum
				}
			}
		}
	}
}

// partGone is the part of today's use gone by the moment drawn.
const partGone = 0.6

// --------------------------------------------------------- worth and requests

// spend sets each account's requests and worth on each day: every one of the
// three's together as busy as the day was, over its month, each month's
// worth and requests as the frames draw them; each's share as far as its
// week rose that day; this week's days before today what today's lines leave
// of the week, yesterday's as the Log has it. The rest of the accounts spend
// as they did this week for each point of their week used.
func (y year) spend() {
	worths, requests := y.monthsSpent()
	for d := range thisWeek {
		y.share(three, d, worths[d], requests[d])
	}
	y.spendThisWeek()
	for _, id := range slices.DeleteFunc(slices.Clone(tenAccounts.IDs()), func(id string) bool { return slices.Contains(three, id) }) {
		y.spendAsThisWeek(id)
	}
}

// monthsSpent are every one of the three's worth and requests each day before
// this week: each month's, as monthWorths gives them, over its days as busy as
// each was, but for this week's days, which spendThisWeek gives, and the days
// of September before the last 12 months, spent as October was.
func (y year) monthsSpent() ([]float64, []int) {
	worths, requests := make([]float64, thisWeek), make([]int, thisWeek)
	thisWeeks, thisWeeksRequests := 0.0, 0
	for _, id := range three {
		requests, worth := weekUses[id].beforeToday()
		thisWeeks += worth
		thisWeeksRequests += requests
	}
	for i, worth := range monthWorths {
		first, last := monthDays(i)
		var months []int
		for d := first; d < min(last, thisWeek); d++ {
			months = append(months, d)
		}
		if i == len(monthWorths)-1 {
			worth -= thisWeeks
		}
		want := int(math.Round(monthWorths[i] * requestsADollar))
		if i == len(monthWorths)-1 {
			want -= thisWeeksRequests
		}
		scale := worth / busyOver(months)
		counts := apportion(want, weightsOf(months, busy))
		for j, d := range months {
			worths[d], requests[d] = busy(d)*scale, counts[j]
		}
		if i == 0 {
			for d := range first {
				worths[d] = busy(d) * scale
				requests[d] = int(math.Round(worths[d] * float64(want) / worth))
			}
		}
	}
	return worths, requests
}

// monthDays are the first day of the year in the ith of monthWorths's months
// and the day after its last: October 2025's from the 2nd, as the last 12
// months to today begin then.
func monthDays(i int) (int, int) {
	first := dayOf(time.Date(2025, time.October+time.Month(i), 1, 0, 0, 0, 0, time.UTC))
	if i == 0 {
		first++
	}
	return first, dayOf(time.Date(2025, time.October+time.Month(i)+1, 1, 0, 0, 0, 0, time.UTC))
}

// share shares a day's worth and requests among the accounts given, each as
// far as its week rose that day: the worth among those that had a request.
func (y year) share(ids []string, d int, worth float64, requests int) {
	rises := make([]float64, len(ids))
	sum := 0.0
	for i, id := range ids {
		rises[i] = y.days[id][d].rise
		sum += rises[i]
	}
	if sum == 0 {
		return
	}
	counts := apportion(requests, rises)
	sum = 0
	for i := range rises {
		if counts[i] == 0 {
			rises[i] = 0
		}
		sum += rises[i]
	}
	for i, id := range ids {
		y.days[id][d].worth, y.days[id][d].requests = worth*rises[i]/sum, counts[i]
	}
}

// spendThisWeek spends each of the three's week, but what today's lines hold
// of it, over this week's days before today: yesterday's requests and worth
// as the Log has them, shared as each account's week rose, and the rest of
// each one's over Monday and Tuesday as its week rose on each.
func (y year) spendThisWeek() {
	wednesday := today - 1
	y.share(three, wednesday, yesterday.worth, yesterday.requests)
	for _, id := range three {
		days := y.days[id]
		requests, worth := weekUses[id].beforeToday()
		requests -= days[wednesday].requests
		worth -= days[wednesday].worth
		rises := []float64{days[thisWeek].rise, days[thisWeek+1].rise}
		counts := apportion(requests, rises)
		for i, rise := range rises {
			days[thisWeek+i].worth = worth * rise / (rises[0] + rises[1])
			days[thisWeek+i].requests = counts[i]
		}
	}
}

// spendAsThisWeek spends the account with the given id's days as it spent
// this week before today, worth and requests alike, for each point of its
// week used: no worth on a day of too little use for a request.
func (y year) spendAsThisWeek(id string) {
	days := y.days[id]
	requests, worth := weekUses[id].beforeToday()
	risen := 0.0
	for d := thisWeek; d < today; d++ {
		risen += days[d].rise
	}
	worthAPoint := worth / risen
	requestsAPoint := float64(requests) / risen
	rises := make([]float64, 0, today-thisWeek)
	for d := firstDay(id); d < today; d++ {
		days[d].worth = days[d].rise * worthAPoint
		if d < thisWeek {
			days[d].requests = int(math.Round(days[d].rise * requestsAPoint))
			days[d].worth *= float64(min(days[d].requests, 1))
		} else {
			rises = append(rises, days[d].rise)
		}
	}
	for i, n := range apportion(requests, rises) {
		days[thisWeek+i].requests = n
	}
}

// weightsOf are what weigh gives each of days.
func weightsOf(days []int, weigh func(int) float64) []float64 {
	weights := make([]float64, len(days))
	for i, d := range days {
		weights[i] = weigh(d)
	}
	return weights
}

// apportion shares n among parts as their weights are, in whole parts, each
// rounded down and the rest given to those that lost the most to rounding.
func apportion(n int, weights []float64) []int {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	counts := make([]int, len(weights))
	if sum == 0 {
		return counts
	}
	type part struct {
		i    int
		lost float64
	}
	parts := make([]part, len(weights))
	given := 0
	for i, w := range weights {
		exact := float64(n) * w / sum
		counts[i] = int(math.Floor(exact))
		given += counts[i]
		parts[i] = part{i, exact - float64(counts[i])}
	}
	slices.SortStableFunc(parts, func(a, b part) int { return cmp.Compare(b.lost, a.lost) })
	for _, p := range parts[:n-given] {
		counts[p.i]++
	}
	return counts
}

// --------------------------------------------------------- limits

// limit sets each account's 5-hour windows on each of its days, and those
// that reached their limits: the three's this week as the frames tell of
// them; the eight weeks' and the week before them as acct3.limit_day spreads
// them; and before them, each month's as many as BY MONTH draws, every
// weekday of it work's first, then personal's, then side's.
func (y year) limit() {
	limited := make(map[string]map[int]bool)
	for _, a := range tenAccounts {
		limited[a.ID] = make(map[int]bool)
		for d := max(firstDay(a.ID), eightWeeks-7); d < lastLimited(a.ID); d++ {
			limited[a.ID][d] = limitDay(a.ID, d)
		}
	}
	for i, worth := range monthWorths {
		first, last := monthDays(i)
		var weekdays []int
		for d := first; d < min(last, eightWeeks-7); d++ {
			if d%7 < 5 {
				weekdays = append(weekdays, d)
			}
		}
		n := min(int(math.Round(worth/dollarsALimit)), 3*len(weekdays))
		for j := range n {
			limited[three[j/len(weekdays)]][weekdays[j%len(weekdays)]] = true
		}
	}
	for _, a := range tenAccounts {
		for d := firstDay(a.ID); d < lastLimited(a.ID); d++ {
			y.days[a.ID][d].windows = y.sessionsOf(a.ID, d, limited[a.ID][d])
		}
	}
	y.limitThisWeek()
}

// lastLimited is the day of the year after the last whose 5-hour windows
// limit gives the account with the given id: this week's, of the three,
// limitThisWeek gives.
func lastLimited(id string) int {
	if slices.Contains(three, id) {
		return thisWeek
	}
	return today
}

// fiveHourStarts are when each account's afternoon 5-hour window starts, as
// today's do, its morning one a window before: a window starts at an
// account's first request after its last ended.
var fiveHourStarts = map[string]weekClock{
	"work": {hour: 12, minute: 10}, "personal": {hour: 11, minute: 30}, "side": {hour: 13, minute: 50},
	"client": {hour: 12, minute: 40}, "spare": {hour: 14, minute: 35}, "lab": {hour: 11, minute: 55},
	"team": {hour: 10, minute: 30}, "extra": {hour: 12, minute: 55}, "ops": {hour: 14, minute: 10},
	"studio": {hour: 11, minute: 20},
}

// sessionsOf are the account with the given id's 5-hour windows on day d of
// the year: none on a day it used none of its week; else, on a weekday, a
// morning's and an afternoon's, and at a weekend the afternoon's, each
// rising as far as five times the share of its week the day used, in
// proportion, the afternoon reaching its limit three hours in where
// limited, and its cap shortly before, where the account has one.
func (y year) sessionsOf(id string, d int, limited bool) []session {
	day := y.days[id][d]
	if day.rise == 0 {
		return nil
	}
	c := fiveHourStarts[id]
	afternoon := session{start: dayAt(y.now, d, c.hour, c.minute, 0), use: hundredths(min(day.rise*5*0.6, 0.9)), ran: 4 * time.Hour, step: 0.05}
	if limited {
		afternoon.limited = afternoon.start.Add(3 * time.Hour)
		if capped(id) {
			afternoon.capped = afternoon.limited.Add(-4 * time.Minute)
		}
	}
	if d%7 >= 5 {
		return []session{afternoon}
	}
	morning := session{start: afternoon.start.Add(-sessionLength - 5*time.Minute), use: hundredths(min(day.rise*5*0.4, 0.9)), ran: 4 * time.Hour, step: 0.05}
	return []session{morning, afternoon}
}

// capped reports whether the account with the given id has a cap: a reserve.
func capped(id string) bool {
	i := slices.IndexFunc(tenAccounts, func(a config.Account) bool { return a.ID == id })
	return i >= 0 && tenAccounts[i].Reserve > 0
}

// limitThisWeek sets the three's 5-hour windows on this week's days before
// today as the frames tell of them: on Tuesday, work's limit 43 minutes
// before its window reset, its cap 4 minutes before that, and side's 52
// minutes before its; on Wednesday, work's window from 12:10, at its cap at
// 14:58 and its limit at 15:02 till 17:10, personal's from 18:58, at its
// limit at 19:12:40 till 23:58, and side's from 19:30, running past midnight,
// at 62% at its highest.
func (y year) limitThisWeek() {
	at := func(d, hour, minute, second int) time.Time { return dayAt(y.now, d, hour, minute, second) }
	tuesday, wednesday := thisWeek+1, thisWeek+2
	for _, id := range three {
		for _, d := range []int{thisWeek, tuesday} {
			windows := y.sessionsOf(id, d, false)
			for i := range windows {
				windows[i].step = 0.01
			}
			y.days[id][d].windows = windows
		}
	}
	work, side := y.days["work"][tuesday].windows, y.days["side"][tuesday].windows
	work[1].capped, work[1].limited = at(tuesday, 16, 23, 0), at(tuesday, 16, 27, 0)
	side[1].limited = at(tuesday, 17, 58, 0)
	y.days["work"][wednesday].windows = []session{{start: at(wednesday, 12, 10, 0), capped: at(wednesday, 14, 58, 0), limited: at(wednesday, 15, 2, 0)}}
	y.days["personal"][wednesday].windows = []session{{start: at(wednesday, 18, 58, 0), limited: at(wednesday, 19, 12, 40)}}
	y.days["side"][wednesday].windows = []session{{start: at(wednesday, 19, 30, 0), use: 0.62, ran: 4*time.Hour + 50*time.Minute}}
}

// --------------------------------------------------------- sessions

// session gives each account's day its sessions: the three's over the last
// 12 months before this week as many as Year draws, 1,688, less yesterday's
// and the 10 of today's that didn't run on from yesterday, one at least on a
// day of any request and the rest shared as their requests are; this week's
// as sessionThisWeek gives them; and every other day's one for each
// requestsASession of its requests, one at least. A move takes a session off
// an account at each of its limits before this week, to side, or from side
// to personal.
func (y year) session() {
	type accountDate struct {
		id string
		d  int
	}
	var year []accountDate
	for _, a := range tenAccounts {
		for d := firstDay(a.ID); d < thisWeek; d++ {
			switch {
			case y.days[a.ID][d].requests == 0:
			case slices.Contains(three, a.ID) && d >= lastTwelveMonths:
				year = append(year, accountDate{a.ID, d})
			default:
				n := max(1, int(math.Round(float64(y.days[a.ID][d].requests)/requestsASession)))
				y.days[a.ID][d].sessions = sessionIDs(dateOf(d), a.ID, n)
			}
		}
	}
	rest := apportion(yearSessions-todaysNewSessions-yesterday.sessions-len(year), weightsOf(indexes(len(year)), func(i int) float64 {
		return float64(y.days[year[i].id][year[i].d].requests)
	}))
	for i, at := range year {
		y.days[at.id][at.d].sessions = sessionIDs(dateOf(at.d), at.id, 1+rest[i])
	}
	for d := range thisWeek {
		for _, id := range three {
			if !y.limited(id, d) || len(y.days[id][d].sessions) == 0 {
				continue
			}
			to := "side"
			if id == "side" {
				to = "personal"
			}
			y.move(id, to, d, y.days[id][d].sessions[0])
		}
	}
	y.sessionThisWeek()
}

// limited reports whether the account with the given id reached a 5-hour
// limit on day d of the year.
func (y year) limited(id string, d int) bool {
	return slices.ContainsFunc(y.days[id][d].windows, func(s session) bool { return !s.limited.IsZero() })
}

// move moves the session with the given id off one account onto another on
// day d of the year.
func (y year) move(from, to string, d int, id string) {
	off, on := &y.days[from][d], &y.days[to][d]
	off.movedOff++
	on.movedOn++
	if !slices.Contains(on.sessions, id) {
		on.sessions = append(on.sessions, id)
		slices.Sort(on.sessions)
	}
}

// sessionThisWeek gives yesterday its 17 sessions: db8a's on personal, 4b0e's
// on work and 5e7d's on side, which ran on into today, and 8a2c's, which
// moved from personal to side at personal's limit; and the rest, among the
// three as their requests are, Monday's and Tuesday's sessions of each
// account its own of them, running on into yesterday. Each of the rest of
// the accounts has its sessions this week as Accounts draws them, every one
// running each day of it.
func (y year) sessionThisWeek() {
	wednesday := today - 1
	given := map[string][]string{"work": {id4B0E}, "personal": {idDB8A, id8A2C}, "side": {id5E7D}}
	rest := apportion(yesterday.sessions-4, weightsOf(indexes(len(three)), func(i int) float64 { return float64(y.days[three[i]][wednesday].requests) }))
	for i, id := range three {
		running := sessionIDs(dateOf(wednesday), id, rest[i])
		for d := thisWeek; d < wednesday; d++ {
			y.days[id][d].sessions = slices.Clone(running)
		}
		day := &y.days[id][wednesday]
		day.sessions = append(running, given[id]...)
		slices.Sort(day.sessions)
	}
	y.move("personal", "side", wednesday, id8A2C)
	for id, n := range weekSessions {
		running := sessionIDs(dateOf(thisWeek), id, n)
		for d := thisWeek; d < today; d++ {
			y.days[id][d].sessions = slices.Clone(running)
		}
	}
}

// sessionIDs are n sessions' ids, of an account's day, each its own.
func sessionIDs(date, account string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		hash := sha256.Sum256(fmt.Appendf(nil, "%s/%s/%d", date, account, i))
		sum := binary.BigEndian.Uint64(hash[:])
		ids[i] = fmt.Sprintf("%08x-%04x-4%03x-a%03x-%012x", uint32(sum>>32), uint16(sum>>16), uint16(sum)&0xfff, uint16(sum>>4)&0xfff, sum&0xffffffffffff)
	}
	slices.Sort(ids)
	return ids
}

// dateOf is day d of the year's date, as the ledger files its days.
func dateOf(d int) string {
	return civil(d).Format(time.DateOnly)
}

// --------------------------------------------------------- versions

// The model versions the worlds' requests ask for.
const (
	opus55   = "claude-opus-5-5"
	opus5    = "claude-opus-5"
	sonnet55 = "claude-sonnet-5-5"
	sonnet5  = "claude-sonnet-5"
	fable51  = "claude-fable-5-1"
	haiku45  = "claude-haiku-4-5"
)

// versionOrder is the order the version shares below give their versions in.
var versionOrder = []string{opus55, opus5, sonnet55, sonnet5, fable51, haiku45}

// weekVersions are each account's worth this week by model version, in
// percent, in versionOrder's order, as Accounts draws them
// (accounts_history.VSHARE); and monthVersions the three's over the last 30
// days (accounts_history.VSHARE30).
var (
	weekVersions = map[string][]float64{
		"work": {55, 7, 28, 3, 5, 2}, "personal": {29, 0, 60, 7, 0, 4}, "side": {82, 0, 16, 0, 1, 1},
		"client": {44, 4, 43, 4, 0, 5}, "spare": {12, 0, 74, 6, 0, 8}, "lab": {64, 6, 25, 0, 3, 2},
		"team": {70, 4, 22, 0, 2, 2}, "extra": {35, 0, 55, 5, 0, 5}, "ops": {20, 0, 70, 6, 0, 4},
		"studio": {50, 5, 41, 0, 1, 3},
	}
	monthVersions = map[string][]float64{"work": {52, 8, 30, 3, 5, 2}, "personal": {31, 0, 57, 7, 1, 4}, "side": {79, 0, 19, 0, 1, 1}}
)

// opus55From is the day of the year Opus 5.5 was first used: Monday 22 June
// 2026.
const opus55From = 266

// thirtyDays is the first day of the year of the last 30 days to today.
const thirtyDays = today - 29

// version sets each account's day's worth by model version: this week's as
// Accounts draws it, of every account, and every day of the seven of the
// frames of six and ten so; of the three, this week's days before today as
// leaves the week, today's lines and all, as Accounts draws it, the rest of
// the last 30 days as leaves them their 30 days' as Accounts draws it, and
// the days before as the families drift from Sonnet to Opus over the months,
// Opus 5.5 from 22 June taking a share of each day's worth from 45% to 58%.
func (y year) version() {
	for _, a := range tenAccounts {
		if !slices.Contains(three, a.ID) {
			y.spread(a.ID, firstDay(a.ID), today, weekVersions[a.ID])
		}
	}
	for _, id := range three {
		for d := range thirtyDays {
			y.days[id][d].versions = drifted(d)
		}
		y.spread(id, thirtyDays, thisWeek, y.leaving(id))
		y.spread(id, thisWeek, today, weekLeaving(id))
	}
}

// weekLeaving are the versions, in versionOrder's order, in dollars, the
// account with the given id's days of this week before today must be worth
// by for its week to be worth by weekVersions's: what the week's worth less
// today's lines' leaves of each, none of a version less than none.
func weekLeaving(id string) []float64 {
	u := weekUses[id]
	shares := make([]float64, len(versionOrder))
	for i, v := range versionOrder {
		shares[i] = max(weekVersions[id][i]/100*u.worth-u.today.worths[v], 0)
	}
	return shares
}

// spread gives the account with the given id's days from one day of the year
// to another the versions given, in versionOrder's order, in percent.
func (y year) spread(id string, from, to int, shares []float64) {
	for d := from; d < to; d++ {
		y.days[id][d].versions = versionsOf(shares)
	}
}

// leaving are the versions, in versionOrder's order, the account with the
// given id's days of the last 30 days before this week must be worth by for
// its 30 days to be worth by monthVersions's, this week's, today's lines and
// all, being worth by weekVersions's: what the 30 days' worth less this
// week's leaves of each, none of a version less than none.
func (y year) leaving(id string) []float64 {
	todays := weekUses[id].today.worth()
	month, week := todays, todays
	for d := thirtyDays; d < today; d++ {
		month += y.days[id][d].worth
		if d >= thisWeek {
			week += y.days[id][d].worth
		}
	}
	shares := make([]float64, len(versionOrder))
	for i := range shares {
		shares[i] = max(monthVersions[id][i]*month-weekVersions[id][i]*week, 0)
	}
	return shares
}

// versionsOf are the shares given, in versionOrder's order, by version,
// summing to 1, but those of none.
func versionsOf(shares []float64) map[string]float64 {
	sum := 0.0
	for _, s := range shares {
		sum += s
	}
	held := make(map[string]float64)
	for i, s := range shares {
		if s > 0 {
			held[versionOrder[i]] = s / sum
		}
	}
	return held
}

// drifted are the versions day d of the year's worth goes by, by its month's
// families as BY MONTH's bars split them (acct11.year): Opus from 28% to 58%,
// Sonnet from 66% to 38%, Fable 1% from March, Haiku from 5% to 2%; Opus 5
// and Sonnet 5 before 22 June, and from then Opus 5.5, its share rising from
// 45% to 58% of the day's worth, the rest of Opus's Opus 5's, and Sonnet 5.5
// nine in ten of Sonnet's.
func drifted(d int) map[string]float64 {
	day := civil(d)
	f := float64(max((day.Year()-2025)*12+int(day.Month()-time.October), 0)) / 11
	opus, sonnet, fable, haiku := math.Round(28+30*f), math.Round(66-28*f), 0.0, 5-math.Round(3*f)
	if f > .4 {
		fable = 1
	}
	sum := opus + sonnet + fable + haiku
	shares := []float64{0, opus / sum, 0, sonnet / sum, fable / sum, haiku / sum}
	if d >= opus55From {
		latest := min(.45+.13*float64(d-opus55From)/float64(today-opus55From), shares[1])
		shares[0], shares[1] = latest, shares[1]-latest
		shares[2], shares[3] = shares[3]*0.9, shares[3]*0.1
	}
	return versionsOf(shares)
}

// ids are the keys of a map of versions, in versionOrder's order.
func ids(versions map[string]float64) []string {
	return slices.SortedFunc(maps.Keys(versions), func(a, b string) int {
		return cmp.Compare(slices.Index(versionOrder, a), slices.Index(versionOrder, b))
	})
}
