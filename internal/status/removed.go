package status

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/prose"
)

// RemovedAccount is an account the accounts record keeps that the config
// doesn't configure: one removed, or one only another Mac configures, as an
// import joins its record in. It's kept by its id, its label, which names it
// wherever the history does, and, where it was removed, when.
type RemovedAccount struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Removed time.Time `json:"removed,omitzero"`
}

// Said says when the account was removed, by its local day, so it's told
// apart from an account configured since under the same label: "removed 4
// Sep". It's "" of one never removed.
func (a RemovedAccount) Said() string {
	if a.Removed.IsZero() {
		return ""
	}
	return "removed " + prose.DayMonth(a.Removed.Local())
}

// RemovedAccounts are the accounts removed, as RemovedOf gives them.
type RemovedAccounts []RemovedAccount

// RemovedOf are the accounts record keeps that configured doesn't configure,
// ids compared ignoring case, in record's order.
func RemovedOf(record identity.Record, configured config.Accounts) RemovedAccounts {
	var removed RemovedAccounts
	for _, e := range record.Accounts {
		if configured.Index(e.ID) < 0 {
			removed = append(removed, RemovedAccount{ID: e.ID, Label: e.Label, Removed: e.Removed})
		}
	}
	return removed
}

// Of returns the removed account with the given id, ids compared ignoring
// case, reporting false where it's none of them.
func (r RemovedAccounts) Of(id string) (RemovedAccount, bool) {
	i := slices.IndexFunc(r, func(a RemovedAccount) bool { return strings.EqualFold(a.ID, id) })
	if i < 0 {
		return RemovedAccount{}, false
	}
	return r[i], true
}

// AccountName is what an account is called wherever it's named by its id.
// Embedded beside an account's id, it gives the verbs' JSON its label.
type AccountName struct {
	// Label names it: as configured, where it's configured; else as the
	// accounts record keeps it; else its id.
	Label string `json:"label,omitempty"`
	// Other is set on an account the accounts record keeps that isn't
	// configured: one removed, or one only another Mac configures.
	Other bool `json:"-"`
	// Removed says when it was removed, as RemovedAccount's Said says: "" of
	// one never removed.
	Removed string `json:"-"`
}

// ConfiguredName names the account configured as a is, as AccountName has
// it.
func ConfiguredName(a config.Account) AccountName {
	return namedAs(a.Label, a.ID)
}

// namedAs names a configured account by its label, else its id.
func namedAs(label, id string) AccountName {
	return AccountName{Label: cmp.Or(label, id)}
}

// Said is the account as the verbs' text names it, cleaned, as it's from
// elsewhere: its label, and of one removed, when, as "team (removed 4 Sep)".
func (n AccountName) Said() string {
	label := prose.Clean(n.Label)
	if n.Removed == "" {
		return label
	}
	return label + " (" + n.Removed + ")"
}

// Namer names accounts by their ids, as AccountName has it: a Moment does,
// and Names.
type Namer interface {
	Named(id string) AccountName
}

// Names name the configured accounts and the accounts removed, as
// RemovedAccounts' Named does, for a reader with no document to name them
// by. Their zero value names every account by its id.
type Names struct {
	configured config.Accounts
	removed    RemovedAccounts
}

// NamesOf are the names of the accounts configured, and of the accounts
// removed, as RemovedOf gives them.
func NamesOf(configured config.Accounts, removed RemovedAccounts) Names {
	return Names{configured: configured, removed: removed}
}

// Named names the account with the given id, as AccountName has it.
func (n Names) Named(id string) AccountName {
	return n.removed.Named(n.configured, id)
}

// Knows reports whether an account has the given id, ignoring case: one the
// config configures, or one the accounts record keeps.
func (n Names) Knows(id string) bool {
	_, removed := n.removed.Of(id)
	return removed || n.configured.Index(id) >= 0
}

// Labelled are the ids of the accounts labelled as given, ignoring case: those
// configured, in the config's order, then those the accounts record keeps, in
// its order, as a label removed may have been given again.
func (n Names) Labelled(label string) []string {
	var ids []string
	for _, a := range n.configured {
		if a.Label != "" && strings.EqualFold(a.Label, label) {
			ids = append(ids, a.ID)
		}
	}
	for _, a := range n.removed {
		if a.Label != "" && strings.EqualFold(a.Label, label) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// Named names the account with the given id, configured as configured says,
// ids compared ignoring case, as AccountName has it.
func (r RemovedAccounts) Named(configured config.Accounts, id string) AccountName {
	if a, ok := configured.ByID(id); ok {
		return namedAs(a.Label, id)
	}
	return r.named(id)
}

// named names the account with the given id, which isn't configured, as
// AccountName has it.
func (r RemovedAccounts) named(id string) AccountName {
	a, ok := r.Of(id)
	if !ok {
		return AccountName{Label: id}
	}
	return AccountName{Label: cmp.Or(a.Label, id), Other: true, Removed: a.Said()}
}

// Named names the account of the moment with the given id, as AccountName
// has it: configured where its document holds it, else as the accounts
// removed its document was given, as Document's WithRemoved gives them,
// ids compared ignoring case.
func (m Moment) Named(id string) AccountName {
	if i := slices.IndexFunc(m.Accounts, func(a AccountNow) bool { return strings.EqualFold(a.ID, id) }); i >= 0 {
		return namedAs(m.Accounts[i].Label, id)
	}
	return m.doc.removed.named(id)
}
