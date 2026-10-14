package watch

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// moveFocus moves the Overview's focus by accounts, in the config's order, up
// where by is less than zero, as ↑ and ↓ do, no further than the first
// account or the last, scrolling USAGE to the account it's on where that's
// out of view.
func (m Model) moveFocus(by int) Model {
	accounts := m.doc.Accounts
	i := slices.IndexFunc(accounts, func(a status.Account) bool { return a.ID == m.focus })
	if i < 0 {
		return m
	}
	return m.focusOn(accounts[min(max(i+by, 0), len(accounts)-1)].ID)
}

// focusOn moves the Overview's focus to the account with the given id, as a
// click on its row does, scrolling USAGE to it where it's out of view: where
// the document has no such account, it does nothing.
func (m Model) focusOn(id string) Model {
	if !m.has(id) {
		return m
	}
	now := m.now()
	m.focus, m.scroll = id, m.frame(now).Revealing(m.doc, now, id)
	return m
}

// stillThere is the model with the focus on the first account, in the
// config's order, where it's on none, as when the first document arrives, or
// the document no longer has its account: on none while it has no accounts.
func (m Model) stillThere() Model {
	switch {
	case m.has(m.focus):
	case len(m.doc.Accounts) > 0:
		m.focus = m.doc.Accounts[0].ID
	default:
		m.focus = ""
	}
	return m
}

// has reports whether the document on screen has the account with the given
// id.
func (m Model) has(id string) bool {
	_, ok := m.doc.Account(id)
	return ok
}
