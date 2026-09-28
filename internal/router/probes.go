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
	// retryAfter is how soon an account whose probe failed is probed again.
	retryAfter = time.Minute
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

// start probes each of the accounts that's due a probe, unless a probe of it
// is under way already, and returns every probe of them under way.
func (p *probes) start(as accounts) []probing {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	var underway []probing
	for _, a := range as {
		done, running := p.running[a.ID]
		if !running {
			if p.stopped || !p.state.due(a.ID, now) {
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

// await probes the accounts as start does, and waits for those probes to end:
// for limit at most, or until ctx ends, after which they go on without it. It
// reports whether there were any.
func (p *probes) await(ctx context.Context, as accounts, limit time.Duration) bool {
	underway := p.start(as)
	if len(underway) == 0 {
		return false
	}
	ids := make([]string, len(underway))
	for i, u := range underway {
		ids[i] = u.account
	}
	started := time.Now()
	timeout := time.NewTimer(limit)
	defer timeout.Stop()
	for _, u := range underway {
		select {
		case <-u.done:
		case <-timeout.C:
			logger.Debug("stopped waiting for probes before choosing", "accounts", strings.Join(ids, ","), "after", limit)
			return true
		case <-ctx.Done():
			return true
		}
	}
	logger.Debug("probed before choosing", "accounts", strings.Join(ids, ","), "duration", time.Since(started).Round(time.Millisecond))
	return true
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
	probed, err := p.prober.Probe(p.ctx, a.token.Reveal())
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
