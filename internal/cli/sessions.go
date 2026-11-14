package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

func newSessionsCommand(a *app) *cobra.Command {
	var (
		form formFlags
		pool string
	)
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

It lists one pool's sessions, each by the pool its requests were routed in,
one from before pools the first pool's. ` + readingAPool + ` A session
given is found in whatever pool it's in, its page that of its work in the
pool it's in now, the one its newest request was routed in; or with
--pool, in that pool alone, its page that of its work there, among the
router's sessions there.

` + readingAKeptPool + `

On a terminal, sessions prints its lines as text. Anywhere else, as in a pipe
or an agent's shell, it prints them as JSON, for an agent or a script to read.
--json prints the JSON, and --pretty the text, wherever stdout is.`,
		Args: oneSessionAtMost,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			asJSON := a.printsJSON(form, out)
			if len(args) == 1 {
				return a.sessionPage(cmd.Context(), out, cmd.ErrOrStderr(), args[0], pool, asJSON)
			}
			return a.sessions(cmd.Context(), out, cmd.ErrOrStderr(), pool, asJSON)
		},
	}
	addPoolFlag(cmd, &pool, "list the sessions of")
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

// sessions prints today's sessions of the pool named, as filedPool has it, as
// Sessions' List has them, telling errOut where the router can't say which
// are running.
func (a *app) sessions(ctx context.Context, out, errOut io.Writer, named string, asJSON bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	readers, err := a.newStateReaders(ctx, cfg)
	if err != nil {
		return err
	}
	pool, cfg, err := a.filedPool(ctx, cfg, readers.dir, named)
	if err != nil {
		return err
	}
	running, unlisted := a.routedSessions(ctx, errOut, pool, "listing today's sessions from the ledger alone, none as running")
	now := a.Now()
	list := views.ListSessions(views.SessionSources{
		Running:  running,
		Unlisted: unlisted,
		Ledger:   pool.sessions(readers),
		Prices:   pricing(cfg),
		Now:      now,
	})
	list = list.Named(status.NamesOf(cfg.Accounts, readers.removed))
	if asJSON {
		return writeJSON(out, list)
	}
	if _, err := io.WriteString(out, pool.heading()); err != nil {
		return err
	}
	return writeSessions(out, list, now)
}

// routedSessions returns the sessions the router lists in the pool given, or,
// for none, in every pool, as the ledger's are read in every pool of a config
// with no pools, and whether no router lists them, as it can't be asked,
// telling errOut so, and what it does instead, as what's printed is then the
// ledger's alone. Of a pool kept but not configured, it asks nothing, as the
// router runs no session in a pool it doesn't route; nor of a router from
// before pools, as routerBeforePools has it, whose sessions are no pool's,
// though it says so, as of one that can't be asked.
func (a *app) routedSessions(ctx context.Context, errOut io.Writer, pool filedPool, instead string) (sessions []status.Session, unlisted bool) {
	switch {
	case pool.kept:
		return nil, false
	case a.routerBeforePools(ctx):
		launch.Notice(errOut, status.BeforePoolsSaid+" — "+instead)
		return nil, true
	}
	sessions, err := a.routerSessions(ctx, func(c *router.Client) ([]status.Session, error) {
		if pool.ID == "" {
			return c.SessionsInEveryPool(ctx)
		}
		return c.InPool(pool.ID).Sessions(ctx)
	})
	switch {
	case router.IsNotRunning(err):
		launch.Notice(errOut, "the router isn't running — "+instead)
	case err != nil:
		launch.Warn(errOut, "couldn't ask the router its sessions", err, instead)
	}
	return sessions, err != nil
}

// routerSessions lists the sessions the router routed in the last hour, as
// list asks its client for them.
func (a *app) routerSessions(ctx context.Context, list func(*router.Client) ([]status.Session, error)) ([]status.Session, error) {
	client, err := a.routerClient()
	if err != nil {
		return nil, err
	}
	return list(client)
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
		prose.Counted(list.Today.Requests, "request"), prose.Separator, list.Today.Cents())
	return err
}

// sessionRow is a session's row: its mark; its id cut short and its
// directory; the account it's on, and its model; what it's doing, at now;
// when it started; its requests today, and their worth; and how it came to
// its account, as the Log's routing says it, "from work, at its cap  $1.82",
// each account named as the List's Named names it.
func sessionRow(s views.ListedSession, now time.Time) []string {
	session := status.ShortID(prose.Clean(s.Session))
	if s.Dir != "" {
		session += " " + prose.Clean(s.Dir)
	}
	started := "-"
	if !s.Started.IsZero() {
		started = prose.Past(now, s.Started)
	}
	return []string{s.Mark.String(), session, orNone(s.Said()), orNone(ledger.ShortModelID(s.Model)),
		s.Activity.State(now), started, prose.Grouped(s.Requests), s.Cents(), s.Routing.Said().String()}
}
