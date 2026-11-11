// Package identity knows each account by the id switchboard makes for it,
// derived from its subscription's organization, and keeps the accounts
// record, <state dir>/accounts.json: this Mac's record of every account it
// has had, removed ones among them. It isn't the config's accounts, which
// internal/accounts edits.
package identity

import (
	"slices"
	"strings"
	"time"
)

// Record is the accounts record: every account this Mac has had.
type Record struct {
	Accounts []Entry
}

// Entry is an account as the accounts record keeps it.
type Entry struct {
	// ID is the id switchboard made for it, which every record keys it by.
	ID string `json:"id"`
	// Organization is the id of its subscription's organization, as every
	// answer from Anthropic's API names it: "" where it isn't known.
	Organization string `json:"organization,omitempty"`
	// Label is what it showed as, kept once it's removed: "" while it's
	// configured, as the config's is the one.
	Label string `json:"label,omitempty"`
	// Added is when it was added, and Removed when it was removed: zero while
	// it's configured.
	Added   time.Time `json:"added"`
	Removed time.Time `json:"removed,omitzero"`
	// Was is the id it had before switchboard made ids, read to import a
	// folder exported before then: "" where it had none.
	Was string `json:"was,omitempty"`
	// Configured is set in the copy of a record an export writes alone, on an
	// account the exporting Mac configures, whose Label is its config's, for
	// a Mac importing it to take: never in a Mac's own record.
	Configured bool `json:"configured,omitzero"`
}

// ByID returns the account with the given id, in any case, reporting false
// where the record has none.
func (r Record) ByID(id string) (Entry, bool) {
	return r.find(func(e Entry) bool { return strings.EqualFold(e.ID, id) })
}

// ByOrganization returns the account of the given organization, reporting
// false where the record has none, as for "", an organization not known.
func (r Record) ByOrganization(organization string) (Entry, bool) {
	return r.find(func(e Entry) bool { return organization != "" && e.Organization == organization })
}

// ByWas returns the account that had the given id, in any case, before
// switchboard made ids, reporting false where the record has none, as for "".
func (r Record) ByWas(was string) (Entry, bool) {
	return r.find(func(e Entry) bool { return was != "" && strings.EqualFold(e.Was, was) })
}

// Removed returns the accounts that have been removed, in the record's
// order.
func (r Record) Removed() []Entry {
	var removed []Entry
	for _, e := range r.Accounts {
		if !e.Removed.IsZero() {
			removed = append(removed, e)
		}
	}
	return removed
}

// find returns the first account that matches, reporting false where none
// does.
func (r Record) find(match func(Entry) bool) (Entry, bool) {
	i := slices.IndexFunc(r.Accounts, match)
	if i < 0 {
		return Entry{}, false
	}
	return r.Accounts[i], true
}
