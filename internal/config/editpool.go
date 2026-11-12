package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
)

var (
	// ErrOnlyPool is what removing a pool fails with, wrapped, when it's the
	// only one: an account added needs a pool to go into.
	ErrOnlyPool = errors.New("a config with pools needs one at least, for an account added to go into")
	// ErrPoolHasAccounts is what removing a pool fails with, wrapped, while
	// an account is in it, so no edit puts a pool's accounts in another.
	ErrPoolHasAccounts = errors.New("has accounts: move each to another pool, or remove it, first")
)

// PoolNotConfigured is the error of acting on the pool with the given id,
// which no pool has: it matches ErrNotConfigured, and hides anything in the
// id that looks like a token.
func PoolNotConfigured(id string) error {
	return fmt.Errorf("%s is %w", poolNamed(id), ErrNotConfigured)
}

// poolConfigured is the error of adding a pool with the given id, which a
// pool has already, ignoring case: it matches ErrConfigured.
func poolConfigured(id string) error {
	return fmt.Errorf("%s is %w", poolNamed(id), ErrConfigured)
}

// NewPool is a pool to add to the config file.
type NewPool struct {
	// ID is the id switchboard made for it.
	ID string
	// Label is the name it's shown by: none shows it by its id.
	Label string
	// Dirs are the directories it's fenced to, each an absolute path or one
	// starting ~/, written as they're given: none leaves it unfenced.
	Dirs []string
}

// taken returns the pool with its label as poolLabelOf takes it, and no dirs
// for none, failing, saying why, unless its id and label are ones a pool can
// have.
func (p NewPool) taken() (NewPool, error) {
	label, labelErr := poolLabelOf(p.ID, p.Label)
	if err := errors.Join(checkID(poolKind, poolNamed(p.ID), p.ID), labelErr); err != nil {
		return NewPool{}, err
	}
	p.Label, p.Dirs = label, dirsGiven(p.Dirs)
	return p, nil
}

// poolLabelOf returns the label given the pool with the given id as labelOf
// returns an account's.
func poolLabelOf(id, given string) (string, error) {
	label, err := takeLabel(poolKind, poolNamed(id), given)
	if err != nil || label == id {
		return "", err
	}
	return label, nil
}

// dirsGiven returns dirs as a pool's table reads them back: nil for none.
func dirsGiven(dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	return slices.Clone(dirs)
}

// AddPool adds a [[pool]] table for the pool: after the last pool's, else
// after the last account's, else before the first table, else at the end,
// its directories on one line. Added to a config from before pools, which
// has accounts, it puts each in the pool, as a config with pools needs every
// account in one. It fails with a WrongID or a WrongLabel for an id or a
// label no pool can have, matching ErrConfigured where a pool has the id
// already, ignoring case, and, as the config it would make does, where its
// directories overlap another's, or it would be a second pool unfenced.
func (d *Draft) AddPool(given NewPool) error {
	p, err := given.taken()
	if err != nil {
		return err
	}
	if d.poolIndex(p.ID) >= 0 {
		return poolConfigured(p.ID)
	}
	change := fmt.Sprintf("add pool %q", p.ID)
	if _, err := d.poolTables(change); err != nil {
		return err
	}
	doc, want := d.doc, d.file
	if len(d.file.Pools) == 0 {
		if doc, want.Accounts, err = d.allInPool(p.ID, change); err != nil {
			return err
		}
	}
	want.Pools = append(slices.Clone(d.file.Pools), filePool(p))
	return d.take(doc.insert(doc.newPoolAt(), p.lines()), want, change)
}

// allInPool returns the draft's text, and its accounts, with every account
// put in the pool with the given id, its pool key set below its table's last
// key, failing as accountTables does.
func (d *Draft) allInPool(id, change string) (document, []fileAccount, error) {
	tables, err := d.accountTables(change)
	if err != nil {
		return document{}, nil, err
	}
	doc, accounts := d.doc, slices.Clone(d.file.Accounts)
	for i := len(tables) - 1; i >= 0; i-- {
		doc = doc.withKey(tables[i], "pool", quote(id))
		accounts[i].Pool = id
	}
	return doc, accounts, nil
}

// RemovePool removes the pool's [[pool]] table, and the comments directly
// above it. It fails, matching ErrNotConfigured, when no pool has the id,
// ignoring case; matching ErrOnlyPool for the only pool; and matching
// ErrPoolHasAccounts, naming them, while any account is in it.
func (d *Draft) RemovePool(id string) error {
	i := d.poolIndex(id)
	if i < 0 {
		return PoolNotConfigured(id)
	}
	id = d.file.Pools[i].ID
	if in := d.accountsIn(id); len(in) > 0 {
		return fmt.Errorf("%s, holding %s, %w", poolNamed(id), prose.List(in), ErrPoolHasAccounts)
	}
	if len(d.file.Pools) == 1 {
		return fmt.Errorf("%s is the only one, and %w", poolNamed(id), ErrOnlyPool)
	}
	change := fmt.Sprintf("remove pool %q", id)
	tables, err := d.poolTables(change)
	if err != nil {
		return err
	}
	want := d.file
	want.Pools = slices.Delete(slices.Clone(d.file.Pools), i, i+1)
	return d.take(d.doc.cut(tables[i]), want, change)
}

