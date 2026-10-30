package screen

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/setup"
)

// pasteCard is the card pasting an account's token, as the screen holds it:
// whose token it saves, as the token card it opened from had it; that card,
// where the paste card goes back to it, as tokenCard.backTo has it; its one
// field, hidden, and where the card stands on it; and whether the token's
// being checked and saved, when it takes no key, and how that ends, kept
// for Run, as the screen can go before it hears.
type pasteCard struct {
	form
	f        setup.TokenFor
	from     *tokenCard
	token    entry
	checking bool
	kept     chan pastedMsg
}

// pasting is m with the paste card open for the token f says, in the token
// card's place, its field empty, going back to from, where it's set.
func (m Model) pasting(f setup.TokenFor, from *tokenCard) Model {
	m.token = nil
	m.paste = &pasteCard{chosen: setup.TokenField, f: f, from: from, token: entry{hidden: true}}
	return m
}

// savedOnceGone reports whether the paste being checked as the screen went
// saved its token, as it ended once it had gone: a failure is logged, as
// nothing else can show it.
func (c pasteCard) savedOnceGone() bool {
	if !c.checking {
		return false
	}
	select {
	case ended := <-c.kept:
		if ended.err != nil {
			logger.Error("pasting a token failed once setup's screen had gone", "account", c.f.Account.ID, "error", ended.err)
		}
		return ended.err == nil && ended.tokened.Result == setup.TokenSaved
	default:
		return false
	}
}

// drawn is the card as dashboard.Setup draws it.
func (c pasteCard) drawn() *dashboard.SetupPasteCard {
	return &dashboard.SetupPasteCard{ID: c.f.Account.ID, Token: c.token.field(!c.checking), Checking: c.checking, Why: c.why}
}

// pasted is the card with text pasted into its field, as entry has it.
func (c pasteCard) pasted(text string) pasteCard {
	c.retyped(&c.token, c.token.pasted(text))
	return c
}

// pastePressed acts on a key at the paste card: esc closes it, saving
// nothing, and adding no account it was to add, back to the card it opened
// from, where it goes back; enter saves the token, as pasteSaving has it;
// and its field takes the keys it takes, as entry has them.
func (m Model) pastePressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.token, m.paste = m.paste.from, nil
		return m, nil
	case "enter":
		return m.pasteSaving()
	}
	card := *m.paste
	if card.typed(&card.token, key) {
		m.paste = &card
	}
	return m, nil
}

// pasteSaving saves the token pasted with PasteToken, as a writing action,
// compared with the accounts read, the card checking it, its spinner moving,
// and taking no key till it's answered. It waits, doing nothing, while any
// account's check is out, so the token's compared with every one the run
// will know.
func (m Model) pasteSaving() (tea.Model, tea.Cmd) {
	if m.status.checksOut() {
		return m, nil
	}
	card := *m.paste
	card.checking, card.kept = true, make(chan pastedMsg, 1)
	card.f.Organizations = m.organizations()
	m.paste = &card
	m.spins++
	return m, tea.Batch(m.jobs.write(pasteToken(m.steps, card.f, card.token.value(), card.kept)), m.armSpin())
}

// pastedMsg is what PasteToken answered, saving the paste card's token.
type pastedMsg struct {
	tokened setup.Tokened
	err     error
}

// pasteToken is the job that saves the token pasted for f, with PasteToken,
// how it ended its outcome, and kept on kept, which holds it.
func pasteToken(steps Steps, f setup.TokenFor, pasted string, kept chan<- pastedMsg) job {
	return func(ctx context.Context) tea.Msg {
		tokened, err := steps.PasteToken(ctx, f, pasted)
		ended := pastedMsg{tokened: tokened, err: err}
		kept <- ended
		return ended
	}
}

// pasteSaved takes in what PasteToken answered, while the paste card is
// open: saved, as tokenSaved has it, the card closed; failing, where the
// card goes back to a token made but unsaved, the card it opened from back
// in its place, as a token made that couldn't be saved, as notSaved has it,
// why made safe to show, without what's pasted, and noted in the log, the
// accounts read again, as the save may have changed them, the step going
// on, as a made token's that isn't saved does; failing otherwise, the step
// failed, as tokenSaved has it, the card closed, the token still on the
// clipboard; cancelled, the card closed; else, saving nothing, why, as
// setup words it, said under the field, which is emptied, as nothing in it
// shows to put right, the card open to paste again.
func (m Model) pasteSaved(msg pastedMsg) (Model, tea.Cmd) {
	if m.paste == nil {
		return m, nil
	}
	card := *m.paste
	switch r := msg.tokened.Result; {
	case msg.err != nil && card.from != nil:
		why := redact.Text(msg.err.Error(), card.token.value())
		logger.Warn("a token pasted couldn't be saved", "account", card.f.Account.ID, "error", why)
		m.token, m.paste = card.from.notSaved(why), nil
		return m.askAgain()
	case msg.err != nil:
		m.paste = nil
		return m.tokenSaved(card.f.Account.ID, msg.err)
	case r == setup.TokenSaved:
		m.paste = nil
		return m.tokenSaved(card.f.Account.ID, nil)
	case r == setup.TokenCancelled:
		m.paste = nil
		return m, nil
	}
	card.checking, card.token = false, entry{hidden: true}
	card.form = card.refusedAt(setup.TokenField, msg.tokened.Refusal())
	m.paste = &card
	return m, nil
}

// tokenSaved is m once a token for the account id is saved, as err says:
// the accounts read again, the row chosen moving to the account once
// they're read; or failing, the step failed, not saved, as changed has it.
func (m Model) tokenSaved(id string, err error) (Model, tea.Cmd) {
	if err == nil {
		m.madeFor = id
	}
	return m.changed(err)
}
