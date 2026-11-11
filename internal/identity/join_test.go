package identity_test

import (
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// joinRandom is what the join tests draw random ids from.
func joinRandom() *rand.ChaCha8 {
	return rand.NewChaCha8([32]byte{7})
}

// thisMacs is the record joined into in the join tests: work, configured
// here, its organization unknown; personal, configured here; side, removed
// here; and home, only another Mac configures, as an import took it in.
func thisMacs() identity.Record {
	return identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Added: added, Was: "work"},
		{ID: "qgr2", Organization: "test-org-personal", Added: added, Was: "personal"},
		{ID: "m2x7", Label: "side", Added: added, Removed: removed, Was: "side"},
		{ID: "p4m2", Organization: "test-org-home", Label: "home", Added: added},
	}}
}

// configuredHere are the accounts this Mac configures in the join tests.
var configuredHere = config.Accounts{{ID: "B6S7", Label: "Work"}, {ID: "qgr2", Label: "Personal"}}

// Joining another Mac's record matches each of its accounts to this one's by
// organization, else by id where no organizations known differ, mapping its
// id onto this one's where they differ: the account takes its organization
// where it has none, and its label only where it's neither configured nor
// removed here. The ids accounts had before are each Mac's own names, so none
// is matched on or taken. One matching none is added as it is.
func TestJoiningARecordTakesInItsAccounts(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	own := thisMacs().Accounts
	tests := []struct {
		name   string
		other  identity.Entry
		want   []identity.Entry
		mapped map[string]string
	}{
		{
			name:  "a configured account matched by its id, taking its organization, keeping its label",
			other: identity.Entry{ID: "b6s7", Organization: "test-org-work", Label: "Office", Added: later, Was: "office"},
			want:  []identity.Entry{{ID: "b6s7", Organization: "test-org-work", Added: added, Was: "work"}, own[1], own[2], own[3]},
		},
		{
			name:   "a configured account under another id there, mapped by its organization",
			other:  identity.Entry{ID: "z9k4", Organization: "test-org-personal", Label: "Home office", Added: later, Was: "home"},
			want:   own,
			mapped: map[string]string{"z9k4": "qgr2"},
		},
		{
			name:  "an account removed here keeps its label and removal",
			other: identity.Entry{ID: "m2x7", Label: "Side gig", Added: later},
			want:  own,
		},
		{
			name:  "an account only another Mac configures takes the label it has there now",
			other: identity.Entry{ID: "p4m2", Organization: "test-org-home", Label: "family", Added: later},
			want:  []identity.Entry{own[0], own[1], own[2], {ID: "p4m2", Organization: "test-org-home", Label: "family", Added: added}},
		},
		{
			name:  "an account removed there under another random id, its id before the same as one here, added as another",
			other: identity.Entry{ID: "r7kq", Label: "side", Added: later, Removed: later, Was: "side"},
			want:  slices.Concat(own, []identity.Entry{{ID: "r7kq", Label: "side", Added: later, Removed: later}}),
		},
		{
			name:  "an account only the other Mac configures, added with its label",
			other: identity.Entry{ID: "h7c2", Organization: "test-org-team", Label: "team", Added: later, Was: "team"},
			want:  slices.Concat(own, []identity.Entry{{ID: "h7c2", Organization: "test-org-team", Label: "team", Added: later}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := thisMacs()
			mapped, err := record.Join(identity.Record{Accounts: []identity.Entry{tt.other}}, configuredHere, joinRandom())
			if err != nil {
				t.Fatalf("Join: %v", err)
			}
			if !slices.Equal(record.Accounts, tt.want) {
				t.Errorf("joined\n%+v\nwant\n%+v", record.Accounts, tt.want)
			}
			if want := cmpMap(tt.mapped); !maps.Equal(mapped, want) {
				t.Errorf("Join mapped %v, want %v", mapped, want)
			}
		})
	}
}

// cmpMap is m, an empty map where it's nil.
func cmpMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// An account whose id is held here already, by an account of another
// organization or as an id one had before, is added under a random id, which
// none here or in the other record holds, mapped onto it.
func TestJoiningAnAccountWhoseIDIsHeldGivesItAnother(t *testing.T) {
	tests := []struct {
		name  string
		other identity.Entry
	}{
		{name: "the id of an account of another organization", other: identity.Entry{ID: "qgr2", Organization: "test-org-other", Label: "other", Added: added}},
		{name: "an id an account had before", other: identity.Entry{ID: "side", Label: "spare", Added: added}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := thisMacs()
			others := identity.Record{Accounts: []identity.Entry{tt.other, {ID: "k3f9", Label: "spare", Added: added}}}
			mapped, err := record.Join(others, configuredHere, joinRandom())
			if err != nil {
				t.Fatalf("Join: %v", err)
			}
			id := mapped[strings.ToLower(tt.other.ID)]
			if !identity.WellFormed(id) || slices.Contains([]string{"b6s7", "qgr2", "m2x7", "p4m2", "k3f9", "side", "work", "personal"}, id) {
				t.Fatalf("Join mapped %s to %q, want an id none holds", tt.other.ID, id)
			}
			if e, ok := record.ByID(id); !ok || e.Label != tt.other.Label || e.Organization != tt.other.Organization {
				t.Errorf("joined, %s is %+v, want the other Mac's account under it", id, e)
			}
			if len(record.Accounts) != len(thisMacs().Accounts)+2 {
				t.Errorf("joined, the record is %+v, want both accounts added", record.Accounts)
			}
		})
	}
}
