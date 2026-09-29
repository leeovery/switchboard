package router

import (
	"context"
	"strings"
	"sync"
	"time"
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
	// ctx ends when the router stops, cutting short the probes under way.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.Mutex
	// running holds, for each account under probe, what closes when the
	// probe ends.
	running map[string]chan struct{}
	stopped bool
}

func newProbes(prober Prober, state *state, now func() time.Time) *probes {
	ctx, cancel := context.WithCancel(context.Background())
	return &probes{prober: prober, state: state, now: now, ctx: ctx, cancel: cancel, running: make(map[string]chan struct{})}
}

// probing is a probe of an account under way.
type probing struct {
	account string
	// done closes when the probe ends.
	done <-chan struct{}
}

// start probes each of the accounts that due says wants a probe, unless a
// probe of it is under way already, and returns every probe of them under
// way.
func (p *probes) start(as accounts, due func(id string, now time.Time) bool) []probing {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	var underway []probing
	for _, a := range as {
		done, running := p.running[a.ID]
		if !running {
			if p.stopped || !due(a.ID, now) {
				continue
			}
			done = make(chan struct{})
			p.running[a.ID] = done
			p.wg.Go(func() {
				p.probe(a)
				p.finish(a.ID)
			})
		}
		underway = append(underway, probing{account: a.ID, done: done})
	}
	return underway
}

func (p *probes) finish(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	close(p.running[id])
	delete(p.running, id)
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

// probe reads an account's usage, and logs how that went.
func (p *probes) probe(a account) {
	started := time.Now()
	probed, err := p.prober.Probe(p.ctx, a.token().Reveal())
	took := time.Since(started).Round(time.Millisecond)
	if p.ctx.Err() != nil {
		// Stopped mid-probe: its failure says nothing of the account.
		return
	}
	p.state.recordProbe(a.ID, probed, err)
	if err != nil {
		logger.Warn("probe failed", "account", a.ID, "duration", took, "error", err)
		return
	}
	logger.Debug("probed account", "account", a.ID, "duration", took, "windows", len(probed.Windows))
	for _, f := range probed.Failures {
		logger.Warn("window unread", "account", a.ID, "window", f.Window, "error", f.Error)
	}
}
