package status

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/prose"
)

// RemovedAccount is an account the config no longer configures that the
// accounts record keeps, removed: by its id, the label it showed last, which
// names it wherever the history does, and when it was removed.
type RemovedAccount struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Removed time.Time `json:"removed"`
}

// Said says when the account was removed, by its local day, so it's told
// apart from an account configured since under the same label: "removed 4
// Sep".
func (a RemovedAccount) Said() string {
	return "removed " + prose.DayMonth(a.Removed.Local())
}

// RemovedAccounts are the accounts removed, as RemovedOf gives them.
type RemovedAccounts []RemovedAccount

// RemovedOf are the accounts record keeps removed that configured doesn't
// configure, ids compared ignoring case, in record's order.
func RemovedOf(record identity.Record, configured config.Accounts) RemovedAccounts {
	var removed RemovedAccounts
	for _, e := range record.Removed() {
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
