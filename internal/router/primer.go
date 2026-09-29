package router

import (
	"context"
	"sync"
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
	// reprimeAfter is how soon an account whose last probe failed as a prime
	// is primed again, so a prime that fails, as on a token the upstream
	// refuses, or that doesn't start the window, isn't sent at every look
	// through the day.
	reprimeAfter = 5 * time.Minute
)

// primer primes the accounts with tokens on the schedule: each account's
// window a request starts is started by a prime, a probe, at the account's
// slot, and again whenever it isn't running until the day ends. A prime
// missed while the Mac slept, or the router was away, goes out as soon as it
// can, unless the day has ended.
type primer struct {
	day config.Day
	// accounts are every account configured: the schedule has slots for
	// those with tokens.
	accounts accounts
	state    *state
	probes   *probes
	now      func() time.Time

	mu   sync.Mutex
	plan plan
}

// plan is the schedule as it was last worked out, and the accounts it has
// slots for: those with tokens then, in the order configured. The zero plan
// has none, as when no account has a token.
type plan struct {
	schedule prime.Schedule
	accounts accounts
}

// newPrimer returns what primes those of the accounts given with tokens on
// the schedule priming gives, or nil when priming is off.
func newPrimer(priming config.Prime, as accounts, state *state, probes *probes, now func() time.Time) *primer {
	if !priming.On() {
		return nil
	}
	p := &primer{day: priming.Day, accounts: as, state: state, probes: probes, now: now}
	p.replan()
	return p
}

// replan works the schedule out again over the accounts with tokens now: as
// the router starts, and whenever an account gains a usable token or loses
// it.
func (p *primer) replan() {
	sendable := p.accounts.sendable()
	var next plan
	if schedule, ok := prime.New(p.day, sendable.configured().IDs(), p.state.policy); ok {
		next = plan{schedule: schedule, accounts: sendable}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plan = next
}

// current returns the plan as it was last worked out.
func (p *primer) current() plan {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.plan
}

// run primes each account as it falls due, until ctx ends.
func (p *primer) run(ctx context.Context) {
	for {
		if underway := p.probes.prime(p.current().accounts, p.due); len(underway) > 0 {
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
	at, ok := p.state.nextPrime(id, p.current().schedule, now)
	return ok && !at.After(now)
}

// wait is how long from now until the next prime falls due, primeLookEvery
// at most.
func (p *primer) wait(now time.Time) time.Duration {
	plan := p.current()
	wait := primeLookEvery
	for _, a := range plan.accounts {
		if at, ok := p.state.nextPrime(a.ID, plan.schedule, now); ok {
			wait = min(wait, max(at.Sub(now), 0))
		}
	}
	return wait
}

// report is the schedule as the router's status document gives it at now,
// with when each account is next primed, or none while there's no schedule.
func (p *primer) report(now time.Time) status.Prime {
	plan := p.current()
	if len(plan.accounts) == 0 {
		return status.Prime{}
	}
	doc := status.Priming(plan.schedule)
	for i, slot := range doc.Slots {
		if at, ok := p.state.nextPrime(slot.Account, plan.schedule, now); ok {
			doc.Slots[i].Next = at.UTC()
		}
	}
	return doc
}
