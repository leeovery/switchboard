package router

import (
	"cmp"
	"context"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// watchEvery is how often the router looks at what it was started from,
// unless its config says otherwise.
const watchEvery = 3 * time.Second

// upkeep keeps the router in step with what it was started from while it runs,
// looking every so often: the accounts' token files, which it takes up in
// place, and its config file, the binary it upgrades from and the system's
// time zone, which it restarts to take up, but for the tokens of accounts the
// config file no longer configures, which count as the primary's at once, and
// the folder it names to export into, which the exports take up at their
// next round. It notices the Mac waking from sleep as it looks.
type upkeep struct {
	every    time.Duration
	tokens   *tokenFiles
	restarts *restarts
	wakes    *wakes
	exports  *exports
	// looked closes once upkeep has first looked, as look does.
	looked     chan struct{}
	lookedOnce sync.Once
}

// newUpkeep returns the upkeep of a router built from cfg, with the accounts
// given, whose refusals state lifts once they go out on another token,
// telling emit of those that lift, noting each change to their tokens for the
// state file to keep with changes, working out the schedule of primer, if it
// primes, again whenever the accounts with tokens change, telling emit of a
// restart as it falls due, restarting once inFlight counts no request in
// flight, and the binary it restarts into, asked as cfg.Check runs it, an
// upgrade copied in first where it runs a copy of its own, doesn't refuse to
// start, noticing wakes, and configuring exports as the config file comes to.
func newUpkeep(cfg Config, as accounts, state *state, changes *changes, primer *primer, inFlight *inFlight, wakes *wakes, exports *exports, emit func(Event)) *upkeep {
	replan := func() {}
	if primer != nil {
		replan = primer.replan
	}
	return &upkeep{
		every: cmp.Or(cfg.WatchEvery, watchEvery),
		tokens: &tokenFiles{
			accounts: as,
			read:     cfg.Token,
			taken:    cfg.Taken,
			now:      cfg.Now,
			kept:     changes.note,
			sendable: replan,
			replaced: func(id string) {
				if lifted, ok := state.tokenReplaced(id); ok {
					emit(lifted)
				}
			},
			gained: state.tokenGained,
			lost:   state.tokenLost,
		},
		restarts: restartsFor(cfg, inFlight, emit),
		wakes:    wakes,
		exports:  exports,
		looked:   make(chan struct{}),
	}
}

// run looks after the router every so often, until ctx ends or the router
// restarts, removing as it returns any upgrade it copied in and kept, so none
// outlives the router's run.
func (u *upkeep) run(ctx context.Context) {
	tick := time.NewTicker(u.every)
	defer tick.Stop()
	defer u.restarts.discardUpgrade()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			u.look()
		case <-u.restarts.ready():
			if u.restarts.restart(ctx) {
				return
			}
		}
	}
}

// look looks at what the router was started from, once, as run does every so
// often, taking up in place what the config file has come to make of the
// accounts' tokens and the folder to export into, where it has changed, and
// closing firstLook's channel the first time.
func (u *upkeep) look() {
	u.wakes.look()
	u.tokens.look()
	u.restarts.look()
	u.tokens.retire(u.restarts.configured())
	if e, changed := u.restarts.configuredExport(); changed {
		u.exports.configure(e)
	}
	u.lookedOnce.Do(func() { close(u.looked) })
}

// firstLook returns what's closed once upkeep has first looked at what the
// router was started from, so a restart due as the router starts is known.
func (u *upkeep) firstLook() <-chan struct{} {
	return u.looked
}

// tidy removes what copies cut short left beside the router's copy, where it
// runs one, as it starts.
func (u *upkeep) tidy() {
	u.restarts.removeLeftovers()
}

// restartComing reports whether the router is to restart itself, as
// restarts' coming says.
func (u *upkeep) restartComing() bool {
	return u.restarts.coming()
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

// restartable returns what clears the router to restart at once, as it's
// asked to, its binary asked whether it could start until ctx ends, or fails,
// saying why, when it can't.
func (u *upkeep) restartable(ctx context.Context) (*binaryClearance, error) {
	return u.restarts.restartable(ctx)
}

// restartNow restarts the router at once, whatever is in flight, once
// restartable has found it can, cleared as it returned.
func (u *upkeep) restartNow(cleared *binaryClearance) {
	u.restarts.atOnce(cleared)
}
