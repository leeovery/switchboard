// Package service manages the LaunchAgent that keeps the router running on
// macOS: launchd starts switchboard serve at login, and again whenever it
// stops.
package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
)

var logger = logs.For("service")

// Label is the LaunchAgent's label: launchd knows the service by it, and its
// plist is named after it.
const Label = "io.github.leeovery.switchboard"

const (
	// StartWait is how long installing or restarting the service waits for
	// the router launchd starts to answer.
	StartWait = 5 * time.Second
	// startPoll is how often it asks.
	startPoll = 100 * time.Millisecond
)

var (
	// ErrUnsupported is what New fails with on a system other than macOS.
	ErrUnsupported = errors.New("the service is macOS only for now: elsewhere, run switchboard serve under your system's service manager")
	// ErrNotInstalled is what Restart fails with when launchd hasn't loaded
	// the service.
	ErrNotInstalled = errors.New("the service isn't installed")
)

// Router asks the router whether it's alive, and which it is:
// *router.Client is one.
type Router interface {
	Health(ctx context.Context) (router.Health, error)
}

// Config is what the service is managed with.
type Config struct {
	// GOOS is the operating system, as runtime.GOOS names it.
	GOOS string
	// UID is the user's id, in whose GUI session the service runs.
	UID int
	// Home is the user's home directory, where launchd looks for the plist.
	Home string
	// Getenv reads the environment the service is installed from.
	Getenv func(key string) string
	// StateDir is switchboard's state directory, where launchd's log goes.
	StateDir string
	// Launchctl runs launchctl.
	Launchctl Runner
	// Router asks the router the service runs whether it's alive.
	Router Router
}

// Service is the LaunchAgent.
type Service struct {
	cfg Config
}

// New returns the LaunchAgent, failing with ErrUnsupported on a system other
// than macOS.
func New(cfg Config) (*Service, error) {
	if cfg.GOOS != "darwin" {
		return nil, ErrUnsupported
	}
	return &Service{cfg: cfg}, nil
}

// Plist is where the LaunchAgent's plist goes.
func (s *Service) Plist() string {
	return filepath.Join(s.cfg.Home, "Library", "LaunchAgents", Label+".plist")
}

// Log is where launchd writes what the router prints itself, such as a
// crash's output.
func (s *Service) Log() string {
	return filepath.Join(logs.Dir(s.cfg.StateDir), "launchd.log")
}

// InstallOptions say how the service runs the router.
type InstallOptions struct {
	// Executable is this switchboard binary, as os.Executable gives it.
	Executable string
	// EnvFile is a file for zsh to source before the router starts, which
	// sets the accounts' tokens: a LaunchAgent doesn't see the shell's
	// environment. "" for none.
	EnvFile string
	// Config is the config file the router serves, as --config gives it, or
	// "" for the one it finds itself.
	Config string
	// LogLevel is the level the router logs at, as serve's --log-level takes
	// it, or "" for the one SWITCHBOARD_LOG_LEVEL names.
	LogLevel string
	// Accounts are the config's accounts, whose tokens the router serves
	// with.
	Accounts config.Accounts
}

// Installed is what installing the service did.
type Installed struct {
	// Warnings are worth the user's attention, though they didn't stop the
	// install.
	Warnings []string
	// Router is the answer of the router launchd started, or nil when none
	// answered within StartWait.
	Router *router.Health
}

// Install writes the LaunchAgent's plist, has launchd load it, in place of
// the one it had loaded if it had, which starts the router, and waits for
// the router to answer. It refuses a switchboard binary that won't last,
// such as go run's, and an env file anyone else could write.
func (s *Service) Install(ctx context.Context, opts InstallOptions) (Installed, error) {
	a, warnings, err := s.prepare(opts)
	if err != nil {
		return Installed{}, err
	}
	if err := s.write(a); err != nil {
		return Installed{}, err
	}
	before := s.pid(ctx)
	if err := s.unload(ctx); err != nil {
		return Installed{}, err
	}
	if err := s.launchctl(ctx, "bootstrap", s.domain(), s.Plist()); err != nil {
		return Installed{}, err
	}
	logger.Info("installed the service", "plist", s.Plist(), "program", strings.Join(a.Program, " "))
	return Installed{Warnings: warnings, Router: s.waitForRouter(ctx, before)}, nil
}

