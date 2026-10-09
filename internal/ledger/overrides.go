package ledger

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// With returns the table with the config's prices in place of its own, the
// table itself left as it is. A price the config gives prices every day
// alike, rather than from a day of its own; one it leaves out keeps the
// table's. A model the table doesn't know is priced only where the config
// gives every kind of token's price, its web searches at the table's price,
// and US-only inference unpriced; With gives, in order, the ids of those it
// passes over, as the config doesn't give every price, for its caller to
// tell of. Overridden is set where any of the config's prices stands in
// place of the table's.
func (t Table) With(prices config.Prices) (Table, []string) {
	t.Models, t.Plans = slices.Clone(t.Models), slices.Clone(t.Plans)
	var passed []string
	for _, id := range slices.Sorted(maps.Keys(prices.Models)) {
		if !t.priceModel(id, prices.Models[id]) {
			passed = append(passed, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(prices.Plans)) {
		t.pricePlan(id, inDollars(prices.Plans[id]))
	}
	return t, passed
}

// priceModel prices the model with the given id as the config does, in
// place of the table's prices of each kind of token it gives: the model's
// every id among them, as they're one model. It reports false for a model
// the table doesn't know whose every price the config doesn't give, which it
// passes over.
func (t *Table) priceModel(id string, given config.ModelPrices) bool {
	switch i := slices.IndexFunc(t.Models, func(m Model) bool { return slices.Contains(m.IDs, id) }); {
	case i < 0 && !given.Complete():
		return false
	case given == (config.ModelPrices{}):
	case i >= 0:
		m := &t.Models[i]
		m.Prices = slices.Clone(m.Prices)
		for j := range m.Prices {
			m.Prices[j] = m.Prices[j].with(given)
		}
		t.Overridden = true
	default:
		t.Models = append(t.Models, Model{IDs: []string{id}, Prices: []Prices{Prices{WebSearch: perThousand(webSearch), USOnlyPercent: globalOnly}.with(given)}})
		t.Overridden = true
	}
	return true
}

// pricePlan prices the plan with the given id at monthly, every day alike.
func (t *Table) pricePlan(id string, monthly Picodollars) {
	i := slices.IndexFunc(t.Plans, func(p Plan) bool { return p.ID == id })
	if i < 0 {
		return
	}
	t.Plans[i].Prices = []PlanPrice{{Monthly: monthly}}
	t.Overridden = true
}

// with returns p with each price given in place of its own.
func (p Prices) with(given config.ModelPrices) Prices {
	for _, set := range []struct {
		price *Picodollars
		given *float64
	}{
		{&p.Input, given.Input}, {&p.Output, given.Output}, {&p.CacheRead, given.CacheRead},
		{&p.CacheWrite5m, given.CacheWrite5m}, {&p.CacheWrite1h, given.CacheWrite1h},
	} {
		if set.given != nil {
			*set.price = perMTok(*set.given)
		}
	}
	return p
}
