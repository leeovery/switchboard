package views

import (
	"iter"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// readBack is a ledger that reads a session's lines back, newest first, as
// a follower's Session does, counting how often it's asked.
type readBack struct {
	LedgerRows
	newest []ledger.Held
	asked  int
}

func (b *readBack) Session(string, ...ledger.Span) iter.Seq[ledger.Held] {
	b.asked++
	return slices.Values(b.newest)
}

// heldIn is a line read back of the pool with the given id, "" naming none.
func heldIn(pool string) ledger.Held {
	return ledger.Held{Pool: pool}
}

// restingOn is a session resting on a state folded from the pools given, as
// a digest's entry stores them, none of its lines after it unless given.
func restingOn(pools []string, after ...string) ledger.SessionFrom {
	s := ledger.SessionFrom{Base: &ledger.SessionBase{Date: "2026-10-06", Pools: pools}}
	for _, pool := range after {
		s.Lines = append(s.Lines, ledger.Line{Pool: pool})
	}
	return s
}

// A session read on a digested day is in the pool the newest of its lines
// recorded, and split where its work is more than one pool's, told by the
// state it rests on beside its lines after it, work in none the first
// pool's: one resting on a state of both pools' work, and no lines after
// it, read back to its newest line, else, its lines pruned, the first pool;
// one resting on a state of one pool's work with lines of another after it,
// by its last; and one of none and the first pool's, or of one pool alone,
// in that pool, split in none, reading nothing back.
func TestASessionOnADigestedDayIsInThePoolItsNewestLineRecorded(t *testing.T) {
	const first = "b2n8"
	tests := []struct {
		name    string
		session ledger.SessionFrom
		newest  []ledger.Held
		pool    string
		split   bool
		asked   int
	}{
		{name: "both pools' state, read back", session: restingOn([]string{"b2n8", "q7mt"}), newest: []ledger.Held{heldIn("q7mt"), heldIn("b2n8")}, pool: "q7mt", split: true, asked: 1},
		{name: "both pools' state, read back to none's", session: restingOn([]string{"", "q7mt"}), newest: []ledger.Held{heldIn(""), heldIn("q7mt")}, pool: first, split: true, asked: 1},
		{name: "both pools' state, its lines pruned", session: restingOn([]string{"b2n8", "q7mt"}), pool: first, split: true, asked: 1},
		{name: "one pool's state, another's lines after", session: restingOn([]string{"b2n8"}, "q7mt", "q7mt"), pool: "q7mt", split: true},
		{name: "none's state, the first pool's lines after", session: restingOn(nil, "b2n8"), pool: first},
		{name: "one pool's state alone", session: restingOn([]string{"q7mt"}), pool: "q7mt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			back := &readBack{newest: tt.newest}
			read := SessionsRead{sessions: map[string]ledger.SessionFrom{"s": tt.session}}
			pool, split := read.PoolNow(back, "s", first)
			if pool != tt.pool || split != tt.split || back.asked != tt.asked {
				t.Errorf("PoolNow = %q, split %v, reading back %d times; want %q, split %v, reading back %d times", pool, split, back.asked, tt.pool, tt.split, tt.asked)
			}
		})
	}
}

// A session the read didn't read is in no pool, split in none.
func TestASessionNotReadIsInNoPool(t *testing.T) {
	back := &readBack{newest: []ledger.Held{heldIn("q7mt")}}
	if pool, split := (SessionsRead{}).PoolNow(back, "s", "b2n8"); pool != "" || split || back.asked != 0 {
		t.Errorf("PoolNow of a session not read = %q, split %v, reading back %d times; want none, unsplit, reading nothing", pool, split, back.asked)
	}
}
