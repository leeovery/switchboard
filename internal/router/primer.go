package router

import (
	"context"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prime"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// primeLookEvery is the longest the router goes between looks at whether
	// a prime is due. A timer's clock stops while a Mac sleeps, so one set
	// for hours on would fire hours late: a prime due while it slept goes out
	// within this of its waking.
	primeLookEvery = time.Minute
	// primeWait bounds how long the router waits for the primes it sends to
	// end before it looks again.
	primeWait = 10 * time.Second
	// reprimeAfter is how soon an account whose last probe read nothing is
	// primed again, so a prime that fails, as on a token the upstream
	// refuses, isn't sent at every look through the day.
	reprimeAfter = 5 * time.Minute
)

// primer primes the accounts with tokens on the schedule: each account's
// window a request starts is started by a prime, a probe, at the account's
// slot, and again whenever it isn't running until the day ends. A prime
// missed while the Mac slept, or the router was away, goes out as soon as it
// can, unless the day has ended.
type primer struct {
	schedule prime.Schedule
	// accounts are those the schedule has slots for: the accounts with
	// tokens, in the order configured.
	accounts accounts
	state    *state
	probes   *probes
	now      func() time.Time
}

// newPrimer returns what primes the accounts with tokens, as, on the day's
// schedule, or nil when there's none to keep, as when priming is off.
func newPrimer(day config.Day, as accounts, state *state, probes *probes, now func() time.Time) *primer {
	schedule, ok := prime.New(day, as.configured().IDs(), state.policy)
	if !ok {
		return nil
	}
	return &primer{schedule: schedule, accounts: as, state: state, probes: probes, now: now}
}

// run primes each account as it falls due, until ctx ends.
func (p *primer) run(ctx context.Context) {
	for {
		if underway := p.probes.prime(p.accounts, p.due); len(underway) > 0 {
			settle(ctx, underway, primeWait)
		}
		select {
		case <-time.After(p.wait(p.now())):
		case <-ctx.Done():
			return
		}
	}
}

// due reports whether the account with the given id is due a prime at now.
func (p *primer) due(id string, now time.Time) bool {
	at, ok := p.state.nextPrime(id, p.schedule, now)
	return ok && !at.After(now)
}

// wait is how long from now until the next prime falls due, primeLookEvery
// at most.
func (p *primer) wait(now time.Time) time.Duration {
	wait := primeLookEvery
	for _, a := range p.accounts {
		if at, ok := p.state.nextPrime(a.ID, p.schedule, now); ok {
			wait = min(wait, max(at.Sub(now), 0))
		}
	}
	return wait
}

// report is the schedule as the router's status document gives it at now,
// with when each account is next primed.
func (p *primer) report(now time.Time) status.Prime {
	doc := status.Priming(p.schedule)
	for i, slot := range doc.Slots {
		if at, ok := p.state.nextPrime(slot.Account, p.schedule, now); ok {
			doc.Slots[i].Next = at.UTC()
		}
	}
	return doc
}
