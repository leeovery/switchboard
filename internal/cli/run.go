package cli

import (
	"cmp"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
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

A slow router is waited for, 5 seconds at most, and one that isn't running is
asked again for the first 2, as it may be restarting. A router that answers
but isn't healthy is routed through all the same, and run says so: its
failures are upstream, where a direct connection would meet them too. When
the router isn't running, or can't be routed through, run starts nothing, and
says why and how to go on: a session without the router would go unrouted
and unrecorded, and spend the primary past its reserve. The config's
when_router_down = "direct" has Claude Code connect directly instead, on
--account's token, else the primary's, else the first account's with a usable
token, and run says so; SWITCHBOARD_WHEN_ROUTER_DOWN, stop or direct, says
which for the launch it's given to, as in:
SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude

When run can't read its config, as when an upgrade's rules refuse it, it
routes through the router all the same while the router answers, on the
accounts the router names, and says so. When switchboard can't take part at
all, as without a config it can read and a router to route through, or
without any account's usable token, Claude Code starts as if switchboard
weren't there, and run says why. --direct starts Claude Code on its own login,
without the router or a token, for what needs the login.

With pools, a session is in the pool of the directory it starts in, for its
whole life: the fenced pool whose directories hold it, as it's given or as
its links lead, else the unfenced pool. Claude Code starts on that pool's
primary's token, --account names one of the pool's accounts, and going
direct goes on the pool's accounts alone. With more than one pool, a router
that doesn't route by the config's pools counts as down. A directory no pool
covers starts nothing, nor does one two fenced pools hold, nor one run can't
tell, and nor does Claude Code's own login inside a fenced pool's
directories, with --direct or without switchboard.

Claude Code's local subcommands, such as doctor, mcp and setup-token, and
its --version and --help, start as if switchboard weren't there, saying
nothing, whether the router runs or not: switchboard has no part in them.
Only the first of Claude Code's arguments names one, so -p doctor is a
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
			err := a.run(cmd.Context(), cmd.ErrOrStderr(), opts, args)
			if errors.Is(err, launch.ErrNotStarted) {
				// run has said why already, and how to go on.
				cmd.SilenceErrors = true
				return expected{err}
			}
			return err
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
// place of an account's token, or when it can't read its config and can't
// route through the running router either. With the config read and the
// router down, or the state directory its socket is in not located, it
// starts nothing, unless told to go direct.
func (a *app) run(ctx context.Context, stderr io.Writer, opts runOptions, args []string) error {
	// Without a home directory, only the install paths outside it are tried;
	// without a working directory, the router is told none.
	home, _ := a.HomeDir()
	dir, _ := a.Getwd()
	l := launch.Launcher{
		Environ:      a.Environ(),
		Dir:          dir,
		Home:         home,
		EvalSymlinks: filepath.EvalSymlinks,
		InstallPaths: claude.InstallPaths(home),
		Executable:   a.Executable,
		PID:          a.PID,
		Now:          a.Now,
		Exec:         a.Exec,
		Stderr:       stderr,
	}
	switch {
	case opts.direct && claude.IsLocal(args):
		// A local subcommand looks after Claude Code, not a session, so it
		// starts on its own login wherever it's asked for.
		return l.Direct(args, nil)
	case opts.direct:
		return l.Direct(args, a.ownLoginPools(ctx))
	case claude.IsLocal(args):
		return l.Local(args)
	}
	if key := l.KeyEnv(); key != "" {
		return l.StepAside(args, key)
	}
	cfg, err := a.loadConfig()
	if err != nil {
		return a.runUnread(ctx, l, opts, args, err)
	}
	if opts.account != "" {
		if err := notALabel(a.namesOf(cfg), opts.account); err != nil {
			return err
		}
	}
	r := launch.Route{
		Config:   cfg,
		Token:    a.readToken,
		Pause:    a.Pause,
		Patience: a.Patience,
		Account:  opts.account,
	}
	client, err := a.routerClient()
	if err != nil {
		return l.RouterUnfound(r, args, err)
	}
	r.Router, r.Moved, r.Kept = client, a.poolsMoved(), a.keptPools
	return l.Run(ctx, r, args)
}

// runUnread starts Claude Code with args in this process's place where the
// config can't be read, as unread says why: through the running router, on
// the pools and accounts it routes for, where its state directory can be
// located, as it can without a config; otherwise as if switchboard weren't
// there, but never inside a fenced pool's directories, as the config file's
// pools, read whatever else in it is wrong, say.
func (a *app) runUnread(ctx context.Context, l launch.Launcher, opts runOptions, args []string, unread error) error {
	written := a.writtenPools()
	client, err := a.routerClient()
	if err != nil {
		logger.Debug("can't find the router", "error", err)
		return l.Unaided(args, written.Pools, launch.CouldntReadConfig, unread)
	}
	r := launch.Route{
		Written:  written,
		Token:    a.readToken,
		Pause:    a.Pause,
		Router:   client,
		Document: client.Status,
		Patience: a.Patience,
		Account:  opts.account,
	}
	return l.RunUnread(ctx, r, args, unread)
}

// ownLoginPools returns the pools a launch on Claude Code's own login is
// refused inside the fences of: the config's; where it can't be read, those
// the config file writes, read whatever else in it is wrong (see
// writtenPools), else the running router's, as its health answer reports
// them, asked once, as RunUnread keeps its own login out of them; and where
// it names none once the move to pools is done, the router's, else those the
// accounts record keeps. None where none of those can be had.
func (a *app) ownLoginPools(ctx context.Context) config.Pools {
	cfg, err := a.loadConfig()
	switch {
	case err != nil:
		logger.Debug("can't read the config for its pools", "error", err)
		if written := a.writtenPools(); len(written.Pools) > 0 {
			return written.Pools
		}
	case len(cfg.Pools) > 0 || !a.poolsMoved():
		return cfg.Pools
	}
	if pools, reported := a.routersPools(ctx); reported {
		return pools
	}
	if err == nil {
		if kept, ok := a.keptPools(cfg); ok {
			return kept.Pools
		}
	}
	return nil
}

// routersPools returns the pools the running router routes for, as its
// health answer reports them, asked once: false where it can't be asked,
// doesn't answer, or reports none.
func (a *app) routersPools(ctx context.Context) (config.Pools, bool) {
	client, err := a.routerClient()
	if err != nil {
		return nil, false
	}
	patience := launch.Patience{Answer: cmp.Or(a.Patience.Answer, launch.AskTimeout)}
	h, err := patience.Ask(ctx, client)
	if err != nil {
		logger.Debug("can't ask the router for its pools", "error", err)
		return nil, false
	}
	return launch.RoutedPools(h)
}

// poolsMovedMark marks the move to pools done, in the state directory.
const poolsMovedMark = "pools.moved"

// poolsMoved reports whether the move to pools is done, as its mark in the
// state directory says: false where the state directory can't be located.
func (a *app) poolsMoved() bool {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, poolsMovedMark))
	return err == nil
}

// keptPools returns cfg, a config naming no pools, with the pools the
// accounts record keeps, each of its accounts in the pool the record keeps
// it in, as launch.Route's Kept does: false where the record keeps none.
// The record keeps no account's pool till the move to pools keeps them.
func (a *app) keptPools(*config.Config) (*config.Config, bool) {
	return nil, false
}

// writtenPools returns the pools the config file writes, read whatever else
// in it is wrong, as config.PoolsOf reads them; where they break the
// config's rules for them, each pool's directories, as config.FencesOf finds
// them, unchecked; none where the file can't be read even so.
func (a *app) writtenPools() launch.Written {
	path, err := a.configFile()
	if err != nil {
		logger.Debug("can't find the config file for its pools", "error", err)
		return launch.Written{}
	}
	pools, err := config.PoolsOf(path, a.HomeDir)
	if err == nil {
		return launch.Written{Pools: pools}
	}
	logger.Debug("the config file's pools can't be read as they're written", "error", err)
	fences, err := config.FencesOf(path, a.HomeDir)
	if err != nil {
		logger.Debug("can't read the config file's pools", "error", err)
		return launch.Written{}
	}
	return launch.Written{Pools: fences, Unchecked: true}
}
