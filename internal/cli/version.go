package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/redact"
)

// versionSaid is what --version, and the version command, print between the
// program's name and its version, as "switchboard version 1.2.3". A
// dashboard asks an upgrade of itself for its version, as versionIn reads
// it, so the form changes only in ways an older version can still read.
const versionSaid = " version "

// versionTemplate is what --version prints, as the version command does.
const versionTemplate = "{{.DisplayName}}" + versionSaid + "{{.Version}}\n"

func newVersionCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, as --version does",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), cmd.Root().DisplayName()+versionSaid+a.Version+"\n")
			return err
		},
	}
}

// versionIn reads the version a switchboard binary printed, out, as
// --version prints it. It fails where out says none, as one word after
// versionSaid.
func versionIn(out []byte) (string, error) {
	said := strings.TrimSpace(string(out))
	_, version, ok := strings.Cut(said, versionSaid)
	if !ok || version == "" || strings.ContainsFunc(version, unicode.IsSpace) {
		return "", fmt.Errorf("it printed %q, not its version", redact.Text(said))
	}
	return version, nil
}
