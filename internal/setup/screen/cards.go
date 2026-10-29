package screen

import (
	"context"
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// editFields are the edit card's fields, in the order tab goes through
// them.
var editFields = []setup.Field{setup.LabelField, setup.PlanField, setup.PrimaryField, setup.ReserveField}

// editCard is an account's edit card as the screen holds it: the account as
// the card opened on it, which says what's changed; its fields, the plan
// chosen by its place among config.PlanChoices, and whether Primary's yes is
// chosen; the field chosen; why setup refused a value, and of which field,
// where it did; and whether it's being saved, when it takes no key.
type editCard struct {
	opened         setup.Reviewed
	label, reserve entry
	plan           int
	primary        bool
	chosen         setup.Field
	refused        setup.Field
	why            string
	saving         bool
}

// editing is the edit card of the account a, as it opens: its values as
// they are, Label chosen.
func editing(a setup.Reviewed) *editCard {
	plans := config.PlanChoices()
	plan := slices.Index(plans, a.PlanChosen())
	if plan < 0 {
		plan = len(plans) - 1
	}
	return &editCard{opened: a, label: entryOf(a.Label), reserve: entryOf(a.ReserveTyped()), plan: plan, primary: a.Primary, chosen: setup.LabelField}
}

// drawn is the card as dashboard.Setup draws it.
func (c editCard) drawn() *dashboard.SetupEditCard {
	return &dashboard.SetupEditCard{
		ID: c.opened.ID, Label: c.label.field(c.chosen == setup.LabelField), Reserve: c.reserve.field(c.chosen == setup.ReserveField),
		Plan: config.PlanChoices()[c.plan], Primary: c.primary, Chosen: c.chosen, Refused: c.refused, Why: c.why,
	}
}

// edit is the change the card asks for: each value that differs from the
// account's as it opened on it, and the primary where yes is chosen of an
// account that isn't. It reports false where it asks for none.
func (c editCard) edit() (setup.Edit, bool) {
	var e setup.Edit
	if label := c.label.value(); label != c.opened.Label {
		e.Label = &label
	}
	if plan := config.PlanChoices()[c.plan]; plan != c.opened.PlanChosen() {
		e.Plan = &plan
	}
	if reserve := c.reserve.value(); reserve != c.opened.ReserveTyped() {
		e.Reserve = &reserve
	}
	e.Primary = c.primary && !c.opened.Primary
	return e, e.Label != nil || e.Plan != nil || e.Reserve != nil || e.Primary
}

// next is the card with the field after the one chosen chosen, the first
// after the last.
func (c editCard) next() editCard {
	c.chosen = editFields[(slices.Index(editFields, c.chosen)+1)%len(editFields)]
	return c
}

// pressed is the card once key is pressed in its field chosen, reporting
// false for a key it doesn't take: a text field's, as entry has them; a
// choice's, ← and → moving its choice, no further than its first or last,
// but for Primary on the primary, which doesn't change.
func (c editCard) pressed(key tea.KeyPressMsg) (editCard, bool) {
	if field := c.typedIn(); field != nil {
		now, ok := field.pressed(key)
		c.retyped(field, now)
		return c, ok
	}
	var by int
	switch key.String() {
	case "left":
		by = -1
	case "right":
		by = 1
	default:
		return c, false
	}
	switch {
	case c.chosen == setup.PlanField:
		c.plan = min(max(c.plan+by, 0), len(config.PlanChoices())-1)
	case !c.opened.Primary:
		c.primary = by > 0
	}
	return c, true
}

// pasted is the card with text pasted into its field chosen, where it's a
// text field.
func (c editCard) pasted(text string) editCard {
	if field := c.typedIn(); field != nil {
		c.retyped(field, field.pasted(text))
	}
	return c
}

// typedIn is the text field chosen, Label or Reserve: nil where a choice is.
func (c *editCard) typedIn() *entry {
	switch c.chosen {
	case setup.LabelField:
		return &c.label
	case setup.ReserveField:
		return &c.reserve
	}
	return nil
}

// retyped has field, the text field chosen, holding now, why setup refused
// its value unsaid once that's changed.
func (c *editCard) retyped(field *entry, now entry) {
	if c.refused == c.chosen && now.value() != field.value() {
		c.refused, c.why = setup.NoField, ""
	}
	*field = now
}

// removeCard is the card asking before an account is removed, as the screen
// holds it: the account's id; why setup refused to remove it, where it did;
// and whether it's being removed, when it takes no key.
type removeCard struct {
	id, refused string
	removing    bool
}

// drawn is the card as dashboard.Setup draws it.
func (c removeCard) drawn() *dashboard.SetupRemoveCard {
	return &dashboard.SetupRemoveCard{ID: c.id, Refused: c.refused}
}

// carded reports whether a card is open over the page, taking every key.
func (m Model) carded() bool {
	return m.edit != nil || m.remove != nil
}

// cardPressed acts on a key while a card is open: ctrl+c leaves; else, while
// it isn't being saved, the edit card's keys, as editPressed has them, or the
// remove card's, as removePressed has them. No other key acts, q among
// them, as the key line, empty while a card's open, offers none.
func (m Model) cardPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.String() == "ctrl+c":
		return m, tea.Quit
	case m.edit != nil && !m.edit.saving:
		return m.editPressed(key)
	case m.remove != nil && !m.remove.removing:
		return m.removePressed(key)
	}
	return m, nil
}

