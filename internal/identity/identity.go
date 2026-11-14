// Package identity knows each account by the id switchboard makes for it,
// derived from its subscription's organization, makes each pool's, and keeps
// the accounts record, <state dir>/accounts.json: this Mac's record of every
// account and pool it has had, removed ones among them. It isn't the
// config's accounts, which internal/accounts edits.
package identity

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
)

// Record is the accounts record: every account this Mac has had, and every
// pool.
type Record struct {
	Accounts []Entry
	// Pools are the pools: none in a record from before pools.
	Pools []PoolEntry
}

// PoolEntry is a pool as the accounts record keeps it.
type PoolEntry struct {
	// ID is the id switchboard made for it, which every record keys it by.
	ID string `json:"id"`
	// Label is what it showed as, kept once it's removed: "" while it's
	// configured, as the config's is the one.
	Label string `json:"label,omitempty"`
	// Added is when it was added, and Removed when it was removed: zero while
	// it's configured.
	Added   time.Time `json:"added"`
	Removed time.Time `json:"removed,omitzero"`
	// First is set on the first pool, the one the move to pools or setup on
	// a new install made, where the history from before pools is.
	First bool `json:"first,omitzero"`
	// Configured is set in the copy of a record an export writes alone, as
	// an Entry's is.
	Configured bool `json:"configured,omitzero"`
	// Here is set while this Mac's config configures it, as KeepUp keeps it,
	// and on the first pool from when the move to pools notes it: never on
	// one only another Mac configures, nor in an export's copy. A heal writes
	// back only these.
	Here bool `json:"here,omitzero"`
	// Written is the pool as this Mac's config last wrote it, kept in step
	// by KeepUp, kept once it's removed: for a heal to write it back where a
	// config from before pools is put back. Zero where this Mac never
	// configured it, or its config wrote no label, directories nor primary;
	// never in an export's copy.
	Written PoolWritten `json:"written,omitzero"`
}

// PoolWritten is a pool as a config writes it: its label, "" where it gives
// none; its directories, as written, such as ~/Code/acme, none where it's
// unfenced; and the id of its primary account, the one it marks, else its
// first, "" where it has no account. PrimaryKnown is set once the record
// has its primary from a config, so a Primary of "" says it had none,
// rather than that it isn't known, as of a pool the move to pools made that
// the record hasn't been kept in step with since.
type PoolWritten struct {
	Label        string   `json:"label,omitempty"`
	Dirs         []string `json:"dirs,omitempty"`
	Primary      string   `json:"primary,omitempty"`
	PrimaryKnown bool     `json:"primary_known,omitzero"`
}

// Ours reports whether this Mac's config has had the pool: Here while it
// has it, and once it's removed, Written, which KeepUp keeps then, and
// which an import never carries, as one only another Mac had has none.
func (p PoolEntry) Ours() bool {
	return p.Here || !p.Written.Equal(PoolWritten{})
}

// Equal reports whether the pool is other, field for field, its times the
// same instants.
func (p PoolEntry) Equal(other PoolEntry) bool {
	return p.ID == other.ID && p.Label == other.Label && p.Added.Equal(other.Added) && p.Removed.Equal(other.Removed) &&
		p.First == other.First && p.Configured == other.Configured && p.Here == other.Here && p.Written.Equal(other.Written)
}

// Equal reports whether the pool is written as other is, field for field.
func (w PoolWritten) Equal(other PoolWritten) bool {
	return w.Label == other.Label && slices.Equal(w.Dirs, other.Dirs) && w.Primary == other.Primary && w.PrimaryKnown == other.PrimaryKnown
}

// PoolByID returns the pool with the given id, in any case, reporting false
// where the record has none.
func (r Record) PoolByID(id string) (PoolEntry, bool) {
	return r.findPool(func(p PoolEntry) bool { return config.SamePool(p.ID, id) })
}

// FirstPool returns the first pool, reporting false where the record notes
// none, as one from before pools.
func (r Record) FirstPool() (PoolEntry, bool) {
	return r.findPool(func(p PoolEntry) bool { return p.First })
}

// findPool returns the first pool that matches, reporting false where none
// does.
func (r Record) findPool(match func(PoolEntry) bool) (PoolEntry, bool) {
	i := slices.IndexFunc(r.Pools, match)
	if i < 0 {
		return PoolEntry{}, false
	}
	return r.Pools[i], true
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
	// Pool is the id of the pool this Mac's config last had it in, kept in
	// step by KeepUp, kept once it's removed: "" where it never had it in
	// one.
	Pool string `json:"pool,omitempty"`
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

// MovedFrom returns the account, not removed, that had the given id, in any
// case, before switchboard made ids: the one a config naming that id names,
// once the move onto the ids is done, which the move's Heal gives the new.
// It reports false where the record has none, as for "".
func (r Record) MovedFrom(was string) (Entry, bool) {
	return r.find(func(e Entry) bool { return was != "" && strings.EqualFold(e.Was, was) && e.Removed.IsZero() })
}

// Holding is what the organization Anthropic's API names a token's account
// in says of the token, held to its account's, as the record has it.
type Holding int

const (
	// Unheld is an organization held to nothing: none named, or the record
	// without the account.
	Unheld Holding = iota
	// HeldOwn is the account's own organization.
	HeldOwn
	// HeldNew is an organization the record doesn't know the account's, nor
	// has for another account: the account's, once it's recorded.
	HeldNew
	// HeldForeign is another subscription's: other than the account's, or,
	// where the account's isn't known, another account's.
	HeldForeign
)

// Hold holds organization, as the API named it of a token of the account
// with the given id, to the account's, as the record has it, and returns the
// other account it's the organization of, where that's what makes it
// another subscription's.
func (r Record) Hold(id, organization string) (Holding, Entry) {
	own, ok := r.ByID(id)
	switch {
	case organization == "" || !ok:
		return Unheld, Entry{}
	case own.Organization == organization:
		return HeldOwn, Entry{}
	case own.Organization != "":
		return HeldForeign, Entry{}
	}
	if other, held := r.ByOrganization(organization); held {
		return HeldForeign, other
	}
	return HeldNew, Entry{}
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
