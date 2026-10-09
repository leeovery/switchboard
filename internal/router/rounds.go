package router

import (
	"context"
	"time"
)

const (
	// unreadFor is how long nothing may have read an account, by an answer, a
	// probe or a prime, before the router's rounds probe it.
	unreadFor = 30 * time.Minute
	// beforeWeekReset is how long before each of an account's windows longer
	// than a day resets that the router's rounds probe it, so the window's
	// last use is read before it goes.
	beforeWeekReset = 5 * time.Minute
	// fullDay is how long a day runs: a window longer is read before it
	// resets.
	fullDay = 24 * time.Hour
	// firstRetry is how soon the router's rounds probe an account again once
	// a probe of it has read nothing, the wait doubling with each such probe
	// in a row, to unreadFor, as the dashboard backs off while an account
	// can't be read.
	firstRetry = 2 * time.Minute
	// roundEvery is the longest the router goes between looks at whether an
	// account is due a probe on its rounds. A timer's clock stops while a Mac
	// sleeps, so one set for longer would fire that much late: a probe due
	// while it slept goes out within this of its waking.
	roundEvery = time.Minute
	// roundWait bounds how long the router waits for the probes it sends on
	// its rounds to end before it looks again.
	roundWait = 10 * time.Second
)

// rounds are the router's own probes of the accounts it can send on, around
// the clock, whatever else reads them: of each one nothing has read lately,
// but one whose 5-hour window has lapsed and that can take a request only
// where the window a probe starts would reset before its next prime; and of
// each one whose week is about to reset unread, whatever the priming
// schedule, as state's onRounds says. A probe of an account under way, as a
// choice's or a prime's, serves its rounds too.
type rounds struct {
	accounts accounts
	state    *state
	probes   *probes
	// clears reports whether a window the account with the given id starts
	// at now would reset before it's next primed: always, without priming.
	clears func(id string, now time.Time) bool
}

// newRounds returns the rounds of the accounts given, primer saying when each
// is next primed, unless it's nil, as when priming is off.
func newRounds(as accounts, state *state, probes *probes, primer *primer) *rounds {
	clears := func(string, time.Time) bool { return true }
	if primer != nil {
		clears = primer.clears
	}
	return &rounds{accounts: as, state: state, probes: probes, clears: clears}
}

// run probes each account as it falls due, as look does, at once and then
// every roundEvery, until ctx ends.
func (r *rounds) run(ctx context.Context) {
	for {
		r.look(ctx)
		select {
		case <-time.After(roundEvery):
		case <-ctx.Done():
			return
		}
	}
}

// look probes each account with a usable token that its rounds find due, as
// rounding says, and waits for those probes, roundWait at most, or until ctx
// ends.
func (r *rounds) look(ctx context.Context) {
	underway := r.probes.start(r.accounts.sendable(), r.state.rounding(r.clears))
	if len(underway) == 0 {
		return
	}
	started := time.Now()
	switch settle(ctx, underway, roundWait) {
	case settled:
		logger.Debug("probed on its rounds", "accounts", accountsOf(underway), "duration", time.Since(started).Round(time.Millisecond))
	case timedOut:
		logger.Debug("stopped waiting for probes on its rounds", "accounts", accountsOf(underway), "after", roundWait)
	}
}

// retryAfter is how long the router's rounds wait after a probe of an
// account ended before they probe it again: reprobeAfter, as every probe
// does; or, after misses probes in a row that read nothing, firstRetry,
// doubled for each but the first, to unreadFor.
func retryAfter(misses int) time.Duration {
	if misses == 0 {
		return reprobeAfter
	}
	wait := firstRetry
	for range misses - 1 {
		if wait >= unreadFor {
			break
		}
		wait *= 2
	}
	return min(wait, unreadFor)
}
