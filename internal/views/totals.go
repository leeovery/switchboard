package views

import (
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
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
// its last reset, by the window's key; what its requests were worth, and by
// model version, each version's share of that worth; the day of its most
// requests, and the most days in a row it had any. A count some of its days
// never counted, as one of a summary of version 1 never counted the minutes,
// leaves those days out, and Partial names it: of none that did, it's left
// out, as not known.
//
// Of every account's, All, they count every account's days, those of the
// requests no account answered among them, a session once however many
// accounts it ran on, and give no share left at a reset.
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
	Worth
	ByModel    []ModelShare `json:"by_model,omitempty"`
	BusiestDay string       `json:"busiest_day,omitempty"`
	LongestRun int          `json:"longest_run"`
	Partial    []string     `json:"partial,omitempty"`
}

// ModelShare is a model version's part of an account's use, by the shortest id
// the version table names it by, or its own where the table doesn't: its
// share of use, weighed at API prices, its worth over the account's, so cache
// reads, which run to millions and cost little, count for little, left out
// where its worth isn't priced; and its worth.
type ModelShare struct {
	Model string   `json:"model"`
	Share *float64 `json:"share,omitempty"`
	Worth
}

// accountTotalsOf are the totals over days of the accounts given, and of every
// account.
func accountTotalsOf(days []Day, accounts config.Accounts, v versions) Totals {
	index := make(map[string]int, len(accounts))
	tallies := make([]*totalling, len(accounts)+1)
	for i, a := range accounts {
		index[a.ID], tallies[i] = i, newTotalling(AccountTotals{Account: a.ID})
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
// of its sessions, and how many days counted beyond those they named; each
// window's last reset; its minutes at its cap and at a limit, and whether
// any day read them and any never did; its worth by model version; and how
// many days in a row it's had requests, and its most in a day.
type totalling struct {
	AccountTotals
	ids          map[string]bool
	unnamed      int
	resets       map[string]ledger.Reset
	atCap        int
	atLimit      int
	read, unread bool
	models       modelTally
	run, most    int
}

// newTotalling begins totals counted from nothing.
func newTotalling(totals AccountTotals) *totalling {
	return &totalling{AccountTotals: totals, ids: make(map[string]bool), resets: make(map[string]ledger.Reset), models: modelTally{}}
}

// add counts an account's day, returning its requests. The sessions a day
// counts beyond those it names, as one of a summary of version 1 names none,
// are counted as distinct, as the nearest there is. The requests no account
// answered have no windows, so no minutes to count; and every account's
// totals give no share left at a reset, so count no resets.
func (t *totalling) add(a ledger.PricedAccount, v versions) int {
	requests := 0
	for _, m := range a.Models {
		requests += m.Requests()
		t.addModel(m)
		t.models.add(v, m, m.Tokens(), "")
	}
	t.Requests += requests
	t.unnamed += max(a.Sessions-len(a.SessionIDs), 0)
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
		t.lastResets(a.Resets)
	}
	if a.Account != "" {
		t.minutes(a.AccountDay)
	}
	return requests
}

// lastResets keeps each window's last reset among those given, and those
// counted before.
func (t *totalling) lastResets(resets []ledger.Reset) {
	for _, r := range resets {
		if last, ok := t.resets[r.Window]; !ok || !r.At.Before(last.At) {
			t.resets[r.Window] = r
		}
	}
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
	totals.LeftAtReset, totals.ByModel = t.leftAtReset(), t.byModel()
	return totals
}

// leftAtReset is each window's share left at its last reset, by its key: none
// where none reset.
func (t *totalling) leftAtReset() map[string]float64 {
	if len(t.resets) == 0 {
		return nil
	}
	left := make(map[string]float64, len(t.resets))
	for key, r := range t.resets {
		left[key] = millionthsOf(r.Before).left().share()
	}
	return left
}

// byModel is each model version's worth, and its share of the account's.
func (t *totalling) byModel() []ModelShare {
	listed := t.models.list()
	shares := make([]ModelShare, len(listed))
	worth := t.amount()
	for i, m := range listed {
		shares[i] = ModelShare{Model: m.Model, Worth: m.Worth}
		if m.Amount != nil && worth > 0 {
			share := float64(*m.Amount) / float64(worth)
			shares[i].Share = &share
		}
	}
	return shares
}
