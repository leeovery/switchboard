package cli

import (
	"io"

	"github.com/spf13/cobra"
)

func newVersionCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, as --version does",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), cmd.Root().DisplayName()+" version "+a.Version+"\n")
			return err
		},
	}
}
