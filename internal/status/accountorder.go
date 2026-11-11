package status

import (
	"github.com/leeovery/switchboard/internal/config"
)

// Configured are the configured accounts given, in the config's order, the
// order every view that lists what's configured keeps: each as the moment
// holds it; else, one the router doesn't hold yet, as just after it's added
// to the config, as the config names and marks it, nothing read of it and
// no place in the document. So each account keeps the one row on every tab
// while the config and the router disagree, as after an account is removed
// while the router waits to restart.
func (m Moment) Configured(configured config.Accounts) []AccountNow {
	accounts := make([]AccountNow, len(configured))
	for i, a := range configured {
		held, ok := m.Account(a.ID)
		if !ok {
			held = AccountNow{ID: a.ID, Label: a.Label, Primary: a.Primary}
		}
		accounts[i] = held
	}
	return accounts
}
