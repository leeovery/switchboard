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
// the terminal shows, judged by out and environ.
func Run(ctx context.Context, cfg Config, out io.Writer, environ []string) error {
	if f, ok := out.(term.File); !ok || !term.IsTerminal(f.Fd()) {
		return ErrNotTerminal
	}
	return run(ctx, cfg, out, tea.WithEnvironment(environ), tea.WithFilter(heeded))
}

// heeded is msg, for the model to take, but for the mouse moving, or a
// button let go, which nothing on the page acts on: dropped before the
// model takes them, so a drag, which tells of every cell it crosses, never
// has the page laid out and drawn again for each.
func heeded(_ tea.Model, msg tea.Msg) tea.Msg {
	switch msg.(type) {
	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return nil
	}
	return msg
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
