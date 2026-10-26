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
	// Answer is how long the router is given to answer.
	Answer time.Duration
	// Back is how long a router that isn't running is asked again, every
	// Again, or every 200ms where Again is zero; where Back is zero, it's
	// asked once.
	Back, Again time.Duration
}

var patient = Patience{Answer: AskTimeout, Back: comingBack, Again: askAgain}

// Ask asks r how it is, as patiently as p says. A router that isn't running,
// as router.ErrNotRunning says, is asked again until Back has passed, as it
// may be coming back, never twice without a pause between. One that connects
// is asked once, and given what's left of Answer, as asking again would add
// work to a router already behind. Any other answer is taken at once. Should
// ctx end first, Ask stops at once.
//
// Should no answer come in time, Ask fails saying how long the router had,
// as "no answer within 5s": Answer, or less where ctx's deadline came first.
func (p Patience) Ask(ctx context.Context, r Router) (router.Health, error) {
	p = cmp.Or(p, patient)
	p.Again = cmp.Or(p.Again, askAgain)
	began := time.Now()
	asking, cancel := context.WithTimeout(ctx, p.Answer)
	defer cancel()
	for next := began; ; {
		h, err := r.Health(asking)
		switch {
		case err != nil && asking.Err() != nil:
			return router.Health{}, p.ended(ctx, began, err)
		case !errors.Is(err, router.ErrNotRunning) || time.Since(began) >= p.Back:
			return h, err
		}
		next = after(next, p.Again)
		if err := pause(asking, time.Until(next)); err != nil {
			return router.Health{}, p.ended(ctx, began, err)
		}
	}
}

// ended is why asking, begun at began, ended before the router answered, as
// err, what it ended with, says: the caller gave up, which err says as it is,
// or a deadline ran out, ctx's where it came first, else Answer.
func (p Patience) ended(ctx context.Context, began time.Time, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return err
	case ctx.Err() != nil:
		deadline, _ := ctx.Deadline()
		return noAnswer(deadline.Sub(began).Round(time.Millisecond))
	default:
		return noAnswer(p.Answer)
	}
}

func noAnswer(within time.Duration) error {
	return fmt.Errorf("no answer within %v", within)
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
