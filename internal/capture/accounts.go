package capture

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// The plans the worlds' accounts are on, as the config names them.
const (
	max20x = "max20x"
	max5x  = "max5x"
)

// tenAccounts are the accounts of milestone 7's frames, in config order, as
// the specs number them: work, keeping 5% of every window back, so capped at
// 95%; personal, the primary; side; and the seven more of the frames of six
// and ten accounts. Each is on Max 20x but spare, extra and ops, on Max 5x.
var tenAccounts = config.Accounts{
	{ID: "work", Label: "work", Reserve: 0.05, Plan: max20x},
	{ID: "personal", Label: "personal", Primary: true, Plan: max20x},
	{ID: "side", Label: "side", Plan: max20x},
	{ID: "client", Label: "client", Plan: max20x},
	{ID: "spare", Label: "spare", Plan: max5x},
	{ID: "lab", Label: "lab", Plan: max20x},
	{ID: "team", Label: "team", Plan: max20x},
	{ID: "extra", Label: "extra", Plan: max5x},
	{ID: "ops", Label: "ops", Plan: max5x},
	{ID: "studio", Label: "studio", Plan: max20x},
}

// firstAccounts are the first n of tenAccounts, a world's own, as the frames
// of one, three, six and ten accounts configure them. With one, work is
// alone, and the primary, as the config makes the first account the primary
// where it marks none.
func firstAccounts(n int) config.Accounts {
	accounts := slices.Clone(tenAccounts[:n])
	if !slices.ContainsFunc(accounts, func(a config.Account) bool { return a.Primary }) {
		accounts[0].Primary = true
	}
	return accounts
}
