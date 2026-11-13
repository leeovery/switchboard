package status_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
)

// shownPools are the dashboard's pools in these tests: Default, unfenced,
// with side in it; Acme, fenced, with work in it; and Client, fenced to two
// directories, with none.
var shownPools = config.Pools{
	{ID: "b2n8", Label: "Default"},
	{ID: "q7mt", Label: "Acme", Dirs: []string{"/home/me/Code/acme"}, WrittenDirs: []string{"~/Code/acme"}},
	{ID: "r2d4", Label: "Client", Dirs: []string{"/home/me/Code/client", "/home/me/Code/client-api"}, WrittenDirs: []string{"~/Code/client", "~/Code/client-api"}},
}

var shownAccounts = config.Accounts{{ID: "work", Label: "Work", Pool: "q7mt"}, {ID: "side", Label: "Side", Pool: "B2N8"}}

// A pool shown is the config's, by its id, ignoring case: its label, its
// directories as the config writes them, and its accounts and the pools
// counted; and none but the count where no pool has the id.
func TestAPoolShownIsTheConfigsByItsID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want status.ShownPool
	}{
		{name: "unfenced", id: "b2n8", want: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 1, Pools: 3}},
		{name: "fenced, its id in another case", id: "Q7MT", want: status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Accounts: 1, Pools: 3}},
		{name: "with no accounts", id: "r2d4", want: status.ShownPool{ID: "r2d4", Label: "Client", Dirs: []string{"~/Code/client", "~/Code/client-api"}, Pools: 3}},
		{name: "none named", id: "", want: status.ShownPool{Pools: 3}},
		{name: "one not configured", id: "x9z9", want: status.ShownPool{Pools: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := status.PoolShown(shownPools, shownAccounts, tt.id)
			if got.ID != tt.want.ID || got.Label != tt.want.Label || !slices.Equal(got.Dirs, tt.want.Dirs) || got.Accounts != tt.want.Accounts || got.Pools != tt.want.Pools {
				t.Errorf("PoolShown(%q) = %+v, want %+v", tt.id, got, tt.want)
			}
		})
	}
}

// A pool shown is named, turns to another and is fenced only as its pools
// say: named and turning once there's a second, never with no pool shown;
// pooled with any; empty with no accounts, as no pool shown never is.
func TestAPoolShownIsNamedOnceThereIsASecond(t *testing.T) {
	tests := []struct {
		name                         string
		pool                         status.ShownPool
		named                        string
		turns, pooled, empty, fenced bool
	}{
		{name: "the only pool", pool: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 1}, pooled: true},
		{name: "one of two", pool: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 2}, named: "Default", turns: true, pooled: true},
		{name: "fenced, with no accounts", pool: status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Pools: 2}, named: "Acme", turns: true, pooled: true, empty: true, fenced: true},
		{name: "none shown, every pool fenced", pool: status.ShownPool{Pools: 2}, turns: true, pooled: true},
		{name: "none shown, the only pool fenced", pool: status.ShownPool{Pools: 1}, pooled: true},
		{name: "no pools", pool: status.ShownPool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.pool
			if p.Named() != tt.named || p.Turns() != tt.turns || p.Pooled() != tt.pooled || p.Empty() != tt.empty || p.Fenced() != tt.fenced {
				t.Errorf("%+v is named %q, turns %v, pooled %v, empty %v, fenced %v; want %q, %v, %v, %v, %v",
					p, p.Named(), p.Turns(), p.Pooled(), p.Empty(), p.Fenced(), tt.named, tt.turns, tt.pooled, tt.empty, tt.fenced)
			}
		})
	}
}

// A screen names the pool it tells of once there's a second, never with
// one or none.
func TestThePoolIsNamedOnceThereIsASecond(t *testing.T) {
	for pools, want := range []bool{false, false, true, true} {
		if got := status.NamesThePool(pools); got != want {
			t.Errorf("NamesThePool(%d) = %v, want %v", pools, got, want)
		}
	}
}

