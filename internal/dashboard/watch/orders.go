package watch

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/status"
)

// noteFor is how long the key line says what a key did, in its place.
const noteFor = 4 * time.Second

// errOrdering is why an order is dropped while the router still carries out
// the last.
var errOrdering = errors.New("the router is still carrying out the last order")

// orderedMsg is how an order went, given by the routing card numbered card.
type orderedMsg struct {
	order order
	card  int
	err   error
}

// order is what the routing card tells the router: send new sessions to the
// best of accounts, and with move, running ones on another account too; or
// with no accounts, route every session on its merits again.
type order struct {
	accounts []string
	move     bool
}

// give has the source carry the order out.
func (o order) give(ctx context.Context, s Source) error {
	if len(o.accounts) == 0 {
		return s.Unpin(ctx)
	}
	return s.Pin(ctx, o.accounts, o.move)
}

// log notes how the order went, naming the accounts by their ids alone.
func (o order) log(err error) {
	accounts := strings.Join(o.accounts, ",")
	switch {
	case err != nil:
		logger.Warn("the router didn't take an order", "accounts", accounts, "move", o.move, "error", err)
	case len(o.accounts) == 0:
		logger.Info("unpinned")
	default:
		logger.Info("pinned", "accounts", accounts, "move", o.move)
	}
}

// command has the router carry out an order, given by the routing card
// open last, as ordered takes in once it has: while the router doesn't
// answer, or still carries out the last order, it's dropped, saying why, as
// a refusal is said.
func (m Model) command(o order) (Model, tea.Cmd) {
	card := m.routing.card
	switch {
	case !m.answering():
		return m.refused(card, status.ErrNoRouter)
	case m.ordering:
		return m.refused(card, errOrdering)
	}
	m.ordering = true
	ctx, source := m.ctx, m.cfg.Source
	return m, func() tea.Msg {
		return orderedMsg{order: o, card: card, err: o.give(ctx, source)}
	}
}

// ordered takes in how an order went: taken, the routing card that gave it
// closes, while it's open; refused, the router's reason is said, as refused
// says. Either way, the router's document is read again at once, to show
// what it did, and the routing card open no longer says the router is still
// carrying it out.
func (m Model) ordered(msg orderedMsg) (tea.Model, tea.Cmd) {
	m.ordering = false
	msg.order.log(msg.err)
	if errors.Is(m.routing.refused, errOrdering) {
		m.routing.refused = nil
	}
	var said tea.Cmd
	switch {
	case msg.err != nil:
		m, said = m.refused(msg.card, msg.err)
	case m.cardOpen(msg.card):
		m, said = m.uncover()
	}
	m, read := m.reread()
	return m, tea.Batch(said, read)
}

// refused says why an order the routing card numbered card gave wasn't
// carried out: in that card, while it's open, until its picks change; else
// in the key line's place, as noting has it.
func (m Model) refused(card int, err error) (Model, tea.Cmd) {
	if m.cardOpen(card) {
		m.routing.refused = err
		return m, nil
	}
	return m.noting(err.Error())
}

// cardOpen reports whether the routing card numbered card is open.
func (m Model) cardOpen(card int) bool {
	return m.open == routingOpen && m.routing.card == card
}

// noting has the key line say note in its place for noteFor, and returns
// what redraws it once that's passed. While a card covers the page, whose
// key line stands still behind it, the note's held, its time unstarted,
// until the page shows again, as seen has it.
func (m Model) noting(note string) (Model, tea.Cmd) {
	m.note = note
	if m.open != nothingOpen {
		m.noteUntil = time.Time{}
		return m, nil
	}
	m.noteUntil = m.now().Add(noteFor)
	return m, m.after(noteFor, noteMsg{})
}

// seen starts the time of a note held while a card covered the page, now
// the page shows again, as noting does.
func (m Model) seen() (Model, tea.Cmd) {
	if m.note == "" || !m.noteUntil.IsZero() {
		return m, nil
	}
	return m.noting(m.note)
}

// noteMsg wakes the model to redraw once a note has had its time.
type noteMsg struct{}

// reread looks at the router's document again as soon as it can, while the
// dashboard reads the router: now, or once the read under way lands, as that
// one may have been read before what prompts this.
func (m Model) reread() (Model, tea.Cmd) {
	switch {
	case !m.routed():
		return m, nil
	case m.fetching:
		m.again = true
		return m, nil
	default:
		return m.read(status.Read{})
	}
}

// answering reports whether the dashboard reads the router, and the router
// answered the last look at its document: orders work only then.
func (m Model) answering() bool {
	return m.routed() && m.lost.IsZero()
}
