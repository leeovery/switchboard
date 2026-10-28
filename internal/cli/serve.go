package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/handover"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/views"
)

func newServeCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the router in the foreground",
		Long: `Run the router in the foreground, until interrupted or terminated. Claude Code
sends its requests to it through ANTHROPIC_BASE_URL, and it sends each on to
the API on the account chosen for it. The service normally runs it.

It takes up a change to a token file as it comes. Run by the service, it
restarts itself once its config file makes another valid config, an
upgrade replaces it, or the Mac's time zone changes, replacing itself with
its binary in place and handing its listeners over, so no request is
refused; run by hand, it logs that a restart is due.

It logs to the router's log, and to the terminal when it runs in one.
--log-level overrides SWITCHBOARD_LOG_LEVEL.`,
		Args:        noArgs,
		Annotations: map[string]string{roleAnnotation: string(logs.RoleRouter)},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.checking {
				return a.check(cmd)
			}
			a.trimLaunchdLog(cmd.OutOrStdout(), cmd.ErrOrStderr())
			return a.serve(cmd.Context())
		},
	}
	cmd.Flags().Var(levelFlag{&a.logLevel}, "log-level",
		"log at `LEVEL` and above: debug, info, warn or error (default $SWITCHBOARD_LOG_LEVEL, else info)")
	cmd.Flags().BoolVar(&a.checking, handover.CheckFlag, false, "check what the router would start from, the config above all, and exit")
	_ = cmd.Flags().MarkHidden(handover.CheckFlag)
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		err = hideTokens(cmd, err)
		// The command line is read up to a flag it can't take, as one the
		// router was started with that this version no longer takes, and the
		// router asking puts the check's own last, so it's looked for in the
		// command line itself.
		if slices.Contains(a.Args, "--"+handover.CheckFlag) {
			return refuse(cmd, err)
		}
		return err
	})
	return cmd
}

// serve runs the router on the config's accounts until it's interrupted or
// terminated, or restarts itself, having made the tokens directory private
// and brought the skill up to date. It finds its config file, its binary and
// the time zone's file as they stand before it reads the config, which the
// router compares them with while it runs, so a change made as it starts
// calls for a restart too.
func (a *app) serve(ctx context.Context) error {
	path, err := a.configFile()
	if err != nil {
		return err
	}
	configFile, binary, zone := router.Watch(path), router.Watch(a.binary()), router.Watch(a.ZoneFile)
	stateDir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	cfg, files, err := a.startingFrom(stateDir, path)
	if err != nil {
		return err
	}
	cfg.ConfigFile, cfg.Binary, cfg.Zone = configFile, binary, zone
	tighten(files)
	a.refreshSkill()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return router.Run(ctx, cfg)
}

// check checks what serve would start the router from, as a router about to
// restart into this binary asks with handover.CheckFlag, and answers as
// handover.Ask reads the answer: nothing, when the router could start, else
// why it couldn't, refusing.
func (a *app) check(cmd *cobra.Command) error {
	if _, err := a.startable(); err != nil {
		return refuse(cmd, err)
	}
	return nil
}

// startable returns what serve would start the router from, failing, saying
// why, where serve would fail before the router listens: where its config
// file can't be found, or the router couldn't start from what it's read from,
// as router.Startable says.
func (a *app) startable() (router.Config, error) {
	path, err := a.configFile()
	if err != nil {
		return router.Config{}, err
	}
	stateDir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return router.Config{}, err
	}
	return a.startableAt(service.Start{StateDir: stateDir, ConfigFile: path})
}

// startableAt returns what serve would start a router from, as start says,
// as the service would run it, failing, saying why, as startable does, and
// where serve couldn't parse its flags.
func (a *app) startableAt(start service.Start) (router.Config, error) {
	if start.LogLevel != "" {
		if _, err := logs.ParseLevel(start.LogLevel); err != nil {
			return router.Config{}, fmt.Errorf("--log-level: %w", err)
		}
	}
	cfg, _, err := a.startingFrom(start.StateDir, start.ConfigFile)
	if err != nil {
		return router.Config{}, err
	}
	return cfg, router.Startable(cfg)
}

// startableIn fails, saying why, as startableAt does.
func (a *app) startableIn(start service.Start) error {
	_, err := a.startableAt(start)
	return err
}

// refuse answers a router asking with handover.CheckFlag that the router
// couldn't start, err saying why: on standard output alone, and by exiting
// handover.Refused, as Execute has a refusal exit.
func refuse(cmd *cobra.Command, err error) error {
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	// The exit status refuses whether or not why is written.
	_, _ = io.WriteString(cmd.OutOrStdout(), err.Error()+"\n")
	// The router asking says why at warn, so the log of the check notes it
	// only at debug.
	return expected{refusal{err}}
}