// editPressed acts on a key at the edit card: esc closes it, saving
// nothing; enter saves it, as saved has it; tab chooses the next field; and
// the field chosen takes the keys it takes, as editCard.pressed has them.
func (m Model) editPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.edit = nil
		return m, nil
	case "enter":
		return m.saved()
	case "tab":
		card := m.edit.next()
		m.edit = &card
		return m, nil
	}
	if card, ok := m.edit.pressed(key); ok {
		m.edit = &card
	}
	return m, nil
}

// editedMsg is what EditAccount answered, saving the edit card.
type editedMsg struct {
	err error
}

// saved saves the edit card with EditAccount, as a writing action, the card
// taking no key till it's saved: where it asks for no change, it's closed,
// as there's nothing to save.
func (m Model) saved() (tea.Model, tea.Cmd) {
	edit, asks := m.edit.edit()
	if !asks {
		m.edit = nil
		return m, nil
	}
	card := *m.edit
	card.saving = true
	m.edit = &card
	return m, m.jobs.write(editAccount(m.steps, card.opened.ID, edit))
}

// editAccount is the job that edits the account id as edit says, with
// EditAccount.
func editAccount(steps Steps, id string, edit setup.Edit) job {
	return func(context.Context) tea.Msg {
		_, err := steps.EditAccount(id, edit)
		return editedMsg{err: err}
	}
}

// editSaved takes in what EditAccount answered: where setup refused a value,
// why, said under its field, the card staying open; where it failed
// otherwise, the step failed, the card closed; else the card closed, and
// the accounts read again, as the change has them.
func (m Model) editSaved(msg editedMsg) (Model, tea.Cmd) {
	if refused, ok := errors.AsType[*setup.Refused](msg.err); ok && m.edit != nil {
		card := *m.edit
		card.saving, card.refused, card.why = false, refused.Field, prose.Clean(refused.Why)
		m.edit = &card
		return m, nil
	}
	m.edit = nil
	return m.changed(msg.err)
}

// changed is m once a change to the accounts is made, as err says: failed,
// failing the step, not saved; or where a removal took the account out of
// the config, but couldn't delete its token file, not deleted, the accounts
// read again, without it; else, the accounts read again, as the change has
// them, the change settling till they are.
func (m Model) changed(err error) (Model, tea.Cmd) {
	if err == nil {
		m.rereading = true
		return m.askAgain()
	}
	m.walk, m.accountsFailed = m.walk.finished(err), dashboard.AccountsNotSaved
	if _, left := errors.AsType[*accounts.TokenFileLeft](err); left {
		m.accountsFailed = dashboard.AccountsNotDeleted
		return m.askAgain()
	}
	return m, nil
}

// removePressed acts on a key at the remove card: esc closes it, keeping the
// account; enter removes the account with RemoveAccount, as a writing
// action, the card taking no key till it's removed.
func (m Model) removePressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.remove = nil
	case "enter":
		card := *m.remove
		card.removing = true
		m.remove = &card
		return m, m.jobs.write(removeAccount(m.steps, card.id))
	}
	return m, nil
}

// removedMsg is what RemoveAccount answered, removing an account.
type removedMsg struct {
	err error
}

// removeAccount is the job that removes the account id, with RemoveAccount.
func removeAccount(steps Steps, id string) job {
	return func(context.Context) tea.Msg {
		_, err := steps.RemoveAccount(id)
		return removedMsg{err: err}
	}
}

// removed takes in what RemoveAccount answered: where setup refused, why,
// said in the card, which stays open; where it failed otherwise, the step
// failed, the card closed; else the card closed, and the accounts read
// again, without the one removed.
func (m Model) removed(msg removedMsg) (Model, tea.Cmd) {
	if refused, ok := errors.AsType[*setup.Refused](msg.err); ok && m.remove != nil {
		card := *m.remove
		card.removing, card.refused = false, prose.Clean(refused.Why)
		m.remove = &card
		return m, nil
	}
	m.remove = nil
	return m.changed(msg.err)
}

// primaryMsg is what EditAccount answered, making an account the primary.
type primaryMsg struct {
	err error
}

// madePrimary makes the account a the primary with EditAccount, as the edit
// card's Primary does, as a writing action, counted in marking till it's
// answered: where it's the primary already, there's nothing to make.
func (m Model) madePrimary(a setup.Reviewed) (Model, tea.Cmd) {
	if a.Primary {
		return m, nil
	}
	m.marking++
	id, steps := a.ID, m.steps
	return m, m.jobs.write(func(context.Context) tea.Msg {
		_, err := steps.EditAccount(id, setup.Edit{Primary: true})
		return primaryMsg{err: err}
	})
}

// marked takes in what EditAccount answered, making an account the primary,
// as changed has it.
func (m Model) marked(msg primaryMsg) (Model, tea.Cmd) {
	m.marking--
	return m.changed(msg.err)
}
