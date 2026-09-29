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
		Long: `Start Claude Code through the router, which sends its conversation on the
account it chooses for each request. Claude Code's own token is the primary
account's, whichever account the conversation goes to, so what doesn't go
through the router, such as publishing an artifact, goes out on the primary.
--account pins the session's conversation to an account while that can take
it; the token stays the primary's.

When the router isn't running, or isn't healthy, Claude Code connects directly
on --account's token, else the primary's, else the first account's with a
usable token, and run says so. When switchboard can't take part at all, as
without a config it can read or any account's usable token, Claude Code
starts as if switchboard weren't there, and run says why. --direct starts
Claude Code on its own login, without the router or a token, for what needs
the login.

Claude Code's local subcommands, such as doctor, mcp and setup-token, start
as if switchboard weren't there, saying nothing: switchboard has no part in
them. Only the first of Claude Code's arguments names one, so -p doctor is a
prompt.

When ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN is set, Claude Code may use
that key in place of an account's token, so its requests wouldn't be routed,
but billed to the key: Claude Code starts as if switchboard weren't there, and
run says why.

Give Claude Code's own arguments after --, as in: switchboard run -- --resume

Run by the name claude, as through a link named claude ahead of the real one
on PATH, switchboard is switchboard run -- with every argument Claude Code's
own, so claude --help is Claude Code's.`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), cmd.ErrOrStderr(), opts, args)
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "pin the session's conversation to account `ID`")
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
// switchboard, or as if switchboard weren't there, for one of Claude Code's
// local subcommands, when its environment sets a key Claude Code may use in
// place of an account's token, or when it can't read its config or locate
// the state directory the router's socket is in.
func (a *app) run(ctx context.Context, stderr io.Writer, opts runOptions, args []string) error {
	// Without a home directory, only the install paths outside it are tried.
	home, _ := a.HomeDir()
	l := launch.Launcher{
		Environ:      a.Environ(),
		InstallPaths: claude.InstallPaths(home),
		Executable:   a.Executable,
		PID:          a.PID,
		Exec:         a.Exec,
		Stderr:       stderr,
	}
	switch {
	case opts.direct:
		return l.Direct(args)
	case claude.IsLocal(args):
		return l.Local(args)
	}
	if key := l.KeyEnv(); key != "" {
		return l.StepAside(args, key)
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
