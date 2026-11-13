package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// readingAPool says, in the help of the verbs that take --pool, which pool
// they read or act on without it.
const readingAPool = `--pool names the pool by its id, as switchboard pool lists them. Without it,
it's the only pool, where there's one, else the one fenced to no directory;
where every pool is fenced, it asks for --pool. The directory it's run in
never chooses.`

// readingAKeptPool says, in the help of the verbs that read the recorded
// files, which read a pool the config no longer has by its id too.
const readingAKeptPool = `--pool also takes a pool the config no longer has, removed, or another
Mac's from an import, by the id its history is kept under, naming it as
removed, or as not configured here, and reads its work as it was recorded,
never in another pool.`

// addPoolFlag registers --pool on cmd, naming the pool it reads or acts on,
// as what says it does, by its id.
func addPoolFlag(cmd *cobra.Command, pool *string, what string) {
	cmd.Flags().StringVar(pool, "pool", "", what+" pool `ID`, else the only one, else the unfenced one")
}

// askedPool returns the pool of pools a verb reads or acts on, named or not,
// as config.Pools' Asked has it, the rule the router answers by too; zero,
// whatever is named, in a config with no pools. It fails with
// router.PoolNeeded where Asked gives none.
func askedPool(pools config.Pools, named string) (config.Pool, error) {
	if len(pools) == 0 {
		return config.Pool{}, nil
	}
	p, ok := pools.Asked(named)
	if !ok {
		return config.Pool{}, router.NeedPool(named, pools)
	}
	return p, nil
}

// pooledConfig returns cfg as its pools stand, for a verb reading a pool, as
// launch.PoolsOf has them, as a launch does: of a config naming none once
// the move to pools is done, the router's, else the accounts record's. It
// fails, saying why, where none of them names a pool, so no verb reads every
// pool's work together for want of knowing them.
func (a *app) pooledConfig(ctx context.Context, cfg *config.Config) (*config.Config, error) {
	pooled, ok := launch.PoolsOf(cfg, a.poolsMoved(), func() router.Health { return a.routersHealth(ctx) }, a.keptPools)
	if !ok {
		return nil, errors.New("can't tell which pool to read: " + launch.NoPoolsNamed)
	}
	return pooled, nil
}

// poolsStanding are the pools of the config as it loads now, as standing
// has them, for a verb that counts or labels them: none where the config
// can't be loaded.
func (a *app) poolsStanding(ctx context.Context) config.Pools {
	cfg, err := a.loadConfig()
	if err != nil {
		logger.Debug("can't load the config, so no pool is counted", "error", err)
		return nil
	}
	return a.standing(ctx, cfg).Pools
}

// standing returns cfg as its pools stand, as pooledConfig has it, for a
// verb that lists, counts or labels them: cfg as it is, naming none, where
// they can't be told, so it lists none rather than refusing.
func (a *app) standing(ctx context.Context, cfg *config.Config) *config.Config {
	pooled, err := a.pooledConfig(ctx, cfg)
	if err != nil {
		logger.Debug("can't tell the pools, so none is counted", "error", err)
		return cfg
	}
	return pooled
}

// filedPool is the pool a verb reads the recorded files of, as each line
// recorded its pool at the time, and how the views read it: zero, every
// pool's, in a config with no pools.
type filedPool struct {
	config.Pool
	read views.PoolRead
	// named is how a verb's text names it, as poolNamed does: "" where
	// there's one pool or none, and where the read is of every pool's; as
	// status.KeptPool's Said says, where it's kept.
	named string
	// kept is set where it's a pool the accounts record keeps that isn't
	// configured, as keptPool reads one: the router routes nothing in it.
	kept bool
}

// heading names the pool in a paragraph of a line, as a verb's text starts
// where it names it: "" where named is.
func (p filedPool) heading() string {
	return poolHeading(p.named)
}

// poolHeading is the paragraph of a line a verb's text starts with naming a
// pool, named as poolNamed names it: "" where named is.
func poolHeading(named string) string {
	if named == "" {
		return ""
	}
	return named + "\n\n"
}

// filedPool returns the pool a verb reading the files in the state directory
// dir reads, named or not, as readPool has it of cfg as its pools stand, as
// pooledConfig gives it, and cfg so: failing as either does.
func (a *app) filedPool(ctx context.Context, cfg *config.Config, dir, named string) (filedPool, *config.Config, error) {
	pooled, err := a.pooledConfig(ctx, cfg)
	if err != nil {
		return filedPool{}, nil, err
	}
	pool, err := a.readPool(pooled, dir, named)
	return pool, pooled, err
}

// readPool returns the pool of cfg's pools a verb reading the files in the
// state directory dir reads, as askedPool has it, named or not, else the one
// named that the accounts record keeps, as keptPool has it; failing as
// askedPool does, saying --pool names one.
func (a *app) readPool(cfg *config.Config, dir, named string) (filedPool, error) {
	record := recordIn(dir)
	p, err := askedPool(cfg.Pools, named)
	switch {
	case err != nil && named != "":
		return a.keptPool(cfg, dir, record, named, err)
	case err != nil || p.ID == "":
		return filedPool{}, poolFlagNeeded(err)
	}
	return filedPool{Pool: p, read: a.poolRead(cfg, dir, record, p.ID), named: poolNamed(cfg.Pools, p.Label)}, nil
}

// keptPool returns the pool with the id named that the accounts record
// record returns keeps but cfg doesn't configure, as status.KeptPoolOf has
// it, removed, or another Mac's from an import: its work read as it was
// recorded, under its id alone, and named as KeptPool's Said says, whatever
// pools there are. It fails with err, saying --pool names a pool, where the
// record keeps none with that id.
func (a *app) keptPool(cfg *config.Config, dir string, record func() identity.Record, named string, err error) (filedPool, error) {
	kept, ok := status.KeptPoolOf(record(), cfg.Pools, named)
	if !ok {
		return filedPool{}, poolFlagNeeded(err)
	}
	pool := config.Pool{ID: kept.ID, Label: kept.Label}
	return filedPool{Pool: pool, read: a.poolRead(cfg, dir, record, kept.ID), named: kept.Said(), kept: true}, nil
}

// poolRead is how the views read the work of the pool with the given id
// from the files in the state directory dir: work in no pool the first
// pool's, as firstPool has it of cfg's pools, and each account in the pool
// cfg configures it in, else the one the accounts record record returns
// keeps for it.
func (a *app) poolRead(cfg *config.Config, dir string, record func() identity.Record, id string) views.PoolRead {
	return views.PoolRead{ID: id, First: a.firstPool(dir, cfg.Pools), PoolOf: status.AccountPools(cfg.Accounts, record)}
}

// poolNamed names the pool with the given label, as prose.Pool does, where
// pools are more than one, so a verb's text says which it tells of: "" where
// there's one or none, as no screen names a pool till there's a second.
func poolNamed(pools config.Pools, label string) string {
	if len(pools) < 2 {
		return ""
	}
	return prose.Pool(label)
}

// inPoolNamed names the pool with the given label after what a verb says of
// it, as " in pool Acme", where pools are more than one, as poolNamed names
// it: "" where there's one or none.
func inPoolNamed(pools config.Pools, label string) string {
	if named := poolNamed(pools, label); named != "" {
		return " in " + named
	}
	return ""
}

// poolFlagNeeded is err, where it's a router.PoolNeeded, saying --pool is
// how a pool is named; else err as it is.
func poolFlagNeeded(err error) error {
	if _, needs := errors.AsType[router.PoolNeeded](err); needs {
		return fmt.Errorf("%w, with --pool", err)
	}
	return err
}
