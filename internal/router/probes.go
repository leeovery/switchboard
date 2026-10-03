package router

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// staleAfter is how old an account's usage can grow before a choice made
	// afresh probes it again.
	staleAfter = 15 * time.Minute
	// reprobeAfter is how soon an account is probed again once a probe of it
	// has ended, as one that failed; an account without room also waits that
	// long after its usage was last read.
	reprobeAfter = time.Minute
)

// probes reads accounts' usage by probing them: never one account twice at
// once, so choices made together share a probe, and never once the router
// has stopped. It's safe for concurrent use.
type probes struct {
	prober Prober
	state  *state
	now    func() time.Time
	// emit hears of each prime that starts its window.
	emit func(Event)
	// ctx ends when the router stops, cutting short the probes under way.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.Mutex
	// running holds, for each account under probe, the probe.
	running map[string]*run
	stopped bool
}

func newProbes(prober Prober, state *state, now func() time.Time, emit func(Event)) *probes {
	ctx, cancel := context.WithCancel(context.Background())
	return &probes{prober: prober, state: state, now: now, emit: emit, ctx: ctx, cancel: cancel, running: make(map[string]*run)}
}

// run is a probe of an account under way, as probes keeps it.
type run struct {
	// done closes when the probe ends.
	done chan struct{}
	// priming is set once a prime starts the probe, or shares it, which is
	// then logged as a prime.
	priming bool
}

// probing is a probe of an account under way.
type probing struct {
	account string
	// done closes when the probe ends.
	done <-chan struct{}
}

// report notes how a probe of an account, sent at sent, went: what it read,
// or why it read nothing, and how long it took.
type report func(a account, probed quota.Probe, err error, sent time.Time, took time.Duration)

// start probes each of the accounts that due says wants a probe, unless a
// probe of it is under way already, and returns every probe of them under
// way.
func (p *probes) start(as accounts, due func(id string, now time.Time) bool) []probing {
	return p.launch(as, due, false)
}

// prime primes each of the accounts that due says wants a prime, as start
// probes them, and returns every probe of them under way: a prime is a probe,
// and one of an account already under way serves as its prime, and is logged
// as one.
func (p *probes) prime(as accounts, due func(id string, now time.Time) bool) []probing {
	return p.launch(as, due, true)
}

// launch probes each of the accounts that due says wants it, unless a probe
// of it is under way already, as a prime when priming says so, and returns
// the probes of those accounts under way: one under way of an account that
// doesn't want it isn't waited for.
func (p *probes) launch(as accounts, due func(id string, now time.Time) bool, priming bool) []probing {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	var underway []probing
	for _, a := range as {
		if !due(a.ID, now) {
			continue
		}
		r, running := p.running[a.ID]
		if !running {
			if p.stopped {
				continue
			}
			r = &run{done: make(chan struct{})}
			p.running[a.ID] = r
			p.wg.Go(func() {
				p.probe(a, r)
				p.finish(a.ID)
			})
		}
		r.priming = r.priming || priming
		underway = append(underway, probing{account: a.ID, done: r.done})
	}
	return underway
}

func (p *probes) finish(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	close(p.running[id].done)
	delete(p.running, id)
}

// told returns what notes how the probe r went: logPrime once a prime has
// started it or shared it, else logProbe.
func (p *probes) told(r *run) report {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.priming {
		return p.logPrime
	}
	return logProbe
}

// sourceOf is where what the probe r reads comes from: a prime, once a prime
// has started it or shared it, else a probe.
func (p *probes) sourceOf(r *run) source {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.priming {
		return fromPrime
	}
	return fromProbe
}

// await probes the accounts as start does, and waits for those probes to end
// as wait does. It reports whether there were any.
func (p *probes) await(ctx context.Context, as accounts, due func(id string, now time.Time) bool, limit time.Duration) bool {
	underway := p.start(as, due)
	if len(underway) == 0 {
		return false
	}
	p.wait(ctx, underway, limit)
	return true
}

// wait waits for probes under way before a choice, as settle does, and logs
// how that went.
func (p *probes) wait(ctx context.Context, underway []probing, limit time.Duration) {
	started := time.Now()
	switch settle(ctx, underway, limit) {
	case settled:
		logger.Debug("probed before choosing", "accounts", accountsOf(underway), "duration", time.Since(started).Round(time.Millisecond))
	case timedOut:
		logger.Debug("stopped waiting for probes before choosing", "accounts", accountsOf(underway), "after", limit)
	}
}

