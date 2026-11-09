package views_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// tooLargeAcrossDays are two days of work's, each worth $6 million of Claude
// Opus 5.5's input, within what ledger.Picodollars hold, their sum of $12
// million past it; and side's $2 of Claude Sonnet 5.5's.
var tooLargeAcrossDays = daysFrom("2026-10-01",
	day("2026-10-06", account("work", sent("claude-opus-5-5", 1, `{"input_tokens":1500000000000}`))),
	day("2026-10-07", account("side", sent("claude-sonnet-5-5", 1, inputUsage)), account("work", sent("claude-opus-5-5", 1, `{"input_tokens":1500000000000}`))),
)

// A sum of worth past what ledger.Picodollars hold reads as unpriced, never
// wrapped round, in every sum of it: each day as it's read, priced alone,
// stays priced; an account's over the days, every account's, a month's and
// the plans' worth against them don't.
func TestASumOfWorthPastWhatPicodollarsHoldReadsAsUnpriced(t *testing.T) {
	h := views.NewHistory(input(tooLargeAcrossDays))
	tooLarge := []string{ledger.TooLarge}

	for _, d := range h.Days {
		for _, a := range d.Accounts {
			for _, m := range a.Models {
				if m.Worth == nil || len(m.Unpriced) > 0 {
					t.Errorf("%s: %s's %s is worth %v, unpriced %q; want it priced, a day alone within what Picodollars hold", d.Day, a.Account, m.Model, m.Worth, m.Unpriced)
				}
			}
		}
	}
	worths := map[string]views.Worth{}
	for _, a := range h.Totals.Accounts {
		name := a.Account
		if a.All {
			name = "every account's"
		}
		worths[name+" totals"] = a.Worth
	}
	for _, p := range h.Plans {
		name := p.Account
		if p.All {
			name = "every account's"
		}
		worths[name+" plan"] = p.Worth
		if (name == "work" || p.All) && p.Ratio != nil {
			t.Errorf("%s plan has the worth against it %v; want none, its worth too large", name, *p.Ratio)
		}
	}
	october := h.Months[slices.IndexFunc(h.Months, func(m views.Month) bool { return m.Month == "2026-10" })]
	worths["October"] = october.Worth
	if october.Ratio != nil {
		t.Errorf("October has the worth against its plans %v; want none, its worth too large", *october.Ratio)
	}
	for _, name := range []string{"work totals", "every account's totals", "work plan", "every account's plan", "October"} {
		if w := worths[name]; w.Amount != nil || !slices.Equal(w.Unpriced, tooLarge) || w.Cents() != "unpriced" || w.Dollars() != "unpriced" {
			t.Errorf("%s worth is %v, unpriced %q, said %s and %s; want none, unpriced as %q, said unpriced", name, w.Amount, w.Unpriced, w.Cents(), w.Dollars(), tooLarge)
		}
	}
	if w := worths["side totals"]; w.Amount == nil || *w.Amount != *usd(2) || len(w.Unpriced) > 0 {
		t.Errorf("side's worth is %v, unpriced %q; want $2, its own within what Picodollars hold", w.Amount, w.Unpriced)
	}
}

// A sum past what ledger.Picodollars hold stays unpriced whatever is added
// after it, as it's taken in.
func TestAWorthTooLargeStaysUnpricedWhateverComesAfter(t *testing.T) {
	h := views.NewHistory(input(daysFrom("2026-10-01",
		day("2026-10-05", account("work", sent("claude-opus-5-5", 1, `{"input_tokens":1500000000000}`))),
		day("2026-10-06", account("work", sent("claude-opus-5-5", 1, `{"input_tokens":1500000000000}`))),
		day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage))),
	)))
	work := h.Totals.Accounts[slices.IndexFunc(h.Totals.Accounts, func(a views.AccountTotals) bool { return a.Account == "work" })]
	if work.Amount != nil || !slices.Equal(work.Unpriced, []string{ledger.TooLarge}) {
		t.Errorf("work's worth is %v, unpriced %q; want none, unpriced as too large, the $4 after it never starting a sum afresh", work.Amount, work.Unpriced)
	}
}

// Plans' prices and costs that pass what ledger.Picodollars hold together
// are left out, as where no plan is known, never wrapped round: ten accounts
// on a plan the config prices at its most, $1,000,000 a month, cost $10
// million a month together. Each one's own stands.
func TestPlansCostingMoreTogetherThanPicodollarsHoldAreLeftOut(t *testing.T) {
	var many config.Accounts
	for i := range 10 {
		many = append(many, config.Account{ID: fmt.Sprintf("acct-%d", i), Plan: "ultra"})
	}
	table := overridden(t, config.Prices{Plans: map[string]float64{"ultra": 1_000_000}})

	most := views.PlansAtMost(many, table)
	if all := most[len(most)-1]; all.Price != nil || all.Monthly() != "" {
		t.Errorf("every account's plans at their most cost %s a month, want it left out", all.Monthly())
	}
	in := input(sinceJuly)
	in.From, in.Prices, in.Accounts = on("2026-09-08"), table, many
	plans := views.NewHistory(in).Plans
	if one := plans[0]; one.Cost == nil || *one.Cost != 1_000_000*ledger.Dollar {
		t.Errorf("an account's plan costs %v over the month, want $1,000,000, its own within what Picodollars hold", one.Cost)
	}
	if all := plans[len(plans)-1]; all.Cost != nil || all.Ratio != nil || all.Price != nil {
		t.Errorf("every account's plans cost %v, %v a month, against %v; want each left out", all.Cost, all.Price, all.Ratio)
	}
}
