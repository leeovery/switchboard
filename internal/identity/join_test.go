package identity_test

import (
	"cmp"
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

// Two accounts of one id are one but where both organizations are known and
// differ, whatever else either is: removed or used, configured here or not.
// Matched, an account this Mac doesn't configure takes the other's
// organization where it knows none; one it configures takes none, as its
// token tells it. Two of known organizations that differ are two, the other
// taken in under another id.
func TestJoiningMatchesByIDOnlyWhereTheAccountsAreOne(t *testing.T) {
	tests := []struct {
		name       string
		own, other identity.Entry
		configured bool
		same       bool
		// organization is the account's, matched.
		organization string
	}{
		{name: "neither organization known", own: identity.Entry{ID: "k3f9", Added: added}, other: identity.Entry{ID: "k3f9", Added: added}, same: true},
		{name: "this one's unknown, removed", own: identity.Entry{ID: "k3f9", Label: "spare", Added: added, Removed: removed}, other: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, same: true, organization: "test-org-spare"},
		{name: "the other's unknown, removed", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Label: "spare", Added: added, Removed: removed}, same: true, organization: "test-org-spare"},
		{name: "this one's unknown, used", own: identity.Entry{ID: "k3f9", Added: added}, other: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, same: true, organization: "test-org-spare"},
		{name: "the other's unknown, used", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Added: added}, same: true, organization: "test-org-spare"},
		{name: "this one's unknown, configured here", own: identity.Entry{ID: "k3f9", Added: added}, other: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, configured: true, same: true},
		{name: "the other's unknown, configured here", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Added: added}, configured: true, same: true, organization: "test-org-spare"},
		{name: "both known, the same", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, same: true, organization: "test-org-spare"},
		{name: "both known, differing", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Organization: "test-org-other", Added: added}},
		{name: "both known, differing, configured here", own: identity.Entry{ID: "k3f9", Organization: "test-org-spare", Added: added}, other: identity.Entry{ID: "k3f9", Organization: "test-org-other", Added: added}, configured: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := identity.JoinAs{From: otherMac, Labels: true}
			if tt.configured {
				as.Configured = config.Accounts{{ID: tt.own.ID}}
			}
			record, mapped := joined(t, identity.Record{Accounts: []identity.Entry{tt.own}}, as, tt.other)
			if got := len(record.Accounts) == 1 && len(mapped) == 0; got != tt.same {
				t.Fatalf("joined, the record is %+v, mapped %v; want the two one account: %t", record.Accounts, mapped, tt.same)
			}
			if !tt.same {
				if id := mapped["k3f9"]; !identity.Spells(otherMac+"/k3f9", id) {
					t.Errorf("Join mapped k3f9 to %q, want the id spelled from the other Mac's install and its id", id)
				}
				return
			}
			if got := record.Accounts[0].Organization; got != tt.organization {
				t.Errorf("joined, the account's organization is %q, want %q", got, tt.organization)
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

// An account configured here that the record doesn't hold, as where another
// Mac sharing the config added it since this one moved, is the other
// record's account of its id, whatever its organization: it keeps its id,
// added with the organization known there, and joined again, nothing
// changes.
func TestJoiningTakesAnAccountConfiguredHereForTheOtherMacsOfItsID(t *testing.T) {
	configured := identity.JoinAs{Configured: append(slices.Clone(configuredHere), config.Account{ID: "r8tw"}), From: otherMac, Labels: true}
	tests := []struct {
		name         string
		organization string
	}{
		{name: "its organization known there", organization: "test-org-new"},
		{name: "its organization unknown there", organization: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := identity.Entry{ID: "r8tw", Organization: tt.organization, Label: "new", Added: removed, Configured: true}
			record, mapped := joined(t, thisMacs(), configured, other)
			want := append(thisMacs().Accounts, identity.Entry{ID: "r8tw", Organization: tt.organization, Added: removed})
			if !slices.Equal(record.Accounts, want) || len(mapped) != 0 {
				t.Errorf("joined, the record is\n%+v\nmapped %v; want\n%+v\nnothing mapped", record.Accounts, mapped, want)
			}
			again, remapped := joined(t, record, configured, other)
			if !slices.Equal(again.Accounts, record.Accounts) || len(remapped) != 0 {
				t.Errorf("joined again, the record is %+v, mapped %v; want it as it was", again.Accounts, remapped)
			}
		})
	}
}

// An account both Macs configure, of an id made at random as one Mac moved
// without its token, whose organization one Mac knows, as one its token
// told it since, and the other doesn't, is one account, kept under its id,
// this Mac learning no organization of it from the other, as its token
// tells it. Joined again, and again once this Mac knows its organization,
// nothing changes.
func TestJoiningAnAccountBothMacsConfigureOfARandomIDKeepsIt(t *testing.T) {
	const random, organization = "r8tw", "test-org-three"
	as := identity.JoinAs{Configured: config.Accounts{{ID: random}}, From: otherMac, Labels: true}
	tests := []struct {
		name        string
		here, there string
	}{
		{name: "known there", here: "", there: organization},
		{name: "known here", here: organization, there: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			own := identity.Record{Accounts: []identity.Entry{{ID: random, Organization: tt.here, Added: added}}}
			other := identity.Entry{ID: random, Organization: tt.there, Label: "three", Added: removed, Configured: true}
			record, mapped := joined(t, own, as, other)
			if !slices.Equal(record.Accounts, own.Accounts) || len(mapped) != 0 {
				t.Fatalf("joined, the record is %+v, mapped %v; want it as it was, nothing mapped", record.Accounts, mapped)
			}
			again, remapped := joined(t, record, as, other)
			if !slices.Equal(again.Accounts, record.Accounts) || len(remapped) != 0 {
				t.Errorf("joined again, the record is %+v, mapped %v; want it as it was", again.Accounts, remapped)
			}
			again.Organize(random, organization)
			learned := identity.Record{Accounts: slices.Clone(again.Accounts)}
			last, lastMapped := joined(t, again, as, other)
			if !slices.Equal(last.Accounts, learned.Accounts) || len(lastMapped) != 0 {
				t.Errorf("joined once its organization is known here, the record is %+v, mapped %v; want it as it was", last.Accounts, lastMapped)
			}
		})
	}
}

