package views

import (
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// pointsOf returns the points of the windows of the accounts requests, the
// session's with the given id, oldest first, went to that the session took,
// by account, then by window key, in whole points, an estimate: a window
// counts its account's whole use, every session's, so each rise in it is
// shared out by tokens, as sharesOf says, among the requests of the account
// on the session's days, read through l. The rises between two days apart,
// with a day between them the session made no request on, are none of its:
// the requests of that day go unread. A window the account's lines never
// read has no points.
func pointsOf(l Ledger, id string, requests []request) map[string]map[string]int {
	accounts := make(map[string]bool)
	for _, r := range requests {
		if r.Account != "" {
			accounts[r.Account] = true
		}
	}
	taken := make(map[string]map[string]float64)
	for _, run := range runsOf(daysOf(requests)) {
		share(taken, answeredOn(l, run, accounts, id))
	}
	points := make(map[string]map[string]int, len(taken))
	for account, shares := range taken {
		points[account] = make(map[string]int, len(shares))
		for key, share := range shares {
			points[account][key] = int(math.Round(share * 100))
		}
	}
	return points
}

// daysOf returns the dates of the local days requests came on, oldest first.
func daysOf(requests []request) []string {
	dates := make([]string, len(requests))
	for i, r := range requests {
		dates[i] = dayOf(r.At).date
	}
	slices.Sort(dates)
	return slices.Compact(dates)
}

// runsOf returns dates, oldest first, in runs of days one after another.
func runsOf(dates []string) [][]string {
	var runs [][]string
	for i, date := range dates {
		if i == 0 || date != nextDate(dates[i-1]) {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], date)
	}
	return runs
}

// nextDate returns the date of the local day after the one with the given
// date.
func nextDate(date string) string {
	_, end, _ := dayfile.Day(date)
	return dayOf(end).date
}

// answeredOn returns the requests of the accounts given that l files under
// the days with the given dates, every session's, by account, those of the
// session with the given id as ours.
func answeredOn(l Ledger, dates []string, accounts map[string]bool, id string) map[string][]answered {
	answers := make(map[string][]answered)
	for _, date := range dates {
		for line := range l.DayLines(date) {
			if line.Kind == ledger.KindMessage && accounts[line.Account] {
				answers[line.Account] = append(answers[line.Account], answeredOf(&line, id))
			}
		}
	}
	return answers
}

// share adds to taken, by account, then by window key, the session's share
// of each account's windows' rises, as sharesOf gives it, among the
// account's requests answers holds, over a run of days one after another.
func share(taken map[string]map[string]float64, answers map[string][]answered) {
	for account, requests := range answers {
		for key, share := range sharesOf(requests) {
			if taken[account] == nil {
				taken[account] = make(map[string]float64)
			}
			taken[account][key] += share
		}
	}
}

// answered is a request of an account, as the rises in its windows' use are
// shared out: when it ended, its tokens, every kind counted alike, its
// model, whether it's of the session whose share is taken, and the windows
// its answer's limits read.
type answered struct {
	end     time.Time
	tokens  int64
	model   string
	ours    bool
	windows []quota.Window
}

// answeredOf returns the request the line l holds, ours where it's of the
// session with the given id.
func answeredOf(l *ledger.Line, id string) answered {
	t, _ := l.Tokens()
	return answered{
		end:     ended(l),
		tokens:  int64(t.Input + t.Output + t.CacheRead + t.CacheWrite),
		model:   l.Model,
		ours:    l.Session == id,
		windows: claude.LimitWindows(l.Limits),
	}
}

// sharesOf returns the session's share of each window's rises, by its key,
// as a fraction of the window, among requests, an account's every one over a
// run of days: each rise in a window's use from one answer to the next, in
// the order they ended, is shared out among the requests that ended between
// the two, the later included, by their tokens, a later reset than the one
// before starting the window afresh, from nothing. A window every model
// shares counts every request; a model's own, as Fable's week, only those of
// the models whose answers read it. A share is given of each window the
// session's requests count toward.
func sharesOf(requests []answered) map[string]float64 {
	slices.SortStableFunc(requests, func(a, b answered) int { return a.end.Compare(b.end) })
	windows := windowsRead(requests)
	for _, r := range requests {
		for _, w := range windows {
			if w.counts(r.model) {
				w.count(r)
			}
		}
		for _, reading := range r.windows {
			windows[reading.Key].take(reading)
		}
	}
	shares := make(map[string]float64)
	for key, w := range windows {
		if w.counted {
			shares[key] = w.taken
		}
	}
	return shares
}

// windowsRead returns the windows requests' answers read, by their keys,
// each with the models whose answers read it, and whether every model shares
// it, as claude.SharedWindows says.
func windowsRead(requests []answered) map[string]*windowShare {
	windows := make(map[string]*windowShare)
	for _, r := range requests {
		for _, reading := range r.windows {
			w, ok := windows[reading.Key]
			if !ok {
				w = &windowShare{shared: slices.Contains(claude.SharedWindows, reading.Key), models: make(map[string]bool)}
				windows[reading.Key] = w
			}
			w.models[r.model] = true
		}
	}
	return windows
}

// windowShare is a window of an account as its rises are shared out: whether
// every model shares it, else the models whose answers read it; the reading
// of it last taken, once one is; the tokens of the requests that ended
// since, every one's and the session's; whether any of the session's has
// counted toward it; and the session's share of its rises so far, as a
// fraction of the window.
type windowShare struct {
	shared    bool
	models    map[string]bool
	last      quota.Window
	read      bool
	all, ours int64
	counted   bool
	taken     float64
}

// counts reports whether the requests of the model with the given id count
// toward the window.
func (w *windowShare) counts(model string) bool {
	return w.shared || w.models[model]
}

// count counts r, a request that ended since the last reading taken.
func (w *windowShare) count(r answered) {
	w.all += r.tokens
	if r.ours {
		w.ours, w.counted = w.ours+r.tokens, true
	}
}

// take takes reading, the window's next, sharing out its rise from the one
// before among the requests counted since, the session's share of it by
// their tokens.
func (w *windowShare) take(reading quota.Window) {
	rise := reading.Utilization - w.last.Utilization
	if reading.ResetsAt.After(w.last.ResetsAt) {
		rise = reading.Utilization
	}
	if w.read && rise > 0 && w.all > 0 {
		w.taken += rise * float64(w.ours) / float64(w.all)
	}
	w.last, w.read, w.all, w.ours = reading, true, 0, 0
}
