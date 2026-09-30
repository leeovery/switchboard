package watch

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// pinnable is how many accounts have a key that pins new sessions to
	// them, or unpins them: the first nine.
	pinnable = 9
	// noteFor is how long the footer says what a key did.
	noteFor = 4 * time.Second
)

// orderedMsg is how an order a key gave went.
type orderedMsg struct {
	order order
	err   error
}

// order is what a key tells the router: send new sessions to the best of
// accounts, and with move, running ones on another account too; or, with no
// accounts, route every session on its merits again.
type order struct {
	accounts []string
	move     bool
	// done says what the order did, for the footer.
	done string
}

// routing is the order to route every session on its merits again.
var routing = order{done: "routing automatically"}

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

// pressed acts on a key: r reads now, having the router refresh what it
// hasn't read in the last minute, and what can take no request, or probing
// when it doesn't answer; q or ctrl+c quits. While the dashboard reads the router, 1–9 pin new sessions to
// the account in that place, as configured, beside those pinned already, or
// unpin it; a routes every session on its merits again; and m moves running
// sessions to the accounts pinned.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := key.String(); k {
	case "r", "R":
		logger.Debug("refresh key pressed", "already_reading", m.fetching)
		return m.read(Fresh())
	case "q", "Q", "ctrl+c":
		return m, tea.Quit
	case "a", "A":
		return m.command(routing)
	case "m", "M":
		return m.move()
	default:
		if n, ok := place(k); ok {
			return m.toggle(n)
		}
	}
	return m, nil
}

// place is the place a digit key names, from 1 to pinnable.
func place(key string) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '0'+pinnable {
		return 0, false
	}
	return int(key[0] - '0'), true
}

// toggle has the router pin new sessions to the account in place n, counting
// from 1 in the order they're configured, beside the accounts pinned already;
// or, when it's one of them, unpin it, routing every session automatically
// again once none is left.
func (m Model) toggle(n int) (Model, tea.Cmd) {
	if n > len(m.doc.Accounts) {
		return m, nil
	}
	pinned := toggled(m.doc, m.doc.Accounts[n-1].ID)
	if len(pinned) == 0 {
		return m.command(routing)
	}
	return m.command(order{accounts: pinned, done: "new sessions go to " + m.doc.Destination(pinned)})
}

// toggled returns the ids of the accounts doc's pin names, in the order
// configured, with the account with the given id put in when it isn't one of
// them, and taken out when it is.
func toggled(doc status.Document, id string) []string {
	var ids []string
	for _, a := range doc.Accounts {
		if doc.Pin.Has(a.ID) != (a.ID == id) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// move has the router move running sessions to the accounts pinned, or says
// there's none to move them to.
func (m Model) move() (Model, tea.Cmd) {
	switch pinned := m.doc.Pin.Accounts; {
	case !m.routed():
		return m, nil
	case len(pinned) == 0:
		return m.noting("nothing's pinned to move sessions to: pin an account with " + places(len(m.doc.Accounts)))
	default:
		return m.command(order{accounts: pinned, move: true, done: "running sessions move to " + m.doc.Destination(pinned)})
	}
}

// command has the router carry out an order, while the dashboard reads the
// router and it isn't still carrying out the last. Once it has, the footer
// says what the order did, or why it couldn't, and the router's document is
// read again.
func (m Model) command(o order) (Model, tea.Cmd) {
	if !m.routed() || m.ordering {
		return m, nil
	}
	m.ordering = true
	ctx, source := m.ctx, m.cfg.Source
	return m, func() tea.Msg {
		return orderedMsg{order: o, err: o.give(ctx, source)}
	}
}

// ordered takes in how an order went: the footer says, and the router's
// document is read again to show what it did.
func (m Model) ordered(msg orderedMsg) (tea.Model, tea.Cmd) {
	m.ordering = false
	msg.order.log(msg.err)
	note := msg.order.done
	if msg.err != nil {
		note = msg.err.Error()
	}
	m, lapse := m.noting(note)
	m, read := m.reread()
	return m, tea.Batch(lapse, read)
}

// noting has the footer say note for noteFor, and returns what redraws it
// once that's passed.
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
		return m.read(Read{})
	}
}

// keys says what the keys do: while the dashboard reads the router, the ones
// that tell it where to send sessions too.
func (m Model) keys() string {
	if !m.routed() {
		return "r refresh · q quit"
	}
	keys := []string{"r refresh"}
	if len(m.doc.Accounts) > 0 {
		keys = append(keys, places(len(m.doc.Accounts))+" toggle pin")
	}
	return strings.Join(append(keys, "a auto", "m move", "q quit"), " · ")
}

// places names the keys of the first n accounts' places, of the first nine:
// "1", or "1–3".
func places(n int) string {
	n = min(n, pinnable)
	if n == 1 {
		return "1"
	}
	return "1–" + strconv.Itoa(n)
}
