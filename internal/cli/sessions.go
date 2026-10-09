package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func newSessionsCommand(a *app) *cobra.Command {
	var form formFlags
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List today's sessions: where each runs, what it's doing, and what it has been worth",
		Long: `List today's sessions: those running, the oldest started first, then those
that ended today, the latest ended first, a line each with its id cut short and
its directory, the account it's on, its model, what it's doing, when it
started, its requests today and what they'd have cost through the API, and how
it came to its account; then a line summing them. It reads the ledger's files,
and the router's sessions where it answers: without the router, it lists
today's sessions from the ledger alone, none as running.

On a terminal, sessions prints its lines as text. Anywhere else, as in a pipe
or an agent's shell, it prints them as JSON, for an agent or a script to read.
--json prints the JSON, and --pretty the text, wherever stdout is.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			return a.sessions(cmd.Context(), out, cmd.ErrOrStderr(), a.printsJSON(form, out))
		},
	}
	form.add(cmd)
	return cmd
}

// sessions prints today's sessions, as Sessions' List has them, telling
// errOut where the router can't say which are running.
func (a *app) sessions(ctx context.Context, out, errOut io.Writer, asJSON bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	now := a.Now()
	list := views.ListSessions(views.SessionSources{
		Running: a.routedSessions(ctx, errOut),
		Ledger:  a.stateReadersOrNone(cfg).ledger,
		Prices:  pricing(cfg),
		Now:     now,
	})
	if asJSON {
		return writeJSON(out, list)
	}
	return writeSessions(out, list, now)
}

// routedSessions returns the sessions the router lists, telling errOut where
// it can't ask them, as the list is then the ledger's alone.
func (a *app) routedSessions(ctx context.Context, errOut io.Writer) []status.Session {
	client, err := a.routerClient()
	var sessions []status.Session
	if err == nil {
		sessions, err = client.Sessions(ctx)
	}
	const instead = "listing today's sessions from the ledger alone, none as running"
	switch {
	case errors.Is(err, router.ErrNotRunning):
		launch.Notice(errOut, "the router isn't running — "+instead)
	case err != nil:
		launch.Warn(errOut, "couldn't ask the router its sessions", err, instead)
	}
	return sessions
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
	var table strings.Builder
	if err := writeTable(&table, rows); err != nil {
		return err
	}
	for row := range strings.Lines(table.String()) {
		if _, err := fmt.Fprintln(out, strings.TrimRight(row, " \n")); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "\ntoday  %s%s%s%s%s at API prices\n", status.SessionCount(list.Today.Sessions), status.Separator,
		requestsCount(list.Today.Requests), status.Separator, todaysWorth(list))
	return err
}

// sessionRow is a session's row: its mark; its id cut short and its
// directory; the account it's on, and its model; what it's doing, at now;
// when it started; its requests today, and their worth; and how it came to
// its account.
func sessionRow(s views.ListedSession, now time.Time) []string {
	session := status.ShortID(status.Clean(s.Session))
	if s.Dir != "" {
		session += " " + status.Clean(s.Dir)
	}
	started := "-"
	if !s.Started.IsZero() {
		started = status.Past(now, s.Started)
	}
	return []string{mark(s), session, orNone(status.Clean(s.Account)), orNone(strings.TrimPrefix(status.Clean(s.Model), "claude-")),
		doing(s, now), started, thousands(s.Requests), worthSaid(s), routing(s, now)}
}

// mark is the mark a session's row starts with, the first that applies: ↑
// while it asks, ↓ while it's answered, ▸ where a move brought it to the
// account it's on; else none.
func mark(s views.ListedSession) string {
	switch {
	case s.State == status.Asking:
		return "↑"
	case s.State == status.Answering:
		return "↓"
	case s.Moved != nil:
		return "▸"
	}
	return ""
}

// doing says what a session is doing at now: asking or answering; idle, for
// as long as it has been since the router last saw it; or when it ended.
func doing(s views.ListedSession, now time.Time) string {
	switch {
	case s.State != "":
		return s.State
	case !s.Running:
		return "ended " + status.TimeOfDay(now, s.Ended)
	case s.LastSeen.IsZero():
		return "idle"
	}
	return "idle " + span(s.LastSeen, now)
}

// span says how long it's been from since to now, as briefly as a row has
// room for: in seconds under a minute, as "34s", else as a countdown says it,
// as "9m" or "1h 5m".
func span(since, now time.Time) string {
	if d := now.Sub(since); d < time.Minute {
		return strconv.Itoa(int(max(d, 0)/time.Second)) + "s"
	}
	return status.Countdown(since, now)
}

// worthSaid says what a session's requests today were worth, to the cent, as
// "$17.86", "+" after it where it leaves some unpriced, and "unpriced" where
// it prices none of them.
func worthSaid(s views.ListedSession) string {
	switch {
	case len(s.Unpriced) == 0:
		return s.Worth.Cents()
	case s.Worth == 0:
		return "unpriced"
	}
	return s.Worth.Cents() + "+"
}

// todaysWorth says what the List's sessions' requests today were worth, as
// worthSaid says a session's, "+" after it where any leaves some unpriced.
func todaysWorth(list views.SessionList) string {
	worth := list.Today.Worth.Cents()
	if slices.ContainsFunc(list.Sessions, func(s views.ListedSession) bool { return len(s.Unpriced) > 0 }) {
		worth += "+"
	}
	return worth
}

// routing says how a session came to the account it's on, in the routing
// words: the move that brought it, as "from work, at its cap", and what
// writing its context again there cost, where that's known, as "$1.82";
// "pinned here" for one its own pin put there; else nothing, as it started
// there.
func routing(s views.ListedSession, now time.Time) string {
	if m := s.Moved; m != nil {
		said := status.RoutingWords(m.Reason, status.Facts{From: m.From, To: s.Account, Now: now}).String()
		if m.Cost != nil {
			said += "  " + m.Cost.Cents()
		}
		return said
	}
	if slices.ContainsFunc(s.Models, func(m views.SessionModel) bool { return m.Account == s.Account && m.Reason == status.ReasonPinned }) {
		return "pinned here"
	}
	return ""
}

// requestsCount counts requests in words, as "1,240 requests" or "1 request".
func requestsCount(n int) string {
	if n == 1 {
		return "1 request"
	}
	return thousands(n) + " requests"
}

// thousands writes n with its thousands set apart by commas, as "1,240".
func thousands(n int) string {
	digits := strconv.Itoa(n)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}
