package router

import (
	"cmp"
	"maps"
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
	// was holds, by id, the pool each account that has left the tables before
	// this one, while the router runs, was in last, as they had it: none of
	// those poolOf holds.
	was map[string]string
	// recorded holds, by id, the pool the accounts record keeps each account
	// in, as it read as the table was made, as recordedPools gives them: what
	// the tables can't know of an account removed before the router started.
	recorded map[string]string
	// primary holds, by the pool's id, the id of the account the config
	// makes each pool's primary, as poolOf spells the pool.
	primary map[string]string
	// closed, where it's set, is why no account is in any pool, so nothing
	// is routed: which pool each is in can't be told, as closedTable says.
	closed string
	// replaced is closed once membership has replaced the table with another,
	// for a reader held open to look at the pools again: membership gives
	// each table it stores one of its own, and a table it never stored has
	// none.
	replaced chan struct{}
}

// newPoolTable returns the pools accounts and pools make, as a config gives
// them, each account known by the id it has now, as idNow gives it, as a
// config put back from before switchboard made ids names it by the one it
// had; first being the id of the first pool, as the accounts record notes
// it: the pool a request that names none is in, where the config has it,
// else the config's first. A Mac that moved to pools before a config shared
// with another reached it keeps a first pool of its own that the config it
// shares has put aside, and its sessions from before pools go where that
// config put every account.
func newPoolTable(accounts config.Accounts, pools config.Pools, first string, idNow func(id string) string) *poolTable {
	t := &poolTable{
		pools: slices.Clone(pools), poolOf: make(map[string]string, len(accounts)), primary: make(map[string]string),
	}
	for _, a := range accounts {
		id := idNow(a.ID)
		t.poolOf[id] = a.Pool
		if a.Primary {
			t.primary[a.Pool] = id
		}
	}
	t.first = firstPool(pools, first)
	return t
}

