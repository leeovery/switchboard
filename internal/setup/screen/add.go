package screen

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// addCard is the card adding an account, as the screen holds it: its
// fields, the plan chosen by its place among setup.AddPlans, noPlan where
// none is, and its Pool, nil in a config without pools; where it stands
// among them; and whether the account it gives is being checked, when it
// takes no key.
type addCard struct {
	form
	label    entry
	plan     int
	pool     *poolField
	checking bool
}

// addOpened is the add card over review as it opens: its fields empty, no
// plan chosen, as the plan is the user's to say, and its pool, where the config has pools, the one accounts
// add adds an account to without --pool, else, where every pool is fenced,
// the first; Label chosen.
func addOpened(review *setup.Review) *addCard {
	var implicit string
	if review != nil {
		if p, ok := review.Pools.Implicit(); ok {
			implicit = p.ID
		}
	}
	return &addCard{chosen: setup.LabelField, plan: noPlan, pool: poolOf(review, implicit)}
}

// noPlan is the add card's plan where none is chosen.
const noPlan = -1

// planChosen is the plan chosen, "" where none is.
func (c addCard) planChosen() string {
	if c.plan == noPlan {
		return ""
	}
	return setup.AddPlans()[c.plan]
}

// fields are the card's fields, in the order tab goes through them: Label,
// Plan, and Pool where it has one.
func (c addCard) fields() []setup.Field {
	if c.pool == nil {
		return []setup.Field{setup.LabelField, setup.PlanField}
	}
	return []setup.Field{setup.LabelField, setup.PlanField, setup.PoolField}
}

// drawn is the card as dashboard.Setup draws it.
func (c addCard) drawn() *dashboard.SetupAddCard {
	card := &dashboard.SetupAddCard{
		Label: c.label.field(c.chosen == setup.LabelField),
		Plans: setup.AddPlans(), Plan: c.planChosen(), Chosen: c.chosen, Refused: c.refused, Why: c.why,
	}
	if c.pool != nil {
		card.Pool = c.pool.drawn(c.pool.usedBy())
	}
	return card
}

// account is the account the card gives, as it's typed and chosen.
func (c addCard) account() setup.Adding {
	adding := setup.Adding{Label: c.label.value(), Plan: c.planChosen()}
	if c.pool != nil {
		adding.Pool, adding.NewPool = c.pool.placed()
	}
	return adding
}

// pressed is the card once key is pressed in its field chosen, reporting
// false for a key it doesn't take: a text field's, as entry has them; a
// choice's, ← and → moving its choice, as planMoved moves Plan's, no further
// than its first or last, and unsaying a refusal of it.
func (c addCard) pressed(key tea.KeyPressMsg) (addCard, bool) {
	if field := c.typedIn(); field != nil {
		ok := c.typed(field, key)
		return c, ok
	}
	by, ok := moved(key)
	switch {
	case !ok:
	case c.chosen == setup.PoolField:
		pool := c.pool.moved(by)
		c.pool, c.form = &pool, c.unrefused(setup.PoolField)
	default:
		c.plan, c.form = planMoved(c.plan, by, len(setup.AddPlans())), c.unrefused(setup.PlanField)
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

// typedIn is the text field chosen, Label: nil where a choice is.
func (c *addCard) typedIn() *entry {
	if c.chosen == setup.LabelField {
		return &c.label
	}
	return nil
}

// makingPool reports whether enter opens the new pool card: where the card's
// Pool is on new pool, whichever field is chosen, as the account has no pool
// to go in till one's made.
func (c addCard) makingPool() bool {
	if c.pool == nil {
		return false
	}
	_, chosen := c.pool.choice()
	return !chosen
}

// addPressed acts on a key at the add card: esc closes it, adding nothing;
// enter, with Pool on new pool, opens the new pool card over it, holding the
// new pool it holds, where it holds one, and elsewhere has setup check the
// account it gives, with CheckAdding, the card taking no key till it's
// checked; tab chooses the next field; and the field chosen takes the keys
// it takes, as addCard.pressed has them.
func (m Model) addPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.add = nil
		return m, nil
	case "enter":
		if m.add.makingPool() {
			m.newPool = newPoolOpened(m.add.pool.made)
			return m, nil
		}
		if m.status.checksOut() {
			return m, nil
		}
		card := *m.add
		card.checking = true
		m.add = &card
		return m, m.jobs.read(checkAdding(m.steps, card.account()))
	case "tab":
		card := *m.add
		card.form = card.next(card.fields())
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
	adding accounts.Adding
	err    error
}

// checkAdding is the job that checks the account adding gives, with
// CheckAdding.
func checkAdding(steps Steps, adding setup.Adding) job {
	return func(context.Context) tea.Msg {
		checked, err := steps.CheckAdding(adding)
		return addCheckedMsg{adding: checked, err: err}
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
	return m.tokenFor(msg.adding)
}

// tokenFor hands the account the add card has checked on to have its token
// made, the add card closed for the token card, to be added once the token's
// saved, with the new pool it goes in, where it goes in one, compared with
// the accounts read, as making has it.
func (m Model) tokenFor(adding accounts.Adding) (Model, tea.Cmd) {
	return m.making(setup.TokenFor{Account: adding.Account, NewPool: adding.NewPool, Add: true, Organizations: m.organizations()})
}