// In this Mac's own record, an account of an id this Mac holds is that
// account, even where both know its organization and they differ; in
// another Mac's, it's another then, taken in under another id. Either way,
// where only one knows the organization, it's that account, which takes the
// organization the record joined gives it where it's neither configured
// here nor knows one.
func TestJoiningThisMacsOwnRecordMatchesEveryAccountByItsID(t *testing.T) {
	for _, tt := range []struct {
		name        string
		own         bool
		configured  bool
		here, there string
		same        bool
		// organization is the account's, matched.
		organization string
	}{
		{name: "this Mac's, configured, of an organization this Mac doesn't know", own: true, configured: true, there: "test-org-spare", same: true},
		{name: "this Mac's, unconfigured, of an organization this Mac doesn't know", own: true, there: "test-org-spare", same: true, organization: "test-org-spare"},
		{name: "this Mac's, of an organization the folder doesn't know", own: true, configured: true, here: "test-org-spare", same: true, organization: "test-org-spare"},
		{name: "another Mac's, of an organization this Mac doesn't know", configured: true, there: "test-org-spare", same: true},
		{name: "this Mac's, of another organization", own: true, configured: true, here: "test-org-spare", there: "test-org-other", same: true, organization: "test-org-spare"},
		{name: "another Mac's, of another organization", configured: true, here: "test-org-spare", there: "test-org-other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			here := identity.Record{Accounts: []identity.Entry{{ID: "k3f9", Organization: tt.here, Added: added}}}
			as := identity.JoinAs{From: otherMac, Own: tt.own}
			if tt.configured {
				as.Configured = config.Accounts{{ID: "k3f9"}}
			}
			record, mapped := joined(t, here, as, identity.Entry{ID: "k3f9", Organization: tt.there, Added: added})
			if got := len(record.Accounts) == 1 && len(mapped) == 0; got != tt.same {
				t.Fatalf("joined, the record is %+v, mapped %v; want the two one account: %t", record.Accounts, mapped, tt.same)
			}
			if got := record.Accounts[0].Organization; tt.same && got != tt.organization {
				t.Errorf("joined, the account's organization is %q, want %q", got, tt.organization)
			}
		})
	}
}

