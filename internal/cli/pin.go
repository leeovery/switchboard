package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
)

// auto is what pin takes to go back to routing, which is why config reserves
// it.
const auto = config.ReservedID

func newPinCommand(a *app) *cobra.Command {
	var move bool
	cmd := &cobra.Command{
		Use:   "pin <account>|auto",
		Short: "Send every new session to one account, or go back to routing",
		Long: `Send every new session to one account, while it has room. A session's own pin,
from run --account, still wins. With --move, sessions already running move
there too, each on its next request, at the cost of rebuilding its cache.
"pin auto" goes back to routing every session on its merits.

It needs the router running: see switchboard serve.`,
		Args: func(_ *cobra.Command, args []string) error {
			switch {
			case len(args) != 1:
				return errors.New("give one account to pin, or auto")
			case args[0] == auto && move:
				return errors.New("--move goes with an account to pin, not auto")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.pin(cmd.Context(), cmd.OutOrStdout(), args[0], move)
		},
	}
	cmd.Flags().BoolVar(&move, "move", false, "move running sessions to the account too, each on its next request")
	return cmd
}

// pin pins the router to the account given, or unpins it for auto, and says
// what that does.
func (a *app) pin(ctx context.Context, out io.Writer, account string, move bool) error {
	client, err := a.routerClient()
	if err != nil {
		return err
	}
	if account == auto {
		if _, err := client.Unpin(ctx); err != nil {
			return fromRouter(err)
		}
		_, err := fmt.Fprintln(out, "routing automatically")
		return err
	}
	doc, err := client.Pin(ctx, account, move)
	if err != nil {
		return fromRouter(err)
	}
	_, err = fmt.Fprintln(out, pinned(doc))
	return err
}

// pinned says what the document's pin does.
func pinned(doc status.Document) string {
	name := doc.Pin.Account
	if account, ok := doc.Account(name); ok {
		name = account.Title()
	}
	said := "new sessions go to " + name
	if doc.Pin.Move {
		said += ", and running sessions move on their next request"
	}
	return said
}
