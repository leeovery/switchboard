package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
)

// swap writes a's plist and has launchd load it in place of the service it
// has loaded, if any. It's all or nothing once it begins, with the write:
// where ctx is done, the process is interrupted, or launchd can't be asked
// whether it has the service, before then, it does nothing; from the write
// to the bootstrap's end, none of them stops it, the process's interrupts
// held, as hold has them, each launchctl run bounded by its own limit alone.
// Interrupted as it swaps, it says so as Config.Finishing does, and once it
// has swapped, fails with accounts.ErrInterrupted. The interrupts are let go
// however it ends, a panic among its ends.
func (s *Service) swap(ctx context.Context, a agent) (err error) {
	h := s.holdInterrupts()
	defer func() {
		if interrupted := h.end(); interrupted && err == nil {
			err = fmt.Errorf("the service is installed, but %w before its router answered", accounts.ErrInterrupted)
		}
	}()
	loaded, err := s.beforeSwap(ctx, h)
	if err != nil {
		return err
	}
	h.watch(s.cfg.Finishing)
	return s.swapThrough(context.WithoutCancel(ctx), a, loaded)
}

// beforeSwap is whether launchd has the service loaded, unless the swap is
// stopped, as stopped has it, before it's asked or once it has answered,
// whatever it answered.
func (s *Service) beforeSwap(ctx context.Context, h *held) (bool, error) {
	if err := h.stopped(ctx); err != nil {
		return false, err
	}
	loaded, err := s.loaded(ctx)
	if stop := h.stopped(ctx); stop != nil {
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

// held is the process's interrupts as swap holds them: caught on a channel
// of their own, and, once watch has begun, told of as the first comes.
// SIGPIPE, held with them, is never one.
type held struct {
	signals chan os.Signal
	release func()
	stop    chan struct{}
	// watched is made as watch begins, and closed once it has ended, got set
	// where it caught an interrupt.
	watched chan struct{}
	got     bool
}

// holdInterrupts holds the process's interrupts, as Config.Hold does.
func (s *Service) holdInterrupts() *held {
	// Room for a SIGPIPE or two beside an interrupt, which os/signal drops
	// where the channel is full.
	h := &held{signals: make(chan os.Signal, 4), stop: make(chan struct{})}
	hold := s.cfg.Hold
	if hold == nil {
		hold = signalHold
	}
	h.release = hold(h.signals)
	return h
}

// stopped is why the swap stops before it begins: an interrupt caught, or
// ctx done, nil for neither.
func (h *held) stopped(ctx context.Context) error {
	if h.caught() {
		return fmt.Errorf("the service wasn't installed: %w", accounts.ErrInterrupted)
	}
	return context.Cause(ctx)
}

// caught reports whether an interrupt waits on the channel, while nothing
// watches it, taking what waits there.
func (h *held) caught() bool {
	for {
		select {
		case sig := <-h.signals:
			if interrupt(sig) {
				return true
			}
		default:
			return false
		}
	}
}

// watch tells told, where it's set, of the first interrupt caught from now,
// as it comes, till end.
func (h *held) watch(told func()) {
	h.watched = make(chan struct{})
	go func() {
		defer close(h.watched)
		for {
			select {
			case sig := <-h.signals:
				if !interrupt(sig) {
					continue
				}
				h.got = true
				if told != nil {
					told()
				}
				return
			case <-h.stop:
				return
			}
		}
	}()
}

// end stops watching, where watch began, and releases the interrupts, then
// reports whether one was caught since watch began: one coming once watching
// has stopped, or as they're released, is caught all the same.
func (h *held) end() bool {
	close(h.stop)
	if h.watched != nil {
		<-h.watched
	}
	h.release()
	return h.got || h.caught()
}

// interrupt reports whether sig is an interrupt: any held but SIGPIPE.
func interrupt(sig os.Signal) bool {
	return sig != syscall.SIGPIPE
}

// heldSignals are the signals swap holds.
var heldSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE}

// signalHold holds heldSignals, as holdSignals has it, for the process.
func signalHold(caught chan<- os.Signal) (release func()) {
	return holdSignals(caught, signal.Ignored, signal.Notify, signal.Stop)
}

// holdSignals holds heldSignals but those the process ignores, as ignored
// says, sending each on caught in place of what it would do, until the
// release it returns is called: it notifies caught, and stops notifying it,
// alone, leaving any other handler of them as it was. One ignored stays
// ignored, as holding it would have it handled from then on, as a nohup's
// SIGHUP.
func holdSignals(caught chan<- os.Signal, ignored func(os.Signal) bool, notify func(chan<- os.Signal, ...os.Signal), stop func(chan<- os.Signal)) (release func()) {
	held := slices.DeleteFunc(slices.Clone(heldSignals), ignored)
	if len(held) == 0 {
		// notify with no signals would send every signal on caught.
		return func() {}
	}
	notify(caught, held...)
	return func() { stop(caught) }
}
