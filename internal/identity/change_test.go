package identity_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// Each change to the record changes the account it names, found by its id in
// any case, and reports whether the record has it: an account added
// configured; one returning, configured again, its organization taken where
// it wasn't known, and kept where it was; an organization recorded only where
// none was; and one removed, keeping its label.
func TestTheRecordsChanges(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	record := func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{
			{ID: "m2x7", Organization: "test-org-side", Label: "side", Added: added, Removed: removed},
			{ID: "h7c2", Label: "spare", Added: added, Removed: removed, Was: "spare"},
			{ID: "k3f9", Added: added},
		}}
	}
	tests := []struct {
		name   string
		change func(*identity.Record) bool
		want   []identity.Entry
		ok     bool
	}{
		{
			name:   "an account added",
			change: func(r *identity.Record) bool { r.Add("b6s7", "test-org-work", later); return true },
			want: slices.Concat(record().Accounts, []identity.Entry{
				{ID: "b6s7", Organization: "test-org-work", Added: later},
			}),
			ok: true,
		},
		{
			name:   "an account returning, its organization known",
			change: func(r *identity.Record) bool { return r.Return("M2X7", "test-org-other") },
			want: []identity.Entry{
				{ID: "m2x7", Organization: "test-org-side", Added: added},
				record().Accounts[1], record().Accounts[2],
			},
			ok: true,
		},
		{
			name:   "an account returning, its organization taken",
			change: func(r *identity.Record) bool { return r.Return("h7c2", "test-org-spare") },
			want: []identity.Entry{
				record().Accounts[0],
				{ID: "h7c2", Organization: "test-org-spare", Added: added, Was: "spare"},
				record().Accounts[2],
			},
			ok: true,
		},
		{
			name:   "an account returning that the record hasn't",
			change: func(r *identity.Record) bool { return r.Return("work", "test-org-work") },
			want:   record().Accounts,
		},
		{
			name:   "an organization recorded",
			change: func(r *identity.Record) bool { return r.Organize("k3f9", "test-org-work") },
			want: []identity.Entry{
				record().Accounts[0], record().Accounts[1],
				{ID: "k3f9", Organization: "test-org-work", Added: added},
			},
			ok: true,
		},
		{
			name:   "an organization recorded where one's known",
			change: func(r *identity.Record) bool { return r.Organize("m2x7", "test-org-other") },
			want:   record().Accounts,
			ok:     true,
		},
		{
			name:   "an account removed",
			change: func(r *identity.Record) bool { return r.Remove("K3F9", "Work", later) },
			want: []identity.Entry{
				record().Accounts[0], record().Accounts[1],
				{ID: "k3f9", Label: "Work", Added: added, Removed: later},
			},
			ok: true,
		},
		{
			name:   "an account removed that the record hasn't, as one from before ids were made",
			change: func(r *identity.Record) bool { return r.Remove("work", "Work", later) },
			want:   record().Accounts,
		},
		{
			name:   "an account moved onto its id",
			change: func(r *identity.Record) bool { r.Move("b6s7", "test-org-work", "work", later); return true },
			want: slices.Concat(record().Accounts, []identity.Entry{
				{ID: "b6s7", Organization: "test-org-work", Added: later, Was: "work"},
			}),
			ok: true,
		},
		{
			name:   "an account removed before ids were made",
			change: func(r *identity.Record) bool { r.RemovedBefore("q8n4", "old", added, later); return true },
			want: slices.Concat(record().Accounts, []identity.Entry{
				{ID: "q8n4", Label: "old", Added: added, Removed: later, Was: "old"},
			}),
			ok: true,
		},
		{
			name:   "the id an account had before, recorded",
			change: func(r *identity.Record) bool { return r.Former("M2X7", "side") },
			want: []identity.Entry{
				{ID: "m2x7", Organization: "test-org-side", Label: "side", Added: added, Removed: removed, Was: "side"},
				record().Accounts[1], record().Accounts[2],
			},
			ok: true,
		},
		{
			name:   "the id an account had before, where it has another",
			change: func(r *identity.Record) bool { return r.Former("h7c2", "other") },
			want:   record().Accounts,
		},
		{
			name:   "the id an account had before, where it has that one",
			change: func(r *identity.Record) bool { return r.Former("h7c2", "SPARE") },
			want:   record().Accounts,
			ok:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := record()
			if ok := tt.change(&r); ok != tt.ok {
				t.Errorf("the change reported %v, want %v", ok, tt.ok)
			}
			if !slices.Equal(r.Accounts, tt.want) {
				t.Errorf("the record holds\n%+v\nwant\n%+v", r.Accounts, tt.want)
			}
		})
	}
}

