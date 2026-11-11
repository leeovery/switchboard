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
// It's Configured where a configured account is known to be of the
// organization, as knownAs finds one; else Returning where the record has the
// organization, whether or not it has it removed, as the config may have been
// edited by hand; else Configured where a configured account may be of it, as
// mayBe finds one; else a NewAccount.
func (r Record) Adding(configured map[string]string, organization string) Adding {
	if organization == "" {
		return Adding{Outcome: OrganizationUnknown}
	}
	ids := slices.Sorted(maps.Keys(configured))
	if id, ok := r.knownAs(configured, ids, organization); ok {
		return r.configured(id)
	}
	if e, ok := r.ByOrganization(organization); ok {
		return Adding{Outcome: Returning, ID: e.ID, Entry: e}
	}
	if id, ok := r.mayBe(configured, ids, organization); ok {
		return r.configured(id)
	}
	return Adding{Outcome: NewAccount, ID: r.Derive(organization, ids), Candidates: r.unknown(ids)}
}

// configured is adding the configured account with the given id, refused,
// with the record's account where it has it.
func (r Record) configured(id string) Adding {
	e, _ := r.ByID(id)
	return Adding{Outcome: Configured, ID: id, Entry: e}
}

// knownAs returns the id of the first configured account, of those whose ids
// are ids, in order, that the config or the record knows to be of the given
// organization, reporting false where none is.
func (r Record) knownAs(configured map[string]string, ids []string, organization string) (string, bool) {
	for _, id := range ids {
		if e, _ := r.ByID(id); configured[id] == organization || e.Organization == organization {
			return id, true
		}
	}
	return "", false
}

// mayBe returns the id of the configured account, of those whose ids are ids,
// whose organization neither the config nor the record knows, that has the id
// the given organization would derive were none of those there: which it may
// well be, as ids are derived, so it's taken to be, failing safe. It reports
// false where none has.
func (r Record) mayBe(configured map[string]string, ids []string, organization string) (string, bool) {
	var unknown, known []string
	for _, id := range ids {
		if e, _ := r.ByID(id); configured[id] == "" && e.Organization == "" {
			unknown = append(unknown, id)
		} else {
			known = append(known, id)
		}
	}
	others := Record{Accounts: slices.DeleteFunc(slices.Clone(r.Accounts), func(e Entry) bool { return containsFold(unknown, e.ID) })}
	derived := spellUntil(organization, func(id string) bool {
		return containsFold(unknown, id) || !others.clashes(id, known)
	})
	i := slices.IndexFunc(unknown, func(id string) bool { return strings.EqualFold(id, derived) })
	if i < 0 {
		return "", false
	}
	return unknown[i], true
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
