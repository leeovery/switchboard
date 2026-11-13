package prose_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// A pool with no accounts says a session started in it doesn't start, by
// its directories, or outside every fence, how to add an account or move one
// in by its id, and how long it holds its directories; and one whose
// accounts the router hasn't taken up, that it takes them up as it
// restarts, and how to restart it now.
func TestAPoolWithNoAccountsSaysWhatDoesntStartAndHowToAddOne(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		dirs    []string
		waiting int
		want    [][]string
	}{
		{name: "fenced", label: "Acme", dirs: []string{"~/Code/acme"}, want: [][]string{
			{"Acme has no accounts, so a session started in ~/Code/acme doesn't start."},
			{"Add one with switchboard accounts add <label> --pool q7mt, or in switchboard setup.", "Move one in with switchboard accounts edit <id> --pool q7mt."},
			{"Its directories stay fenced until the pool is removed with switchboard pool remove q7mt."},
		}},
		{name: "fenced to two directories", label: "Acme", dirs: []string{"~/Code/acme", "~/Code/acme-api"}, want: [][]string{
			{"Acme has no accounts, so a session started in ~/Code/acme or ~/Code/acme-api doesn't start."},
			{"Add one with switchboard accounts add <label> --pool q7mt, or in switchboard setup.", "Move one in with switchboard accounts edit <id> --pool q7mt."},
			{"Its directories stay fenced until the pool is removed with switchboard pool remove q7mt."},
		}},
		{name: "unfenced", label: "Default", want: [][]string{
			{"Default has no accounts, so a session started outside every fence doesn't start."},
			{"Add one with switchboard accounts add <label> --pool q7mt, or in switchboard setup.", "Move one in with switchboard accounts edit <id> --pool q7mt."},
			{"It holds every directory outside a fence until the pool is removed with switchboard pool remove q7mt."},
		}},
		{name: "waiting on the router for one", label: "Acme", dirs: []string{"~/Code/acme"}, waiting: 1, want: [][]string{
			{"Acme has no accounts the router routes yet, so a session started in ~/Code/acme doesn't start."},
			{"The config gives it 1 account, which the router takes up as it restarts, at a quiet moment.", "Restart it now with switchboard service restart."},
			{"Its directories stay fenced until the pool is removed with switchboard pool remove q7mt."},
		}},
		{name: "waiting on the router for two", label: "Default", waiting: 2, want: [][]string{
			{"Default has no accounts the router routes yet, so a session started outside every fence doesn't start."},
			{"The config gives it 2 accounts, which the router takes up as it restarts, at a quiet moment.", "Restart it now with switchboard service restart."},
			{"It holds every directory outside a fence until the pool is removed with switchboard pool remove q7mt."},
		}},
		{name: "its label cleaned", label: "Ac\x1bme", dirs: []string{"~/Code/acme"}, want: [][]string{
			{"Ac me has no accounts, so a session started in ~/Code/acme doesn't start."},
			{"Add one with switchboard accounts add <label> --pool q7mt, or in switchboard setup.", "Move one in with switchboard accounts edit <id> --pool q7mt."},
			{"Its directories stay fenced until the pool is removed with switchboard pool remove q7mt."},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prose.EmptyPool(tt.label, "q7mt", tt.dirs, tt.waiting)
			if !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("EmptyPool(%q, %q) = %q, want %q", tt.label, tt.dirs, got, tt.want)
			}
		})
	}
}

// A title is led by its pool's label where there's one to name, and stands
// as it is where there isn't.
func TestATitleIsLedByItsPoolsLabel(t *testing.T) {
	if got, want := prose.PoolLed("Acme", "k3f9 · Acme 1"), "Acme · k3f9 · Acme 1"; got != want {
		t.Errorf("PoolLed(Acme) = %q, want %q", got, want)
	}
	if got, want := prose.PoolLed("", "k3f9 · Acme 1"), "k3f9 · Acme 1"; got != want {
		t.Errorf("PoolLed() of no pool = %q, want %q", got, want)
	}
}

// A turn that fails says why by the first line of its reason, cleaned, as a
// key line holds one: a config that can't be read gives several.
func TestATurnThatFailsSaysWhyInALine(t *testing.T) {
	tests := []struct{ why, want string }{
		{why: "no config file at ~/.config/switchboard/config.toml", want: "can't turn to the next pool: no config file at ~/.config/switchboard/config.toml"},
		{why: "no config file: write one such as\n\n[[account]]\nid = \"work\"\n", want: "can't turn to the next pool: no config file: write one such as"},
		{why: "a pool \x1b[31mred\x1b[0m", want: "can't turn to the next pool: a pool [31mred [0m"},
	}
	for _, tt := range tests {
		if got := prose.TurnFailed(tt.why); got != tt.want {
			t.Errorf("TurnFailed(%q) = %q, want %q", tt.why, got, tt.want)
		}
	}
}
