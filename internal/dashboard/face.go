package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// busyWithin is how lately a session was seen for its dot to light: it's
// busy when it was seen in the last minute, or, as the request stream tells,
// while a request of its is in flight.
const busyWithin = time.Minute

// face is an account's card, as the document has it at a moment: the
// account and its place, its state, the window its front features and its
// other windows, its badges, and its sessions; and as the watch has it,
// whether it has the focus, and whether it's flipped to show its back.
type face struct {
	account status.Account
	place   int
	state   status.State
	// featured is the window it features, where it has one to, and chart its
	// burn-down.
	featured    standing
	hasFeatured bool
	chart       burndown
	// windows are the keys of the windows every card shows, in order, and
	// bars how those it doesn't feature stand, of those it has.
	windows []string
	bars    map[string]standing
	// next is set on the card of the account new sessions go to, of several,
	// and pinned on one the global pin names.
	next, pinned bool
	// busy are its sessions, the one seen last first, each lit while busy,
	// as the router listed them; sessions is how many it has.
	busy     []bool
	sessions int
	// primed is when the router next primes it: zero where unknown.
	primed time.Time
	// read is when the document was read, which a recent rate is measured
	// to.
	read time.Time
	// focused is set on the card with the focus, and flipped on a card
	// turned over to show its sessions.
	focused, flipped bool
}

// faces are doc's accounts' cards at now, and the keys of the windows every
// card's front shows.
func (f Frame) faces(doc status.Document, now time.Time) ([]face, []string) {
	shown := shownWindows(doc, now, f.Policy)
	faces := make([]face, len(doc.Accounts))
	for i, a := range doc.Accounts {
		faces[i] = f.face(doc, a, now, shown)
	}
	return faces, shown
}

// face is doc's account a's card at now, of the windows shown: the window
// its front features, as f.Featured says, its other windows, and its
// sessions, as the router listed them, else as many as doc says it has; and
// whether it has the focus, and is flipped, as f says.
func (f Frame) face(doc status.Document, a status.Account, now time.Time, shown []string) face {
	fc := face{
		account: a, place: place(doc, a.ID), state: doc.StateOf(a, now, f.Policy),
		windows: shown, bars: make(map[string]standing),
		next: len(doc.Accounts) > 1 && doc.Best == a.ID, pinned: doc.Pin.Has(a.ID),
		sessions: a.Sessions, primed: primedNext(doc, a.ID),
		read: cmp.Or(doc.GeneratedAt, now), focused: f.Focus == a.ID, flipped: f.Flipped[a.ID],
	}
	w, ok := featured(doc, a, now, f.Policy, f.Featured, shown)
	if ok {
		fc.featured, fc.hasFeatured = standingOf(doc, a, w, now, f.Policy), true
		fc.chart = f.burndownOf(doc, fc, now)
	}
	for _, key := range shown {
		if bar, has := a.Window(key); has && (!ok || key != w.Key) {
			fc.bars[key] = standingOf(doc, a, bar, now, f.Policy)
		}
	}
	if f.Sessions != nil {
		fc.busy = f.busy(a.ID, now)
		fc.sessions = len(fc.busy)
	}
	return fc
}

// burndownOf is the chart of the window the card of doc's account fc
// features, as it stands at now, in the tone of its state, from its history,
// and while it's held at its limit, on the floor from when that was reached,
// as its standing says.
func (f Frame) burndownOf(doc status.Document, fc face, now time.Time) burndown {
	b := burndown{standing: fc.featured, tone: fc.tone(), now: now, starts: doc.StartsAt(fc.account.ID, now)}
	b.start, b.length, b.spanned = b.window.Span()
	b.trail = f.History[Ref{Account: fc.account.ID, Window: b.window.Key}]
	b.traced = len(b.trail.Readings) > 0
	return b
}

// primedNext is when the router next primes the account with the given id,
// as doc's priming schedule says: zero where it doesn't.
func primedNext(doc status.Document, id string) time.Time {
	i := slices.IndexFunc(doc.Prime.Slots, func(s status.Slot) bool { return s.Account == id })
	if i < 0 {
		return time.Time{}
	}
	return doc.Prime.Slots[i].Next
}

// busy are the sessions on the account with the given id, of those the
// router listed, in their order, the one seen last first: each lit while any
// of its models is busy there, as lit says.
func (f Frame) busy(id string, now time.Time) []bool {
	var busy []bool
	for _, s := range f.Sessions {
		on, lit := false, false
		for _, a := range s.Assignments {
			if a.Account == id {
				on, lit = true, lit || f.lit(seated{session: s, assignment: a}, now)
			}
		}
		if on {
			busy = append(busy, lit)
		}
	}
	return busy
}

// lit reports whether the seat s is busy at now: a request of its in flight,
// as the request stream tells, or it was seen at work in the last minute.
func (f Frame) lit(s seated, now time.Time) bool {
	if c, ok := f.Traffic.call(s.plug()); ok && c.inFlight() {
		return true
	}
	return now.Sub(f.lastSeen(s)) < busyWithin
}

// lastSeen is when the seat s was last seen at work: as the router listed
// it, or later, as the request stream told of it.
func (f Frame) lastSeen(s seated) time.Time {
	seen := s.assignment.LastSeen
	if c, ok := f.Traffic.call(s.plug()); ok {
		seen = latest(seen, c.Seen)
	}
	return seen
}

// latest is the later of two times.
func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// tone is the colour of the account's state, which its featured window's
// digits and chart take.
func (fc face) tone() theme.Token {
	_, tone := marked(fc.state.Condition)
	return tone
}

// marked is how a card marks an account's state: the glyph its words lead
// with, and its colour.
func marked(c status.Condition) (string, theme.Token) {
	switch c {
	case status.Tokenless:
		return "✕", theme.StateDestructive
	case status.Unreadable:
		return "!", theme.StateDestructive
	case status.Unread:
		return "…", theme.TextSubtle
	case status.Limited:
		return "■", theme.StateDestructive
	case status.PartlyLimited:
		return "■", theme.AccentAttention
	case status.Reserved, status.Pressed:
		return "●", theme.AccentAttention
	case status.Idle:
		return "○", theme.TextSubtle
	default:
		return "●", theme.StatePositive
	}
}

// stateLine is what a card says of its account's state: its mark, then what
// holds it back or what it's doing, bold in its colour, but idle's in
// text.tertiary, and nothing read yet's dim; then what that means, muted.
func (fc face) stateLine() line {
	mark, tone := marked(fc.state.Condition)
	says := ink{token: tone, bold: true}
	switch fc.state.Condition {
	case status.Idle:
		says.token = theme.TextTertiary
	case status.Unread:
		says = dimInk
	}
	l := line{{mark + " ", ink{token: tone}}, {fc.state.Says, says}}
	if fc.state.Then != "" {
		l = append(l, span{" · " + fc.state.Then, mutedInk})
	}
	return l
}
