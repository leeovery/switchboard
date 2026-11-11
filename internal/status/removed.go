package status

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/prose"
)

// RemovedAccount is an account the accounts record keeps that the config
// doesn't configure: one removed, or one only another Mac configures, as an
// import joins its record in. It's kept by its id, its label, which names it
// wherever the history does, and, where it was removed, when.
type RemovedAccount struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Removed time.Time `json:"removed,omitzero"`
}

// Said says when the account was removed, by its local day, so it's told
// apart from an account configured since under the same label: "removed 4
// Sep". It's "" of one never removed.
func (a RemovedAccount) Said() string {
	if a.Removed.IsZero() {
		return ""
	}
	return "removed " + prose.DayMonth(a.Removed.Local())
}

// RemovedAccounts are the accounts removed, as RemovedOf gives them.
type RemovedAccounts []RemovedAccount

// RemovedOf are the accounts record keeps that configured doesn't configure,
// ids compared ignoring case, in record's order.
func RemovedOf(record identity.Record, configured config.Accounts) RemovedAccounts {
	var removed RemovedAccounts
	for _, e := range record.Accounts {
		if !slices.ContainsFunc(configured, func(a config.Account) bool { return strings.EqualFold(a.ID, e.ID) }) {
			removed = append(removed, RemovedAccount{ID: e.ID, Label: e.Label, Removed: e.Removed})
		}
	}
	return removed
}

// Of returns the removed account with the given id, reporting false where
// it's none of them.
func (r RemovedAccounts) Of(id string) (RemovedAccount, bool) {
	i := slices.IndexFunc(r, func(a RemovedAccount) bool { return a.ID == id })
	if i < 0 {
		return RemovedAccount{}, false
	}
	return r[i], true
}

// AccountName is what an account is called wherever it's named by its id.
type AccountName struct {
	// Label names it: as configured, where it's configured; else as the
	// accounts record keeps it; else its id.
	Label string
	// Other is set on an account the accounts record keeps that isn't
	// configured: one removed, or one only another Mac configures.
	Other bool
	// Removed says when it was removed, as RemovedAccount's Said says: "" of
	// one never removed.
	Removed string
}

// Named names the account with the given id, configured as configured says,
// as AccountName has it.
func (r RemovedAccounts) Named(configured config.Accounts, id string) AccountName {
	if i := slices.IndexFunc(configured, func(a config.Account) bool { return a.ID == id }); i >= 0 {
		return AccountName{Label: cmp.Or(configured[i].Label, id)}
	}
	return r.named(id)
}

// named names the account with the given id, which isn't configured, as
// AccountName has it.
func (r RemovedAccounts) named(id string) AccountName {
	a, ok := r.Of(id)
	if !ok {
		return AccountName{Label: id}
	}
	return AccountName{Label: cmp.Or(a.Label, id), Other: true, Removed: a.Said()}
}

// Named names the account of the moment with the given id, as AccountName
// has it: configured where its document holds it, else as the accounts
// removed its document was given, as Document's WithRemoved gives them.
func (m Moment) Named(id string) AccountName {
	if a, ok := m.Account(id); ok {
		return AccountName{Label: cmp.Or(a.Label, id)}
	}
	return m.doc.removed.named(id)
}
