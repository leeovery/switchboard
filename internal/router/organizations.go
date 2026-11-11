package router

import (
	"github.com/leeovery/switchboard/internal/identity"
)

// organizations hold the organization each probe's answer names to its
// account's, as the accounts record has it: the one the account is held to,
// recorded from a probe where it isn't known yet. They hold nothing to it
// while the move onto the accounts' ids is owed, as the config's ids and the
// record's differ till it's done.
type organizations struct {
	record identity.File
	// owed reports whether the move onto the accounts' ids is owed.
	owed func() bool
}

// ownership is what the organization a probe's answer names says of the
// token it went out on, as organizations hold it to its account's.
type ownership int

const (
	// unheld is an organization not held to its account's: none named, the
	// move onto the accounts' ids owed, the accounts record unread, or
	// without the account.
	unheld ownership = iota
	// heldOwn is the account's own organization.
	heldOwn
	// heldNew is an organization the record doesn't know the account's, nor
	// has for another account: the account's, once it's recorded.
	heldNew
	// heldForeign is another subscription's: other than the account's, or,
	// where the account's isn't known, another account's.
	heldForeign
)

// hold holds organization, as a probe's answer on the token of the account
// with the given id named it, to the account's, as the record has it now.
func (o organizations) hold(id, organization string) ownership {
	if organization == "" || o.owed() {
		return unheld
	}
	record, err := o.record.Read()
	if err != nil {
		logger.Debug("can't hold the organization a probe named to the account's", "account", id, "error", err)
		return unheld
	}
	return heldIn(record, id, organization)
}

// take records organization, held new as hold has it, as the account's with
// the given id, and holds it to the account's again as it does, under the
// record's lock, as a token given meanwhile may have recorded another: it's
// heldNew where it's recorded.
func (o organizations) take(id, organization string) ownership {
	held := unheld
	err := o.record.Change(func(r *identity.Record, _ func() error) error {
		if held = heldIn(*r, id, organization); held == heldNew {
			r.Organize(id, organization)
		}
		return nil
	})
	if err != nil {
		logger.Warn("can't record the organization of the account's token", "account", id, "error", err)
		return unheld
	}
	return held
}

// heldIn holds organization, as a probe's answer on the token of the account
// with the given id named it, to the account's, as record has it.
func heldIn(record identity.Record, id, organization string) ownership {
	own, ok := record.ByID(id)
	switch {
	case !ok:
		return unheld
	case own.Organization == organization:
		return heldOwn
	case own.Organization != "":
		return heldForeign
	}
	if _, another := record.ByOrganization(organization); another {
		return heldForeign
	}
	return heldNew
}
