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

// form is where a card of setup's stands among its fields, as the screen
// holds it: the field chosen; and why setup refused a value, and of which
// field, where it did.
type form struct {
	chosen, refused setup.Field
	why             string
}

// next is f with the field after the one chosen of fields, in the order tab
// goes through them, chosen, the first after the last.
func (f form) next(fields []setup.Field) form {
	f.chosen = fields[(slices.Index(fields, f.chosen)+1)%len(fields)]
	return f
}

// typed has field, the text field chosen, take key, as entry has it, why
// setup refused its value unsaid once that's changed. It reports false for
// a key the field doesn't take.
func (f *form) typed(field *entry, key tea.KeyPressMsg) bool {
	now, ok := field.pressed(key)
	f.retyped(field, now)
	return ok
}

// retyped has field, the text field chosen, holding now, why setup refused
// its value unsaid once that's changed.
func (f *form) retyped(field *entry, now entry) {
	if f.refused == f.chosen && now.value() != field.value() {
		f.refused, f.why = setup.NoField, ""
	}
	*field = now
}

// refusing is f saying why setup refused a value, as refused has it, made
// safe to show, under the field it's of.
func (f form) refusing(refused *setup.Refused) form {
	return f.refusedAt(refused.Field, refused.Why)
}

// refusedAt is f saying why setup refused the value of field, made safe to
// show, under it.
func (f form) refusedAt(field setup.Field, why string) form {
	f.refused, f.why = field, prose.Clean(why)
	return f
}

// moved is how far key moves a choice: ← an option back, → one on; false
// for any other key.
func moved(key tea.KeyPressMsg) (by int, ok bool) {
	switch key.String() {
	case "left":
		return -1, true
	case "right":
		return 1, true
	}
	return 0, false
}

// planMoved is the plan by options on from plan, by their places among
// config.PlanChoices, no further than the first or the last.
func planMoved(plan, by int) int {
	return min(max(plan+by, 0), len(config.PlanChoices())-1)
}

// editCard is an account's edit card as the screen holds it: the account as
// the card opened on it, which says what's changed; its fields, the plan
// chosen by its place among config.PlanChoices, and whether Primary's yes is
// chosen; where it stands among them; and whether it's being saved, when it
// takes no key.
type editCard struct {
	form
	opened         setup.Reviewed
	label, reserve entry
	plan           int
	primary        bool
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
	return &editCard{chosen: setup.LabelField, opened: a, label: entryOf(a.Label), reserve: entryOf(a.ReserveTyped()), plan: plan, primary: a.Primary}
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

// pressed is the card once key is pressed in its field chosen, reporting
// false for a key it doesn't take: a text field's, as entry has them; a
// choice's, ← and → moving its choice, no further than its first or last,
// but for Primary on the primary, which doesn't change.
func (c editCard) pressed(key tea.KeyPressMsg) (editCard, bool) {
	if field := c.typedIn(); field != nil {
		ok := c.typed(field, key)
		return c, ok
	}
	by, ok := moved(key)
	switch {
	case !ok:
	case c.chosen == setup.PlanField:
		c.plan = planMoved(c.plan, by)
	case !c.opened.Primary:
		c.primary = by > 0
	}
	return c, ok
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
	return m.add != nil || m.edit != nil || m.remove != nil || m.token != nil || m.paste != nil
}

// cardPressed acts on a key while a card is open: ctrl+c leaves; else, while
// it isn't being checked or saved, the add card's keys, as addPressed has
// them, the edit card's, as editPressed has them, the remove card's, as
// removePressed has them, or the paste card's, as pastePressed has them; or
// the token card's, as tokenPressed has them. No other key acts, q among
// them, as the key line, empty while a card's open, offers none.
func (m Model) cardPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.String() == "ctrl+c":
		return m, tea.Quit
	case m.add != nil && !m.add.checking:
		return m.addPressed(key)
	case m.edit != nil && !m.edit.saving:
		return m.editPressed(key)
	case m.remove != nil && !m.remove.removing:
		return m.removePressed(key)
	case m.paste != nil && !m.paste.checking:
		return m.pastePressed(key)
	case m.token != nil:
		return m.tokenPressed(key)
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
		card := *m.edit
		card.form = card.next(editFields)
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
		card.saving, card.form = false, card.refusing(refused)
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
