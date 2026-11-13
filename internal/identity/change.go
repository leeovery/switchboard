package identity

import (
	"cmp"
	"slices"
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
// out, as their ids clash; and, by their ids, what it changed of the pools;
// or, where it changed nothing as a heal is owed, the ids configured that
// accounts had before switchboard made ids.
type Kept struct {
	Added, Removed, Returned, Clashed []string
	Pools                             PoolsKept
	Unhealed                          []string
}

// PoolsKept is what KeepUp changed of the record's pools, by their ids:
// those added, marked removed and configured again, and those configured it
// left out, as their ids clash.
type PoolsKept struct {
	Added, Removed, Returned, Clashed []string
}

// Followed is the config KeepUp keeps the record in step with, as it
// stands: its accounts and its pools; and the label each account, and each
// pool, had in it last, by its id, as what follows the config knows it, ""
// where it knows none.
type Followed struct {
	Accounts     config.Accounts
	AccountLabel func(id string) string
	Pools        config.Pools
	PoolLabel    func(id string) string
}

// KeepUp keeps the record in step with the accounts and pools configured, as
// followed has the config, at the time given, as accounts add and remove
// keep it, so an account or pool added or removed on another Mac sharing
// this one's config is one this Mac has had too:
//   - one configured that the record lacks is added, an account's
//     organization not known, but one whose id clashes, as clashes says,
//     ignoring case, which is left out;
//   - one configured that's removed, or kept as only another Mac configures
//     it, its label the record's, is configured again, as Return has it;
//   - one this Mac configured, an account live and unlabelled, a pool marked
//     this Mac's, as PoolEntry's Here says, that has left the config is
//     marked removed, at the time given, as Remove has it, labelled as
//     followed gives the label it last had in the config, else, of a pool,
//     as the record kept it written, else by its id.
//
// Each pool configured is this Mac's, kept as the config writes it, its
// label, directories and primary, and each account configured keeps the
// pool the config has it in, as PoolEntry's Here and Written and Entry's
// Pool say. A config from before pools, which
// has none, leaves the record's pools, and the accounts' pools, as they are:
// the move to pools' heal writes them back.
//
// It changes nothing while any account configured is named by the id an
// account had before switchboard made ids, as MovedFrom finds it, as where a
// config put back from before the move names them so: the move's Heal gives
// them their new ids as the router restarts, and the record is kept in step
// once it has. Until then they'd only clash, and the accounts by their new
// ids be marked removed. It returns what it changed, or the ids that stopped
// it.
func (r *Record) KeepUp(followed Followed, at time.Time) Kept {
	ids := followed.Accounts.IDs()
	if unhealed := r.unhealed(ids); len(unhealed) > 0 {
		return Kept{Unhealed: unhealed}
	}
	var kept Kept
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
			r.Remove(e.ID, labelIn(followed.AccountLabel, e.ID, ""), at)
			kept.Removed = append(kept.Removed, e.ID)
		}
	}
	kept.Pools = r.KeepPoolsUp(followed, at)
	return kept
}

// KeepPoolsUp keeps the record's pools, and the pool each account is in, in
// step with those followed configures, as KeepUp keeps them, at the time
// given, and returns what it changed: of a config with pools alone, as one
// from before pools leaves them as they are. The move to pools keeps them so
// as it notes the first pool of a config with pools already, before KeepUp
// first runs, so a heal meanwhile writes back every pool the config has.
func (r *Record) KeepPoolsUp(followed Followed, at time.Time) PoolsKept {
	if len(followed.Pools) == 0 {
		return PoolsKept{}
	}
	kept := r.keepPoolsUp(followed, at)
	r.keepMembersUp(followed.Accounts)
	return kept
}

