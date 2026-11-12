package idmove

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// poolsMarkName is the name, in the state directory, of the file that
	// marks the move to pools done, holding when it was.
	poolsMarkName = "pools.moved"
	// firstLabel is the label the first pool is made with.
	firstLabel = "Default"
)

var (
	// ErrAwaitingPools is what a move to pools run beside a router that
	// doesn't route in pools returns once the config holds the first pool:
	// the rest of it, which gives the history that pool, waits for the
	// router to take it up, so no line the router files after a day is moved
	// names none.
	ErrAwaitingPools = errors.New("the config holds the first pool, so the rest of the move to pools waits for the router to take it up")
	// errIDsFirst is what the move to pools fails with while the move onto
	// the accounts' ids is owed, which it follows.
	errIDsFirst = errors.New("the move onto the accounts' ids isn't done yet, and the move to pools follows it")
)

// PoolMover moves switchboard's history to pools, once, as the router first
// starts on a build with pools, once the move onto the accounts' ids is
// done: the first pool noted in the accounts record, where it notes none,
// the config's first where the config has pools, else one made, its id
// random, before anything else changes, so a stop anywhere carries on to the
// same end the next time; written to a config without pools, every account
// in it, in one edit; where a router runs beside it that doesn't route in
// pools, ErrAwaitingPools returned for it to take the pool up; then every
// request line, every pin event and every day's summary given the first
// pool where it names none, as rewriteHistory writes them, so a file moved
// reads the same moved again; and what the router builds of them built
// again.
type PoolMover struct {
	// ConfigPath is the config file, and StateDir the state directory.
	ConfigPath string
	StateDir   string
	// HomeDir returns the home directory the config file is read in, which
	// a pool's directory starting ~/ is in: nil knows of none.
	HomeDir func() (string, error)
	// History is the history's files as the router running beside the move
	// keeps them: nil opens them afresh, where none does.
	History *History
	// Pooled, where it's set, reports whether the router running beside the
	// move routes in pools, as it does once it has taken up the pools its
	// config holds: nil where none runs, as for a copy of a state directory.
	Pooled func() bool
	// Now reads the clock, and Random is what the first pool's id is drawn
	// from: nil reads the system's, and draws from crypto/rand.
	Now    func() time.Time
	Random io.Reader

	// stop, where it's set, is called with each step's name once it's done:
	// a failure stops the move there, as a test of one stopped there has it.
	stop func(step string) error
}

// PoolsMoved reports whether the move to pools is done in the state
// directory given, as its mark says.
func PoolsMoved(stateDir string) bool {
	_, err := os.Lstat(filepath.Join(stateDir, poolsMarkName))
	return err == nil
}

// PoolsPending reports whether the history in the state directory given
// waits on the move to pools: it isn't done, and there's a history to move,
// the request ledger's directory.
func PoolsPending(stateDir string) bool {
	if PoolsMoved(stateDir) {
		return false
	}
	_, err := os.Lstat(ledger.Dir(stateDir))
	return err == nil
}

// Owed is the move the history in the state directory given waits on, as
// status.Moving's Move names it: the move onto the accounts' ids, as Pending
// says, then the move to pools, as PoolsPending says; "" where it waits on
// neither.
func Owed(stateDir string) string {
	switch {
	case Pending(stateDir):
		return status.MovingIDs
	case PoolsPending(stateDir):
		return status.MovingPools
	}
	return ""
}

// Run moves the history to pools, as PoolMover says, unless it's done, and
// marks it done. It fails, changing no more, where a step does, which a Run
// after carries on from; and, changing nothing, while the move onto the
// accounts' ids is owed.
func (m *PoolMover) Run(ctx context.Context) error {
	if PoolsMoved(m.StateDir) {
		return nil
	}
	if !Moved(m.StateDir) {
		return errIDsFirst
	}
	steps := []step{
		{"pools first", m.noteFirst},
		{"pools config", func() error { _, err := m.configure(); return err }},
		{"pools taken up", m.awaitPools},
		{"pools history", func() error { return m.moveHistory(ctx) }},
		{"pools rebuild", func() error { return m.history().Ledger.Rebuild() }},
		{"pools mark", m.mark},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			return err
		}
		if err := m.stopped(s.name); err != nil {
			return err
		}
	}
	return nil
}

// RunBeside runs the move once, as Run does, beside a router that keeps the
// history's files as history has them, and routes in pools as pooled
// reports, failing with ErrAwaitingPools, for the router to run it again
// once it has taken the first pool up.
func (m *PoolMover) RunBeside(ctx context.Context, history History, pooled func() bool) error {
	m.History, m.Pooled = &history, pooled
	return m.Run(ctx)
}

// stopped fails where the move is to stop once the step with the given name
// is done, as stop says.
func (m *PoolMover) stopped(name string) error {
	if m.stop == nil {
		return nil
	}
	return m.stop(name)
}

