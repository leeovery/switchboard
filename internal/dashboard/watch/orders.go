package watch

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/status"
)

// noteFor is how long the key line says what a key did, in its place.
const noteFor = 4 * time.Second

// orderedMsg is how an order the routing card gave went.
type orderedMsg struct {
	order order
	err   error
}

// order is what the routing card tells the router: send new sessions to the
// best of accounts, and with move, running ones on another account too; with
// no accounts, route every session on its merits again; or, of one session
// alone, send it to the account to, or with none, clear its own pin.
type order struct {
	accounts []string
	move     bool
	// session is the id of the session the order is of alone, "" for an
	// order of every session, and to the id of the account it sends it to.
	session, to string
}

// give has the source carry the order out.
func (o order) give(ctx context.Context, s Source) error {
	switch {
	case o.session != "" && o.to != "":
		return s.PinSession(ctx, o.session, o.to)
	case o.session != "":
		return s.UnpinSession(ctx, o.session)
	case len(o.accounts) == 0:
		return s.Unpin(ctx)
	default:
		return s.Pin(ctx, o.accounts, o.move)
	}
}

// log notes how the order went, naming the accounts by their ids alone, and
// a session by its id cut short, as the router logs it.
func (o order) log(err error) {
	if o.session != "" {
		o.logSession(err)
		return
	}
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

// logSession notes how an order of one session went: the session, and the
// account it sends it to, where it sends it to one.
func (o order) logSession(err error) {
	attrs := []any{"session", status.ShortID(o.session)}
	if o.to != "" {
		attrs = append(attrs, "account", o.to)
	}
	switch {
	case err != nil:
		logger.Warn("the router didn't take an order", append(attrs, "error", err)...)
	case o.to != "":
		logger.Info("pinned a session", attrs...)
	default:
		logger.Info("unpinned a session", attrs...)
	}
}

// command has the router carry out an order, while the router answers and
// isn't still carrying out the last. Once it has, the router's document is
// read again.
func (m Model) command(o order) (Model, tea.Cmd) {
	switch {
	case !m.answering():
		return m.unanswered()
	case m.ordering:
		return m, nil
	}
	m.ordering = true
	ctx, source := m.ctx, m.cfg.Source
	return m, func() tea.Msg {
		return orderedMsg{order: o, err: o.give(ctx, source)}
	}
}

// ordered takes in how an order went: the router's reason, where it refused
// it, is noted, and the router's document is read again to show what it did.
func (m Model) ordered(msg orderedMsg) (tea.Model, tea.Cmd) {
	m.ordering = false
	msg.order.log(msg.err)
	var lapse tea.Cmd
	if msg.err != nil {
		m, lapse = m.noting(msg.err.Error())
	}
	m, read := m.reread()
	return m, tea.Batch(lapse, read)
}

// noting has the key line say note in its place for noteFor, and returns
// what redraws it once that's passed.
func (m Model) noting(note string) (Model, tea.Cmd) {
	m.note, m.noteUntil = note, m.now().Add(noteFor)
	return m, m.after(noteFor, noteMsg{})
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

// unanswered is what an order does while the router can't take it: it says
// the router isn't answering while the router's last document is on screen,
// and does nothing while the dashboard probes.
func (m Model) unanswered() (Model, tea.Cmd) {
	if !m.routed() {
		return m, nil
	}
	return m.noting(status.ErrNoRouter.Error())
}
