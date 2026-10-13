package views

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// At is the page as it stands at the moment m, of the document of its
// accounts: where m has an account's window a week long reset, since it was
// read or before it was read again, and the page's days don't hold that reset
// yet, as they were read before it, its weeks are worked out at m's Now as
// though the days held it, filed as the ledger files one, so its left at its
// last reset, its peaks and its verdict change on the second WEEK USED reads
// none, and stay so. Once the days hold it, it isn't counted again. The rest
// stands as it was built.
func (p AccountsPage) At(m status.Moment) AccountsPage {
	resets := p.unfiled(m)
	if len(resets) == 0 {
		return p
	}
	in := p.in
	in.Now = m.Now
	p.Rows = slices.Clone(p.Rows)
	p.weeked(in, filedIn(p.days, resets))
	return p
}

// accountReset is a reset of the window of the account with the given id.
type accountReset struct {
	account string
	reset   ledger.Reset
}

// unfiled are the resets of the windows a week long of the page's accounts
// that m tells of and the page's days don't hold, as endedSince has them.
func (p AccountsPage) unfiled(m status.Moment) []accountReset {
	var resets []accountReset
	for _, a := range m.Accounts {
		if !slices.ContainsFunc(p.in.Accounts, func(c config.Account) bool { return c.ID == a.ID }) {
			continue
		}
		own := accountDays(p.days, a.ID)
		for _, w := range a.Windows {
			if reset, ok := endedSince(w, p.days, own); ok {
				resets = append(resets, accountReset{account: a.ID, reset: reset})
			}
		}
	}
	return resets
}

// endedSince is the reset that ended a run of the window w a week long, of
// an account whose days are own, a day each of days, where the days don't
// hold it yet, as the ledger would file it: of a run that has reset since it
// was read, at its reset as read; or, read again since it reset, of the run
// before the one it reads, at that one's start. It closes at the most read
// of the run, the days' as highestBefore has it, or the reading's of a run
// read before it reset. It reports false where the days hold it, of any
// other window, and where the run closes at nothing, as the ledger files no
// reset of a week of none.
func endedSince(w status.WindowNow, days []Day, own []ledger.AccountDay) (ledger.Reset, bool) {
	length, ok := quota.Length(w.Key)
	if !ok || !quota.WeekLong(w.Key) {
		return ledger.Reset{}, false
	}
	var end time.Time
	var read float64
	switch {
	case w.ResetSinceRead && !resetAfter(own, w.Key, w.Read.At):
		end, read = w.Read.ResetsAt, w.Read.Used
	// The days file the reset that began the run read at its start as Claude
	// gives it, to the second or so: the one before came a whole length
	// before, so one filed within half of that is this one.
	case !w.ResetSinceRead && !w.Start.IsZero() && !resetAfter(own, w.Key, w.Start.Add(-length/2)):
		end = w.Start
	default:
		return ledger.Reset{}, false
	}
	before := max(read, highestBefore(days, own, w.Key, end).share())
	if before == 0 {
		return ledger.Reset{}, false
	}
	return ledger.Reset{Window: w.Key, At: end.UTC(), Before: before}, true
}

// resetAfter reports whether an account's days hold a reset of the window with
// the given key after at, as one the router saw once it read it then.
func resetAfter(days []ledger.AccountDay, key string, at time.Time) bool {
	return slices.ContainsFunc(days, func(d ledger.AccountDay) bool {
		return slices.ContainsFunc(d.Resets, func(r ledger.Reset) bool { return r.Window == key && r.At.After(at) })
	})
}

// highestBefore is the highest use read of the window with the given key in
// the run that ended at end, of an account's days, own, a day each of days,
// oldest first: since its last reset among them, as soFar has it, where that
// came on or after the day the run began on, the window's length before end;
// else of the days after that one, as that day's highest may be the run
// before's. None where none is read.
func highestBefore(days []Day, own []ledger.AccountDay, key string, end time.Time) millionths {
	length, _ := quota.Length(key)
	began := dayfile.DateOf(end.Add(-length))
	var peak millionths
	for i := len(own) - 1; i >= 0 && days[i].Day >= began; i-- {
		if resetOn(own[i], key) {
			peak, _ = soFar(own, key, i)
			return peak
		}
		if highest, ok := own[i].Highest[key]; ok && days[i].Day > began {
			peak = max(peak, millionthsOf(highest))
		}
	}
	return peak
}

// filedIn are days, in order, with the resets given filed among them, as the
// ledger files one: under the account's day on the local day of the reset,
// in the order its resets came, the day, and the account's day, added where
// days don't hold them. The days given are left as they were.
func filedIn(days []Day, resets []accountReset) []Day {
	filed := slices.Clone(days)
	for _, r := range resets {
		date := dayfile.DateOf(r.reset.At)
		i, found := slices.BinarySearchFunc(filed, date, func(d Day, date string) int { return strings.Compare(d.Day, date) })
		if !found {
			filed = slices.Insert(filed, i, Day{Day: date})
		}
		filed[i].Accounts = filedFor(filed[i].Accounts, r)
	}
	return filed
}

// filedFor are a day's accounts with the reset given filed under its
// account's, added where they don't hold it. The accounts given are left as
// they were.
func filedFor(accounts []ledger.PricedAccount, r accountReset) []ledger.PricedAccount {
	accounts = slices.Clone(accounts)
	j := slices.IndexFunc(accounts, func(a ledger.PricedAccount) bool { return a.Account == r.account })
	if j < 0 {
		j, accounts = len(accounts), append(accounts, ledger.PricedAccount{Account: r.account})
	}
	resets := append(slices.Clone(accounts[j].Resets), r.reset)
	slices.SortStableFunc(resets, func(a, b ledger.Reset) int { return a.At.Compare(b.At) })
	accounts[j].Resets = resets
	return accounts
}
