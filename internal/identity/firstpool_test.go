package identity_test

import (
	"slices"
	"strings"
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

// Noting a pool first marks the record's pool so, this Mac's and configured,
// or adds it so where the record hasn't it, as of a config another Mac gave
// its pools, keeping the label it's written with, where it's given one.
func TestNotingTheFirstPoolMarksItOrAddsIt(t *testing.T) {
	at := added.Add(time.Hour)
	everything := identity.PoolWritten{Label: "Everything", Primary: "b6s7", PrimaryKnown: true}
	tests := []struct {
		name    string
		pools   []identity.PoolEntry
		written identity.PoolWritten
		want    []identity.PoolEntry
	}{
		{
			name:    "one the record has, written as the config has it",
			pools:   []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: added}},
			written: everything,
			want:    []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: added, First: true, Here: true, Written: everything}},
		},
		{
			name:    "one the record keeps as only another Mac configures it",
			pools:   []identity.PoolEntry{{ID: "b2n8", Label: "Everything", Added: added}},
			written: everything,
			want:    []identity.PoolEntry{{ID: "b2n8", Added: added, First: true, Here: true, Written: everything}},
		},
		{
			name:    "one the record hasn't, made, written with its label, its primary not known",
			pools:   []identity.PoolEntry{{ID: "q7mt", Added: added}},
			written: identity.PoolWritten{Label: "Default"},
			want:    []identity.PoolEntry{{ID: "q7mt", Added: added}, {ID: "b2n8", Added: at, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := identity.Record{Pools: slices.Clone(tt.pools)}
			record.NoteFirst("b2n8", tt.written, at)
			if !slices.EqualFunc(record.Pools, tt.want, identity.PoolEntry.Equal) {
				t.Errorf("the pools are %+v, want %+v", record.Pools, tt.want)
			}
		})
	}
}

