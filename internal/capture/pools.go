package capture

import (
	"context"
	"errors"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
)

// The pools of the pools' frames: the first, Default, the auto world's;
// Acme, a client's, fenced to ~/Code/acme, its three accounts the auto
// world's labelled Acme 1, Acme 2 and Acme 3; and Acme with none.
var (
	defaultPool = status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 2}
	acmePool    = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Accounts: 3, Pools: 2}
	emptyPool   = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Pools: 2}
)

// acmeLabels are the labels Acme gives the auto world's accounts, by their
// ids.
var acmeLabels = map[string]string{"work": "Acme 1", "personal": "Acme 2", "side": "Acme 3"}

// acme is the auto world as pool Acme has it: its accounts labelled as
// acmeLabels has them, so every view names them so.
func (a accountWorlds) acme() accountWorld {
	return a.kept("acme", func() accountWorld {
		w := a.auto()
		for i, account := range w.accounts {
			w.accounts[i].Label = acmeLabels[account.ID]
		}
		return w
	})
}

// noAccounts is the world of a pool with no accounts: the router healthy,
// with nothing to route.
func (a accountWorlds) noAccounts() accountWorld {
	return a.kept("noAccounts", func() accountWorld { return accountWorld{} })
}

// errOnePool is what P's turn fails with in a capture, which shows its
// fixture's pool alone.
var errOnePool = errors.New("a capture shows its fixture's pool alone")

// turnNowhere is a capture's Turn, where its fixture's pool is one of
// several: it lists P pool as the dashboard does, and turns nowhere.
func turnNowhere(context.Context, string) (watch.Pooled, error) {
	return watch.Pooled{}, errOnePool
}