// prepare checks what the service is to run, and describes the LaunchAgent
// that runs it, with what's worth the user's attention: an env file others
// can read, and the router having no token to serve with.
func (s *Service) prepare(opts InstallOptions) (agent, []string, error) {
	binary, err := s.binary(opts.Executable)
	if err != nil {
		return agent{}, nil, err
	}
	envFile, warning, err := s.checkEnvFile(opts.EnvFile)
	if err != nil {
		return agent{}, nil, err
	}
	var warnings []string
	if warning != "" {
		warnings = append(warnings, warning)
		logger.Warn("other users can read the env file", "path", envFile)
	}
	if s.tokenless(opts.Accounts, envFile) {
		warnings = append(warnings, "no account's token is set, and there's no --env-file to set them where the router runs: give --env-file a file that sets "+
			strings.Join(opts.Accounts.TokenEnvs(), " or "))
		logger.Warn("no account's token is set, and there's no env file")
	}
	opts.Executable, opts.EnvFile = binary, envFile
	a, err := s.agent(opts)
	return a, warnings, err
}

// tokenless reports whether the router looks set to start with none of the
// accounts' tokens: there's no env file to set them, and none is set where
// the service is installed either.
func (s *Service) tokenless(accounts config.Accounts, envFile string) bool {
	return envFile == "" && len(accounts) > 0 && !slices.ContainsFunc(accounts, func(a config.Account) bool {
		_, ok := a.Token(s.cfg.Getenv)
		return ok
	})
}

// Uninstall has launchd stop the router and forget the service, if it had
// loaded it, then removes its plist, reporting whether there was one. A
// service that isn't installed is left as it is.
func (s *Service) Uninstall(ctx context.Context) (removed bool, err error) {
	if err := s.unload(ctx); err != nil {
		return false, err
	}
	err = os.Remove(s.Plist())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("remove the service's plist: %w", err)
	}
	logger.Info("uninstalled the service", "plist", s.Plist())
	return true, nil
}

// Restart has launchd stop the router and start it again, as after the
// tokens change, and returns the answer of the router it starts, or nil when
// none answers within StartWait. It fails with ErrNotInstalled when launchd
// hasn't loaded the service.
func (s *Service) Restart(ctx context.Context) (*router.Health, error) {
	loaded, err := s.loaded(ctx)
	switch {
	case err != nil:
		return nil, err
	case !loaded:
		return nil, ErrNotInstalled
	}
	before := s.pid(ctx)
	if err := s.launchctl(ctx, "kickstart", "-k", s.target()); err != nil {
		return nil, err
	}
	logger.Info("restarted the service")
	return s.waitForRouter(ctx, before), nil
}

// Status is how the service stands.
type Status struct {
	// Installed is whether its plist is in place, and Loaded whether launchd
	// has loaded it.
	Installed, Loaded bool
	// Router is the router's answer to its health check, or nil when it
	// didn't answer, and RouterErr then says why.
	Router    *router.Health
	RouterErr error
}

// Status reports whether the service is installed, whether launchd has
// loaded it, and how the router answers.
func (s *Service) Status(ctx context.Context) (Status, error) {
	_, err := os.Stat(s.Plist())
	st := Status{Installed: err == nil}
	if st.Loaded, err = s.loaded(ctx); err != nil {
		return Status{}, err
	}
	if h, err := s.cfg.Router.Health(ctx); err == nil {
		st.Router = &h
	} else {
		st.RouterErr = err
	}
	return st, nil
}

// domain is the user's GUI session, which the service runs in.
func (s *Service) domain() string {
	return "gui/" + strconv.Itoa(s.cfg.UID)
}

// target is the service, as launchctl names it.
func (s *Service) target() string {
	return s.domain() + "/" + Label
}

// binary returns the switchboard binary for launchd to run: exe, absolute,
// by the path it was run by, so that a link, such as Homebrew's, stays the
// link an upgrade moves on, not the version it led to. It fails for a build
// that won't last, judged by where its links lead.
func (s *Service) binary(exe string) (string, error) {
	path, err := filepath.Abs(exe)
	if err != nil {
		return "", fmt.Errorf("find this switchboard binary: %w", err)
	}
	built, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("find this switchboard binary: %w", err)
	}
	if temporary(built, cmp.Or(s.cfg.Getenv("TMPDIR"), "/tmp")) {
		return "", fmt.Errorf("this switchboard is a temporary build, %s, which won't be there for launchd to start: install it with go install, and install the service with that one", built)
	}
	return path, nil
}

// temporary reports whether the binary at path, its symlinks resolved, is a
// build that won't last: one in a go-build directory, where go run builds,
// or in the temporary directory tmp.
func temporary(path, tmp string) bool {
	if slices.ContainsFunc(strings.Split(filepath.Dir(path), string(filepath.Separator)), isGoBuild) {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = resolved
	}
	rel, err := filepath.Rel(tmp, path)
	return err == nil && filepath.IsLocal(rel)
}

