package watch

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// moveFocus moves the Overview's focus by accounts, in the config's order, up
// where by is less than zero, as ↑ and ↓ do, no further than the first
// account or the last, scrolling USAGE to the account it's on where that's
// out of view.
func (m Model) moveFocus(by int) Model {
	now := m.now()
	accounts := m.doc.At(now, m.cfg.Policy).Accounts
	i := slices.IndexFunc(accounts, func(a status.AccountNow) bool { return a.ID == m.focus })
	if i < 0 {
		return m
	}
	return m.focusedOn(accounts[min(max(i+by, 0), len(accounts)-1)].ID, now)
}

// focusOn moves the Overview's focus to the account with the given id, as a
// click on its row does, scrolling USAGE to it where it's out of view: where
// the document has no such account, it does nothing.
func (m Model) focusOn(id string) Model {
	now := m.now()
	if _, ok := m.doc.At(now, m.cfg.Policy).Account(id); !ok {
		return m
	}
	return m.focusedOn(id, now)
}

// focusedOn is the model with the Overview's focus on the account of the
// document on screen with the given id, USAGE scrolled to it at now where
// it's out of view.
func (m Model) focusedOn(id string, now time.Time) Model {
	m.focus, m.scroll = id, m.frame(now).Revealing(m.doc, now, id)
	return m
}

// stillThere is the model with the focus on the first account of look, in
// the config's order, where it's on none of look's accounts, as when the
// first document arrives, or the document no longer has its account: on none
// while it has no accounts.
func (m Model) stillThere(look status.Moment) Model {
	_, there := look.Account(m.focus)
	switch {
	case there:
	case len(look.Accounts) > 0:
		m.focus = look.Accounts[0].ID
	default:
		m.focus = ""
	}
	return m
}
