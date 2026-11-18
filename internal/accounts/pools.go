package accounts

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

var (
	// ErrNoPools is what adding a pool to a config naming none fails with: the
	// router writes the first into it as it moves the history to pools, as
	// it first starts, on a new install as on one from before pools, and the
	// pools it routes for, as the move left them, as it starts on a config
	// that lost them; a pool added before would take every account in.
	ErrNoPools = errors.New("the config names no pools yet: the router writes them into it as it starts, and pools are added after: " +
		"start the router with switchboard service install where it isn't running; where it is, switchboard logs router says what holds it up; " +
		"then add the pool again")
	// ErrNoPoolLabel is what adding a pool without a label fails with: every
	// pool has one.
	ErrNoPoolLabel = errors.New("a pool needs a label")
)

// PoolLabelTakenError is the error of giving a pool the label another pool
// shows already, ignoring case.
type PoolLabelTakenError struct {
	Label string
	// Holder is the pool showing it.
	Holder config.Pool
}

func (e *PoolLabelTakenError) Error() string {
	return fmt.Sprintf("%s is %s's label already, ignoring case: give the pool its own", prose.Quoted(prose.Clean(e.Label)), poolSaid(e.Holder))
}

// UnfencedError is the error of leaving a second pool unfenced: Unfenced is
// the one that is already, as a session started outside every fence would
// have two pools to be in.
type UnfencedError struct {
	Unfenced config.Pool
}

func (e *UnfencedError) Error() string {
	return fmt.Sprintf("%s is unfenced already, and only one pool can be, as a session started outside every fence would have two pools to be in: "+
		"give this one its directories, or fence %s first with switchboard pool edit %s --dirs",
		poolSaid(e.Unfenced), prose.Clean(e.Unfenced.Label), prose.Clean(e.Unfenced.ID))
}

// PoolNeededError is what adding an account that names no pool fails with
// where the config has pools, every one fenced, so there's none the verbs act
// on to add it to: Pools are those it can go in.
type PoolNeededError struct {
	Pools config.Pools
}

func (e *PoolNeededError) Error() string {
	named := make([]string, len(e.Pools))
	for i, p := range e.Pools {
		named[i] = poolSaid(p)
	}
	return "every pool is fenced, so which the account goes in is yours to say: add it with switchboard accounts add <label> --pool <id>, naming " + prose.ListOr(named)
}

// inPool returns account in the pool it's added to, of those cfg configures,
// nil configuring none: the one it names, else, where cfg has pools, the one
// the verbs act on, as config.Pools' Implicit gives it, as accounts add adds
// one without --pool. It fails, as config.PoolNotConfigured says, where it
// names one cfg doesn't configure, as in a config without pools, and with a
// PoolNeededError where it names none and every pool is fenced.
func inPool(cfg *config.Config, account config.NewAccount) (config.NewAccount, error) {
	var pools config.Pools
	if cfg != nil {
		pools = cfg.Pools
	}
	switch _, configured := pools.ByID(account.Pool); {
	case account.Pool != "" && !configured:
		return config.NewAccount{}, config.PoolNotConfigured(account.Pool)
	case account.Pool != "" || len(pools) == 0:
		return account, nil
	}
	p, ok := pools.Implicit()
	if !ok {
		return config.NewAccount{}, &PoolNeededError{Pools: pools}
	}
	account.Pool = p.ID
	return account, nil
}

// poolSaid names a pool by its label and its id, as "Acme (q7mt)".
func poolSaid(p config.Pool) string {
	return prose.Clean(p.Label) + " (" + prose.Clean(p.ID) + ")"
}

// PoolChange is what editing a pool changes of it: each field left nil
// leaves what it sets as it is.
type PoolChange struct {
	// Label is what it's shown by, which every pool has, its own.
	Label *string
	// Dirs are the directories it's fenced to, in place of those it has,
	// each an absolute path or one starting ~/: none unfences it.
	Dirs *[]string
}

// Asks reports whether the change asks for anything.
func (c PoolChange) Asks() bool {
	return c.Label != nil || c.Dirs != nil
}

// EditedPool is a pool as it was and as an edit left it, and every pool as
// the edit left them.
type EditedPool struct {
	Was, Now config.Pool
	Pools    config.Pools
}

// Before returns the pools as they were before the edit: Pools, the pool
// edited as it was.
func (e EditedPool) Before() config.Pools {
	before := slices.Clone(e.Pools)
	if i := slices.IndexFunc(before, func(p config.Pool) bool { return config.SamePool(p.ID, e.Was.ID) }); i >= 0 {
		before[i] = e.Was
	}
	return before
}

