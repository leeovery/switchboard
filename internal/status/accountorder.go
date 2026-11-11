package status

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// Order is the accounts' order every view keeps, wherever accounts are
// listed: each account's place in the router's document, from 1, which is
// the config's order once the router has taken the config up; and an
// account the document doesn't hold yet, as one just added to the config,
// after the document's last, in the config's order among those it lacks,
// the place the router gives it once it has it, as accounts are added at
// the config's end. So an account keeps the one place on every tab while
// the config and the router disagree, as after an account is removed while
// the router waits to restart. Its zero value holds no document's accounts,
// placing the configured accounts in the config's order.
type Order struct {
	// held are the ids of the document's accounts, in its order.
	held []string
}

// order is the order of the document's accounts.
func (d Document) order() Order {
	held := make([]string, len(d.Accounts))
	for i, a := range d.Accounts {
		held[i] = a.ID
	}
	return Order{held: held}
}

// Configured are the configured accounts given, in the config's order, as
// the config names and marks them, nothing read of them, each with its
// place in the order as its Place.
func (o Order) Configured(configured config.Accounts) []AccountNow {
	accounts := make([]AccountNow, len(configured))
	next := len(o.held)
	for i, a := range configured {
		place := slices.Index(o.held, a.ID) + 1
		if place == 0 {
			next++
			place = next
		}
		accounts[i] = AccountNow{ID: a.ID, Label: a.Label, Primary: a.Primary, Place: place}
	}
	return accounts
}

// order is the order of the moment's document.
func (m Moment) order() Order {
	return m.doc.order()
}

// Configured are the configured accounts given, in the config's order, each
// as the moment has it, else, where the moment lacks it, as one the config
// has just gained, as its order's Configured has it; each with its place in
// the order as its Place.
func (m Moment) Configured(configured config.Accounts) []AccountNow {
	accounts := m.order().Configured(configured)
	for i, a := range accounts {
		if held, ok := m.Account(a.ID); ok {
			accounts[i] = held
		}
	}
	return accounts
}
