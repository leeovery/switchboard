package identity_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
)

// Adding an account comes to what the organization its token's answer named
// says: a new account, with the id it derives and the accounts whose
// organization isn't known that it may be; a removed account coming back,
// under its id; one configured already, refused, named; or, where none was
// named, the user's to say. The config says which accounts are configured,
// whatever the record has removed.
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
		want         want
	}{
		{name: "no organization named", configured: configured, want: want{outcome: identity.OrganizationUnknown}},
		{
			name:         "one the record hasn't",
			configured:   configured,
			organization: "test-org-work",
			want:         want{outcome: identity.NewAccount, id: "b6s7", candidates: unknown},
		},
		{
			name:         "one the record hasn't, deriving an id another account has",
			configured:   configured,
			organization: "00000000-0000-4000-8000-000000000001",
			want:         want{outcome: identity.NewAccount, id: "pd2uh", candidates: unknown},
		},
		{
			name:         "a configured account's, as the config knows, the record not having it",
			configured:   map[string]string{"k3f9": "", "z1y2": "test-org-work"},
			organization: "test-org-work",
			want:         want{outcome: identity.Configured, id: "z1y2"},
		},
		{
			name:         "a configured account's, as the config knows, the record having it under the account's id",
			configured:   configured,
			organization: "test-org-team",
			want:         want{outcome: identity.Configured, id: "pd2u", entry: "pd2u"},
		},
		{
			name:         "a configured account's, as the record knows",
			configured:   configured,
			organization: "test-org-configured",
			want:         want{outcome: identity.Configured, id: "k3f9", entry: "k3f9"},
		},
		{
			name:         "a configured account's, as the record knows, the config saying another's",
			configured:   map[string]string{"k3f9": "test-org-other"},
			organization: "test-org-configured",
			want:         want{outcome: identity.Configured, id: "k3f9", entry: "k3f9"},
		},
		{
			name:         "a configured account's, as the record knows, configured in another case",
			configured:   map[string]string{"K3F9": ""},
			organization: "test-org-configured",
			want:         want{outcome: identity.Configured, id: "K3F9", entry: "k3f9"},
		},
		{
			name:         "one the config has again, the record having it removed",
			configured:   configured,
			organization: "test-org-readded",
			want:         want{outcome: identity.Configured, id: "w5t1", entry: "w5t1"},
		},
		{
			name:         "a removed account's",
			configured:   configured,
			organization: "test-org-removed",
			want:         want{outcome: identity.Returning, id: "m2x7", entry: "m2x7"},
		},
		{
			name:         "one the config no longer has, the record not having it removed",
			configured:   configured,
			organization: "test-org-unconfigured",
			want:         want{outcome: identity.Returning, id: "r4n8", entry: "r4n8"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.check(t, record.Adding(tt.configured, tt.organization))
		})
	}
}

