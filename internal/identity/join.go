package identity

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// ErrConflict is what joining an accounts record that contradicts this one
// fails with, wrapped.
var ErrConflict = errors.New("the accounts records disagree")

// Join takes the accounts of another record, as an exported folder carries
// another Mac's, into r, and returns the ids of those it took in under other
// ids, in lower case, each with its id in r. The ids each had before
// switchboard made ids are the other Mac's own names, so none is taken, nor
// matched on. Each is matched to r's account of the same organization, else
// of the same id, but where both organizations are known and differ:
//   - matched, it's that account, which keeps its id, times and removal, and
//     takes its organization where it has none, and its label where it's
//     neither configured, as configured says, nor removed, so a label
//     changed on the other Mac arrives;
//   - matching none, it's added as it is, its label and removal kept, as an
//     account only the other Mac configures or removed there; under a random
//     id drawn from random where its own is held already, as one of
//     another organization's here, or one an account had before.
//
// It fails, changing nothing, where a random id can't be drawn, and with
// ErrConflict wrapped where r, joined, breaks the record's rules, as check
// says.
func (r *Record) Join(other Record, configured config.Accounts, random io.Reader) (map[string]string, error) {
	j := joining{
		record:     Record{Accounts: slices.Clone(r.Accounts)},
		configured: configured.IDs(),
		random:     random,
	}
	j.held = slices.Concat(j.configured, other.ids())
	mapped := make(map[string]string)
	for _, e := range other.Accounts {
		id, err := j.join(e)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(id, e.ID) {
			mapped[strings.ToLower(e.ID)] = id
		}
	}
	if err := j.record.check(); err != nil {
		return nil, fmt.Errorf("%w: joined, %w", ErrConflict, err)
	}
	*r = j.record
	return mapped, nil
}

// joining is a record as Join takes another's accounts into it: the ids
// configured, those of the other record's accounts, which a random id is
// never drawn as, and what one is drawn from.
type joining struct {
	record     Record
	configured []string
	held       []string
	random     io.Reader
}

// join takes the account e into the record, as Join says, and returns the
// id it has there.
func (j *joining) join(e Entry) (string, error) {
	e.Was = ""
	if i := j.record.matching(e); i >= 0 {
		own := &j.record.Accounts[i]
		own.take(e, containsFold(j.configured, own.ID))
		return own.ID, nil
	}
	if j.record.clashes(e.ID, j.configured) {
		id, err := j.record.Random(j.random, j.held)
		if err != nil {
			return "", fmt.Errorf("take in account %s: %w", e.ID, err)
		}
		e.ID = id
	}
	j.record.Accounts = append(j.record.Accounts, e)
	return e.ID, nil
}

// matching is the place of the account e is, by its organization, else by
// its id where their organizations don't differ: -1 where it's none.
func (r Record) matching(e Entry) int {
	if e.Organization != "" {
		if i := slices.IndexFunc(r.Accounts, func(own Entry) bool { return own.Organization == e.Organization }); i >= 0 {
			return i
		}
	}
	return slices.IndexFunc(r.Accounts, func(own Entry) bool {
		return strings.EqualFold(own.ID, e.ID) && (own.Organization == "" || e.Organization == "" || own.Organization == e.Organization)
	})
}

// take takes what the account doesn't know of it from e, the same account as
// another record keeps it: its organization, where it has none, and its
// label, where it's labelled, unless the account is configured, as
// configured says, or removed.
func (own *Entry) take(e Entry, configured bool) {
	own.organize(e.Organization)
	if !configured && own.Removed.IsZero() && e.Label != "" {
		own.Label = e.Label
	}
}

// ids are the record's accounts' ids.
func (r Record) ids() []string {
	ids := make([]string, len(r.Accounts))
	for i, e := range r.Accounts {
		ids[i] = e.ID
	}
	return ids
}
