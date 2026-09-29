package watch

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	// pinnable is how many accounts have a key that pins new sessions to
	// them: the first nine.
	pinnable = 9
	// noteFor is how long the footer says what a key did.
	noteFor = 4 * time.Second
)

// orderedMsg is how an order a key gave went.
type orderedMsg struct {
	order order
	err   error
}

// order is what a key tells the router: send new sessions to an account, and
// with move, running ones too; or, with no account, route every session on
// its merits again.
type order struct {
	account string
	move    bool
	// done says what the order did, for the footer.
	done string
}

// give has the source carry the order out.
func (o order) give(ctx context.Context, s Source) error {
	if o.account == "" {
		return s.Unpin(ctx)
	}
	return s.Pin(ctx, o.account, o.move)
}

// log notes how the order went, naming the account by its id alone.
func (o order) log(err error) {
	switch {
	case err != nil:
		logger.Warn("the router didn't take an order", "account", o.account, "move", o.move, "error", err)
	case o.account == "":
		logger.Info("unpinned")
	default:
		logger.Info("pinned", "account", o.account, "move", o.move)
	}
}

// pressed acts on a key: r reads now, having the router refresh what it
// hasn't read in the last minute, or probing when it doesn't answer; q or
// ctrl+c quits. While the dashboard reads the router, 1–9 pin new sessions to
// the account in that place, as configured; a routes every session on its
// merits again; and m moves running sessions to the account pinned.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := key.String(); k {
	case "r", "R":
		logger.Debug("refresh key pressed", "already_reading", m.fetching)
		return m.read(Read{Refresh: freshFor, Probe: true})
	case "q", "Q", "ctrl+c":
		return m, tea.Quit
	case "a", "A":
		return m.command(order{done: "routing automatically"})
	case "m", "M":
		return m.move()
	default:
		if n, ok := place(k); ok {
			return m.pin(n)
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

// pin has the router send new sessions to the account in place n, counting
// from 1 in the order they're configured.
func (m Model) pin(n int) (Model, tea.Cmd) {
	if n > len(m.doc.Accounts) {
		return m, nil
	}
	a := m.doc.Accounts[n-1]
	return m.command(order{account: a.ID, done: "new sessions go to " + a.Title()})
}

// move has the router move running sessions to the account pinned, or says
// there's none to move them to.
func (m Model) move() (Model, tea.Cmd) {
	switch pin := m.doc.Pin.Account; {
	case !m.routed():
		return m, nil
	case pin == "":
		return m.noting("nothing's pinned to move sessions to: pin an account with " + places(len(m.doc.Accounts)))
	default:
		name := pin
		if a, ok := m.doc.Account(pin); ok {
			name = a.Title()
		}
		return m.command(order{account: pin, move: true, done: "running sessions move to " + name})
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
		return m.read(Read{Probe: true})
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
		keys = append(keys, places(len(m.doc.Accounts))+" pin")
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
