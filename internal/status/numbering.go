package status

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// Numbering is how every view numbers the accounts, wherever an account's
// name heads a row, each number the key that picks it on the routing card:
// its place in the router's document, from 1, which is the config's order
// once the router has taken the config up; and an account the document
// doesn't hold yet, as one just added to the config, after the document's
// last, in the config's order among those it lacks, the place the router
// gives it once it has it, as accounts are added at the config's end. So an
// account keeps the one number on every tab while the config and the router
// disagree, as after an account is removed while the router waits to
// restart. Its zero value holds no document's accounts, numbering the
// configured accounts in the config's order.
type Numbering struct {
	// held are the ids of the document's accounts, in its order.
	held []string
}

// numbering is the numbering of the document's accounts.
func (d Document) numbering() Numbering {
	held := make([]string, len(d.Accounts))
	for i, a := range d.Accounts {
		held[i] = a.ID
	}
	return Numbering{held: held}
}

// Held is how many accounts the document numbers holds, numbered 1 to it.
func (n Numbering) Held() int {
	return len(n.held)
}

// Configured are the configured accounts given, in the config's order, as
// the config names and marks them, nothing read of them, each with its
// number as its Place.
func (n Numbering) Configured(configured config.Accounts) []AccountNow {
	accounts := make([]AccountNow, len(configured))
	next := len(n.held)
	for i, a := range configured {
		place := slices.Index(n.held, a.ID) + 1
		if place == 0 {
			next++
			place = next
		}
		accounts[i] = AccountNow{ID: a.ID, Label: a.Label, Primary: a.Primary, Place: place}
	}
	return accounts
}

// InPlace reports whether every configured account given is numbered by its
// place in the config, from 1 to their count: as where the config and the
// router agree, or the config has gained accounts at its end the router
// doesn't hold yet, or lost its last while the router holds it still.
func (n Numbering) InPlace(configured config.Accounts) bool {
	for i, a := range n.Configured(configured) {
		if a.Place != i+1 {
			return false
		}
	}
	return true
}

// numbering is the numbering of the moment's document.
func (m Moment) numbering() Numbering {
	return m.doc.numbering()
}

// Configured are the configured accounts given, in the config's order, each
// as the moment has it, else, where the moment lacks it, as one the config
// has just gained, as its numbering's Configured has it; each with its
// number as its Place.
func (m Moment) Configured(configured config.Accounts) []AccountNow {
	accounts := m.numbering().Configured(configured)
	for i, a := range accounts {
		if held, ok := m.Account(a.ID); ok {
			accounts[i] = held
		}
	}
	return accounts
}

// Numbering is the numbering of the document whose moment h holds: nil holds
// none, numbering the configured accounts in the config's order.
func (h *HeldMoment) Numbering() Numbering {
	if h == nil {
		return Numbering{}
	}
	return h.numbering
}
