package identity_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
)

// Adding an account comes to what the organization its token's answer named
// says: a new account, with the id it derives; a removed account coming back,
// under its id; one configured already, refused; or, where none was named,
// the user's to say. The config says which accounts are configured, whatever
// the record has removed.
func TestAddingDecidesByTheOrganization(t *testing.T) {
	record := identity.Record{Accounts: []identity.Entry{
		{ID: "k3f9", Organization: "test-org-configured", Added: added},
		{ID: "m2x7", Organization: "test-org-removed", Label: "side", Added: added, Removed: removed},
		{ID: "r4n8", Organization: "test-org-unconfigured", Added: added},
		{ID: "w5t1", Organization: "test-org-readded", Label: "team", Added: added, Removed: removed},
		{ID: "pd2u", Organization: "test-org-team", Added: added},
		{ID: "h7c2", Label: "spare", Added: added, Removed: removed},
	}}
	configured := []string{"k3f9", "w5t1", "pd2u"}
	tests := []struct {
		name         string
		organization string
		want         identity.Outcome
		wantID       string
		// wantEntry is the id of the record's account given: "" for none.
		wantEntry string
	}{
		{name: "no organization named", want: identity.OrganizationUnknown},
		{name: "one the record hasn't", organization: "test-org-work", want: identity.NewAccount, wantID: "b6s7"},
		{
			name:         "one the record hasn't, deriving an id another account has",
			organization: "00000000-0000-4000-8000-000000000001",
			want:         identity.NewAccount,
			wantID:       "pd2uh",
		},
		{name: "a removed account's", organization: "test-org-removed", want: identity.Returning, wantID: "m2x7", wantEntry: "m2x7"},
		{
			name:         "one the config no longer has, the record not having it removed",
			organization: "test-org-unconfigured",
			want:         identity.Returning,
			wantID:       "r4n8",
			wantEntry:    "r4n8",
		},
		{name: "a configured account's", organization: "test-org-configured", want: identity.Configured, wantID: "k3f9", wantEntry: "k3f9"},
		{
			name:         "one the config has again, the record having it removed",
			organization: "test-org-readded",
			want:         identity.Configured,
			wantID:       "w5t1",
			wantEntry:    "w5t1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := record.Adding(configured, tt.organization)
			if got.Outcome != tt.want || got.ID != tt.wantID || got.Entry.ID != tt.wantEntry {
				t.Errorf("Adding(%q) = outcome %d, id %q, account %q; want outcome %d, id %q, account %q",
					tt.organization, got.Outcome, got.ID, got.Entry.ID, tt.want, tt.wantID, tt.wantEntry)
			}
		})
	}
}
