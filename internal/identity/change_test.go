package identity_test

import (
	"slices"
	"testing"
	"time"

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
