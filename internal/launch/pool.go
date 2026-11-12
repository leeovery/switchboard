package launch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// place is where a launch is among the pools configured: the pool the
// directory Claude Code starts in is in, which the session keeps for its
// whole life, and the accounts it can start on.
type place struct {
	// pools are every pool configured: none in a config from before pools.
	pools config.Pools
	// pool is the session's: zero without pools.
	pool config.Pool
	// accounts are the pool's, in the config's order, or every account
	// without pools.
	accounts config.Accounts
}

// pooled reports whether the config has pools, so the session is in one.
func (p place) pooled() bool {
	return len(p.pools) > 0
}

// asBefore reports whether a launch goes as it did before pools: the config
// has none, or one alone, unfenced, which every directory is in, so no
// session can go out on another pool's account, or inside a fence.
func (p place) asBefore() bool {
	return len(p.pools) == 0 || len(p.pools) == 1 && !p.pools[0].Fenced()
}

// compared reports whether the router's pools are compared with the
// config's: with one, no session can be spread across pools.
func (p place) compared() bool {
	return len(p.pools) > 1
}

// id is the id of the session's pool: "" without pools.
func (p place) id() string {
	return p.pool.ID
}

// holds reports whether a is one of the accounts the session can start on:
// one of its pool's, or any without pools.
func (p place) holds(a config.Account) bool {
	return !p.pooled() || strings.EqualFold(a.Pool, p.pool.ID)
}

// elsewhere is why a pin to a, an account in another pool, is refused,
// saying which pool each is in, as "Acme 1 (k3f9) is in pool Acme; this
// directory is in pool Default".
func (p place) elsewhere(a config.Account) error {
	theirs, ok := p.pools.ByID(a.Pool)
	if !ok {
		theirs = config.Pool{ID: a.Pool}
	}
	return fmt.Errorf("%s is in pool %s; this directory is in pool %s", accountSaid(a), poolSaid(theirs), poolSaid(p.pool))
}

// withPools returns the config Run launches on, the router's health answer
// being h: r's, but where it names no pools once the move to pools is done.
// Then, as a session naming no pool would be routed in the first pool for
// its life, which may be fenced, it takes the pools the router routes for,
// where h reports them (see pooledBy); else those the accounts record keeps;
// else it refuses, failing with ErrNotStarted, Stderr hearing why. Before
// the move, a config naming no pools launches as before pools.
func (l Launcher) withPools(r Route, h router.Health) (*config.Config, error) {
	if !r.Moved || len(r.Config.Pools) > 0 {
		return r.Config, nil
	}
	if pools, reported := RoutedPools(h); reported {
		return pooledBy(r.Config, h, pools), nil
	}
	if r.Kept != nil {
		if kept, ok := r.Kept(r.Config); ok && len(kept.Pools) > 0 {
			return kept, nil
		}
	}
	return nil, l.refuse("the config names no pools, and switchboard can't tell which pool "+l.where()+" is in", "dir", l.Dir)
}

// pooledBy returns cfg, a config naming no pools, with pools, those the
// router routes for, as its health answer h reports them, each account in
// the pool h lists it in, the primary of each as h names it: one h doesn't
// list, as one added since the router started, is in none.
func pooledBy(cfg *config.Config, h router.Health, pools config.Pools) *config.Config {
	pooled := *cfg
	pooled.Pools = pools
	pooled.Accounts = make(config.Accounts, len(cfg.Accounts))
	for i, a := range cfg.Accounts {
		a.Pool, a.Primary = "", false
		for _, p := range h.Pools {
			if slices.ContainsFunc(p.Accounts, func(id string) bool { return strings.EqualFold(id, a.ID) }) {
				a.Pool, a.Primary = p.ID, strings.EqualFold(p.Primary, a.ID)
			}
		}
		pooled.Accounts[i] = a
	}
	return &pooled
}

// placed returns where a launch on cfg is among its pools: the pool whose
// directories hold Dir, links resolved, else the unfenced pool. In a config
// without pools, it's every account's, as before pools. It refuses, failing
// with ErrNotStarted, Stderr hearing why, where no pool covers Dir, as where
// every pool is fenced, or the pool has no accounts.
func (l Launcher) placed(cfg *config.Config) (place, error) {
	if len(cfg.Pools) == 0 {
		return place{accounts: cfg.Accounts}, nil
	}
	pool, ok := l.covering(cfg.Pools)
	if !ok {
		return place{}, l.refuse("no pool covers "+l.where(), "dir", l.Dir)
	}
	in := place{pools: cfg.Pools, pool: pool, accounts: cfg.Accounts.InPool(pool.ID)}
	if len(in.accounts) == 0 {
		return place{}, l.refuse("pool "+poolSaid(pool)+" has no accounts", "pool", pool.ID)
	}
	return in, nil
}

