package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// Week is a calendar week of Weeks, by its first day, from the day weeks
// start on: each configured account's own weeks placed under it, in the
// config's order, then every account's.
type Week struct {
	Week     string        `json:"week"`
	Accounts []AccountWeek `json:"accounts"`
}

// AccountWeek is what an account's own weeks placed under a calendar week
// give: Peaks, the highest use of each window a week long, by the window's
// key, of its week placed there, the week still running's so far; and
// Limits, the limits it reached in its week placed there, by window. Of
// every account's, All, Peaks are the mean of those of the accounts that
// have one, and there are no Limits.
type AccountWeek struct {
	Account string             `json:"account,omitempty"`
	All     bool               `json:"all,omitempty"`
	Peaks   map[string]float64 `json:"peaks,omitempty"`
	Limits  map[string]int     `json:"limits,omitempty"`
}

// halfAWeek is how far into a calendar week a week can begin and still be
// placed under it, rather than under the next.
const halfAWeek = 7 * 24 * time.Hour / 2

// weekly tells the accounts' own weeks, and places them as Weeks does: key is
// that of the window a week long every model shares, the account's week, and
// length how long it lasts; starts is the day calendar weeks start on; and
// now is the time now, which a week still running runs to.
type weekly struct {
	key    string
	length time.Duration
	starts time.Weekday
	now    time.Time
}

// weeklyOf is weekly with the account's week the window with the given key,
// as long as its key says.
func weeklyOf(key string, starts time.Weekday, now time.Time) weekly {
	length, _ := quota.Length(key)
	return weekly{key: key, length: length, starts: starts, now: now}
}

// isWeek reports whether the window with the given key is a week long, as
// the account's week is, and a model's own week, as Fable's, is.
func (w weekly) isWeek(key string) bool {
	length, ok := quota.Length(key)
	return ok && length == w.length
}

// ownWeek is a week of an account's window, from one reset of it to the next:
// peak is its use read just before the reset that ended it, at end, or, of the
// week still running, open, its highest use so far, which known reports is
// known.
type ownWeek struct {
	start, end time.Time
	open       bool
	peak       millionths
	known      bool
}

// holds reports whether t falls in the week.
func (o ownWeek) holds(t time.Time) bool {
	return !t.Before(o.start) && (o.open || t.Before(o.end))
}

// ownWeeks are an account's own weeks, in order, by the key of the window a
// week long each is of.
type ownWeeks map[string][]ownWeek

// holding returns the account's own week a limit of the window with the given
// key, reached at at, was reached in: one of the window's own, where it has
// weeks, else one of the account's week's, of the window with the key week;
// reporting false where none holds it.
func (o ownWeeks) holding(key, week string, at time.Time) (ownWeek, bool) {
	weeks := o[key]
	if len(weeks) == 0 {
		weeks = o[week]
	}
	for _, own := range weeks {
		if own.holds(at) {
			return own, true
		}
	}
	return ownWeek{}, false
}

// ownWeeksOf are an account's own weeks over its days, each a day of the days
// asked for, the zero day where it has none: of each window a week long, a
// week from one of its resets the days give to the next, and one still
// running from the last, while that's less than a week ago. A reset the days
// don't give, as one on a day of no requests, isn't taken for one that never
// came: a week is no longer than its window, so a week's start is never
// further back than that from its end.
func (w weekly) ownWeeksOf(days []ledger.AccountDay) ownWeeks {
	weeks := ownWeeks{}
	last, lastDay := make(map[string]time.Time), make(map[string]int)
	for i, d := range days {
		for _, r := range d.Resets {
			if !w.isWeek(r.Window) || !r.At.After(last[r.Window]) {
				continue
			}
			weeks[r.Window] = append(weeks[r.Window], ownWeek{start: w.began(last[r.Window], r.At), end: r.At, peak: millionthsOf(r.Before), known: true})
			last[r.Window], lastDay[r.Window] = r.At, i
		}
	}
	for key, at := range last {
		if w.now.Before(at.Add(w.length)) {
			peak, known := soFar(days, key, lastDay[key])
			weeks[key] = append(weeks[key], ownWeek{start: at, open: true, peak: peak, known: known})
		}
	}
	return weeks
}

