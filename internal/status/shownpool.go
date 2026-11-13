package status

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// ShownPool is the pool a dashboard shows, as the pools stood as it was
// chosen: its id and label; its directories, as the config writes them,
// none where it's unfenced; how many accounts the config put in it; and how
// many pools there are, as no screen names a pool till there's a second.
// One with no id is no pool: a config without pools, or one of several,
// every one fenced, with none named.
type ShownPool struct {
	ID, Label string
	Dirs      []string
	Accounts  int
	Pools     int
}

// PoolShown is the pool of pools with the id given, ignoring case, as a
// dashboard shows it, its accounts those PoolAccounts gives of accounts: no
// pool, but for how many there are, where none of pools has the id.
func PoolShown(pools config.Pools, accounts config.Accounts, id string) ShownPool {
	p, ok := pools.ByID(id)
	if !ok {
		return ShownPool{Pools: len(pools)}
	}
	return ShownPool{ID: p.ID, Label: p.Label, Dirs: slices.Clone(p.WrittenDirs), Accounts: len(PoolAccounts(pools, accounts, p.ID)), Pools: len(pools)}
}

// PoolAccounts are the accounts of accounts a dashboard shows of the pool of
// pools with the id given, ignoring case, as its Accounts tab, History and
// Graphs list them: every one in a config without pools; those in the pool;
// and none where none of pools has the id, as where every pool is fenced
// and none is shown. So no screen lists one pool's accounts in another's.
func PoolAccounts(pools config.Pools, accounts config.Accounts, id string) config.Accounts {
	if len(pools) == 0 {
		return accounts
	}
	p, ok := pools.ByID(id)
	if !ok {
		return nil
	}
	return accounts.InPool(p.ID)
}

// NamesThePool reports whether a screen, or a verb's text, names the pool it
// tells of, of pools as many as given: once there's a second, as with one
// switchboard says what it said before pools.
func NamesThePool(pools int) bool {
	return pools > 1
}

// Named is the pool's label, as a screen names the pool it shows once
// there's a second, as NamesThePool has it: "" where there's one or none, or
// no pool is shown.
func (p ShownPool) Named() string {
	if p.ID == "" || !p.Turns() {
		return ""
	}
	return p.Label
}

// Turns reports whether there's another pool to turn to, as the key line
// lists P: where there's a second, as NamesThePool has it.
func (p ShownPool) Turns() bool {
	return NamesThePool(p.Pools)
}

// Pooled reports whether there are pools at all, so P asks for the next
// however many there were as the pool was chosen: one added since, to a
// config of one, shows.
func (p ShownPool) Pooled() bool {
	return p.Pools > 0
}

// Empty reports whether the pool shown has no accounts, so a session started
// in it doesn't start, and the dashboard draws a page saying so in place of
// its views: as AsRead has it, where the dashboard has read the pool.
func (p ShownPool) Empty() bool {
	return p.ID != "" && p.Accounts == 0
}

// AsRead is the pool shown as the moment mo of the document read of it has
// it: its accounts those mo holds, once a document is read, so an account
// added to the pool, or the last taken out, shows at the next read; before
// the first, as the config had them.
func (p ShownPool) AsRead(mo Moment) ShownPool {
	if mo.Routed || mo.Probed {
		p.Accounts = len(mo.Accounts)
	}
	return p
}

// Fenced reports whether the pool shown is fenced to directories: else it's
// the pool of every session started outside every fence.
func (p ShownPool) Fenced() bool {
	return len(p.Dirs) > 0
}

// PoolAfter returns the pool P turns to from the one with the id given,
// ignoring case: the next of pools, in the config's order, the first after
// the last; else, where none of them has the id, as where every pool is
// fenced and none is shown, the first. It reports false where there are no
// pools.
func PoolAfter(pools config.Pools, id string) (config.Pool, bool) {
	if len(pools) == 0 {
		return config.Pool{}, false
	}
	i := slices.IndexFunc(pools, func(p config.Pool) bool { return strings.EqualFold(p.ID, id) })
	return pools[(i+1)%len(pools)], true
}
