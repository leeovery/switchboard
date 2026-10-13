package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func newSessionsCommand(a *app) *cobra.Command {
	var form formFlags
	cmd := &cobra.Command{
		Use:   "sessions [<id>]",
		Short: "List today's sessions, or show one: where each runs, what it's doing, and what it has been worth",
		Long: `List today's sessions: those running, the oldest started first, then those
that ended today, the latest ended first, a line each with its id cut short
and its directory, the account it's on, its model, what it's doing, when it
started, its requests today and what they'd have cost through the API, at the
prices switchboard carries, or the config's [prices] where it gives them, and
how it came to its account; then a line summing them. It reads the ledger's
files, and the router's sessions where it answers: without the router, it
lists today's sessions from the ledger alone, none as running.

Given a session, by its id or as much of it as is unique among today's
sessions, it shows that session as its page has it: what it's doing, where it
runs and why, each account it ran on, with its requests there, the points of
each window they took, an estimate, and what they'd have cost through the API,
then its turns, newest first, a prompt each with every request that served
it, and its totals. A session of an earlier day is found by its whole id.

On a terminal, sessions prints its lines as text. Anywhere else, as in a pipe
or an agent's shell, it prints them as JSON, for an agent or a script to read.
--json prints the JSON, and --pretty the text, wherever stdout is.`,
		Args: oneSessionAtMost,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			asJSON := a.printsJSON(form, out)
			if len(args) == 1 {
				return a.sessionPage(cmd.Context(), out, cmd.ErrOrStderr(), args[0], asJSON)
			}
			return a.sessions(cmd.Context(), out, cmd.ErrOrStderr(), asJSON)
		},
	}
	form.add(cmd)
	return cmd
}

func oneSessionAtMost(_ *cobra.Command, args []string) error {
	switch {
	case len(args) > 1:
		return errors.New("give one session's id, or none to list today's sessions")
	case len(args) == 1 && args[0] == "":
		return errors.New("give a session's id, or none to list today's sessions")
	}
	return nil
}

// sessions prints today's sessions, as Sessions' List has them, telling
// errOut where the router can't say which are running.
func (a *app) sessions(ctx context.Context, out, errOut io.Writer, asJSON bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	readers, err := a.newStateReaders(cfg)
	if err != nil {
		return err
	}
	running, unlisted := a.routedSessions(ctx, errOut, "listing today's sessions from the ledger alone, none as running")
	now := a.Now()
	list := views.ListSessions(views.SessionSources{
		Running:  running,
		Unlisted: unlisted,
		Ledger:   readers.ledger,
		Prices:   pricing(cfg),
		Now:      now,
	})
	if asJSON {
		return writeJSON(out, list)
	}
	return writeSessions(out, list, now)
}

// routedSessions returns the sessions the router lists, and whether no
// router lists them, as it can't be asked, telling errOut so, and what it
// does instead, as what's printed is then the ledger's alone.
func (a *app) routedSessions(ctx context.Context, errOut io.Writer, instead string) (sessions []status.Session, unlisted bool) {
	sessions, err := a.routerSessions(ctx)
	switch {
	case errors.Is(err, router.ErrNotRunning):
		launch.Notice(errOut, "the router isn't running — "+instead)
	case err != nil:
		launch.Warn(errOut, "couldn't ask the router its sessions", err, instead)
	}
	return sessions, err != nil
}

func (a *app) routerSessions(ctx context.Context) ([]status.Session, error) {
	client, err := a.routerClient()
	if err != nil {
		return nil, err
	}
	return client.Sessions(ctx)
}

// sessionHeads are the heads of the columns of sessions' text, as the List's.
var sessionHeads = []string{"", "session", "on", "model", "state", "started", "requests", "worth", "routing"}

// writeSessions writes list as sessions' text, at now: a row a session under
// the List's heads, then a line summing them; or says there were none.
func writeSessions(out io.Writer, list views.SessionList, now time.Time) error {
	if len(list.Sessions) == 0 {
		_, err := fmt.Fprintln(out, "no sessions today")
		return err
	}
	rows := [][]string{sessionHeads}
	for _, s := range list.Sessions {
		rows = append(rows, sessionRow(s, now))
	}
	if err := writeTable(out, rows); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "\ntoday  %s%s%s%s%s at API prices\n", prose.SessionCount(list.Today.Sessions), prose.Separator,
		prose.Counted(list.Today.Requests, "request"), prose.Separator, dashboard.WorthCents(list.Today.Worth))
	return err
}

// sessionRow is a session's row: its mark; its id cut short and its
// directory; the account it's on, and its model; what it's doing, at now;
// when it started; its requests today, and their worth; and how it came to
// its account.
func sessionRow(s views.ListedSession, now time.Time) []string {
	session := status.ShortID(prose.Clean(s.Session))
	if s.Dir != "" {
		session += " " + prose.Clean(s.Dir)
	}
	started := "-"
	if !s.Started.IsZero() {
		started = prose.Past(now, s.Started)
	}
	return []string{s.Mark.String(), session, orNone(prose.Clean(s.Account)), orNone(ledger.ShortModelID(prose.Clean(s.Model))),
		doingSaid(s.Activity, now), started, prose.Grouped(s.Requests), dashboard.WorthCents(s.Worth), routingSaid(s.Routing)}
}

// doingSaid says what a session is doing at now, as its Activity has it:
// asking or answering; with a request throttled, to go out again, as "…
// 429", or refused, before it goes out again elsewhere, as "✕ 403"; idle,
// for as long as it has been since it was last at work, as "idle 9m"; or
// when it ended, as "ended 11:21".
func doingSaid(a views.Activity, now time.Time) string {
	switch a.Doing {
	case views.Asking:
		return status.Asking
	case views.Answering:
		return status.Answering
	case views.Throttled:
		return "… " + strconv.Itoa(a.Status)
	case views.Refused:
		return "✕ " + strconv.Itoa(a.Status)
	case views.Ended:
		return "ended " + prose.Past(now, a.Since)
	}
	if a.Since.IsZero() {
		return "idle"
	}
	return "idle " + prose.Span(now.Sub(a.Since))
}

// routingSaid says how a session came to the account it's on, as its
// routing has it: its words, as "from work, at its cap", or "pinned here";
// and what writing its context again there cost, where that's known, as
// "$1.82".
func routingSaid(r views.RoutingSaid) string {
	said := r.Words.String()
	if r.Cost != nil {
		said += "  " + r.Cost.Cents()
	}
	return said
}