// Changed reports whether the edit changed the pool, as the config reads it.
func (e EditedPool) Changed() bool {
	return e.Was.Label != e.Now.Label || !slices.Equal(e.Was.WrittenDirs, e.Now.WrittenDirs)
}

// AddPool adds a pool, shown as the label given, fenced to the directories
// given, each an absolute path or one starting ~/, or unfenced without any:
// a [[pool]] table in the config file, under an id switchboard makes, as
// identity's NewPoolID draws one, never one the accounts record or the
// config holds, drawn under the record's lock. It fails with ErrNoPools in a
// config from before pools; with ErrNoPoolLabel, or a PoolLabelTakenError,
// for a label that's none or another pool's; with an UnfencedError where it
// would be a second pool unfenced; and as the config file's edits do, as for
// directories that overlap another pool's.
func (r Registry) AddPool(label string, dirs []string) (AddedPool, error) {
	var added AddedPool
	err := r.Record.Change(func(record *identity.Record, _ func() error) error {
		draft, err := config.Edit(r.ConfigPath, r.HomeDir)
		if err != nil {
			return err
		}
		id, err := poolMade(draft, label, dirs, r.drawnFrom(*record))
		if err != nil {
			return err
		}
		if err := draft.Save(); err != nil {
			return err
		}
		added.Pools = draft.Config().Pools
		added.Pool, _ = added.Pools.ByID(id)
		return nil
	})
	if err != nil {
		return AddedPool{}, err
	}
	poolAdded(added.Pool)
	return added, nil
}

// AddedPool is a pool AddPool added, and every pool as the add left them.
type AddedPool struct {
	Pool  config.Pool
	Pools config.Pools
}

// Before returns the pools as they were before the add: Pools, but the pool
// added.
func (a AddedPool) Before() config.Pools {
	return slices.DeleteFunc(slices.Clone(a.Pools), func(p config.Pool) bool { return config.SamePool(p.ID, a.Pool.ID) })
}

// poolAdded logs a pool added, by AddPool, or made with an account added or
// moved into it.
func poolAdded(added config.Pool) {
	logger.Info("added a pool", "pool", added.ID, "fenced", added.Fenced())
}

// poolTaken fails where the pool with the given id, "" for one not yet
// added, can't be shown as the label given, as poolLabelTaken says, or
// where unfenced asks for it to be unfenced and another is already, as
// unfencedTaken says.
func poolTaken(pools config.Pools, id, label string, unfenced bool) error {
	if err := poolLabelTaken(pools, id, label); err != nil {
		return err
	}
	return unfencedTaken(pools, id, unfenced)
}

// poolLabelTaken fails with ErrNoPoolLabel for a label given that shows
// nothing; as config.TakePoolLabel does for one no pool can have; and where
// a pool, but the one with the given id, shows the label as it's taken, as
// a PoolLabelTakenError.
func poolLabelTaken(pools config.Pools, id, given string) error {
	if prose.Clean(given) == "" {
		return ErrNoPoolLabel
	}
	label, err := config.TakePoolLabel(id, given)
	if err != nil {
		return err
	}
	if holder, taken := pools.Labelled(id, label); taken {
		return &PoolLabelTakenError{Label: label, Holder: holder}
	}
	return nil
}

// unfencedTaken fails where unfenced asks for the pool with the given id to
// be unfenced and another is already, as an UnfencedError.
func unfencedTaken(pools config.Pools, id string, unfenced bool) error {
	if other, ok := pools.Unfenced(); ok && unfenced && other.ID != id {
		return &UnfencedError{Unfenced: other}
	}
	return nil
}

