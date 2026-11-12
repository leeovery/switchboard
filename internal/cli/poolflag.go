package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// readingAPool says, in the help of the verbs that take --pool, which pool
// they read or act on without it.
const readingAPool = `--pool names the pool by its id, as switchboard pool lists them. Without it,
it's the only pool, where there's one, else the one fenced to no directory;
where every pool is fenced, it asks for --pool. The directory it's run in
never chooses.`

// addPoolFlag registers --pool on cmd, naming the pool it reads or acts on,
// as what says it does, by its id.
func addPoolFlag(cmd *cobra.Command, pool *string, what string) {
	cmd.Flags().StringVar(pool, "pool", "", what+" pool `ID`, else the only one, else the unfenced one")
}

// askedPool returns the pool of pools a verb reads or acts on: the one with
// the id named, else the one the verbs act on, as config.Pools' Implicit has
// it, the rule the router answers by too; zero, whatever is named, in a
// config with no pools. It fails with router.PoolNeeded where the pool named
// isn't configured, or none is named and there's none the verbs act on.
func askedPool(pools config.Pools, named string) (config.Pool, error) {
	if len(pools) == 0 {
		return config.Pool{}, nil
	}
	p, ok := pools.Implicit()
	if named != "" {
		p, ok = pools.ByID(named)
	}
	if !ok {
		return config.Pool{}, router.NeedPool(named, pools)
	}
	return p, nil
}

// filedPool is the pool a verb reads the recorded files of, as each line
// recorded its pool at the time, and how the views read it: zero, every
// pool's, in a config with no pools.
type filedPool struct {
	config.Pool
	read views.PoolRead
}

// readPool returns the pool of cfg's pools a verb reading the files in the
// state directory dir reads, as askedPool has it, named or not, failing as it
// does, saying --pool names one.
func readPool(cfg *config.Config, dir, named string) (filedPool, error) {
	p, err := askedPool(cfg.Pools, named)
	if err != nil || p.ID == "" {
		return filedPool{}, poolFlagNeeded(err)
	}
	inPool := func(pool string) func(account string) bool {
		return func(account string) bool {
			a, ok := cfg.Accounts.ByID(account)
			return ok && (pool == "" || strings.EqualFold(a.Pool, pool))
		}
	}
	return filedPool{Pool: p, read: views.PoolRead{ID: p.ID, First: idmove.FirstPoolID(dir), In: inPool(p.ID), Pooled: inPool("")}}, nil
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