// accountsIn returns the accounts in the pool with the given id, each named
// as errors name it.
func (d *Draft) accountsIn(id string) []string {
	var in []string
	for i, a := range d.file.Accounts {
		if strings.EqualFold(a.Pool, id) {
			in = append(in, a.name(i))
		}
	}
	return in
}

// SetPoolLabel sets the label of the pool with the given id, ignoring case,
// as SetLabel sets an account's. It fails, matching ErrNotConfigured, when no
// pool has the id, and with a WrongLabel for a label no pool can have.
func (d *Draft) SetPoolLabel(id, given string) error {
	i := d.poolIndex(id)
	if i < 0 {
		return PoolNotConfigured(id)
	}
	id = d.file.Pools[i].ID
	label, err := poolLabelOf(id, given)
	if err != nil {
		return err
	}
	if d.file.Pools[i].shownAs() == (filePool{ID: id, Label: label}).shownAs() {
		return nil
	}
	return d.setPoolKey(i, "label", written(label), func(p *filePool) { p.Label = label })
}

// SetPoolDirs sets the directories of the pool with the given id, ignoring
// case, in place of those it has, each an absolute path or one starting ~/,
// written as given on one line: none takes them out, leaving it unfenced. It
// fails, matching ErrNotConfigured, when no pool has the id, and, as the
// config it would make does, for a directory a pool can't have, one that
// overlaps another, or a second pool unfenced.
func (d *Draft) SetPoolDirs(id string, dirs []string) error {
	i := d.poolIndex(id)
	if i < 0 {
		return PoolNotConfigured(id)
	}
	dirs = dirsGiven(dirs)
	return d.setPoolKey(i, "dirs", writtenDirs(dirs), func(p *filePool) { p.Dirs = dirs })
}

// SetPool moves the account with the given id into the pool with the given
// id, ignoring case, as setAccountKey sets a key, writing the pool's own id.
// Moved out of a pool, it takes off any primary it sets, as it's that pool's
// primary no longer and the pool it goes into may have one. It fails,
// matching ErrNotConfigured, when no account or no pool has the id.
func (d *Draft) SetPool(id, pool string) error {
	i := d.index(id)
	if i < 0 {
		return NotConfigured(id)
	}
	p := d.poolIndex(pool)
	if p < 0 {
		return PoolNotConfigured(pool)
	}
	pool = d.file.Pools[p].ID
	if strings.EqualFold(d.file.Accounts[i].Pool, pool) {
		return nil
	}
	change := fmt.Sprintf("set pool to %q in account %q", pool, id)
	tables, err := d.accountTables(change)
	if err != nil {
		return err
	}
	want := d.file
	want.Accounts = slices.Clone(d.file.Accounts)
	want.Accounts[i].Pool, want.Accounts[i].Primary = pool, false
	// The pool key goes in below the table's last key, so the primary lines,
	// among its keys, stay where they were.
	marks := d.doc.linesOf(primaryKey, tables[i])
	return d.take(d.doc.withKey(tables[i], "pool", quote(pool)).without(marks), want, change)
}

// setPoolKey sets key, in the [[pool]] table of the pool at i, to value, as
// setAccountKey sets an account's, set making the change of the pool.
func (d *Draft) setPoolKey(i int, key, value string, set func(*filePool)) error {
	want := d.file
	want.Pools = slices.Clone(d.file.Pools)
	set(&want.Pools[i])
	if reflect.DeepEqual(want.Pools[i], d.file.Pools[i]) {
		return nil
	}
	id := d.file.Pools[i].ID
	change := fmt.Sprintf("set %s to %s in pool %q", key, value, id)
	if value == "" {
		change = fmt.Sprintf("take %s out of pool %q", key, id)
	}
	tables, err := d.poolTables(change)
	if err != nil {
		return err
	}
	return d.take(d.doc.withKey(tables[i], key, value), want, change)
}

// poolIndex returns where the pool with the given id, ignoring case, is
// among the draft's pools, or -1 when none has it.
func (d *Draft) poolIndex(id string) int {
	return configuredPool(d.file.Pools, id)
}

// poolTables returns the span of each pool's [[pool]] table, in the pools'
// order, failing, as change can't be made, when the text doesn't give each
// pool a table of its own.
func (d *Draft) poolTables(change string) ([]span, error) {
	tables := d.doc.pools()
	if len(tables) != len(d.file.Pools) {
		return nil, d.cantMake(change)
	}
	return tables, nil
}
