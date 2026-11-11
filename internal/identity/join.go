package identity

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrConflict is what joining an accounts record that disagrees with this
// one fails with, wrapped.
var ErrConflict = errors.New("the accounts records disagree")

// Join takes the accounts of another record, as an exported folder carries
// another Mac's, into r. Each is matched to r's account of the same id,
// organization or id it had before switchboard made ids, which keeps its own
// label, times and removal, and takes from it what it doesn't know, its
// organization and former id; one that matches none is added as it is. It
// fails with ErrConflict wrapped, changing nothing, where an account matches
// two of r's, or one whose id, organization or former id, where both know
// it, isn't its own, and where r, joined, breaks the record's rules, as
// check says.
func (r *Record) Join(other Record) error {
	joined := Record{Accounts: slices.Clone(r.Accounts)}
	for _, e := range other.Accounts {
		if err := joined.join(e); err != nil {
			return fmt.Errorf("%w: %w", ErrConflict, err)
		}
	}
	if err := joined.check(); err != nil {
		return fmt.Errorf("%w: joined, %w", ErrConflict, err)
	}
	*r = joined
	return nil
}

// join takes the account e into r, as Join says, failing, saying why, where
// it conflicts.
func (r *Record) join(e Entry) error {
	i, err := r.match(e)
	switch {
	case err != nil:
		return err
	case i < 0:
		r.Accounts = append(r.Accounts, e)
		return nil
	}
	own := &r.Accounts[i]
	switch {
	case !strings.EqualFold(own.ID, e.ID):
		return fmt.Errorf("account %s is account %s here", e.ID, own.ID)
	case own.Organization != "" && e.Organization != "" && own.Organization != e.Organization:
		return fmt.Errorf("account %s is of another organization here", e.ID)
	case own.Was != "" && e.Was != "" && !strings.EqualFold(own.Was, e.Was):
		return fmt.Errorf("account %s had another id before switchboard made ids here", e.ID)
	}
	own.organize(e.Organization)
	if own.Was == "" {
		own.Was = e.Was
	}
	return nil
}

// match is the place of r's account that e matches, by its id, its
// organization or the id it had before switchboard made ids: -1 where none
// does. It fails where two do.
func (r Record) match(e Entry) (int, error) {
	found := -1
	for i, own := range r.Accounts {
		if !own.matches(e) {
			continue
		}
		if found >= 0 {
			return -1, fmt.Errorf("account %s is two accounts here, %s and %s", e.ID, r.Accounts[found].ID, own.ID)
		}
		found = i
	}
	return found, nil
}

// matches reports whether e is the account, by its id, its organization or
// the id it had before switchboard made ids, each where it's known.
func (own Entry) matches(e Entry) bool {
	return strings.EqualFold(own.ID, e.ID) ||
		own.Organization != "" && own.Organization == e.Organization ||
		own.Was != "" && strings.EqualFold(own.Was, e.Was)
}
