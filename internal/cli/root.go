// Package cli is switchboard's command line. Commands stay thin: they parse
// flags, call the packages that do the work, and print.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// Deps is what the commands take from the process around them. main passes the
// real ones; tests pass their own.
type Deps struct {
	Version string
	Getenv  func(key string) string
	// Environ lists the whole environment, as os.Environ does: the one run
	// starts Claude Code in, and where the dashboard reads which colours the
	// terminal shows.
	Environ func() []string
	HomeDir func() (string, error)
	// Executable returns the path of this switchboard binary, as
	// os.Executable does: what the LaunchAgent runs, and what run passes
	// over as it looks for claude.
	Executable func() (string, error)
	Now        func() time.Time
	// ClaudeVersion returns the Claude Code version that probes claim to be.
	// Every read asks it, as a watch can outlive the version it started with.
	ClaudeVersion func() string
	// Watch shows the dashboard full screen on out until the user quits, as
	// watch.Run does.
	Watch func(ctx context.Context, cfg watch.Config, out io.Writer, environ []string) error
	// Notifier posts desktop notifications, as notify.Desktop does.
	Notifier Notifier
	// FollowEvery is how often logs --follow looks for new lines. Zero means
	// every half second.
	FollowEvery time.Duration
	// Exec replaces this process with the program at path, as launch.Exec
	// does. It returns only when it fails.
	Exec func(path string, argv, env []string) error
	// Launchctl runs launchctl, as service.Launchctl does.
	Launchctl service.Runner
	// GOOS is the operating system, as runtime.GOOS names it, and UID the
	// user's id: the token files must be theirs.
	GOOS string
	UID  int
}

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(message string) error
}

// policy is Claude's say in scoring accounts.
var policy = score.Policy{Shared: claude.SharedWindows, Perishable: claude.PerishableWindow, Tiebreak: claude.TiebreakWindow}

// roleAnnotation is the annotation a command logs its role under, when it
// isn't the CLI: serve runs as the router.
const roleAnnotation = "log-role"

var logger = logs.For("cli")

// NewRootCommand builds the switchboard command tree.
func NewRootCommand(deps Deps) *cobra.Command {
	a := &app{Deps: deps}
	root := &cobra.Command{
		Use:     "switchboard",
		Short:   "Spread Claude Code sessions across several Claude subscriptions",
		Version: deps.Version,
		// Flags and arguments have parsed by the time this runs, so usage still
		// follows a mistyped command line but not a failure after it.
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			cmd.SilenceUsage = true
			a.startLog(cmd)
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", "",
		"config file (default $SWITCHBOARD_CONFIG, else $XDG_CONFIG_HOME/switchboard/config.toml, else ~/.config/switchboard/config.toml)")
	root.AddCommand(newAccountsCommand(a), newStatusCommand(a), newUsageCommand(a), newLogsCommand(a), newServeCommand(a), newPinCommand(a),
		newRunCommand(a), newServiceCommand(a))
	return root
}

// Args returns the arguments a command tree from NewRootCommand parses for a
// process started as argv, as os.Args gives it: those after the program's
// name, or, run by the name claude, as through the claude link, run's, every
// one of them Claude Code's own, so claude --help is Claude Code's.
func Args(argv []string) []string {
	if len(argv) == 0 {
		// Never nil, for which Cobra parses os.Args itself.
		return []string{}
	}
	if filepath.Base(argv[0]) != claude.Command {
		return argv[1:]
	}
	return append([]string{"run", "--"}, argv[1:]...)
}

// Execute runs a command tree from NewRootCommand and returns the process's
// exit status. Cobra has already printed any error; the log notes it too, as
// a warning unless it's expected.
func Execute(root *cobra.Command) int {
	status := 0
	if cmd, err := root.ExecuteC(); err != nil {
		logger.Log(context.Background(), failureLevel(err), "command failed", "command", cmd.CommandPath(), "error", err)
		status = 1
	}
	logs.Close(status)
	return status
}

// expected is a failure that's an everyday answer rather than trouble, such
// as a statusline polling after a session the router isn't running for. The
// command fails as any other does, but the log notes it at debug, so a
// statusline asking every few seconds doesn't fill it with warnings.
type expected struct {
	error
}

func (e expected) Unwrap() error {
	return e.error
}

