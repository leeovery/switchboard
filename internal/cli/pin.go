package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
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

With --session, pin one running session alone to one account, from its next
request, in place of any pin it had, the one run --account gave it included;
"pin auto --session" clears its own pin, and it's routed like any other.

` + namingASession + `

It needs the router running: start it with switchboard service install (or
switchboard serve).`,
		Args: opts.check,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.pin(cmd.Context(), cmd.OutOrStdout(), args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.move, "move", false, "move running sessions to the accounts too, each on its next request")
	cmd.Flags().BoolVar(&opts.force, "force", false, "clear every session's own pin too, the one run --account gave it included")
	cmd.Flags().StringVar(&opts.session, "session", "", "pin session `ID` alone, by as much of its id as is unique")
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
	client, err := a.routerClient()
	if err != nil {
		return err
	}
	said, err := a.pinning(ctx, client, accounts, opts)
	if err != nil {
		return fromRouter(err)
	}
	_, err = fmt.Fprintln(out, said)
	return err
}

// pinning pins the session opts names to the one account given, or else the
// router to the accounts given, or unpins either for auto, and says what that
// does.
func (a *app) pinning(ctx context.Context, client *router.Client, accounts []string, opts pinOptions) (string, error) {
	switch {
	case opts.session != "":
		return a.pinSession(ctx, client, opts.session, accounts[0])
	case isAuto(accounts[0]):
		if _, err := client.Unpin(ctx, opts.force); err != nil {
			return "", err
		}
		return forced("routing automatically", opts.force), nil
	}
	doc, err := client.Pin(ctx, router.PinRequest{Accounts: accounts, Move: opts.move, Force: opts.force})
	if err != nil {
		return "", err
	}
	return forced(pinned(doc), opts.force), nil
}

// pinSession pins the session given, by as much of its id as is unique, to
// the account given, or clears its own pin for auto, and says what that does.
func (a *app) pinSession(ctx context.Context, client *router.Client, given, account string) (string, error) {
	id, err := a.findSession(ctx, client, given)
	if err != nil {
		return "", err
	}
	session := "session " + status.ShortID(status.Clean(id))
	if isAuto(account) {
		if _, err := client.UnpinSession(ctx, id); err != nil {
			return "", err
		}
		return session + " is routed automatically from its next request", nil
	}
	if _, err := client.PinSession(ctx, id, account); err != nil {
		return "", err
	}
	return session + " goes to " + account + " from its next request", nil
}

// pinned says what the document's pin does.
func pinned(doc status.Document) string {
	said := "new sessions go to " + doc.Destination(doc.Pin.Accounts)
	if doc.Pin.Move {
		said += ", and running sessions move on their next request"
	}
	return said
}

// forced follows what a pin does, as said, with that it cleared every
// session's own pin, when force says it did.
func forced(said string, force bool) string {
	if !force {
		return said
	}
	return said + ", every session's own pin cleared"
}
