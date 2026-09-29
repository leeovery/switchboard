package cli

import (
	"context"
	"encoding/json"
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
		asJSON  bool
		probe   bool
		session string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show every account's usage and when it resets",
		Long: `Show every account's usage and when it resets: the router's, while it runs,
with the sessions it has routed in the last hour, its pin, the limits it has
seen and its health; else read by probing each account, as --probe does
whether the router runs or not. The last line says which. When the claude a
shell runs from PATH isn't switchboard, so the sessions it starts don't go
through the router, the first line says so.

With --session, print the id of the account the router sends a Claude Code
session's requests to, the one its last-used model went to, as a statusline
asks; or with --json, the session's every model and why it went where it did.
It needs the router running: start it with switchboard service install (or
switchboard serve).

` + namingASession,
		Args: func(cmd *cobra.Command, args []string) error {
			switch {
			case cmd.Flags().Changed("session") && session == "":
				return errors.New("--session takes the id of a session")
			case session != "" && probe:
				return errors.New("--session asks the router, so it takes no --probe")
			}
			return cobra.NoArgs(cmd, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if session != "" {
				return a.sessionStatus(cmd.Context(), cmd.OutOrStdout(), session, asJSON)
			}
			doc, err := a.collect(cmd.Context(), probe)
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), doc)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), a.unrouted()+doc.Text(a.Now(), a.running(cmd.Context(), doc)...))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	cmd.Flags().BoolVar(&probe, "probe", false, "probe every account, even while the router runs")
	cmd.Flags().StringVar(&session, "session", "", "print the account the router sends session `ID`'s requests to")
	return cmd
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

// running lists the sessions the router has routed in the last hour, when doc
// is the router's: none otherwise, nor when the router, gone meanwhile, can't
// say.
func (a *app) running(ctx context.Context, doc status.Document) []status.Session {
	if doc.Source != status.SourceRouter {
		return nil
	}
	client, err := a.routerClient()
	var sessions []status.Session
	if err == nil {
		sessions, err = client.Sessions(ctx)
	}
	if err != nil {
		logger.Warn("can't list the router's sessions", "error", err)
		return nil
	}
	return sessions
}

// sessionStatus prints the id of the account the router sends a session's
// requests to, the session given by as much of its id as is unique.
func (a *app) sessionStatus(ctx context.Context, out io.Writer, given string, asJSON bool) error {
	client, err := a.routerClient()
	if err != nil {
		return err
	}
	session, err := a.session(ctx, client, given)
	if err != nil {
		return sessionError(err)
	}
	if asJSON {
		return writeJSON(out, session)
	}
	_, err = fmt.Fprintln(out, session.Account.ID)
	return err
}

// session asks the router where the requests of the session given, by as
// much of its id as is unique, go.
func (a *app) session(ctx context.Context, client *router.Client, given string) (status.Session, error) {
	id, err := a.findSession(ctx, client, given)
	if err != nil {
		return status.Session{}, err
	}
	return client.Session(ctx, id)
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

// writeJSON writes v as indented JSON, ending with a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
