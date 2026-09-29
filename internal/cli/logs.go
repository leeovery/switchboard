package cli

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/logs"
)

// defaultLines is how many lines logs shows unless told otherwise.
const defaultLines = 50

// logsOptions are the logs command's flags.
type logsOptions struct {
	lines  int
	follow bool
	path   bool
}

func newLogsCommand(a *app) *cobra.Command {
	var opts logsOptions
	cmd := &cobra.Command{
		Use:   "logs [router|cli]",
		Short: "Show the last lines of switchboard's log",
		Long: `Show the last lines of switchboard's log: the router's, or the one every other
command shares. Without one named, it's the router's once the router has logged,
else the CLI's.

SWITCHBOARD_LOG_LEVEL sets how much is logged: debug, info, warn or error. It's
info unless set.`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := a.logPath(args)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case opts.path:
				_, err := fmt.Fprintln(out, path)
				return err
			case opts.follow:
				ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
				defer stop()
				return logs.Follow(ctx, out, path, opts.lines, a.FollowEvery)
			default:
				return logs.Tail(out, path, opts.lines)
			}
		},
	}
	cmd.Flags().IntVarP(&opts.lines, "lines", "n", defaultLines, "show the last `N` lines")
	cmd.Flags().BoolVarP(&opts.follow, "follow", "f", false, "go on printing lines as they're logged, until interrupted")
	cmd.Flags().BoolVar(&opts.path, "path", false, "print the log's path, and nothing else")
	return cmd
}

// parseArgs checks the log named, if any, and the number of lines.
func (o *logsOptions) parseArgs(_ *cobra.Command, args []string) error {
	switch {
	case o.lines < 0:
		return fmt.Errorf("invalid -n %d: give a number of lines, 0 or more", o.lines)
	case len(args) > 1:
		return fmt.Errorf("give one log, router or cli, not %d", len(args))
	case len(args) == 1 && !isRole(args[0]):
		return fmt.Errorf("unknown log %q: give router or cli", args[0])
	}
	return nil
}

func isRole(name string) bool {
	return name == string(logs.RoleRouter) || name == string(logs.RoleCLI)
}

// logPath is the log the logs command shows: the one named, else the
// router's once the router has logged, else the CLI's.
func (a *app) logPath(args []string) (string, error) {
	dir, err := a.logDir()
	if err != nil {
		return "", err
	}
	if len(args) == 1 {
		return logs.Role(args[0]).Path(dir), nil
	}
	return logs.DefaultPath(dir), nil
}
