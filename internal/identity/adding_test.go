package identity_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
)

// Adding an account comes to what the organization its token's answer named
// says: a new account, with the id it derives and the accounts whose
// organization isn't known that it may be; a removed account coming back,
// under its id; one configured already, refused, named; or, where none was
// named, the user's to say. The config says which accounts are configured,
// whatever the record has removed, and what the record lacks of them; where
// it can't be told whether a configured account is the token's, it's taken
// to be.
func TestAddingDecidesByTheOrganization(t *testing.T) {
	record := identity.Record{Accounts: []identity.Entry{
		{ID: "k3f9", Organization: "test-org-configured", Added: added},
		{ID: "m2x7", Organization: "test-org-removed", Label: "side", Added: added, Removed: removed},
		{ID: "r4n8", Organization: "test-org-unconfigured", Added: added},
		{ID: "w5t1", Organization: "test-org-readded", Label: "team", Added: added, Removed: removed},
		{ID: "pd2u", Organization: "test-org-team", Added: added},
		{ID: "h7c2", Label: "spare", Added: added, Removed: removed, Was: "spare"},
		{ID: "j9q4", Added: added},
		{ID: "c8v3", Added: added},
	}}
	configured := map[string]string{"k3f9": "", "w5t1": "", "pd2u": "test-org-team", "c8v3": ""}
	// unknown are the accounts a new account may be: those the record has
	// whose organization isn't known, but the one configured.
	unknown := []string{"h7c2", "j9q4"}
	tests := []struct {
		name         string
		configured   map[string]string
		organization string
		want         identity.Outcome
		wantID       string
		// wantEntry is the id of the record's account given: "" for none.
		wantEntry      string
		wantCandidates []string
	}{
		{name: "no organization named", configured: configured, want: identity.OrganizationUnknown},
		{
			name:           "one the record hasn't",
			configured:     configured,
			organization:   "test-org-work",
			want:           identity.NewAccount,
			wantID:         "b6s7",
			wantCandidates: unknown,
		},
		{
			name:           "one the record hasn't, deriving an id another account has",
			configured:     configured,
			organization:   "00000000-0000-4000-8000-000000000001",
			want:           identity.NewAccount,
			wantID:         "pd2uh",
			wantCandidates: unknown,
		},
		{
			name:           "one the record hasn't, deriving an id a configured account of another organization has",
			configured:     with(configured, "b6s7", "test-org-other"),
			organization:   "test-org-work",
			want:           identity.NewAccount,
			wantID:         "b6s7e",
			wantCandidates: unknown,
		},
		{
			name:         "one the record hasn't, deriving the id of a configured account whose organization isn't known",
			configured:   with(configured, "b6s7", ""),
			organization: "test-org-work",
			want:         identity.Configured,
			wantID:       "b6s7",
		},
		{
			name:         "the record empty, deriving the id of a configured account whose organization isn't known",
			configured:   map[string]string{"b6s7": ""},
			organization: "test-org-work",
			want:         identity.Configured,
			wantID:       "b6s7",
		},
		{
			name:         "a configured account's, as the config knows, the record not having it",
			configured:   with(configured, "z1y2", "test-org-work"),
			organization: "test-org-work",
			want:         identity.Configured,
			wantID:       "z1y2",
		},
		{
			name:         "a configured account's, as the config knows, the record having it under the account's id",
			configured:   configured,
			organization: "test-org-team",
			want:         identity.Configured,
			wantID:       "pd2u",
			wantEntry:    "pd2u",
		},
		{name: "a removed account's", configured: configured, organization: "test-org-removed", want: identity.Returning, wantID: "m2x7", wantEntry: "m2x7"},
		{
			name:         "one the config no longer has, the record not having it removed",
			configured:   configured,
			organization: "test-org-unconfigured",
			want:         identity.Returning,
			wantID:       "r4n8",
			wantEntry:    "r4n8",
		},
		{
			name:         "a configured account's, as the record knows",
			configured:   configured,
			organization: "test-org-configured",
			want:         identity.Configured,
			wantID:       "k3f9",
			wantEntry:    "k3f9",
		},
		{
			name:         "a configured account's, as the record knows, configured in another case",
			configured:   map[string]string{"K3F9": ""},
			organization: "test-org-configured",
			want:         identity.Configured,
			wantID:       "k3f9",
			wantEntry:    "k3f9",
		},
		{
			name:         "one the config has again, the record having it removed",
			configured:   configured,
			organization: "test-org-readded",
			want:         identity.Configured,
			wantID:       "w5t1",
			wantEntry:    "w5t1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := record.Adding(tt.configured, tt.organization)
			if got.Outcome != tt.want || got.ID != tt.wantID || got.Entry.ID != tt.wantEntry {
				t.Errorf("Adding(%q) = outcome %d, id %q, account %q; want outcome %d, id %q, account %q",
					tt.organization, got.Outcome, got.ID, got.Entry.ID, tt.want, tt.wantID, tt.wantEntry)
			}
			if candidates := ids(got.Candidates); !slices.Equal(candidates, tt.wantCandidates) {
				t.Errorf("Adding(%q) gives the candidates %q, want %q", tt.organization, candidates, tt.wantCandidates)
			}
		})
	}
}

// with is configured with the account of the given id, of organization.
func with(configured map[string]string, id, organization string) map[string]string {
	more := maps.Clone(configured)
	more[id] = organization
	return more
}
