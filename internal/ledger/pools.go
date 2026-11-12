package ledger

import (
	"slices"
	"strings"
	"sync"
)

// poolsVersion is the summary version that split each account's day's work
// by pool: a summary before it holds all its work in none.
const poolsVersion = 7

// FirstPool has the ledger, or a follower of it, know the first pool, the
// one the history from before pools was moved to, as first gives its id, ""
// where there's none: so a day whose summary holds work in none that its
// lines, moved to the first pool, no longer do is summarised again as
// fitted says. first is asked only of such a day.
func FirstPool(first func() string) Option {
	return func(o *options) { o.firstPool = first }
}

// fitted returns made, a day summarised afresh, and held, its summary
// before, as made is to know held, so each account's pools add up to its
// whole once it does, as fittedAccount fits each account's day.
func (d days) fitted(made Summary, held *Summary) (Summary, *Summary) {
	if held == nil {
		return made, nil
	}
	fit := *held
	fit.Accounts = slices.Clone(held.Accounts)
	made.Accounts = slices.Clone(made.Accounts)
	first := sync.OnceValue(func() string {
		if d.firstPool == nil {
			return ""
		}
		return d.firstPool()
	})
	for i, h := range fit.Accounts {
		j := slices.IndexFunc(made.Accounts, func(a AccountDay) bool { return a.Account == h.Account })
		if j >= 0 {
			made.Accounts[j], fit.Accounts[i] = fittedAccount(made.Accounts[j], h, held.Version < poolsVersion, first)
		}
	}
	return made, &fit
}

// fittedAccount returns made, an account's day summarised afresh, and held,
// its day before, as made is to know held, so its pools add up to its whole:
//   - held of a version before pools, unsplit, holds all its work in one
//     pool whatever its lines named, as a downgrade to a release before pools
//     summarises a day whose lines name pools: where made counts no less, as
//     covers says, held is split as made is, so made's pools stand, knowing
//     held's whole; else, some of held's lines lost since, which pool they
//     were in can't be told, so made's work is in held's one pool, as all of
//     held's is;
//   - held of a later version that has work in none that made, whose lines
//     give none, gives in the first pool, as first gives it, has its lines
//     renamed into that pool since, as the move to pools renames them, its
//     summary yet to follow: so held's work in none is the first pool's.
func fittedAccount(made, held AccountDay, unsplit bool, first func() string) (AccountDay, AccountDay) {
	switch {
	case unsplit && made.Work.covers(held.Work):
		held.Pool, held.Pools = made.Pool, made.Pools
	case unsplit:
		pool := held.Pool
		if held.Pools != nil {
			pool = ""
		}
		made.Pool, made.Pools = pool, nil
		held.Pool, held.Pools = pool, nil
	case held.worksIn("") && !made.worksIn("") && first() != "" && made.worksIn(first()):
		held, _ = held.unpooledIn(first())
	}
	return made, held
}

// worksIn reports whether the account did any work in the pool with the
// given id.
func (a AccountDay) worksIn(pool string) bool {
	return slices.ContainsFunc(a.poolDays(), func(p PoolDay) bool { return p.Pool == pool })
}

// covers reports whether w, work summarised afresh, counts no less than
// held, the same work summarised before: each of held's models among its
// own, counting as many or more of each, as countsNoLess says, and as many
// sessions and moves or more; so its lines hold every one held's did, as far
// as can be told.
func (w Work) covers(held Work) bool {
	if w.Sessions < held.Sessions || w.MovedOn < held.MovedOn || w.MovedOff < held.MovedOff {
		return false
	}
	return !slices.ContainsFunc(held.Models, func(h ModelDay) bool {
		i := slices.IndexFunc(w.Models, func(m ModelDay) bool { return m.key() == h.key() })
		return i < 0 || !w.Models[i].countsNoLess(h)
	})
}

// countsNoLess reports whether m counts as many of each kind of request as
// held, the same model's day, or more, and as many sessions.
func (m ModelDay) countsNoLess(held ModelDay) bool {
	return m.Upstream >= held.Upstream && m.NoUsage >= held.NoUsage && m.Unsent >= held.Unsent &&
		m.Checks >= held.Checks && m.Counts >= held.Counts && m.Sessions >= held.Sessions
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
