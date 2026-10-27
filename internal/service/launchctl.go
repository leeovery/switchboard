package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/childenv"
)

const (
	// launchctlTimeout bounds each run of launchctl, which normally takes a
	// moment.
	launchctlTimeout = 10 * time.Second
	// bootoutTimeout bounds booting the service out and waiting for launchd
	// to let it go, which it does only once the router has stopped: launchd
	// gives it exitTimeout before it kills it.
	bootoutTimeout = exitTimeout + launchctlTimeout
	// unloadPoll is how often unloading the service asks launchd whether it
	// has let it go.
	unloadPoll = 200 * time.Millisecond
)

// unknownService is the status launchctl print exits with for a service
// launchd hasn't loaded, saying "Could not find service … in domain": seen
// on macOS itself, and the one exit status the service relies on.
const unknownService = 113

// Runner runs launchctl with args to its end, as Launchctl does, returning
// what it printed. It fails, with an error that has an ExitCode method, as
// *exec.ExitError does, when launchctl exits with a status other than 0.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Launchctl runs launchctl with args, by name, so the one on PATH runs, and
// returns what it printed, its output and its errors together. It runs in no
// more of this process's environment than it needs, which can hold a token,
// as a Claude Code session's does.
func Launchctl(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "launchctl", args...)
	cmd.Env = childenv.Minimal(os.Getenv)
	return cmd.CombinedOutput()
}

// exitError is a program's exit with a status other than 0.
type exitError interface {
	error
	ExitCode() int
}

// launchctl runs launchctl with args, failing, with what it printed, when it
// exits with a status other than 0.
func (s *Service) launchctl(ctx context.Context, args ...string) error {
	return s.launchctlWithin(ctx, launchctlTimeout, args...)
}

// launchctlWithin runs launchctl with args as launchctl does, for up to
// limit.
func (s *Service) launchctlWithin(ctx context.Context, limit time.Duration, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := s.cfg.Launchctl(ctx, args...)
	command := strings.Join(args, " ")
	logger.Debug("ran launchctl", "args", command, "error", err)
	if err == nil {
		return nil
	}
	if said := strings.TrimSpace(string(out)); said != "" {
		return fmt.Errorf("launchctl %s: %s (%w)", command, said, err)
	}
	return fmt.Errorf("launchctl %s: %w", command, err)
}

// loaded reports whether launchd has loaded the service, as launchctl print
// says: it knows the service, or exits unknownService.
func (s *Service) loaded(ctx context.Context) (bool, error) {
	err := s.launchctl(ctx, "print", s.target())
	if exit, ok := errors.AsType[exitError](err); ok && exit.ExitCode() == unknownService {
		return false, nil
	}
	return err == nil, err
}

// unload has launchd stop the service and forget it, when it has loaded it,
// and waits, up to bootoutTimeout from the bootout, for launchd to let it
// go: bootout can return while the router is still finishing its requests
// in flight, launchd keeping the service loaded until it has stopped. It
// calls draining once, should launchd still have the service loaded once
// bootout returns, and fails, saying so, should it still have it at the
// deadline.
func (s *Service) unload(ctx context.Context, draining func()) error {
	loaded, err := s.loaded(ctx)
	if err != nil || !loaded {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, bootoutTimeout)
	defer cancel()
	if err := s.launchctlWithin(wait, bootoutTimeout, "bootout", s.target()); err != nil {
		return err
	}
	err = s.waitUnloaded(wait, draining)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return fmt.Errorf("launchd still had the service loaded %v after booting it out", bootoutTimeout)
	}
	return err
}

// waitUnloaded waits until launchd no longer has the service loaded, asking
// every unloadPoll, or ctx ends, calling draining once, should the first ask
// find it loaded.
func (s *Service) waitUnloaded(ctx context.Context, draining func()) error {
	for first := true; ; first = false {
		loaded, err := s.loaded(ctx)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			return err
		case !loaded:
			return nil
		case first:
			logger.Info("launchd still has the service loaded, its router finishing its requests in flight")
			draining()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(unloadPoll):
		}
	}
}
