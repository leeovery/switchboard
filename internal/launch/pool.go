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
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
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

// noToken is why a launch in the session's pool doesn't start where none of
// its accounts has a usable token: Claude Code's own login would go out on
// whatever account it's signed into, in another pool or none.
func (p place) noToken() string {
	return "pool " + poolSaid(p.pool) + " has no account with a usable token"
}

// elsewhere is why a pin to a, an account in another pool, is refused,
// saying which pool each is in, as "Acme 1 (k3f9) is in pool Acme; this
// directory is in pool Default". An account in none is one the router, whose
// pools these are, doesn't route yet.
func (p place) elsewhere(a config.Account) error {
	named := prose.AccountNamed(a.Label, a.ID)
	if a.Pool == "" {
		return fmt.Errorf("%s is in no pool the router routes yet; this directory is in pool %s", named, poolSaid(p.pool))
	}
	theirs, ok := p.pools.ByID(a.Pool)
	if !ok {
		theirs = config.Pool{ID: a.Pool}
	}
	return fmt.Errorf("%s is in pool %s; this directory is in pool %s", named, poolSaid(theirs), poolSaid(p.pool))
}

// withPools returns r as Run launches on it, ask asking the router how it
// is, once, where it's to be asked: as it is, but where its config names no
// pools once the move to pools is done. Then, as a session naming no pool
// would be routed in the first pool for its life, which may be fenced, it
// takes the pools the router routes for, where its answer reports them (see
// pooledBy); else those the accounts record keeps; else it refuses, failing
// with ErrNotStarted, Stderr hearing why. Before the move, a config naming no
// pools launches as before pools.
func (l Launcher) withPools(r Route, ask func() (router.Health, routerState)) (Route, error) {
	if !r.Moved || len(r.Config.Pools) > 0 {
		return r, nil
	}
	h, _ := ask()
	if pools, reported := RoutedPools(h); reported {
		r.Config, r.fromRouter = pooledBy(r.Config, h, pools), true
		return r, nil
	}
	if r.Kept != nil {
		if kept, ok := r.Kept(r.Config); ok && len(kept.Pools) > 0 {
			r.Config = kept
			return r, nil
		}
	}
	return r, l.refuse(l.unpooled(), "dir", l.Dir)
}

// unpooled is why nothing starts where the config names no pools, or can't
// be read, once the move to pools is done, and neither the router nor the
// accounts record says which there are: any directory may be in a fence.
func (l Launcher) unpooled() string {
	return "can't tell which pool " + l.where() + " is in: neither the config, the router nor the accounts record names any pools"
}

// Unpooled starts nothing, for a launch on Claude Code's own login where
// OwnLoginFences can't tell which pools there are, failing with
// ErrNotStarted, Stderr hearing why.
func (l Launcher) Unpooled() error {
	return l.refuse(l.unpooled(), "mode", "own login", "dir", l.Dir)
}

// OwnLoginFences returns the pools Claude Code's own login is kept out of
// the fences of where the config can't be read or names none: w's, the
// config file's, as Written's Fences gives them, routers returning the
// router's; and where those are none once the move to pools is done, moved,
// those the accounts record keeps, as kept returns them, as a router that
// has failed closed reports pools but lists none. Each is asked for only
// where it's needed. It reports false where after the move none of them
// names a pool, so which directories are fenced can't be told.
func OwnLoginFences(w Written, moved bool, routers, kept func() config.Pools) (config.Pools, bool) {
	if pools := w.Fences(routers); len(pools) > 0 || !moved {
		return pools, true
	}
	if pools := kept(); len(pools) > 0 {
		return pools, true
	}
	return nil, false
}

