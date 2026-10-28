package screen

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
)

// ErrNotTerminal is what Run returns when its output isn't a terminal.
var ErrNotTerminal = errors.New("output isn't a terminal")

// Run shows setup's screen full on out, which must be a terminal, until the
// user leaves it, and says how they left it. The screen is drawn in full
// colour and brought down to what the terminal shows, judged by out and
// environ.
func Run(ctx context.Context, cfg Config, out io.Writer, environ []string) (Left, error) {
	if f, ok := out.(term.File); !ok || !term.IsTerminal(f.Fd()) {
		return Left{}, ErrNotTerminal
	}
	return run(ctx, cfg, out, tea.WithEnvironment(environ))
}

// run shows setup's screen on out until the user leaves it, putting back the
// terminal's background however it stops, as backdrop.Run has it, and only
// once every job of setup's still running has been cancelled and has ended,
// says how it was left: as the screen last stood, whatever stopped it.
func run(ctx context.Context, cfg Config, out io.Writer, opts ...tea.ProgramOption) (Left, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := New(ctx, cfg)
	logger.Info("setup's screen started", "step", m.walk.step.Title())
	last, err := backdrop.Run(m, out, append(opts, tea.WithContext(ctx))...)
	cancel()
	m.jobs.wait()
	left := last.walk.left()
	logger.Info("setup's screen stopped", "step", last.walk.step.Title(), "failed", left.Ending == Failed)
	// In raw mode ctrl+c arrives as a key, so an interrupt comes from outside
	// the terminal, and leaves setup as q does.
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return left, fmt.Errorf("run setup's screen: %w", err)
	}
	return left, nil
}
