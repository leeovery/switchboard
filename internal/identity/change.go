package identity

import (
	"cmp"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// Add adds an account to the record, configured: its id, its organization,
// "" where it isn't known, and when it was added.
func (r *Record) Add(id, organization string, at time.Time) {
	r.Accounts = append(r.Accounts, Entry{ID: id, Organization: organization, Added: at})
}

// Move adds an account to the record, configured, moved onto the id
// switchboard made for it from the one it had before, was: its organization,
// "" where it isn't known, and when it was moved.
func (r *Record) Move(id, organization, was string, at time.Time) {
	r.Accounts = append(r.Accounts, Entry{ID: id, Organization: organization, Added: at, Was: was})
}

// RemovedBefore adds an account removed before switchboard made ids, which
// had the id was, to the record under the id given, removed, its label was,
// its organization not known: when the history first and last tells of it,
// as added and removed.
func (r *Record) RemovedBefore(id, was string, added, removed time.Time) {
	r.Accounts = append(r.Accounts, Entry{ID: id, Label: was, Added: added, Removed: removed, Was: was})
}

// Former records was as the id the account with the given id had before
// switchboard made ids, where the record has none of it, reporting whether
// it has that one now: false where the record hasn't the account, or has
// another.
func (r *Record) Former(id, was string) bool {
	e := r.entry(id)
	if e == nil {
		return false
	}
	if e.Was == "" {
		e.Was = was
	}
	return strings.EqualFold(e.Was, was)
}

// Return has the account with the given id configured again, so its label
// and its removal are the record's no longer, and records its organization
// as Organize does. It reports false where the record hasn't the account.
func (r *Record) Return(id, organization string) bool {
	e := r.entry(id)
	if e == nil {
		return false
	}
	e.Label, e.Removed = "", time.Time{}
	e.organize(organization)
	return true
}

// Organize records the organization of the account with the given id, where
// the record doesn't know it: "" leaves it as it is. It reports false where
// the record hasn't the account.
func (r *Record) Organize(id, organization string) bool {
	e := r.entry(id)
	if e == nil {
		return false
	}
	e.organize(organization)
	return true
}

// Remove marks the account with the given id removed at the time given,
// keeping the label it showed, reporting false where the record hasn't the
// account, as it hasn't one from before switchboard made ids.
func (r *Record) Remove(id, label string, at time.Time) bool {
	e := r.entry(id)
	if e == nil {
		return false
	}
	e.Label, e.Removed = label, at
	return true
}

// Kept is what KeepUp changed of the record, by the accounts' ids: those
// added, marked removed and configured again, and those configured it left
// out, as their ids clash.
type Kept struct {
	Added, Removed, Returned, Clashed []string
}

// KeepUp keeps the record in step with the accounts configured, as the config
// stands at the time given, as accounts add and remove keep it, so an account
// added or removed on another Mac sharing this one's config is one this Mac
// has had too:
//   - one configured that the record lacks is added, its organization not
//     known, but one whose id clashes, as clashes says, ignoring case, which
//     is left out;
//   - one configured that's removed, or kept as only another Mac configures
//     it, its label the record's, is configured again, as Return has it;
//   - one this Mac configured, live and unlabelled, that has left the config
//     is marked removed, at the time given, as Remove has it, labelled as
//     labelOf gives the label it last had in the config, else by its id.
//
// It returns what it changed.
func (r *Record) KeepUp(configured config.Accounts, labelOf func(id string) string, at time.Time) Kept {
	var kept Kept
	ids := configured.IDs()
	for _, id := range ids {
		switch e := r.entry(id); {
		case e == nil && r.clashes(id, nil):
			kept.Clashed = append(kept.Clashed, id)
		case e == nil:
			r.Add(id, "", at)
			kept.Added = append(kept.Added, id)
		case !e.Removed.IsZero() || e.Label != "":
			kept.Returned = append(kept.Returned, e.ID)
			r.Return(id, "")
		}
	}
	for _, e := range r.Accounts {
		if e.Removed.IsZero() && e.Label == "" && !containsFold(ids, e.ID) {
			r.Remove(e.ID, cmp.Or(labelOf(e.ID), e.ID), at)
			kept.Removed = append(kept.Removed, e.ID)
		}
	}
	return kept
}

// entry is the record's account with the given id, in any case, to change:
// nil where it hasn't one.
func (r *Record) entry(id string) *Entry {
	for i := range r.Accounts {
		if strings.EqualFold(r.Accounts[i].ID, id) {
			return &r.Accounts[i]
		}
	}
	return nil
}

// organize takes the organization given as the account's, where it isn't
// known.
func (e *Entry) organize(organization string) {
	if e.Organization == "" {
		e.Organization = organization
	}
}
