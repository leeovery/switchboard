package watch

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
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

// pressed acts on a key: q or ctrl+c quits, whatever's open; while the theme
// picker or the help is open, it takes every other key. Otherwise, j, k,
// PgDn and PgUp scroll the cards where they don't fit; ? opens the help;
// tab and shift-tab show the next view and the one before, r reads now,
// having the router refresh what it hasn't read in the last minute, and what
// can take no request, or probing when it doesn't answer; t opens the theme
// picker; and w has the cards feature the next window.
// While the router answers, of more than one account, 1–9 pin new sessions
// to the account in that place, as configured, beside those pinned already,
// or unpin it; a routes every session on its merits again; and m moves
// running sessions to the accounts pinned. With one account, there's no
// other to send them to, and they do nothing.
func (m Model) pressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k := key.String(); {
	case k == "q" || k == "Q" || k == "ctrl+c":
		return m, tea.Quit
	case m.picker.open:
		return m.pickerKey(k)
	case m.helping:
		return m.helpKey(k), nil
	}
	if rows, ok := m.scrollKey(key.String()); ok {
		return m.scrollBy(rows), nil
	}
	switch k := key.String(); k {
	case "?":
		m.helping = true
		return m, nil
	case "tab":
		return m.turn(1)
	case "shift+tab":
		return m.turn(-1)
	case "r", "R":
		logger.Debug("refresh key pressed", "already_reading", m.fetching)
		return m.read(Fresh())
	case "t", "T":
		return m.openPicker()
	case "w", "W":
		return m.feature()
	case "a", "A":
		if m.single() {
			return m, nil
		}
		return m.command(routing)
	case "m", "M":
		if m.single() {
			return m, nil
		}
		return m.move()
	default:
		if n, ok := place(k); ok && !m.single() {
			return m.toggle(n)
		}
	}
	return m, nil
}

// single reports whether the document on screen is of one account, so
// there's none other to send sessions to.
func (m Model) single() bool {
	return len(m.doc.Accounts) == 1
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

// toggled returns the ids of the accounts doc's pin names that stay pinned,
// as stays says, in the order configured, with the account with the given id
// put in when the pin doesn't name it, and taken out when it does. The one
// put in is put in whether it has a token or not, so the router says why it
// can't be pinned.
func toggled(doc status.Document, id string) []string {
	var ids []string
	for _, a := range doc.Accounts {
		if a.ID == id && !doc.Pin.Has(id) || a.ID != id && stays(doc, a) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// staying returns the ids of the accounts doc's pin names that stay pinned,
// as stays says, in the order configured.
func staying(doc status.Document) []string {
	var ids []string
	for _, a := range doc.Accounts {
		if stays(doc, a) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// stays reports whether the account a stays pinned in an order a key gives:
// doc's pin names it, and it has a usable token, as doc shows. The router
// refuses a pin naming any account without one, as one that lost its token
// since it was pinned.
func stays(doc status.Document, a status.Account) bool {
	return doc.Pin.Has(a.ID) && a.TokenSet
}

// move has the router move running sessions to the accounts pinned that have
// a usable token, or says there's none to move them to.
func (m Model) move() (Model, tea.Cmd) {
	switch pinned := staying(m.doc); {
	case !m.answering():
		return m.unanswered()
	case m.doc.Pin.IsZero():
		return m.noting("nothing's pinned to move sessions to: pin an account with " + places(len(m.doc.Accounts)))
	case len(pinned) == 0:
		return m.noting("no account pinned has a usable token to move sessions to: pin another with " + places(len(m.doc.Accounts)))
	default:
		return m.command(order{accounts: pinned, move: true, done: "running sessions move to " + m.doc.Destination(pinned)})
	}
}

// command has the router carry out an order, while the router answers and
// isn't still carrying out the last. Once it has, the footer says what the
// order did, or why it couldn't, and the router's document is read again.
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

// answering reports whether the dashboard reads the router, and the router
// answered the last look at its document: the keys that give it orders work
// only then.
func (m Model) answering() bool {
	return m.routed() && m.lost.IsZero()
}

// unanswered is what a key that gives the router an order does while the
// router can't take it: it says the router isn't answering while the
// router's last document is on screen, and does nothing while the dashboard
// probes.
func (m Model) unanswered() (Model, tea.Cmd) {
	if !m.routed() {
		return m, nil
	}
	return m.noting(ErrNoRouter.Error())
}

// keyListing is a key as the model stands, as the footer and the help list it:
// what the footer says it does, in a word or two, "" for a key it leaves
// behind ?, and what the help says, at more length; whether the footer
// lists it wherever there's room; and whether it works there, and so is
// listed at all.
type keyListing struct {
	key, footer, help string
	always, works     bool
}

// listings are every key there is, as m stands, in the design's order, so
// the footer comes out as the design has it once every key works: tab,
// while there's another view to move to; w, in the Accounts view, saying
// which window the cards feature; j and k, where the cards scroll; while the
// router answers, of more than one account, the digits of their places, a
// and m; r; t, in colour; and ? and q, always. The footer leaves j and k, r
// and t behind ?.
func (m Model) listings() []keyListing {
	window := m.featured.Name(m.doc, m.now(), m.cfg.Policy)
	return []keyListing{
		{key: "tab", footer: "views", help: "the next view; shift-tab, the one before", works: len(m.views) > 1},
		{key: "w", footer: "window: " + window, help: "cycle the window every card features, now " + window, works: m.view == dashboard.Accounts},
		{key: "j k", help: "scroll the cards; PgUp and PgDn a page, or the wheel", works: m.scrolling().Most > 0},
		{key: places(len(m.doc.Accounts)), footer: "pin", help: "toggle the account in that place in the pin", works: m.orders()},
		{key: "a", footer: "auto", help: "route automatically again", works: m.orders()},
		{key: "m", footer: "move", help: "move running sessions to the pinned accounts", works: m.orders()},
		{key: "r", help: "refresh", works: true},
		{key: "t", help: "the theme picker", works: m.coloured()},
		{key: "?", footer: "keys", help: "these keys, and the key to the glyphs", always: true, works: true},
		{key: "q", footer: "quit", help: "quit", always: true, works: true},
	}
}

// keys are the keys the footer lists, as listings has them: those that work
// where they are, but those it leaves behind ?.
func (m Model) keys() []dashboard.Key {
	var keys []dashboard.Key
	for _, l := range m.listings() {
		if l.works && l.footer != "" {
			keys = append(keys, dashboard.Key{Key: l.key, Does: l.footer, Always: l.always})
		}
	}
	return keys
}

// helpKeys are the keys the help lists, as listings has them: every key that
// works where it is, and what it does, at more length than the footer says.
func (m Model) helpKeys() []dashboard.Key {
	var keys []dashboard.Key
	for _, l := range m.listings() {
		if l.works {
			keys = append(keys, dashboard.Key{Key: l.key, Does: l.help})
		}
	}
	return keys
}

// helpKey acts on a key while the help is open, which takes every key but
// those that quit: ? and esc close it, and the rest do nothing.
func (m Model) helpKey(key string) Model {
	if key == "?" || key == "esc" {
		m.helping = false
	}
	return m
}

// orders reports whether the keys that give the router orders work: while the
// router answers, of more than one account.
func (m Model) orders() bool {
	return m.answering() && !m.single()
}

// places names the keys of the first n accounts' places, of the first nine:
// "1", or "1-3".
func places(n int) string {
	n = min(n, pinnable)
	if n == 1 {
		return "1"
	}
	return "1-" + strconv.Itoa(n)
}
