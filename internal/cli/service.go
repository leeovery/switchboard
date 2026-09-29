package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
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
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServiceInstallCommand(a), newServiceUninstallCommand(a), newServiceRestartCommand(a), newServiceStatusCommand(a))
	return cmd
}

func newServiceInstallCommand(a *app) *cobra.Command {
	var envFile string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the LaunchAgent, which starts the router",
		Long: `Install the LaunchAgent, which starts the router now, at every login, and
whenever it stops. It runs this switchboard binary, so install that with go
install first. The router serves the config --config gives, else the one it
finds, as the CLI does.

A LaunchAgent doesn't see the shell's environment, where the accounts' tokens
are. --env-file names a file the shell sources for them: zsh sources it too,
each time the router starts. After the tokens change, switchboard service
restart picks them up.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.installService(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), envFile)
		},
	}
	cmd.Flags().StringVar(&envFile, "env-file", "", "have zsh source `FILE`, which sets the accounts' tokens, before the router starts")
	return cmd
}

func newServiceUninstallCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the router, and remove the LaunchAgent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.uninstallService(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func newServiceRestartCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the router, as after the tokens change",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.restartService(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func newServiceStatusCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the LaunchAgent is installed and loaded, and the router's health",
		Args:  cobra.NoArgs,
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

// installService installs the LaunchAgent to run this switchboard binary,
// and says how the router it starts answers.
func (a *app) installService(ctx context.Context, out, errOut io.Writer, envFile string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	exe, err := a.Executable()
	if err != nil {
		return fmt.Errorf("find this switchboard binary: %w", err)
	}
	installed, err := svc.Install(ctx, service.InstallOptions{Executable: exe, EnvFile: envFile, Config: a.configPath})
	if err != nil {
		return err
	}
	for _, warning := range installed.Warnings {
		if _, err := fmt.Fprintln(errOut, "warning: "+warning); err != nil {
			return err
		}
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

// restartService has launchd restart the router, and says how it answers.
func (a *app) restartService(ctx context.Context, out io.Writer) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	h, err := svc.Restart(ctx)
	if errors.Is(err, service.ErrNotLoaded) {
		return fmt.Errorf("%w: install it with switchboard service install", err)
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
		answer = health(*st.Router)
	case !errors.Is(st.RouterErr, router.ErrNotRunning):
		answer = "not answering: " + st.RouterErr.Error()
	}
	return writeTable(out, [][]string{{"LaunchAgent", installed}, {"launchd", loaded}, {"router", answer}})
}

// started says how the router launchd started answered, failing, with where
// to find out why, when it didn't.
func started(out io.Writer, svc *service.Service, h *router.Health) error {
	if h == nil {
		return fmt.Errorf("the router didn't answer within %v of starting: see why with switchboard logs router, and in %s", service.StartWait, svc.Log())
	}
	_, err := fmt.Fprintln(out, "the router is up: "+health(*h))
	return err
}

// health says how a router answered its health check.
func health(h router.Health) string {
	state := "healthy"
	if !h.OK {
		state = "unhealthy: " + h.Reason
	}
	return fmt.Sprintf("%s, pid %d", state, h.PID)
}
