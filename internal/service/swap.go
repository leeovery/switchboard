package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/interrupts"
)

// swap writes a's plist and has launchd load it in place of the service it
// has loaded, if any. It's all or nothing once it begins, with the write:
// where ctx is done, the process is interrupted, or launchd can't be asked
// whether it has the service, before then, it does nothing; from the write
// to the bootstrap's end, none of them stops it, the process's interrupts
// held, as Config.Hold has them, each launchctl run bounded by its own limit
// alone. Interrupted as it swaps, it says so as Config.Finishing does, and
// once it has swapped, fails with accounts.ErrInterrupted. The interrupts are
// let go however it ends, a panic among its ends.
func (s *Service) swap(ctx context.Context, a agent) (err error) {
	h := interrupts.Hold(s.cfg.Hold)
	defer func() {
		if interrupted := h.End(); interrupted && err == nil {
			err = fmt.Errorf("the service is installed, but %w before its router answered", accounts.ErrInterrupted)
		}
	}()
	loaded, err := s.beforeSwap(ctx, h)
	if err != nil {
		return err
	}
	h.Watch(s.cfg.Finishing)
	return s.swapThrough(context.WithoutCancel(ctx), a, loaded)
}

// beforeSwap is whether launchd has the service loaded, unless the swap is
// stopped, as stopped has it, before it's asked or once it has answered,
// whatever it answered.
func (s *Service) beforeSwap(ctx context.Context, h *interrupts.Held) (bool, error) {
	if err := stopped(ctx, h); err != nil {
		return false, err
	}
	loaded, err := s.loaded(ctx)
	if stop := stopped(ctx, h); stop != nil {
		return false, stop
	}
	return loaded, err
}

// swapThrough writes a's plist and has launchd load it in place of the
// service it has loaded, where loaded says it has, running through whatever
// fails between: once the old service is booted out, nothing starts a router
// till the new one is loaded, so a bootout that fails is followed by the
// bootstrap all the same, tried till launchd must have let the old one go. A
// service launchd still holds only has the bootstrap fail.
func (s *Service) swapThrough(ctx context.Context, a agent, loaded bool) error {
	if err := s.write(a); err != nil {
		return err
	}
	var until time.Time
	var unloaded error
	if loaded {
		until, unloaded = s.bootOut(ctx)
	}
	if err := s.bootstrap(ctx, until); err != nil {
		return errors.Join(unloaded, err)
	}
	if unloaded != nil {
		logger.Warn("launchd loaded the service, though letting the one before go failed", "error", unloaded)
	}
	logger.Info("installed the service", "plist", s.Plist(), "program", strings.Join(a.Program, " "))
	return nil
}

// bootOut has launchd stop the service and forget it, and waits until it
// has: launchctl bootout can return at once, as on macOS 26, while launchd
// keeps the service until its router has finished its requests in flight.
// Where it fails, it returns when launchd must have let the service go,
// bootoutTimeout on from the bootout, as launchd kills its router at its
// ExitTimeOut before then, having asked launchd again till then where asking
// it failed; once launchd has let it go, it returns the zero time, as
// there's nothing to wait out.
func (s *Service) bootOut(ctx context.Context) (time.Time, error) {
	until := s.clock().Now().Add(bootoutTimeout)
	if err := s.launchctlWithin(ctx, bootoutTimeout, "bootout", s.target()); err != nil {
		return until, err
	}
	for {
		loaded, err := s.loaded(ctx)
		switch {
		case err == nil && !loaded:
			return time.Time{}, nil
		case ctx.Err() != nil:
			return until, context.Cause(ctx)
		case !s.clock().Now().Before(until):
			return until, cmp.Or(err, fmt.Errorf("launchd didn't let the service go within %v of booting it out", bootoutTimeout))
		}
		s.clock().Sleep(unloadPoll)
	}
}

// clock is the clock waits for launchd are on, as Config.Clock has it.
func (s *Service) clock() Clock {
	if s.cfg.Clock != nil {
		return s.cfg.Clock
	}
	return systemClock{}
}

// stopped is why the swap stops before it begins: an interrupt caught, as
// h has it, or ctx done, nil for neither.
func stopped(ctx context.Context, h *interrupts.Held) error {
	if h.Caught() {
		return fmt.Errorf("the service wasn't installed: %w", accounts.ErrInterrupted)
	}
	return context.Cause(ctx)
}
