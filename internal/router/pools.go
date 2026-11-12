package router

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// poolTable is which pool each account is in, as a config the router follows
// makes them, and the pool a request that names none is in. It's never
// changed once made, only replaced whole, as membership replaces it, so a
// request reads one as it stood, never one half changed.
type poolTable struct {
	// pools are the pools configured, in the config's order: none in a
	// config from before pools, whose accounts are every one in the one pool
	// there is, whatever pool a request names.
	pools config.Pools
	// poolOf holds, by id, the pool each account configured is in, by the
	// pool's id as the config spells it.
	poolOf map[string]string
	// first is the id of the pool a request that names none is in, as one
	// from a session started before pools: where all work before pools was.
	first string
}

// newPoolTable returns the pools accounts and pools make, as a config gives
// them, first being the id of the first pool, as the accounts record notes
// it: the pool a request that names none is in, where the config has it,
// else the config's first. A Mac that moved to pools before a config shared
// with another reached it keeps a first pool of its own that the config it
// shares has put aside, and its sessions from before pools go where that
// config put every account.
func newPoolTable(accounts config.Accounts, pools config.Pools, first string) *poolTable {
	t := &poolTable{pools: slices.Clone(pools), poolOf: make(map[string]string, len(accounts))}
	for _, a := range accounts {
		t.poolOf[a.ID] = a.Pool
	}
	switch p, ok := pools.ByID(first); {
	case ok:
		t.first = p.ID
	case len(pools) > 0:
		t.first = pools[0].ID
	}
	return t
}

// of returns the id of the pool a request naming the pool given is in, as its
// header names it: the first, where it names none; else the one it names, as
// the config spells its id, or as named where the config has no such pool. In
// a config from before pools it's none, "", whatever is named.
func (t *poolTable) of(named string) string {
	if len(t.pools) == 0 {
		return ""
	}
	if named == "" {
		return t.first
	}
	if p, ok := t.pools.ByID(named); ok {
		return p.ID
	}
	return named
}

// holds reports whether the account with the given id is in the pool with the
// given id, as of returns it: in a config from before pools, every account
// is, as today; else only an account the config configures, in the pool it
// names, so one that has left the config is in none.
func (t *poolTable) holds(pool, id string) bool {
	if len(t.pools) == 0 {
		return true
	}
	in, ok := t.poolOf[id]
	return ok && in == pool
}

// serves reports whether a request naming the pool given, "" for none, can go
// out on the account with the given id, as holds says of the pool it's in.
func (t *poolTable) serves(named, id string) bool {
	return t.holds(t.of(named), id)
}

// keeping returns the table with the accounts given in the pools they're in,
// and any other in none, as it has left the config: the pools and the first
// stand.
func (t *poolTable) keeping(accounts config.Accounts) *poolTable {
	kept := &poolTable{pools: t.pools, first: t.first, poolOf: make(map[string]string, len(accounts))}
	for _, a := range accounts {
		if pool, ok := t.poolOf[a.ID]; ok {
			kept.poolOf[a.ID] = pool
		}
	}
	return kept
}

// healedTable returns the pools of accounts, from a config that names none
// once the move to pools is done, as the router's restart heals the config:
// the first pool, the one with the given id, and every account in it.
func healedTable(accounts config.Accounts, first string) *poolTable {
	t := newPoolTable(nil, config.Pools{{ID: first, Label: first}}, first)
	for _, a := range accounts {
		t.poolOf[a.ID] = first
	}
	return t
}

// shown returns how what switchboard tells a client names the pool with the
// given id: by its label, else its id, where there are pools to tell apart;
// "" where there's one at most, as nothing names a pool till there's a second.
func (t *poolTable) shown(pool string) string {
	if len(t.pools) < 2 {
		return ""
	}
	if p, ok := t.pools.ByID(pool); ok {
		return p.Label
	}
	return pool
}

// membership holds the pools the router routes within, as a poolTable, taken
// up from the config the router follows at once, at the look upkeep takes,
// rather than at its next restart. Every request's choice reads the table
// whole, as current gives it, and follow replaces it whole, so none reads it
// half changed. It's safe for concurrent use.
type membership struct {
	table atomic.Pointer[poolTable]
	// first reads the id of the first pool, as the accounts record notes it:
	// "" where it notes none.
	first func() string

	// mu is held as the pools are followed, and guards what they were last
	// followed from: accounts and pools, and followed, set once they have
	// been; pooled, the table the last config with pools made, nil till one
	// has been followed; and warned, set once the log has said a config names
	// no pools since.
	mu       sync.Mutex
	accounts config.Accounts
	pools    config.Pools
	followed bool
	pooled   *poolTable
	warned   bool
}