// The accounts a dashboard shows are the pool's alone, by its id, ignoring
// case: every one in a config without pools, and none with no pool shown,
// so no pool's accounts are listed in another's.
func TestTheAccountsShownAreThePoolsAlone(t *testing.T) {
	tests := []struct {
		name  string
		pools config.Pools
		id    string
		want  []string
	}{
		{name: "Acme", pools: shownPools, id: "q7mt", want: []string{"work"}},
		{name: "Default, its id in another case", pools: shownPools, id: "B2N8", want: []string{"side"}},
		{name: "Client, with none", pools: shownPools, id: "r2d4"},
		{name: "none shown, every pool fenced", pools: shownPools},
		{name: "one not configured", pools: shownPools, id: "x9z9"},
		{name: "a config without pools", want: []string{"work", "side"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.PoolAccounts(tt.pools, shownAccounts, tt.id).IDs(); !slices.Equal(got, tt.want) {
				t.Errorf("PoolAccounts(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// A pool shown is empty as the router's document of it has it, once one is
// read: the accounts it routes there, ignoring the id's case. One the config
// gives accounts the router hasn't taken up is waiting on it. A probed
// document, one of another pool, and one of none, as a router that can't
// tell its pools answers, leave it as the config has it.
func TestAPoolShownIsEmptyAsTheRoutersDocumentOfItHasIt(t *testing.T) {
	one := []status.AccountNow{{ID: "work"}}
	acme := status.ShownPool{ID: "q7mt", Label: "Acme", Pools: 2}
	acmeOfOne := acme
	acmeOfOne.Accounts = 1
	tests := []struct {
		name           string
		pool           status.ShownPool
		mo             status.Moment
		empty, waiting bool
	}{
		{name: "none in the config, none read yet", pool: acme, empty: true},
		{name: "none in the config, one routed since", pool: acme, mo: status.Moment{Routed: true, PoolID: "Q7MT", Accounts: one}},
		{name: "one in the config, none read yet", pool: acmeOfOne},
		{name: "one in the config, none routed yet", pool: acmeOfOne, mo: status.Moment{Routed: true, PoolID: "q7mt"}, empty: true, waiting: true},
		{name: "one in the config, routed", pool: acmeOfOne, mo: status.Moment{Routed: true, PoolID: "q7mt", Accounts: one}},
		{name: "one in the config, probed with none", pool: acmeOfOne, mo: status.Moment{Probed: true, PoolID: "q7mt"}},
		{name: "none in the config, probed with one", pool: acme, mo: status.Moment{Probed: true, PoolID: "q7mt", Accounts: one}, empty: true},
		{name: "one in the config, another pool's document", pool: acmeOfOne, mo: status.Moment{Routed: true, PoolID: "b2n8"}},
		{name: "one in the config, a document of no pool", pool: acmeOfOne, mo: status.Moment{Routed: true}},
		{name: "no pool shown", pool: status.ShownPool{Pools: 2}, mo: status.Moment{Routed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pool.AsRead(tt.mo)
			if got.Empty() != tt.empty || got.Waiting() != tt.waiting {
				t.Errorf("%+v, as read, is empty %v, waiting %v; want %v, %v", tt.pool, got.Empty(), got.Waiting(), tt.empty, tt.waiting)
			}
		})
	}
}

// The pool shown is read again by its id, ignoring case; where it's gone,
// it's the one P would turn to from it.
func TestThePoolShownIsReadAgainElseTheOnePWouldTurnTo(t *testing.T) {
	tests := []struct{ id, want string }{
		{id: "Q7MT", want: "q7mt"},
		{id: "x9z9", want: "b2n8"},
	}
	for _, tt := range tests {
		if got, ok := status.PoolAgain(shownPools, tt.id); !ok || got.ID != tt.want {
			t.Errorf("PoolAgain(%q) = %q, %v; want %q", tt.id, got.ID, ok, tt.want)
		}
	}
	if got, ok := status.PoolAgain(nil, "q7mt"); ok {
		t.Errorf("PoolAgain() of no pools = %q, want none", got.ID)
	}
}

// P turns to the next pool in the config's order, the first after the last,
// and to the first from none shown or one no longer configured.
func TestPTurnsToTheNextPoolInTheConfigsOrder(t *testing.T) {
	tests := []struct {
		from, want string
	}{
		{from: "b2n8", want: "q7mt"},
		{from: "Q7MT", want: "r2d4"},
		{from: "r2d4", want: "b2n8"},
		{from: "", want: "b2n8"},
		{from: "x9z9", want: "b2n8"},
	}
	for _, tt := range tests {
		if got, ok := status.PoolAfter(shownPools, tt.from); !ok || got.ID != tt.want {
			t.Errorf("PoolAfter(%q) = %q, %v; want %q", tt.from, got.ID, ok, tt.want)
		}
	}
	if got, ok := status.PoolAfter(nil, "b2n8"); ok {
		t.Errorf("PoolAfter() of no pools = %q, want none", got.ID)
	}
}

// A moment's accounts have changed where the moment before, read, held
// others, by their ids; never against one before any read.
func TestAMomentsAccountsHaveChangedWhereTheReadBeforeHeldOthers(t *testing.T) {
	work, side := status.AccountNow{ID: "work"}, status.AccountNow{ID: "side"}
	read := func(accounts ...status.AccountNow) status.Moment {
		return status.Moment{Routed: true, Accounts: accounts}
	}
	tests := []struct {
		name        string
		before, now status.Moment
		changed     bool
	}{
		{name: "the same", before: read(work, side), now: read(work, side)},
		{name: "one taken up", before: read(work), now: read(work, side), changed: true},
		{name: "one let go", before: read(work, side), now: read(work), changed: true},
		{name: "probed before", before: status.Moment{Probed: true, Accounts: []status.AccountNow{work}}, now: read(side), changed: true},
		{name: "none read before", now: read(work)},
	}
	for _, tt := range tests {
		if got := tt.now.AccountsChanged(tt.before); got != tt.changed {
			t.Errorf("%s: AccountsChanged() = %v, want %v", tt.name, got, tt.changed)
		}
	}
}