// failureLevel is the level a command's failure is logged at.
func failureLevel(err error) slog.Level {
	if _, ok := errors.AsType[expected](err); ok {
		return slog.LevelDebug
	}
	return slog.LevelWarn
}

// app is what every command shares: the dependencies and the flags.
type app struct {
	Deps
	configPath string
	// logLevel is the level serve's --log-level gives, or nil to log at the
	// level SWITCHBOARD_LOG_LEVEL names.
	logLevel slog.Leveler
}

// startLog starts this invocation's log, in the role the command plays: the
// CLI's unless its annotation says otherwise. A command never fails for its
// log, so without a state directory it logs nowhere.
func (a *app) startLog(cmd *cobra.Command) {
	dir, _ := a.logDir()
	role := logs.Role(cmd.Annotations[roleAnnotation])
	logs.Init(logs.Options{
		Dir:     dir,
		Role:    role,
		Level:   a.logLevel,
		Getenv:  a.Getenv,
		Mirror:  mirror(cmd, role),
		Version: a.Version,
		Command: cmd.CommandPath(),
	})
}

// mirror is where the router's records go as well as its log: the terminal it
// runs in, if it runs in one. Other commands keep stderr for their own output.
func mirror(cmd *cobra.Command, role logs.Role) io.Writer {
	if role != logs.RoleRouter {
		return nil
	}
	if f, ok := cmd.ErrOrStderr().(term.File); ok && term.IsTerminal(f.Fd()) {
		return f
	}
	return nil
}

// logDir is where the logs live, in the state directory.
func (a *app) logDir() (string, error) {
	state, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return "", err
	}
	return logs.Dir(state), nil
}

// tokens returns the accounts' token files, in the state directory.
func (a *app) tokens() (tokens.Store, error) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return tokens.Store{}, err
	}
	return tokens.NewStore(dir, a.UID), nil
}

// readToken reads the token of the account with the given id from its file,
// failing as locating the state directory does, when it does.
func (a *app) readToken(id string) (tokens.Token, error) {
	files, err := a.tokens()
	if err != nil {
		return tokens.Token{}, err
	}
	return files.Read(id)
}

// errRouterDown is what a command that needs the router fails with when the
// router isn't running, saying how to start it.
var errRouterDown = fmt.Errorf("%w: start it with switchboard service install (or switchboard serve)", router.ErrNotRunning)

// routerClient returns a client of the router whose control socket is in the
// state directory.
func (a *app) routerClient() (*router.Client, error) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, err
	}
	return router.NewClient(router.SocketPath(dir)), nil
}

// fromRouter is err, a router client's, saying plainly when the router isn't
// running.
func fromRouter(err error) error {
	if errors.Is(err, router.ErrNotRunning) {
		return errRouterDown
	}
	return err
}

// loadConfig loads the config file named by --config, else the one config.Path finds.
func (a *app) loadConfig() (*config.Config, error) {
	path, err := a.configFile()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		example := strings.TrimSuffix(config.Example, "\n")
		return nil, fmt.Errorf("no config file at %s\n\nCreate one like this:\n\n%s", path, example)
	}
	if err != nil {
		return nil, err
	}
	logger.Debug("loaded config", "path", path, "accounts", len(cfg.Accounts))
	return cfg, nil
}

func (a *app) configFile() (string, error) {
	if a.configPath != "" {
		return a.configPath, nil
	}
	return config.Path(a.Getenv, a.HomeDir)
}

// collect loads the config and reads the status document status and usage
// print, from the source that decides where it comes from, probing alone
// when probe says so.
func (a *app) collect(ctx context.Context, probe bool) (status.Document, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return status.Document{}, err
	}
	return a.source(cfg, probe).Read(ctx, watch.Read{Probe: true})
}

// source returns where the status document is read for the config given: the
// router, unless probe says to probe alone, else probing every account in the
// config. Every command that reports usage reads it here, so they all read it
// the same way.
func (a *app) source(cfg *config.Config, probe bool) usageSource {
	source := usageSource{probe: probeSource{deps: a.Deps, readToken: a.readToken, upstream: cfg.Upstream, accounts: cfg.Accounts}, ask: !probe}
	if source.ask {
		// Without a state directory there's no socket to find the router at,
		// which reads as a router that isn't running.
		var err error
		if source.router, err = a.routerClient(); err != nil {
			logger.Debug("can't find the router", "error", err)
		}
	}
	return source
}
