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
// accounts: where m has an account's window a week long reset since it was
// read, and the page's days don't hold that reset yet, as before the router
// reads the account again, its weeks are worked out at m's Now as though the
// days held it, filed as the ledger files one, so its left at its last reset,
// its peaks and its verdict change on the second WEEK USED reads none. Once
// the days hold it, it isn't counted again. The rest stands as it was built.
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
// that m has reset since they were read and the page's days don't hold, each
// as the ledger would file it: at its reset, as read, the highest use read of
// the week it ended before it, the days' or the reading's; none where that's
// nothing, as the ledger files no reset of a week of none.
func (p AccountsPage) unfiled(m status.Moment) []accountReset {
	var resets []accountReset
	for _, a := range m.Accounts {
		if !slices.ContainsFunc(p.in.Accounts, func(c config.Account) bool { return c.ID == a.ID }) {
			continue
		}
		own := accountDays(p.days, a.ID)
		for _, w := range a.Windows {
			if !w.ResetSinceRead || !quota.WeekLong(w.Key) || resetAfter(own, w.Key, w.Read.At) {
				continue
			}
			before := max(w.Read.Used, highestSince(own, w.Key).share())
			if before == 0 {
				continue
			}
			resets = append(resets, accountReset{account: a.ID, reset: ledger.Reset{Window: w.Key, At: w.Read.ResetsAt.UTC(), Before: before}})
		}
	}
	return resets
}

// resetAfter reports whether an account's days hold a reset of the window with
// the given key after at, as one the router saw once it read it then.
func resetAfter(days []ledger.AccountDay, key string, at time.Time) bool {
	return slices.ContainsFunc(days, func(d ledger.AccountDay) bool {
		return slices.ContainsFunc(d.Resets, func(r ledger.Reset) bool { return r.Window == key && r.At.After(at) })
	})
}

// highestSince is the highest use of the window with the given key an
// account's days, oldest first, read since its last reset among them, as
// soFar has it, or of all of them, where none reset it: none where none is
// known.
func highestSince(days []ledger.AccountDay, key string) millionths {
	for i, d := range slices.Backward(days) {
		if resetOn(d, key) {
			peak, _ := soFar(days, key, i)
			return peak
		}
	}
	var peak millionths
	for _, d := range days {
		if highest, ok := d.Highest[key]; ok {
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