// closedTable returns the table of a router that can't tell which pool any
// account is in, for the reason given: none is in any, so nothing goes out
// on any account, and every request is answered by switchboard, saying why.
func closedTable(why string) *poolTable {
	return &poolTable{poolOf: map[string]string{}, primary: map[string]string{}, closed: why}
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
	if !t.pooled() {
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
// names, so one that has left the config is in none; and none at all in a
// closed table.
func (t *poolTable) holds(pool, id string) bool {
	switch {
	case t.closed != "":
		return false
	case !t.pooled():
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

// pooled reports whether the table has pools: a config's that has them, or
// those kept of one that lost them, as membership's kept keeps them.
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

// poolIn returns the id of the pool the account with the given id is in, as
// lastIn has it, else the one the accounts record keeps it in, as recorded
// has it, "" where none says.
func (t *poolTable) poolIn(id string) string {
	if pool, ok := t.lastIn(id); ok {
		return pool
	}
	return t.recorded[id]
}

// after returns the table as it follows before, knowing the pool each
// account that has left it was in last, as before had it, as was says, and
// the pool the accounts record keeps each in, as recorded gives it.
func (t *poolTable) after(before *poolTable, recorded map[string]string) *poolTable {
	next := *t
	next.recorded = recorded
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

// stored returns a copy of t for membership to store, with a replaced of its
// own, open.
func (t *poolTable) stored() *poolTable {
	fresh := *t
	fresh.replaced = make(chan struct{})
	return &fresh
}

// sameAs reports whether t routes and tells of the pools as u does: the same
// pools, as identicalPools says, each account in the same one, the same
// primaries and the same first, each account that has left them in the same
// one last, the record keeping each in the same one, and closed alike.
func (t *poolTable) sameAs(u *poolTable) bool {
	return identicalPools(t.pools, u.pools) && maps.Equal(t.poolOf, u.poolOf) && t.first == u.first &&
		maps.Equal(t.primary, u.primary) && maps.Equal(t.was, u.was) && maps.Equal(t.recorded, u.recorded) && t.closed == u.closed
}

// keeping returns the table with the accounts given in the pools they're in,
// any other in none, as it has left the config, and one given that the table
// doesn't have in none too, as which pool it's in can't be told: the pools,
// their primaries and the first stand. Each is known by the id it has now, as
// idNow gives it, as a config put back from before switchboard made ids
// names it by the one it had.
func (t *poolTable) keeping(accounts config.Accounts, idNow func(id string) string) *poolTable {
	kept := &poolTable{pools: t.pools, first: t.first, primary: t.primary, poolOf: make(map[string]string, len(accounts))}
	for _, a := range accounts {
		id := idNow(a.ID)
		if pool, ok := t.poolOf[id]; ok {
			kept.poolOf[id] = pool
		}
	}
	return kept
}

// keptTable returns the pools of accounts, from a config that names none
// once the move to pools has noted a first pool, till the config is given
// its pools again, as upkeep's heal gives them: those record keeps, as
// idmove's PoolsKept gives them, which the heal writes, each account by the
// id it has now, as idNow gives it, in the pool PoolsKept has it in, and one
// it has in none in none, never to serve; each pool's primary as the heal
// marks it, as idmove's PrimaryKept says. The pools are read for their ids,
// labels and members alone, their directories as written. It fails as
// PoolsKept does, as before the move.
func keptTable(record identity.Record, accounts config.Accounts, idNow func(id string) string) (*poolTable, error) {
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = idNow(a.ID)
	}
	kept, err := idmove.PoolsKept(record, ids)
	if err != nil {
		return nil, err
	}
	pools := make(config.Pools, len(kept.Pools))
	for i, p := range kept.Pools {
		pools[i] = config.Pool{ID: p.ID, Label: cmp.Or(p.Label, p.ID), WrittenDirs: p.Dirs}
	}
	first, _ := record.FirstPool()
	t := newPoolTable(nil, pools, first.ID, sameID)
	for i, id := range ids {
		pool := kept.In[id]
		if pool == "" {
			continue
		}
		t.poolOf[id] = pool
		if idmove.PrimaryKept(record, pool, id, accounts[i].Primary) {
			t.primary[pool] = id
		}
	}
	return t, nil
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
	// record reads the accounts record, for its first pool, the pools it
	// keeps of this Mac's config and the pool it keeps each account in,
	// reporting false where it can't; moved reports whether the move to pools
	// is done, as its mark says, whether the record can be read or not. idNow
	// gives the id an account a config names has now, as accounts' idNow
	// does.
	record func() (identity.Record, bool)
	moved  func() bool
	idNow  func(id string) string

	// mu is held as the pools are followed, and guards what they were last
	// followed from: accounts and pools, and followed, set once they have
	// been; told, the table last made from what tells the pools, a config
	// with pools or the accounts record, nil till one has been; and said,
	// what the log last said of a config naming no pools, "" since one with
	// pools.
	mu       sync.Mutex
	accounts config.Accounts
	pools    config.Pools
	followed bool
	told     *poolTable
	said     string
}

// newMembership returns the membership of a router that reads the accounts
// record as record does, tells the move to pools done as moved does, and the
// id an account a config names has now as idNow does, following no pools
// yet: every account in the one pool there is, as in a config from before
// pools.
func newMembership(record func() (identity.Record, bool), moved func() bool, idNow func(id string) string) *membership {
	m := &membership{record: record, moved: moved, idNow: idNow}
	m.table.Store(newPoolTable(nil, nil, "", idNow).stored())
	return m
}

// unpooled returns the membership of a router that routes within no pools,
// and has no accounts record: every account in the one pool there is, as a
// playback of the history has it.
func unpooled() *membership {
	return newMembership(noRecord, never, sameID)
}

// first returns the id of the first pool, as the accounts record notes it:
// "" where it notes none, or can't be read.
func (m *membership) first() string {
	r, _ := m.record()
	first, _ := r.FirstPool()
	return first.ID
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
	return m.current().poolIn(id)
}

// current returns the pools as they stand now.
func (m *membership) current() *poolTable {
	return m.table.Load()
}

// follow takes up the pools a config makes of the accounts and pools it
// configures, as made says: of one with pools, where they differ from those
// it last took up, which pool each account is in, the pools themselves with
// their directories, the accounts' primaries, and the first, read afresh
// from the accounts record; of one naming none, afresh each time, so a
// record that couldn't be read is taken up once it can. Each table made
// knows the pool each account that leaves it was in, as after says, and the
// pool the record keeps each in, read with it, so no request reads the
// record. The move to pools writes the first pool to the record before the
// config, so a config that changes finds it there.
func (m *membership) follow(accounts config.Accounts, pools config.Pools) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.followed && len(pools) > 0 && slices.Equal(accounts, m.accounts) && identicalPools(pools, m.pools) {
		return
	}
	record, readable := m.record()
	// A table that changes nothing leaves the current one standing, so its
	// readers aren't woken. One that does is stored as a copy with a replaced
	// of its own, so none is closed twice, whatever table made gives back.
	current := m.current()
	if made := m.made(record, readable, accounts, pools).after(current, recordedPools(record)); !made.sameAs(current) {
		close(m.table.Swap(made.stored()).replaced)
	}
	m.accounts, m.pools, m.followed = slices.Clone(accounts), slices.Clone(pools), true
}

// made returns the pools a config makes of the accounts and pools it
// configures, the accounts record read as record, readable saying whether it
// could be. A config with pools makes them. One with none, before the move
// to pools is done, makes none, every account in the one pool there is, as
// before pools; but once it's done, a config naming none has lost them, as
// one put back by hand or from a repository it's kept in, which upkeep's
// heal gives them back: meanwhile the pools stand as kept says, so no pool's
// work goes out on another's accounts, whether or not the heal can be
// written, and the log says how, once for each way they stand. m.mu must be
// held.
func (m *membership) made(record identity.Record, readable bool, accounts config.Accounts, pools config.Pools) *poolTable {
	if len(pools) > 0 {
		first, _ := record.FirstPool()
		m.told, m.said = newPoolTable(accounts, pools, first.ID, m.idNow), ""
		return m.told
	}
	kept, said := m.kept(record, readable, accounts)
	if kept == nil {
		return newPoolTable(accounts, nil, "", m.idNow)
	}
	if said != m.said {
		m.said = said
		logger.Warn(said)
	}
	return kept
}

// kept returns the pools a config of accounts naming none keeps once the
// move to pools is done, and what the log says of them: those the accounts
// record keeps, as keptTable gives them, the pools the heal writes back, so
// the router routes as the config will be, whether it started on the config
// or found it at a look; else, where the record, read as record, readable
// saying whether it could be, can't be read, or keeps no pool of this Mac's
// config, those the router routes within, as last told, but for accounts
// that have left the config, and those new to them, in none, as keeping
// says; else none at all, closed, nothing routed, as which pool each account
// is in can't be told. Before the move is done it's nil, and says nothing.
// m.mu must be held.
func (m *membership) kept(record identity.Record, readable bool, accounts config.Accounts) (*poolTable, string) {
	if !m.moved() {
		return nil, ""
	}
	const lost = "the config names no pools"
	why := "the accounts record can't be read"
	if readable {
		t, err := keptTable(record, accounts, m.idNow)
		if err == nil {
			m.told = t
			return t, lost + "; the router routes within those the accounts record keeps of this Mac's config till the config is given them again"
		}
		why = err.Error()
	}
	if m.told != nil {
		return m.told.keeping(accounts, m.idNow), lost + ", and " + why + "; the pools stand as the router last routed within them, " +
			"an account new to them in none, till the config is given them again"
	}
	closed := lost + " and " + why + ", so the pool each account is in can't be told"
	return closedTable(closed), closed + "; nothing is routed till that's put right"
}

// recordedPools returns, by id, the pool record keeps each account in, as
// this Mac's config last had it, kept once it's removed; and by the id it
// had before switchboard made ids, where it had one, for a router that
// started on those, so each account is known as the tables know it.
func recordedPools(record identity.Record) map[string]string {
	pools := make(map[string]string)
	for _, e := range record.Accounts {
		if e.Pool != "" && e.Was != "" {
			pools[e.Was] = e.Pool
		}
	}
	for _, e := range record.Accounts {
		if e.Pool != "" {
			pools[e.ID] = e.Pool
		}
	}
	return pools
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

// movedIn returns what reports whether the move to pools is done in the
// state directory given, as idmove's PoolsMoved says: never where the router
// has none.
func movedIn(stateDir string) func() bool {
	if stateDir == "" {
		return never
	}
	return func() bool { return idmove.PoolsMoved(stateDir) }
}

// noRecord has no accounts record to read.
func noRecord() (identity.Record, bool) {
	return identity.Record{}, false
}

// never has the move to pools never done, as for a router with no state
// directory.
func never() bool {
	return false
}

// sameID knows of no id an account had before switchboard made ids: an
// account's id now is the one given.
func sameID(id string) string {
	return id
}
