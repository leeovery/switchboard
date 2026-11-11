package screen

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// whichCard is the card asking which account a token made, or pasted, to
// add an account is for, as the screen holds it, where its organization
// doesn't settle it: whose token it is; the token held, and the question
// asked; the token card of the making that asked, whose making Run may give
// back once the screen has gone, nil for a paste of none; the choice chosen,
// by its place among the question's answers, a new account first; and
// whether the answer's being saved, when it takes no key.
type whichCard struct {
	f      setup.TokenFor
	asked  *setup.Asked
	from   *tokenCard
	chosen int
	saving bool
}

// whichAsked is the card asking which account the token asked, f's, is
// for, a new account chosen, its making's token card from, where there's
// one.
func whichAsked(f setup.TokenFor, asked *setup.Asked, from *tokenCard) *whichCard {
	return &whichCard{f: f, asked: asked, from: from}
}

// drawn is the card as dashboard.Setup draws it at now, what setup says made
// safe to show.
func (c whichCard) drawn(now time.Time) *dashboard.SetupWhichCard {
	ask := c.asked.Asking(now)
	for i, says := range ask.Says {
		ask.Says[i] = prose.Clean(says)
	}
	for i, choice := range ask.Choices {
		ask.Choices[i].Label = prose.Clean(choice.Label)
	}
	return &dashboard.SetupWhichCard{Label: prose.Clean(c.f.Name()), Ask: ask, Chosen: c.chosen}
}

// whichPressed acts on a key at the card asking which account a token is
// for: ↑ and k choose the answer above, ↓ and j the one below, no further
// than the first or the last; enter saves the token as the answer chosen
// says, as a writing action, the card taking no key till it's answered; and
// esc closes it, adding nothing.
func (m Model) whichPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	card := *m.which
	switch key.String() {
	case "esc":
		m.which = nil
		return m, nil
	case "enter":
		card.saving = true
		m.which = &card
		return m, m.jobs.write(answer(m.steps, card.asked, card.chosen))
	case "up", "k":
		card.chosen = max(card.chosen-1, 0)
	case "down", "j":
		card.chosen = min(card.chosen+1, len(card.asked.Question.Choices))
	}
	m.which = &card
	return m, nil
}

// answeredMsg is what Answer answered, saving the asked token.
type answeredMsg struct {
	tokened setup.Tokened
	err     error
}

// answer is the job that adds the account the asked token is for, as the
// choice at the place given says, with Answer.
func answer(steps Steps, asked *setup.Asked, choice int) job {
	return func(ctx context.Context) tea.Msg {
		tokened, err := steps.Answer(ctx, asked, asked.Answered(choice))
		return answeredMsg{tokened: tokened, err: err}
	}
}

// answered takes in what Answer answered, while the card asking which
// account a token is for is open: saved, or failing, as tokenSaved has it,
// the card closed; asked again, as the accounts it may be have changed since,
// the card asking afresh; cancelled, the card closed; else, saving nothing,
// the token card in its place, saying why, as setup words it, made safe to
// show, to try again.
func (m Model) answered(msg answeredMsg) (Model, tea.Cmd) {
	if m.which == nil {
		return m, nil
	}
	card := *m.which
	m.which = nil
	switch r := msg.tokened.Result; {
	case msg.err != nil || r == setup.TokenSaved:
		return m.tokenSaved(msg.tokened.ID, msg.err)
	case r == setup.TokenAsked:
		m.which = whichAsked(card.f, msg.tokened.Asked, card.from)
	case r != setup.TokenCancelled:
		unsaved := msg.tokened.Unsaved().Clean()
		m.token = &tokenCard{f: card.f, unsaved: &unsaved}
	}
	return m, nil
}
