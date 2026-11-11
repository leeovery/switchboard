package identity_test

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// otherMac is the install of the Mac whose record the join tests join.
const otherMac = "fedcba9876543210"

// thisMacs is the record joined into in the join tests: work and personal,
// configured here; side, removed here, its organization unknown; and home,
// only another Mac configures, as an import took it in.
func thisMacs() identity.Record {
	return identity.Record{Accounts: []identity.Entry{
		{ID: "b6s7", Organization: "test-org-work", Added: added, Was: "work"},
		{ID: "qgr2", Organization: "test-org-personal", Added: added, Was: "personal"},
		{ID: "m2x7", Label: "side", Added: added, Removed: removed, Was: "side"},
		{ID: "p4m2", Organization: "test-org-home", Label: "home", Added: added},
	}}
}

// configuredHere are the accounts this Mac configures in the join tests.
var configuredHere = config.Accounts{{ID: "B6S7", Label: "Work"}, {ID: "qgr2", Label: "Personal"}}

// joinAs joins as the join tests do, labels taken.
var joinAs = identity.JoinAs{Configured: configuredHere, From: otherMac, Labels: true}

// joined is record with other's accounts joined into it as as says, and what
// Join mapped, failing the test where it fails.
func joined(t *testing.T, record identity.Record, as identity.JoinAs, other ...identity.Entry) (identity.Record, map[string]string) {
	t.Helper()
	mapped, err := record.Join(identity.Record{Accounts: other}, as)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	return record, mapped
}

// Joining another Mac's record matches each of its accounts to this one's by
// organization, else by id, mapping its id onto this one's where they
// differ: the account takes its organization where it has none, and its
// label only where the other Mac configures it and this one neither
// configures nor removed it. The ids accounts had before are each Mac's own
// names, so none is matched on or taken. One matching none is added as it
// is, but for the ids it had before and the other Mac's mark.
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
			name:  "a configured account matched by its id, keeping its label",
			other: identity.Entry{ID: "b6s7", Organization: "test-org-work", Label: "Office", Added: later, Was: "office", Configured: true},
			want:  own,
		},
		{
			name:   "a configured account under another id there, mapped by its organization",
			other:  identity.Entry{ID: "z9k4", Organization: "test-org-personal", Label: "Home office", Added: later, Was: "home", Configured: true},
			want:   own,
			mapped: map[string]string{"z9k4": "qgr2"},
		},
		{
			name:  "an account removed here, its organization unknown, matched by its id, taking its organization, keeping its label",
			other: identity.Entry{ID: "m2x7", Organization: "test-org-side", Label: "Side gig", Added: later, Configured: true},
			want:  []identity.Entry{own[0], own[1], {ID: "m2x7", Organization: "test-org-side", Label: "side", Added: added, Removed: removed, Was: "side"}, own[3]},
		},
		{
			name:  "an account only another Mac configures takes the label it's configured by there now",
			other: identity.Entry{ID: "p4m2", Organization: "test-org-home", Label: "family", Added: later, Configured: true},
			want:  []identity.Entry{own[0], own[1], own[2], {ID: "p4m2", Organization: "test-org-home", Label: "family", Added: added}},
		},
		{
			name:  "an account the other Mac doesn't configure gives no label",
			other: identity.Entry{ID: "p4m2", Organization: "test-org-home", Label: "family", Added: later},
			want:  own,
		},
		{
			name:  "an account removed there under another random id, its id before the same as one here, added as another",
			other: identity.Entry{ID: "r7kq", Label: "side", Added: later, Removed: later, Was: "side"},
			want:  slices.Concat(own, []identity.Entry{{ID: "r7kq", Label: "side", Added: later, Removed: later}}),
		},
		{
			name:  "an account only the other Mac configures, added with its label",
			other: identity.Entry{ID: "h7c2", Organization: "test-org-team", Label: "team", Added: later, Was: "team", Configured: true},
			want:  slices.Concat(own, []identity.Entry{{ID: "h7c2", Organization: "test-org-team", Label: "team", Added: later}}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, mapped := joined(t, thisMacs(), joinAs, tt.other)
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

// No label is taken from this Mac's own export, which may be older than a
// label another Mac gave since.
func TestJoiningThisMacsOwnRecordTakesNoLabel(t *testing.T) {
	as := joinAs
	as.Labels = false
	record, _ := joined(t, thisMacs(), as, identity.Entry{ID: "p4m2", Organization: "test-org-home", Label: "family", Added: added, Configured: true})
	if !slices.Equal(record.Accounts, thisMacs().Accounts) {
		t.Errorf("joined, the record is\n%+v\nwant it as it was", record.Accounts)
	}
}

// spareID is the id test-org-spare spells, as Derive spells one.
var spareID = identity.Record{}.Derive("test-org-spare", nil)

// Two accounts of one id are one where their organizations are the same, or
// neither is known, or the one whose organization isn't known is removed, or
// the one known spells the id; one used, its organization unknown, and one
// of a known organization that doesn't spell the id are two, as
// subscriptions given one id at random are.
func TestJoiningMatchesByIDOnlyWhereTheAccountsAreOne(t *testing.T) {
	tests := []struct {
		name       string
		own, other identity.Entry
		same       bool
	}{
		{name: "neither organization known", own: identity.Entry{ID: "k3f9", Added: added}, other: identity.Entry{ID: "k3f9", Added: added}, same: true},
		{name: "this one's unknown, removed", own: identity.Entry{ID: "k3f9", Label: "spare", Added: added, Removed: removed}, other: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, same: true},
		{name: "the other's unknown, removed", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Label: "spare", Added: added, Removed: removed}, same: true},
		{name: "this one's unknown, used", own: identity.Entry{ID: "k3f9", Added: added}, other: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}},
		{name: "the other's unknown, used", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Added: added}},
		{name: "this one's unknown, used, the other's spelling the id", own: identity.Entry{ID: spareID, Added: added}, other: identity.Entry{ID: spareID, Organization: "test-org-spare", Added: added}, same: true},
		{name: "the other's unknown, used, this one's spelling the id", own: identity.Entry{ID: spareID, Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: spareID, Added: added}, same: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, mapped := joined(t, identity.Record{Accounts: []identity.Entry{tt.own}}, joinAs, tt.other)
			if got := len(record.Accounts) == 1 && len(mapped) == 0; got != tt.same {
				t.Errorf("joined, the record is %+v, mapped %v; want the two one account: %t", record.Accounts, mapped, tt.same)
			}
		})
	}
}

