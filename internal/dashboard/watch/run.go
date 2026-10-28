package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
)

// ErrNotTerminal is what Run returns when its output isn't a terminal.
var ErrNotTerminal = errors.New("output isn't a terminal")

// Run shows the dashboard full screen on out, which must be a terminal, until
// the user quits. The frame is drawn in full colour and brought down to what
// the terminal shows, judged by out and environ. The mouse moving, or a
// button let go, never reaches the model, as backdrop.Heeded has it.
func Run(ctx context.Context, cfg Config, out io.Writer, environ []string) error {
	if f, ok := out.(term.File); !ok || !term.IsTerminal(f.Fd()) {
		return ErrNotTerminal
	}
	return run(ctx, cfg, out, tea.WithEnvironment(environ), tea.WithFilter(backdrop.Heeded))
}

// run shows the dashboard on out until the user quits, noting in the log when
// it starts and stops, and puts back the terminal's background as it stops,
// however it stops, as backdrop.Run has it.
func run(ctx context.Context, cfg Config, out io.Writer, opts ...tea.ProgramOption) error {
	logger.Info("watch started", "interval", cfg.Interval)
	started := time.Now()
	_, err := backdrop.Run(New(ctx, cfg), out, append(opts, tea.WithContext(ctx))...)
	logger.Info("watch stopped", "ran", time.Since(started).Round(time.Second))
	// In raw mode ctrl+c arrives as a key, so an interrupt comes from outside
	// the terminal, and ends a watch as q does.
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return fmt.Errorf("run the dashboard: %w", err)
	}
	return nil
}
