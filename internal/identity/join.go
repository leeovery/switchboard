package identity

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// ErrConflict is what joining an accounts record that contradicts this one
// fails with, wrapped.
var ErrConflict = errors.New("the accounts records disagree")

// JoinAs is how another Mac's record is joined: Configured are the accounts
// this Mac configures; From is the install of the Mac whose record it is, by
// which an account whose id is held here is given another; and Labels is
// set where its labels are taken, as they aren't from this Mac's own
// export, which may be older than a label another Mac's gave since.
type JoinAs struct {
	Configured config.Accounts
	From       string
	Labels     bool
}

// Join takes the accounts of another record, as an exported folder carries
// another Mac's, into r, and returns the ids of those it took in under other
// ids, in lower case, each with its id in r. The ids each had before
// switchboard made ids are the other Mac's own names, so none is taken, nor
// matched on. Each is matched to r's account of the same organization, else
// of the same id, as sameByID says:
//   - matched, it's that account, which keeps its id, times and removal, and
//     takes its organization where it has none, and its label where the
//     other Mac configures it, as Configured says, and this one neither
//     configures it nor removed it, labels taken, as as says, so a label
//     changed there arrives;
//   - matching none, it's added as it is, its label and removal kept, as an
//     account only the other Mac configures or removed there; where its id
//     is held here already, as one of another account's, or one an account
//     had before, under one spelled from the other Mac's install and its id,
//     as Derive spells one, so it's the same however often it's joined.
//
// It fails, changing nothing, with ErrConflict wrapped where r, joined,
// breaks the record's rules, as check says.
func (r *Record) Join(other Record, as JoinAs) (map[string]string, error) {
	j := joining{record: Record{Accounts: slices.Clone(r.Accounts)}, as: as, configured: as.Configured.IDs(), staged: other.ids()}
	mapped := make(map[string]string)
	for _, e := range other.Accounts {
		if id := j.join(e); !strings.EqualFold(id, e.ID) {
			mapped[strings.ToLower(e.ID)] = id
		}
	}
	if err := j.record.check(); err != nil {
		return nil, fmt.Errorf("%w: joined, %w", ErrConflict, err)
	}
	*r = j.record
	return mapped, nil
}

// joining is a record as Join takes another's accounts into it, as as says:
// the ids configured, and those of the other record's accounts, which an id
// spelled for one is never.
type joining struct {
	record             Record
	as                 JoinAs
	configured, staged []string
}

// join takes the account e into the record, as Join says, and returns the
// id it has there.
func (j *joining) join(e Entry) string {
	labelled := j.as.Labels && e.Configured
	e.Was, e.Configured = "", false
	if i := j.record.matching(e); i >= 0 {
		return j.take(i, e, labelled)
	}
	if !j.record.clashes(e.ID, j.configured) {
		j.record.Accounts = append(j.record.Accounts, e)
		return e.ID
	}
	matched := -1
	id := spelledFrom(j.as.From+"/"+strings.ToLower(e.ID), func(id string) bool {
		if i := slices.IndexFunc(j.record.Accounts, func(own Entry) bool { return strings.EqualFold(own.ID, id) }); i >= 0 {
			if sameByID(j.record.Accounts[i], e) {
				matched = i
				return true
			}
			return false
		}
		return !j.record.clashes(id, j.configured) && !containsFold(j.staged, id)
	})
	if matched >= 0 {
		return j.take(matched, e, labelled)
	}
	e.ID = id
	j.record.Accounts = append(j.record.Accounts, e)
	return id
}

// take has the record's account at place i take what it doesn't know of e,
// the same account as another record keeps it: its organization, where it
// has none, and its label, where labelled says it's to be taken and the
// account is neither configured here nor removed. It returns the account's
// id.
func (j *joining) take(i int, e Entry, labelled bool) string {
	own := &j.record.Accounts[i]
	own.organize(e.Organization)
	if labelled && e.Label != "" && own.Removed.IsZero() && !containsFold(j.configured, own.ID) {
		own.Label = e.Label
	}
	return own.ID
}

// matching is the place of the account e is, by its organization, else by
// its id where sameByID says it's the same: -1 where it's none.
func (r Record) matching(e Entry) int {
	if e.Organization != "" {
		if i := slices.IndexFunc(r.Accounts, func(own Entry) bool { return own.Organization == e.Organization }); i >= 0 {
			return i
		}
	}
	return slices.IndexFunc(r.Accounts, func(own Entry) bool { return strings.EqualFold(own.ID, e.ID) && sameByID(own, e) })
}

// sameByID reports whether e, of own's id, is own: their organizations the
// same where both are known, or where neither is; and where one is known
// alone, where the other's account is removed. Two subscriptions given one
// id at random, as where an organization couldn't be known, are so never
// taken for one while both are used.
func sameByID(own, e Entry) bool {
	switch {
	case own.Organization != "" && e.Organization != "":
		return own.Organization == e.Organization
	case own.Organization == "" && e.Organization == "":
		return true
	case own.Organization == "":
		return !own.Removed.IsZero()
	}
	return !e.Removed.IsZero()
}

// ids are the record's accounts' ids.
func (r Record) ids() []string {
	ids := make([]string, len(r.Accounts))
	for i, e := range r.Accounts {
		ids[i] = e.ID
	}
	return ids
}
