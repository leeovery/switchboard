package launch_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
)

// The pools a config's accounts are in are the config's, where it names any
// or the move to pools isn't done; else the router's, as its health answer
// reports them; else the accounts record's; else none can be told. The
// router and the record are asked only where they're needed.
func TestThePoolsOfAConfigAreItsElseTheRoutersElseTheRecords(t *testing.T) {
	unpooled := &config.Config{Accounts: config.Accounts{{ID: "work"}, {ID: "side"}}}
	pooled := &config.Config{Accounts: config.Accounts{{ID: "work", Pool: "b2n8"}}, Pools: config.Pools{{ID: "b2n8", Label: "Default"}}}
	routed := router.Health{OK: true, ReportsPools: true, Pools: []router.RoutedPool{
		{ID: "b2n8", Label: "Default", Accounts: []string{"side"}, Primary: "side"},
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/work/acme"}, Accounts: []string{"work"}, Primary: "work"},
	}}
	kept := &config.Config{Accounts: config.Accounts{{ID: "work", Pool: "h4w2"}, {ID: "side", Pool: "h4w2"}}, Pools: config.Pools{{ID: "h4w2", Label: "Kept"}}}
	tests := []struct {
		name    string
		cfg     *config.Config
		moved   bool
		health  router.Health
		kept    *config.Config
		want    map[string]string
		ok      bool
		asked   int
		keptRan int
	}{
		{name: "before the move, a config naming none as it is", cfg: unpooled, health: routed, kept: kept, want: map[string]string{"work": "", "side": ""}, ok: true},
		{name: "a config naming pools, its own", cfg: pooled, moved: true, health: routed, kept: kept, want: map[string]string{"work": "b2n8"}, ok: true},
		{name: "after the move, the router's", cfg: unpooled, moved: true, health: routed, kept: kept, want: map[string]string{"work": "q7mt", "side": "b2n8"}, ok: true, asked: 1},
		{name: "after the move, the router down, the record's", cfg: unpooled, moved: true, kept: kept, want: map[string]string{"work": "h4w2", "side": "h4w2"}, ok: true, asked: 1, keptRan: 1},
		{name: "after the move, none naming any", cfg: unpooled, moved: true, asked: 1, keptRan: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked, keptRan int
			health := func() router.Health { asked++; return tt.health }
			keeps := func(*config.Config) (*config.Config, bool) { keptRan++; return tt.kept, tt.kept != nil }
			got, ok := launch.PoolsOf(tt.cfg, tt.moved, health, keeps)
			if ok != tt.ok || asked != tt.asked || keptRan != tt.keptRan {
				t.Fatalf("PoolsOf() reports %v, asking the router %d times and the record %d; want %v, %d and %d", ok, asked, keptRan, tt.ok, tt.asked, tt.keptRan)
			}
			if !ok {
				return
			}
			for id, pool := range tt.want {
				if a, _ := got.Accounts.ByID(id); a.Pool != pool {
					t.Errorf("PoolsOf() has %s in pool %q, want %q", id, a.Pool, pool)
				}
			}
		})
	}
}
