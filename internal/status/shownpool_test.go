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

// A pool shown is empty as the moment of its document read has it, once
// one is read, by the router or by probing, an account added since it was
// chosen, or its last taken out, showing at once; before the first read, as
// the config had it.
func TestAPoolShownIsEmptyAsItsDocumentReadHasIt(t *testing.T) {
	one := []status.AccountNow{{ID: "work"}}
	tests := []struct {
		name  string
		pool  status.ShownPool
		mo    status.Moment
		empty bool
	}{
		{name: "none in the config, none read yet", pool: status.ShownPool{ID: "q7mt", Pools: 2}, empty: true},
		{name: "none in the config, one added since, as the router reads", pool: status.ShownPool{ID: "q7mt", Pools: 2}, mo: status.Moment{Routed: true, Accounts: one}},
		{name: "none in the config, one added since, as probed", pool: status.ShownPool{ID: "q7mt", Pools: 2}, mo: status.Moment{Probed: true, Accounts: one}},
		{name: "one in the config, none read yet", pool: status.ShownPool{ID: "q7mt", Accounts: 1, Pools: 2}},
		{name: "one in the config, taken out since", pool: status.ShownPool{ID: "q7mt", Accounts: 1, Pools: 2}, mo: status.Moment{Routed: true}, empty: true},
		{name: "no pool shown", pool: status.ShownPool{Pools: 2}, mo: status.Moment{Routed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pool.AsRead(tt.mo).Empty(); got != tt.empty {
				t.Errorf("%+v, as read, is empty %v; want %v", tt.pool, got, tt.empty)
			}
		})
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
