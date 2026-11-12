package ledger

import "slices"

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
