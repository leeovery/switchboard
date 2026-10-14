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
// accounts: its weeks as the days stand at m, as Past's filedAt files them,
// so its left at its last reset, its peaks and its verdict change on the
// second WEEK USED reads none, and stay so, as Weeks' do. The rest stands as
// it was built.
func (p AccountsPage) At(m status.Moment) AccountsPage {
	stood, ok := Past{in: p.in, days: p.days}.filedAt(m)
	if !ok {
		return p
	}
	p.Rows = slices.Clone(p.Rows)
	p.weeked(stood.in, stood.days)
	return p
}

// filedAt is the Past as it stands at the moment m, of the document of its
// accounts, reporting false where it stands as it is: where what m reads of
// an account's week has it reset since the days were read, and the days don't
// hold that reset yet, it's filed among them, as the ledger files it once it
// reads them again, and the Past is at m's Now. Once the days hold it, it
// isn't filed again. Every page that tells the accounts' weeks stands so, so
// each tells them alike.
func (p Past) filedAt(m status.Moment) (Past, bool) {
	resets := p.unfiled(m)
	if len(resets) == 0 {
		return p, false
	}
	p.in.Now, p.days = m.Now, filedIn(p.days, resets)
	return p, true
}

// Until is when the page, as At stands it at the moment m, next stands
// otherwise, as Past's standsUntil has it: till then, At stands it alike at
// every moment of m's document.
func (p AccountsPage) Until(m status.Moment) time.Time {
	return Past{in: p.in, days: p.days}.standsUntil(m)
}

// standsUntil is when the Past, as filedAt stands it at the moment m, next
// stands otherwise, at a moment of m's document: the first time after m's
// Now that the day ends, as a Past at m's Now tells its weeks to today, that
// a configured account's week resets, as m reads it, or that a week long
// window runs a length on from a reset the days, as they stand, give, as the
// week it began ends then. Till then, filedAt stands it alike at every
// moment of m's document, as what it files, and the weeks the days tell,
// change only as those come.
func (p Past) standsUntil(m status.Moment) time.Time {
	stood, _ := p.filedAt(m)
	next := dayfile.DayStart(m.Now, 1)
	ahead := func(at time.Time) {
		if at.After(m.Now) && at.Before(next) {
			next = at
		}
	}
	for _, c := range p.in.Accounts {
		if a, ok := m.Account(c.ID); ok {
			if w, read := a.Window(p.in.WeekWindow); read {
				ahead(w.Read.ResetsAt)
			}
		}
		for _, d := range accountDays(stood.days, c.ID) {
			for _, r := range d.Resets {
				if length, ok := quota.Length(r.Window); ok && quota.WeekLong(r.Window) {
					ahead(r.At.Add(length))
				}
			}
		}
	}
	return next
}

// accountReset is a reset of the window of the account with the given id.
type accountReset struct {
	account string
	reset   ledger.Reset
}

// unfiled are the resets of the configured accounts' own weeks, the week
// window alone, as the weeks the views tell are of it alone, that the ledger
// files of what m reads and the days don't hold, as endedSince has them.
func (p Past) unfiled(m status.Moment) []accountReset {
	var resets []accountReset
	for _, a := range m.Accounts {
		w, read := a.Window(p.in.WeekWindow)
		if !read || !slices.ContainsFunc(p.in.Accounts, func(c config.Account) bool { return c.ID == a.ID }) {
			continue
		}
		for _, reset := range p.endedSince(w, accountDays(p.days, a.ID), m.Now) {
			resets = append(resets, accountReset{account: a.ID, reset: reset})
		}
	}
	return resets
}

// endedSince are the resets of the window w of an account, whose days are
// own, a day each of the Past's, that the ledger files of its readings since
// the days were read, to now, as its Climb files them: the days' last reading
// of the window first, as the run it read stood as they were read, at its
// highest use; then w's reading, at the time it was read. The days' run is
// the one w reads, as it began before they were read, or they hold the reset
// that began it; else it ended as w's began, after they were read. So a run
// begun with no reset the days hold, on a fresh install, an account added
// part-way through it, after a week that lapsed or one of no use, ends none
// before it, as the ledger files none of a run it never read, nor of one
// that read nothing, and of one that lapsed, its own long since. None are
// filed where the days hold a reset since w was read, as they've read past
// the run it reads.
func (p Past) endedSince(w status.WindowNow, own []ledger.AccountDay, now time.Time) []ledger.Reset {
	length, ok := quota.Length(w.Key)
	if !ok || resetAfter(own, w.Key, w.Read.At) {
		return nil
	}
	read := quota.Window{Key: w.Key, Utilization: w.Read.Used, ResetsAt: w.Read.ResetsAt}
	climb := ledger.NewClimb(w.Key, nil)
	if began := runBegun(w, length); p.begunSince(began, own, w.Key, length) {
		climb.Carry(quota.Window{Key: w.Key, Utilization: highestBefore(p.days, own, w.Key, began).share(), ResetsAt: began}, p.in.read)
	} else {
		climb.BeginAt(highestBefore(p.days, own, w.Key, w.Read.ResetsAt).share())
	}
	climb.Take(read, w.Read.At)
	climb.End(now)
	return climb.Resets()
}

// runBegun is when the run of the window w a reading read began, the window
// length long: its start as the moment has it, where it hasn't reset since;
// else a length before the reset it read.
func runBegun(w status.WindowNow, length time.Duration) time.Time {
	if w.ResetSinceRead {
		return w.Read.ResetsAt.Add(-length)
	}
	return w.Start
}

// begunSince reports whether a run of the window with the given key, length
// long, that began at began, did so after the Past's days were read, when
// they were, not at its Now, as a page is built of them after they land, and
// they hold no reset that began it: none of the window since half its length
// before it, as the days file the reset that began a run at its start as
// Claude gives it, to the second or so, and the one before came a whole
// length before.
func (p Past) begunSince(began time.Time, own []ledger.AccountDay, key string, length time.Duration) bool {
	return began.After(p.in.read) && !resetAfter(own, key, began.Add(-length/2))
}

// resetAfter reports whether an account's days hold a reset of the window with
// the given key after at, as one the router saw once it read it then.
func resetAfter(days []ledger.AccountDay, key string, at time.Time) bool {
	return slices.ContainsFunc(days, func(d ledger.AccountDay) bool {
		return slices.ContainsFunc(d.Resets, func(r ledger.Reset) bool { return r.Window == key && r.At.After(at) })
	})
}

// highestBefore is the highest use read of the window with the given key in
// the run that ends at end, of an account's days, own, a day each of days,
// oldest first, those to the day end falls on: since its last reset among
// them, as soFar has it, where that came on or after the day the run began
// on, the window's length before end; else of the days after that one, as
// that day's highest may be the run before's. None where none is read.
func highestBefore(days []Day, own []ledger.AccountDay, key string, end time.Time) millionths {
	length, _ := quota.Length(key)
	began := dayfile.DateOf(end.Add(-length))
	ends, on := slices.BinarySearchFunc(days, dayfile.DateOf(end), func(d Day, date string) int { return strings.Compare(d.Day, date) })
	if on {
		ends++
	}
	days, own = days[:ends], own[:ends]
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