// began is when a week that ended at end began: at the reset before it, at
// last, where that was within a week of end; else, where there was none, or
// one came between that the days don't give, a week before end, as a
// window's span gives it.
func (w weekly) began(last, end time.Time) time.Time {
	if start := end.Add(-w.length); last.Before(start) {
		return start
	}
	return last
}

// soFar is the highest use of the window with the given key in the week that
// began on the day of days with the index given, at the window's last reset:
// the highest the days after read, else what that day's use rose to after
// the reset, as afterReset says, reporting false where neither is known.
func soFar(days []ledger.AccountDay, key string, began int) (millionths, bool) {
	var peak millionths
	known := false
	for _, d := range days[began+1:] {
		if highest, ok := d.Highest[key]; ok {
			peak, known = max(peak, millionthsOf(highest)), true
		}
	}
	if known {
		return peak, true
	}
	return afterReset(days, key, began)
}

// afterReset is the use of the window with the given key since its last reset
// on the day of days with the index given, which that day's highest, the week
// that ended's, hides. A week's use only rises, so the day's rise is the rise
// from the use it began the day at to its first reset's before, from nothing
// to each other's, and from nothing on since the last: the use since is the
// rise less the befores, more the use it began the day at, the day before's
// highest, where that day had no reset of the window. It reports false where
// the day's rise, or the use it began at, isn't known.
func afterReset(days []ledger.AccountDay, key string, i int) (millionths, bool) {
	if i == 0 {
		return 0, false
	}
	day, before := days[i], days[i-1]
	rise, risen := day.Rise[key]
	began, read := before.Highest[key]
	if !risen || !read || resetOn(before, key) {
		return 0, false
	}
	use := millionthsOf(rise) + millionthsOf(began)
	for _, r := range day.Resets {
		if r.Window == key {
			use -= millionthsOf(r.Before)
		}
	}
	return max(use, 0), true
}

// resetOn reports whether the window with the given key reset on the day.
func resetOn(day ledger.AccountDay, key string) bool {
	for _, r := range day.Resets {
		if r.Window == key {
			return true
		}
	}
	return false
}

// placedUnder is the first day of the calendar week a week that began at
// start is placed under, as Weeks places its columns: the one whose first
// day, by the clock, it began nearest.
func (w weekly) placedUnder(start time.Time) string {
	day := dateOf(start)
	week := weekOf(day, w.starts)
	hour, minute, second := start.Local().Clock()
	into := day.Sub(week) + time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute + time.Duration(second)*time.Second
	if into >= halfAWeek {
		week = week.AddDate(0, 0, 7)
	}
	return week.Format(time.DateOnly)
}

// placedWeek is a calendar week, by its first day, with what the configured
// accounts' own weeks placed under it give, each account's in the config's
// order: peaks, by window, and limits, by window.
type placedWeek struct {
	week   string
	peaks  []map[string]millionths
	limits []map[string]int
}

// peak takes use as the peak of the window with the given key of the account
// with the given index, where it's the highest yet: two of its weeks are
// placed under one only where one was cut short by hand.
func (p *placedWeek) peak(account int, key string, use millionths) {
	if p.peaks[account] == nil {
		p.peaks[account] = make(map[string]millionths)
	}
	if held, ok := p.peaks[account][key]; !ok || use > held {
		p.peaks[account][key] = use
	}
}

// limit counts a limit the account with the given index reached, of the
// window with the given key.
func (p *placedWeek) limit(account int, key string) {
	if p.limits[account] == nil {
		p.limits[account] = make(map[string]int)
	}
	p.limits[account][key]++
}

