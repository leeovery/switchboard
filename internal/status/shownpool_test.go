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
// empty with no accounts, as no pool shown never is.
func TestAPoolShownIsNamedOnceThereIsASecond(t *testing.T) {
	tests := []struct {
		name                 string
		pool                 status.ShownPool
		named                string
		turns, empty, fenced bool
	}{
		{name: "the only pool", pool: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 1}},
		{name: "one of two", pool: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 2}, named: "Default", turns: true},
		{name: "fenced, with no accounts", pool: status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Pools: 2}, named: "Acme", turns: true, empty: true, fenced: true},
		{name: "none shown, every pool fenced", pool: status.ShownPool{Pools: 2}, turns: true},
		{name: "no pools", pool: status.ShownPool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.pool
			if p.Named() != tt.named || p.Turns() != tt.turns || p.Empty() != tt.empty || p.Fenced() != tt.fenced {
				t.Errorf("%+v is named %q, turns %v, empty %v, fenced %v; want %q, %v, %v, %v",
					p, p.Named(), p.Turns(), p.Empty(), p.Fenced(), tt.named, tt.turns, tt.empty, tt.fenced)
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
