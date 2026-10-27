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

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/tokens"
)

var logger = logs.For("service")

// Label is the LaunchAgent's label: launchd knows the service by it, and its
// plist is named after it.
const Label = "io.github.leeovery.switchboard"

const (
	// StartWait is how long installing or restarting the service waits for
	// the router launchd starts to answer, once any router before it has had
	// the time launchd gives it to stop.
	StartWait = 5 * time.Second
	// startPoll is how often it asks.
	startPoll = 100 * time.Millisecond
	// bootstrapTries is how many times installing the service has launchd
	// load it before it gives up, and bootstrapPause how long it waits
	// between them.
	bootstrapTries = 5
	bootstrapPause = 500 * time.Millisecond
	// unloadPoll is how often booting the service out asks launchd whether
	// it has let the service go.
	unloadPoll = 250 * time.Millisecond
)

var (
	// ErrUnsupported is what New fails with on a system other than macOS.
	ErrUnsupported = errors.New("the service is macOS only for now: elsewhere, run switchboard serve under your system's service manager")
	// ErrNotLoaded is what Restart fails with when launchd hasn't loaded the
	// service.
	ErrNotLoaded = errors.New("the service isn't loaded")
)

// Router asks the router whether it's alive, and which it is, and has it
// restart: *router.Client is one.
type Router interface {
	Health(ctx context.Context) (router.Health, error)
	Restart(ctx context.Context) (router.Restart, error)
}

// Restarting is how the router restarts, once it has finished its requests
// in flight.
type Restarting int

const (
	// InPlace is the router replacing itself with its binary, in the process
	// it runs in, as it's asked to.
	InPlace Restarting = iota
	// ByLaunchd is the router stopping, for launchd to start it again: one
	// that can't replace itself, or one from before routers restarted when
	// asked, at a signal.
	ByLaunchd
)

// String says what follows the router finishing its requests in flight.
func (r Restarting) String() string {
	if r == InPlace {
		return "it restarts in place"
	}
	return "launchd starts it again"
}

// Config is what the service is managed with.
type Config struct {
	// GOOS is the operating system, as runtime.GOOS names it.
	GOOS string
	// UID is the user's id, in whose GUI session the service runs: the token
	// files must be theirs.
	UID int
	// Home is the user's home directory, where launchd looks for the plist.
	Home string
	// Getenv reads the environment the service is installed from.
	Getenv func(key string) string
	// StateDir is switchboard's state directory, where launchd's log goes,
	// and the router reads the accounts' tokens.
	StateDir string
	// Launchctl runs launchctl, and Plutil plutil, which reads a binary
	// plist back as XML.
	Launchctl, Plutil Runner
	// Router asks the router the service runs whether it's alive: the one
	// whose state is in StateDir.
	Router Router
	// RouterAt returns the router whose state is in the directory given, for
	// For to ask the one an environment kept puts elsewhere.
	RouterAt func(stateDir string) Router
}

// Service is the LaunchAgent.
type Service struct {
	cfg Config
	// home is the home directory the router runs in, where the environment
	// the service is seen in names one other than the user's.
	home string
	// emptyHome is whether the environment the service is seen in sets
	// HOME empty, as launchd passes it.
	emptyHome bool
	// also are the routers the service may run besides cfg.Router's, as one
	// seen in another environment keeps the one at the state directory it's
	// asked from, and base the service as it's asked about.
	also []Router
	base *Service
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
	return LogIn(s.cfg.StateDir)
}

// LogIn is where launchd writes what the router prints itself, as Log says,
// for the state directory given.
func LogIn(stateDir string) string {
	return filepath.Join(logs.Dir(stateDir), "launchd.log")
}

// InstallOptions say how the service runs the router.
type InstallOptions struct {
	// Executable is this switchboard binary, as os.Executable gives it.
	Executable string
	// Config is the config file the router serves, as --config gives it, or
	// "" for the one it finds itself.
	Config string
	// LogLevel is the level the router logs at, as serve's --log-level takes
	// it, or "" for the one SWITCHBOARD_LOG_LEVEL names.
	LogLevel string
	// Accounts are the config's accounts, whose tokens the router serves
	// with.
	Accounts config.Accounts
	// environment is what launchd is to set in the router's environment, as
	// Options reads it back from the plist in place, or Planned works it out:
	// nil for Install to work it out as Planned does. It's theirs to give, as
	// the variables decide where the router keeps its state, its tokens with
	// it.
	environment map[string]string
	// unread is why Planned couldn't read back the plist in place, whose
	// variables added by hand are then gone, nil where it could or there's
	// none.
	unread error
}

