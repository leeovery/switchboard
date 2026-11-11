package identity

import "slices"

// Outcome is what adding an account with a token comes to.
type Outcome int

const (
	// OrganizationUnknown is a token whose answer named no organization:
	// whether it's a new account or one of the removed ones is the user's to
	// say.
	OrganizationUnknown Outcome = iota
	// NewAccount is an account this Mac hasn't had, given the id its
	// organization derives.
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
	// for one Returning or Configured, and "" where it's OrganizationUnknown.
	ID string
	// Entry is the record's account, for one Returning or Configured.
	Entry Entry
}

// Adding decides what adding an account with a token comes to, by the
// organization the token's answer named, "" for none, and the ids of the
// accounts configured: an account of an organization the record has is
// Configured where its id is among them, else Returning, whether or not the
// record has it removed, as the config may have been edited by hand; any
// other's is a NewAccount.
func (r Record) Adding(configured []string, organization string) Adding {
	if organization == "" {
		return Adding{Outcome: OrganizationUnknown}
	}
	e, ok := r.ByOrganization(organization)
	switch {
	case !ok:
		return Adding{Outcome: NewAccount, ID: r.Derive(organization)}
	case slices.Contains(configured, e.ID):
		return Adding{Outcome: Configured, ID: e.ID, Entry: e}
	}
	return Adding{Outcome: Returning, ID: e.ID, Entry: e}
}
