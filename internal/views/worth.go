package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Worth is what requests would have cost through the API, summed exactly:
// Amount, of those the price table priced, left out where it priced none;
// and Unpriced, what Amount leaves out as it can't be priced, never taken
// for free: the paths of the counts it can't price, no_usage where requests
// went upstream without usage, and the id of a model it doesn't know whose
// requests spent anything.
type Worth struct {
	Amount   *ledger.Picodollars `json:"worth,omitempty"`
	Unpriced []string            `json:"unpriced,omitempty"`
}

// addModel adds what a model's day was worth.
func (w *Worth) addModel(m ledger.PricedModel) {
	switch {
	case m.Worth != nil:
		w.addAmount(*m.Worth)
		w.unprice(m.Unpriced...)
	case len(m.Usage) > 0 || m.NoUsage > 0:
		w.unprice(m.Model)
	}
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
