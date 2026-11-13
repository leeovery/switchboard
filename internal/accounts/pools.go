package accounts

import (
	"errors"
	"fmt"
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/prose"
)

var (
	// ErrNoPools is what adding a pool to a config naming none fails with: the
	// router writes the first into it as it moves the history to pools, and
	// the pools it routes for, as the move left them, as it starts on a
	// config that lost them; a pool added before would take every account in.
	ErrNoPools = errors.New("the config names no pools: the router writes them into it, the first as it moves the history to pools, and pools are added after")
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

// EditedPool is a pool as it was and as an edit left it.
type EditedPool struct {
	Was, Now config.Pool
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
func (r Registry) AddPool(label string, dirs []string) (config.Pool, error) {
	var added config.Pool
	err := r.Record.Change(func(record *identity.Record, _ func() error) error {
		draft, err := config.Edit(r.ConfigPath, r.HomeDir)
		if err != nil {
			return err
		}
		cfg := draft.Config()
		if cfg == nil || len(cfg.Pools) == 0 {
			return ErrNoPools
		}
		if err := poolTaken(cfg.Pools, "", label, len(dirs) == 0); err != nil {
			return err
		}
		id, err := record.NewPoolID(r.random(), slices.Concat(cfg.Accounts.IDs(), cfg.Pools.IDs()))
		if err != nil {
			return err
		}
		if err := draft.AddPool(config.NewPool{ID: id, Label: label, Dirs: dirs}); err != nil {
			return err
		}
		if err := draft.Save(); err != nil {
			return err
		}
		added, _ = draft.Config().Pools.ByID(id)
		return nil
	})
	if err != nil {
		return config.Pool{}, err
	}
	logger.Info("added a pool", "pool", added.ID, "fenced", added.Fenced())
	return added, nil
}

// poolTaken fails where a pool, but the one with the given id, shows label,
// as a PoolLabelTakenError, or where unfenced asks for the pool to be
// unfenced and another is already, as an UnfencedError; and with
// ErrNoPoolLabel for no label.
func poolTaken(pools config.Pools, id, label string, unfenced bool) error {
	if prose.Clean(label) == "" {
		return ErrNoPoolLabel
	}
	if holder, taken := pools.Labelled(id, label); taken {
		return &PoolLabelTakenError{Label: label, Holder: holder}
	}
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
	label, unfencing := was.Label, change.Dirs != nil && len(*change.Dirs) == 0
	if change.Label != nil {
		label = *change.Label
	}
	if err := poolTaken(draft.Config().Pools, was.ID, label, unfencing); err != nil {
		return EditedPool{}, err
	}
	if err := change.apply(draft, was.ID); err != nil {
		return EditedPool{}, err
	}
	now, err := configuredPool(draft.Config(), was.ID)
	if err != nil {
		return EditedPool{}, err
	}
	edited := EditedPool{Was: was, Now: now}
	if !edited.Changed() {
		return edited, nil
	}
	if err := draft.Save(); err != nil {
		return EditedPool{}, err
	}
	logger.Info("edited a pool", "pool", was.ID, "fenced", now.Fenced())
	return edited, nil
}

// apply makes the change to the pool with the given id in the draft.
func (c PoolChange) apply(draft *config.Draft, id string) error {
	if c.Label != nil {
		if err := draft.SetPoolLabel(id, *c.Label); err != nil {
			return err
		}
	}
	if c.Dirs != nil {
		return draft.SetPoolDirs(id, *c.Dirs)
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
// as where cfg is nil.
func configuredPool(cfg *config.Config, id string) (config.Pool, error) {
	if cfg != nil {
		if p, ok := cfg.Pools.ByID(id); ok {
			return p, nil
		}
	}
	return config.Pool{}, config.PoolNotConfigured(id)
}
