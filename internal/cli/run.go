package cli

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/launch"
)

// runOptions are the run command's flags.
type runOptions struct {
	account string
	direct  bool
}

func newRunCommand(a *app) *cobra.Command {
	var opts runOptions
	cmd := &cobra.Command{
		Use:   "run [--account <id>] [--direct] [-- <claude args>]",
		Short: "Start Claude Code through the router",
		Long: `Start Claude Code through the router, which sends its requests on the account
it chooses for them. Claude Code gets a token of its own too, for the requests
that don't go through the router: --account's, else the account the router
rates best, else the first with a usable token.

When the router isn't running, or isn't healthy, Claude Code connects directly
on that token instead, and run says so. When switchboard can't take part at
all, as without a config it can read or any account's usable token, Claude
Code starts as if switchboard weren't there, and run says why. --account pins
the session to an account, while it has room. --direct starts Claude Code on
its own login, without the router or a token, for what needs the login.

Give Claude Code's own arguments after --, as in: switchboard run -- --resume`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), cmd.ErrOrStderr(), opts, args)
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "pin the session to account `ID`")
	cmd.Flags().BoolVar(&opts.direct, "direct", false, "start Claude Code on its own login, without the router or a token")
	return cmd
}

// parseArgs checks the flags go together, and that Claude Code's arguments
// come after --. None is echoed back: they can hold a prompt.
func (o *runOptions) parseArgs(cmd *cobra.Command, args []string) error {
	switch {
	case cmd.Flags().Changed("account") && o.account == "":
		return errors.New("--account takes the id of an account")
	case o.account != "" && o.direct:
		return errors.New("--direct starts Claude Code on its own login, so it takes no --account")
	case len(args) > 0 && cmd.ArgsLenAtDash() != 0:
		return errors.New("give Claude Code's arguments after --, as in: switchboard run -- --resume")
	}
	return nil
}

// run starts Claude Code with args in this process's place: through
// switchboard, or, when it can't read its config or locate the state
// directory the router's socket is in, as if switchboard weren't there.
func (a *app) run(ctx context.Context, stderr io.Writer, opts runOptions, args []string) error {
	// Without a home directory, only the install paths outside it are tried.
	home, _ := a.HomeDir()
	l := launch.Launcher{
		Environ:      a.Environ(),
		LookPath:     a.LookPath,
		InstallPaths: claude.InstallPaths(home),
		Exec:         a.Exec,
		Stderr:       stderr,
	}
	if opts.direct {
		return l.Direct(args)
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return l.Unaided(args, "couldn't read the config", err)
	}
	client, err := a.routerClient()
	if err != nil {
		return l.Unaided(args, "couldn't find the router", err)
	}
	return l.Run(ctx, launch.Route{Config: cfg, Token: a.readToken, Router: client, Account: opts.account}, args)
}
