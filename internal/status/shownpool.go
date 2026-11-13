package status

import (
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// ShownPool is the pool a dashboard shows, as the pools stood as it was
// chosen: its id and label; its directories, as the config writes them,
// none where it's unfenced; how many accounts are in it; and how many pools
// there are, as no screen names a pool till there's a second. One with no
// id is no pool: a config without pools, or one of several, every one
// fenced, with none named.
type ShownPool struct {
	ID, Label string
	Dirs      []string
	Accounts  int
	Pools     int
}

// PoolShown is the pool of pools with the id given, ignoring case, as a
// dashboard shows it, its accounts those of accounts in it: no pool, but for
// how many there are, where none of pools has the id.
func PoolShown(pools config.Pools, accounts config.Accounts, id string) ShownPool {
	p, ok := pools.ByID(id)
	if !ok {
		return ShownPool{Pools: len(pools)}
	}
	return ShownPool{ID: p.ID, Label: p.Label, Dirs: slices.Clone(p.WrittenDirs), Accounts: len(accounts.InPool(p.ID)), Pools: len(pools)}
}

// Named is the pool's label, as a screen names the pool it shows once
// there's a second: "" where there's one or none, or no pool is shown.
func (p ShownPool) Named() string {
	if p.ID == "" || !p.Turns() {
		return ""
	}
	return p.Label
}

// Turns reports whether there's another pool to turn to, as P turns them:
// where there's a second.
func (p ShownPool) Turns() bool {
	return p.Pools > 1
}

// Empty reports whether the pool shown has no accounts, so a session started
// in it doesn't start, and the dashboard draws a page saying so in place of
// its views.
func (p ShownPool) Empty() bool {
	return p.ID != "" && p.Accounts == 0
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
