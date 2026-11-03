package capture

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// tenAccounts are the accounts of milestone 7's frames, in config order, as
// the specs number them: work, keeping 5% of every window back, so capped at
// 95%; personal, the primary; side; and the seven more of the frames of six
// and ten accounts. Each is on Ultra but spare, extra and ops, on Max.
var tenAccounts = config.Accounts{
	{ID: "work", Label: "work", Reserve: 0.05, Plan: config.PlanUltra},
	{ID: "personal", Label: "personal", Primary: true, Plan: config.PlanUltra},
	{ID: "side", Label: "side", Plan: config.PlanUltra},
	{ID: "client", Label: "client", Plan: config.PlanUltra},
	{ID: "spare", Label: "spare", Plan: config.PlanMax},
	{ID: "lab", Label: "lab", Plan: config.PlanUltra},
	{ID: "team", Label: "team", Plan: config.PlanUltra},
	{ID: "extra", Label: "extra", Plan: config.PlanMax},
	{ID: "ops", Label: "ops", Plan: config.PlanMax},
	{ID: "studio", Label: "studio", Plan: config.PlanUltra},
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
