package router

import (
	"cmp"
	"maps"
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
	// was holds, by id, the pool each account that has left the tables before
	// this one, while the router runs, was in last, as they had it: none of
	// those poolOf holds.
	was map[string]string
	// primary holds, by the pool's id, the id of the account the config
	// makes each pool's primary, as poolOf spells the pool.
	primary map[string]string
	// replaced is closed once membership has replaced the table with another,
	// for a reader held open to look at the pools again.
	replaced chan struct{}
}

// newPoolTable returns the pools accounts and pools make, as a config gives
// them, first being the id of the first pool, as the accounts record notes
// it: the pool a request that names none is in, where the config has it,
// else the config's first. A Mac that moved to pools before a config shared
// with another reached it keeps a first pool of its own that the config it
// shares has put aside, and its sessions from before pools go where that
// config put every account.
func newPoolTable(accounts config.Accounts, pools config.Pools, first string) *poolTable {
	t := &poolTable{
		pools: slices.Clone(pools), poolOf: make(map[string]string, len(accounts)), primary: make(map[string]string),
		replaced: make(chan struct{}),
	}
	for _, a := range accounts {
		t.poolOf[a.ID] = a.Pool
		if a.Primary {
			t.primary[a.Pool] = a.ID
		}
	}
	t.first = firstPool(pools, first)
	return t
}

