package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/childenv"
)

const (
	// launchctlTimeout bounds each run of launchctl, which normally takes a
	// moment.
	launchctlTimeout = 10 * time.Second
	// bootoutTimeout bounds booting the service out, with the wait for
	// launchd to let it go once the router stops: launchd gives the router
	// exitTimeout before it kills it.
	bootoutTimeout = exitTimeout + launchctlTimeout
)

// unknownService is the status launchctl print exits with for a service
// launchd hasn't loaded, saying "Could not find service … in domain": seen
// on macOS itself, and the one exit status the service relies on.
const unknownService = 113

// Runner runs a program, launchctl as Launchctl does, or plutil as Plutil
// does, with args to its end, returning what it printed. It fails, with an
// error that has an ExitCode method, as *exec.ExitError does, when the
// program exits with a status other than 0.
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
	_, err := s.launchctlOutput(ctx, limit, args...)
	return err
}

// launchctlOutput runs launchctl with args as launchctl does, for up to
// limit, and returns what it printed.
func (s *Service) launchctlOutput(ctx context.Context, limit time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := s.cfg.Launchctl(ctx, args...)
	command := strings.Join(args, " ")
	logger.Debug("ran launchctl", "args", command, "error", err)
	if err == nil {
		return out, nil
	}
	if said := strings.TrimSpace(string(out)); said != "" {
		return out, fmt.Errorf("launchctl %s: %s (%w)", command, said, err)
	}
	return out, fmt.Errorf("launchctl %s: %w", command, err)
}

// job is the service as launchd has it.
type job struct {
	// loaded is whether launchd has loaded it, running whether it runs a
	// router, and pid that router's process id, 0 where none runs or it
	// can't be read.
	loaded, running bool
	pid             int
}

// jobPID and jobRunning are how launchctl print names the process of the
// job it prints, and says it runs: lines of the job's own, one tab in, which
// it prints only while one runs.
var (
	jobPID     = regexp.MustCompile(`(?m)^\tpid = (\d+)$`)
	jobRunning = regexp.MustCompile(`(?m)^\tstate = running$`)
)

// job is the service as launchctl print says launchd has it: loaded where it
// knows the service, not where it exits unknownService; running where it
// says the job runs, or names the job's process, as a format it doesn't read
// may still say one of them.
func (s *Service) job(ctx context.Context) (job, error) {
	out, err := s.launchctlOutput(ctx, launchctlTimeout, "print", s.target())
	if exit, ok := errors.AsType[exitError](err); ok && exit.ExitCode() == unknownService {
		return job{}, nil
	}
	if err != nil {
		return job{}, err
	}
	j := job{loaded: true, running: jobRunning.Match(out)}
	if m := jobPID.FindSubmatch(out); m != nil {
		j.pid, _ = strconv.Atoi(string(m[1]))
	}
	j.running = j.running || j.pid != 0
	return j, nil
}

// loaded reports whether launchd has loaded the service, as job says.
func (s *Service) loaded(ctx context.Context) (bool, error) {
	j, err := s.job(ctx)
	return j.loaded, err
}
