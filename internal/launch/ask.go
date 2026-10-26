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
// few seconds at most. The router's client gives any call as long, so neither
// cuts the other short.
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
	// Again, which is more than zero where Back is: where Back is zero, it's
	// asked once.
	Back, Again time.Duration
}

var patient = Patience{Answer: AskTimeout, Back: comingBack, Again: askAgain}

// Ask asks r how it is, as patiently as p says. A router that isn't running,
// as router.ErrNotRunning says, is asked again until Back has passed, as it
// may be coming back. One that connects is asked once, and given what's left
// of Answer, as asking again would add work to a router already behind:
// should it not answer by then, Ask fails saying so, as "no answer within
// 5s". Any other answer is taken at once. When ctx ends, Ask stops at once.
func (p Patience) Ask(ctx context.Context, r Router) (router.Health, error) {
	p = cmp.Or(p, patient)
	asking, cancel := context.WithTimeout(ctx, p.Answer)
	defer cancel()
	began := time.Now()
	for next := began.Add(p.Again); ; next = next.Add(p.Again) {
		h, err := r.Health(asking)
		switch {
		case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
			return router.Health{}, fmt.Errorf("no answer within %v", p.Answer)
		case !errors.Is(err, router.ErrNotRunning) || time.Since(began) >= p.Back:
			return h, err
		}
		if err := pause(ctx, time.Until(next)); err != nil {
			return router.Health{}, err
		}
	}
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
