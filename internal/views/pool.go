package views

import (
	"cmp"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolRead is a pool as a verb reads its work from what's recorded: the work
// of the pool with the id ID, as each request, session's start or move, or
// pin recorded its pool at the time, and what befell the accounts in it now,
// whose windows are their own whatever pool they're in. Its zero value reads
// every pool's, as of a config with no pools.
type PoolRead struct {
	ID string
	// First is the id of the first pool, once the move to pools is done, ""
	// till then: work recorded in no pool is the first's.
	First string
	// In reports whether the account with the given id is in the pool now,
	// and Pooled whether it's in any configured pool.
	In, Pooled func(account string) bool
}

// every reports whether the read is of every pool's work.
func (p PoolRead) every() bool {
	return p.ID == ""
}

// Lines are those of lines routed in the pool read, as each recorded at the
// time, ledger.Line's InPool has it: every one where the read is of every
// pool's.
func (p PoolRead) Lines(lines iter.Seq[ledger.Held]) iter.Seq[ledger.Held] {
	if p.every() {
		return lines
	}
	return func(yield func(ledger.Held) bool) {
		for h := range lines {
			if h.InPool(p.ID, p.First) && !yield(h) {
				return
			}
		}
	}
}

// Days reads the pool's days from those l reads, as LedgerDays reads them:
// each day's work in the pool alone, as ledger.Summary's InPool gives it,
// and of its accounts, those that worked in the pool that day, or are in it
// now, as In says, for their windows: every day whole where the read is of
// every pool's.
func (p PoolRead) Days(l LedgerDays) LedgerDays {
	if p.every() {
		return l
	}
	return poolDays{read: p, days: l}
}

// poolDays reads a pool's days, as PoolRead's Days has them.
type poolDays struct {
	read PoolRead
	days LedgerDays
}

func (d poolDays) Days(from time.Time) []ledger.Summary {
	days := d.days.Days(from)
	for i, s := range days {
		s = s.InPool(d.read.ID, d.read.First)
		s.Accounts = slices.DeleteFunc(s.Accounts, func(a ledger.AccountDay) bool {
			return a.Work.Idle() && (a.Account == "" || !d.read.In(a.Account))
		})
		days[i] = s
	}
	return days
}

// Shows reports whether the event e is of the pool read: one naming a pool,
// where it's the pool's; one of a kind that names its pool, but naming none,
// as before pools, where the pool is the first; one befalling an account in a
// configured pool, as its windows go, where the account is in the pool now;
// and one befalling none, as the router's own restarts and health, or an
// account in no pool, as one removed, is every pool's. Every event is the
// pool's where the read is of every pool's.
func (p PoolRead) Shows(e status.Event) bool {
	switch {
	case p.every():
		return true
	case e.Pool != "":
		return strings.EqualFold(e.Pool, p.ID)
	case namesItsPool(e.Kind):
		return strings.EqualFold(p.First, p.ID)
	}
	account := eventAccount(e)
	return !p.Pooled(account) || p.In(account)
}

// Events are those of held the pool read shows, as Shows has it.
func (p PoolRead) Events(held iter.Seq[events.Held]) iter.Seq[events.Held] {
	if p.every() {
		return held
	}
	return func(yield func(events.Held) bool) {
		for h := range held {
			if p.Shows(h.Event) && !yield(h) {
				return
			}
		}
	}
}

// namesItsPool reports whether an event of the kind given names the pool
// it's of, as the router files it: a session's start or move, by the pool
// its request was routed in, and a pin set or cleared, by the pool whose pin
// it is.
func namesItsPool(kind string) bool {
	switch kind {
	case status.EventStarted, status.EventMoved, status.EventPin, status.EventAuto:
		return true
	}
	return false
}

// eventAccount returns the account the event e befell, as a pool read shows
// it by, as the router shows an event in a pool's document: the one it
// names; else the first the pin names; else the one a session moved to,
// where its session stays, or from, where it moved to none; "" for none.
func eventAccount(e status.Event) string {
	var pinned string
	if len(e.Accounts) > 0 {
		pinned = e.Accounts[0]
	}
	return cmp.Or(e.Account, pinned, e.To, e.From)
}

// InPool is the List of the sessions of the pool read alone, what they come
// to together summed again: one the router lists in the pool, of running,
// which are the pool's, or one with a request in it today, of today's lines,
// as ledger.Line's InPool has it. The List is as it is where the read is of
// every pool's.
func (l SessionList) InPool(p PoolRead, running []status.Session, today []ledger.Held) SessionList {
	if p.every() {
		return l
	}
	in := make(map[string]bool)
	for _, s := range running {
		in[s.ID] = true
	}
	for _, h := range today {
		if h.InPool(p.ID, p.First) {
			in[h.Session] = true
		}
	}
	listed := SessionList{GeneratedAt: l.GeneratedAt, PricesAsOf: l.PricesAsOf, Sessions: []ListedSession{}}
	for _, s := range l.Sessions {
		if in[s.Session] {
			listed.Sessions = append(listed.Sessions, s)
			listed.Today.Requests += s.Requests
			listed.Today.add(s.Worth)
		}
	}
	listed.Today.Sessions = len(listed.Sessions)
	return listed
}
