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
	"github.com/leeovery/switchboard/internal/setup"
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
// what the walk's last job came to, should the screen have gone first, as
// Model.gone has it; the summary as it's left; and how the token card's
// making left it, as Model.tokenLeft has it. Where the service step's own
// job runs on once moment, given the jobs it waits for, has come, it says on
// out that setup is finishing it.
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
	last = last.gone()
	left := last.walk.left()
	left.Summary = last.leaving()
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

// gone is m once its screen has gone and every job has ended, with what the
// walk's last job came to taken in, as Update takes it, where the screen
// went first, as a swap or a write it was waiting for ends after: while the
// step showing is under way, or the offer card adds the line, as taking it
// in either ends the step or starts another job. The commands it gives are
// dropped, as nothing runs once the screen has gone.
func (m Model) gone() Model {
	if m.walk.done && (m.offer == nil || !m.offer.adding) {
		return m
	}
	select {
	case msg := <-m.walk.kept:
		next, _ := m.Update(msg)
		return next.(Model)
	default:
		return m
	}
}

// errUnanswered is why a token the screen was checking as it went is
// unchecked: no check answers it now.
var errUnanswered = errors.New("setup was left before the API answered")

// leaving is the summary as setup is left: as summary has it, each token
// still to be checked, or being checked, unchecked, as no check answers it
// now.
func (m Model) leaving() setup.Summary {
	s := m.summary()
	if s.Review == nil {
		return s
	}
	review := *s.Review
	for _, a := range s.Review.Accounts {
		review = review.Checked(a.ID, setup.Reviewed{}, errUnanswered)
	}
	s.Review = &review
	return s
}
