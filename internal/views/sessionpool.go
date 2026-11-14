package views

import (
	"cmp"
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// PoolNow is the pool a session is in now, as the newest of its lines, held
// oldest first, recorded it, work in none the first pool's, first its id, as
// ledger.RecordedIn has it: what a verb given the session, and no pool, reads
// it in, so it never reads its work in every pool together. "" where held are
// none, or where they name none and first is none, as before the move to
// pools.
func PoolNow(held []ledger.Held, first string) string {
	if len(held) == 0 {
		return ""
	}
	return cmp.Or(held[len(held)-1].Pool, first)
}

// PoolNow is the pool the session with the given id, as r read it of every
// pool's work, is in now, as PoolNow has it of its newest line: the last of
// its lines after the state it rests on, else the newest l reads back,
// where it rests on a state of all its lines, else, of one whose lines are
// pruned, the first pool; and whether its work is split
// between pools, its state's and its lines', work in none the first pool's,
// first its id. A session whose work is all one pool's, or that r didn't
// read, is split in none, and is read in its pool as every pool's read reads
// it, so it reads no pool to be read in again.
func (r SessionsRead) PoolNow(l LedgerSession, id, first string) (pool string, split bool) {
	s, ok := r.sessions[id]
	if !ok || !splitIn(s, first) {
		return "", false
	}
	if len(s.Lines) > 0 {
		return cmp.Or(s.Lines[len(s.Lines)-1].Pool, first), true
	}
	for h := range l.Session(id) {
		return cmp.Or(h.Pool, first), true
	}
	return first, true
}

// splitIn reports whether the session s's work is split between pools: the
// pools the state it rests on was folded from, and those its lines after it
// recorded, more than one pool's, work in none the first pool's, first its
// id, as ledger.RecordedIn has it.
func splitIn(s ledger.SessionFrom, first string) bool {
	var recorded []string
	if s.Base != nil {
		recorded = ledger.RecordedPools(s.Base.Pools)
	}
	for _, line := range s.Lines {
		recorded = append(recorded, line.Pool)
	}
	if len(recorded) == 0 {
		return false
	}
	one := cmp.Or(recorded[0], first)
	return slices.ContainsFunc(recorded[1:], func(pool string) bool { return !ledger.RecordedIn(pool, one, first) })
}
