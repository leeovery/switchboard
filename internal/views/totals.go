package views

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// Totals are what Accounts and History count over the days: each configured
// account's, in the config's order, then every account's.
type Totals struct {
	Accounts []AccountTotals `json:"accounts"`
}

// AccountTotals are an account's totals over the days, as Accounts counts
// them: its requests; its sessions, each once, however many days it ran on;
// the sessions moved onto it and off it; the limits it reached, by window;
// the minutes it spent at its cap and at a limit; each window's share left at
// its last reset, and the date of the day it last reached its limit in the
// run that reset ended, where it did, each by the window's key; what its
// requests were worth, and by model version, each version's share of that
// worth; the day of its most requests, and the most days in a row it had
// any. A count some of its days never counted, as one of a summary of
// version 1 never counted the minutes, leaves those days out, and Partial
// names it: of none that did, it's left out, as not known.
//
// Of every account's, All, they count every account's days, those of the
// requests no account answered among them, a session once however many
// accounts it ran on, and give no share left at a reset, nor its run out.
type AccountTotals struct {
	Account        string             `json:"account,omitempty"`
	All            bool               `json:"all,omitempty"`
	Requests       int                `json:"requests"`
	Sessions       int                `json:"sessions"`
	MovedOn        int                `json:"moved_on"`
	MovedOff       int                `json:"moved_off"`
	Limits         map[string]int     `json:"limits,omitempty"`
	MinutesAtCap   *int               `json:"minutes_at_cap,omitempty"`
	MinutesAtLimit *int               `json:"minutes_at_limit,omitempty"`
	LeftAtReset    map[string]float64 `json:"left_at_reset,omitempty"`
	RanOut         map[string]string  `json:"ran_out,omitempty"`
	Worth
	ByModel    []ModelShare `json:"by_model,omitempty"`
	BusiestDay string       `json:"busiest_day,omitempty"`
	LongestRun int          `json:"longest_run"`
	Partial    []string     `json:"partial,omitempty"`
}

