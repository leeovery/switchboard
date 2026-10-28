package setup

import (
	"context"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// PartStanding is how a part of setup's status line stands, which marks it
// and inks its words.
type PartStanding int

const (
	// PartTold is a part told alone, unmarked, its words muted.
	PartTold PartStanding = iota
	// PartUp is a part that's up, as a router that answers.
	PartUp
	// PartDown is a part that's not up yet, as a router not running.
	PartDown
)

// Part is a part of setup's status line: how it stands, and its words.
type Part struct {
	Standing PartStanding
	Words    string
}

// StatusLine is setup's status line: the router's part, then the accounts'
// wherever there are any.
func StatusLine(rt RouterStanding, review Review) []Part {
	parts := []Part{rt.Part(len(review.Accounts))}
	if len(review.Accounts) > 0 {
		parts = append(parts, review.Part())
	}
	return parts
}

// RouterState is whether a router answers, and how.
type RouterState int

const (
	// RouterDown is no router answering.
	RouterDown RouterState = iota
	// RouterHealthy is a router answering healthy.
	RouterHealthy
	// RouterUnhealthy is a router answering, but unhealthy.
	RouterUnhealthy
)

// RouterStanding is whether a router answers, as setup's status line tells
// it.
type RouterStanding struct {
	State RouterState
	// Reason is why a router that answers is unhealthy, as it says: ""
	// otherwise.
	Reason string
}

// standingWait is how long Standing waits for a router to answer: the status
// line tells of one, and is drawn again as setup goes, so it never holds
// the screen up for as long as a launch waits.
const standingWait = 2 * time.Second

// Standing is whether a router answers, of those the service may run, as the
// service step asks: one that hasn't within standingWait is down.
func (s *Setup) Standing(ctx context.Context) RouterStanding {
	ctx, cancel := context.WithTimeout(ctx, standingWait)
	defer cancel()
	h := s.LaunchAgent.Answering(ctx)
	switch {
	case h == nil:
		return RouterStanding{State: RouterDown}
	case !h.OK:
		return RouterStanding{State: RouterUnhealthy, Reason: h.Reason}
	}
	return RouterStanding{State: RouterHealthy}
}

// Part is the router's part of setup's status line, of configured accounts:
// "router up, healthy", or unhealthy, saying why; "router not running yet";
// or, with no account and no router, "nothing set up yet".
func (r RouterStanding) Part(configured int) Part {
	switch {
	case r.State != RouterDown:
		return Part{Standing: PartUp, Words: "router up, " + service.Healthy(router.Health{OK: r.State == RouterHealthy, Reason: r.Reason})}
	case configured == 0:
		return Part{Standing: PartDown, Words: "nothing set up yet"}
	}
	return Part{Standing: PartDown, Words: "router not running yet"}
}
