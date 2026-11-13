package cli

import (
	"context"
	"errors"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// shownPools is how the dashboard reads the pool it shows: probing alone
// where probe says, and, where read says there's a state directory, dir,
// the request ledger in it and the router's events through events; stamp is
// how the config file stood before the config it opens on was read, as
// configStamp has it. It holds no ledger: the dashboard's is the model's
// alone, so none it no longer reads is kept.
type shownPools struct {
	a      *app
	probe  bool
	dir    string
	events views.Events
	read   bool
	stamp  filestate.State
}

// opened is what the dashboard opens on of the config cfg: the pool named,
// else the one the verbs act on, of the pools as they stand, as
// pooledConfig has them, as shown has it. It fails where a pool is named
// and the pools can't be told, as pooledConfig fails; where none is named,
// it opens on none, warning, its source reading the pool the verbs act on,
// as before pools, its accounts the config's, which names none, and none of
// the router's events, nor any pool's work of the request ledger, so the
// dashboard never shows every pool's together.
func (s shownPools) opened(ctx context.Context, cfg *config.Config, named string) (watch.Pooled, error) {
	pooled, err := s.a.pooledConfig(ctx, cfg)
	switch {
	case err != nil && named != "":
		return watch.Pooled{}, err
	case err != nil:
		logger.Warn("can't tell which pool the dashboard shows, so it reads none of the router's files of events, nor the request ledger", "error", err)
		return watch.Pooled{
			Accounts: status.PoolAccounts(cfg.Pools, cfg.Accounts, ""), Source: s.a.source(ctx, cfg, s.probe), Events: events.Empty{}, Read: views.NoPool(), Stamp: s.stamp,
			Placed: status.Placed(cfg.Pools, cfg.Accounts), Grounds: s.groundsOf(cfg, cfg.Pools),
		}, nil
	}
	shown, err := s.shown(ctx, cfg, pooled, named)
	shown.Stamp, shown.Grounds = s.stamp, s.groundsOf(cfg, pooled.Pools)
	return shown, err
}

// turn is what the dashboard shows of the pool asked for, as poolAsked has
// it, from the one with the id from: the config read afresh, its pools as
// they stand, so a pool added, removed or changed while it's open shows at
// the next P, or as the config changes; stamped as the config file stood
// before it was read, as configStamp has it, so a change as it's read is
// read again; the grounds of its ledger, as groundsOf has them, with the
// ledger made on them, as ledgerOn makes it, for the watch to make only
// where the one in use was made on others; and the accounts removed, as
// removedIn gives them where there's a state directory. It fails where the
// config can't be read, or the pools told.
func (s shownPools) turn(ctx context.Context, from string, ask watch.PoolAsk) (watch.Pooled, error) {
	stamp := s.a.configStamp()
	cfg, err := s.a.loadConfig()
	if err != nil {
		return watch.Pooled{}, err
	}
	pooled, err := s.a.pooledConfig(ctx, cfg)
	if err != nil {
		return watch.Pooled{}, err
	}
	named, err := poolAsked(pooled.Pools, from, ask)
	if err != nil {
		return watch.Pooled{}, err
	}
	shown, err := s.shown(ctx, cfg, pooled, named)
	if err != nil {
		return watch.Pooled{}, err
	}
	shown.Stamp, shown.Grounds = stamp, s.groundsOf(cfg, pooled.Pools)
	shown.NewLedger = s.ledgerOn(shown.Grounds.Caps, pooled.Pools)
	if s.read {
		shown.Removed = removedIn(s.dir, cfg)
	}
	return shown, nil
}

// groundsOf are the grounds the dashboard's request ledger is made on, of
// the config cfg, its pools as pools gives them: the caps of the accounts
// it configures, and the first pool, as firstPool has it, where there's a
// state directory, as the one it opens with is made. The first pool is for
// comparing alone: the ledger reads it afresh as it asks, as ledgerOn has it.
func (s shownPools) groundsOf(cfg *config.Config, pools config.Pools) ledger.Grounds {
	grounds := ledger.Grounds{Caps: caps(cfg)}
	if s.read {
		grounds.First = s.a.firstPool(s.dir, pools)
	}
	return grounds
}

// ledgerOn makes the request ledger as the dashboard reads it, its days
// summarised with caps: following the one in the state directory, knowing
// the first pool of pools as idmove.FirstPool has it, the accounts record
// read at each ask, as the one it opens with does, so a dashboard open
// across the move to pools follows it; else an empty one. It makes none
// till it's called.
func (s shownPools) ledgerOn(caps ledger.Caps, pools config.Pools) func() views.Ledger {
	if !s.read {
		return func() views.Ledger { return ledger.NewEmpty(s.a.Now, caps) }
	}
	return func() views.Ledger {
		return s.a.followerIn(s.dir, caps, idmove.FirstPool(s.dir, func() config.Pools { return pools }))
	}
}

// poolAsked is the id of the pool the dashboard asks for, as ask says, of
// pools, from the one with the id from: the one P turns to, as
// status.PoolAfter has it; or that one again, as status.PoolAgain has it,
// but of none, as where every pool is fenced and none is named, none again,
// for the one the verbs act on to show. It fails where there are no pools to
// turn to.
func poolAsked(pools config.Pools, from string, ask watch.PoolAsk) (string, error) {
	if ask == watch.SamePool && from == "" {
		return "", nil
	}
	pick := status.PoolAfter
	if ask == watch.SamePool {
		pick = status.PoolAgain
	}
	p, ok := pick(pools, from)
	if !ok {
		return "", errors.New("the config names no pools")
	}
	return p.ID, nil
}

// configStamp is how the config file the dashboard reads stands, the one
// --config names, else the one config.Path finds, as the dashboard compares
// it at each read, to read the pool it shows again only where the config
// has changed, as the router compares the config it was started from: none
// where it can't be found.
func (a *app) configStamp() filestate.State {
	path, err := a.configFile()
	if err != nil {
		return filestate.State{}
	}
	return filestate.Of(path)
}

// shown is what the dashboard shows of the pool named of the config cfg,
// its pools as pooled has them, else of the one the verbs act on, as
// askedPool has it: the pool, as status.PoolShown has it; its accounts, as
// status.PoolAccounts has them; its source, reading the router's document
// of it, else probing its accounts; its share of the router's events and the
// request ledger, as shownFiled has them; and how pooled places every
// account in its pools, as status.Placed has it. It fails where the pool
// named isn't configured, naming those that are: a pool the accounts record
// keeps alone has no accounts to show, nor a document the router gives.
// Where every pool is fenced and none is named, it shows none, nor any
// accounts: the router refuses its source's reads, saying so, and it reads
// none of the router's events, nor any pool's work of the ledger, warning.
func (s shownPools) shown(ctx context.Context, cfg, pooled *config.Config, named string) (watch.Pooled, error) {
	p, err := askedPool(pooled.Pools, named)
	if err != nil && named != "" {
		return watch.Pooled{}, poolFlagNeeded(err)
	}
	shown := watch.Pooled{
		Pool:     status.PoolShown(pooled.Pools, pooled.Accounts, p.ID),
		Accounts: status.PoolAccounts(pooled.Pools, pooled.Accounts, p.ID),
		Source:   s.a.source(ctx, cfg, s.probe).inPool(p.ID),
		Placed:   status.Placed(pooled.Pools, pooled.Accounts),
	}
	if err != nil {
		logger.Warn("can't tell which pool the dashboard shows, so it reads none of the router's files of events, nor the request ledger", "error", err)
		shown.Events, shown.Read = events.Empty{}, views.NoPool()
		return shown, nil
	}
	shown.Events, shown.PoolNamed, shown.Read = s.shownFiled(pooled, p.ID)
	return shown, nil
}

// shownFiled reads what's recorded of the pool with the given id as the
// dashboard shows it, by its id, as the verbs read a pool's, every pool's in
// a config without pools alone: its share of the router's events, by the
// one rule status's InPool gives; what names their pools, as poolsNamed has
// it of pooled's pools; and how the views read its work of the request
// ledger, its read itself, so a pool added while it's open never shows in
// it; the accounts record read afresh. Without a state directory, there are
// no events, and the ledger is read as it is, holding none.
func (s shownPools) shownFiled(pooled *config.Config, id string) (views.Events, func(id string) string, views.PoolRead) {
	if !s.read {
		return events.Empty{}, nil, views.PoolRead{}
	}
	record := recordIn(s.dir)
	read := s.a.poolRead(pooled, s.dir, record, id)
	return read.Filed(s.events), poolsNamed(pooled.Pools, record), read
}
