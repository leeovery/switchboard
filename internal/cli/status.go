package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func newStatusCommand(a *app) *cobra.Command {
	var (
		form    formFlags
		probe   bool
		refresh bool
		session string
		pool    string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show every account's usage and when it resets",
		Long: `Show every account's usage and when it resets: the router's, while it runs,
with the sessions it has routed in the last hour, its pin, the limits it has
seen and its health; else read by probing each account, as --probe does
whether the router runs or not. The last line says which, but for a restart
the router has due, which the line under it says, with how to have it now.
When the claude a shell runs from PATH isn't switchboard, so the sessions it
starts don't go through the router, the first line says so. Where the config
can't be read, as when an upgrade's rules refuse it and the router's restart
into the new version is held, status shows the router's document while the
router answers, then fails, saying why the config can't be read.

It shows one pool's accounts. ` + readingAPool + ` Where there's more
than one pool, the text names the one shown first.

On a terminal, status prints this as text. Anywhere else, as in a pipe or an
agent's shell, it prints the status document as JSON, for an agent or a
script to read. --json prints the JSON, and --pretty the text, wherever
stdout is.

With --refresh, the router first reads every account it may, as usage
--refresh has it do, and waits for those reads, ten seconds at most, so once
a limit is reset by hand, the router sees it. Without the router, or with
--probe, every account is probed anyway, so --refresh changes nothing.

With --session, print the label of the account the router sends a Claude Code
session's requests to, the one its last-used model went to, as a statusline
asks, on a terminal or off one; or with --json, the session's every model and
why it went where it did, each account by its id. It needs the router
running: start it with switchboard service install (or switchboard serve).

` + namingASession,
		Args: func(cmd *cobra.Command, args []string) error {
			switch {
			case cmd.Flags().Changed("session") && session == "":
				return errors.New("--session takes the id of a session")
			case session != "" && probe:
				return errors.New("--session asks the router, so it takes no --probe")
			case session != "" && refresh:
				return errors.New("--session asks after one session, not the accounts, so it takes no --refresh")
			case session != "" && cmd.Flags().Changed("pool"):
				return errors.New("--session asks after one session, in whatever pool it's in, so it takes no --pool")
			}
			return noArgs(cmd, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if session != "" {
				return a.sessionStatus(cmd.Context(), out, session, form.json)
			}
			doc, unread, err := a.statusDocument(cmd.Context(), probe, readOnce(refresh), pool)
			if err != nil {
				return err
			}
			if a.printsJSON(form, out) {
				err = writeJSON(out, doc)
			} else {
				m := a.withRemoved(doc).At(a.Now(), claude.Policy())
				_, err = io.WriteString(out, a.unrouted()+a.poolHeading(m)+m.Text(a.running(cmd.Context(), m)...))
			}
			return cmp.Or(err, unread)
		},
	}
	form.addWithJSON(cmd, "print the status document as JSON, as off a terminal; with --session, the session's every model and why it went where it did")
	cmd.Flags().BoolVar(&probe, "probe", false, "probe every account, even while the router runs")
	cmd.Flags().BoolVarP(&refresh, "refresh", "r", false, "have the router read every account it may first, as usage --refresh does")
	cmd.Flags().StringVar(&session, "session", "", "print the account the router sends session `ID`'s requests to")
	addPoolFlag(cmd, &pool, "show")
	return cmd
}

// poolHeading names the pool the moment is of, in a paragraph of a line,
// as poolNamed names it: "" where the config has one pool or none, or can't
// be loaded, as where the router's document is printed in its place.
func (a *app) poolHeading(m status.Moment) string {
	cfg, err := a.loadConfig()
	if err != nil {
		return ""
	}
	if named := poolNamed(cfg.Pools, m.PoolLabel); named != "" {
		return named + "\n\n"
	}
	return ""
}

// unrouted says, in a paragraph of a line, when the claude a shell runs from
// PATH isn't switchboard, so the sessions it starts don't go through the
// router: "" when it is, or when there's no telling.
func (a *app) unrouted() string {
	through, err := claude.ThroughSwitchboard(a.Getenv("PATH"), a.Executable)
	if err != nil {
		logger.Debug("can't tell whether claude on PATH is switchboard", "error", err)
		return ""
	}
	if through {
		return ""
	}
	return "claude on PATH isn't switchboard, so the sessions it starts don't go through the router: run switchboard setup\n\n"
}

// running lists the sessions the router has routed in the last hour in the
// pool of the document m is a moment of, where it's the router's: none
// otherwise, nor when the router, gone meanwhile, can't say.
func (a *app) running(ctx context.Context, m status.Moment) []status.Session {
	if !m.Routed {
		return nil
	}
	sessions, err := a.routerSessions(ctx, func(c *router.Client) ([]status.Session, error) { return c.InPool(m.PoolID).Sessions(ctx) })
	if err != nil {
		logger.Warn("can't list the router's sessions", "error", err)
		return nil
	}
	return sessions
}

// sessionStatus prints the account the router sends a session's requests to,
// as the verbs' text names it, the session given by as much of its id as is
// unique.
func (a *app) sessionStatus(ctx context.Context, out io.Writer, given string, asJSON bool) error {
	client, err := a.routerClient()
	if err != nil {
		return err
	}
	names := a.accountNames()
	session, err := a.session(ctx, client, names, given)
	if err != nil {
		return sessionError(err)
	}
	if asJSON {
		return writeJSON(out, session)
	}
	_, err = fmt.Fprintln(out, names.Named(session.AccountID()).Said())
	return err
}

// session asks the router where the requests of the session given, by as
// much of its id as is unique, go, in whatever pool it's in, its accounts as
// names name them.
func (a *app) session(ctx context.Context, client *router.Client, names status.Names, given string) (status.Session, error) {
	id, answered, err := a.lookUpSession(ctx, client, names, given)
	if err != nil {
		return status.Session{}, err
	}
	if answered != nil {
		return *answered, nil
	}
	return client.Session(ctx, id)
}

// withRemoved is doc, its moments naming the accounts removed that the
// accounts record keeps, as removedHere gives them, so its text names a
// session on one by the label it showed last: doc as it is where the config
// can't be loaded, as where the router's document is printed in its place.
func (a *app) withRemoved(doc status.Document) status.Document {
	cfg, err := a.loadConfig()
	if err != nil {
		return doc
	}
	return doc.WithRemoved(a.removedHere(cfg))
}

// sessionError is err, from asking the router after a session, marked
// expected when it's what a statusline meets as a matter of course: the
// router isn't running, or hasn't seen the session yet.
func sessionError(err error) error {
	err = fromRouter(err)
	if errors.Is(err, errRouterDown) || errors.Is(err, router.ErrUnknownSession) {
		return expected{err}
	}
	return err
}