// firstPool returns the id of the first of pools: the one with the id first,
// as the accounts record notes it, where pools has it, else the first
// configured, as the config spells its id; "" where there are none.
func firstPool(pools config.Pools, first string) string {
	switch p, ok := pools.ByID(first); {
	case ok:
		return p.ID
	case len(pools) > 0:
		return pools[0].ID
	}
	return ""
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

// pooled reports whether the table has pools, as one made from a config from
// before pools hasn't.
func (t *poolTable) pooled() bool {
	return len(t.pools) > 0
}

// configured reports whether the table has the pool with the given id.
func (t *poolTable) configured(pool string) bool {
	_, ok := t.pools.ByID(pool)
	return ok
}

// placed reports whether the table puts the account with the given id in a
// pool, as one the config configures.
func (t *poolTable) placed(id string) bool {
	_, ok := t.poolOf[id]
	return ok
}

// lastIn returns the id of the pool the account with the given id is in,
// else the one it was in last as it left the tables before this one,
// reporting false where the table knows neither.
func (t *poolTable) lastIn(id string) (string, bool) {
	if pool, ok := t.poolOf[id]; ok {
		return pool, true
	}
	pool, ok := t.was[id]
	return pool, ok
}

// after returns the table as it follows before, knowing the pool each
// account that has left it was in last, as before had it, as was says.
func (t *poolTable) after(before *poolTable) *poolTable {
	next := *t
	next.was = maps.Clone(before.was)
	if next.was == nil {
		next.was = make(map[string]string)
	}
	for id, pool := range before.poolOf {
		if pool != "" {
			next.was[id] = pool
		}
	}
	for id := range t.poolOf {
		delete(next.was, id)
	}
	return &next
}

// keeping returns the table with the accounts given in the pools they're in,
// and any other in none, as it has left the config: the pools, their
// primaries and the first stand. Each is known by the id it has now, as
// idNow gives it, as a config put back from before switchboard made ids
// names it by the one it had. Where one given is an account the table
// doesn't have, it's the table whole: what the config makes of the accounts
// can't be told.
func (t *poolTable) keeping(accounts config.Accounts, idNow func(id string) string) *poolTable {
	kept := &poolTable{
		pools: t.pools, first: t.first, primary: t.primary, poolOf: make(map[string]string, len(accounts)),
		replaced: make(chan struct{}),
	}
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

// healedTable returns the pools of accounts, from a config that names none
// once the move to pools is done, till the router's restart gives the config
// its pools again: the first pool, as the accounts record keeps it, labelled
// by its id where it keeps no label, and every account in it, by the id it
// has now, as idNow gives it, its primary the config's.
func healedTable(accounts config.Accounts, first config.Pool, idNow func(id string) string) *poolTable {
	t := newPoolTable(nil, config.Pools{{ID: first.ID, Label: cmp.Or(first.Label, first.ID)}}, first.ID)
	for _, a := range accounts {
		t.poolOf[idNow(a.ID)] = first.ID
	}
	if len(accounts) > 0 {
		t.primary[first.ID] = idNow(accounts.Primary().ID)
	}
	return t
}

// apart reports whether the table has pools to tell apart, two or more: what
// switchboard says names no pool till there's a second.
func (t *poolTable) apart() bool {
	return len(t.pools) > 1
}

// shown returns how what switchboard tells a client names the pool with the
// given id: by its label, else its id, where there are pools to tell apart,
// as apart says; "" where there aren't.
func (t *poolTable) shown(pool string) string {
	if !t.apart() {
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
	// first reads the first pool, its id and its label, as the accounts
	// record notes it: zero where it notes none. idNow gives the id an
	// account a config names has now, as accounts' idNow does. recorded reads
	// the id of the pool the accounts record keeps the account with the id
	// given in, "" where it keeps none: what the pools the router follows
	// can't know of an account removed before it started.
	first    func() config.Pool
	idNow    func(id string) string
	recorded func(id string) string

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
// as first does, the id an account a config names has now as idNow does, and
// the pool the accounts record keeps an account in as recorded does,
// following no pools yet: every account in the one pool there is, as in a
// config from before pools.
func newMembership(first func() config.Pool, idNow, recorded func(id string) string) *membership {
	m := &membership{first: first, idNow: idNow, recorded: recorded}
	m.table.Store(newPoolTable(nil, nil, ""))
	return m
}

// heirs returns what says which account the tokens of an account that leaves
// a config of accounts and pools count as, the first pool read as the
// membership reads it, once at most, and the pool the account was in as
// poolWas has it, as the pools stand as it's asked.
func (m *membership) heirs(accounts config.Accounts, pools config.Pools) heirs {
	return heirs{accounts: accounts, pools: pools, first: sync.OnceValue(m.first), poolWas: m.poolWas}
}

// poolWas returns the id of the pool the account with the given id is in, or
// was in, as poolIn says of the pools as they stand now.
func (m *membership) poolWas(id string) string {
	return m.poolIn(m.current(), id)
}

// poolIn returns the id of the pool the account with the given id is in, as
// pools has it, else the one it was in last as it left the pools while the
// router ran, else the one the accounts record keeps it in, "" where none
// says.
func (m *membership) poolIn(pools *poolTable, id string) string {
	if pool, ok := pools.lastIn(id); ok {
		return pool
	}
	return m.recorded(id)
}

// unpooled returns the membership of a router that routes within no pools,
// and knows of no first pool: every account in the one pool there is, as a
// playback of the history has it.
func unpooled() *membership {
	return newMembership(noFirst, sameID, noPoolKnown)
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
// before the config, so a config that changes finds it there. The pool each
// account that leaves them was in is kept, as after says.
func (m *membership) follow(accounts config.Accounts, pools config.Pools) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.followed && slices.Equal(accounts, m.accounts) && identicalPools(pools, m.pools) {
		return
	}
	was := m.table.Swap(m.made(accounts, pools).after(m.current()))
	close(was.replaced)
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
		m.pooled, m.warned = newPoolTable(accounts, pools, m.first().ID), false
		return m.pooled
	}
	kept := m.kept(accounts)
	if kept == nil {
		return newPoolTable(accounts, nil, "")
	}
	if !m.warned {
		m.warned = true
		logger.Warn("the config names no pools; the pools stand as they were till the router restarts and gives the config its pools again")
	}
	return kept
}

// kept returns the pools a config of accounts naming none keeps once the
// move to pools is done, as far as the router can tell, nil before: those the
// last config with pools it followed made, but for accounts that have left
// the config, in none at once, as keeping says; else, where the accounts
// record notes a first pool, as for a router started on such a config, that
// pool alone with every account in it, as healedTable says. m.mu must be
// held.
func (m *membership) kept(accounts config.Accounts) *poolTable {
	if m.pooled != nil {
		return m.pooled.keeping(accounts, m.idNow)
	}
	if first := m.first(); first.ID != "" {
		return healedTable(accounts, first, m.idNow)
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

// firstIn returns what reads the first pool from the accounts record in the
// state directory given, as its pool marked First, by its id and its label:
// zero where it marks none, has no state directory, or can't be read, which
// it logs at debug, the config's first pool standing in.
func firstIn(stateDir string) func() config.Pool {
	if stateDir == "" {
		return noFirst
	}
	record := identity.NewFile(stateDir)
	return func() config.Pool {
		r, err := record.Read()
		if err != nil {
			logger.Debug("can't read the accounts record's first pool; the config's first stands in", "error", err)
			return config.Pool{}
		}
		i := slices.IndexFunc(r.Pools, func(p identity.PoolEntry) bool { return p.First })
		if i < 0 {
			return config.Pool{}
		}
		return config.Pool{ID: r.Pools[i].ID, Label: r.Pools[i].Label}
	}
}

// noFirst knows of no first pool.
func noFirst() config.Pool {
	return config.Pool{}
}

// noPoolKnown knows of no pool an account was in.
func noPoolKnown(string) string {
	return ""
}

// sameID knows of no id an account had before switchboard made ids: an
// account's id now is the one given.
func sameID(id string) string {
	return id
}
