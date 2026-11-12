package idmove

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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
	// doesn't take up the pools of its config returns once the config holds
	// the first pool: the rest of it, which gives the history that pool,
	// waits for the router to take it up, so no line the router files after
	// a day is moved names none.
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
// in it, in one edit; taken up by the router running beside it, as TakeUp
// has it, else ErrAwaitingPools returned for it to take the pool up; then
// every request line, every pin event and every day's summary given the
// first pool where it names none, as rewriteHistory writes them, so a file
// moved reads the same moved again; and what the router builds of them
// built again.
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
	// TakeUp, where it's set, has the router running beside the move take up
	// the pools its config now holds, at once, as the move has written the
	// first there, reporting whether it routes in them, so no line it files
	// after a day is moved names none: nil where none runs, as for a copy of
	// a state directory.
	TakeUp func() bool
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
		{"pools config", m.writeFirst},
		{"pools taken up", m.takeUp},
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
// history's files as history has them, and takes up the pools of its config
// as takeUp does, as TakeUp says, failing with ErrAwaitingPools, for the
// router to run it again once it has taken the first pool up.
func (m *PoolMover) RunBeside(ctx context.Context, history History, takeUp func() bool) error {
	m.History, m.TakeUp = &history, takeUp
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
// as where another Mac sharing it moved first, the record kept in step with
// every pool the config has, as KeepPoolsUp keeps it; else one made, its id
// drawn as identity's NewPoolID draws one. The config is read under the
// lock, as accounts add and remove save it with the record under it.
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
			now := m.now().UTC()
			r.KeepPoolsUp(identity.Followed{Accounts: cfg.Accounts, Pools: cfg.Pools}, now)
			r.NoteFirst(cfg.Pools[0].ID, identity.WrittenOf(cfg.Pools[0], cfg.Accounts), now)
			return nil
		}
		id, err := r.NewPoolID(m.random(), configuredIDs(draft))
		if err != nil {
			return fmt.Errorf("make the first pool: %w", err)
		}
		r.NoteFirst(id, identity.PoolWritten{Label: firstLabel}, m.now().UTC())
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

// configure writes the pools the accounts record keeps to a config that has
// none, as restorePools has them, in one edit, under the record's lock: made
// afresh where the config changed as it was edited, configTries times at
// most. A config with pools is left as it is, as is one without accounts,
// of which draft holds no config. It reports whether it changed the config,
// and fails as restorePools does.
func (m *PoolMover) configure() (bool, error) {
	for range configTries {
		changed := false
		err := m.record().Change(func(r *identity.Record, _ func() error) error {
			draft, err := config.Edit(m.ConfigPath, m.HomeDir)
			if err != nil {
				return err
			}
			if cfg := draft.Config(); cfg == nil || len(cfg.Pools) > 0 {
				return nil
			}
			if err := restorePools(draft, *r); err != nil {
				return err
			}
			changed = true
			return draft.Save()
		})
		if !errors.Is(err, config.ErrChanged) {
			return changed, err
		}
	}
	return false, fmt.Errorf("write the pools in the config: %w", config.ErrChanged)
}

// KeptPools are the pools the accounts record keeps of this Mac's config, as
// a config naming none is to have them once the move to pools is done: what
// HealPools writes back, and what a router routes within till it has, so the
// two never differ.
type KeptPools struct {
	// Pools are those the record keeps as this Mac's, as PoolEntry's Here
	// says, in its order, each as the config last wrote it, its id, label and
	// directories.
	Pools []config.NewPool
	// In is the id of the pool each account is in, by the id it was given
	// by: the one the record has it in, as recorded finds it, where that's
	// among Pools; else the unfenced one; else none, "", every pool fenced,
	// so it's in none, never to serve a pool's requests, never the first's
	// for want of another, as a client's sessions would go out on it.
	In map[string]string
}

// PoolsKept returns the pools record keeps of this Mac's config, as
// KeptPools says, with the pool each account with one of the ids given is
// in. It fails where the record notes no first pool, as before the move to
// pools, or keeps no pool as this Mac's.
func PoolsKept(record identity.Record, ids []string) (KeptPools, error) {
	if _, ok := record.FirstPool(); !ok {
		return KeptPools{}, errors.New("the accounts record notes no first pool")
	}
	pools := configuredPools(record)
	if len(pools) == 0 {
		return KeptPools{}, errors.New("the accounts record keeps no pool as this Mac's config's")
	}
	written := func(id string) (string, bool) {
		i := slices.IndexFunc(pools, func(p config.NewPool) bool { return strings.EqualFold(p.ID, id) })
		if i < 0 {
			return "", false
		}
		return pools[i].ID, true
	}
	var unfenced string
	if i := slices.IndexFunc(pools, func(p config.NewPool) bool { return len(p.Dirs) == 0 }); i >= 0 {
		unfenced = pools[i].ID
	}
	in := make(map[string]string, len(ids))
	for _, id := range ids {
		in[id] = unfenced
		if e, ok := recorded(record, id); ok {
			if pool, ok := written(e.Pool); ok {
				in[id] = pool
			}
		}
	}
	return KeptPools{Pools: pools, In: in}, nil
}

