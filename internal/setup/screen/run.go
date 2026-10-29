package screen

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

// Run shows setup's screen full on out, which must be a terminal, until the
// user leaves it, and says how they left it. The screen is drawn in full
// colour and brought down to what the terminal shows, judged by out and
// environ. The mouse moving, or a button let go, never reaches the model, as
// backdrop.Heeded has it.
func Run(ctx context.Context, cfg Config, out io.Writer, environ []string) (Left, error) {
	if f, ok := out.(term.File); !ok || !term.IsTerminal(f.Fd()) {
		return Left{}, ErrNotTerminal
	}
	return run(ctx, cfg, out, waitedSaid, tea.WithEnvironment(environ), tea.WithFilter(backdrop.Heeded))
}

// run shows setup's screen on out until the user leaves it, putting back the
// terminal's background however it stops, as backdrop.Run has it, and only
// once every job of setup's still running has been cancelled and has ended,
// says how it was left: as the screen last stood, whatever stopped it, with
// how the token card's making left it, as Model.tokenLeft has it. Where the
// service step's own job runs on once moment, given the jobs it waits for,
// has come, it says on out that setup is finishing it.
func run(ctx context.Context, cfg Config, out io.Writer, moment func(*jobs) <-chan time.Time, opts ...tea.ProgramOption) (Left, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := New(ctx, cfg)
	logger.Info("setup's screen started", "step", m.walk.step.Title())
	last, err := backdrop.Run(m, out, append(opts, tea.WithContext(ctx))...)
	cancel()
	waitSaying(m.jobs.wait, moment(m.jobs), func() {
		if finishing(last.walk, m.jobs) {
			_, _ = fmt.Fprintln(out, finishingSaid)
		}
	})
	left := last.walk.left()
	left.Unsaved, left.Interrupted = last.tokenLeft()
	logger.Info("setup's screen stopped", "step", last.walk.step.Title(), "failed", left.Ending == Failed)
	// In raw mode ctrl+c arrives as a key, so an interrupt comes from outside
	// the terminal, and leaves setup as q does.
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return left, fmt.Errorf("run setup's screen: %w", err)
	}
	return left, nil
}

// waitingSaid is how long the screen, once it has gone, waits for setup's
// jobs to end before it says it's waiting.
const waitingSaid = 500 * time.Millisecond

// waitedSaid is the moment run says it's waiting: waitingSaid on.
func waitedSaid(*jobs) <-chan time.Time {
	return time.After(waitingSaid)
}

// waitSaying waits as wait does, saying so with say should it still be
// waiting once moment comes.
func waitSaying(wait func(), moment <-chan time.Time, say func()) {
	waited := make(chan struct{})
	go func() {
		wait()
		close(waited)
	}()
	select {
	case <-waited:
		return
	case <-moment:
		say()
	}
	<-waited
}

// finishing reports whether setup, its screen gone, is finishing the service
// step: the walk standing there while it's under way, and a write, its job,
// running still.
func finishing(w walk, js *jobs) bool {
	return w.finishingService() && js.writes()
}