// Installed is what installing the service did.
type Installed struct {
	// Warnings are worth the user's attention, though they didn't stop the
	// install.
	Warnings []string
	// Router is the answer of the router launchd started, or nil when none
	// answered within StartWait.
	Router *router.Health
	// Unread is why the plist in place couldn't be read back, whose
	// variables added by hand are then gone, nil where it could or there was
	// none.
	Unread error
}

// Install writes the LaunchAgent's plist, has launchd load it, in place of
// the one it had loaded if it had, which starts the router, and waits for
// the router to answer. It refuses a switchboard binary that won't last,
// such as go run's.
func (s *Service) Install(ctx context.Context, opts InstallOptions) (Installed, error) {
	s, opts, err := s.Planned(ctx, opts)
	if err != nil {
		return Installed{}, err
	}
	a, err := s.describe(opts)
	if err != nil {
		return Installed{}, err
	}
	if err := s.write(a); err != nil {
		return Installed{}, err
	}
	before, i, _ := s.answering(ctx)
	if err := s.unload(ctx); err != nil {
		return Installed{}, err
	}
	if err := s.bootstrap(ctx); err != nil {
		return Installed{}, err
	}
	logger.Info("installed the service", "plist", s.Plist(), "program", strings.Join(a.Program, " "))
	// With none answering before, one that answers now is launchd's,
	// wherever it is; with one answering, it may be one run otherwise.
	waitFor := s.routers()
	if i >= 0 {
		waitFor = waitFor[:1]
	}
	h, at := s.waitForRouter(ctx, waitFor, before, StartWait)
	installed := Installed{Router: h, Unread: opts.unread}
	// Judged where the router answered, whose token files it reads.
	if s.seenAt(at).tokenless(opts.Accounts) {
		installed.Warnings = append(installed.Warnings, "no account has a usable token, so the router will have nothing to route to: switchboard accounts says why")
		logger.Warn("no account has a usable token")
	}
	return installed, nil
}

// describe describes the LaunchAgent Install writes for opts, with the
// switchboard binary as Binary finds it.
func (s *Service) describe(opts InstallOptions) (agent, error) {
	binary, err := s.Binary(opts.Executable)
	if err != nil {
		return agent{}, err
	}
	opts.Executable = binary
	return s.agent(opts)
}