// keepPoolsUp keeps the record's pools in step with those followed
// configures, as KeepUp keeps the accounts, PoolLabel giving the label each
// had in the config last: each configured this Mac's, as PoolEntry's Here
// says, kept as the config writes it, of the accounts configured.
func (r *Record) keepPoolsUp(followed Followed, at time.Time) PoolsKept {
	var kept PoolsKept
	for _, c := range followed.Pools {
		switch p := r.pool(c.ID); {
		case p == nil && r.clashes(c.ID, nil):
			kept.Clashed = append(kept.Clashed, c.ID)
			continue
		case p == nil:
			r.Pools = append(r.Pools, PoolEntry{ID: c.ID, Added: at})
			kept.Added = append(kept.Added, c.ID)
		case !p.Removed.IsZero() || p.Label != "":
			p.Label, p.Removed = "", time.Time{}
			kept.Returned = append(kept.Returned, p.ID)
		}
		p := r.pool(c.ID)
		p.Here, p.Written = true, WrittenOf(c, followed.Accounts)
	}
	ids := followed.Pools.IDs()
	for i := range r.Pools {
		if p := &r.Pools[i]; p.Here && !containsFold(ids, p.ID) {
			p.Label, p.Removed, p.Here = labelIn(followed.PoolLabel, p.ID, p.Written.Label), at, false
			kept.Removed = append(kept.Removed, p.ID)
		}
	}
	return kept
}

// WrittenOf is the pool configured as its config writes it, of the accounts
// configured: a label that's its id, as the config gives a pool it writes
// none for, none; and its primary, known, as the config makes it, none
// where no account is in it.
func WrittenOf(p config.Pool, accounts config.Accounts) PoolWritten {
	w := PoolWritten{Dirs: slices.Clone(p.WrittenDirs), PrimaryKnown: true}
	if !strings.EqualFold(p.Label, p.ID) {
		w.Label = p.Label
	}
	if i := slices.IndexFunc(accounts, func(a config.Account) bool { return a.Primary && config.SamePool(a.Pool, p.ID) }); i >= 0 {
		w.Primary = accounts[i].ID
	}
	return w
}

// keepMembersUp has each account configured that the record holds keep the
// pool the config has it in.
func (r *Record) keepMembersUp(configured config.Accounts) {
	for _, a := range configured {
		if e := r.entry(a.ID); e != nil && a.Pool != "" {
			e.Pool = a.Pool
		}
	}
}

// NoteFirst notes the pool with the given id as the first, where the history
// from before pools is, this Mac's, configured, adding it at the time given
// where the record hasn't it: for a record noting none first, as a record
// notes one at most. written is how the config writes it, as PoolEntry's
// Written says: as the move to pools writes the one it makes, or as
// WrittenOf gives one the config holds already, so a heal before KeepUp
// first keeps it writes it as it is.
func (r *Record) NoteFirst(id string, written PoolWritten, at time.Time) {
	p := r.pool(id)
	if p == nil {
		r.Pools = append(r.Pools, PoolEntry{ID: id, Added: at})
		p = &r.Pools[len(r.Pools)-1]
	}
	p.First, p.Here, p.Label, p.Removed, p.Written = true, true, "", time.Time{}, written
}

// labelIn returns the label labelOf gives the account or pool with the given
// id, else the label kept, else its id: nil gives none.
func labelIn(labelOf func(id string) string, id, kept string) string {
	var given string
	if labelOf != nil {
		given = labelOf(id)
	}
	return cmp.Or(given, kept, id)
}

// pool is the record's pool with the given id, in any case, to change: nil
// where it hasn't one.
func (r *Record) pool(id string) *PoolEntry {
	for i := range r.Pools {
		if config.SamePool(r.Pools[i].ID, id) {
			return &r.Pools[i]
		}
	}
	return nil
}

// unhealed are those of ids that are accounts' ids before switchboard made
// ids, as MovedFrom finds them, which the move's Heal gives the new.
func (r Record) unhealed(ids []string) []string {
	var unhealed []string
	for _, id := range ids {
		if _, moved := r.MovedFrom(id); moved {
			unhealed = append(unhealed, id)
		}
	}
	return unhealed
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