// EditPool changes the pool with the given id, ignoring case, as change
// says, in its [[pool]] table in the config file: a change that leaves it as
// it was writes nothing. It fails, matching config.ErrNotConfigured, where no
// pool has the id; as AddPool does for its label and directories; and as the
// config file's edits do, as for directories written over lines, which it
// can't change in place.
func (r Registry) EditPool(id string, change PoolChange) (EditedPool, error) {
	draft, err := config.Edit(r.ConfigPath, r.HomeDir)
	if err != nil {
		return EditedPool{}, err
	}
	was, err := configuredPool(draft.Config(), id)
	if err != nil {
		return EditedPool{}, err
	}
	pools, unfencing := draft.Config().Pools, change.Dirs != nil && len(*change.Dirs) == 0
	if change.Label != nil {
		if err := poolLabelTaken(pools, was.ID, *change.Label); err != nil {
			return EditedPool{}, err
		}
	}
	if err := unfencedTaken(pools, was.ID, unfencing); err != nil {
		return EditedPool{}, err
	}
	if err := change.apply(draft, was.ID); err != nil {
		return EditedPool{}, err
	}
	now, err := configuredPool(draft.Config(), was.ID)
	if err != nil {
		return EditedPool{}, err
	}
	edited := EditedPool{Was: was, Now: now, Pools: draft.Config().Pools}
	if !edited.Changed() {
		return edited, nil
	}
	if err := draft.Save(); err != nil {
		return EditedPool{}, err
	}
	logger.Info("edited a pool", "pool", was.ID, "fenced", now.Fenced())
	return edited, nil
}

// apply makes the change to the pool with the given id in the draft, its
// directories written as the draft's FromHome writes them.
func (c PoolChange) apply(draft *config.Draft, id string) error {
	if c.Label != nil {
		if err := draft.SetPoolLabel(id, *c.Label); err != nil {
			return err
		}
	}
	if c.Dirs != nil {
		return draft.SetPoolDirs(id, draft.FromHome(*c.Dirs))
	}
	return nil
}

// RemovePool removes the pool with the given id, ignoring case: its
// [[pool]] table in the config file, with the comments directly above it.
// It fails, matching config.ErrNotConfigured, where no pool has the id;
// matching config.ErrPoolHasAccounts while an account is in it, naming
// them; and matching config.ErrOnlyPool for the only pool.
func (r Registry) RemovePool(id string) (config.Pool, error) {
	draft, err := config.Edit(r.ConfigPath, r.HomeDir)
	if err != nil {
		return config.Pool{}, err
	}
	removed, err := configuredPool(draft.Config(), id)
	if err != nil {
		return config.Pool{}, err
	}
	if err := draft.RemovePool(removed.ID); err != nil {
		return config.Pool{}, err
	}
	if err := draft.Save(); err != nil {
		return config.Pool{}, err
	}
	logger.Info("removed a pool", "pool", removed.ID)
	return removed, nil
}

// configuredPool returns the pool with the given id, ignoring case, that cfg
// configures, failing, matching config.ErrNotConfigured, where none has it,
// as where cfg is nil; and where a pool is labelled so, ignoring case, saying
// to give its id, as "Acme is a label: give its id, q7mt".
func configuredPool(cfg *config.Config, id string) (config.Pool, error) {
	if cfg == nil {
		return config.Pool{}, config.PoolNotConfigured(id)
	}
	if p, ok := cfg.Pools.ByID(id); ok {
		return p, nil
	}
	if p, ok := cfg.Pools.Labelled("", id); ok {
		return config.Pool{}, fmt.Errorf("%w: %s is a label: give its id, %s", config.PoolNotConfigured(id), prose.Clean(redact.Text(id)), prose.Clean(p.ID))
	}
	return config.Pool{}, config.PoolNotConfigured(id)
}

// SignedIn returns the account of cfg the browser and the Claude apps are
// signed in to, as config.Config's SignedIn has it of the first pool the
// accounts record notes, as notedFirst reads it. It reports false where that
// pool, or the config, has no account.
func (r Registry) SignedIn(cfg *config.Config) (config.Account, bool) {
	return cfg.SignedIn(r.notedFirst())
}

// FirstPool returns the id of cfg's first pool, whose primary the browser
// and the Claude apps are signed in to, as config.Pools' First has it of the
// one the accounts record notes, as notedFirst reads it: "" where cfg has no
// pools.
func (r Registry) FirstPool(cfg *config.Config) string {
	return cfg.Pools.First(r.notedFirst())
}

// notedFirst is the id of the first pool the accounts record notes: "" where
// it notes none, or can't be read, which is logged once for each record, as
// unreadFirst keeps them, the config's first pool standing in.
func (r Registry) notedFirst() string {
	record, err := r.Record.Read()
	if err != nil {
		if _, warned := unreadFirst.LoadOrStore(r.Record.Path(), true); !warned {
			logger.Warn("can't read the accounts record, so the config's first pool is taken for the one the browser and the Claude apps are signed in to", "error", err)
		}
	}
	first, _ := record.FirstPool()
	return first.ID
}

// unreadFirst holds the paths of the accounts records notedFirst couldn't
// read, so each is warned of once, not at every review and token, as
// idmove's FirstPool warns of one.
var unreadFirst sync.Map
