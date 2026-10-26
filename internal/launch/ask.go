package launch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// AskTimeout is how long, from the first ask, the router is given to answer
// its health check, wherever it's asked: a router that's slow, as on a machine
// far behind, is waited for, and one that's stuck holds Claude Code up for a
// few seconds at most.
const AskTimeout = 5 * time.Second

const (
	// comingBack is how long, from the first ask, a router that isn't running
	// is asked again, every askAgain: launchd may be bringing it back, or it
	// may be restarting.
	comingBack = 2 * time.Second
	askAgain   = 200 * time.Millisecond
)

// Patience is how long the router is waited for, timed from the first ask of
// its health check. The zero Patience is switchboard's own: AskTimeout to
// answer, a router that isn't running asked again every 200ms for 2s.
type Patience struct {
	// Answer is how long the router is given to answer, AskTimeout where it's
	// zero.
	Answer time.Duration
	// Back is how long a router that isn't running is asked again, every
	// Again, 200ms where it's zero; where Back is zero, it's asked once.
	Back, Again time.Duration
}

var patient = Patience{Answer: AskTimeout, Back: comingBack, Again: askAgain}

// orDefault is p as it's asked with: switchboard's own where it's zero, else
// AskTimeout to answer and 200ms between asks where it doesn't say.
func (p Patience) orDefault() Patience {
	if p == (Patience{}) {
		return patient
	}
	p.Answer = cmp.Or(p.Answer, AskTimeout)
	p.Again = cmp.Or(p.Again, askAgain)
	return p
}

// Ask asks r how it is, as patiently as p says, timed from the first ask,
// until a deadline: Answer on from it, or ctx's deadline where that comes
// first.
//
//   - An answer, or an error that's neither router.ErrNotRunning nor a
//     context's, is taken as it is, at once, even one that lands as the
//     deadline passes.
//   - A router that isn't running, as router.ErrNotRunning says, may be
//     coming back, and is asked again Again on from the last ask, but never
//     sooner than Again from now, as after an ask slow to fail, and never
//     past Back: then Ask stops with the last answer, which it does too when
//     the deadline passes first. A router that connects is asked once, as
//     asking again would add work to a router already behind.
//   - An ask cut off, its dial or its request alike, as a context's error
//     says, fails, once the deadline has passed, saying how long the router
//     had, as "no answer within 5s", whatever the asks before it said;
//     otherwise, as when a deadline of the client's own cut it off, as it is.
//
// Should the caller cancel ctx, Ask stops at once, failing as that says.
func (p Patience) Ask(ctx context.Context, r Router) (router.Health, error) {
	p = p.orDefault()
	began := time.Now()
	deadline := began.Add(p.Answer)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	asking, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for next := began; ; {
		h, err := r.Health(asking)
		switch {
		case isContextErr(err):
			return router.Health{}, cutOff(ctx, asking, deadline.Sub(began), err)
		case !errors.Is(err, router.ErrNotRunning):
			return h, err
		}
		next = after(next, p.Again)
		if next.After(began.Add(p.Back)) {
			return h, err
		}
		if paused := pause(asking, time.Until(next)); paused != nil {
			if cancelled(ctx) {
				return router.Health{}, paused
			}
			return h, err
		}
	}
}

// cutOff is why an ask was cut off, failing with err, a context's error: the
// caller cancelled, or a deadline of the router's client's own ran out, each
// as err says it; or asking ended at its deadline, had on from the first ask,
// as no answer within that.
func cutOff(ctx, asking context.Context, had time.Duration, err error) error {
	switch {
	case cancelled(ctx):
		return err
	case asking.Err() != nil:
		return fmt.Errorf("no answer within %v", max(had, 0).Round(time.Millisecond))
	default:
		return err
	}
}

func isContextErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func cancelled(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.Canceled)
}

// after returns when the ask after one due at last comes: a beat on from it,
// but never sooner than a beat from now, as after an ask slow to fail.
func after(last time.Time, beat time.Duration) time.Time {
	next := last.Add(beat)
	if now := time.Now(); !next.After(now) {
		return now.Add(beat)
	}
	return next
}

// pause waits for d, unless ctx ends first, failing then.
func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("ask the router: %w", ctx.Err())
	}
}
