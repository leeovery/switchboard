package ledger

import (
	"slices"
	"strings"
)

// poolsVersion is the summary version that split each account's day's work
// by pool: a summary before it holds all its work in none.
const poolsVersion = 7

// FirstPool has the ledger, or a follower of it, know the first pool, the
// one the history from before pools was moved to, as first gives its id, ""
// where there's none: so a day whose summary, of a version before pools,
// holds its work in none while its lines name a pool, as a downgrade to a
// release before pools can leave one, is summarised again as heldPooled
// says. first is asked only of such a day.
func FirstPool(first func() string) Option {
	return func(o *options) { o.firstPool = first }
}

// heldPooled returns held, a day's summary before, as made, the day
// summarised afresh, is to know it: where held is of a version before pools,
// all its work in none, while made names a pool, its work in none the first
// pool's, as firstPool names it, as the move to pools gave it, so the pools
// made knows don't add up past its whole; else as it is.
func (d days) heldPooled(made Summary, held *Summary) *Summary {
	if held == nil || held.Version >= poolsVersion || d.firstPool == nil || held.pooled() || !made.pooled() {
		return held
	}
	first := d.firstPool()
	if first == "" {
		return held
	}
	pooled := held.unpooledIn(first)
	return &pooled
}

// pooled reports whether any of the day's work names a pool.
func (s Summary) pooled() bool {
	return slices.ContainsFunc(s.Accounts, func(a AccountDay) bool {
		return slices.ContainsFunc(a.poolDays(), func(p PoolDay) bool { return p.Pool != "" })
	})
}

// unpooledIn returns the day with each account's work in no pool in the pool
// with the given id instead, as AccountDay's unpooledIn gives it.
func (s Summary) unpooledIn(pool string) Summary {
	accounts := make([]AccountDay, len(s.Accounts))
	for i, a := range s.Accounts {
		accounts[i], _ = a.unpooledIn(pool)
	}
	s.Accounts = accounts
	return s
}

// unpooledIn returns the account's day with its work in no pool, of the
// requests that named none, as every one did before pools, in the pool with
// the given id instead, merged with its work there, as mergedPool merges it;
// and reports whether it had any work in none.
func (a AccountDay) unpooledIn(pool string) (AccountDay, bool) {
	days := slices.Clone(a.poolDays())
	i := slices.IndexFunc(days, func(p PoolDay) bool { return p.Pool == "" })
	if i < 0 {
		return a, false
	}
	days[i].Pool = pool
	return a.pooled(mergedBy(days, PoolDay.key, mergedPool, strings.Compare)), true
}

// InPool returns the day in the pool with the given id alone: each account's
// day as AccountDay.InPool gives it, and the day of the requests no account
// answered where some of them were in the pool. Its version, lines and bytes
// are the whole day's. "" gives the work of the requests that named no pool,
// as none did before pools: every day of a summary before version 7.
func (s Summary) InPool(pool string) Summary {
	var accounts []AccountDay
	for _, a := range s.Accounts {
		if in := a.InPool(pool); in.Account != "" || !in.idle() {
			accounts = append(accounts, in)
		}
	}
	s.Accounts = accounts
	return s
}

// InPool returns the account's day in the pool with the given id alone: its
// work there, none where it did none there, and its windows as they are, an
// account's windows being its own, whatever pool it's in. Which accounts'
// windows a pool shows is its reader's to say, by the pool each is in now.
func (a AccountDay) InPool(pool string) AccountDay {
	a.Work = a.workIn(pool)
	a.Pool, a.Pools = pool, nil
	return a
}

// workIn returns the account's work in the pool with the given id: none
// where it did none there, its sessions named as its whole work names its.
func (a AccountDay) workIn(pool string) Work {
	if a.Pools == nil && a.Pool == pool {
		return a.Work
	}
	if i := slices.IndexFunc(a.Pools, func(p PoolDay) bool { return p.Pool == pool }); i >= 0 {
		return a.Pools[i].Work
	}
	if a.SessionIDs == nil {
		return Work{}
	}
	return Work{SessionIDs: []string{}}
}

// poolDays returns the account's work by pool, in the order of their ids:
// its Pools, where it was in more than one; else its work, in its Pool; none
// where its work is none, as an account's the readings history alone tells
// of.
func (a AccountDay) poolDays() []PoolDay {
	switch {
	case a.Pools != nil:
		return a.Pools
	case a.idle():
		return nil
	}
	return []PoolDay{{Pool: a.Pool, Work: a.Work}}
}

// key names the account's work in a pool by its pool.
func (p PoolDay) key() string {
	return p.Pool
}

// pooled returns the account's day with its work by pool as pools gives it,
// in the order of their ids: named by its one pool, where there's one, or
// listed, where there are more. Its work in every pool together is its own.
func (a AccountDay) pooled(pools []PoolDay) AccountDay {
	a.Pool, a.Pools = "", nil
	switch len(pools) {
	case 0:
	case 1:
		a.Pool = pools[0].Pool
	default:
		a.Pools = pools
	}
	return a
}

// idle reports whether the work is none: no model's requests, no session and
// no move, as an account's day of no requests holds.
func (w Work) idle() bool {
	return len(w.Models) == 0 && w.Sessions == 0 && w.MovedOn == 0 && w.MovedOff == 0
}
