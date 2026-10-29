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
// stands among them; whether the account it gives is being checked, when it
// takes no key; and the account as setup last checked it, waiting for its
// token, as tokenFor leaves it.
type addCard struct {
	form
	id, label entry
	plan      int
	checking  bool
	waiting   *config.NewAccount
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
// ← and → moving its choice, no further than its first or last. The account
// waiting is dropped where the key changes the account the card gives.
func (c addCard) pressed(key tea.KeyPressMsg) (addCard, bool) {
	was := c.account()
	ok := false
	if field := c.typedIn(); field != nil {
		ok = c.typed(field, key)
	} else if by, moves := moved(key); moves {
		c.plan, ok = planMoved(c.plan, by), true
	}
	return c.changedFrom(was), ok
}

// pasted is the card with text pasted into its field chosen, where it's a
// text field, the account waiting dropped where that changes it.
func (c addCard) pasted(text string) addCard {
	was := c.account()
	if field := c.typedIn(); field != nil {
		c.retyped(field, field.pasted(text))
	}
	return c.changedFrom(was)
}

// changedFrom is c, the account waiting dropped where the account c gives
// isn't was, so the one waiting is only ever the one the card shows.
func (c addCard) changedFrom(was setup.Adding) addCard {
	if c.account() != was {
		c.waiting = nil
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
// staying open, no account waiting; where it failed otherwise, the step
// failed, not checked, the card closed; else the account goes on to have
// its token made, as tokenFor has it.
func (m Model) addChecked(msg addCheckedMsg) (Model, tea.Cmd) {
	if m.add == nil {
		return m, nil
	}
	card := *m.add
	card.checking = false
	if refused, ok := errors.AsType[*setup.Refused](msg.err); ok {
		card.form, card.waiting = card.refusing(refused), nil
		m.add = &card
		return m, nil
	}
	if msg.err != nil {
		m.add = nil
		m.walk, m.accountsFailed = m.walk.finished(msg.err), dashboard.AccountsNotChecked
		return m, nil
	}
	m.add = &card
	return m.tokenFor(msg.account)
}

// tokenFor hands the account the add card has checked on to have its token
// made, in the token card, and be added once it's saved. Until there's a
// token card, the add card holds the account, waiting, and stays open,
// adding nothing; esc closes it, as ever, the account dropped.
func (m Model) tokenFor(account config.NewAccount) (Model, tea.Cmd) {
	card := *m.add
	card.waiting = &account
	m.add = &card
	return m, nil
}
