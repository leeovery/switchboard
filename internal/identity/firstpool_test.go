package identity_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

func TestTheFirstPoolIsTheOneTheRecordNotesFirst(t *testing.T) {
	record := identity.Record{Pools: []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: added, First: true}}}
	if got, ok := record.FirstPool(); !ok || got.ID != "b2n8" {
		t.Errorf("FirstPool() = %+v, %v; want b2n8", got, ok)
	}
	if got, ok := (identity.Record{Pools: record.Pools[:1]}).FirstPool(); ok {
		t.Errorf("FirstPool() of a record noting none first = %+v, want none", got)
	}
}

// Noting a pool first marks the record's pool so, or adds it, configured,
// where the record hasn't it, as of a config another Mac gave its pools.
func TestNotingTheFirstPoolMarksItOrAddsIt(t *testing.T) {
	at := added.Add(time.Hour)
	tests := []struct {
		name  string
		pools []identity.PoolEntry
		want  []identity.PoolEntry
	}{
		{
			name:  "one the record has",
			pools: []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: added}},
			want:  []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: added, First: true}},
		},
		{
			name:  "one the record hasn't",
			pools: []identity.PoolEntry{{ID: "q7mt", Added: added}},
			want:  []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: at, First: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := identity.Record{Pools: slices.Clone(tt.pools)}
			record.NoteFirst("b2n8", at)
			if !slices.Equal(record.Pools, tt.want) {
				t.Errorf("the pools are %+v, want %+v", record.Pools, tt.want)
			}
		})
	}
}

// Joining another Mac's record takes in its pools: matched by id, the
// record's own, keeping its times and removal, and its label but where
// another Mac configures it and this one neither configures nor removed it;
// else added as the other Mac keeps it, its label and removal with it, but
// never as this Mac's first.
func TestJoiningARecordTakesInItsPools(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	own := []identity.PoolEntry{
		{ID: "b2n8", Added: added, First: true},
		{ID: "q7mt", Label: "Acme", Added: added, Removed: removed},
		{ID: "h3ll", Label: "Home", Added: added},
		{ID: "g0n3", Label: "g0n3", Added: added, Removed: removed},
	}
	poolsHere := config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "w9rk", Label: "Work"}}
	tests := []struct {
		name  string
		other identity.PoolEntry
		// want is the record's pools joined, those after own's four.
		want  []identity.PoolEntry
		owned []identity.PoolEntry
	}{
		{
			name:  "this Mac's first, configured here, matched by its id, as it was",
			other: identity.PoolEntry{ID: "b2n8", Label: "Everything", Added: later, Configured: true},
			owned: own,
		},
		{
			name:  "one removed here, matched by its id, keeping its label and removal",
			other: identity.PoolEntry{ID: "q7mt", Label: "Acme Inc", Added: later, Configured: true},
			owned: own,
		},
		{
			name:  "one only another Mac configures, taking the label it's configured by there now",
			other: identity.PoolEntry{ID: "h3ll", Label: "House", Added: later, Configured: true},
			owned: []identity.PoolEntry{own[0], own[1], {ID: "h3ll", Label: "House", Added: added}, own[3]},
		},
		{
			name:  "one removed here named by its id alone, taking the label the other gives it",
			other: identity.PoolEntry{ID: "g0n3", Label: "Gone", Added: later, Removed: later},
			owned: []identity.PoolEntry{own[0], own[1], own[2], {ID: "g0n3", Label: "Gone", Added: added, Removed: removed}},
		},
		{
			name:  "one configured here the record lacks, added with its time alone",
			other: identity.PoolEntry{ID: "w9rk", Label: "Office", Added: later, Configured: true},
			owned: own,
			want:  []identity.PoolEntry{{ID: "w9rk", Added: later}},
		},
		{
			name:  "the other Mac's first, added as it keeps it, never this Mac's first",
			other: identity.PoolEntry{ID: "z4z4", Label: "Default", Added: later, First: true, Configured: true},
			owned: own,
			want:  []identity.PoolEntry{{ID: "z4z4", Label: "Default", Added: later}},
		},
		{
			name:  "one removed there, added with its label and removal",
			other: identity.PoolEntry{ID: "c1nt", Label: "Client", Added: added, Removed: later},
			owned: own,
			want:  []identity.PoolEntry{{ID: "c1nt", Label: "Client", Added: added, Removed: later}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := identity.Record{Pools: slices.Clone(own)}
			as := identity.JoinAs{Pools: poolsHere, From: otherMac, Labels: true}
			if _, err := record.Join(identity.Record{Pools: []identity.PoolEntry{tt.other}}, as); err != nil {
				t.Fatalf("Join: %v", err)
			}
			if want := slices.Concat(tt.owned, tt.want); !slices.Equal(record.Pools, want) {
				t.Errorf("the pools joined are\n%+v\nwant\n%+v", record.Pools, want)
			}
		})
	}
}

// Joining this Mac's own record, as its own export carries it, takes no
// pool's label, as it may be older than one another Mac gave since.
func TestJoiningThisMacsOwnRecordTakesNoPoolsLabel(t *testing.T) {
	record := identity.Record{Pools: []identity.PoolEntry{{ID: "h3ll", Label: "Home", Added: added}}}
	other := identity.Record{Pools: []identity.PoolEntry{{ID: "h3ll", Label: "Old home", Added: added, Configured: true}}}
	if _, err := record.Join(other, identity.JoinAs{From: otherMac, Own: true}); err != nil {
		t.Fatalf("Join: %v", err)
	}
	if got := record.Pools[0].Label; got != "Home" {
		t.Errorf("the pool's label is %q, want this Mac's, Home", got)
	}
}