func isGoBuild(dir string) bool {
	return strings.HasPrefix(dir, "go-build")
}

// checkEnvFile returns the env file at path, absolute and its symlinks
// resolved, so what launchd sources is the file checked, wherever a link
// leads later. zsh runs what's in it with every token, so it refuses one
// anyone but the user could have written, and warns when others can read
// it, as it holds tokens. There's nothing to check for "".
func (s *Service) checkEnvFile(path string) (resolved, warning string, err error) {
	if path == "" {
		return "", "", nil
	}
	resolved, info, err := openEnvFile(path)
	if err != nil {
		return "", "", err
	}
	if err := s.onlyUserWrites(resolved, info); err != nil {
		return "", "", err
	}
	if info.Mode().Perm()&0o044 != 0 {
		warning = fmt.Sprintf("other users can read the env file %s (mode %04o), and it holds tokens: chmod 600 it", resolved, info.Mode().Perm())
	}
	return resolved, warning, nil
}

// openEnvFile finds the env file at path, which must be a file this user can
// read, and returns where it is, absolute and its symlinks resolved, and what
// it is.
func openEnvFile(path string) (string, fs.FileInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, fmt.Errorf("find the env file: %w", err)
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", nil, fmt.Errorf("read the env file: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("read the env file: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	switch {
	case err != nil:
		return "", nil, fmt.Errorf("find the env file: %w", err)
	case !info.Mode().IsRegular():
		return "", nil, fmt.Errorf("the env file %s isn't a file", resolved)
	}
	return resolved, info, nil
}

// onlyUserWrites fails for the env file at path, which info describes, when
// anyone but the user could write it: it's another user's, others can write
// it, or they can write its directory, and so put a file of their own in its
// place.
func (s *Service) onlyUserWrites(path string, info fs.FileInfo) error {
	if uid, ok := owner(info); !ok || uid != s.cfg.UID {
		return fmt.Errorf("the env file %s is another user's, and zsh would run what's in it with every token: use a file of your own", path)
	}
	if mode := info.Mode().Perm(); mode&0o022 != 0 {
		return fmt.Errorf("other users can write the env file %s (mode %04o), and zsh would run what they put in it with every token: chmod 600 it", path, mode)
	}
	dir := filepath.Dir(path)
	d, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("read the env file's directory: %w", err)
	}
	if mode := d.Mode().Perm(); mode&0o022 != 0 {
		return fmt.Errorf("other users can write the env file's directory %s (mode %04o), and put a file of their own in its place for zsh to run with every token: chmod go-w it", dir, mode)
	}
	return nil
}

// absolute returns path made absolute, or "" for "".
func absolute(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("find the config file: %w", err)
	}
	return abs, nil
}

// write writes the agent's plist, readable by all as launchd's are, and
// makes the directory launchd writes its log in, which launchd leaves to it.
func (s *Service) write(a agent) error {
	data, err := a.plist()
	if err != nil {
		return fmt.Errorf("describe the service: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Plist()), 0o755); err != nil {
		return fmt.Errorf("create the LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(s.Plist(), data, 0o644); err != nil {
		return fmt.Errorf("write the service's plist: %w", err)
	}
	if err := os.Chmod(s.Plist(), 0o644); err != nil {
		return fmt.Errorf("make the service's plist readable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(a.Log), 0o700); err != nil {
		return fmt.Errorf("create the log directory: %w", err)
	}
	return nil
}

// unload has launchd stop the service and forget it, when it has loaded it.
func (s *Service) unload(ctx context.Context) error {
	loaded, err := s.loaded(ctx)
	if err != nil || !loaded {
		return err
	}
	return s.launchctl(ctx, "bootout", s.target())
}

// pid is the process id of the router answering now, or 0 when none is.
func (s *Service) pid(ctx context.Context) int {
	h, err := s.cfg.Router.Health(ctx)
	if err != nil {
		return 0
	}
	return h.PID
}

// waitForRouter waits up to StartWait for a router to answer, other than the
// one whose process id is before, which launchd may not have stopped yet,
// and returns its answer, or nil when none does.
func (s *Service) waitForRouter(ctx context.Context, before int) *router.Health {
	ctx, cancel := context.WithTimeout(ctx, StartWait)
	defer cancel()
	for {
		if h, err := s.cfg.Router.Health(ctx); err == nil && h.PID != before {
			logger.Info("the router answered", "pid", h.PID, "ok", h.OK)
			return &h
		}
		select {
		case <-ctx.Done():
			logger.Warn("the router didn't answer", "within", StartWait)
			return nil
		case <-time.After(startPoll):
		}
	}
}