// ending is how waiting for probes under way ended.
type ending int

const (
	// settled is every probe having ended.
	settled ending = iota
	// timedOut is the wait's limit passing first.
	timedOut
	// abandoned is the waiter's context ending first.
	abandoned
)

// settle waits for probes under way to end: for limit at most, or until ctx
// ends, after which they go on without it. It says which came first.
func settle(ctx context.Context, underway []probing, limit time.Duration) ending {
	timeout := time.NewTimer(limit)
	defer timeout.Stop()
	for _, u := range underway {
		select {
		case <-u.done:
		case <-timeout.C:
			return timedOut
		case <-ctx.Done():
			return abandoned
		}
	}
	return settled
}

// accountsOf lists the accounts of probes under way, as the log shows them.
func accountsOf(underway []probing) string {
	ids := make([]string, len(underway))
	for i, u := range underway {
		ids[i] = u.account
	}
	return strings.Join(ids, ",")
}

// stop cuts short the probes under way, and waits for them to end. None
// starts after it.
func (p *probes) stop() {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.cancel()
	p.wg.Wait()
}

// probe reads an account's usage, as r, and notes how that went.
func (p *probes) probe(a account, r *run) {
	started, at, sent := time.Now(), p.now(), p.state.mark()
	probed, err := p.prober.Probe(p.ctx, a.token().Reveal())
	took := time.Since(started).Round(time.Millisecond)
	if p.ctx.Err() != nil {
		// Stopped mid-probe: its failure says nothing of the account.
		return
	}
	p.state.recordProbe(a.ID, probed, err, sent, p.sourceOf(r))
	p.told(r)(a, probed, err, at, took)
}

// logProbe logs how a probe of an account went.
func logProbe(a account, probed quota.Probe, err error, _ time.Time, took time.Duration) {
	if err != nil {
		logger.Warn("probe failed", "account", a.ID, "duration", took, "error", err)
		return
	}
	logger.Debug("probed account", "account", a.ID, "duration", took, "windows", len(probed.Windows))
	logUnread(a, probed)
}

// logPrime logs how a prime of an account, sent at sent, went: at info, with
// the reset it read of the window it primed, or at warn when it failed, or
// didn't start that window. A prime that started it is told of too, but for
// one that found it running already, as when the Claude apps started it.
func (p *probes) logPrime(a account, probed quota.Probe, err error, sent time.Time, took time.Duration) {
	if err != nil {
		logger.Warn("prime failed", "account", a.ID, "duration", took, "error", err)
		return
	}
	attrs := []any{"account", a.ID, "duration", took}
	primed := Primed{Account: a.ID, Window: p.state.policy.Started}
	if i := slices.IndexFunc(probed.Windows, func(w quota.Window) bool { return w.Key == primed.Window }); i >= 0 {
		primed.ResetsAt = probed.Windows[i].ResetsAt
		attrs = append(attrs, "resets", primed.ResetsAt)
	}
	if p.state.primeFailed(a.ID) {
		logger.Warn("prime didn't start the window", append(attrs, "window", primed.Window)...)
	} else {
		logger.Info("primed", attrs...)
		if primed.startedBy(sent) {
			p.emit(primed)
		}
	}
	logUnread(a, probed)
}

// markedBack is the most the upstream takes the start of a window back by:
// to the ten-minute mark it falls in.
const markedBack = 10 * time.Minute

// startedBy reports whether the window the prime read is one it started,
// having been sent at sent: it resets the window's length after that, or
// after the ten-minute mark it falls in. One that resets sooner was running
// already.
func (p Primed) startedBy(sent time.Time) bool {
	length, ok := quota.Length(p.Window)
	return ok && !p.ResetsAt.IsZero() && !p.ResetsAt.Before(sent.Add(length-markedBack))
}

// logUnread logs each window a probe of an account expected, and couldn't
// read.
func logUnread(a account, probed quota.Probe) {
	for _, f := range probed.Failures {
		logger.Warn("window unread", "account", a.ID, "window", f.Window, "error", f.Error)
	}
}
