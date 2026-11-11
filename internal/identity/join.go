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

// JoinAs is how a Mac's record is joined: Configured are the accounts this
// Mac configures; From is the install of the Mac whose record it is, by
// which an account whose id is held here is given another; Own is set where
// it's this Mac's own record, as its own export carries it, whose ids are
// this Mac's; and Labels is set where its labels are taken, as they aren't
// from this Mac's own export, which may be older than a label another Mac
// gave since.
type JoinAs struct {
	Configured config.Accounts
	From       string
	Own        bool
	Labels     bool
}

// Join takes the accounts of another record, as an exported folder carries
// a Mac's, into r, and returns the ids of those it took in under other ids,
// in lower case, each with its id in r. The ids each had before switchboard
// made ids are the Mac's own names, so none is taken, nor matched on. Each
// is matched to r's account of the same organization, else of the same id,
// as same says:
//   - matched, it's that account, which keeps its id, times and removal, and
//     takes its organization where it has none and isn't configured here,
//     as only a join can tell it, a token or the router's probe telling one
//     configured; and its label where the other Mac configures it, as
//     Configured says, and this one neither configures it nor removed it,
//     labels taken, as as says, so a label changed there arrives;
//   - matching none, of an id configured here that r doesn't hold, it's that
//     account, added to r with its organization and time added alone, as an
//     account added on another Mac sharing this one's config;
//   - matching none otherwise, it's added as it is, its label and removal
//     kept, as an account only the other Mac configures or removed there;
//     where its id is held here already, as one of an account of another
//     organization, or one an account had before, under one spelled from
//     the other Mac's install and its id, as Derive spells one, so it's the
//     same however often it's joined.
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
	if i := j.matching(e); i >= 0 {
		return j.take(i, e, labelled)
	}
	if j.configuredUnheld(e) {
		j.record.Accounts = append(j.record.Accounts, Entry{ID: e.ID, Organization: e.Organization, Added: e.Added})
		return e.ID
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

// configuredUnheld reports whether e is the account configured here under
// its id that the record doesn't hold, as Join says.
func (j *joining) configuredUnheld(e Entry) bool {
	return containsFold(j.configured, e.ID) && !j.record.clashes(e.ID, nil)
}

// take has the record's account at place i take what it doesn't know of e,
// the same account as another record keeps it: its organization, where it
// has none and isn't configured here; and its label, where labelled says
// it's to be taken and the account is neither configured here nor removed.
// It returns the account's id.
func (j *joining) take(i int, e Entry, labelled bool) string {
	own := &j.record.Accounts[i]
	if containsFold(j.configured, own.ID) {
		return own.ID
	}
	own.organize(e.Organization)
	if labelled && e.Label != "" && own.Removed.IsZero() {
		own.Label = e.Label
	}
	return own.ID
}

// matching is the place of the account e is, by its organization, else by
// its id where same says it's the same, the id first in this Mac's own
// record, as its ids are this Mac's: -1 where it's none.
func (j *joining) matching(e Entry) int {
	byID := func(own Entry) bool { return strings.EqualFold(own.ID, e.ID) && j.same(own, e) }
	if j.as.Own {
		if i := slices.IndexFunc(j.record.Accounts, byID); i >= 0 {
			return i
		}
	}
	if e.Organization != "" {
		if i := slices.IndexFunc(j.record.Accounts, func(own Entry) bool { return own.Organization == e.Organization }); i >= 0 {
			return i
		}
	}
	return slices.IndexFunc(j.record.Accounts, byID)
}

// same reports whether e, of own's id, is own: always, in this Mac's own
// record, as its ids are this Mac's whatever either knows of the account's
// organization; else as sameByID says.
func (j *joining) same(own, e Entry) bool {
	return j.as.Own || sameByID(own, e)
}

// sameByID reports whether e, of own's id, is own: always, but where both
// organizations are known and differ. Ids are made, never typed, so two
// subscriptions share one only by chance, about one in a million a pair, as
// the design allows ids; and where one's organization isn't known, such two
// are taken for one.
func sameByID(own, e Entry) bool {
	return own.Organization == "" || e.Organization == "" || own.Organization == e.Organization
}

// ids are the record's accounts' ids.
func (r Record) ids() []string {
	ids := make([]string, len(r.Accounts))
	for i, e := range r.Accounts {
		ids[i] = e.ID
	}
	return ids
}