// In this Mac's own record, an account is matched by its id before its
// organization, so an identical id is always that account, though the
// organization the record joined gives it is another account's here, which
// it never takes; in another Mac's, the organization is matched first.
func TestJoiningThisMacsOwnRecordMatchesByIDBeforeOrganization(t *testing.T) {
	other := identity.Entry{ID: "m5t2", Organization: "test-org-other", Added: added}
	exported := identity.Entry{ID: "k3f9", Organization: "test-org-other", Added: added}
	for _, tt := range []struct {
		name string
		own  bool
		// organization is k3f9's here.
		organization string
		want         string
	}{
		{name: "this Mac's", own: true, organization: "test-org-spare", want: "k3f9"},
		{name: "this Mac's, k3f9's organization unknown here", own: true, want: "k3f9"},
		{name: "another Mac's", organization: "test-org-spare", want: "m5t2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			own := identity.Record{Accounts: []identity.Entry{{ID: "k3f9", Organization: tt.organization, Added: added}, other}}
			record, mapped := joined(t, own, identity.JoinAs{From: otherMac, Own: tt.own}, exported)
			if got := cmp.Or(mapped["k3f9"], "k3f9"); got != tt.want || !slices.Equal(record.Accounts, own.Accounts) {
				t.Errorf("joined, k3f9 is %s, the record %+v; want %s, the record as it was", got, record.Accounts, tt.want)
			}
		})
	}
}

// An account of an id configured here that the record doesn't hold, whose
// organization another account here has, is that account, as matched by
// its organization before it's taken for the one configured.
func TestJoiningAnAccountConfiguredHereUnheldMatchesByOrganizationFirst(t *testing.T) {
	as := identity.JoinAs{Configured: append(slices.Clone(configuredHere), config.Account{ID: "r8tw"}), From: otherMac}
	record, mapped := joined(t, thisMacs(), as, identity.Entry{ID: "r8tw", Organization: "test-org-home", Added: removed})
	if want := map[string]string{"r8tw": "p4m2"}; !slices.Equal(record.Accounts, thisMacs().Accounts) || !maps.Equal(mapped, want) {
		t.Errorf("joined, the record is %+v, mapped %v; want it as it was, mapped %v", record.Accounts, mapped, want)
	}
}

// Where the id spelled for an account whose id is held is held by an
// account whose organization isn't known, that's the account, as one id is
// one account, which takes its organization, as it isn't configured here.
func TestTheIDSpelledForAnAccountWhoseIDIsHeldMayBeItsAlready(t *testing.T) {
	other := identity.Entry{ID: "qgr2", Organization: "test-org-other", Label: "other", Added: added}
	_, mapped := joined(t, thisMacs(), joinAs, other)
	spelled := mapped["qgr2"]
	own := identity.Record{Accounts: append(thisMacs().Accounts, identity.Entry{ID: spelled, Added: added})}

	record, mapped := joined(t, own, joinAs, other)
	if id := mapped["qgr2"]; id != spelled || len(record.Accounts) != len(own.Accounts) {
		t.Fatalf("joined, qgr2 is mapped to %q, the record %+v; want it %s, the account held", id, record.Accounts, spelled)
	}
	if e, _ := record.ByID(spelled); e.Organization != other.Organization {
		t.Errorf("joined, %s's organization is %q, want %q", spelled, e.Organization, other.Organization)
	}
}