// covering returns the pool of pools a session started in Dir is in, as
// config.Pools' Covering finds it, the links in Dir and in each pool's
// directories resolved first, so a directory reached through a link is in
// the pool of where it leads. It reports false where no pool covers Dir.
func (l Launcher) covering(pools config.Pools) (config.Pool, bool) {
	resolved := make(config.Pools, len(pools))
	for i, p := range pools {
		resolved[i] = config.Pool{ID: p.ID, Dirs: make([]string, len(p.Dirs))}
		for j, dir := range p.Dirs {
			resolved[i].Dirs[j] = l.resolved(dir)
		}
	}
	found, ok := resolved.Covering(l.resolved(l.Dir))
	if !ok {
		return config.Pool{}, false
	}
	return pools.ByID(found.ID)
}

// resolved returns path with its links resolved, or as it is where they
// can't be, as where nothing's there.
func (l Launcher) resolved(path string) string {
	if path == "" {
		return ""
	}
	if real, err := l.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// fencedIn returns the fenced pool of pools whose directories hold Dir,
// links resolved, reporting false where none does.
func (l Launcher) fencedIn(pools config.Pools) (config.Pool, bool) {
	p, ok := l.covering(pools)
	return p, ok && p.Fenced()
}

// where names Dir as Stderr hears it: as shownDir shows it, such as
// ~/Code/acme, or as the directory claude starts in where it isn't known.
func (l Launcher) where() string {
	return cmp.Or(l.shownDir(), "the directory claude starts in")
}

// inFence is why a launch on Claude Code's own login doesn't start in Dir,
// inside the fenced pool p's directories: it would go out on whatever
// account Claude Code is signed into.
func (l Launcher) inFence(p config.Pool) string {
	return l.where() + " is in pool " + poolSaid(p) + ", where claude never starts on its own login"
}

// refuse starts nothing, for why, as "no pool covers ~/Code/other", which
// Stderr hears, followed by "— not starting claude", and the log, with
// attrs. It fails with ErrNotStarted.
func (l Launcher) refuse(why string, attrs ...any) error {
	logger.Warn("not starting claude", append([]any{"reason", why}, attrs...)...)
	Notice(l.Stderr, why+" — not starting claude")
	return ErrNotStarted
}

// takingUp returns how the router is for a launch in place, its health
// answer h having found it as state: as it is where the pools the router
// routes for are the config's, as poolsDiffer compares them, or there's one
// pool alone, with which no session can be spread across pools; else as it
// answers once it has taken the config's up, asked again Patience's Again
// apart for as long as its TakeUp; else down, routing by other pools, saying
// how they differ. Its asked counts the waiting.
func (r Route) takingUp(ctx context.Context, h router.Health, state routerState, in place) routerState {
	if !in.compared() {
		return state
	}
	why := poolsDiffer(h, r.Config, in.id())
	if why == "" {
		return state
	}
	p := r.Patience.orDefault()
	began := time.Now()
	waiting, cancel := context.WithTimeout(ctx, p.TakeUp)
	defer cancel()
	for pause(waiting, p.Again) == nil {
		answer, err := r.Router.Health(waiting)
		next := stateOf(answer, err)
		if !next.routable() {
			continue
		}
		if why = poolsDiffer(answer, r.Config, in.id()); why == "" {
			next.asked = state.asked + since(began)
			return next
		}
	}
	return routerState{name: otherPools, reason: why, asked: state.asked + since(began)}
}

// poolsDiffer says how the pools the router routes for, as its health answer
// h reports them, aren't cfg's, as a launch in the pool with the given id
// needs them: "" where they are. They're the same pools, by id, each with the
// same directories as cfg finds them, links unresolved; every account the
// router routes in a pool is in that pool in cfg, while an account cfg has
// added, which waits for the router's restart, can be missing; each pool's
// primary is cfg's, where the router has that account; and the router routes
// one of the launch's pool's accounts at least.
func poolsDiffer(h router.Health, cfg *config.Config, pool string) string {
	if !h.ReportsPools {
		return "it's from before pools"
	}
	routed := routing(h.Pools)
	if len(routed) != len(cfg.Pools) {
		return "its pools aren't the config's"
	}
	for _, p := range cfg.Pools {
		if why := routed.differs(p, cfg.Accounts); why != "" {
			return why
		}
	}
	if launched, _ := routed.of(pool); len(launched.Accounts) == 0 {
		ours, _ := cfg.Pools.ByID(pool)
		return "it routes none of pool " + poolSaid(ours) + "'s accounts till it restarts"
	}
	return ""
}

// routing is the pools a router routes for, as its health answer reports
// them.
type routing []router.RoutedPool

// of returns the pool with the given id, ids compared ignoring case,
// reporting false where there's none.
func (r routing) of(id string) (router.RoutedPool, bool) {
	i := slices.IndexFunc(r, func(p router.RoutedPool) bool { return strings.EqualFold(p.ID, id) })
	if i < 0 {
		return router.RoutedPool{}, false
	}
	return r[i], true
}

// has reports whether the router has the account with the given id, in any
// pool.
func (r routing) has(id string) bool {
	return slices.ContainsFunc(r, func(p router.RoutedPool) bool {
		return slices.ContainsFunc(p.Accounts, func(a string) bool { return strings.EqualFold(a, id) })
	})
}

// differs says how the router's pool p, of accounts, isn't the config's, as
// poolsDiffer compares them: "" where it is.
func (r routing) differs(p config.Pool, accounts config.Accounts) string {
	routed, ok := r.of(p.ID)
	switch {
	case !ok:
		return "its pools aren't the config's"
	case !sameDirs(p.Dirs, routed.Dirs):
		return "pool " + poolSaid(p) + "'s directories aren't the config's"
	case !allIn(routed.Accounts, p.ID, accounts):
		return "pool " + poolSaid(p) + "'s accounts aren't the config's"
	case !r.samePrimary(routed, p.ID, accounts):
		return "pool " + poolSaid(p) + "'s primary isn't the config's"
	}
	return ""
}

// sameDirs reports whether a and b are the same directories, in any order.
func sameDirs(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// allIn reports whether each account whose id is among ids is in the pool
// with the given id, of accounts.
func allIn(ids []string, pool string, accounts config.Accounts) bool {
	return !slices.ContainsFunc(ids, func(id string) bool {
		a, ok := accounts.ByID(id)
		return !ok || !strings.EqualFold(a.Pool, pool)
	})
}

// samePrimary reports whether the router's pool routed has as its primary
// the one accounts give the pool with the given id, where the router has
// that account: one the config has added, waiting for the router's restart,
// can't be its primary yet.
func (r routing) samePrimary(routed router.RoutedPool, pool string, accounts config.Accounts) bool {
	primary, ok := accounts.PrimaryOf(pool)
	return !ok || !r.has(primary.ID) || strings.EqualFold(routed.Primary, primary.ID)
}

// RoutedPools returns the pools a router routes for, as its health answer h
// reports them, as a config has them: their ids, labels and directories. It
// reports false where h reports none, as from a router from before pools.
func RoutedPools(h router.Health) (config.Pools, bool) {
	if !h.ReportsPools || len(h.Pools) == 0 {
		return nil, false
	}
	pools := make(config.Pools, len(h.Pools))
	for i, p := range h.Pools {
		pools[i] = config.Pool{ID: p.ID, Label: cmp.Or(p.Label, p.ID), Dirs: p.Dirs, WrittenDirs: p.WrittenDirs}
	}
	return pools, true
}

// routedConfig is a config of pools, the pools the router's health answer h
// reports, and of their accounts, as it reports them, each pool's in its
// order, its primary marked, each labelled by its id, as the answer gives no
// label. It fails where h names no account.
func routedConfig(h router.Health, pools config.Pools) (*config.Config, error) {
	var accounts config.Accounts
	for _, p := range h.Pools {
		for _, id := range p.Accounts {
			accounts = append(accounts, config.Account{ID: id, Label: id, Pool: p.ID, Primary: id == p.Primary})
		}
	}
	if len(accounts) == 0 {
		return nil, errors.New("the router's health answer names no account in any pool")
	}
	return &config.Config{Accounts: accounts, Pools: pools}, nil
}

// accountSaid names the account a by its label, and its id beside where that
// differs, as "Acme 1 (k3f9)": one labelled by its id is named once.
func accountSaid(a config.Account) string {
	label, id := status.ConfiguredName(a).Said(), prose.Clean(a.ID)
	if strings.EqualFold(label, id) {
		return id
	}
	return label + " (" + id + ")"
}

// poolSaid names the pool p by its label, else its id.
func poolSaid(p config.Pool) string {
	return prose.Clean(cmp.Or(p.Label, p.ID))
}
