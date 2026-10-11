package watch

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// cardKey acts on a key of the Overview's cards, reporting false for a key
// it doesn't take, or while there are no cards: ← and → move the focus along
// a row of cards, ↑ and ↓ between rows; and space flips the card with the
// focus, and s every card, or back. Where no card has the focus yet, an
// arrow gives it to the first card in view, and space and s give it there
// before they flip. The cards are laid out once for a key, as the screen
// lays them out.
func (m Model) cardKey(key string) (Model, bool) {
	if m.view != dashboard.Overview || len(m.doc.Accounts) == 0 {
		return m, false
	}
	switch key {
	case "left":
		return m.along(m.cards(), -1), true
	case "right":
		return m.along(m.cards(), 1), true
	case "up":
		return m.vertically(m.cards(), -1), true
	case "down":
		return m.vertically(m.cards(), 1), true
	case "space":
		return m.flip(m.cards()), true
	case "s":
		return m.flipEvery(m.cards()), true
	}
	return m, false
}

// cards are the document's cards as the screen lays them out now.
func (m Model) cards() dashboard.Cards {
	now := m.now()
	return m.frame(now).Cards(m.doc, now)
}

// along moves the focus by cards along its row, to the left where by is less
// than zero: no further than the row goes.
func (m Model) along(cards dashboard.Cards, by int) Model {
	if m.focus == "" {
		return m.focusFirst(cards)
	}
	id, ok := cards.Neighbour(m.focus, by, 0)
	return m.focusOn(cards, id, ok)
}

// vertically moves the focus to the card below, or above where by is less
// than zero, where there's one.
func (m Model) vertically(cards dashboard.Cards, by int) Model {
	if m.focus == "" {
		return m.focusFirst(cards)
	}
	id, ok := cards.Neighbour(m.focus, 0, by)
	return m.focusOn(cards, id, ok)
}

// flip turns the card with the focus over to its back, or back to its
// front.
func (m Model) flip(cards dashboard.Cards) Model {
	if m.focus == "" {
		m = m.focusFirst(cards)
	}
	flipped := maps.Clone(m.flipped)
	if flipped[m.focus] {
		delete(flipped, m.focus)
	} else {
		flipped = with(flipped, m.focus)
	}
	m.flipped = flipped
	return m
}

// flipEvery turns every card over to its back, or, where every one is
// already, back to its front.
func (m Model) flipEvery(cards dashboard.Cards) Model {
	if m.focus == "" {
		m = m.focusFirst(cards)
	}
	if m.everyFlipped() {
		m.flipped = nil
		return m
	}
	var flipped map[string]bool
	for _, a := range m.doc.Accounts {
		flipped = with(flipped, a.ID)
	}
	m.flipped = flipped
	return m
}

// everyFlipped reports whether every card is flipped.
func (m Model) everyFlipped() bool {
	return !slices.ContainsFunc(m.doc.Accounts, func(a status.Account) bool { return !m.flipped[a.ID] })
}

// with is the set given with id in it, made where it's nil.
func with(set map[string]bool, id string) map[string]bool {
	if set == nil {
		set = make(map[string]bool)
	}
	set[id] = true
	return set
}

// focusFirst gives the focus to the first of the cards in view.
func (m Model) focusFirst(cards dashboard.Cards) Model {
	id := cards.InView()
	return m.focusOn(cards, id, id != "")
}

// focusOn gives the focus to the card of the account with the given id, of
// the cards, where ok says there's one, and scrolls the cards as far as they
// must go to show it whole.
func (m Model) focusOn(cards dashboard.Cards, id string, ok bool) Model {
	if !ok {
		return m
	}
	m.focus = id
	m.scroll = cards.Reveal(id)
	return m
}

// stillThere is the model with the focus given up, and the cards no longer
// flipped, where the document no longer has their accounts.
func (m Model) stillThere() Model {
	if !m.has(m.focus) {
		m.focus = ""
	}
	flipped := maps.Clone(m.flipped)
	maps.DeleteFunc(flipped, func(id string, _ bool) bool { return !m.has(id) })
	m.flipped = flipped
	return m
}

// has reports whether the document on screen has the account with the given
// id.
func (m Model) has(id string) bool {
	_, ok := m.doc.Account(id)
	return ok
}
