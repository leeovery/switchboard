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
// and US-only inference unpriced. Overridden is set where any of the
// config's prices stands in place of the table's.
func (t Table) With(prices config.Prices) Table {
	t.Models, t.Plans = slices.Clone(t.Models), slices.Clone(t.Plans)
	for _, id := range slices.Sorted(maps.Keys(prices.Models)) {
		t.priceModel(id, prices.Models[id])
	}
	for _, id := range slices.Sorted(maps.Keys(prices.Plans)) {
		t.pricePlan(id, inDollars(prices.Plans[id]))
	}
	return t
}

// priceModel prices the model with the given id as the config does, in
// place of the table's prices of each kind of token it gives: the model's
// every id among them, as they're one model.
func (t *Table) priceModel(id string, given config.ModelPrices) {
	if given == (config.ModelPrices{}) {
		return
	}
	switch i := slices.IndexFunc(t.Models, func(m Model) bool { return slices.Contains(m.IDs, id) }); {
	case i >= 0:
		m := &t.Models[i]
		m.Prices = slices.Clone(m.Prices)
		for j := range m.Prices {
			m.Prices[j] = m.Prices[j].with(given)
		}
	case given.Complete():
		t.Models = append(t.Models, Model{IDs: []string{id}, Prices: []Prices{Prices{WebSearch: perThousand(webSearch), USOnlyPercent: globalOnly}.with(given)}})
	default:
		return
	}
	t.Overridden = true
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
