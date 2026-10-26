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
	// Back is how long a router that isn't running is asked again: where
	// Back is zero, it's asked once.
	Back time.Duration
	// Again is how long after an ask that finds the router not running it's
	// asked again, 200ms where Again is zero.
	Again time.Duration
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
//   - An answer, or an error that's neither a context's nor one of a router
//     not running, is taken as it is, at once, even one that lands as the
//     deadline passes.
//   - A router that isn't running, as router.ErrNotRunning says, or that
//     closed the connection before it began to answer, as
//     router.ErrUnanswered says and one stopping does, may be coming back,
//     and is asked again Again after the last ask ended, but never past
//     Back, nor at or past the deadline: where the next ask would be, Ask
//     stops with the last answer. A router that answers, or begins to, is
//     asked once, as asking again would add work to a router already behind.
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
	for {
		h, err := r.Health(asking)
		switch {
		case isContextErr(err):
			return router.Health{}, cutOff(ctx, asking, deadline.Sub(began), err)
		case !mayBeComingBack(err):
			return h, err
		}
		next := time.Now().Add(p.Again)
		if next.After(began.Add(p.Back)) || !next.Before(deadline) {
			return h, err
		}
		// The pause ends before the deadline, so only the caller can cut it off.
		if paused := pause(asking, time.Until(next)); paused != nil {
			return router.Health{}, paused
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

// mayBeComingBack reports whether err is an ask's that found the router not
// running at that moment: nothing listening, or the connection closed before
// any answer.
func mayBeComingBack(err error) bool {
	return errors.Is(err, router.ErrNotRunning) || errors.Is(err, router.ErrUnanswered)
}

func isContextErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func cancelled(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.Canceled)
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
