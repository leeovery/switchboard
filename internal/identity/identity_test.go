package identity_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
)

// added is when the tests' accounts were added, and removed when the removed
// ones were removed.
var (
	added   = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	removed = time.Date(2026, 10, 20, 7, 45, 0, 0, time.UTC)
)

// Each lookup finds the account the record has by what it's given, an id in
// any case, and none for what no account has, nor for "", which many have of
// an organization or an id before switchboard made ids.
func TestTheRecordsLookups(t *testing.T) {
	record := identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Organization: "test-org-work", Added: added},
		{ID: "qgr2", Organization: "test-org-personal", Label: "personal", Added: added, Removed: removed, Was: "personal"},
		{ID: "m2x7", Label: "side", Added: added, Removed: removed, Was: "side"},
		{ID: "k3f9", Added: added},
	}}
	tests := []struct {
		name   string
		lookup func() (identity.Entry, bool)
		// want is the id of the account found: "" for none.
		want string
	}{
		{name: "by id", lookup: func() (identity.Entry, bool) { return record.ByID("qgr2") }, want: "qgr2"},
		{name: "by id, in another case", lookup: func() (identity.Entry, bool) { return record.ByID("QGR2") }, want: "qgr2"},
		{name: "by an id none has", lookup: func() (identity.Entry, bool) { return record.ByID("zzzz") }},
		{name: "by no id", lookup: func() (identity.Entry, bool) { return record.ByID("") }},
		{name: "by an id one had before", lookup: func() (identity.Entry, bool) { return record.ByID("personal") }},
		{name: "by organization", lookup: func() (identity.Entry, bool) { return record.ByOrganization("test-org-personal") }, want: "qgr2"},
		{name: "by an organization none has", lookup: func() (identity.Entry, bool) { return record.ByOrganization("test-org-other") }},
		{name: "by no organization", lookup: func() (identity.Entry, bool) { return record.ByOrganization("") }},
		{name: "by the id before", lookup: func() (identity.Entry, bool) { return record.ByWas("side") }, want: "m2x7"},
		{name: "by the id before, in another case", lookup: func() (identity.Entry, bool) { return record.ByWas("Side") }, want: "m2x7"},
		{name: "by an id before none had", lookup: func() (identity.Entry, bool) { return record.ByWas("other") }},
		{name: "by no id before", lookup: func() (identity.Entry, bool) { return record.ByWas("") }},
		{name: "by an id made, as one before", lookup: func() (identity.Entry, bool) { return record.ByWas("b6s7") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.lookup()
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("found %q, %v; want %q", got.ID, ok, tt.want)
			}
		})
	}
}

// Removed gives the accounts removed, in the record's order, and none of
// those configured.
func TestRemovedGivesTheAccountsRemoved(t *testing.T) {
	record := identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Organization: "test-org-work", Added: added},
		{ID: "qgr2", Label: "personal", Added: added, Removed: removed},
		{ID: "k3f9", Added: added},
		{ID: "m2x7", Label: "side", Added: added, Removed: removed},
	}}
	if got := ids(record.Removed()); !slices.Equal(got, []string{"qgr2", "m2x7"}) {
		t.Errorf("Removed() = %q, want %q", got, []string{"qgr2", "m2x7"})
	}
	if got := (identity.Record{Accounts: record.Accounts[:1]}).Removed(); len(got) != 0 {
		t.Errorf("Removed() of a record with none removed = %q, want none", ids(got))
	}
}

// ids are the accounts' ids, in order.
func ids(accounts []identity.Entry) []string {
	got := make([]string, len(accounts))
	for i, e := range accounts {
		got[i] = e.ID
	}
	return got
}
