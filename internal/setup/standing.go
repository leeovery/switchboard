package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// PartStanding is how a part of setup's status line stands, which marks it
// and inks its words.
type PartStanding int

const (
	// PartTold is a part told alone, unmarked, its words muted.
	PartTold PartStanding = iota
	// PartUp is a part that's up, as a router that answers healthy.
	PartUp
	// PartWarning is a part that's up, but not well, as a router that
	// answers unhealthy.
	PartWarning
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
		parts = append(parts, review.part())
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

// Standing is whether a router answers, of those the service may run, asked
// as the service step asks: the installed plist's first, else, where it
// can't be read, this shell's. One that hasn't answered within
// launch.AskTimeout is down. It fails only with ctx's error, once ctx is
// done.
func (s *Setup) Standing(ctx context.Context) (RouterStanding, error) {
	asking, cancel := context.WithTimeout(ctx, launch.AskTimeout)
	defer cancel()
	h := s.readPlist(asking).served(s.LaunchAgent).Answering(asking)
	switch {
	case ctx.Err() != nil:
		return RouterStanding{}, ctx.Err()
	case h == nil:
		return RouterStanding{State: RouterDown}, nil
	case !h.OK:
		return RouterStanding{State: RouterUnhealthy, Reason: h.Reason}, nil
	}
	return RouterStanding{State: RouterHealthy}, nil
}

// Part is the router's part of setup's status line, of configured accounts:
// "router up, healthy", or unhealthy, saying why; "router not running yet";
// or, with no account and no router, "nothing set up yet".
func (r RouterStanding) Part(configured int) Part {
	switch {
	case r.State == RouterHealthy:
		return Part{Standing: PartUp, Words: r.up()}
	case r.State == RouterUnhealthy:
		return Part{Standing: PartWarning, Words: r.up()}
	case configured == 0:
		return Part{Standing: PartDown, Words: "nothing set up yet"}
	}
	return Part{Standing: PartDown, Words: "router not running yet"}
}

// up words a router that answers, as the service step has it: "router up,
// healthy", or "router up, unhealthy: " and why.
func (r RouterStanding) up() string {
	return "router up, " + service.Healthy(router.Health{OK: r.State == RouterHealthy, Reason: r.Reason})
}

// plistRead is the service's plist in place, as read back: the options it was
// installed with, and the service they run, its router where the plist puts
// its state.
type plistRead struct {
	options service.InstallOptions
	svc     *service.Service
	// unread is why the plist couldn't be read back, matching
	// fs.ErrNotExist where there's none; placeless, why the service it runs
	// couldn't be made of it.
	unread, placeless error
}

// readPlist reads back the service's plist in place, and the service it
// runs.
func (s *Setup) readPlist(ctx context.Context) plistRead {
	options, err := s.LaunchAgent.Options(ctx)
	if err != nil {
		return plistRead{unread: err}
	}
	svc, err := s.LaunchAgent.For(options)
	if err != nil {
		return plistRead{options: options, placeless: err}
	}
	return plistRead{options: options, svc: svc}
}

// served is the service the plist runs, else base, where it couldn't be
// read or made.
func (p plistRead) served(base *service.Service) *service.Service {
	if p.svc == nil {
		return base
	}
	return p.svc
}
