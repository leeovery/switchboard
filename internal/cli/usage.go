package cli

import (
	"io"
	"strconv"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// defaultWidth is the width the dashboard is drawn at when the terminal's
// isn't known.
const defaultWidth = 80

func newUsageCommand(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Show every account's usage as a dashboard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := a.collect(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			frame := dashboard.Render(doc, a.Now(), dashboard.Options{Width: a.terminalWidth(out), Color: true})
			// The frame is drawn in full colour; the writer brings it down to
			// what the terminal shows, which is none when it isn't one.
			_, err = io.WriteString(colorprofile.NewWriter(out, a.Environ()), frame+"\n")
			return err
		},
	}
}

// terminalWidth is how many cells wide out is: its size when it's a terminal,
// else $COLUMNS, else defaultWidth.
func (a *app) terminalWidth(out io.Writer) int {
	if f, ok := out.(term.File); ok && term.IsTerminal(f.Fd()) {
		if width, _, err := term.GetSize(f.Fd()); err == nil && width > 0 {
			return width
		}
	}
	if columns, err := strconv.Atoi(a.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	return defaultWidth
}
