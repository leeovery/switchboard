package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// busyWithin is how lately a session was seen for its dot to light: without
// the request stream, a session is busy when it was seen in the last minute.
const busyWithin = time.Minute

// face is the front of an account's card, as the document has it at a
// moment: the account and its place, its state, the window it features and
// its other windows, its badges, and its sessions.
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
	// limitAt is when it reached its limit, as the router told of it, and
	// primed when the router next primes it: zero where unknown.
	limitAt, primed time.Time
}

// faces are the fronts of doc's accounts' cards at now, and the keys of the
// windows every card shows.
func (f Frame) faces(doc status.Document, now time.Time) ([]face, []string) {
	shown := shownWindows(doc, now, f.Policy)
	faces := make([]face, len(doc.Accounts))
	for i, a := range doc.Accounts {
		faces[i] = f.face(doc, a, now, shown)
	}
	return faces, shown
}

// face is the front of doc's account a's card at now, of the windows shown:
// the window it features, as f.Featured says, its other windows, and its
// sessions, as the router listed them, else as many as doc says it has.
func (f Frame) face(doc status.Document, a status.Account, now time.Time, shown []string) face {
	fc := face{
		account: a, place: place(doc, a.ID), state: doc.StateOf(a, now, f.Policy),
		windows: shown, bars: make(map[string]standing),
		next: len(doc.Accounts) > 1 && doc.Best == a.ID, pinned: doc.Pin.Has(a.ID),
		sessions: a.Sessions, limitAt: limitReached(doc, a.ID, now), primed: primedNext(doc, a.ID),
	}
	w, ok := featured(doc, a, now, f.Policy, f.Featured, shown)
	if ok {
		fc.featured, fc.hasFeatured = standingOf(doc, a, w, now, f.Policy), true
		fc.chart = f.burndownOf(doc, a, fc.featured, fc.tone(), now)
	}
	for _, key := range shown {
		if bar, has := a.Window(key); has && (!ok || key != w.Key) {
			fc.bars[key] = standingOf(doc, a, bar, now, f.Policy)
		}
	}
	if f.Sessions != nil {
		fc.busy = busy(f.Sessions, a.ID, now)
		fc.sessions = len(fc.busy)
	}
	return fc
}

// burndownOf is the chart of doc's account a's window as it stands, s, at
// now, in the tone of its state, from its history.
func (f Frame) burndownOf(doc status.Document, a status.Account, s standing, tone theme.Token, now time.Time) burndown {
	b := burndown{standing: s, tone: tone, now: now, starts: doc.StartsAt(a.ID, now)}
	b.start, b.length, b.spanned = s.window.Span()
	b.trail = f.History[Ref{Account: a.ID, Window: s.window.Key}]
	b.traced = len(b.trail.Readings) > 0
	return b
}

// limitReached is when the account with the given id last reached its limit
// at or before now, as doc's events tell of it: zero where none does.
func limitReached(doc status.Document, id string, now time.Time) time.Time {
	i := slices.IndexFunc(doc.Events, func(e status.Event) bool {
		return e.Kind == status.EventLimit && e.Account == id && !e.At.After(now)
	})
	if i < 0 {
		return time.Time{}
	}
	return doc.Events[i].At
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

// busy are the sessions on the account with the given id, of those listed,
// in their order, the one seen last first: each lit while busy, its requests
// to the account seen in the last minute before now.
func busy(sessions []status.Session, id string, now time.Time) []bool {
	var lit []bool
	for _, s := range sessions {
		seen, on := time.Time{}, false
		for _, a := range s.Assignments {
			if a.Account == id {
				seen, on = latest(seen, a.LastSeen), true
			}
		}
		if on {
			lit = append(lit, now.Sub(seen) < busyWithin)
		}
	}
	return lit
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
