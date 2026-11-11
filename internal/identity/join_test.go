package identity_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
)

// Joining another Mac's record matches each of its accounts to this one's by
// id, organization or the id it had before, taking what this one doesn't
// know of it and keeping what it does, and adds those it has none of.
func TestJoiningARecordTakesInItsAccounts(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	own := func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{
			{ID: "b6s7", Added: added},
			{ID: "qgr2", Organization: "test-org-personal", Added: added, Was: "personal"},
			{ID: "m2x7", Label: "side", Added: added, Removed: removed},
		}}
	}
	tests := []struct {
		name  string
		other []identity.Entry
		want  []identity.Entry
	}{
		{
			name:  "nothing",
			other: nil,
			want:  own().Accounts,
		},
		{
			name: "an account matched by its id, its organization and id before taken",
			other: []identity.Entry{
				{ID: "b6s7", Organization: "test-org-work", Label: "work", Added: later, Removed: later, Was: "work"},
			},
			want: []identity.Entry{
				{ID: "b6s7", Organization: "test-org-work", Added: added, Was: "work"},
				own().Accounts[1], own().Accounts[2],
			},
		},
		{
			name:  "an account matched by its organization, as it was",
			other: []identity.Entry{{ID: "qgr2", Organization: "test-org-personal", Added: later}},
			want:  own().Accounts,
		},
		{
			name:  "an account matched by its id before, in another case",
			other: []identity.Entry{{ID: "qgr2", Added: later, Was: "Personal"}},
			want:  own().Accounts,
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := own()
			if err := record.Join(identity.Record{Accounts: tt.other}); err != nil {
				t.Fatalf("Join: %v", err)
			}
			if !slices.Equal(record.Accounts, tt.want) {
				t.Errorf("joined\n%+v\nwant\n%+v", record.Accounts, tt.want)
			}
		})
	}
}

// A record that disagrees with this one is refused, changing nothing: an
// account of this one's under another id, of another organization, or having
// had another id before; one matching two of this one's; and one that, joined,
// breaks the record's rules, having had one of this one's ids before.
func TestJoiningARecordThatDisagreesIsRefused(t *testing.T) {
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
		{name: "its organization under another id", other: identity.Entry{ID: "k3f9", Organization: "test-org-work", Added: added}},
		{name: "its id before under another id", other: identity.Entry{ID: "k3f9", Added: added, Was: "side"}},
		{name: "its id of another organization", other: identity.Entry{ID: "qgr2", Organization: "test-org-other", Added: added}},
		{name: "its id, having had another before", other: identity.Entry{ID: "b6s7", Added: added, Was: "other"}},
		{name: "two of its accounts", other: identity.Entry{ID: "b6s7", Organization: "test-org-personal", Added: added}},
		{name: "one of its ids, as an id had before", other: identity.Entry{ID: "zzzz", Added: added, Was: "qgr2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := own()
			err := record.Join(identity.Record{Accounts: []identity.Entry{tt.other}})
			if !errors.Is(err, identity.ErrConflict) {
				t.Errorf("Join = %v, want ErrConflict", err)
			}
			if !slices.Equal(record.Accounts, own().Accounts) {
				t.Errorf("refused, the record changed to\n%+v", record.Accounts)
			}
		})
	}
}