// restorePools writes in draft, a config without pools, the pools record
// keeps, as PoolsKept gives them, in its order, each as the config last wrote
// it; has each account in the pool PoolsKept has it in; and marks each
// pool's primary as the config last did, as restorePrimaries says. It fails,
// writing nothing, as PoolsKept does, where an account is in no pool, as
// none a pooled config can have, or where draft refuses an edit.
func restorePools(draft *config.Draft, record identity.Record) error {
	ids := draft.Config().Accounts.IDs()
	kept, err := PoolsKept(record, ids)
	if err != nil {
		return err
	}
	for _, p := range kept.Pools {
		if err := draft.AddPool(p); err != nil {
			return err
		}
	}
	for _, id := range ids {
		pool := kept.In[id]
		if pool == "" {
			return fmt.Errorf("account %s is in no pool the accounts record keeps, and every pool it writes back is fenced, so none can hold it", id)
		}
		if err := draft.SetPool(id, pool); err != nil {
			return err
		}
	}
	return restorePrimaries(draft, record)
}

// recorded is the account record has for the one a config names by the id
// given: by the id it had before switchboard made ids, as MovedFrom finds it,
// as in a config put back from before both moves, whose ids Heal writes back
// as the router restarts, as KeepUp tells those apart; else by its id.
func recorded(record identity.Record, id string) (identity.Entry, bool) {
	if e, moved := record.MovedFrom(id); moved {
		return e, true
	}
	return record.ByID(id)
}

// restorePrimaries marks in draft, its pools written back, each pool's
// primary as record has the config last make it: the account it was, where
// the config has that account in that pool still, and no other. A pool whose
// primary the record doesn't know, as PoolWritten's PrimaryKnown says, as one
// the move to pools made that the record hasn't been kept in step with
// since, or that the config last had no account in, so had no primary,
// keeps the marks draft has.
func restorePrimaries(draft *config.Draft, record identity.Record) error {
	cfg := draft.Config()
	for _, a := range cfg.Accounts {
		p, _ := record.PoolByID(a.Pool)
		if !p.Written.PrimaryKnown || p.Written.Primary == "" {
			continue
		}
		e, ok := recorded(record, a.ID)
		marked := ok && strings.EqualFold(p.Written.Primary, e.ID)
		var err error
		switch {
		case marked && !a.Primary:
			err = draft.SetPrimary(a.ID)
		case !marked && a.Primary:
			err = draft.UnmarkPrimary(a.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// configuredPools are the pools record keeps as this Mac's, as PoolEntry's
// Here says, in its order, as restorePools writes them.
func configuredPools(record identity.Record) []config.NewPool {
	var pools []config.NewPool
	for _, p := range record.Pools {
		if p.Here && p.Removed.IsZero() {
			pools = append(pools, config.NewPool{ID: p.ID, Label: p.Written.Label, Dirs: p.Written.Dirs})
		}
	}
	return pools
}

// HealPools writes back, in the config file at configPath, the pools the
// accounts record in the state directory stateDir keeps, their labels and
// directories, and each account in the pool the record last had it in, as
// restorePools writes them, where the move to pools is done there and the
// config names no pool, as where a change to the config made by hand, or a
// repository it's kept in, put one from before pools back. The config is
// read in the home directory homeDir returns. A router heals its config
// before it reads it, so it starts in pools; and as it follows it, so one
// put back is healed at once. It holds the record's lock as it does, so it's
// never called holding it. It reports whether it changed the config.
func HealPools(configPath, stateDir string, homeDir func() (string, error)) (bool, error) {
	if !PoolsMoved(stateDir) {
		return false, nil
	}
	m := &PoolMover{ConfigPath: configPath, StateDir: stateDir, HomeDir: homeDir}
	return m.configure()
}

// writeFirst writes the first pool the accounts record notes to a config
// without pools, every account in it, as configure writes it, as the move's
// step.
func (m *PoolMover) writeFirst() error {
	changed, err := m.configure()
	if changed {
		logger.Info("wrote the first pool in the config, every account in it", "path", m.ConfigPath)
	}
	return err
}

// takeUp has the router running beside the move take the pools its config
// now holds up, failing with ErrAwaitingPools where it doesn't route in them
// once it has.
func (m *PoolMover) takeUp() error {
	if m.TakeUp == nil || m.TakeUp() {
		return nil
	}
	return ErrAwaitingPools
}

// FirstPool is the ledger's option that has it know the first pool, as
// ledger.FirstPool says, as the accounts record in the state directory given
// notes it as the ledger asks, once the move to pools is done there: none
// before, where the record can't be read, which is logged once, or where it
// notes none.
func FirstPool(stateDir string) ledger.Option {
	var warned atomic.Bool
	return ledger.FirstPool(func() string {
		if !PoolsMoved(stateDir) {
			return ""
		}
		record, err := identity.NewFile(stateDir).Read()
		if err != nil {
			if !warned.Swap(true) {
				logger.Warn("can't read the accounts record, so a day's work in no pool can't be given the first pool's", "error", err)
			}
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
