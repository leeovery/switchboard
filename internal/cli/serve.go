package cli

import (
	"context"
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
)

func newServeCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the router in the foreground",
		Long: `Run the router in the foreground, until interrupted or terminated. Claude Code
sends its requests to it through ANTHROPIC_BASE_URL, and it sends each on to
the API on the account chosen for it. The service normally runs it.

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
// terminated.
func (a *app) serve(ctx context.Context) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	stateDir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return router.Run(ctx, router.Config{
		Accounts:      cfg.Accounts,
		Getenv:        a.Getenv,
		Upstream:      cfg.Upstream,
		Provider:      claude.Provider{},
		Prober:        installedProber{upstream: cfg.Upstream, version: a.ClaudeVersion},
		Policy:        policy,
		Now:           a.Now,
		Version:       a.Version,
		Notifier:      a.Notifier,
		Notifications: cfg.Notifications,
		Listen:        cfg.Listen,
		StateDir:      stateDir,
	})
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
