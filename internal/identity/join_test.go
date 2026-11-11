package identity_test

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
)

// Joining another Mac's record matches each of its accounts to this one's by
// id, organization or the id it had before, where no organizations known
// differ, taking what this one doesn't know of it and keeping what it does,
// and mapping its id onto this one's where they differ; and adds those it has
// none of, labels kept.
func TestJoiningARecordTakesInItsAccounts(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	own := func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{
			{ID: "b6s7", Added: added},
			{ID: "qgr2", Organization: "test-org-personal", Added: added, Was: "personal"},
			{ID: "m2x7", Label: "side", Added: added, Removed: removed, Was: "side"},
		}}
	}
	tests := []struct {
		name   string
		other  []identity.Entry
		want   []identity.Entry
		mapped map[string]string
	}{
		{
			name: "nothing",
			want: own().Accounts,
		},
		{
			name: "an account matched by its id, its organization, id before and label taken",
			other: []identity.Entry{
				{ID: "b6s7", Organization: "test-org-work", Label: "work", Added: later, Was: "work"},
			},
			want: []identity.Entry{
				{ID: "b6s7", Organization: "test-org-work", Label: "work", Added: added, Was: "work"},
				own().Accounts[1], own().Accounts[2],
			},
		},
		{
			name:  "an account matched by its organization, as it was",
			other: []identity.Entry{{ID: "qgr2", Organization: "test-org-personal", Added: later}},
			want:  own().Accounts,
		},
		{
			name:   "a subscription that had another id before on the other Mac, mapped by its organization",
			other:  []identity.Entry{{ID: "z9k4", Organization: "test-org-personal", Added: later, Was: "home"}},
			want:   own().Accounts,
			mapped: map[string]string{"z9k4": "qgr2"},
		},
		{
			name:   "an account removed on both Macs, under another random id there, mapped by its id before",
			other:  []identity.Entry{{ID: "r7kq", Organization: "test-org-side", Label: "old side", Added: later, Removed: later, Was: "Side"}},
			want:   []identity.Entry{own().Accounts[0], own().Accounts[1], {ID: "m2x7", Organization: "test-org-side", Label: "side", Added: added, Removed: removed, Was: "side"}},
			mapped: map[string]string{"r7kq": "m2x7"},
		},
		{
			name: "an account this one has none of, added as it is",
			other: []identity.Entry{
				{ID: "h7c2", Label: "team", Added: added, Removed: later, Was: "team"},
			},
			want: slices.Concat(own().Accounts, []identity.Entry{
				{ID: "h7c2", Label: "team", Added: added, Removed: later, Was: "team"},
			}),
		},
		{
			name:  "an account only the other Mac configures, added with its label",
			other: []identity.Entry{{ID: "h7c2", Organization: "test-org-team", Label: "team", Added: added}},
			want:  slices.Concat(own().Accounts, []identity.Entry{{ID: "h7c2", Organization: "test-org-team", Label: "team", Added: added}}),
		},
		{
			name:  "another subscription that once had the same id before, added without it",
			other: []identity.Entry{{ID: "k3f9", Organization: "test-org-other", Label: "personal", Added: added, Was: "personal"}},
			want:  slices.Concat(own().Accounts, []identity.Entry{{ID: "k3f9", Organization: "test-org-other", Label: "personal", Added: added}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := own()
			mapped, err := record.Join(identity.Record{Accounts: tt.other})
			if err != nil {
				t.Fatalf("Join: %v", err)
			}
			if !slices.Equal(record.Accounts, tt.want) {
				t.Errorf("joined\n%+v\nwant\n%+v", record.Accounts, tt.want)
			}
			if want := tt.mapped; !maps.Equal(mapped, want) {
				t.Errorf("Join mapped %v, want %v", mapped, want)
			}
		})
	}
}

// An account removed here that the other Mac's record keeps configured,
// labelled, keeps this Mac's label and removal.
func TestJoiningKeepsTheLabelOfAnAccountRemovedHere(t *testing.T) {
	record := identity.Record{Accounts: []identity.Entry{{ID: "m2x7", Label: "side", Added: added, Removed: removed}}}
	if _, err := record.Join(identity.Record{Accounts: []identity.Entry{{ID: "m2x7", Label: "Side Gig", Added: added}}}); err != nil {
		t.Fatal(err)
	}
	if got := record.Accounts[0]; got.Label != "side" || !got.Removed.Equal(removed) {
		t.Errorf("joined, the account removed here is %+v, want its label and removal as they were", got)
	}
}

// A record that contradicts this one is refused, changing nothing: an account
// matching two of this one's, one with the id of one of another
// organization, and one that, joined, breaks the record's rules, having had
// one of this one's ids before.
func TestJoiningARecordThatContradictsItIsRefused(t *testing.T) {
	own := func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{
			{ID: "b6s7", Organization: "test-org-work", Added: added, Was: "work"},
			{ID: "qgr2", Organization: "test-org-personal", Added: added},
			{ID: "m2x7", Label: "side", Added: added, Removed: removed, Was: "side"},
		}}
	}
	tests := []struct {
		name  string
		other identity.Entry
	}{
		{name: "two of its accounts", other: identity.Entry{ID: "b6s7", Organization: "test-org-personal", Added: added}},
		{name: "two of its accounts, by organization and id before", other: identity.Entry{ID: "k3f9", Organization: "test-org-work", Added: added, Was: "side"}},
		{name: "its id of another organization", other: identity.Entry{ID: "qgr2", Organization: "test-org-other", Added: added}},
		{name: "one of its ids, as an id had before", other: identity.Entry{ID: "zzzz", Added: added, Was: "qgr2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := own()
			_, err := record.Join(identity.Record{Accounts: []identity.Entry{tt.other}})
			if !errors.Is(err, identity.ErrConflict) {
				t.Errorf("Join = %v, want ErrConflict", err)
			}
			if !slices.Equal(record.Accounts, own().Accounts) {
				t.Errorf("refused, the record changed to\n%+v", record.Accounts)
			}
		})
	}
}