// refusal is serve's check refusing, as refuse answers.
type refusal struct {
	error
}

func (r refusal) Unwrap() error {
	return r.error
}

// startingFrom reads what serve starts the router from, given its state
// directory and its config file's path: the router's config, but for the
// files it watches, which serve finds before it reads the config file, and
// the token files it reads. It fails where the config file doesn't make a
// valid config.
func (a *app) startingFrom(stateDir, path string) (router.Config, tokens.Store, error) {
	cfg, err := a.loadConfigAt(path)
	if err != nil {
		return router.Config{}, tokens.Store{}, err
	}
	files := tokens.NewStore(stateDir, a.UID)
	return router.Config{
		Accounts:      cfg.Accounts,
		Token:         files.Read,
		Taken:         files.Taken,
		Upstream:      cfg.Upstream,
		Provider:      claude.Provider{},
		Prober:        installedProber{upstream: cfg.Upstream, version: a.ClaudeVersion},
		Policy:        claude.Policy(),
		Prime:         cfg.Prime,
		Now:           a.Now,
		Version:       a.Version,
		Notifier:      a.Notifier,
		Notifications: cfg.Notifications,
		History:       cfg.History,
		Ledger:        cfg.Ledger,
		Digester:      views.Digests{},
		Listen:        cfg.Listen,
		StateDir:      stateDir,
		Supervised:    a.Getenv(launchdJob) == service.Label,
		Exec:          a.Exec,
		Check:         a.Check,
		Args:          a.Args,
		Environ:       a.Environ(),
		Handed:        a.Getenv(handover.Variable),
		WatchEvery:    a.WatchEvery,
	}, files, nil
}

// launchdJob is the variable launchd sets to the label of the job it runs a
// program as: the service's, when it runs the router.
const launchdJob = "XPC_SERVICE_NAME"

// trimLaunchdLog trims launchd's log as the router starts, as logs.TrimOutput
// does, where held, the router's output and its errors, are it, as launchd
// makes them when the service runs the router: nothing else trims it. A
// router run by hand, its output elsewhere, leaves it alone, and one without
// a state directory fails to serve, saying why.
func (a *app) trimLaunchdLog(held ...io.Writer) {
	stateDir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return
	}
	path := service.LogIn(stateDir)
	if err := logs.TrimOutput(path, held...); err != nil {
		logger.Warn("can't trim launchd's log", "path", path, "error", err)
	}
}

// binary returns this switchboard binary, by the path it was run by, which
// the router watches for an upgrade, or "" when it can't be found.
func (a *app) binary() string {
	exe, err := a.Executable()
	if err != nil {
		logger.Warn("can't find this switchboard binary, so an upgrade won't restart the router", "error", err)
		return ""
	}
	return exe
}

// tighten makes the tokens directory private, when it's there, as the router
// starts.
func tighten(files tokens.Store) {
	was, tightened, err := files.Tighten()
	switch {
	case err != nil:
		logger.Warn("can't make the tokens directory private", "path", files.Dir(), "error", err)
	case tightened:
		logger.Info("made the tokens directory private", "path", files.Dir(), "was", fmt.Sprintf("%04o", was))
	}
}

// refreshSkill brings the skill installed where Claude Code reads it up to
// date, as the router starts, and logs what it did: it installs none that
// setup didn't.
func (a *app) refreshSkill() {
	path, err := skill.Path(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Warn("can't find the skill to bring it up to date", "error", err)
		return
	}
	found, err := skill.Refresh(path)
	switch {
	case err != nil:
		logger.Warn("can't bring the skill up to date", "path", path, "error", err)
	case !found.Installed:
		logger.Info("no skill installed to bring up to date; setup writes it", "path", path)
	case found.Rewritten:
		logger.Info("brought the skill up to date", "path", path, "was", found.Was, "version", skill.Version())
	default:
		logger.Info("the skill is up to date", "path", path, "version", found.Was)
	}
}

// installedProber probes as the Claude Code installed at the time of each
// probe: the router runs for days, and outlives the version it started with.
type installedProber struct {
	upstream string
	version  func() string
}

func (p installedProber) Probe(ctx context.Context, token string) (quota.Probe, error) {
	prober := &claude.Prober{Upstream: p.upstream, Version: p.version()}
	return prober.Probe(ctx, token)
}

// levelFlag is --log-level: a level's name, which it sets level to.
type levelFlag struct {
	level *slog.Leveler
}

func (f levelFlag) String() string {
	if *f.level == nil {
		return ""
	}
	return strings.ToLower((*f.level).Level().String())
}

func (f levelFlag) Set(name string) error {
	level, err := logs.ParseLevel(name)
	if err != nil {
		return err
	}
	*f.level = level
	return nil
}

func (f levelFlag) Type() string {
	return "level"
}
