package watch

import (
	"maps"
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// cardKey acts on a key of the Accounts view's cards, reporting false for a
// key it doesn't take, or while there are no cards: ← and → move the focus
// along a row of cards, ↑ and ↓ between rows, over the sessions on the back
// of a flipped card first, picking them out; space flips the card with the
// focus, and s every card, or back; and esc ends the selection. Where no
// card has the focus yet, an arrow gives it to the first card in view, and
// space and s give it there before they flip.
func (m Model) cardKey(key string) (Model, bool) {
	if m.view != dashboard.Accounts || len(m.doc.Accounts) == 0 {
		return m, false
	}
	switch key {
	case "left":
		return m.along(-1), true
	case "right":
		return m.along(1), true
	case "up":
		return m.vertically(-1), true
	case "down":
		return m.vertically(1), true
	case "space":
		return m.flip(), true
	case "s", "S":
		return m.flipEvery(), true
	case "esc":
		m.selected = dashboard.Seat{}
		return m, true
	}
	return m, false
}

// along moves the focus by cards along its row, to the left where by is less
// than zero, ending the selection: no further than the row goes.
func (m Model) along(by int) Model {
	if m.focus == "" {
		return m.focusFirst()
	}
	return m.focusOn(m.neighbour(by, 0))
}

// vertically moves down by one, or up where by is less than zero: over the
// sessions on the back of the card with the focus first, where it's
// flipped, picking out the next of them, or where none is picked out, the
// first going down and the last going up; then on to the card below or
// above, ending the selection, where there's one.
func (m Model) vertically(by int) Model {
	if m.focus == "" {
		return m.focusFirst()
	}
	seats := m.seats()
	i := slices.Index(seats, m.selected)
	switch {
	case len(seats) > 0 && i < 0 && by > 0:
		m.selected = seats[0]
	case len(seats) > 0 && i < 0:
		m.selected = seats[len(seats)-1]
	case i >= 0 && i+by >= 0 && i+by < len(seats):
		m.selected = seats[i+by]
	default:
		return m.focusOn(m.neighbour(0, by))
	}
	return m
}

// flip turns the card with the focus over to its back, or back to its
// front, ending the selection.
func (m Model) flip() Model {
	if m.focus == "" {
		m = m.focusFirst()
	}
	flipped := maps.Clone(m.flipped)
	if flipped[m.focus] {
		delete(flipped, m.focus)
	} else {
		flipped = with(flipped, m.focus)
	}
	m.flipped, m.selected = flipped, dashboard.Seat{}
	return m
}

// flipEvery turns every card over to its back, or, where every one is
// already, back to its front, ending the selection.
func (m Model) flipEvery() Model {
	if m.focus == "" {
		m = m.focusFirst()
	}
	if m.everyFlipped() {
		m.flipped, m.selected = nil, dashboard.Seat{}
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

// focusFirst gives the focus to the first card in view, as the frame finds
// it.
func (m Model) focusFirst() Model {
	now := m.now()
	id := m.frame(now).InView(m.shown(now), now)
	return m.focusOn(id, id != "")
}

// focusOn gives the focus to the card of the account with the given id,
// where ok says there's one, ending the selection, and scrolls the cards as
// far as they must go to show it whole.
func (m Model) focusOn(id string, ok bool) Model {
	if !ok {
		return m
	}
	now := m.now()
	m.focus, m.selected = id, dashboard.Seat{}
	m.scroll = m.frame(now).Reveal(m.shown(now), now, id)
	return m
}

// neighbour is the account whose card is across cards along the row of the
// card with the focus, or down rows of cards, as the screen lays them out,
// reporting false where there's none.
func (m Model) neighbour(across, down int) (string, bool) {
	now := m.now()
	return m.frame(now).Neighbour(m.shown(now), now, m.focus, across, down)
}

// seats are the seats on the back of the card with the focus, a row each,
// where it's flipped and there's another account to move one to: none
// otherwise.
func (m Model) seats() []dashboard.Seat {
	if !m.flipped[m.focus] || m.single() {
		return nil
	}
	return m.frame(m.now()).Seats(m.focus)
}

// selecting reports whether a session is picked out on a card's back.
func (m Model) selecting() bool {
	return !m.selected.IsZero()
}

// stillThere is the model with the focus given up, and the cards no longer
// flipped, where the document no longer has their accounts; and the
// selection ended where the card's back no longer has the session picked
// out.
func (m Model) stillThere() Model {
	if !m.has(m.focus) {
		m.focus = ""
	}
	flipped := maps.Clone(m.flipped)
	maps.DeleteFunc(flipped, func(id string, _ bool) bool { return !m.has(id) })
	m.flipped = flipped
	if !slices.Contains(m.seats(), m.selected) {
		m.selected = dashboard.Seat{}
	}
	return m
}

// has reports whether the document on screen has the account with the given
// id.
func (m Model) has(id string) bool {
	_, ok := m.doc.Account(id)
	return ok
}