// The record keeps up with the accounts configured, ids found in any case:
// one it lacks is added, but one whose id clashes with one an account had
// before, or that isn't one switchboard makes; one removed, or kept as only
// another Mac configures it, is configured again; and one this Mac
// configured that has left the config is marked removed, with the label it
// last had there, else its id, one only another Mac configures left as it is.
func TestTheRecordKeepsUpWithTheAccountsConfigured(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	record := func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{
			{ID: "b6s7", Organization: "test-org-work", Added: added, Was: "work"},
			{ID: "qgr2", Organization: "test-org-personal", Label: "personal", Added: added, Removed: removed},
			{ID: "m2x7", Label: "side", Added: added},
			{ID: "k3f9", Added: added},
			{ID: "h7c2", Label: "spare", Added: added, Removed: removed, Was: "spare"},
		}}
	}
	labelled := func(id string) string { return map[string]string{"k3f9": "Team"}[id] }
	unlabelled := func(string) string { return "" }
	configured := func(ids ...string) config.Accounts {
		as := make(config.Accounts, len(ids))
		for i, id := range ids {
			as[i] = config.Account{ID: id, Label: "label-" + id}
		}
		return as
	}
	at := func(r identity.Record, i int, e identity.Entry) []identity.Entry {
		accounts := slices.Clone(r.Accounts)
		accounts[i] = e
		return accounts
	}
	tests := []struct {
		name       string
		configured config.Accounts
		labelOf    func(string) string
		want       []identity.Entry
		kept       identity.Kept
	}{
		{name: "in step", configured: configured("b6s7", "K3F9"), want: record().Accounts},
		{
			name: "an account the record lacks", configured: configured("b6s7", "k3f9", "n4p8"),
			want: append(record().Accounts, identity.Entry{ID: "n4p8", Added: later}),
			kept: identity.Kept{Added: []string{"n4p8"}},
		},
		{
			name: "one removed, configured again", configured: configured("b6s7", "k3f9", "QGR2"),
			want: at(record(), 1, identity.Entry{ID: "qgr2", Organization: "test-org-personal", Added: added}),
			kept: identity.Kept{Returned: []string{"qgr2"}},
		},
		{
			name: "one only another Mac configures, configured here", configured: configured("b6s7", "k3f9", "m2x7"),
			want: at(record(), 2, identity.Entry{ID: "m2x7", Added: added}),
			kept: identity.Kept{Returned: []string{"m2x7"}},
		},
		{
			name: "one that left the config", configured: configured("b6s7"), labelOf: labelled,
			want: at(record(), 3, identity.Entry{ID: "k3f9", Label: "Team", Added: added, Removed: later}),
			kept: identity.Kept{Removed: []string{"k3f9"}},
		},
		{
			name: "one that left the config, its label not known", configured: configured("b6s7"), labelOf: unlabelled,
			want: at(record(), 3, identity.Entry{ID: "k3f9", Label: "k3f9", Added: added, Removed: later}),
			kept: identity.Kept{Removed: []string{"k3f9"}},
		},
		{
			name: "one whose id an account had before", configured: configured("b6s7", "k3f9", "SPARE"),
			want: record().Accounts, kept: identity.Kept{Clashed: []string{"SPARE"}},
		},
		{
			name: "one whose id isn't one switchboard makes", configured: configured("b6s7", "k3f9", "9abc"),
			want: record().Accounts, kept: identity.Kept{Clashed: []string{"9abc"}},
		},
		{
			name: "one by the id it had before, as a config from before the move names it, which changes nothing till it's healed", configured: configured("WORK", "k3f9", "n4p8"),
			want: record().Accounts, kept: identity.Kept{Unhealed: []string{"WORK"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := record()
			labelOf := tt.labelOf
			if labelOf == nil {
				labelOf = unlabelled
			}
			kept := r.KeepUp(identity.Followed{Accounts: tt.configured, AccountLabel: labelOf}, later)
			if !slices.Equal(r.Accounts, tt.want) {
				t.Errorf("the record holds\n%+v\nwant\n%+v", r.Accounts, tt.want)
			}
			if !reflect.DeepEqual(kept, tt.kept) {
				t.Errorf("kept %+v, want %+v", kept, tt.kept)
			}
		})
	}
}
