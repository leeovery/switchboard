package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// auto is what pin takes to go back to routing, in any case, which is why
// config reserves it.
const auto = config.ReservedID

// isAuto reports whether pin was given auto, in any case, as config reserves
// it.
func isAuto(account string) bool {
	return strings.EqualFold(account, auto)
}

// pinOptions are the pin command's flags.
type pinOptions struct {
	move, force bool
	// session names the one session to pin, by as much of its id as is
	// unique, or is "" to pin the router.
	session string
	// pool is the id of the pool whose pin is set or cleared, "" for the
	// one the verbs act on, or the one the accounts given are in; or, with
	// session, the pool it's found in, "" for whatever pool it's in.
	pool string
}

func newPinCommand(a *app) *cobra.Command {
	var opts pinOptions
	cmd := &cobra.Command{
		Use:   "pin <account>...|auto",
		Short: "Send sessions to the accounts given, or go back to routing",
		Long: `Send every new session to the accounts given, while one has room: to the one,
or to whichever of several would be routed to were they the only accounts,
spending their reserves. When none has room, sessions are routed as though
nothing were pinned. The accounts replace any pinned before. A session's own
pin, from run --account or pin --session, still wins. With --move, sessions
already running on another account move to them too, each on its next
request, at the cost of rebuilding its cache. With --force, no session keeps
a pin of its own, the one run --account gave it included, so --move --force
puts every session on the accounts given. "pin auto" goes back to routing
every session on its merits, and with --force clears every session's own pin
as well.

Each pool has its own pin, its sessions alone moved, and with --force, their
own pins alone cleared. The accounts given are pinned in the pool they're in, so they must be of one pool, and of
the one --pool names, where it's given. "pin auto" clears the pin of the pool
--pool names, by its id; without it, the only pool, where there's one, else
the one fenced to no directory; where every pool is fenced, it asks for
--pool. The directory it's run in never chooses. Where there's more than one
pool, what's said names it.

With --session, pin one running session alone to one account, from its next
request, in place of any pin it had, the one run --account gave it included;
"pin auto --session" clears its own pin, and it's routed like any other. The
session is found in whatever pool it's in, or with --pool, in that pool
alone.

` + namingASession + `

It needs the router running: start it with switchboard service install (or
switchboard serve).`,
		Args: opts.check,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.pin(cmd.Context(), cmd.OutOrStdout(), args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.move, "move", false, "move running sessions on other accounts to them too, each on its next request")
	cmd.Flags().BoolVar(&opts.force, "force", false, "clear every session's own pin too, the pool's sessions' alone, the one run --account gave it included")
	cmd.Flags().StringVar(&opts.session, "session", "", "pin session `ID` alone, by as much of its id as is unique")
	addPoolFlag(cmd, &opts.pool, "set or clear the pin of")
	return cmd
}

// check accepts the accounts to pin, or auto alone, and the flags that go
// with them: one account alone with --session.
func (o *pinOptions) check(cmd *cobra.Command, args []string) error {
	switch {
	case len(args) == 0:
		return errors.New("give the accounts to pin, or auto")
	case len(args) > 1 && slices.ContainsFunc(args, isAuto):
		return errors.New("auto goes back to routing, so it takes no account to pin")
	case cmd.Flags().Changed("session") && o.session == "":
		return errors.New("--session takes the id of a session")
	case o.session != "" && len(args) > 1:
		return errors.New("--session pins one session to one account, so give one")
	case o.session != "" && (o.move || o.force):
		return errors.New("--session pins one session alone, so it takes no --move or --force")
	case isAuto(args[0]) && o.move:
		return errors.New("--move goes with an account to pin, not auto")
	}
	return nil
}

// pin pins the accounts given as the command line asks, and says what that
// does.
func (a *app) pin(ctx context.Context, out io.Writer, accounts []string, opts pinOptions) error {
	if !isAuto(accounts[0]) {
		names := a.accountNames()
		for _, given := range accounts {
			if err := notALabel(names, given); err != nil {
				return err
			}
		}
	}
	client, err := a.routerClient()
	if err != nil {
		return err
	}
	if a.routerBeforePools(ctx) {
		return status.ErrBeforePools
	}
	said, err := a.pinning(ctx, client, accounts, opts)
	if err != nil {
		return poolFlagNeeded(fromRouterChange(err))
	}
	_, err = fmt.Fprintln(out, said)
	return err
}

// pinning pins the session opts names to the one account given, or else the
// router to the accounts given, in their pool, or unpins either for auto, the
// pool's pin in the pool opts names, and says what that does, naming the
// pool where the config has more than one.
func (a *app) pinning(ctx context.Context, client *router.Client, accounts []string, opts pinOptions) (string, error) {
	switch {
	case opts.session != "":
		return a.pinSession(ctx, client, opts.session, opts.pool, accounts[0])
	case isAuto(accounts[0]):
		doc, err := client.InPool(opts.pool).Unpin(ctx, opts.force, router.ByCLI)
		if err != nil {
			return "", err
		}
		in := a.inPool(ctx, doc.At(a.Now(), claude.Policy()))
		return forced("routing automatically"+in, in, opts.force), nil
	}
	doc, err := client.InPool(opts.pool).Pin(ctx, router.PinRequest{Accounts: accounts, Move: opts.move, Force: opts.force, By: router.ByCLI})
	if err != nil {
		return "", err
	}
	m := doc.At(a.Now(), claude.Policy())
	in := a.inPool(ctx, m)
	return forced(pinned(m, in), in, opts.force), nil
}

// inPool names the pool the moment is of, as " in pool Acme", where there's
// more than one, as poolsStanding counts them and poolNamed names it: "" where
// there's one or none.
func (a *app) inPool(ctx context.Context, m status.Moment) string {
	return inPoolNamed(a.poolsStanding(ctx), m.PoolLabel)
}

// pinSession pins the session given, by as much of its id as is unique, in
// the pool with the id given, "" for whatever pool it's in, to the account
// given, or clears its own pin for auto, and says what that does.
func (a *app) pinSession(ctx context.Context, client *router.Client, given, pool, account string) (string, error) {
	id, err := a.findSession(ctx, client, a.accountNames(), given, pool)
	if err != nil {
		// Only a read was asked yet, so a router closing reads as not running.
		return "", fromRouter(err)
	}
	session := "session " + status.ShortID(prose.Clean(id))
	if isAuto(account) {
		if _, err := client.UnpinSession(ctx, id, router.ByCLI); err != nil {
			return "", err
		}
		return session + " is routed automatically from its next request", nil
	}
	if _, err := client.PinSession(ctx, id, account, router.ByCLI); err != nil {
		return "", err
	}
	return session + " goes to " + account + " from its next request", nil
}

// pinned says what the pin of the document m is a moment of does, to the new
// sessions in the pool in names, as inPool names it.
func pinned(m status.Moment, in string) string {
	said := "new sessions" + in + " go to " + m.Destination(m.Pin.Accounts)
	if m.Pin.Move {
		said += ", and running sessions move on their next request"
	}
	return said
}

// forced follows what a pin does, as said, with whose own pins it cleared,
// where force says it did: those of the sessions in the pool in names, as
// inPool names it where there's more than one, else every session's.
func forced(said, in string, force bool) string {
	switch {
	case !force:
		return said
	case in == "":
		return said + ", every session's own pin cleared"
	default:
		return said + ", the own pin of every session" + in + " cleared"
	}
}
