package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// PoolNow is the pool a session is in now, as the newest of its lines, held
// oldest first, recorded it, as ledger.PoolRecorded has it of first, the
// first pool's id: what a verb given the session, and no pool, reads it in,
// so it never reads its work in every pool together. "" where held are
// none, or where they name none and first is none, as before the move to
// pools.
func PoolNow(held []ledger.Held, first string) string {
	if len(held) == 0 {
		return ""
	}
	return ledger.PoolRecorded(held[len(held)-1].Pool, first)
}

// PoolNow is the pool the session with the given id, as r read it of every
// pool's work, is in now, and whether its work is split between pools, as
// poolsOf has them of first, the first pool's id. Of a split session, it's
// the pool the newest of its lines recorded, as PoolNow has it: the last of
// its lines after the state it rests on, else the newest l reads back,
// where it rests on a state of all its lines, else, of one whose lines are
// pruned, the first pool. A session whose work is all one pool's is in that
// pool, and is read in it as every pool's read reads it, so it reads no
// pool to be read in again; one r didn't read is in none.
func (r SessionsRead) PoolNow(l LedgerSession, id, first string) (pool string, split bool) {
	s, ok := r.sessions[id]
	if !ok {
		return "", false
	}
	if pool, split = poolsOf(s, first); !split {
		return pool, false
	}
	if len(s.Lines) > 0 {
		return ledger.PoolRecorded(s.Lines[len(s.Lines)-1].Pool, first), true
	}
	for h := range l.Session(id) {
		return ledger.PoolRecorded(h.Pool, first), true
	}
	return first, true
}

// poolsOf is the pool the session s's work was first routed in, of the
// pools the state it rests on was folded from and those its lines after it
// recorded, as ledger.PoolRecorded has it of first, the first pool's id: ""
// where it has neither. And it reports whether its work is split between
// pools: more than one pool's of those, as ledger.RecordedIn has it.
func poolsOf(s ledger.SessionFrom, first string) (pool string, split bool) {
	var recorded []string
	if s.Base != nil {
		recorded = ledger.RecordedPools(s.Base.Pools)
	}
	for _, line := range s.Lines {
		recorded = append(recorded, line.Pool)
	}
	if len(recorded) == 0 {
		return "", false
	}
	one := ledger.PoolRecorded(recorded[0], first)
	return one, slices.ContainsFunc(recorded[1:], func(pool string) bool { return !ledger.RecordedIn(pool, one, first) })
}