// calendar is the calendar weeks days span, by their first days' dates: a
// week placed outside them is left out.
type calendar map[string]*placedWeek

// placedWeeks are the calendar weeks days span, with the accounts' own weeks
// placed under them.
func (w weekly) placedWeeks(days []Day, accounts config.Accounts) []placedWeek {
	spans := spansOf(days, weekKey(w.starts))
	weeks := make([]placedWeek, len(spans))
	at := make(calendar, len(spans))
	for i, s := range spans {
		weeks[i] = placedWeek{week: s.key, peaks: make([]map[string]millionths, len(accounts)), limits: make([]map[string]int, len(accounts))}
		at[s.key] = &weeks[i]
	}
	for i, a := range accounts {
		own := accountDays(days, a.ID)
		weeksOf := w.ownWeeksOf(own)
		w.placePeaks(at, i, weeksOf)
		w.placeLimits(at, i, weeksOf, own, days)
	}
	return weeks
}

// placePeaks places the peaks of the own weeks given, those known, of the
// account with the given index.
func (w weekly) placePeaks(at calendar, account int, weeks ownWeeks) {
	for key, list := range weeks {
		for _, o := range list {
			if p, ok := at[w.placedUnder(o.start)]; ok && o.known {
				p.peak(account, key, o.peak)
			}
		}
	}
}

// placeLimits places the limits of the account with the given index, its
// own weeks and its days, own, a day each of days, given: each in the week of
// the account's it was reached in, or, where the days give none, in the
// calendar week of the day it's filed under.
func (w weekly) placeLimits(at calendar, account int, weeks ownWeeks, own []ledger.AccountDay, days []Day) {
	filed := weekKey(w.starts)
	for i, d := range own {
		for _, l := range d.Limits {
			week, _ := filed(days[i])
			if o, ok := weeks.holding(l.Window, w.key, l.At); ok {
				week = w.placedUnder(o.start)
			}
			if p, ok := at[week]; ok {
				p.limit(account, l.Window)
			}
		}
	}
}

// accountDays are the days of the account with the given id, a day each of
// days, the zero day where it has none.
func accountDays(days []Day, id string) []ledger.AccountDay {
	own := make([]ledger.AccountDay, len(days))
	for i, d := range days {
		for _, a := range d.Accounts {
			if a.Account == id {
				own[i] = a.AccountDay
			}
		}
	}
	return own
}

// weeksOf are Weeks' weeks, as placed gives them, of the accounts given.
func weeksOf(placed []placedWeek, accounts config.Accounts) []Week {
	weeks := make([]Week, len(placed))
	for i, p := range placed {
		weeks[i] = p.of(accounts)
	}
	return weeks
}

// of is the week as Weeks gives it, its accounts those given: every
// account's peak of a window the mean of theirs that have one.
func (p placedWeek) of(accounts config.Accounts) Week {
	week := Week{Week: p.week, Accounts: make([]AccountWeek, 0, len(accounts)+1)}
	sums, counts := make(map[string]millionths), make(map[string]int)
	for i, a := range accounts {
		week.Accounts = append(week.Accounts, AccountWeek{Account: a.ID, Peaks: shares(p.peaks[i]), Limits: p.limits[i]})
		for key, peak := range p.peaks[i] {
			sums[key] += peak
			counts[key]++
		}
	}
	all := AccountWeek{All: true}
	for key, sum := range sums {
		if all.Peaks == nil {
			all.Peaks = make(map[string]float64)
		}
		all.Peaks[key] = meanOf(sum, counts[key])
	}
	week.Accounts = append(week.Accounts, all)
	return week
}

// shares are uses as shares of their windows, by key: none where there are
// none.
func shares(uses map[string]millionths) map[string]float64 {
	if len(uses) == 0 {
		return nil
	}
	given := make(map[string]float64, len(uses))
	for key, use := range uses {
		given[key] = use.share()
	}
	return given
}
