package cli

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
)

func newStatusCommand(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show every account's usage and when it resets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			collector := status.Collector{
				Prober: &claude.Prober{Upstream: cfg.Upstream, Version: a.ClaudeVersion()},
				Getenv: a.Getenv,
				Now:    a.Now,
			}
			doc := collector.Collect(cmd.Context(), cfg.Accounts)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), doc)
			}
			_, err = io.WriteString(cmd.OutOrStdout(), doc.Text(a.Now()))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the status document as JSON")
	return cmd
}

// writeJSON writes v as indented JSON, ending with a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
