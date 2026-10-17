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
	w.addSummed(prices.Summed(l, date))
	return w
}

// addSummed adds what a request was worth, as ledger.Table.Summed gives it:
// its cost, where the table priced it, and what it left unpriced.
func (w *Worth) addSummed(worth ledger.Worth, priced bool) {
	if priced {
		w.addAmount(worth.Cost)
	}
	w.unprice(worth.Unpriced...)
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

// How a worth is said where it isn't all priced: PartUnpriced after the
// priced part's, where part of it is unpriced; and unpriced where none of it
// is priced.
const (
	PartUnpriced = "+"
	unpriced     = "unpriced"
)

// underACent is a worth more than nothing that rounds to nothing, to the
// cent, so it reads as more.
const underACent = "<$0.01"

// Cents is the worth to the cent, as a request's, a turn's or a session's is
// said, the Log's rule: "$0.15"; "<$0.01" where it's more than nothing but
// under half a cent, which would read as nothing; "unpriced" where none of it
// is priced; and PartUnpriced after it where part of it is, as "$17.86+".
// Nothing at all is "$0.00".
func (w Worth) Cents() string {
	return w.centsAt(w.amount().ToTheCent())
}

// centsAt says the worth as Cents does, its amount to the cent the one
// given, as a part's is where its rounding is reconciled to a whole's, as
// toTheCent reconciles it: "<$0.01" where that's none but the worth is more.
func (w Worth) centsAt(rounded ledger.Picodollars) string {
	if w.Amount == nil && len(w.Unpriced) > 0 {
		return unpriced
	}
	said := rounded.Cents()
	if w.amount() > 0 && rounded == 0 {
		said = underACent
	}
	if len(w.Unpriced) > 0 {
		said += PartUnpriced
	}
	return said
}

// toTheCent are the amounts given, parts of a whole, each to the cent, so
// together they're the whole to the cent, as Cents says it: each is the
// cents its running sum through it comes to, less those the sum before it
// does, as a stacked bar shares its cells among its parts. A part can so
// come a cent off what it'd be to the cent alone, never under nothing.
func toTheCent(amounts []ledger.Picodollars) []ledger.Picodollars {
	rounded := make([]ledger.Picodollars, len(amounts))
	var sum, before ledger.Picodollars
	for i, a := range amounts {
		sum += a
		through := sum.ToTheCent()
		rounded[i], before = through-before, through
	}
	return rounded
}

// rewriteSaid says what writing a moved session's context again on the
// account it moved to cost, its cache write, exactly, as Cents says it,
// "$1.82"; "unpriced" where the context was written again, as rewrote says,
// but the price table can't price it, cost nil; and "" where it wasn't, as
// no request has yet, or the cache would have run out anyway. It's every
// view's saying of a move's cost.
func rewriteSaid(rewrote bool, cost *ledger.Picodollars) string {
	switch {
	case cost != nil:
		return Worth{Amount: cost}.Cents()
	case rewrote:
		return unpriced
	}
	return ""
}

// Dollars is the worth to the dollar, as History says it, its thousands set
// apart, as "$27,480"; PartUnpriced after it where part of it is unpriced, as
// "$184+"; "unpriced" where none of it is priced; and "" where there's none.
func (w Worth) Dollars() string {
	switch {
	case w.Amount == nil && len(w.Unpriced) > 0:
		return unpriced
	case w.Amount == nil:
		return ""
	case len(w.Unpriced) > 0:
		return w.Amount.GroupedDollars() + PartUnpriced
	}
	return w.Amount.GroupedDollars()
}

// amount is what the priced requests cost: none where none was priced.
func (w Worth) amount() ledger.Picodollars {
	if w.Amount == nil {
		return 0
	}
	return *w.Amount
}

// shareOf is part's share of whole, as a family's worth of every family's,
// or a version's of all use's: none where none of part is priced, or whole
// is worth nothing.
func shareOf(part Worth, whole ledger.Picodollars) *float64 {
	if part.Amount == nil || whole <= 0 {
		return nil
	}
	share := float64(*part.Amount) / float64(whole)
	return &share
}
