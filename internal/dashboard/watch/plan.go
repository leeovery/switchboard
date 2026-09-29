package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// lookEvery is how often the dashboard looks at the router's document,
	// while it reads the router: over a local socket, which costs nothing
	// upstream.
	lookEvery = 5 * time.Second
	// freshFor is how lately the router must have read an account for r to
	// leave it be.
	freshFor = time.Minute
)

// plan is when the dashboard next reads its source, and what it asks for.
// The source's document says how often to read it. The router's costs
// nothing to read, so the dashboard looks at it every lookEvery, and once an
// interval has the router refresh the accounts it hasn't read for that long;
// and a look once a window on screen has reset has the router refresh first
// the accounts it hasn't read in the last minute, as an idle account's
// window would read "resets now" until the next full read, unless the
// account has a window that has lapsed, which the router won't probe. A
// document built by probing is read again once an interval, sooner after a
// read that failed or once a window on screen resets, and the router is asked
// after once a minute in between, so the dashboard reads it again soon after
// it comes back, but never goes back and forth faster than that.
type plan struct {
	interval time.Duration
	// due is when the next full read is due: one that has the router
	// refresh, or probes every account when the router doesn't answer.
	due time.Time
	// next is when the next look at the router's document is due, while the
	// dashboard reads the router.
	next time.Time
	// reset is when the first window on screen to reset since fresh wants
	// reading again, a grace period after it resets, while the dashboard
	// reads the router: zero when none does.
	reset time.Time
	// fresh is when the router last refreshed the accounts it hadn't read in
	// the last minute: a window that reset before then has been read again.
	fresh time.Time
	// asked is when a read last asked after the router.
	asked time.Time
	// failures counts the full reads that failed in a row, in whole or in
	// part.
	failures int
}

// full is the read of every account a plan asks for once an interval.
func (p plan) full() Read {
	return Read{Refresh: p.interval, Probe: true}
}

// at returns the read due at now, reading the router when routed, and
// reports false when none is: a full read once it's due; else, reading the
// router, a look at its document; else, once a minute, a question after the
// router, which has it refresh when it answers, and probes nothing when it
// doesn't.
func (p plan) at(now time.Time, routed bool) (Read, bool) {
	switch {
	case !now.Before(p.due):
		return p.full(), true
	case routed && !now.Before(p.next):
		return p.look(now), true
	case !routed && now.Truncate(time.Minute).After(p.asked):
		return Read{Refresh: p.interval}, true
	default:
		return Read{}, false
	}
}

// look is the look at the router's document due at now: one that has the
// router refresh the accounts it hasn't read in the last minute first, once a
// window on screen has reset since it last did.
func (p plan) look(now time.Time) Read {
	if !p.reset.IsZero() && !now.Before(p.reset) {
		return Read{Refresh: freshFor, Probe: true}
	}
	return Read{Probe: true}
}

// landed notes a read that asked for r landing at now with doc. After the
// router's, the next look is lookEvery on, and after it refreshed, the next
// full read is an interval on, sooner after a read that failed in part.
// After one built by probing, the next is an interval on, sooner after a
// read that failed in part or for a window that resets before then.
func (p plan) landed(r Read, doc status.Document, now time.Time) plan {
	p.asked = now
	var wait time.Duration
	if !routed(doc) {
		p.failures, wait = backoff(p.failures, incomplete(doc), p.interval)
		p.due = nextFetch(doc, now, wait)
		return p
	}
	if r.Refresh > 0 {
		p.failures, wait = backoff(p.failures, incomplete(doc), p.interval)
		p.due = now.Add(wait)
	}
	if r.Refresh > 0 && r.Refresh <= freshFor {
		p.fresh = now
	}
	p.next = now.Add(lookEvery)
	p.reset = rereadAfterReset(doc, p.fresh)
	return p
}

// failed notes a read that failed at now, while doc, as it stands on screen,
// was read before: the next is due sooner, backing off as the failures run
// on, and sooner still for a window in doc that resets before then.
func (p plan) failed(doc status.Document, now time.Time) plan {
	p.asked = now
	var wait time.Duration
	p.failures, wait = backoff(p.failures, true, p.interval)
	p.due = nextFetch(doc, now, wait)
	return p
}

// missed notes a question after the router, at now, that found it gone.
func (p plan) missed(now time.Time) plan {
	p.asked = now
	return p
}