// LimitedWindows are the windows any row of the page records a limit of,
// every account's among them, by their keys, each once, sorted.
func (p AccountsPage) LimitedWindows() []string {
	var keys []string
	for _, row := range append(slices.Clone(p.Rows), p.All) {
		keys = append(keys, slices.Collect(maps.Keys(row.Totals.Limits))...)
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

// LimitCount is how many limits the account reached, every window's
// together, as every account's are counted.
func (t AccountTotals) LimitCount() int {
	return limitCount(t.Limits)
}

// LimitsHit says the limits the account reached, by window, each named as
// name names its key, as LimitsHit says them.
func (t AccountTotals) LimitsHit(name func(key string) string) []string {
	return LimitsHit(t.Limits, name)
}

// AtCapOrLimit is how many minutes the account spent at its cap or at a
// limit, together, reporting false where its days never read one of them.
func (t AccountTotals) AtCapOrLimit() (int, bool) {
	if t.MinutesAtCap == nil || t.MinutesAtLimit == nil {
		return 0, false
	}
	return *t.MinutesAtCap + *t.MinutesAtLimit, true
}

// limitCount is how many limits were reached, by window, as AccountTotals'
// Limits holds them, every window's together.
func limitCount(limits map[string]int) int {
	n := 0
	for _, hit := range limits {
		n += hit
	}
	return n
}

// LimitsHit says the limits reached, by window, as AccountTotals' Limits
// holds them: each window, in the order the owner reads windows listed, as
// status.CompareWindows has it by Claude's policy, by its name as name gives
// it, as a sentence has it, and how many times, its thousands set apart, as
// "week ×3" and "5-hour ×402"; none where none was.
func LimitsHit(limits map[string]int, name func(key string) string) []string {
	keys := slices.SortedFunc(maps.Keys(limits), status.CompareWindows(claude.Policy()))
	hit := make([]string, len(keys))
	for i, key := range keys {
		hit[i] = status.InProse(name(key)) + " ×" + prose.Grouped(limits[key])
	}
	return hit
}

// ModelShare is a model version's part of an account's use, by the shortest id
// the version table names it by, or its own where the table doesn't: its
// share of use, weighed at API prices, its worth over the account's, so cache
// reads, which run to millions and cost little, count for little, left out
// where none of its worth is priced; and its worth.
type ModelShare struct {
	Model string   `json:"model"`
	Share *float64 `json:"share,omitempty"`
	Worth
}

// ShareSaid says a version's share of a row's use, as BY MODEL and history
// say it, a whole percentage, "55%", or "<1%" where it rounds to none,
// reporting false where it has none, as one none of whose use is priced.
func ShareSaid(share *float64) (string, bool) {
	switch {
	case share == nil:
		return "", false
	case prose.WholePercent(*share) == 0:
		return "<1%", true
	}
	return prose.Percent(*share), true
}

// accountTotalsOf are the totals over days of the accounts given, and of every
// account, the days before them, oldest first, telling no more than the runs
// of their windows that ended among them.
func accountTotalsOf(before, days []Day, accounts config.Accounts, v versions) Totals {
	index := make(map[string]int, len(accounts))
	tallies := make([]*totalling, len(accounts)+1)
	for i, a := range accounts {
		index[a.ID], tallies[i] = i, newTotalling(AccountTotals{Account: a.ID})
		tallies[i].before = accountDays(before, a.ID)
	}
	all := len(accounts)
	tallies[all] = newTotalling(AccountTotals{All: true})
	for _, d := range days {
		requests := make([]int, len(tallies))
		for _, a := range d.Accounts {
			n := tallies[all].add(a, v)
			requests[all] += n
			if i, ok := index[a.Account]; ok {
				tallies[i].add(a, v)
				requests[i] += n
			}
		}
		for i, t := range tallies {
			t.day(d.Day, requests[i])
		}
	}
	totals := Totals{Accounts: make([]AccountTotals, len(tallies))}
	for i, t := range tallies {
		totals.Accounts[i] = t.totals()
	}
	return totals
}

// totalling is an account's totals as they're counted, day by day: the ids
// of its sessions, and how many days counted beyond those they named; its
// days, and those before them, which the runs of its windows that ended
// among its days are told from; its minutes at its cap and at a limit, and
// whether any day read them and any never did; its worth by model version;
// and how many days in a row it's had requests, and its most in a day.
type totalling struct {
	AccountTotals
	ids          map[string]bool
	unnamed      int
	before, days []ledger.AccountDay
	atCap        int
	atLimit      int
	read, unread bool
	models       modelTally
	run, most    int
}

// newTotalling begins totals counted from nothing.
func newTotalling(totals AccountTotals) *totalling {
	return &totalling{AccountTotals: totals, ids: make(map[string]bool), models: modelTally{}}
}

// add counts an account's day, returning its requests. The sessions a day
// counts beyond those it names, as AccountDay.Unnamed says, as one of a
// summary of version 1 names none, are counted as distinct, as the nearest
// there is. The requests no account answered have no windows, so no minutes
// to count; and every account's totals give no share left at a reset, so
// keep no days.
func (t *totalling) add(a ledger.PricedAccount, v versions) int {
	requests := 0
	for _, m := range a.Models {
		requests += m.Requests()
		t.addModel(m)
		t.models.add(v, m, m.Tokens(), "")
	}
	t.Requests += requests
	t.unnamed += a.Unnamed()
	for _, id := range a.SessionIDs {
		t.ids[id] = true
	}
	t.MovedOn += a.MovedOn
	t.MovedOff += a.MovedOff
	for _, l := range a.Limits {
		if t.Limits == nil {
			t.Limits = make(map[string]int)
		}
		t.Limits[l.Window]++
	}
	if !t.All {
		t.days = append(t.days, a.AccountDay)
	}
	if a.Account != "" {
		t.minutes(a.AccountDay)
	}
	return requests
}

// minutes counts the day's minutes at the account's cap and at a limit,
// where it read them.
func (t *totalling) minutes(a ledger.AccountDay) {
	if a.MinutesAtCap == nil || a.MinutesAtLimit == nil {
		t.unread = true
		return
	}
	t.atCap += *a.MinutesAtCap
	t.atLimit += *a.MinutesAtLimit
	t.read = true
}

// day counts the day with the given date's requests toward the busiest day
// and the longest run, the days coming one after another.
func (t *totalling) day(date string, requests int) {
	if requests > t.most {
		t.most, t.BusiestDay = requests, date
	}
	if requests == 0 {
		t.run = 0
		return
	}
	t.run++
	t.LongestRun = max(t.LongestRun, t.run)
}

// totals are the totals as counted.
func (t *totalling) totals() AccountTotals {
	totals := t.AccountTotals
	totals.Sessions = len(t.ids) + t.unnamed
	if t.read {
		totals.MinutesAtCap, totals.MinutesAtLimit = &t.atCap, &t.atLimit
		if t.unread {
			totals.Partial = []string{"minutes_at_cap", "minutes_at_limit"}
		}
	}
	totals.LeftAtReset, totals.RanOut = t.endedRuns()
	totals.ByModel = t.byModel()
	return totals
}

// endedRuns are, of each window whose run ended among the days, as lastEnded
// tells its last, from the days before them too, as a run may begin there,
// the share it had left as it reset, and the date it last reached its limit
// in that run, where it did, each by the window's key: none where none ended.
func (t *totalling) endedRuns() (left map[string]float64, ranOut map[string]string) {
	for _, key := range resetWindows(t.days) {
		run, _ := lastEnded(slices.Concat(t.before, t.days), key)
		if left == nil {
			left = make(map[string]float64)
		}
		left[key] = run.left
		if run.ranOut != "" {
			if ranOut == nil {
				ranOut = make(map[string]string)
			}
			ranOut[key] = run.ranOut
		}
	}
	return left, ranOut
}

// resetWindows are the keys of the windows that reset among an account's
// days, each once.
func resetWindows(days []ledger.AccountDay) []string {
	var keys []string
	for _, d := range days {
		for _, r := range d.Resets {
			if !slices.Contains(keys, r.Window) {
				keys = append(keys, r.Window)
			}
		}
	}
	return keys
}

// byModel is each model version's worth, and its share of the account's,
// none where none of its use is priced.
func (t *totalling) byModel() []ModelShare {
	listed := t.models.list()
	shares := make([]ModelShare, len(listed))
	worth := t.amount()
	for i, m := range listed {
		shares[i] = ModelShare{Model: m.Model, Worth: m.Worth}
		if m.Amount != nil && *m.Amount > 0 && worth > 0 {
			share := float64(*m.Amount) / float64(worth)
			shares[i].Share = &share
		}
	}
	return shares
}
