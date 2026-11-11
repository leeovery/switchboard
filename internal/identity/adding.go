package identity

import (
	"maps"
	"slices"
	"strings"
)

// Outcome is what adding an account with a token comes to.
type Outcome int

const (
	// OrganizationUnknown is a token whose answer named no organization:
	// whether it's a new account or one of the removed ones is the user's to
	// say.
	OrganizationUnknown Outcome = iota
	// NewAccount is an account of an organization this Mac hasn't had, given
	// the id its organization derives.
	NewAccount
	// Returning is an account this Mac has had, and has configured no
	// longer, coming back under its id, so its history carries on.
	Returning
	// Configured is an account configured already, which is refused.
	Configured
)

// Adding is what adding an account with a token comes to, as Record.Adding
// decides it.
type Adding struct {
	Outcome Outcome
	// ID is the account's id: the one derived for a NewAccount, the record's
	// for one Returning, the configured account's for one Configured, and ""
	// where it's OrganizationUnknown.
	ID string
	// Entry is the record's account, for one Returning, and for one
	// Configured where the record has it.
	Entry Entry
	// Candidates are, for a NewAccount, the accounts the record has whose
	// organization isn't known and that aren't configured, as those removed
	// before switchboard made ids: the token may be any of them, which is the
	// user's to say.
	Candidates []Entry
}

// Adding decides what adding an account with a token comes to, by the
// organization the token's answer named, "" for none, and the accounts
// configured, each one's organization by its id, "" where it isn't known.
// It's Configured where a configured account is of the organization, as
// configuredOf finds one; else Returning where the record has the
// organization, whether or not it has it removed, as the config may have been
// edited by hand; else a NewAccount.
func (r Record) Adding(configured map[string]string, organization string) Adding {
	if organization == "" {
		return Adding{Outcome: OrganizationUnknown}
	}
	ids := slices.Sorted(maps.Keys(configured))
	if id, ok := r.configuredOf(configured, ids, organization); ok {
		e, _ := r.ByID(id)
		return Adding{Outcome: Configured, ID: id, Entry: e}
	}
	if e, ok := r.ByOrganization(organization); ok {
		return Adding{Outcome: Returning, ID: e.ID, Entry: e}
	}
	return Adding{Outcome: NewAccount, ID: r.Derive(organization, ids), Candidates: r.unknown(ids)}
}

// configuredOf returns the id of the configured account of the given
// organization, of those configured, whose ids are ids, in order: the one
// the record has as the organization's, or one the config knows to be of it,
// or, failing safe, one whose organization isn't known that has the id the
// record would derive for the organization, which it may well be, as ids are
// derived. It reports false where none is.
func (r Record) configuredOf(configured map[string]string, ids []string, organization string) (string, bool) {
	if e, ok := r.ByOrganization(organization); ok && containsFold(ids, e.ID) {
		return e.ID, true
	}
	derived := r.Derive(organization, nil)
	for _, id := range ids {
		known := configured[id]
		if known == organization || known == "" && strings.EqualFold(id, derived) {
			return id, true
		}
	}
	return "", false
}

// unknown returns the accounts the record has whose organization isn't known
// and whose ids aren't among configured, in the record's order.
func (r Record) unknown(configured []string) []Entry {
	var found []Entry
	for _, e := range r.Accounts {
		if e.Organization == "" && !containsFold(configured, e.ID) {
			found = append(found, e)
		}
	}
	return found
}
