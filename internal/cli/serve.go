package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/tokens"
)

func newServeCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the router in the foreground",
		Long: `Run the router in the foreground, until interrupted or terminated. Claude Code
sends its requests to it through ANTHROPIC_BASE_URL, and it sends each on to
the API on the account chosen for it. The service normally runs it.

It takes up a change to a token file as it comes. Run by the service, it
restarts itself once its config file makes another valid config, or an
upgrade replaces it; run by hand, it logs that a restart is due.

It logs to the router's log, and to the terminal when it runs in one.
--log-level overrides SWITCHBOARD_LOG_LEVEL.`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{roleAnnotation: string(logs.RoleRouter)},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.serve(cmd.Context())
		},
	}
	cmd.Flags().Var(levelFlag{&a.logLevel}, "log-level",
		"log at `LEVEL` and above: debug, info, warn or error (default $SWITCHBOARD_LOG_LEVEL, else info)")
	return cmd
}

// serve runs the router on the config's accounts until it's interrupted or
// terminated, or restarts itself, having made the tokens directory private
// and brought the skill up to date.
func (a *app) serve(ctx context.Context) error {
	path, err := a.configFile()
	if err != nil {
		return err
	}
	cfg, err := a.loadConfigAt(path)
	if err != nil {
		return err
	}
	stateDir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	files := tokens.NewStore(stateDir, a.UID)
	tighten(files)
	a.refreshSkill()
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return router.Run(ctx, router.Config{
		Accounts:      cfg.Accounts,
		Token:         files.Read,
		Upstream:      cfg.Upstream,
		Provider:      claude.Provider{},
		Prober:        installedProber{upstream: cfg.Upstream, version: a.ClaudeVersion},
		Policy:        policy,
		Prime:         cfg.Prime,
		Now:           a.Now,
		Version:       a.Version,
		Notifier:      a.Notifier,
		Notifications: cfg.Notifications,
		Listen:        cfg.Listen,
		StateDir:      stateDir,
		ConfigFile:    path,
		Binary:        a.binary(),
		Supervised:    a.Getenv(launchdJob) == service.Label,
		WatchEvery:    a.WatchEvery,
	})
}

// launchdJob is the variable launchd sets to the label of the job it runs a
// program as: the service's, when it runs the router.
const launchdJob = "XPC_SERVICE_NAME"

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
