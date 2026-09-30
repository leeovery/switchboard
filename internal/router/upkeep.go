package router

import (
	"cmp"
	"context"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// watchEvery is how often the router looks at what it was started from,
// unless its config says otherwise.
const watchEvery = 3 * time.Second

// upkeep keeps the router in step with what it was started from while it runs,
// looking every so often: the accounts' token files, which it takes up in
// place, and its config file, its binary and the system's time zone, which it
// restarts to take up, but for the tokens of accounts the config file no
// longer configures, which count as the primary's at once. It notices the
// Mac waking from sleep as it looks.
type upkeep struct {
	every    time.Duration
	tokens   *tokenFiles
	restarts *restarts
	wakes    *wakes
}

// newUpkeep returns the upkeep of a router built from cfg, with the accounts
// given, whose refusals state lifts once they go out on another token, noting
// each change to their tokens for the state file to keep with changes,
// working out the schedule of primer, if it primes, again whenever the
// accounts with tokens change, restarting once inFlight counts no request in
// flight, and noticing wakes.
func newUpkeep(cfg Config, as accounts, state *state, changes *changes, primer *primer, inFlight *inFlight, wakes *wakes) *upkeep {
	replan := func() {}
	if primer != nil {
		replan = primer.replan
	}
	return &upkeep{
		every: cmp.Or(cfg.WatchEvery, watchEvery),
		tokens: &tokenFiles{
			accounts: as,
			read:     cfg.Token,
			now:      cfg.Now,
			kept:     changes.note,
			sendable: replan,
			replaced: state.tokenReplaced,
		},
		restarts: newRestarts(cfg.ConfigFile, cfg.Binary, cfg.Zone, cfg.Supervised, inFlight, cfg.Now),
		wakes:    wakes,
	}
}

// run looks after the router every so often, until ctx ends or the router
// restarts.
func (u *upkeep) run(ctx context.Context) {
	tick := time.NewTicker(u.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			u.wakes.look()
			u.tokens.look()
			u.restarts.look()
			u.tokens.retire(u.restarts.configured())
		case <-u.restarts.ready():
			if u.restarts.restart() {
				return
			}
		}
	}
}

// restartDue is the restart the router has due, zero while none is.
func (u *upkeep) restartDue() status.Restart {
	return u.restarts.report()
}

// restarted returns what's closed as the router restarts itself.
func (u *upkeep) restarted() <-chan struct{} {
	return u.restarts.restarted
}

// restartReason says why the router restarts, once restarted is closed.
func (u *upkeep) restartReason() string {
	return u.restarts.reason
}

// restartable fails, saying why, when the router can't restart at once, as
// it's asked to.
func (u *upkeep) restartable() error {
	return u.restarts.restartable()
}

// restartNow restarts the router at once, whatever is in flight, once
// restartable has found it can.
func (u *upkeep) restartNow() {
	u.restarts.atOnce()
}
