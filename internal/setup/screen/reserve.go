package screen

import (
	"context"
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// reserveCard is the primary's reserve card as the screen holds it: the
// primary as the card opened on it; its Reserve, the edit card's field,
// typed in; why setup refused it, where it did; and whether it's being
// saved, when it takes no key.
type reserveCard struct {
	form
	opened  setup.Reviewed
	reserve entry
	saving  bool
}

// drawn is the card as dashboard.Setup draws it.
func (c reserveCard) drawn() *dashboard.SetupReserveCard {
	return &dashboard.SetupReserveCard{ID: c.opened.ID, Reserve: c.reserve.field(true), Why: c.why}
}

// pressed is the card once key is typed in its field, as entry has it,
// reporting false for a key the field doesn't take.
func (c reserveCard) pressed(key tea.KeyPressMsg) (reserveCard, bool) {
	ok := c.typed(&c.reserve, key)
	return c, ok
}

// pasted is the card with text pasted into its field.
func (c reserveCard) pasted(text string) reserveCard {
	c.retyped(&c.reserve, c.reserve.pasted(text))
	return c
}

// askingReserve is the primary's reserve card as it opens, as enter moves on
// from the Accounts page: on a first run, once only, where the primary's
// reserve is unset, its field empty; nil where it doesn't open.
func (m Model) askingReserve() *reserveCard {
	if !m.firstRun || m.reserveAsked {
		return nil
	}
	accounts := m.status.review.Accounts
	i := slices.IndexFunc(accounts, func(a setup.Reviewed) bool { return a.Primary })
	if i < 0 || accounts[i].Reserve != 0 {
		return nil
	}
	return &reserveCard{chosen: setup.ReserveField, opened: accounts[i]}
}

// reservePressed acts on a key at the reserve card: esc closes it, saving
// nothing, and moves on; enter saves it, as reserveEntered has it; and its
// field takes the keys it takes.
func (m Model) reservePressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.reserve = nil
		return m.movedOn()
	case "enter":
		return m.reserveEntered()
	}
	if card, ok := m.reserve.pressed(key); ok {
		m.reserve = &card
	}
	return m, nil
}

// reserveEntered saves the reserve typed with EditAccount, as the edit
// card's Reserve is saved, as a writing action, the card taking no key till
// it's saved: where it's none, typed so or left empty, there's nothing to
// save, and the card's closed, moving on.
func (m Model) reserveEntered() (tea.Model, tea.Cmd) {
	card := *m.reserve
	change := card.opened.ReserveChange(card.reserve.value())
	if change == nil {
		m.reserve = nil
		return m.movedOn()
	}
	card.saving = true
	m.reserve = &card
	id, steps := card.opened.ID, m.steps
	return m, m.jobs.write(func(context.Context) tea.Msg {
		_, err := steps.EditAccount(id, setup.Edit{Reserve: change})
		return reservedMsg{err: err}
	})
}

// reservedMsg is what EditAccount answered, saving the reserve card.
type reservedMsg struct {
	err error
}

// reserved takes in what EditAccount answered, as the edit card's save is
// taken in: where setup refused the reserve, why, said under the field, the
// card staying open; where it failed otherwise, the step failed, the card
// closed; else the card closed, the accounts read again, as the change has
// them, moving on.
func (m Model) reserved(msg reservedMsg) (tea.Model, tea.Cmd) {
	if refused, ok := errors.AsType[*setup.Refused](msg.err); ok && m.reserve != nil {
		card := *m.reserve
		card.saving, card.form = false, card.refusing(refused)
		m.reserve = &card
		return m, nil
	}
	m.reserve = nil
	m, reread := m.changed(msg.err)
	if msg.err != nil {
		return m, reread
	}
	onward, cmd := m.movedOn()
	return onward, tea.Batch(reread, cmd)
}
