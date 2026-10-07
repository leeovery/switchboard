package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

func newServiceCommand(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the LaunchAgent that keeps the router running",
		Long: `Manage the LaunchAgent that keeps the router running on macOS: launchd starts
switchboard serve at login, and again whenever it stops.`,
		// Runnable, so a mistyped subcommand is an error rather than help.
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceInstallCommand(a), newServiceUninstallCommand(a), newServiceRestartCommand(a), newServiceStatusCommand(a))
	return cmd
}

func newServiceInstallCommand(a *app) *cobra.Command {
	var logLevel slog.Leveler
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the LaunchAgent, which starts the router",
		Long: `Install the LaunchAgent, which starts the router now, at every login, and
whenever it stops. It runs this switchboard binary, which must be one that
lasts, such as Homebrew's or one go install built: a temporary build, such as
go run's, is refused. The router serves the config --config gives, else the
one it finds, as the CLI does; a config the router couldn't serve fails the
install. It logs at the level --log-level gives, else SWITCHBOARD_LOG_LEVEL's.

The router reads the accounts' tokens from their files, as the CLI does, so it
needs none of the shell's environment. Install warns when no account has a
usable token, as the router would have nothing to route to.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := service.InstallOptions{LogLevel: levelFlag{&logLevel}.String()}
			return a.installService(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), opts)
		},
	}
	cmd.Flags().Var(levelFlag{&logLevel}, "log-level",
		"have the router log at `LEVEL` and above: debug, info, warn or error (default $SWITCHBOARD_LOG_LEVEL, else info)")
	return cmd
}

func newServiceUninstallCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the router, and remove the LaunchAgent",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.uninstallService(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func newServiceRestartCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the router, which reads the config and the tokens afresh",
		Long: `Restart the router, which reads the config and the tokens afresh. It gives the
requests in flight up to 30 seconds to finish, then replaces itself with its
binary, in place, handing its listeners over, so no request is refused;
restart waits for the new one to answer. A router from before routers
restarted when asked stops as it does at a signal, and launchd starts it
again. With no router answering, there's nothing to finish, and launchd
starts the service afresh at once.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.restartService(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func newServiceStatusCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the LaunchAgent is installed and loaded, and the router's health",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.serviceStatus(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

// service returns the LaunchAgent, whose router's control socket is in the
// state directory.
func (a *app) service() (*service.Service, error) {
	home, err := a.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate the home directory: %w", err)
	}
	stateDir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, err
	}
	return service.New(service.Config{
		GOOS:      a.GOOS,
		UID:       a.UID,
		Home:      home,
		Getenv:    a.Getenv,
		StateDir:  stateDir,
		Launchctl: a.Launchctl,
		Router:    router.NewClient(router.SocketPath(stateDir)),
	})
}

// installService installs the LaunchAgent to run this switchboard binary as
// opts says, serving the config the CLI finds, once it has read the config
// as the router would, and says how the router it starts answers.
func (a *app) installService(ctx context.Context, out, errOut io.Writer, opts service.InstallOptions) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	if opts.Executable, err = a.Executable(); err != nil {
		return fmt.Errorf("find this switchboard binary: %w", err)
	}
	opts.Config, opts.Accounts = a.configPath, cfg.Accounts
	installed, err := svc.Install(ctx, opts)
	if err != nil {
		return err
	}
	for _, warning := range installed.Warnings {
		launch.Notice(errOut, warning)
	}
	if _, err := fmt.Fprintln(out, "installed "+svc.Plist()); err != nil {
		return err
	}
	return started(out, svc, installed.Router)
}

// uninstallService stops the router and removes the LaunchAgent.
func (a *app) uninstallService(ctx context.Context, out io.Writer) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	removed, err := svc.Uninstall(ctx)
	if err != nil {
		return err
	}
	said := "the service isn't installed"
	if removed {
		said = "uninstalled: removed " + svc.Plist()
	}
	_, err = fmt.Fprintln(out, said)
	return err
}

// restartService restarts the router, saying how while a router finishes its
// requests in flight first, and says how the one it becomes answers.
func (a *app) restartService(ctx context.Context, out io.Writer) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	h, err := svc.Restart(ctx, func(how service.Restarting) {
		_, _ = fmt.Fprintln(out, "the router is finishing its requests in flight, then "+how.String())
	})
	if errors.Is(err, service.ErrNotLoaded) {
		return fmt.Errorf("%w: run switchboard service install", err)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "restarted"); err != nil {
		return err
	}
	return started(out, svc, h)
}

// serviceStatus prints whether the LaunchAgent is installed and loaded, and
// how the router answers.
func (a *app) serviceStatus(ctx context.Context, out io.Writer) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	st, err := svc.Status(ctx)
	if err != nil {
		return err
	}
	installed, loaded := "not installed", "not loaded"
	if st.Installed {
		installed = svc.Plist()
	}
	if st.Loaded {
		loaded = "loaded"
	}
	answer := "not running"
	switch {
	case st.Router != nil:
		answer = service.Health(*st.Router)
	case !errors.Is(st.RouterErr, router.ErrNotRunning):
		answer = "not answering: " + st.RouterErr.Error()
	}
	return writeTable(out, [][]string{{"LaunchAgent", installed}, {"launchd", loaded}, {"router", answer}})
}

// started says how the router launchd started answered, failing, with where
// to find out why, when it didn't.
func started(out io.Writer, svc *service.Service, h *router.Health) error {
	if err := svc.Answered(h); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "the router is up: "+service.Health(*h))
	return err
}
