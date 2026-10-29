package screen

import (
	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// reviewed takes in what Review answered, as status.reviewAnswered has it:
// at Accounts, the step done once the accounts are first read, failed where
// they couldn't be, and its chosen row kept among them each time they're
// read, as fewer may be read than before.
func (m Model) reviewed(msg reviewMsg) Model {
	m.status = m.status.reviewAnswered(msg)
	if m.walk.step != dashboard.SetupAccounts || !m.status.reviewed {
		return m
	}
	if !m.walk.done {
		var err error
		if m.status.review == nil {
			err = msg.err
		}
		m.walk = m.walk.finished(err)
	}
	return m.chose(0)
}

// rows is how many rows the Accounts page chooses between: an account's
// each, and + add an account.
func (m Model) rows() int {
	if m.status.review == nil {
		return 1
	}
	return len(m.status.review.Accounts) + 1
}

// adding reports whether + add an account is the row chosen.
func (m Model) adding() bool {
	return m.chosen == m.rows()-1
}

// choosing is how far the key moves the Accounts page's row chosen,
// reporting false for a key that doesn't, or off that page: ↑ and k a row
// up, ↓ and j a row down.
func (m Model) choosing(key string) (by int, ok bool) {
	if m.walk.step != dashboard.SetupAccounts {
		return 0, false
	}
	switch key {
	case "up", "k":
		return -1, true
	case "down", "j":
		return 1, true
	}
	return 0, false
}

// chose is m with the row by rows on from the one chosen chosen, no further
// than the first or the last, kept in view.
func (m Model) chose(by int) Model {
	m.chosen = min(max(m.chosen+by, 0), m.rows()-1)
	return m.chosenInView()
}

// chosenInView is m with the Accounts page scrolled as little as keeps its
// row chosen in view, as dashboard.Setup.ScrolledToChosen has it.
func (m Model) chosenInView() Model {
	if m.walk.step == dashboard.SetupAccounts {
		m.scroll = m.page().ScrolledToChosen(m.size.Width, m.size.Height)
	}
	return m
}

// accountsEntered acts on enter at the Accounts page, once its accounts are
// read: on an account's row, it moves on, changing nothing, but where two
// accounts or more have none the primary, which waits for one; on + add an
// account, it does nothing yet.
func (m Model) accountsEntered() (tea.Model, tea.Cmd) {
	if m.adding() || m.status.review.Unmarked {
		return m, nil
	}
	return m.movedOn()
}

// accountsPage is what the Accounts page has of its own, as dashboard.Setup
// draws it: the row chosen. Its accounts, as the status line has them, and
// why they couldn't be read, where the step failed, are the summary's.
func (m Model) accountsPage() *dashboard.SetupAccountsPage {
	return &dashboard.SetupAccountsPage{Chosen: m.chosen}
}
