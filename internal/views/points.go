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
// on the days the session made requests on, as byDate holds every account's
// requests by the date of the day whose files hold them. The rises between
// two days apart, with a day between them the session made no request on,
// are none of its: the requests of that day go uncounted. A window the
// account's lines never read has no points.
func pointsOf(byDate map[string][]ledger.Line, id string, requests []request) map[string]map[string]int {
	accounts := make(map[string]bool)
	for _, r := range requests {
		if r.Account != "" {
			accounts[r.Account] = true
		}
	}
	taken := make(map[string]map[string]float64)
	for _, run := range runsOf(daysOf(byDate, id)) {
		byAccount := make(map[string][]answered)
		for _, date := range run {
			for i := range byDate[date] {
				if l := &byDate[date][i]; accounts[l.Account] {
					byAccount[l.Account] = append(byAccount[l.Account], answeredOf(l, id))
				}
			}
		}
		share(taken, byAccount)
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

// daysOf returns the dates of the days byDate holds a request of the session
// with the given id on, oldest first.
func daysOf(byDate map[string][]ledger.Line, id string) []string {
	var dates []string
	for date, lines := range byDate {
		if slices.ContainsFunc(lines, func(l ledger.Line) bool { return l.Session == id }) {
			dates = append(dates, date)
		}
	}
	slices.Sort(dates)
	return dates
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
// shared out: when its answer's headers came, reading its windows, and when
// it ended; its tokens, every kind counted alike; its model; whether it's of
// the session whose share is taken; and the windows its answer's limits
// read.
type answered struct {
	read, end time.Time
	tokens    int64
	model     string
	ours      bool
	windows   []quota.Window
}

// answeredOf returns the request the line l holds, ours where it's of the
// session with the given id: its windows read as its answer's first byte
// passed on, where one did, else as it ended.
func answeredOf(l *ledger.Line, id string) answered {
	t, _ := l.Tokens()
	a := answered{
		end:     ended(l),
		tokens:  int64(t.Input + t.Output + t.CacheRead + t.CacheWrite),
		model:   l.Model,
		ours:    l.Session == id,
		windows: claude.LimitWindows(l.Limits),
	}
	a.read = a.end
	if l.FirstMS != nil {
		a.read = l.At.Add(time.Duration(*l.FirstMS) * time.Millisecond)
	}
	return a
}

// sharesOf returns the session's share of each window's rises, by its key,
// as a fraction of the window, among requests, an account's every one over a
// run of days. Its readings are taken in the order they were read, each
// climbing the window as ledger.Climb says, as a day's summary counts its
// rise; and each climb is shared out, by their tokens, among the requests
// that ended between the reading before it and it, each in the climb of the
// first reading after it ended: a request's own reading comes with its
// answer's first byte, before its output, which a later one counts. A window
// every model shares counts every request; a model's own, as Fable's week,
// only those of the models whose answers read it. A share is given of each
// window the session's requests count toward.
func sharesOf(requests []answered) map[string]float64 {
	ending := slices.SortedStableFunc(slices.Values(requests), func(a, b answered) int { return a.end.Compare(b.end) })
	reading := slices.SortedStableFunc(slices.Values(requests), func(a, b answered) int { return a.read.Compare(b.read) })
	windows := windowsRead(requests)
	for _, r := range reading {
		for _, w := range r.windows {
			windows[w.Key].take(ending, w, r.read)
		}
	}
	shares := make(map[string]float64)
	for key, w := range windows {
		if w.used {
			shares[key] = w.share
		}
	}
	return shares
}

// windowsRead returns the windows requests' answers read, by their keys,
// each with the models whose answers read it, whether every model shares it,
// as claude.SharedWindows says, and whether the session's requests count
// toward it.
func windowsRead(requests []answered) map[string]*windowShare {
	windows := make(map[string]*windowShare)
	for _, r := range requests {
		for _, reading := range r.windows {
			w, ok := windows[reading.Key]
			if !ok {
				w = &windowShare{climb: ledger.NewClimb(reading.Key, nil), shared: slices.Contains(claude.SharedWindows, reading.Key),
					models: make(map[string]bool)}
				windows[reading.Key] = w
			}
			w.models[r.model] = true
		}
	}
	for _, w := range windows {
		w.used = slices.ContainsFunc(requests, func(r answered) bool { return r.ours && w.countsToward(r.model) })
	}
	return windows
}

// windowShare is a window of an account as its rises are shared out: how its
// use climbs; whether every model shares it, else the models whose answers
// read it; how many of the requests, in the order they ended, its readings
// have reached; whether any of the session's requests count toward it; and
// the session's share of its rises so far, as a fraction of the window.
type windowShare struct {
	climb  *ledger.Climb
	shared bool
	models map[string]bool
	next   int
	used   bool
	share  float64
}

// countsToward reports whether the requests of the model with the given id
// count toward the window.
func (w *windowShare) countsToward(model string) bool {
	return w.shared || w.models[model]
}

// take takes reading, the window's, read at at, ending listing the requests
// in the order they ended: its climb is shared out among those that ended
// since the reading before it, by their tokens. A reading that says nothing
// of the window, as ledger.Climb says, counts none of them, leaving them to
// the next.
func (w *windowShare) take(ending []answered, reading quota.Window, at time.Time) {
	climbed, ok := w.climb.Take(reading, at)
	if !ok {
		return
	}
	var all, ours int64
	for ; w.next < len(ending) && !ending[w.next].end.After(at); w.next++ {
		if r := ending[w.next]; w.countsToward(r.model) {
			all += r.tokens
			if r.ours {
				ours += r.tokens
			}
		}
	}
	if climbed > 0 && all > 0 {
		w.share += climbed * float64(ours) / float64(all)
	}
}