// tokenless reports whether the router looks set to start with none of the
// accounts' tokens: none has a token file it can use.
func (s *Service) tokenless(accounts config.Accounts) bool {
	files := tokens.NewStore(s.cfg.StateDir, s.cfg.UID)
	return len(accounts) > 0 && !slices.ContainsFunc(accounts, func(a config.Account) bool {
		_, err := files.Read(a.ID)
		return err == nil
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

// Restart has the router restart, reading the config and the tokens afresh,
// and returns the answer of the router it becomes, or nil when none answers
// in time. A router that answers is asked to restart: it finishes the
// requests in flight, within router.StopTimeout, then replaces itself with
// its binary, in the process it runs in, or, as it says when it can't, exits
// for launchd to start it again. One that can't be asked, as one from before
// routers restarted when asked can't, is sent SIGTERM, and stops as it does
// at any signal, finishing the requests in flight within exitTimeout, as
// launchd would give it; launchd, keeping the service alive, then starts it
// again. Restart calls draining, saying how the router restarts, as it
// finishes its requests, before it waits. With none answering, there's
// nothing to finish, and launchd starts the service afresh at once. It fails
// with ErrNotLoaded when launchd hasn't loaded the service, and saying why
// when the router refuses to restart, as one run by hand does.
func (s *Service) Restart(ctx context.Context, draining func(Restarting)) (*router.Health, error) {
	loaded, err := s.loaded(ctx)
	switch {
	case err != nil:
		return nil, err
	case !loaded:
		return nil, ErrNotLoaded
	}
	answering, i, _ := s.answering(ctx)
	if i < 0 {
		return s.startAfresh(ctx)
	}
	rt := s.routers()[i]
	how, before, err := s.restart(ctx, answering, rt)
	if err != nil {
		return nil, err
	}
	draining(how)
	h, _ := s.waitForRouter(ctx, []Router{rt}, before, exitTimeout+StartWait)
	return h, nil
}

// restart has the router rt, answering, restart, and returns how, with the
// router whose replacement to wait for: the one that took the request, which
// may have restarted since it answered as answering, or, one that can't be
// asked, answering, which launchd starts again once SIGTERM stops it.
func (s *Service) restart(ctx context.Context, answering router.Health, rt Router) (Restarting, router.Health, error) {
	asked, err := rt.Restart(ctx)
	switch {
	case err == nil && asked.InPlace:
		logger.Info("restarting the router in place", "pid", asked.PID)
		return InPlace, asked.Health, nil
	case err == nil:
		logger.Info("restarting the router, which can't replace itself, for launchd to start again", "pid", asked.PID)
		return ByLaunchd, asked.Health, nil
	case !errors.Is(err, router.ErrNoRestart):
		return 0, router.Health{}, err
	}
	if killed := s.launchctl(ctx, "kill", "SIGTERM", s.target()); killed != nil {
		return 0, router.Health{}, killed
	}
	logger.Info("stopping the router, for launchd to start again", "pid", answering.PID, "error", err)
	return ByLaunchd, answering, nil
}

// startAfresh has launchd start the service afresh, stopping any process of
// it at once, and returns the answer of the router it starts, or nil when
// none answers within StartWait. None answered before, so any of the routers
// the service may run that answers now is the one launchd started.
func (s *Service) startAfresh(ctx context.Context) (*router.Health, error) {
	if err := s.launchctl(ctx, "kickstart", "-k", s.target()); err != nil {
		return nil, err
	}
	logger.Info("started the service afresh")
	h, _ := s.waitForRouter(ctx, s.routers(), router.Health{}, StartWait)
	return h, nil
}

// Status is how the service stands.
type Status struct {
	// Installed is whether its plist is in place, and Loaded whether launchd
	// has loaded it.
	Installed, Loaded bool
	// PID is the process id of the router launchd runs, as launchctl print
	// says, 0 when none runs.
	PID int
	// Router is the answer of the first router to answer its health check,
	// or nil when none did, and RouterErr then says why the one at its state
	// directory didn't. Elsewhere is whether the one that answered isn't
	// that one, but the one at the state directory the service is asked
	// from, so that which the service runs can't be told.
	Router    *router.Health
	RouterErr error
	Elsewhere bool
}

// Status reports whether the service is installed, whether launchd has
// loaded it, and how the router answers.
func (s *Service) Status(ctx context.Context) (Status, error) {
	_, err := os.Stat(s.Plist())
	st := Status{Installed: err == nil}
	j, err := s.job(ctx)
	if err != nil {
		return Status{}, err
	}
	st.Loaded, st.PID = j.loaded, j.pid
	if h, i, err := s.answering(ctx); i >= 0 {
		st.Router, st.Elsewhere = &h, i > 0
	} else {
		st.RouterErr = err
	}
	return st, nil
}

// Answered fails, saying where to find out why, when the router launchd
// started didn't answer: when Install or Restart found none answering.
func (s *Service) Answered(h *router.Health) error {
	if h == nil {
		return fmt.Errorf("the router didn't answer within %v of starting: see why with switchboard logs router, and in %s", StartWait, s.Log())
	}
	return nil
}

// Health says how a router answered its health check, such as "healthy, pid
// 4242".
func Health(h router.Health) string {
	state := "healthy"
	if !h.OK {
		state = "unhealthy: " + h.Reason
	}
	return fmt.Sprintf("%s, pid %d", state, h.PID)
}

// domain is the user's GUI session, which the service runs in.
func (s *Service) domain() string {
	return "gui/" + strconv.Itoa(s.cfg.UID)
}

// target is the service, as launchctl names it.
func (s *Service) target() string {
	return s.domain() + "/" + Label
}

// Binary returns the switchboard binary for launchd to run, and for the
// claude link to lead to: exe, absolute, by the path it was run by, so that
// a link, such as Homebrew's, stays the link an upgrade moves on, not the
// version it led to. It fails for a build that won't last, judged by where
// its links lead.
func (s *Service) Binary(exe string) (string, error) {
	path, err := filepath.Abs(exe)
	if err != nil {
		return "", fmt.Errorf("find this switchboard binary: %w", err)
	}
	built, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("find this switchboard binary: %w", err)
	}
	if temporary(built, cmp.Or(s.cfg.Getenv("TMPDIR"), "/tmp")) {
		return "", fmt.Errorf("this switchboard is a temporary build, %s, which won't last: use one that does, such as Homebrew's or one go install built", built)
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

// absolute returns path made absolute, or "" for "".
func absolute(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return filepath.Abs(path)
}

// write writes the agent's plist whole, where any link in its place leads,
// readable by all as launchd's are, and has logs make the directory launchd
// writes its log in, which launchd leaves to it.
func (s *Service) write(a agent) error {
	data, err := a.plist()
	if err != nil {
		return fmt.Errorf("describe the service: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Plist()), 0o755); err != nil {
		return fmt.Errorf("create the LaunchAgents directory: %w", err)
	}
	target, err := atomicfile.Target(s.Plist())
	if err != nil {
		return fmt.Errorf("find where the service's plist goes: %w", err)
	}
	if err := atomicfile.Write(target, data, 0o644); err != nil {
		return fmt.Errorf("write the service's plist: %w", err)
	}
	return logs.MakeDir(filepath.Dir(a.Log))
}

// bootstrap has launchd load the service from its plist, trying again,
// bootstrapPause apart, bootstrapTries times in all, while launchctl
// fails: launchd refuses to load a service it hasn't finished letting go, as
// with "5: Input/output error". It fails as the last try did.
func (s *Service) bootstrap(ctx context.Context) error {
	for try := 1; ; try++ {
		err := s.launchctl(ctx, "bootstrap", s.domain(), s.Plist())
		if err == nil || try == bootstrapTries {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(bootstrapPause):
		}
		logger.Info("launchd didn't load the service; trying again", "try", try, "error", err)
	}
}

// unload has launchd stop the service and forget it, when it has loaded it,
// and waits until it has: launchctl bootout can return at once, as on macOS
// 26, while launchd keeps the service until its router has finished its
// requests in flight. It gives up bootoutTimeout after booting it out.
func (s *Service) unload(ctx context.Context) error {
	loaded, err := s.loaded(ctx)
	if err != nil || !loaded {
		return err
	}
	ctx, cancel := context.WithTimeoutCause(ctx, bootoutTimeout, fmt.Errorf("launchd didn't let the service go within %v of booting it out", bootoutTimeout))
	defer cancel()
	if err := s.launchctlWithin(ctx, bootoutTimeout, "bootout", s.target()); err != nil {
		return err
	}
	for {
		loaded, err := s.loaded(ctx)
		switch {
		case err == nil && !loaded:
			return nil
		case ctx.Err() != nil:
			return context.Cause(ctx)
		case err != nil:
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-time.After(unloadPoll):
		}
	}
}

// Answering is the answer of the router the service runs, if one answers,
// else nil.
func (s *Service) Answering(ctx context.Context) *router.Health {
	if h, i, _ := s.answering(ctx); i >= 0 {
		return &h
	}
	return nil
}

// answering is the answer of the first of the routers the service may run
// that answers, with its index among them, or, when none does, -1, and why
// the first didn't.
func (s *Service) answering(ctx context.Context) (router.Health, int, error) {
	return answeringOf(ctx, s.routers())
}

// answeringOf is the answer of the first of routers that answers, with its
// index, or, when none does, -1, and why the first didn't.
func answeringOf(ctx context.Context, routers []Router) (router.Health, int, error) {
	var first error
	for i, rt := range routers {
		h, err := rt.Health(ctx)
		if err == nil {
			return h, i, nil
		}
		if i == 0 {
			first = err
		}
	}
	return router.Health{}, -1, first
}

// routers are those the service may run: the one at its state directory,
// then those For keeps.
func (s *Service) routers() []Router {
	return append([]Router{s.cfg.Router}, s.also...)
}

// waitForRouter waits up to wait for the first of routers to answer as
// another router than before, which may not have stopped yet, and returns
// its answer, with its index among them, or nil and -1 when none does: no
// other router stands in for them.
func (s *Service) waitForRouter(ctx context.Context, routers []Router, before router.Health, wait time.Duration) (*router.Health, int) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		if h, i, _ := answeringOf(ctx, routers); i >= 0 && !h.Same(before) {
			logger.Info("the router answered", "pid", h.PID, "ok", h.OK)
			return &h, i
		}
		select {
		case <-ctx.Done():
			logger.Warn("the router didn't answer", "within", wait)
			return nil, -1
		case <-time.After(startPoll):
		}
	}
}
