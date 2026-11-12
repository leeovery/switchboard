package ledger

import (
	"slices"
	"strings"
)

// poolsVersion is the summary version that split each account's day's work
// by pool: a summary before it holds all its work in none.
const poolsVersion = 7

// FirstPool has the ledger, or a follower of it, know the first pool, the
// one the history from before pools was moved to, as first gives its id:
// "" before the move to pools is done. Work in no pool once it is, as a
// release before pools files, or a router draining as another takes over,
// or a pin filed before pins named their pool, is the first pool's, as
// fitted has it. first is asked only of a day with work in none.
func FirstPool(first func() string) Option {
	return func(o *options) { o.firstPool = first }
}

// fitted returns made, a day summarised afresh, and held, its summary
// before, where there's one, as made is to know held: each account's day
// fitted as fittedAccount fits it, so its pools add up to its whole once
// made knows held; then, where the ledger knows the first pool, as
// FirstPool says, each's work in no pool, made's and held's, the first
// pool's, as unpooledIn gives it.
func (d days) fitted(made Summary, held *Summary) (Summary, *Summary) {
	made.Accounts = slices.Clone(made.Accounts)
	first, asked := d.firstOf(made, held)
	var fit *Summary
	if held != nil {
		f := *held
		f.Accounts = slices.Clone(held.Accounts)
		for i, h := range f.Accounts {
			j := slices.IndexFunc(made.Accounts, func(a AccountDay) bool { return a.Account == h.Account })
			if j >= 0 {
				made.Accounts[j], f.Accounts[i] = fittedAccount(made.Accounts[j], h, held.Version < poolsVersion, first != "")
			}
		}
		fit = &f
	}
	// fittedAccount's second case can leave work in no pool where neither
	// had any.
	if !asked {
		first, _ = d.firstOf(made, fit)
	}
	if first != "" {
		made = made.unpooledIn(first)
		if fit != nil {
			*fit = fit.unpooledIn(first)
		}
	}
	return made, fit
}

// firstOf returns the first pool, as FirstPool has the ledger know it,
// where made or held, where it's given, has work in no pool, reporting that
// it asked: else "", unasked.
func (d days) firstOf(made Summary, held *Summary) (string, bool) {
	unpooled := made.worksIn("") || held != nil && held.worksIn("")
	if !unpooled || d.firstPool == nil {
		return "", false
	}
	return d.firstPool(), true
}

// fittedAccount returns made, an account's day summarised afresh, and held,
// its day before, as made is to know held, so its pools add up to its whole:
//   - held unsplit, of a version before pools, holding all its work in one
//     pool whatever its lines named, as a downgrade's summary does, or held
//     with work in no pool where made has none, as one made before its lines
//     were given the first pool by the move to pools, the first pool not
//     known, as firstKnown says: where made counts no less, as covers says,
//     its lines hold every one held's did, so held is split as made is,
//     made's pools standing, knowing held's whole. Once the first pool is
//     known, held's work in none is the first pool's, so the last case fits
//     it: held split as made is would hide a loss in another pool behind
//     what was filed in the first since;
//   - else, held unsplit, some of its lines lost since: which pool they were
//     in can't be told, so made's work is in held's one pool, as all of
//     held's is;
//   - else each pool knows no less than held's, as knowing has it, the whole
//     their sum where that's more: the whole day counting no less never
//     says which pool's lines were lost, as more filed since in another
//     would hide a loss in one.
func fittedAccount(made, held AccountDay, unsplit, firstKnown bool) (AccountDay, AccountDay) {
	renamed := !firstKnown && held.worksIn("") && !made.worksIn("")
	switch {
	case (unsplit || renamed) && made.covers(held.Work):
		held.Pool, held.Pools = made.Pool, made.Pools
	case unsplit:
		pool := held.Pool
		if held.Pools != nil {
			pool = ""
		}
		made.Pool, made.Pools = pool, nil
		held.Pool, held.Pools = pool, nil
	}
	return made, held
}

// worksIn reports whether any account's day has work in the pool with the
// given id.
func (s Summary) worksIn(pool string) bool {
	return slices.ContainsFunc(s.Accounts, func(a AccountDay) bool { return a.worksIn(pool) })
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
// day as AccountDay.InPool gives it, first the first pool's id, and the day
// of the requests no account answered where some of them were in the pool.
// Its version, lines and bytes are the whole day's.
func (s Summary) InPool(pool, first string) Summary {
	var accounts []AccountDay
	for _, a := range s.Accounts {
		if in := a.InPool(pool, first); in.Account != "" || !in.idle() {
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
// first is the id of the first pool, once the move to pools is done, "" till
// then: work in no pool left in a summary is the first's, as FirstPool says,
// and "" gives none; before, "" gives the work of the requests that named
// none, as none did before pools, every day's of a summary before version 7.
func (a AccountDay) InPool(pool, first string) AccountDay {
	if first != "" {
		a, _ = a.unpooledIn(first)
	}
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