// A configured account whose organization neither the config nor the record
// knows, whose id, in any case, the token's organization spells at its
// length, may well be the token's, as ids are derived: it's taken to be,
// refused, named, failing safe; an account at that id whose organization is
// known to be another's isn't, and the id derived is lengthened past it. One
// known to be the token's is named before it, and a removed account of the
// token's organization comes back before it.
func TestAddingTakesAConfiguredAccountThatMayBeTheTokensToBe(t *testing.T) {
	tests := []struct {
		name       string
		record     []identity.Entry
		configured map[string]string
		want       want
	}{
		{
			name:       "the record empty",
			configured: map[string]string{"b6s7": ""},
			want:       want{outcome: identity.Configured, id: "b6s7"},
		},
		{
			name:       "configured in another case",
			configured: map[string]string{"B6S7": ""},
			want:       want{outcome: identity.Configured, id: "B6S7"},
		},
		{
			name:       "at the id lengthened, the record empty, as after a state directory lost",
			configured: map[string]string{"b6s7e": ""},
			want:       want{outcome: identity.Configured, id: "b6s7e"},
		},
		{
			name:       "a removed account of the token's organization in the record, coming back first",
			record:     []identity.Entry{{ID: "m2x7", Organization: "test-org-work", Label: "side", Added: added, Removed: removed}},
			configured: map[string]string{"b6s7": ""},
			want:       want{outcome: identity.Returning, id: "m2x7", entry: "m2x7"},
		},
		{
			name:       "the record having it, its organization not known",
			record:     []identity.Entry{{ID: "b6s7", Added: added}},
			configured: map[string]string{"b6s7": ""},
			want:       want{outcome: identity.Configured, id: "b6s7", entry: "b6s7"},
		},
		{
			name:       "at the id lengthened past another organization's, the record having both",
			record:     []identity.Entry{{ID: "b6s7", Organization: "test-org-other", Added: added}, {ID: "b6s7e", Added: added}},
			configured: map[string]string{"b6s7": "test-org-other", "b6s7e": ""},
			want:       want{outcome: identity.Configured, id: "b6s7e", entry: "b6s7e"},
		},
		{
			name:       "at the id lengthened past another organization's, as the config knows it",
			configured: map[string]string{"b6s7": "test-org-other", "b6s7e": ""},
			want:       want{outcome: identity.Configured, id: "b6s7e"},
		},
		{
			name:       "one known to be the token's beside it, named first",
			configured: map[string]string{"b6s7": "", "z1y2": "test-org-work"},
			want:       want{outcome: identity.Configured, id: "z1y2"},
		},
		{
			name:       "at the id, of another organization, as the config knows",
			configured: map[string]string{"b6s7": "test-org-other"},
			want:       want{outcome: identity.NewAccount, id: "b6s7e"},
		},
		{
			name:       "at the id, of another organization, as the record knows",
			record:     []identity.Entry{{ID: "b6s7", Organization: "test-org-other", Added: added}},
			configured: map[string]string{"b6s7": ""},
			want:       want{outcome: identity.NewAccount, id: "b6s7e"},
		},
		{
			name:       "none at the id, one whose organization isn't known elsewhere",
			record:     []identity.Entry{{ID: "k3f9", Added: added}},
			configured: map[string]string{"k3f9": ""},
			want:       want{outcome: identity.NewAccount, id: "b6s7"},
		},
		{
			name:       "none at the id, one whose organization isn't known elsewhere, the record empty",
			configured: map[string]string{"k3f9": ""},
			want:       want{outcome: identity.NewAccount, id: "b6s7"},
		},
		{
			name:       "one whose id starts as the spelling does, but isn't it at its length, b6s7eho",
			configured: map[string]string{"b6s7eh0": ""},
			want:       want{outcome: identity.NewAccount, id: "b6s7"},
		},
		{
			name:       "a removed account at the id, its organization not known, offered",
			record:     []identity.Entry{{ID: "b6s7", Label: "side", Added: added, Removed: removed}},
			configured: map[string]string{},
			want:       want{outcome: identity.NewAccount, id: "b6s7e", candidates: []string{"b6s7"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.check(t, identity.Record{Accounts: tt.record}.Adding(tt.configured, "test-org-work"))
		})
	}
}

// want is what adding an account is to come to: its outcome, its id, the id
// of the record's account given, "" for none, and the ids of the candidates.
type want struct {
	outcome    identity.Outcome
	id, entry  string
	candidates []string
}

// check fails the test unless got is what's wanted.
func (w want) check(t *testing.T, got identity.Adding) {
	t.Helper()
	if got.Outcome != w.outcome || got.ID != w.id || got.Entry.ID != w.entry {
		t.Errorf("Adding() = outcome %d, id %q, account %q; want outcome %d, id %q, account %q",
			got.Outcome, got.ID, got.Entry.ID, w.outcome, w.id, w.entry)
	}
	if candidates := ids(got.Candidates); !slices.Equal(candidates, w.candidates) {
		t.Errorf("Adding() gives the candidates %q, want %q", candidates, w.candidates)
	}
}
