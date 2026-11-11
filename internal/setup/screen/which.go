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
// doesn't settle it: whose token it is; how its save ended, asking, the
// token held and the question asked; the token card of the making that
// asked, whose making Run may give back once the screen has gone, nil for a
// paste of none; the choice chosen, by its place among the question's
// answers, a new account first; and whether the answer's being saved, when
// it takes no key, and how that ends, kept for Run, as the screen can go
// before it hears.
type whichCard struct {
	f      setup.TokenFor
	ended  setup.Tokened
	asked  *setup.Asked
	from   *tokenCard
	chosen int
	saving bool
	kept   chan answeredMsg
}

// whichAsked is the card asking which account the token f's save held, as
// ended says, is for, a new account chosen, its making's token card from,
// where there's one.
func whichAsked(f setup.TokenFor, ended setup.Tokened, from *tokenCard) *whichCard {
	return &whichCard{f: f, ended: ended, asked: ended.Asked, from: from}
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
// esc closes it, adding nothing: a token made, which lasts a year on the
// account, is shown in its making's token card, to copy, as one that isn't
// saved is; one pasted, still the user's, goes back to the card the paste
// card opened from, where it goes back, as the paste card's esc does.
func (m Model) whichPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	card := *m.which
	switch key.String() {
	case "esc":
		m.which, m.token = nil, card.from
		if card.made() {
			m.token = card.from.showing(card.ended)
		}
		return m, nil
	case "enter":
		card.saving, card.kept = true, make(chan answeredMsg, 1)
		m.which = &card
		return m, m.jobs.write(answer(m.steps, card.asked, card.chosen, card.kept))
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
// choice at the place given says, with Answer, how it ended its outcome, and
// kept on kept, which holds it.
func answer(steps Steps, asked *setup.Asked, choice int, kept chan<- answeredMsg) job {
	return func(ctx context.Context) tea.Msg {
		tokened, err := steps.Answer(ctx, asked, asked.Answered(choice))
		ended := answeredMsg{tokened: tokened, err: err}
		kept <- ended
		return ended
	}
}

// answered takes in what Answer answered, while the card asking which
// account a token is for is open: saved, or failing, as tokenSaved has it,
// the card closed; asked again, as the accounts it may be have changed since,
// the card asking afresh; cancelled, the card closed; else, saving nothing,
// the token card in its place, saying why, as setup words it, made safe to
// show, to try again: for a token made, the making's own card, and, where
// the answer's ending shows the token, held as how it's left, as heldToken
// has it, else left as the making ended, which shows it.
func (m Model) answered(msg answeredMsg) (Model, tea.Cmd) {
	if m.which == nil {
		return m, nil
	}
	card := *m.which
	m.which = nil
	switch r := msg.tokened.Result; {
	case msg.err != nil || r == setup.TokenSaved:
		return m.tokenSaved(card.f, msg.tokened.ID, msg.err)
	case r == setup.TokenAsked:
		m.which = whichAsked(card.f, msg.tokened, card.from)
	case r == setup.TokenCancelled:
	case card.made():
		if len(msg.tokened.Shown) > 0 {
			m.held = m.held.unsavedAs(card.from.told.making, msg.tokened)
		}
		m.token = card.from.showing(msg.tokened)
	default:
		unsaved := msg.tokened.Unsaved().Clean()
		m.token = &tokenCard{f: card.f, unsaved: &unsaved}
	}
	return m, nil
}

// made reports whether the card's token was made, not pasted: shown as it
// was asked, its making's card there to show it again.
func (c whichCard) made() bool {
	return c.from != nil && len(c.ended.Shown) > 0
}