// Joining another Mac's record takes in its pools: matched by id, the
// record's own, keeping its times and removal, and its label but where
// another Mac configures it and this one neither configures, as its config
// or its record says, nor removed it; else added as the other Mac keeps it,
// its label and removal with it, labelled by its id where it's live and has
// none, but never as this Mac's, nor as its first.
func TestJoiningARecordTakesInItsPools(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	own := []identity.PoolEntry{
		{ID: "b2n8", Added: added, First: true, Here: true},
		{ID: "q7mt", Label: "Acme", Added: added, Removed: removed},
		{ID: "h3ll", Label: "Home", Added: added},
		{ID: "g0n3", Label: "g0n3", Added: added, Removed: removed},
		{ID: "c4nt", Added: added, Here: true},
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
			owned: []identity.PoolEntry{own[0], own[1], {ID: "h3ll", Label: "House", Added: added}, own[3], own[4]},
		},
		{
			name:  "one removed here named by its id alone, taking the label the other gives it",
			other: identity.PoolEntry{ID: "g0n3", Label: "Gone", Added: later, Removed: later},
			owned: []identity.PoolEntry{own[0], own[1], own[2], {ID: "g0n3", Label: "Gone", Added: added, Removed: removed}, own[4]},
		},
		{
			name:  "this Mac's, as its record says, its config not read, taking no label",
			other: identity.PoolEntry{ID: "c4nt", Label: "Client there", Added: later, Configured: true},
			owned: own,
		},
		{
			name:  "one configured here the record lacks, added with its time alone, this Mac's only once kept up",
			other: identity.PoolEntry{ID: "w9rk", Label: "Office", Added: later, Configured: true},
			owned: own,
			want:  []identity.PoolEntry{{ID: "w9rk", Added: later}},
		},
		{
			name:  "one live there, given no label, added labelled by its id, never this Mac's",
			other: identity.PoolEntry{ID: "n3wp", Added: later, Here: true, Written: identity.PoolWritten{Dirs: []string{"~/Code/new"}}},
			owned: own,
			want:  []identity.PoolEntry{{ID: "n3wp", Label: "n3wp", Added: later}},
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
			if want := slices.Concat(tt.owned, tt.want); !slices.EqualFunc(record.Pools, want, identity.PoolEntry.Equal) {
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

// Joining another Mac's record takes each of its pools in as the pool here
// holding the accounts its config puts in it, each by its id here, those not
// configured here passed over, so a Mac taking over from another reads the
// history in its own pools: that pool not added to the record. Where none
// holds them, they're in several here, or its config isn't known, the pool
// is kept apart, added as the other Mac keeps it, and Join says why, where
// it's live there. One configured here, or this Mac's own, is matched by its
// id, as ever.
func TestJoiningARecordTakesItsPoolsInAsThePoolHereHoldingTheirAccounts(t *testing.T) {
	own := identity.Record{
		Accounts: []identity.Entry{
			{ID: "b6s7", Organization: "test-org-work", Added: added},
			{ID: "qgr2", Organization: "test-org-personal", Added: added},
		},
		Pools: []identity.PoolEntry{{ID: "b2n8", Added: added, First: true, Here: true}, {ID: "w9rk", Added: added, Here: true}, {ID: "g0n3", Label: "Gone", Added: added, Removed: removed, Written: identity.PoolWritten{Label: "Gone"}}},
	}
	as := identity.JoinAs{
		Configured: config.Accounts{{ID: "b6s7", Label: "Work", Pool: "b2n8"}, {ID: "qgr2", Label: "Personal", Pool: "W9RK"}},
		Pools:      config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "w9rk", Label: "Office"}},
		From:       otherMac, Labels: true, TheirsKnown: true,
	}
	theirs := identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Organization: "test-org-work", Added: added, Configured: true},
		{ID: "z9k4", Organization: "test-org-personal", Added: added, Configured: true},
		{ID: "n0n3", Organization: "test-org-other", Added: added, Configured: true},
	}}
	tests := []struct {
		name   string
		theirs config.Accounts
		known  bool
		pool   identity.PoolEntry
		here   string
		apart  string
	}{
		{name: "holding an account of the same id", theirs: config.Accounts{{ID: "b6s7", Pool: "p1p1"}}, here: "b2n8"},
		{name: "holding one of the same subscription under another id", theirs: config.Accounts{{ID: "z9k4", Pool: "p1p1"}}, here: "W9RK"},
		{name: "holding one here and one not configured here", theirs: config.Accounts{{ID: "b6s7", Pool: "P1P1"}, {ID: "n0n3", Pool: "p1p1"}}, here: "b2n8"},
		{name: "holding accounts in two pools here", theirs: config.Accounts{{ID: "b6s7", Pool: "p1p1"}, {ID: "z9k4", Pool: "p1p1"}}, apart: identity.ApartSplit},
		{name: "holding none configured here", theirs: config.Accounts{{ID: "n0n3", Pool: "p1p1"}, {ID: "b6s7", Pool: "x2x2"}}, apart: identity.ApartNoneHere},
		{name: "its Mac's config not known", apart: identity.ApartNoConfig},
		{name: "removed there, its accounts gone, kept apart unsaid", pool: identity.PoolEntry{ID: "p1p1", Label: "Old", Added: added, Removed: removed}},
		{name: "configured here, matched by its id", theirs: config.Accounts{{ID: "qgr2", Pool: "b2n8"}}, pool: identity.PoolEntry{ID: "b2n8", Label: "Default", Added: added, Configured: true}},
		{name: "this Mac's own, removed here, matched by its id", theirs: config.Accounts{{ID: "b6s7", Pool: "g0n3"}}, pool: identity.PoolEntry{ID: "g0n3", Label: "Gone", Added: added, Configured: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.pool
			if pool.ID == "" {
				pool = identity.PoolEntry{ID: "p1p1", Label: "Everything", Added: added, First: true, Configured: true}
			}
			other := theirs
			other.Pools = []identity.PoolEntry{pool}
			record := identity.Record{Accounts: slices.Clone(own.Accounts), Pools: slices.Clone(own.Pools)}
			joinAs := as
			joinAs.Theirs, joinAs.TheirsKnown = tt.theirs, tt.theirs != nil

			j, err := record.Join(other, joinAs)
			if err != nil {
				t.Fatalf("Join: %v", err)
			}
			if got := j.Pools[strings.ToLower(pool.ID)]; got != tt.here {
				t.Errorf("Join took the pool in as %q, want %q", got, tt.here)
			}
			var apart []identity.Apart
			if tt.apart != "" {
				apart = []identity.Apart{{Install: otherMac, Pool: "p1p1", Label: "Everything", Why: tt.apart}}
			}
			if !slices.Equal(j.Apart, apart) {
				t.Errorf("Join kept apart %+v, want %+v", j.Apart, apart)
			}
			_, added := record.PoolByID(pool.ID)
			_, wasOwn := own.PoolByID(pool.ID)
			if keptApart := tt.here == ""; added != keptApart && !wasOwn {
				t.Errorf("the record holds the pool: %v, want it held only where it's kept apart: %v", added, keptApart)
			}
		})
	}
}
