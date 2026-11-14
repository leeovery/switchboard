package status

import (
	"cmp"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
)

// Befell returns the account the event befell, as the pool it's of goes by:
// the one it names; else the first a pool's pin names; else the one a
// session moved to, where its session stays, or from, where it moved to
// none; "" for none, as of the router's own.
func (e Event) Befell() string {
	var pinned string
	if len(e.Accounts) > 0 {
		pinned = e.Accounts[0]
	}
	return cmp.Or(e.Account, pinned, e.To, e.From)
}

// InPool reports whether the event is of the pool with the id given, first
// being the id of the first pool, "" where it isn't known, and poolOf giving
// the id of the pool each account is in, as AccountPools has it, reporting
// false where none places it, ids compared ignoring case:
//   - a session's start or move, or a pin set or cleared, by the pool it
//     recorded at the time, one naming none, as from before pools, being the
//     first's, so one naming a pool since removed is in none;
//   - any other by the pool of the account it befell, as Befell has it;
//   - one befalling no account, as the router's own restarts and health, or
//     an account none places, in every pool.
func (e Event) InPool(pool, first string, poolOf func(account string) (string, bool)) bool {
	if KindNamesItsPool(e.Kind) {
		return config.SamePool(ledger.PoolRecorded(e.Pool, first), pool)
	}
	account := e.Befell()
	return account == "" || AccountIn(account, pool, poolOf)
}

// AccountIn reports whether what's of the account with the given id, its
// windows and what befell it, is of the pool with the id given, poolOf
// placing it as InPool says: where it places the account there, or nowhere,
// as one removed before pools, which every pool shows.
func AccountIn(account, pool string, poolOf func(account string) (string, bool)) bool {
	in, placed := poolOf(account)
	return !placed || config.SamePool(in, pool)
}

// KindNamesItsPool reports whether an event of the kind given names the pool
// it's of, as the router files it: a session's start or move, by the pool
// its request was routed in, and a pin set or cleared, by the pool whose pin
// it is.
func KindNamesItsPool(kind string) bool {
	switch kind {
	case EventStarted, EventMoved, EventPin, EventAuto:
		return true
	}
	return false
}

// AccountPools returns what gives the id of the pool each account is in, by
// its id, ignoring case: the pool configured gives it, of one configured;
// else the one the accounts record record returns kept it in last, of one
// removed, record called only for an account not configured; reporting false
// where neither places it.
func AccountPools(configured config.Accounts, record func() identity.Record) func(account string) (string, bool) {
	return func(account string) (string, bool) {
		if a, ok := configured.ByID(account); ok {
			return a.Pool, a.Pool != ""
		}
		e, ok := record().ByID(account)
		return e.Pool, ok && e.Pool != ""
	}
}