// An account whose id is held here already, by an account of another
// organization or as an id one had before, is added under an id spelled from
// the other Mac's install and its id, mapped onto it, the same however often
// the record is joined, so a folder imported again doubles nothing.
func TestJoiningAnAccountWhoseIDIsHeldGivesItAnotherForGood(t *testing.T) {
	tests := []struct {
		name  string
		other identity.Entry
	}{
		{name: "the id of an account of another organization", other: identity.Entry{ID: "qgr2", Organization: "test-org-other", Label: "other", Added: added}},
		{name: "an id an account had before", other: identity.Entry{ID: "side", Label: "spare", Added: added, Removed: removed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, mapped := joined(t, thisMacs(), joinAs, tt.other)
			id := mapped[strings.ToLower(tt.other.ID)]
			if len(id) != 4 || !identity.Spells(otherMac+"/"+strings.ToLower(tt.other.ID), id) {
				t.Fatalf("Join mapped %s to %q, want the id spelled from the other Mac's install and its id", tt.other.ID, id)
			}
			if e, ok := record.ByID(id); !ok || e.Label != tt.other.Label || e.Organization != tt.other.Organization {
				t.Errorf("joined, %s is %+v, want the other Mac's account under it", id, e)
			}
			again, remapped := joined(t, record, joinAs, tt.other)
			if !slices.Equal(again.Accounts, record.Accounts) || !maps.Equal(remapped, mapped) {
				t.Errorf("joined again, the record is %+v, mapped %v; want it as it was, mapped %v", again.Accounts, remapped, mapped)
			}
		})
	}
}

// Where the id spelled for an account whose id is held is held too, by an
// account here or a configured one the record doesn't hold yet, it's spelled
// a character longer.
func TestTheIDSpelledForAnAccountWhoseIDIsHeldIsNeverHeld(t *testing.T) {
	other := identity.Entry{ID: "qgr2", Organization: "test-org-other", Label: "other", Added: added}
	seed := otherMac + "/qgr2"
	_, mapped := joined(t, thisMacs(), joinAs, other)
	spelled := mapped["qgr2"]
	tests := []struct {
		name string
		own  identity.Record
		as   identity.JoinAs
	}{
		{name: "an account here", own: identity.Record{Accounts: append(thisMacs().Accounts, identity.Entry{ID: spelled, Organization: "test-org-x", Added: added})}, as: joinAs},
		{name: "a configured one not in the record", own: thisMacs(), as: identity.JoinAs{Configured: append(slices.Clone(configuredHere), config.Account{ID: spelled}), From: otherMac}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, mapped := joined(t, tt.own, tt.as, other)
			if id := mapped["qgr2"]; len(id) != 5 || !identity.Spells(seed, id) {
				t.Errorf("Join mapped qgr2 to %q, want the id spelled a character longer than the held %s", id, spelled)
			}
		})
	}
}

