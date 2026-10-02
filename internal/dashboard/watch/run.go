package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

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
	return run(ctx, cfg, out, tea.WithEnvironment(environ))
}

// run shows the dashboard on out until the user quits, noting in the log when
// it starts and stops, and puts back the terminal's background as it stops,
// however it stops that Bubble Tea catches: a quit, an interrupt, a
// terminate signal, which it takes as a quit, or a panic it recovers from.
func run(ctx context.Context, cfg Config, out io.Writer, opts ...tea.ProgramOption) error {
	last := New(ctx, cfg)
	program := tea.NewProgram(tracked{Model: last, last: &last}, append(opts, tea.WithContext(ctx), tea.WithOutput(out))...)
	logger.Info("watch started", "interval", cfg.Interval)
	started := time.Now()
	_, err := program.Run()
	last.backdrop.putBack(out)
	logger.Info("watch stopped", "ran", time.Since(started).Round(time.Second))
	// In raw mode ctrl+c arrives as a key, so an interrupt comes from outside
	// the terminal, and ends a watch as q does.
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return fmt.Errorf("run the dashboard: %w", err)
	}
	return nil
}

// tracked is the model as Bubble Tea runs it, each step it takes noted in
// last: Bubble Tea gives back no model from a panic it recovers from, and the
// background the model set is put back all the same.
type tracked struct {
	Model
	last *Model
}

// Update takes in a message as the model does, noting where it leaves it.
func (t tracked) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := t.Model.Update(msg)
	*t.last = next.(Model)
	return tracked{Model: *t.last, last: t.last}, cmd
}
