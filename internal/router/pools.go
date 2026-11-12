package router

import (
	"cmp"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
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

// members returns the ids of the accounts the pool with the given id holds,
// as holds says, sorted: those the config the table was made from puts in
// it, which may name some the router doesn't have till it restarts.
func (t *poolTable) members(pool string) []string {
	var ids []string
	for id := range t.poolOf {
		if t.holds(pool, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// serves reports whether a request naming the pool given, "" for none, can go
// out on the account with the given id, as holds says of the pool it's in.
func (t *poolTable) serves(named, id string) bool {
	return t.holds(t.of(named), id)
}

// keeping returns the table with the accounts given in the pools they're in,
// and any other in none, as it has left the config: the pools and the first
// stand. Each is known by the id it has now, as idNow gives it, as a config
// put back from before switchboard made ids names it by the one it had.
// Where one given is an account the table doesn't have, it's the table
// whole: what the config makes of the accounts can't be told.
func (t *poolTable) keeping(accounts config.Accounts, idNow func(id string) string) *poolTable {
	kept := &poolTable{pools: t.pools, first: t.first, poolOf: make(map[string]string, len(accounts))}
	for _, a := range accounts {
		id := idNow(a.ID)
		pool, ok := t.poolOf[id]
		if !ok {
			return t
		}
		kept.poolOf[id] = pool
	}
	return kept
}

// keptTable returns the pools of accounts, from a config that names none
// once the move to pools has noted a first pool, till the config is given
// its pools again, as upkeep's heal gives them: those record keeps, as
// idmove's PoolsKept gives them, which the heal writes, each account by the
// id it has now, as idNow gives it, in the pool PoolsKept has it in, and one
// it has in none in none, never to serve. The pools are read for their ids,
// labels and members alone, their directories as written. It reports false
// where PoolsKept fails, as before the move.
func keptTable(record identity.Record, accounts config.Accounts, idNow func(id string) string) (*poolTable, bool) {
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = idNow(a.ID)
	}
	kept, err := idmove.PoolsKept(record, ids)
	if err != nil {
		return nil, false
	}
	pools := make(config.Pools, len(kept.Pools))
	for i, p := range kept.Pools {
		pools[i] = config.Pool{ID: p.ID, Label: cmp.Or(p.Label, p.ID), WrittenDirs: p.Dirs}
	}
	first, _ := record.FirstPool()
	t := newPoolTable(nil, pools, first.ID)
	for _, id := range ids {
		if pool := kept.In[id]; pool != "" {
			t.poolOf[id] = pool
		}
	}
	return t, true
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
	// record reads the accounts record, for its first pool and the pools it
	// keeps of this Mac's config, reporting false where it can't. idNow
	// gives the id an account a config names has now, as accounts' idNow
	// does.
	record func() (identity.Record, bool)
	idNow  func(id string) string

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

// newMembership returns the membership of a router that reads the accounts
// record as record does, and the id an account a config names has now as
// idNow does, following no pools yet: every account in the one pool there
// is, as in a config from before pools.
func newMembership(record func() (identity.Record, bool), idNow func(id string) string) *membership {
	m := &membership{record: record, idNow: idNow}
	m.table.Store(newPoolTable(nil, nil, ""))
	return m
}

// unpooled returns the membership of a router that routes within no pools,
// and has no accounts record: every account in the one pool there is, as a
// playback of the history has it.
func unpooled() *membership {
	return newMembership(noRecord, sameID)
}

// first returns the id of the first pool, as the accounts record notes it:
// "" where it notes none, or can't be read.
func (m *membership) first() string {
	r, _ := m.record()
	first, _ := r.FirstPool()
	return first.ID
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
// them, as one put back by hand or from a repository it's kept in, which
// upkeep's healPools gives them back, at the look that finds it, as the
// router's restart does: meanwhile the pools stand as kept says, so no
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
		logger.Warn("the config names no pools; the pools stand as they were till the config is given its pools again")
	}
	return kept
}

// kept returns the pools a config of accounts naming none keeps once the
// move to pools has noted a first pool, nil before: those the accounts
// record keeps, as keptTable gives them, the pools the heal writes back, so
// the router routes as the config will be, whether it started on the config
// or found it at a look; else, where the record can't be read, or keeps no
// pool of this Mac's config, those the last config with pools it followed
// made, but for accounts that have left the config, in none at once, as
// keeping says. m.mu must be held.
func (m *membership) kept(accounts config.Accounts) *poolTable {
	if r, ok := m.record(); ok {
		if t, ok := keptTable(r, accounts, m.idNow); ok {
			return t
		}
	}
	if m.pooled != nil {
		return m.pooled.keeping(accounts, m.idNow)
	}
	return nil
}

// identicalPools reports whether a and b are the same pools as far as routing
// goes, what the router tells of them and what the accounts record keeps of
// them: the same ids, in the same order, with the same labels and
// directories.
func identicalPools(a, b config.Pools) bool {
	return slices.EqualFunc(a, b, func(p, q config.Pool) bool {
		return p.ID == q.ID && p.Label == q.Label && slices.Equal(p.Dirs, q.Dirs) && slices.Equal(p.WrittenDirs, q.WrittenDirs)
	})
}

// recordIn returns what reads the accounts record in the state directory
// given: false where the router has no state directory, or the record can't
// be read, which it logs at debug, the config's first pool standing in for
// the record's.
func recordIn(stateDir string) func() (identity.Record, bool) {
	if stateDir == "" {
		return noRecord
	}
	file := identity.NewFile(stateDir)
	return func() (identity.Record, bool) {
		r, err := file.Read()
		if err != nil {
			logger.Debug("can't read the accounts record's pools; the config's first stands in", "error", err)
			return identity.Record{}, false
		}
		return r, true
	}
}

// noRecord has no accounts record to read.
func noRecord() (identity.Record, bool) {
	return identity.Record{}, false
}

// sameID knows of no id an account had before switchboard made ids: an
// account's id now is the one given.
func sameID(id string) string {
	return id
}
