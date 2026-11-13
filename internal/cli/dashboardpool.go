package cli

import (
	"context"
	"errors"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// shownPools is how the dashboard reads the pool it shows: probing alone
// where probe says, and the router's events through the readers r gives,
// where read says there are any.
type shownPools struct {
	a     *app
	probe bool
	r     stateReaders
	read  bool
}

// opened is what the dashboard opens on of the config cfg: the pool named,
// else the one the verbs act on, of the pools as they stand, as
// pooledConfig has them, as shown has it. It fails where a pool is named
// and the pools can't be told, as pooledConfig fails; where none is named,
// it opens on none, warning, its source reading the pool the verbs act on,
// as before pools, its accounts the config's, which names none, and none of
// the router's events, so the dashboard never shows every pool's together.
func (s shownPools) opened(ctx context.Context, cfg *config.Config, named string) (watch.Pooled, error) {
	pooled, err := s.a.pooledConfig(ctx, cfg)
	switch {
	case err != nil && named != "":
		return watch.Pooled{}, err
	case err != nil:
		logger.Warn("can't tell which pool the dashboard shows, so it reads none of the router's files of events", "error", err)
		return watch.Pooled{Accounts: status.PoolAccounts(cfg.Pools, cfg.Accounts, ""), Source: s.a.source(ctx, cfg, s.probe), Events: events.Empty{}}, nil
	}
	return s.shown(ctx, cfg, pooled, named)
}

// turn is what the dashboard shows of the pool P turns to from the one with
// the id from, as status.PoolAfter has it: the config read afresh, its
// pools as they stand, so a pool added or removed while it's open shows at
// the next P. It fails where the config can't be read, or the pools told.
func (s shownPools) turn(ctx context.Context, from string) (watch.Pooled, error) {
	cfg, err := s.a.loadConfig()
	if err != nil {
		return watch.Pooled{}, err
	}
	pooled, err := s.a.pooledConfig(ctx, cfg)
	if err != nil {
		return watch.Pooled{}, err
	}
	next, ok := status.PoolAfter(pooled.Pools, from)
	if !ok {
		return watch.Pooled{}, errors.New("the config names no pools")
	}
	return s.shown(ctx, cfg, pooled, next.ID)
}

// shown is what the dashboard shows of the pool named of the config cfg,
// its pools as pooled has them, else of the one the verbs act on, as
// askedPool has it: the pool, as status.PoolShown has it; its accounts, as
// status.PoolAccounts has them; its source, reading the router's document
// of it, else probing its accounts; and its share of the router's events,
// as shownEvents has it. It fails where the pool named isn't configured,
// naming those that are: a pool the accounts record keeps alone has no
// accounts to show, nor a document the router gives. Where every pool is
// fenced and none is named, it shows none, nor any accounts: the router
// refuses its source's reads, saying so, and it reads none of the router's
// events, warning.
func (s shownPools) shown(ctx context.Context, cfg, pooled *config.Config, named string) (watch.Pooled, error) {
	p, err := askedPool(pooled.Pools, named)
	if err != nil && named != "" {
		return watch.Pooled{}, poolFlagNeeded(err)
	}
	shown := watch.Pooled{
		Pool:     status.PoolShown(pooled.Pools, pooled.Accounts, p.ID),
		Accounts: status.PoolAccounts(pooled.Pools, pooled.Accounts, p.ID),
		Source:   s.a.source(ctx, cfg, s.probe).inPool(p.ID),
	}
	if err != nil {
		logger.Warn("can't tell which pool the dashboard shows, so it reads none of the router's files of events", "error", err)
		shown.Events = events.Empty{}
		return shown, nil
	}
	shown.Events, shown.PoolNamed = s.shownEvents(pooled, p.ID)
	return shown, nil
}

// shownEvents reads the router's events of the pool with the given id as
// the dashboard shows them, as the verbs read a pool's, by the one rule
// status's InPool gives, every pool's in a config without pools; and names
// their pools, as poolsNamed has it of pooled's pools, the accounts record
// read afresh. Without a state directory, there are none.
func (s shownPools) shownEvents(pooled *config.Config, id string) (views.Events, func(id string) string) {
	if !s.read {
		return events.Empty{}, nil
	}
	record := recordIn(s.r.dir)
	return s.a.poolRead(pooled, s.r.dir, record, id).Filed(s.r.events), poolsNamed(pooled.Pools, record)
}