// The id spelled for an account whose id is held is never one of the other
// record's accounts' ids, though the record doesn't hold it yet.
func TestTheIDSpelledForAnAccountWhoseIDIsHeldIsNeverAnotherOfTheRecordJoined(t *testing.T) {
	other := identity.Entry{ID: "qgr2", Organization: "test-org-other", Label: "other", Added: added}
	_, mapped := joined(t, thisMacs(), joinAs, other)
	spelled := mapped["qgr2"]
	staged := identity.Entry{ID: spelled, Organization: "test-org-x", Label: "x", Added: added}

	record, mapped := joined(t, thisMacs(), joinAs, other, staged)
	id := mapped["qgr2"]
	if len(id) != 5 || !identity.Spells(otherMac+"/qgr2", id) {
		t.Errorf("Join mapped qgr2 to %q, want the id spelled a character longer than %s, the other record's", id, spelled)
	}
	if e, ok := record.ByID(spelled); !ok || e.Organization != staged.Organization {
		t.Errorf("joined, %s is %+v, want the other record's account of that id", spelled, e)
	}
}

// newID is the id test-org-new spells, as Derive spells one: the id of an
// account configured here whose token this Mac hasn't been given.
var newID = identity.Record{}.Derive("test-org-new", nil)

// An account configured here whose organization this Mac doesn't know yet,
// as one whose token it hasn't been given, is the other record's account of
// its id whose organization is unknown or spells it, whether the record
// holds it, as the move added it, or not, as where another Mac sharing the
// config added it since: it keeps its id, the record learning no
// organization of one it holds, as its token tells it, and joined again,
// nothing changes. One whose organization spells another id is another.
func TestJoiningTakesAnAccountConfiguredHereForTheOtherMacsOfItsID(t *testing.T) {
	configured := identity.JoinAs{Configured: append(slices.Clone(configuredHere), config.Account{ID: newID}), From: otherMac, Labels: true}
	tests := []struct {
		name  string
		own   identity.Record
		other identity.Entry
		want  []identity.Entry
	}{
		{
			name:  "held, its organization unknown here",
			own:   identity.Record{Accounts: append(thisMacs().Accounts, identity.Entry{ID: newID, Added: added})},
			other: identity.Entry{ID: newID, Organization: "test-org-new", Label: "new", Added: removed, Configured: true},
			want:  append(thisMacs().Accounts, identity.Entry{ID: newID, Added: added}),
		},
		{
			name:  "not held, its organization known there",
			own:   thisMacs(),
			other: identity.Entry{ID: newID, Organization: "test-org-new", Label: "new", Added: removed, Configured: true},
			want:  append(thisMacs().Accounts, identity.Entry{ID: newID, Organization: "test-org-new", Added: removed}),
		},
		{
			name:  "not held, its organization unknown there too",
			own:   thisMacs(),
			other: identity.Entry{ID: newID, Label: "new", Added: removed, Configured: true},
			want:  append(thisMacs().Accounts, identity.Entry{ID: newID, Added: removed}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, mapped := joined(t, tt.own, configured, tt.other)
			if !slices.Equal(record.Accounts, tt.want) || len(mapped) != 0 {
				t.Errorf("joined, the record is\n%+v\nmapped %v; want\n%+v\nnothing mapped", record.Accounts, mapped, tt.want)
			}
			again, remapped := joined(t, record, configured, tt.other)
			if !slices.Equal(again.Accounts, record.Accounts) || len(remapped) != 0 {
				t.Errorf("joined again, the record is %+v, mapped %v; want it as it was", again.Accounts, remapped)
			}
		})
	}
	t.Run("not held, its organization spelling another id", func(t *testing.T) {
		other := identity.Entry{ID: newID, Organization: "test-org-spare", Added: removed}
		_, mapped := joined(t, thisMacs(), configured, other)
		if id := mapped[strings.ToLower(newID)]; !identity.Spells(otherMac+"/"+strings.ToLower(newID), id) {
			t.Errorf("Join mapped %s to %q, want it taken in under another id", newID, id)
		}
	})
}

// In this Mac's own record, an account of an id this Mac holds is that
// account, whatever either knows of its organization, as where a token told
// this Mac its organization since it exported; in another Mac's, it's
// another, its organization unknown, not spelling the id, and used.
func TestJoiningThisMacsOwnRecordMatchesEveryAccountByItsID(t *testing.T) {
	own := identity.Record{Accounts: []identity.Entry{{ID: "k3f9", Organization: "test-org-spare", Added: added}}}
	exported := identity.Entry{ID: "k3f9", Added: added}
	for _, tt := range []struct {
		name string
		own  bool
		same bool
	}{
		{name: "this Mac's", own: true, same: true},
		{name: "another Mac's", own: false, same: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record, mapped := joined(t, own, identity.JoinAs{From: otherMac, Own: tt.own}, exported)
			if got := slices.Equal(record.Accounts, own.Accounts) && len(mapped) == 0; got != tt.same {
				t.Errorf("joined, the record is %+v, mapped %v; want the two one account: %t", record.Accounts, mapped, tt.same)
			}
		})
	}
}
