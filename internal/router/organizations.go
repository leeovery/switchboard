package router

import (
	"github.com/leeovery/switchboard/internal/identity"
)

// organizations hold the organization each probe's answer names to its
// account's, as the accounts record has it, as the record's Hold holds it:
// the one the account is held to, recorded from a probe where it isn't known
// yet. They hold nothing to it while the move onto the accounts' ids is
// owed, as the config's ids and the record's differ till it's done.
type organizations struct {
	record identity.File
	// owed reports whether the move onto the accounts' ids is owed.
	owed func() bool
}

// hold holds organization, as a probe's answer on the token of the account
// with the given id named it, to the account's, as the record has it now:
// unheld where none's named, the move's owed, or the record can't be read.
func (o organizations) hold(id, organization string) identity.Holding {
	if organization == "" || o.owed() {
		return identity.Unheld
	}
	record, err := o.record.Read()
	if err != nil {
		logger.Debug("can't hold the organization a probe named to the account's", "account", id, "error", err)
		return identity.Unheld
	}
	held, _ := record.Hold(id, organization)
	return held
}

// take records organization, held new as hold has it, as the account's with
// the given id, and holds it to the account's again as it does, under the
// record's lock, as a token given meanwhile may have recorded another: it's
// held new where it's recorded.
func (o organizations) take(id, organization string) identity.Holding {
	held := identity.Unheld
	err := o.record.Change(func(r *identity.Record, _ func() error) error {
		if held, _ = r.Hold(id, organization); held == identity.HeldNew {
			r.Organize(id, organization)
		}
		return nil
	})
	if err != nil {
		logger.Warn("can't record the organization of the account's token", "account", id, "error", err)
		return identity.Unheld
	}
	return held
}
