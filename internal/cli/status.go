package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/spf13/cobra"
)

func newStatusCommand(a *app) *cobra.Command {
	var (
		asJSON  bool
		session string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show every account's usage and when it resets",
		Long: `Show every account's usage and when it resets, read by probing each account.

With --session, show the account the router sends a Claude Code session's
requests to, as a statusline asks: that account alone, or with --json, the
session's every model and why it went where it did. It needs the router
running.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("session") && session == "" {
				return errors.New("--session takes the id of a session")
			}
			return cobra.NoArgs(cmd, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if session != "" {
				return a.sessionStatus(cmd.Context(), cmd.OutOrStdout(), session, asJSON)
			}
			doc, err := a.collect(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), doc)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), doc.Text(a.Now()))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	cmd.Flags().StringVar(&session, "session", "", "show the account the router sends session `ID`'s requests to")
	return cmd
}

// sessionStatus prints the account the router sends a session's requests to.
func (a *app) sessionStatus(ctx context.Context, out io.Writer, id string, asJSON bool) error {
	client, err := a.routerClient()
	if err != nil {
		return err
	}
	session, err := client.Session(ctx, id)
	if err != nil {
		return fromRouter(err)
	}
	if asJSON {
		return writeJSON(out, session)
	}
	_, err = io.WriteString(out, session.Account.Text(a.Now()))
	return err
}

// writeJSON writes v as indented JSON, ending with a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
