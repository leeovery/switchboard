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

// launchctlTimeout bounds each run of launchctl, which normally takes a
// moment.
const launchctlTimeout = 10 * time.Second

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
	ctx, cancel := context.WithTimeout(ctx, launchctlTimeout)
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
