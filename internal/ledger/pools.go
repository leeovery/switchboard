package ledger

import (
	"cmp"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// poolsVersion is the summary version that split each account's day's work
// by pool: a summary before it holds all its work in none.
const poolsVersion = 7

// FirstPool has the ledger, or a follower of it, know the first pool, the
// one the history from before pools was moved to, as first gives its id:
// "" before the move to pools is done. Work in no pool once it is, as a
// release before pools files, or a router draining as another takes over,
// or a pin filed before pins named their pool, is the first pool's, as
// fitted has it. first is asked only of a day with work in none, and, as a
// day's digest is built, of one holding a session split between pools some
// of whose lines named none.
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
// given id, ids compared as config.SamePool compares them.
func (a AccountDay) worksIn(pool string) bool {
	return slices.ContainsFunc(a.poolDays(), func(p PoolDay) bool { return config.SamePool(p.Pool, pool) })
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

// RecordingPool is pools, the pools some lines recorded, each once, in the
// order of their ids, "" of those that named none, with the pool a line
// recorded taken in: pools itself where it holds it already.
func RecordingPool(pools []string, pool string) []string {
	if i, held := slices.BinarySearch(pools, pool); !held {
		pools = slices.Insert(pools, i, pool)
	}
	return pools
}

// StoredPools are pools, as RecordingPool keeps them, as what's built of
// lines stores them, a digest's entry or a day's head: none where the lines
// named none, as before pools and in a config without them, so what's
// stored is as it was where no line names a pool.
func StoredPools(pools []string) []string {
	if slices.Equal(pools, []string{""}) {
		return nil
	}
	return pools
}

// RecordedPools are the pools StoredPools stores: none but none where it
// stores none.
func RecordedPools(stored []string) []string {
	if len(stored) == 0 {
		return []string{""}
	}
	return stored
}

// RecordedIn reports whether a request that recorded the pool with the id
// recorded, as a line records it at the time, "" where it named none, was
// routed in the pool with the given id, ids compared as config.SamePool
// compares them. first is the id of the first pool, once the move to pools
// is done, "" till then: a request naming no pool is the first's, as
// FirstPool says, and before, the pool of none. It's the one rule of which
// pool a request is of, whether read of its line or of what's built of it.
func RecordedIn(recorded, pool, first string) bool {
	return config.SamePool(cmp.Or(recorded, first), pool)
}

// RoutedIn reports whether a request that recorded the pool given, as a line
// records it at the time, "" where it named none, was routed in the pool a
// reader of sessions reads, or a builder folds them in, as RecordedIn has it
// of that pool and the first: nil of every pool's.
type RoutedIn func(recorded string) bool

// routedIn is what reports whether a request was routed in the pool with
// the given id, as RecordedIn has it of first, the first pool's id.
func routedIn(pool, first string) RoutedIn {
	return func(recorded string) bool { return RecordedIn(recorded, pool, first) }
}

// poolsApart are pools, as RecordingPool keeps them, apart by the pool each
// was routed in, as RecordedIn has it of first, the first pool's id: each
// pool's in their order, the pools in the order of the first of each, one
// where they're all one pool's, as a first pool's id and none are once the
// first is known.
func poolsApart(pools []string, first string) [][]string {
	var apart [][]string
	for _, pool := range pools {
		i := slices.IndexFunc(apart, func(of []string) bool { return RecordedIn(pool, poolOf(of, first), first) })
		if i < 0 {
			apart = append(apart, []string{pool})
			continue
		}
		apart[i] = append(apart[i], pool)
	}
	return apart
}

// poolOf is the id of the pool requests that recorded pools, one pool's
// apart, as poolsApart has them, were routed in, as RecordedIn has it of
// first: the first's, of those that named none.
func poolOf(pools []string, first string) string {
	return cmp.Or(pools[0], first)
}

// entryState is a state a digest's entry holds of a session, as a reader in
// a pool rests on it, as DigestEntry's in gives it: entry, the entry as of
// the state, its RestsOn its own, its Pools, State and Part the state's;
// apart, which of the entry's states it is, as stateKey has it; and outside,
// where none of the lines the entry rests on was in the pool, so it holds no
// state of the pool's, the session's lines after it being all it holds of
// the pool.
type entryState struct {
	entry   DigestEntry
	apart   int
	outside bool
}

// in returns e's state in the pool in reads, as entryState has it: its own
// where every line it rests on was in's, as every one is where in is nil, of
// every pool's; none, outside, where none was; else its state apart whose
// pools are exactly those of e's that are in's, as the builder folded each
// from its pool's lines alone. It reports false where e holds no state of
// the pool's alone, as of a builder that placed lines naming none in another
// pool than the reader's first, or a split it folded none of apart.
func (e DigestEntry) in(in RoutedIn) (entryState, bool) {
	if in == nil {
		return entryState{entry: e}, true
	}
	kept := slices.DeleteFunc(slices.Clone(e.Pools), func(pool string) bool { return !in(pool) })
	switch {
	case len(kept) == 0:
		return entryState{outside: true}, true
	case len(kept) == len(e.Pools):
		return entryState{entry: e}, true
	}
	i := slices.IndexFunc(e.Apart, func(s PoolState) bool { return slices.Equal(s.Pools, kept) })
	if i < 0 {
		return entryState{}, false
	}
	s := e.Apart[i]
	return entryState{entry: DigestEntry{RestsOn: e.RestsOn, Pools: s.Pools, State: s.State, Part: s.Part}, apart: i + 1}, true
}

// InPool returns the day in the pool with the given id alone: each account's
// day as AccountDay.InPool gives it, first the first pool's id, and the day
// of the requests no account answered where some of them were in the pool.
// Its version, lines and bytes are the whole day's.
func (s Summary) InPool(pool, first string) Summary {
	var accounts []AccountDay
	for _, a := range s.Accounts {
		if in := a.InPool(pool, first); in.Account != "" || !in.Idle() {
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

// workIn returns the account's work in the pool with the given id, ids
// compared as config.SamePool compares them, the work of each of its pools
// the id names merged, as mergedWork merges it, as lines may record one
// pool's id in two cases: none where it did none there, its sessions named
// as its whole work names its.
func (a AccountDay) workIn(pool string) Work {
	if a.Pools == nil && config.SamePool(a.Pool, pool) {
		return a.Work
	}
	var in []Work
	for _, p := range a.Pools {
		if config.SamePool(p.Pool, pool) {
			in = append(in, p.Work)
		}
	}
	switch {
	case len(in) > 0:
		return mergedWorks(in)
	case a.SessionIDs == nil:
		return Work{}
	}
	return Work{SessionIDs: []string{}}
}

// mergedWorks merges works, one or more, as mergedWork merges two.
func mergedWorks(works []Work) Work {
	merged := works[0]
	for _, w := range works[1:] {
		merged = mergedWork(merged, w)
	}
	return merged
}

// poolDays returns the account's work by pool, in the order of their ids:
// its Pools, where it was in more than one; else its work, in its Pool; none
// where its work is none, as an account's the readings history alone tells
// of.
func (a AccountDay) poolDays() []PoolDay {
	switch {
	case a.Pools != nil:
		return a.Pools
	case a.Idle():
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

// Idle reports whether the work is none: no model's requests, no session and
// no move, as an account's day of no requests holds.
func (w Work) Idle() bool {
	return len(w.Models) == 0 && w.Sessions == 0 && w.MovedOn == 0 && w.MovedOff == 0
}
