package identity

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrConflict is what joining an accounts record that contradicts this one
// fails with, wrapped.
var ErrConflict = errors.New("the accounts records disagree")

// Join takes the accounts of another record, as an exported folder carries
// another Mac's, into r, and returns the ids of those it took as accounts r
// knows under other ids, in lower case, each with r's id. Each is matched to
// r's account of the same id, organization or id it had before switchboard
// made ids, but where both organizations are known and differ:
//   - matching none, it's added as it is, its label kept, as of an account
//     only the other Mac configures; but for its former id, where that's the
//     former id of an account of r's of another organization, as two
//     subscriptions once named alike, which it's added without;
//   - matching one, it's that account, which keeps its own label, times and
//     removal, and takes what it doesn't know of it: its organization, its
//     former id, and its label, where it has none and isn't removed.
//
// It fails with ErrConflict wrapped, changing nothing, where an account
// matches two of r's, or has the id of one of another organization, and
// where r, joined, breaks the record's rules, as check says.
func (r *Record) Join(other Record) (map[string]string, error) {
	joined := Record{Accounts: slices.Clone(r.Accounts)}
	mapped := make(map[string]string)
	for _, e := range other.Accounts {
		id, err := joined.join(e)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrConflict, err)
		}
		if !strings.EqualFold(id, e.ID) {
			mapped[strings.ToLower(e.ID)] = id
		}
	}
	if err := joined.check(); err != nil {
		return nil, fmt.Errorf("%w: joined, %w", ErrConflict, err)
	}
	*r = joined
	return mapped, nil
}

// join takes the account e into r, as Join says, and returns the id it has
// in r, failing, saying why, where it contradicts it.
func (r *Record) join(e Entry) (string, error) {
	var matched []int
	for i, own := range r.Accounts {
		switch {
		case own.apart(e) && strings.EqualFold(own.ID, e.ID):
			return "", fmt.Errorf("account %s is another organization's here", e.ID)
		case own.apart(e) && own.Was != "" && strings.EqualFold(own.Was, e.Was):
			e.Was = ""
		case own.matches(e):
			matched = append(matched, i)
		}
	}
	switch len(matched) {
	case 0:
		r.Accounts = append(r.Accounts, e)
		return e.ID, nil
	case 1:
		own := &r.Accounts[matched[0]]
		own.take(e)
		return own.ID, nil
	}
	return "", fmt.Errorf("account %s is two accounts here, %s and %s", e.ID, r.Accounts[matched[0]].ID, r.Accounts[matched[1]].ID)
}

// apart reports whether e is another subscription's than the account's, both
// their organizations known.
func (own Entry) apart(e Entry) bool {
	return own.Organization != "" && e.Organization != "" && own.Organization != e.Organization
}

// matches reports whether e is the account, by its id, its organization or
// the id it had before switchboard made ids, each where it's known.
func (own Entry) matches(e Entry) bool {
	return strings.EqualFold(own.ID, e.ID) ||
		own.Organization != "" && own.Organization == e.Organization ||
		own.Was != "" && strings.EqualFold(own.Was, e.Was)
}

// take takes what the account doesn't know of it from e, the same account as
// another record keeps it: its organization, the id it had before
// switchboard made ids, and its label, where it has none and isn't removed.
func (own *Entry) take(e Entry) {
	own.organize(e.Organization)
	if own.Was == "" {
		own.Was = e.Was
	}
	if own.Label == "" && own.Removed.IsZero() {
		own.Label = e.Label
	}
}
