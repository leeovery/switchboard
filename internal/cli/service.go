package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/redact"
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
restart waits for the new one to answer. The router first asks its binary
whether it could start, and refuses, saying why, where the binary refuses,
as a newer version can refuse a config the running one takes: put the config
right as it says. A router from before routers restarted when asked stops as
it does at a signal, and launchd starts it again. With no router answering,
there's nothing to finish, and launchd starts the service afresh at once.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.restartService(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func newServiceStatusCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the LaunchAgent is installed and loaded, and the router's health",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.serviceStatus(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr())
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
		Plutil:    a.Plutil,
		Router:    router.NewClient(router.SocketPath(stateDir)),
		RouterAt: func(dir string) service.Router {
			return router.NewClient(router.SocketPath(dir))
		},
	})
}

// installedService returns the LaunchAgent as the router it runs sees it,
// in the environment its plist keeps, where its state directory, and its
// router, may be other than this shell's; or, where there's no plist, as
// service sees it, and where it can't be read back, so too, saying so on
// errOut.
func (a *app) installedService(ctx context.Context, errOut io.Writer) (*service.Service, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	opts, err := svc.Options(ctx)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return svc.Unplaced(), nil
	case err != nil:
		launch.Notice(errOut, "the service's plist can't be read back, so the router is looked for where this shell keeps its state ("+redact.Text(err.Error())+")")
		return svc.Unplaced(), nil
	}
	installed, err := svc.For(opts)
	if err != nil {
		launch.Notice(errOut, "where the service's router keeps its state can't be worked out from its plist, so it's looked for where this shell keeps its state ("+redact.Text(err.Error())+")")
		return svc.Unplaced(), nil
	}
	return installed, nil
}

// installService installs the LaunchAgent to run this switchboard binary as
// opts says, serving the config given, else the one its router finds, once
// it has checked what the router would start from, as serve's check does, so
// a router that couldn't start never replaces one that runs, and says how
// the router it starts answers. It's seen as the router it installs sees it,
// in the environment it writes, which keeps the variables added to the plist
// in place by hand.
func (a *app) installService(ctx context.Context, out, errOut io.Writer, opts service.InstallOptions) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	opts.Config = a.configPath
	svc, opts, err = svc.Planned(ctx, opts)
	if err != nil {
		return err
	}
	start, err := svc.StartsFrom(opts)
	if err != nil {
		return err
	}
	cfg, err := a.startableAt(start)
	if err != nil {
		return err
	}
	if opts.Executable, err = a.Executable(); err != nil {
		return fmt.Errorf("find this switchboard binary: %w", err)
	}
	opts.Accounts = cfg.Accounts
	installed, err := svc.Install(ctx, opts)
	if err != nil {
		return err
	}
	if installed.Unread != nil {
		launch.Notice(errOut, "the plist in place couldn't be read back, so any variable added to it by hand is gone ("+redact.Text(installed.Unread.Error())+")")
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
func (a *app) restartService(ctx context.Context, out, errOut io.Writer) error {
	svc, err := a.installedService(ctx, errOut)
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
func (a *app) serviceStatus(ctx context.Context, out, errOut io.Writer) error {
	svc, err := a.installedService(ctx, errOut)
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
	switch {
	case st.PID != 0:
		loaded = fmt.Sprintf("loaded, running pid %d", st.PID)
	case st.Running:
		loaded = "loaded, running"
	case st.Loaded:
		loaded = "loaded, not running"
	}
	answer := "not running"
	switch {
	case st.Elsewhere:
		answer = service.Health(*st.Router) + ", where this shell keeps its state, not where the plist does"
	case st.Router != nil:
		answer = service.Health(*st.Router)
	case !router.IsNotRunning(st.RouterErr):
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