// newMembership returns the membership of a router that reads the first pool
// as first does, following no pools yet: every account in the one pool there
// is, as in a config from before pools.
func newMembership(first func() string) *membership {
	m := &membership{first: first}
	m.table.Store(newPoolTable(nil, nil, ""))
	return m
}

// unpooled returns the membership of a router that routes within no pools,
// and knows of no first pool: every account in the one pool there is, as a
// playback of the history has it.
func unpooled() *membership {
	return newMembership(noFirst)
}

// current returns the pools as they stand now.
func (m *membership) current() *poolTable {
	return m.table.Load()
}

// follow takes up the pools a config makes of the accounts and pools it
// configures, as made says, where they differ from those it last took up:
// which pool each account is in, the pools themselves with their
// directories, the accounts' primaries, and the first, read afresh from the
// accounts record. The move to pools writes the first pool to the record
// before the config, so a config that changes finds it there.
func (m *membership) follow(accounts config.Accounts, pools config.Pools) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.followed && slices.Equal(accounts, m.accounts) && identicalPools(pools, m.pools) {
		return
	}
	m.table.Store(m.made(accounts, pools))
	m.accounts, m.pools, m.followed = slices.Clone(accounts), slices.Clone(pools), true
}

// made returns the pools a config makes of the accounts and pools it
// configures. A config with pools makes them. One with none, from before the
// move to pools, makes none, every account in the one pool there is, as
// before pools; but once the move is done a config naming none has lost
// them, as one put back by hand or from a repository it's kept in, which the
// router's restart heals: meanwhile the pools stand as kept says, so no
// pool's work goes out on another's accounts, and the log says so, once.
// m.mu must be held.
func (m *membership) made(accounts config.Accounts, pools config.Pools) *poolTable {
	if len(pools) > 0 {
		m.pooled, m.warned = newPoolTable(accounts, pools, m.first()), false
		return m.pooled
	}
	kept := m.kept(accounts)
	if kept == nil {
		return newPoolTable(accounts, nil, "")
	}
	if !m.warned {
		m.warned = true
		logger.Warn("the config names no pools; the pools stand as they were till the router restarts, giving the config its first pool again")
	}
	return kept
}

// kept returns the pools a config of accounts naming none keeps once the
// move to pools is done, as far as the router can tell, nil before: those the
// last config with pools it followed made, but for accounts that have left
// the config, in none at once; else, where the accounts record notes a first
// pool, as for a router started on such a config, that pool alone with every
// account in it, as the restart's heal will make them. m.mu must be held.
func (m *membership) kept(accounts config.Accounts) *poolTable {
	if m.pooled != nil {
		return m.pooled.keeping(accounts)
	}
	if first := m.first(); first != "" {
		return healedTable(accounts, first)
	}
	return nil
}

// samePools reports whether a and b are the same pools as far as the
// accounts record goes: the same ids, in the same order, with the same
// labels.
func samePools(a, b config.Pools) bool {
	return slices.EqualFunc(a, b, func(p, q config.Pool) bool { return p.ID == q.ID && p.Label == q.Label })
}

// identicalPools reports whether a and b are the same pools as far as routing
// goes, and what the router tells of them: the same ids, in the same order,
// with the same labels and directories.
func identicalPools(a, b config.Pools) bool {
	return slices.EqualFunc(a, b, func(p, q config.Pool) bool {
		return p.ID == q.ID && p.Label == q.Label && slices.Equal(p.Dirs, q.Dirs) && slices.Equal(p.WrittenDirs, q.WrittenDirs)
	})
}

// firstIn returns what reads the id of the first pool from the accounts
// record in the state directory given, as its pool marked First: "" where it
// marks none, has no state directory, or can't be read, which it logs at
// debug, the config's first pool standing in.
func firstIn(stateDir string) func() string {
	if stateDir == "" {
		return noFirst
	}
	record := identity.NewFile(stateDir)
	return func() string {
		r, err := record.Read()
		if err != nil {
			logger.Debug("can't read the accounts record's first pool; the config's first stands in", "error", err)
			return ""
		}
		i := slices.IndexFunc(r.Pools, func(p identity.PoolEntry) bool { return p.First })
		if i < 0 {
			return ""
		}
		return r.Pools[i].ID
	}
}

// noFirst knows of no first pool.
func noFirst() string {
	return ""
}
