package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// launchctlTimeout bounds each run of launchctl, which normally takes a
// moment.
const launchctlTimeout = 10 * time.Second

// notLoadedStatuses are launchctl's exit statuses for a service launchd
// hasn't loaded: 3, "No such process", from bootout on older systems, and
// 113, "Could not find specified service", from newer ones, and from print
// and kickstart.
var notLoadedStatuses = []int{3, 113}

// Runner runs launchctl with args to its end, as Launchctl does, returning
// what it printed. It fails, with an error that has an ExitCode method, as
// *exec.ExitError does, when launchctl exits with a status other than 0.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Launchctl runs launchctl with args, by name, so the one on PATH runs, and
// returns what it printed, its output and its errors together.
func Launchctl(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "launchctl", args...).CombinedOutput()
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

// exited reports whether err is launchctl's having run and exited with a
// status other than 0, rather than its failing to run at all.
func exited(err error) bool {
	_, ok := errors.AsType[exitError](err)
	return ok
}

// notLoaded reports whether err is launchctl's for a service launchd hasn't
// loaded.
func notLoaded(err error) bool {
	exit, ok := errors.AsType[exitError](err)
	return ok && slices.Contains(notLoadedStatuses, exit.ExitCode())
}
