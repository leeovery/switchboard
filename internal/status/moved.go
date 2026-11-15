package status

import (
	"cmp"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
)

// MovedAccount is an account configured in another pool than the one read,
// as one moved out of it: its work recorded in the pool read stays that
// pool's, as every request is recorded under the pool it ran in, and is
// shown as its, by its label and the pool it's in now, by that pool's id
// and label.
type MovedAccount struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Pool      string `json:"pool"`
	PoolLabel string `json:"pool_label"`
}

// Said says where the account is now, as RemovedAccount's Said says when one
// was removed: "now in Acme".
func (a MovedAccount) Said() string {
	return "now in " + prose.Clean(a.PoolLabel)
}

// Name is the account as AccountName names it: by its label, else its id,
// not of the pool read, and where it is now.
func (a MovedAccount) Name() AccountName {
	return AccountName{Label: cmp.Or(a.Label, a.ID), Other: true, Moved: a.Said()}
}

// MovedAccounts are the accounts configured in another pool than the one
// read, as MovedOf gives them.
type MovedAccounts []MovedAccount

// MovedOf are the accounts of configured, in its order, that are in another
// pool than the one with the given id, ids compared ignoring case, each with
// the pool it's in, labelled as pools label it, else by its id: none where
// pools are none or the id is "", as a read of every pool's work has none
// elsewhere, nor an account that names no pool.
func MovedOf(pools config.Pools, configured config.Accounts, pool string) MovedAccounts {
	if len(pools) == 0 || pool == "" {
		return nil
	}
	var moved MovedAccounts
	for _, a := range configured {
		if a.Pool == "" || config.SamePool(a.Pool, pool) {
			continue
		}
		label := a.Pool
		if p, ok := pools.ByID(a.Pool); ok {
			label = cmp.Or(p.Label, p.ID)
		}
		moved = append(moved, MovedAccount{ID: a.ID, Label: a.Label, Pool: a.Pool, PoolLabel: label})
	}
	return moved
}

// Of returns the moved account with the given id, ids compared ignoring
// case, reporting false where it's none of them.
func (m MovedAccounts) Of(id string) (MovedAccount, bool) {
	i := slices.IndexFunc(m, func(a MovedAccount) bool { return strings.EqualFold(a.ID, id) })
	if i < 0 {
		return MovedAccount{}, false
	}
	return m[i], true
}

// named names the account with the given id as AccountName has it,
// reporting false where it's none of them.
func (m MovedAccounts) named(id string) (AccountName, bool) {
	a, ok := m.Of(id)
	if !ok {
		return AccountName{}, false
	}
	return a.Name(), true
}
