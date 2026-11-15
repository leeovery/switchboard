package router

import (
	"context"
	"slices"
	"strings"
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

// primer primes the accounts with tokens on each pool's schedule: each
// account's window a request starts is started by a prime, a probe, at the
// account's slot, and again whenever it isn't running until the day ends. A
// prime missed while the Mac slept, or the router was away, goes out as soon
// as it can, unless the day has ended. An account whose answers no longer
// report that window has none to prime, and no slot.
type primer struct {
	day config.Day
	// accounts are every account configured: the schedules have slots for
	// those with tokens that have a window to prime.
	accounts accounts
	state    *state
	probes   *probes
	now      func() time.Time

	mu   sync.Mutex
	plan plan
}

// plan is each pool's schedule as it was last worked out, in the pools'
// order, of those with any account to prime. The zero plan has none, as when
// no account has a token.
type plan []poolPlan

// poolPlan is a pool's schedule, by the pool's id, "" the one pool of a
// config from before pools, and the accounts it has slots for: the pool's
// with tokens then that had a window to prime, in the order configured.
type poolPlan struct {
	pool     string
	schedule prime.Schedule
	accounts accounts
}

// accounts returns the accounts the plan has slots for, each pool's in turn.
func (pl plan) accounts() accounts {
	var all accounts
	for _, pp := range pl {
		all = append(all, pp.accounts...)
	}
	return all
}

// ids returns the ids of the accounts the plan has slots for, each pool's in
// turn, in order.
func (pl plan) ids() []string {
	return pl.accounts().configured().IDs()
}

// same reports whether pl has the slots o has: the same pools, each with the
// same accounts.
func (pl plan) same(o plan) bool {
	return slices.EqualFunc(pl, o, func(a, b poolPlan) bool {
		return a.pool == b.pool && slices.Equal(a.accounts.configured().IDs(), b.accounts.configured().IDs())
	})
}

// scheduleOf returns the schedule of the pool whose slots have the account
// with the given id: none, which has no slot for any, where none has.
func (pl plan) scheduleOf(id string) prime.Schedule {
	for _, pp := range pl {
		if slices.ContainsFunc(pp.accounts, func(a account) bool { return a.ID == id }) {
			return pp.schedule
		}
	}
	return prime.Schedule{}
}

// newPrimer returns what primes those of the accounts given with tokens on
// the schedule priming gives, or nil when priming is off.
func newPrimer(priming config.Prime, as accounts, state *state, probes *probes, now func() time.Time) *primer {
	if !priming.On() {
		return nil
	}
	p := &primer{day: priming.Day, accounts: as, state: state, probes: probes, now: now}
	p.plan = p.next()
	return p
}

// replan works each pool's schedule out again over its accounts with tokens
// now that have a window to prime, as the state's primable says, as the
// pools stand: at each look, and whenever an account gains a usable token or
// loses it. It notes in the log each change of the accounts the schedules
// have slots for, whichever of those finds it, as one whose window to prime
// has been retired, or is back, one that gains its token or loses it, or one
// that moves to another pool.
func (p *primer) replan() {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := p.next()
	changed := !next.same(p.plan)
	p.plan = next
	if changed {
		logger.Info("priming schedule worked out again", "accounts", strings.Join(next.ids(), ","))
	}
}

// next is the plan worked out over each pool's accounts with tokens now that
// have a window to prime, as the pools stand: each pool staggers its own
// accounts' resets through the day, as prime.New spreads them.
func (p *primer) next() plan {
	primed := slices.DeleteFunc(p.accounts.sendable(), func(a account) bool { return !p.state.primable(a.ID) })
	t := p.state.pools.current()
	var next plan
	for _, pool := range t.each() {
		in := t.routed(pool, primed)
		if schedule, ok := prime.New(p.day, in.configured().IDs(), p.state.policy); ok {
			next = append(next, poolPlan{pool: pool, schedule: schedule, accounts: in})
		}
	}
	return next
}

// current returns the plan as it was last worked out.
func (p *primer) current() plan {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.plan
}

// run primes each account as it falls due, until ctx ends, working the
// schedule out again first at each look, as replan says: the state's changes
// are heard with its lock held, so the primer looks for them rather than
// hears of them.
func (p *primer) run(ctx context.Context) {
	for {
		p.replan()
		if underway := p.probes.prime(p.current().accounts(), p.due); len(underway) > 0 {
			settle(ctx, underway, primeWait)
		}
		select {
		case <-time.After(p.wait(p.now())):
		case <-ctx.Done():
			return
		}
	}
}

// due reports whether the account with the given id is due a prime at now,
// on its pool's schedule.
func (p *primer) due(id string, now time.Time) bool {
	at, ok := p.state.nextPrime(id, p.current().scheduleOf(id), now)
	return ok && !at.After(now)
}

// clears reports whether a window the account with the given id starts at
// now would reset before the account is next primed, as its pool's
// schedule's Clear says.
func (p *primer) clears(id string, now time.Time) bool {
	return p.current().scheduleOf(id).Clear(id, now)
}

// wait is how long from now until the next prime could fall due, as
// nextLook says of each account on its pool's schedule, primeLookEvery at
// most.
func (p *primer) wait(now time.Time) time.Duration {
	wait := primeLookEvery
	for _, pp := range p.current() {
		for _, a := range pp.accounts {
			if at, ok := p.state.nextLook(a.ID, pp.schedule, now); ok {
				wait = min(wait, max(at.Sub(now), 0))
			}
		}
	}
	return wait
}

// report is the priming schedule as the router's status document gives it at
// now, of each pool shows reports true of, by its id, its slots each pool's
// in turn, with when each account is next primed: none while no such pool
// has a schedule.
func (p *primer) report(now time.Time, shows func(pool string) bool) status.Prime {
	var doc status.Prime
	for _, pp := range p.current() {
		if !shows(pp.pool) {
			continue
		}
		each := status.Priming(pp.schedule)
		for i, slot := range each.Slots {
			if at, ok := p.state.nextPrime(slot.Account, pp.schedule, now); ok {
				each.Slots[i].Next = at.UTC()
			}
		}
		doc.Day, doc.Window, doc.Slots = each.Day, each.Window, append(doc.Slots, each.Slots...)
	}
	return doc
}
