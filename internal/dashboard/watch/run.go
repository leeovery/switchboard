package watch

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

// ErrNotTerminal is what Run returns when its output isn't a terminal.
var ErrNotTerminal = errors.New("output isn't a terminal")

// Run shows the dashboard full screen on out, which must be a terminal, until
// the user quits. The frame is drawn in full colour and brought down to what
// the terminal shows, judged by out and environ.
func Run(ctx context.Context, cfg Config, out io.Writer, environ []string) error {
	if f, ok := out.(term.File); !ok || !term.IsTerminal(f.Fd()) {
		return ErrNotTerminal
	}
	program := tea.NewProgram(New(ctx, cfg), tea.WithContext(ctx), tea.WithOutput(out), tea.WithEnvironment(environ))
	// In raw mode ctrl+c arrives as a key, so an interrupt comes from outside
	// the terminal, and ends a watch as q does.
	if _, err := program.Run(); err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return fmt.Errorf("run the dashboard: %w", err)
	}
	return nil
}
