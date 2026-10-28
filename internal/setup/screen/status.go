package screen

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/setup"
)

// status is what the status line is said of: whether a router answers and
// the accounts, as Standing and Review last answered, each asked together,
// as the screen starts and whenever askAgain asks.
type status struct {
	// asked numbers the asks made; standingOf and reviewOf the asks the
	// answers held answered, an answer to an earlier ask being dropped.
	asked                int
	standingOf, reviewOf int
	// standing is nil until Standing first answers.
	standing *setup.RouterStanding
	review   setup.Review
}

// standingMsg is what Standing answered to the ask numbered ask.
type standingMsg struct {
	ask      int
	standing setup.RouterStanding
	err      error
}

// reviewMsg is what Review answered to the ask numbered ask.
type reviewMsg struct {
	ask    int
	review setup.Review
	err    error
}

// asking is s with an ask more made.
func (s status) asking() status {
	s.asked++
	return s
}

// line is the status line's parts, as setup.StatusLine has them: none until
// Standing first answers.
func (s status) line() []setup.Part {
	if s.standing == nil {
		return nil
	}
	return setup.StatusLine(*s.standing, s.review)
}

// standingAnswered takes in what Standing answered, but for an answer to an
// ask before the one held, and one failing, as it does only once its
// context is done.
func (s status) standingAnswered(msg standingMsg) status {
	if msg.err != nil || msg.ask < s.standingOf {
		return s
	}
	s.standing, s.standingOf = &msg.standing, msg.ask
	return s
}

// reviewAnswered takes in what Review answered, but for an answer to an ask
// before the one held; one failing leaves the accounts as they were read
// before, noting why in the log.
func (s status) reviewAnswered(msg reviewMsg) status {
	switch {
	case msg.ask < s.reviewOf:
		return s
	case msg.err != nil:
		logger.Warn("couldn't read the accounts for setup's status line", "error", msg.err)
		return s
	}
	s.review, s.reviewOf = msg.review, msg.ask
	return s
}

// ask is the command that asks Standing and Review, beside whatever else
// runs, their answers the ask's as last numbered.
func (m Model) ask() tea.Cmd {
	ask, steps := m.status.asked, m.steps
	return tea.Batch(
		m.jobs.read(func(ctx context.Context) tea.Msg {
			standing, err := steps.Standing(ctx)
			return standingMsg{ask: ask, standing: standing, err: err}
		}),
		m.jobs.read(func(context.Context) tea.Msg {
			review, err := steps.Review()
			return reviewMsg{ask: ask, review: review, err: err}
		}),
	)
}

// askAgain asks Standing and Review again, as a step that changes either has
// them asked.
func (m Model) askAgain() (Model, tea.Cmd) {
	m.status = m.status.asking()
	return m, m.ask()
}