// noteFirst notes the first pool in the accounts record, under its lock,
// where it notes none: the config's first, in its order, where it has pools,
// as where another Mac sharing it moved first; else one made, its id drawn
// as identity's NewPoolID draws one. The config is read under the lock, as
// accounts add and remove save it with the record under it.
func (m *PoolMover) noteFirst() error {
	return m.record().Change(func(r *identity.Record, _ func() error) error {
		if _, ok := r.FirstPool(); ok {
			return nil
		}
		draft, err := config.Edit(m.ConfigPath, m.HomeDir)
		if err != nil {
			return err
		}
		if cfg := draft.Config(); cfg != nil && len(cfg.Pools) > 0 {
			r.NoteFirst(cfg.Pools[0].ID, m.now().UTC())
			return nil
		}
		id, err := r.NewPoolID(m.random(), configuredIDs(draft))
		if err != nil {
			return fmt.Errorf("make the first pool: %w", err)
		}
		r.NoteFirst(id, m.now().UTC())
		logger.Info("made the first pool", "pool", id)
		return nil
	})
}

// configuredIDs are the ids of the accounts and pools draft configures.
func configuredIDs(draft *config.Draft) []string {
	cfg := draft.Config()
	if cfg == nil {
		return nil
	}
	return slices.Concat(cfg.Accounts.IDs(), cfg.Pools.IDs())
}

// configure writes the first pool the accounts record notes to a config
// that has no pools, as config's AddPool adds it, every account in it, in
// one edit, under the record's lock: labelled as the record last knew it,
// where it's removed, else firstLabel; made afresh where the config changed
// as it was edited, configTries times at most. A config with pools, or
// without accounts, is left as it is. It reports whether it changed the
// config, and fails where the record notes no first pool.
func (m *PoolMover) configure() (bool, error) {
	for range configTries {
		changed := false
		err := m.record().Change(func(r *identity.Record, _ func() error) error {
			first, ok := r.FirstPool()
			if !ok {
				return errors.New("the accounts record notes no first pool")
			}
			draft, err := config.Edit(m.ConfigPath, m.HomeDir)
			if err != nil {
				return err
			}
			if cfg := draft.Config(); cfg == nil || len(cfg.Pools) > 0 {
				return nil
			}
			if err := draft.AddPool(config.NewPool{ID: first.ID, Label: cmp.Or(first.Label, firstLabel)}); err != nil {
				return err
			}
			changed = true
			return draft.Save()
		})
		if err == nil && changed {
			logger.Info("wrote the first pool in the config, every account in it", "path", m.ConfigPath)
		}
		if !errors.Is(err, config.ErrChanged) {
			return changed, err
		}
	}
	return false, fmt.Errorf("write the first pool in the config: %w", config.ErrChanged)
}

// HealPools writes the first pool the accounts record in the state
// directory stateDir notes in the config file at configPath, every account
// in it, as the move to pools writes it, where that move is done there and
// the config names no pool, as where a change to the config made by hand,
// or a repository it's kept in, put one from before pools back. A router
// heals its config before it reads it, so it starts in pools. The config is
// read in the home directory homeDir returns. It reports whether it changed
// the config.
func HealPools(configPath, stateDir string, homeDir func() (string, error)) (bool, error) {
	if !PoolsMoved(stateDir) {
		return false, nil
	}
	m := &PoolMover{ConfigPath: configPath, StateDir: stateDir, HomeDir: homeDir}
	return m.configure()
}

// awaitPools fails with ErrAwaitingPools where the router running beside the
// move doesn't route in pools.
func (m *PoolMover) awaitPools() error {
	if m.Pooled == nil || m.Pooled() {
		return nil
	}
	return ErrAwaitingPools
}

// FirstPool is the ledger's option that gives the work of a day's summary
// from before pools the first pool, where the day's lines name a pool, as
// ledger.FirstPool says, the first pool as the accounts record in the state
// directory given notes it as the ledger asks: none where the record can't be
// read, which is logged, or notes none.
func FirstPool(stateDir string) ledger.Option {
	return ledger.FirstPool(func() string {
		record, err := identity.NewFile(stateDir).Read()
		if err != nil {
			logger.Warn("can't read the accounts record, so a day's summary from before pools keeps its work in no pool", "error", err)
			return ""
		}
		first, _ := record.FirstPool()
		return first.ID
	})
}

// moveHistory gives each line of the history that names no pool the first
// pool the accounts record notes, as rewriteHistory writes it. It stops,
// failing, once ctx ends.
func (m *PoolMover) moveHistory(ctx context.Context) error {
	record, err := m.record().Read()
	if err != nil {
		return err
	}
	first, ok := record.FirstPool()
	if !ok {
		return errors.New("move the history to pools: the accounts record notes no first pool")
	}
	return rewriteHistory(ctx, m.history(), ledger.Renaming{Unpooled: first.ID}, "pools ", m.stopped)
}

// mark marks the move done, as PoolsMoved reads it.
func (m *PoolMover) mark() error {
	if err := markDone(m.StateDir, poolsMarkName, m.now()); err != nil {
		return fmt.Errorf("mark the move to pools done: %w", err)
	}
	logger.Info("moved the history to pools")
	return nil
}

// history is the history's files the move writes anew: the router's, where
// it's running, else opened afresh, as historyIn opens them.
func (m *PoolMover) history() History {
	if m.History != nil {
		return *m.History
	}
	return historyIn(m.StateDir, m.now)
}

// record is the accounts record's file.
func (m *PoolMover) record() identity.File {
	return identity.NewFile(m.StateDir)
}

// now reads the mover's clock, as readClock does.
func (m *PoolMover) now() time.Time {
	return readClock(m.Now)
}

// random is what the first pool's id is drawn from, as drawnFrom gives it.
func (m *PoolMover) random() io.Reader {
	return drawnFrom(m.Random)
}
