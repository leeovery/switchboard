package status_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
)

// The pools of the moved accounts' tests: Default, where work and personal
// were, and Acme, where work is now, with acme-1 beside it.
var (
	movedPools    = config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "q7mt", Label: "Acme"}}
	movedAccounts = config.Accounts{
		{ID: "work", Label: "work", Pool: "q7mt"},
		{ID: "personal", Label: "personal", Pool: "b2n8"},
		{ID: "acme1", Label: "acme-1", Pool: "Q7MT"},
	}
)

// The accounts moved, seen from a pool, are those configured in any other,
// in the config's order, each with the pool it's in now, by its id and
// label, pool ids compared ignoring case; seen from a pool removed, every
// account is in another; and there are none of no pool, nor in a config
// without pools.
func TestTheAccountsMovedAreThoseInAnotherPoolThanTheOneRead(t *testing.T) {
	workInAcme := status.MovedAccount{ID: "work", Label: "work", Pool: "q7mt", PoolLabel: "Acme"}
	acmeInAcme := status.MovedAccount{ID: "acme1", Label: "acme-1", Pool: "Q7MT", PoolLabel: "Acme"}
	tests := []struct {
		name  string
		pools config.Pools
		pool  string
		want  status.MovedAccounts
	}{
		{name: "the old pool", pools: movedPools, pool: "b2n8", want: status.MovedAccounts{workInAcme, acmeInAcme}},
		{name: "the new pool", pools: movedPools, pool: "Q7MT", want: status.MovedAccounts{{ID: "personal", Label: "personal", Pool: "b2n8", PoolLabel: "Default"}}},
		{name: "a pool removed", pools: movedPools, pool: "x9z9", want: status.MovedAccounts{workInAcme, {ID: "personal", Label: "personal", Pool: "b2n8", PoolLabel: "Default"}, acmeInAcme}},
		{name: "no pool", pools: movedPools, pool: ""},
		{name: "no pools", pool: "b2n8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.MovedOf(tt.pools, movedAccounts, tt.pool); !slices.Equal(got, tt.want) {
				t.Errorf("MovedOf(%q) = %+v, want %+v", tt.pool, got, tt.want)
			}
		})
	}
}

// A pool with no label is named by its id, wherever an account moved to it
// says where it is.
func TestAnAccountMovedToAPoolWithNoLabelSaysItsID(t *testing.T) {
	moved := status.MovedOf(config.Pools{{ID: "b2n8", Label: "Default"}, {ID: "q7mt"}}, movedAccounts[:1], "b2n8")
	if len(moved) != 1 || moved[0].Said() != "now in q7mt" {
		t.Errorf("MovedOf() = %+v, want work, now in q7mt", moved)
	}
}

// A document given the config works out the accounts moved for its own pool:
// Default's names work as now in Acme, Acme's names it as its own.
func TestADocumentGivenTheConfigNamesTheAccountsMovedOutOfItsPool(t *testing.T) {
	doc := routerDocOf(status.Account{ID: "personal", Label: "personal", TokenSet: true, FetchedAt: built})
	doc.PoolID = "b2n8"
	want := status.AccountName{Label: "work", Other: true, Moved: "now in Acme"}
	if got := doc.WithMovedOf(movedPools, movedAccounts).At(built, policy).Named("work"); got != want {
		t.Errorf("Default's moment names work %+v, want %+v", got, want)
	}
	acme := routerDocOf(status.Account{ID: "work", Label: "work", TokenSet: true, FetchedAt: built})
	acme.PoolID = "q7mt"
	if got := acme.WithMovedOf(movedPools, movedAccounts).At(built, policy).Named("personal"); got.Moved != "now in Default" {
		t.Errorf("Acme's moment names personal %+v, want it now in Default", got)
	}
}

// An account moved with no label is labelled by its id, as a configured one
// is named, so the JSON's label and the name agree.
func TestAnAccountMovedWithNoLabelIsLabelledByItsID(t *testing.T) {
	moved := status.MovedOf(movedPools, config.Accounts{{ID: "k3f9", Pool: "q7mt"}}, "b2n8")
	if len(moved) != 1 || moved[0].Label != "k3f9" || moved[0].Name().Label != "k3f9" {
		t.Errorf("MovedOf() = %+v, want k3f9 labelled by its id", moved)
	}
}

// Seen from the pool it left, an account moved is named by its label, set
// apart as other, saying where it is now, alike by names and by a moment of
// the pool's document, which holds the pool's accounts alone; seen from the
// pool it's in, it's named as any account configured there.
func TestAnAccountMovedIsNamedByItsLabelAndWhereItIsNow(t *testing.T) {
	moved := status.AccountName{Label: "work", Other: true, Moved: "now in Acme"}
	fromDefault := status.MovedOf(movedPools, movedAccounts, "b2n8")
	names := status.NamesOf(movedAccounts, nil).WithMoved(fromDefault)
	m := routerDocOf(status.Account{ID: "personal", Label: "personal", TokenSet: true, FetchedAt: built}).
		WithMoved(fromDefault).At(built, policy)
	for _, n := range []status.Namer{names, m} {
		if got := n.Named("WORK"); got != moved {
			t.Errorf("%T Named(WORK) seen from Default = %+v, want %+v", n, got, moved)
		}
		if got := n.Named("personal"); got != (status.AccountName{Label: "personal"}) {
			t.Errorf("%T Named(personal) seen from Default = %+v, want it configured there", n, got)
		}
	}
	if got := moved.Said(); got != "work (now in Acme)" {
		t.Errorf("Said() = %q, want work (now in Acme)", got)
	}

	fromAcme := status.NamesOf(movedAccounts, nil).WithMoved(status.MovedOf(movedPools, movedAccounts, "q7mt"))
	acme := routerDocOf(status.Account{ID: "work", Label: "work", TokenSet: true, FetchedAt: built}).
		WithMoved(status.MovedOf(movedPools, movedAccounts, "q7mt")).At(built, policy)
	for _, n := range []status.Namer{fromAcme, acme} {
		if got := n.Named("work"); got != (status.AccountName{Label: "work"}) {
			t.Errorf("%T Named(work) seen from Acme = %+v, want it configured there", n, got)
		}
	}
}
