package capture

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// The plans the worlds' accounts are on, as the config names them.
const (
	planUltra = "ultra"
	planMax   = "max"
)

// tenAccounts are the accounts of milestone 7's frames, in config order, as
// the specs number them: work, keeping 5% of every window back, so capped at
// 95%; personal, the primary; side; and the seven more of the frames of six
// and ten accounts. Each is on Ultra but spare, extra and ops, on Max.
var tenAccounts = config.Accounts{
	{ID: "work", Label: "work", Reserve: 0.05, Plan: planUltra},
	{ID: "personal", Label: "personal", Primary: true, Plan: planUltra},
	{ID: "side", Label: "side", Plan: planUltra},
	{ID: "client", Label: "client", Plan: planUltra},
	{ID: "spare", Label: "spare", Plan: planMax},
	{ID: "lab", Label: "lab", Plan: planUltra},
	{ID: "team", Label: "team", Plan: planUltra},
	{ID: "extra", Label: "extra", Plan: planMax},
	{ID: "ops", Label: "ops", Plan: planMax},
	{ID: "studio", Label: "studio", Plan: planUltra},
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
