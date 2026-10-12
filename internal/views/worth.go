package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Worth is what requests would have cost through the API, summed exactly,
// every verb's sum of it, a day's model's or a line's: Amount, of those the
// price table priced, left out where it priced none; and Unpriced, what
// Amount leaves out as it can't be priced, never taken for free, as the
// ledger names it: the paths of the counts it can't price, no_usage where
// requests went upstream without usage, and of requests it prices none of
// that spent anything, the id of a model it doesn't know, inference_geo and
// the geo where it doesn't price a model in the geo they asked for, and
// no_model where they named none.
type Worth struct {
	Amount   *ledger.Picodollars `json:"worth,omitempty"`
	Unpriced []string            `json:"unpriced,omitempty"`
}

// summed is what the line's request was worth, at the prices in effect on
// the local day with the given date, as ledger.Table.Summed gives it: no
// Amount where the table priced none of it.
func summed(prices ledger.Table, l *ledger.Line, date string) Worth {
	var w Worth
	worth, priced := prices.Summed(l, date)
	if priced {
		w.addAmount(worth.Cost)
	}
	w.unprice(worth.Unpriced...)
	return w
}

// addModel adds what a model's day was worth.
func (w *Worth) addModel(m ledger.PricedModel) {
	if m.Worth != nil {
		w.addAmount(*m.Worth)
	}
	w.unprice(m.Unpriced...)
}

// add adds another sum.
func (w *Worth) add(other Worth) {
	if other.Amount != nil {
		w.addAmount(*other.Amount)
	}
	w.unprice(other.Unpriced...)
}

// addAmount adds amount to what the priced requests cost. It sets Amount
// afresh rather than through its pointer, which a copy of w may share.
func (w *Worth) addAmount(amount ledger.Picodollars) {
	if w.Amount != nil {
		amount += *w.Amount
	}
	w.Amount = &amount
}

// unprice names what's unpriced among Unpriced, each once, in order.
func (w *Worth) unprice(names ...string) {
	for _, name := range names {
		if i, found := slices.BinarySearch(w.Unpriced, name); !found {
			w.Unpriced = slices.Insert(slices.Clip(w.Unpriced), i, name)
		}
	}
}

// amount is what the priced requests cost: none where none was priced.
func (w Worth) amount() ledger.Picodollars {
	if w.Amount == nil {
		return 0
	}
	return *w.Amount
}

// shareOf is the worth as a share of whole, as a family's of every family's,
// or a version's of all use's: none where none of the worth is priced, or
// whole is worth nothing.
func (w Worth) shareOf(whole ledger.Picodollars) *float64 {
	if w.Amount == nil || whole <= 0 {
		return nil
	}
	share := float64(*w.Amount) / float64(whole)
	return &share
}