// keptFences returns the pools the accounts record keeps, as Kept gives them
// of a config naming no account: none where Kept is nil, or keeps none.
func (r Route) keptFences() config.Pools {
	if r.Kept == nil {
		return nil
	}
	if kept, ok := r.Kept(&config.Config{}); ok {
		return kept.Pools
	}
	return nil
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

// placed returns where a launch on r's config is among its pools: the pool
// sessionPool finds. In a config without pools, it's every account's, as
// before pools. It refuses, failing with ErrNotStarted, Stderr hearing why,
// where the session can be in no pool, as sessionPool says, or its pool has
// no accounts, or, of the router's accounts, none it routes yet.
func (l Launcher) placed(r Route) (place, error) {
	if len(r.Config.Pools) == 0 {
		return place{accounts: r.Config.Accounts}, nil
	}
	pool, why := l.sessionPool(r.Config.Pools)
	if why != "" {
		return place{}, l.refuse(why, "dir", l.Dir)
	}
	in := place{pools: r.Config.Pools, pool: pool, accounts: r.Config.Accounts.InPool(pool.ID)}
	if len(in.accounts) > 0 {
		return in, nil
	}
	if r.fromRouter {
		return place{}, l.refuse("the router routes no account of pool "+poolSaid(pool)+" yet: any the config gives it waits for the router to restart", "pool", pool.ID)
	}
	return place{}, l.refuse("pool "+poolSaid(pool)+" has no accounts", "pool", pool.ID)
}

// unknownDir is why no launch starts where the directory it starts in isn't
// known while a pool is fenced: it may be inside that pool's fence.
const unknownDir = "can't tell which pool the directory claude starts in is in"

// sessionPool returns the pool of pools a session started in Dir is in: the
// fenced pool holding Dir, as fencesHolding finds it, so a link into a fence
// or out of one leaves the session inside it; else the unfenced pool. Where
// it can be in none, it returns why instead: Dir isn't known while a pool is
// fenced; two fenced pools hold it, through its links; or none covers it, as
// where every pool is fenced.
func (l Launcher) sessionPool(pools config.Pools) (config.Pool, string) {
	if l.Dir == "" && slices.ContainsFunc(pools, config.Pool.Fenced) {
		return config.Pool{}, unknownDir
	}
	switch held := l.fencesHolding(pools); len(held) {
	case 0:
		if p, ok := pools.Unfenced(); ok {
			return p, ""
		}
		return config.Pool{}, "no pool covers " + l.where()
	case 1:
		return held[0], ""
	default:
		return config.Pool{}, l.where() + " is in " + poolsNamed(held) + ", as its links lead"
	}
}

// ownLoginBarred says why Claude Code doesn't start on its own login in Dir,
// of pools, placed as placedInHome places them, which would go out on
// whatever account it's signed into: Dir is in a fenced pool, as
// fencesHolding finds it, or isn't known while a pool is fenced; or a pool
// writes a directory that can't be placed, which may hold Dir. It says ""
// where it can start so. What a pool writes is redacted, as a token pasted
// into a pool's directories reads as one that can't be placed.
func (l Launcher) ownLoginBarred(pools config.Pools) string {
	if l.Dir == "" && slices.ContainsFunc(pools, config.Pool.Fenced) {
		return unknownDir
	}
	if held := l.fencesHolding(pools); len(held) > 0 {
		return l.where() + " is in " + poolsNamed(held) + ", where claude never starts on its own login"
	}
	if i := slices.IndexFunc(pools, func(p config.Pool) bool { return len(p.Unplaced) > 0 }); i >= 0 {
		return "can't tell which pool " + l.where() + " is in: pool " + poolSaid(pools[i]) + "'s directory " + prose.Quoted(redact.Text(pools[i].Unplaced[0])) + " can't be placed"
	}
	return ""
}

// placedInHome returns pools as config.Pool's PlacedIn leaves each one, its
// Unplaced directories placed in Home where they can be, as one starting ~/
// can be where Home is known.
func (l Launcher) placedInHome(pools config.Pools) config.Pools {
	placed := make(config.Pools, len(pools))
	for i, p := range pools {
		placed[i] = p.PlacedIn(l.Home)
	}
	return placed
}

// fencesHolding returns the fenced pools of pools that hold Dir, in their
// order, each id once: those with a directory that Dir, as given or with its
// links resolved, is within, the directory as written or with its own links
// resolved. No two pools' directories overlap as written, but they can once
// links are resolved, so more than one can.
func (l Launcher) fencesHolding(pools config.Pools) config.Pools {
	starts := l.alsoResolved(l.Dir)
	var held config.Pools
	for _, p := range pools {
		fence := config.Pool{Dirs: l.alsoResolved(p.Dirs...)}
		if p.Fenced() && slices.ContainsFunc(starts, fence.Covers) && !slices.ContainsFunc(held, sameID(p.ID)) {
			held = append(held, p)
		}
	}
	return held
}

// sameID matches a pool with the given id, ignoring case.
func sameID(id string) func(config.Pool) bool {
	return func(p config.Pool) bool { return strings.EqualFold(p.ID, id) }
}

// alsoResolved returns paths, each followed by itself with its links
// resolved, where that differs.
func (l Launcher) alsoResolved(paths ...string) []string {
	var both []string
	for _, path := range paths {
		both = append(both, path)
		if real := l.resolved(path); real != path {
			both = append(both, real)
		}
	}
	return both
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

// where names Dir as Stderr hears it: as shownDir shows it, such as
// ~/Code/acme, or as the directory claude starts in where it isn't known.
func (l Launcher) where() string {
	return cmp.Or(l.shownDir(), "the directory claude starts in")
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
// how they differ. Its asked counts the waiting. A router that has failed
// closed, as it answers at first or while it's waited for, is as it then is:
// it refuses every request, saying why, so a session routed through it goes
// nowhere outside its pool, where one counted down would go direct past it,
// and it routes the session once it takes its pools up.
func (r Route) takingUp(ctx context.Context, h router.Health, state routerState, in place) routerState {
	if !in.compared() || r.failedClosed(h) {
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
		if why = poolsDiffer(answer, r.Config, in.id()); why == "" || r.failedClosed(answer) {
			next.asked = state.asked + since(began)
			return next
		}
	}
	return routerState{name: otherPools, reason: why, asked: state.asked + since(began)}
}

// failedClosed reports whether the router, as its health answer h has it,
// has failed closed once the move to pools is done, as neither its config
// nor the accounts record names its pools: it answers unhealthy, reporting
// pools but listing none. One answering healthy so routes by a config from
// before pools, every account in one pool, as one does before the move.
func (r Route) failedClosed(h router.Health) bool {
	return r.Moved && !h.OK && h.ReportsPools && len(h.Pools) == 0
}

// passesHeaders is why a router that reports its pools can't be told a
// session's pool: without its header, every session would be in its first.
const passesHeaders = "it would pass switchboard's own headers on to the API"

// poolsDiffer says how the pools the router routes for, as its health answer
// h reports them, aren't cfg's, as a launch in the pool with the given id
// needs them: "" where they are. The router takes switchboard's own headers
// off what it sends upstream, so it can be told the pool; they're the same
// pools, by id, each with the same directories, as fencesDiffer compares
// them; every account the router routes in a pool is in that pool in cfg,
// while an account cfg has added, which waits for the router's restart, can
// be missing; each pool's primary is cfg's, where the router has that
// account; and the router routes one of the launch's pool's accounts at
// least.
func poolsDiffer(h router.Health, cfg *config.Config, pool string) string {
	switch {
	case !h.ReportsPools:
		return "it's from before pools"
	case !h.StripsOwnHeaders:
		return passesHeaders
	}
	routed := routing(h.Pools)
	if why := routed.fencesDiffer(cfg.Pools); why != "" {
		return why
	}
	for _, p := range cfg.Pools {
		if why := routed.accountsDiffer(p, cfg.Accounts); why != "" {
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

// fencesDiffer says how the router's pools aren't pools: "" where they're
// the same pools, by id, each with the same directories, links unresolved.
// Pools that repeat an id, ignoring case, as pools read unchecked can, are
// never the router's: two of them could match one of its pools, and leave
// another unmatched.
func (r routing) fencesDiffer(pools config.Pools) string {
	for i, p := range pools {
		if slices.ContainsFunc(pools[:i], sameID(p.ID)) {
			return "the config's pools repeat the id " + prose.Quoted(p.ID)
		}
	}
	if len(r) != len(pools) {
		return "its pools aren't the config's"
	}
	for _, p := range pools {
		routed, ok := r.of(p.ID)
		switch {
		case !ok:
			return "its pools aren't the config's"
		case !sameDirs(p.Dirs, routed.Dirs):
			return "pool " + poolSaid(p) + "'s directories aren't the config's"
		}
	}
	return ""
}

// accountsDiffer says how the accounts of the router's pool p, of accounts,
// aren't the config's, as poolsDiffer compares them: "" where they are.
func (r routing) accountsDiffer(p config.Pool, accounts config.Accounts) string {
	routed, _ := r.of(p.ID)
	switch {
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

// Written is the pools the config file writes, as read where the config
// can't be: Pools, read by every rule the config holds them to; or, where
// they break one, Unchecked, each pool's id, label and directories, those
// that can't be placed Unplaced, which keep Claude Code's own login out of
// their fences, and out of every directory while one can't be placed, but
// are never routed by.
type Written struct {
	Pools     config.Pools
	Unchecked bool
}

// Fences returns the pools Claude Code's own login is kept out of the fences
// of where the config can't be read, routers returning the router's, as its
// health answer reports them, asked only where they're needed: w's, where
// the file writes any by the config's rules, as the user would have them;
// w's and the router's, where they break those rules, which only ever keeps
// it out of more; else the router's.
func (w Written) Fences(routers func() config.Pools) config.Pools {
	switch {
	case len(w.Pools) > 0 && !w.Unchecked:
		return w.Pools
	case len(w.Pools) > 0:
		return append(slices.Clone(w.Pools), routers()...)
	}
	return routers()
}

// unroutedBy says why the router, as its health answer h reports the pools
// it routes for, isn't routed through where the config can't be read, its
// file writing w: "" where it is. Where w has a directory that can't be
// placed, the fences the user would have aren't known. One reporting pools,
// of several, would pass the pool on to the API; or its pools aren't w's, by
// id and directories, where w has any, as fencesDiffer compares them, as the
// router would route by fences the user has since changed. One reporting
// none routes every account in one pool, which isn't w's where w has
// several, or any that break the config's rules; nor, once the move to pools
// is done, moved, where w has none, as the pools there are, which the
// accounts record keeps, may be fenced.
func (w Written) unroutedBy(h router.Health, moved bool) string {
	if slices.ContainsFunc(w.Pools, func(p config.Pool) bool { return len(p.Unplaced) > 0 }) {
		return "the config file writes a directory that can't be placed"
	}
	if _, reported := RoutedPools(h); reported {
		switch {
		case len(h.Pools) > 1 && !h.StripsOwnHeaders:
			return passesHeaders
		case len(w.Pools) > 0:
			return routing(h.Pools).fencesDiffer(w.Pools)
		}
		return ""
	}
	switch {
	case len(w.Pools) > 1:
		return "it reports no pools, and the config file writes several"
	case w.Unchecked && len(w.Pools) > 0:
		return "it reports no pools, and the config file's break its rules"
	case moved && len(w.Pools) == 0:
		return "it reports no pools, and the config file writes none once the move to pools is done"
	}
	return ""
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

// poolSaid names the pool p by its label, else its id, redacted, as the
// label of a pool read unchecked can hold anything.
func poolSaid(p config.Pool) string {
	return prose.Clean(redact.Text(cmp.Or(p.Label, p.ID)))
}

// poolsNamed names pools, as "pool Acme", or "pools Acme and Client".
func poolsNamed(pools config.Pools) string {
	said := make([]string, len(pools))
	for i, p := range pools {
		said[i] = poolSaid(p)
	}
	if len(said) == 1 {
		return "pool " + said[0]
	}
	return "pools " + prose.List(said)
}
