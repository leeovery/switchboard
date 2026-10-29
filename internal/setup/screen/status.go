package screen

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/setup"
)

// status is what the status line, and the Accounts page, are said of:
// whether a router answers and the accounts, as Standing and Review last
// answered, each asked together, as the screen starts and whenever askAgain
// asks, and each account's token as ReviewOf has checked it. An answer to an
// ask before the last is dropped, so the line says what it says until the
// last ask answers.
type status struct {
	// asked numbers the asks made.
	asked int
	// standing is nil until Standing first answers; reviewed is set once
	// Review first answers, and review is the accounts it read, each token
	// as checked so far, nil where it's read none.
	standing *setup.RouterStanding
	reviewed bool
	review   *setup.Review
	// read is the ask the review was read in answer to.
	read int
	// checking are the ids of the accounts whose tokens ReviewOf is
	// checking.
	checking []string
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

// checkedMsg is what ReviewOf answered of the account id, asked of the
// review read in the ask numbered read.
type checkedMsg struct {
	read int
	id   string
	row  setup.Reviewed
	err  error
}

// asking is s with an ask more made.
func (s status) asking() status {
	s.asked++
	return s
}

// line is the status line's parts, as setup.StatusLine has them: none until
// Standing and Review have both first answered, as the router's part goes by
// the accounts.
func (s status) line() []setup.Part {
	if s.standing == nil || !s.reviewed {
		return nil
	}
	return setup.StatusLine(*s.standing, s.review)
}

// standingAnswered takes in what Standing answered, but for an answer to an
// ask before the last, and one failing, as it does only once its context is
// done.
func (s status) standingAnswered(msg standingMsg) status {
	if msg.err != nil || msg.ask < s.asked {
		return s
	}
	s.standing = &msg.standing
	return s
}

// reviewAnswered takes in what Review answered, but for an answer to an ask
// before the last, each token checked before kept as it was checked while
// it's the same token, as setup.Review.Keeping has it; one failing leaves
// the accounts as they were read before, where they were, noting why in the
// log.
func (s status) reviewAnswered(msg reviewMsg) status {
	if msg.ask < s.asked {
		return s
	}
	s.reviewed = true
	if msg.err != nil {
		logger.Warn("couldn't read the accounts for setup's status line", "error", msg.err)
		return s
	}
	review := msg.review
	if s.review != nil {
		review = review.Keeping(*s.review)
	}
	s.review, s.read = &review, msg.ask
	return s
}

// checkAnswered takes in what ReviewOf answered, noting why in the log where
// it failed: asked of the review as it was read, folded in as
// setup.Review.Checked has it; asked of one read before, as
// setup.Review.Keeping has it, which keeps it only where it's of the token
// read since, and drops a failure, so the token read since is checked.
func (s status) checkAnswered(msg checkedMsg) status {
	s.checking = slices.DeleteFunc(slices.Clone(s.checking), func(id string) bool { return id == msg.id })
	if msg.err != nil {
		logger.Warn("couldn't check an account's token", "account", msg.id, "error", msg.err)
	}
	var review setup.Review
	switch {
	case s.review == nil:
		return s
	case msg.read == s.read:
		review = s.review.Checked(msg.id, msg.row, msg.err)
	case msg.err == nil:
		review = s.review.Keeping(setup.Review{Accounts: []setup.Reviewed{msg.row}})
	default:
		return s
	}
	s.review = &review
	return s
}

// checksOut reports whether any account's token is still to be checked, or
// being checked, so its organization isn't known yet.
func (s status) checksOut() bool {
	return len(s.checking) > 0 || len(s.unchecked()) > 0
}

// unchecked are the ids of the review's accounts whose tokens are read but
// not yet checked, nor being checked.
func (s status) unchecked() []string {
	if s.review == nil {
		return nil
	}
	var ids []string
	for _, a := range s.review.Accounts {
		if a.Token.State == setup.TokenRead && !slices.Contains(s.checking, a.ID) {
			ids = append(ids, a.ID)
		}
	}
	return ids
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

// check is m with every account whose token is read but not yet checked,
// nor being checked, being checked, and the command that checks them with
// ReviewOf, all at once, beside whatever else runs.
func (m Model) check() (Model, tea.Cmd) {
	ids, read, steps := m.status.unchecked(), m.status.read, m.steps
	if len(ids) == 0 {
		return m, nil
	}
	cmds := make([]tea.Cmd, len(ids))
	for i, id := range ids {
		cmds[i] = m.jobs.read(func(ctx context.Context) tea.Msg {
			row, err := steps.ReviewOf(ctx, id)
			return checkedMsg{read: read, id: id, row: row, err: err}
		})
	}
	m.status.checking = append(slices.Clip(m.status.checking), ids...)
	return m, tea.Batch(cmds...)
}
