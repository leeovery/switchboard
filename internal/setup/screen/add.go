package screen

import (
	"context"
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// addFields are the add card's fields, in the order tab goes through them.
var addFields = []setup.Field{setup.IDField, setup.LabelField, setup.PlanField}

// addCard is the card adding an account, as the screen holds it: its
// fields, the plan chosen by its place among config.PlanChoices; where it
// stands among them; and whether the account it gives is being checked,
// when it takes no key.
type addCard struct {
	form
	id, label entry
	plan      int
	checking  bool
}

// addOpened is the add card as it opens: its fields empty, its plan not set,
// ID chosen.
func addOpened() *addCard {
	return &addCard{chosen: setup.IDField, plan: slices.Index(config.PlanChoices(), config.PlanNone)}
}

// drawn is the card as dashboard.Setup draws it.
func (c addCard) drawn() *dashboard.SetupAddCard {
	return &dashboard.SetupAddCard{
		ID: c.id.field(c.chosen == setup.IDField), Label: c.label.field(c.chosen == setup.LabelField),
		Plan: config.PlanChoices()[c.plan], Chosen: c.chosen, Refused: c.refused, Why: c.why,
	}
}

// account is the account the card gives, as it's typed and chosen.
func (c addCard) account() setup.Adding {
	return setup.Adding{ID: c.id.value(), Label: c.label.value(), Plan: config.PlanChoices()[c.plan]}
}

// pressed is the card once key is pressed in its field chosen, reporting
// false for a key it doesn't take: a text field's, as entry has them; Plan's,
// ← and → moving its choice, no further than its first or last.
func (c addCard) pressed(key tea.KeyPressMsg) (addCard, bool) {
	if field := c.typedIn(); field != nil {
		ok := c.typed(field, key)
		return c, ok
	}
	by, ok := moved(key)
	if ok {
		c.plan = planMoved(c.plan, by)
	}
	return c, ok
}

// pasted is the card with text pasted into its field chosen, where it's a
// text field.
func (c addCard) pasted(text string) addCard {
	if field := c.typedIn(); field != nil {
		c.retyped(field, field.pasted(text))
	}
	return c
}

// typedIn is the text field chosen, ID or Label: nil where Plan is.
func (c *addCard) typedIn() *entry {
	switch c.chosen {
	case setup.IDField:
		return &c.id
	case setup.LabelField:
		return &c.label
	}
	return nil
}

// addPressed acts on a key at the add card: esc closes it, adding nothing;
// enter has setup check the account it gives, with CheckAdding, the card
// taking no key till it's checked; tab chooses the next field; and the
// field chosen takes the keys it takes, as addCard.pressed has them.
func (m Model) addPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.add = nil
		return m, nil
	case "enter":
		if m.status.checksOut() {
			return m, nil
		}
		card := *m.add
		card.checking = true
		m.add = &card
		return m, m.jobs.read(checkAdding(m.steps, card.account()))
	case "tab":
		card := *m.add
		card.form = card.next(addFields)
		m.add = &card
		return m, nil
	}
	if card, ok := m.add.pressed(key); ok {
		m.add = &card
	}
	return m, nil
}

// addCheckedMsg is what CheckAdding answered, checking the add card's
// account.
type addCheckedMsg struct {
	account config.NewAccount
	err     error
}

// checkAdding is the job that checks the account adding gives, with
// CheckAdding.
func checkAdding(steps Steps, adding setup.Adding) job {
	return func(context.Context) tea.Msg {
		account, err := steps.CheckAdding(adding)
		return addCheckedMsg{account: account, err: err}
	}
}

// addChecked takes in what CheckAdding answered, while the add card is
// open: where setup refused a value, why, said under its field, the card
// staying open; where it failed otherwise, the step failed, not checked,
// the card closed; else the account goes on to have its token made, as
// tokenFor has it.
func (m Model) addChecked(msg addCheckedMsg) (Model, tea.Cmd) {
	if m.add == nil {
		return m, nil
	}
	card := *m.add
	card.checking = false
	if refused, ok := errors.AsType[*setup.Refused](msg.err); ok {
		card.form = card.refusing(refused)
		m.add = &card
		return m, nil
	}
	m.add = nil
	if msg.err != nil {
		m.walk, m.accountsFailed = m.walk.finished(msg.err), dashboard.AccountsNotChecked
		return m, nil
	}
	return m.tokenFor(msg.account)
}

// tokenFor hands the account the add card has checked on to have its token
// made, the add card closed for the token card, to be added once the token's
// saved, compared with the accounts read, as making has it.
func (m Model) tokenFor(account config.NewAccount) (Model, tea.Cmd) {
	return m.making(setup.TokenFor{Account: account, Add: true, Organizations: m.organizations()})
}
