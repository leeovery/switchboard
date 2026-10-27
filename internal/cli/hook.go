package cli

import (
	"io"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/sessionhook"
)

// newHookCommand is the hooks Claude Code runs, which setup puts in its
// settings: hidden, as no one runs them by hand.
func newHookCommand(a *app) *cobra.Command {
	hook := &cobra.Command{
		Use:    "hook",
		Short:  "The hooks Claude Code runs, which setup installs",
		Hidden: true,
		// Runnable, so a mistyped subcommand is an error rather than help.
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	hook.AddCommand(&cobra.Command{
		Use:   "session-start",
		Short: "Tell a Claude Code session that didn't start through switchboard so, as it starts",
		Long: `Run by Claude Code as each session starts, it tells a session that didn't
start through switchboard so, and that switchboard setup puts it right. It
reads Claude Code's hook input on stdin, asks nothing of the router or the
config, and always succeeds.`,
		Args: noArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			a.sessionStart(cmd.InOrStdin(), cmd.OutOrStdout())
		},
	})
	return hook
}

// sessionStart prints what a session starting hears from switchboard, from
// Claude Code's hook input on in, to out. It never fails the hook, so never
// holds the session up: what goes wrong is logged, and it says nothing.
func (a *app) sessionStart(in io.Reader, out io.Writer) {
	var input []byte
	if !a.TerminalInput(in) {
		read, err := sessionhook.ReadInput(in)
		if err != nil {
			logger.Warn("can't read the session hook's input", "error", err)
		}
		input = read
	}
	notice := sessionhook.Notice(input, launch.StartedBy(a.Getenv))
	if notice == nil {
		return
	}
	if _, err := out.Write(notice); err != nil {
		logger.Warn("can't tell the session it isn't going through switchboard", "error", err)
	}
}

// IsTerminalInput reports whether in is a terminal: Deps.TerminalInput, for
// the process's own input.
func IsTerminalInput(in io.Reader) bool {
	f, ok := in.(term.File)
	return ok && term.IsTerminal(f.Fd())
}
