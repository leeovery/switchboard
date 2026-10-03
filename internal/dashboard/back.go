package dashboard

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The columns of a seat's row on a card's back: the cells before its dot,
// where ▸ marks the one picked out; its session's id, its model, at the
// least, and what it's doing's word, each with the blanks after it; and how
// far in the note under it starts.
const (
	seatLead    = 2
	idColumn    = 6
	modelColumn = 8
	doingColumn = 11
	noteIndent  = 4
)

// backKeysGap is the blank cells between the keys at the foot of a card's
// back.
const backKeysGap = 2

// Seat is a session's requests of one model on an account: a row of the back
// of the account's card.
type Seat struct {
	Session, Model string
}

// IsZero reports whether the seat is none, as while no session is picked
// out.
func (s Seat) IsZero() bool {
	return s == Seat{}
}

// Shown is the seat's session's id as the dashboard shows it: its first
// four characters, cleaned, such as d28c.
func (s Seat) Shown() string {
	return sessionID(s.Session)
}

// Seats are the seats of the sessions listed on the account with the given
// id, the request stream's moves among them, active there at now, as
// activeOn says, a row each of its card's back, in the order the router
// lists them, the session seen last first, and of each session, the model
// used last first.
func (f Frame) Seats(id string, now time.Time) []Seat {
	on := f.activeOn(id, now)
	seats := make([]Seat, len(on))
	for i, s := range on {
		seats[i] = s.seat()
	}
	return seats
}

// seated is a session's model on an account, as the router listed it: the
// session, and its assignment there.
type seated struct {
	session    status.Session
	assignment status.Assignment
}

// seat is where s sits: its session, and its model.
func (s seated) seat() Seat {
	return Seat{Session: s.session.ID, Model: s.assignment.Model}
}

// plug is where s is plugged in: its seat, on its account.
func (s seated) plug() Plug {
	return Plug{Account: s.assignment.Account, Seat: s.seat()}
}

// activeOn are the seats of the sessions on the account with the given id,
// of those the router listed, the request stream's moves among them, that
// count among its sessions at now, as active says, in the order Seats
// gives.
func (f Frame) activeOn(id string, now time.Time) []seated {
	return slices.DeleteFunc(seatedOn(f.listing(), id), func(s seated) bool { return !f.active(s, now) })
}

// seatedOn are the sessions listed on the account with the given id, a seat
// each of their models there, in the order Seats gives.
func seatedOn(sessions []status.Session, id string) []seated {
	var on []seated
	for _, s := range sessions {
		for _, a := range s.Assignments {
			if a.Account == id {
				on = append(on, seated{session: s, assignment: a})
			}
		}
	}
	return on
}

// back draws the back of the card fc, inside its edges, width cells wide and
// rows tall from x along row y: how many sessions its account has, and how
// many of them are busy; a blank row, but where its sessions' rows would
// all fit without it alone; a row for each of its sessions' models active
// there, or why it has none; then, where there's room, LATELY, what has
// befallen it lately; and at its foot, the keys.
func (f Frame) back(c *canvas, doc status.Document, fc face, now time.Time, x, y, width, rows int) {
	if rows < 1 {
		return
	}
	seats := f.activeOn(fc.account.ID, now)
	c.line(x, y, tally(fc).fit(width))
	if rows < 2 {
		return
	}
	foot, body := y+rows-1, y+2
	if len(seats) > foot-body {
		body = y + 1
	}
	c.line(x, foot, f.backKeys(seats).fit(width))
	under := y + 1
	if drawn := f.backBody(c, doc, fc, seats, now, x, body, width, foot-body); drawn > 0 {
		under = body + drawn
	}
	f.lately(c, fc, now, x, under, width, foot)
}

// tally says how many sessions are on the card fc's account and, where the
// router listed them, how many are busy, as in "3 sessions  ·  2 busy"; or
// "no sessions".
func tally(fc face) line {
	l := line{{status.SessionCount(fc.sessions), titleInk}}
	if fc.sessions > 0 && fc.busy != nil {
		lit := 0
		for _, busy := range fc.busy {
			if busy {
				lit++
			}
		}
		l = append(l, span{"  ·  " + strconv.Itoa(lit) + " busy", mutedInk})
	}
	return l
}

// backKeys is the line of keys at the foot of a card's back, whose seats
// are those given: the keys that work on its sessions, where it has any, then
// space, which flips it back.
func (f Frame) backKeys(seats []seated) line {
	keys := []Key{{Key: "space", Does: "flip"}}
	if len(seats) > 0 {
		keys = slices.Concat(f.Patch, keys)
	}
	var l line
	for i, k := range keys {
		if i > 0 {
			l = append(l, spaces(backKeysGap))
		}
		l = append(l, k.words()...)
	}
	return l
}

// backBody draws the body of the card fc's back, its seats, from x along row
// y, width cells wide and room rows tall at most, and returns how many rows
// it drew: a row each, the one picked out marked, each with its note under
// it where there's room for every one's; where there isn't room for every
// row, as many as fit, among them the one picked out, then how many more
// there are. Without seats, it's why the account has no sessions, where that
// can be said.
func (f Frame) backBody(c *canvas, doc status.Document, fc face, seats []seated, now time.Time, x, y, width, room int) int {
	if room < 1 {
		return 0
	}
	if len(seats) == 0 {
		return f.unseated(c, doc, fc, now, x, y, width, room)
	}
	picked := f.picked(fc)
	first, shown := shownOf(seats, room, picked)
	noted, column, row := 2*len(seats) <= room, modelColumnOf(seats), y
	for _, s := range seats[first : first+shown] {
		chosen := picked(s)
		c.line(x, row, f.seatLine(s, column, chosen, now).fit(width))
		if chosen {
			c.surface(x-padding, row, width+2*padding, hue{token: theme.BgSelection})
		}
		row++
		if noted {
			c.line(x+noteIndent, row, line{{"╰ " + note(doc, fc.account.ID, s, now), mutedInk}}.fit(width-noteIndent))
			row++
		}
	}
	if shown < len(seats) {
		c.line(x, row, line{{"+" + strconv.Itoa(len(seats)-shown) + " more", dimInk}}.fit(width))
		row++
	}
	return row - y
}

// picked reports, of a seat on the back of the card fc, whether it's the one
// picked out, which only the card with the focus has.
func (f Frame) picked(fc face) func(seated) bool {
	return func(s seated) bool {
		return fc.focused && !f.Selected.IsZero() && s.seat() == f.Selected
	}
}

// shownOf is which of the seats a back's body room rows tall shows: shown
// of them from first, every one where they fit; else as many as fit with a
// row left to say how many more there are, among them the one picked out.
func shownOf(seats []seated, room int, picked func(seated) bool) (first, shown int) {
	if len(seats) <= room {
		return 0, len(seats)
	}
	shown = room - 1
	if i := slices.IndexFunc(seats, picked); i >= shown {
		first = i - shown + 1
	}
	return first, shown
}

// modelColumnOf is how many cells the column of the seats' models takes:
// modelColumn, or more for a longer model's name, with a blank after it.
func modelColumnOf(seats []seated) int {
	column := modelColumn
	for _, s := range seats {
		column = max(column, ansi.StringWidth(s.assignment.Name())+2)
	}
	return column
}

// seatLine is a seat's row on a card's back: ▸ where it's the one picked out;
// its dot, lit while busy; its session's id, bright and bold while busy; its
// model, its column cells wide; and what it's doing.
func (f Frame) seatLine(s seated, column int, chosen bool, now time.Time) line {
	lead := spaces(seatLead)
	if chosen {
		lead = span{"▸ ", keyInk}
	}
	busy := f.lit(s, now)
	id := span{padded(sessionID(s.session.ID), idColumn), titleInk}
	if !busy {
		id.ink = idleIDInk
	}
	d := f.doing(s, now)
	return line{lead, sessionDot(busy), spaces(1), id, {padded(s.assignment.Name(), column), secondaryInk}, {padded(d.word, doingColumn), d.ink}, {d.detail, secondaryInk}}
}

// idleIDInk is an idle session's id: muted, and not bold, as only a busy
// one's is.
var idleIDInk = ink{token: theme.TextMuted}

// doing is what a session's model on an account is doing, as its row on a
// card's back says it: a word, in its ink, and what follows it.
type doing struct {
	word   string
	ink    ink
	detail string
}

// The words of what a request the request stream tells of is doing, as a
// card's back says them: its answer streaming, or just ended, in
// accent.mode, or sent, and waiting on it, in accent.attention.
var (
	streamingInk = ink{token: theme.AccentMode, bold: true}
	waitingInk   = ink{token: theme.AccentAttention, bold: true}
)

// doing is what the seat s is doing at now: where the request stream tells
// of a request of its in flight, its answer streaming, and how many tokens
// so far, as in "streaming ↓ ~1.2k", or, sent with nothing back yet, how long
// it has waited, as in "waiting 38s"; while an answer of its is held as it
// ends, its tokens, exact as its closing usage counts them, as in "↓ 1.3k".
// Else, with the stream, how long it has been idle, as in "idle 38s"; and
// without it, busy, "seen now", or idle so long, as in "idle 9m".
func (f Frame) doing(s seated, now time.Time) doing {
	c, ok := f.Traffic.call(s.plug())
	switch {
	case ok && c.Doing == Streaming:
		return doing{word: "streaming", ink: streamingInk, detail: c.streamed()}
	case ok && c.Doing.InFlight():
		return doing{word: "waiting", ink: waitingInk, detail: lapsed(c.Since, now)}
	case ok && c.Doing == Answered:
		return doing{word: c.streamed(), ink: streamingInk}
	case !f.Traffic.Live && f.lit(s, now):
		return doing{word: "seen", ink: secondaryInk, detail: "now"}
	default:
		return doing{word: "idle", ink: dimInk, detail: lapsed(f.lastSeen(s), now)}
	}
}

// lapsed says how long has passed from since to now: in seconds, as "38s",
// within a minute, and from then as seenAgo rounds it, as "9m".
func lapsed(since, now time.Time) string {
	if d := now.Sub(since); d < time.Minute {
		return strconv.Itoa(max(int(d/time.Second), 0)) + "s"
	}
	return seenAgo(now, since)
}

// note is what's noted under a seat's row on the back of the card of the
// account with the given id, times in now's time zone, as Dated shows them.
// Where its session's own pin names another account: that the pin yielded
// here at a limit, where it did, as the router's reason for the seat says,
// else that the session goes there from its next request. Else where its
// other models go, to other accounts, as in "its opus is on side"; that its
// own pin keeps it here, and since when, where the pin moved it here; where
// it moved here from, and when, as the router told of it; else since when
// it's been here.
func note(doc status.Document, id string, s seated, now time.Time) string {
	pin := s.session.Pin
	switch {
	case pin != "" && pin != id && yieldedFrom(s.assignment.Reason) == pin:
		return "its pin to " + named(doc, pin) + " yielded here"
	case pin != "" && pin != id:
		return "goes to " + named(doc, pin) + " from its next request"
	}
	if others := elsewhere(doc, s.session, id); others != "" {
		return others
	}
	move, moved := movedHere(doc, s, id)
	pinned := s.assignment.Pinned || pin == id
	switch {
	case pinned && moved && move.Reason == reasonOwnPin:
		return "pinned here at " + status.Dated(now, move.At)
	case pinned:
		return "pinned here"
	case moved:
		return "moved from " + named(doc, move.From) + " at " + status.Dated(now, move.At)
	default:
		return "here since " + status.Dated(now, s.assignment.AssignedAt)
	}
}

// yieldedFrom is the account a session's own pin named as it yielded at a
// limit, as the reason the router gives for where the session went says, as
// in "pin yields: side has no room": "" where the pin didn't yield.
func yieldedFrom(reason string) string {
	rest, ok := strings.CutPrefix(reason, reasonPinYields)
	if !ok {
		return ""
	}
	account, _, _ := strings.Cut(rest, " ")
	return account
}

// elsewhere says where the session's models go that go to other accounts
// than the one with the given id, as in "its opus is on side", or "its opus
// and haiku are on side, its sonnet on client": "" where none does.
func elsewhere(doc status.Document, s status.Session, id string) string {
	var said []string
	for _, models := range s.ByAccount() {
		if models.Account == id {
			continue
		}
		verb := ""
		switch {
		case len(said) > 0:
		case len(models.Names) > 1:
			verb = " are"
		default:
			verb = " is"
		}
		said = append(said, "its "+prose.List(models.Names)+verb+" on "+named(doc, models.Account))
	}
	return strings.Join(said, ", ")
}

// movedHere is the newest of doc's events that tells of the seat's session's
// model moving to the account with the given id, reporting false where none
// does.
func movedHere(doc status.Document, s seated, id string) (status.Event, bool) {
	i := slices.IndexFunc(doc.Events, func(e status.Event) bool {
		return e.Kind == status.EventMoved && e.To == id && e.Session == s.session.ID && e.Model == s.assignment.Model
	})
	if i < 0 {
		return status.Event{}, false
	}
	return doc.Events[i], true
}

// unseated draws why the card fc's account has no sessions, where it can be
// said, from x along row y, width cells wide and room rows tall at most, and
// returns how many rows it drew, as unseatedBy words it.
func (f Frame) unseated(c *canvas, doc status.Document, fc face, now time.Time, x, y, width, room int) int {
	if fc.sessions > 0 {
		return 0
	}
	lines := flow(f.unseatedBy(doc, fc.account.ID, now), slices.Repeat([]int{width}, room), " ")
	for i, l := range lines {
		c.line(x, y+i, l)
	}
	return len(lines)
}

// unseatedBy says why the account with the given id has no sessions, times
// in now's time zone, as Dated shows them: probing, why there's no router to
// list them, as RECENT says; else, of the router's events, the newest that
// took sessions off it, a limit it reached, as in "3 moved to side at 14:12,
// when personal reached its limit", or a session moving, as in "d28c moved
// to side at 14:39 (pin)". It's nothing where none did, or where a session
// has come to it since.
func (f Frame) unseatedBy(doc status.Document, id string, now time.Time) []chunk {
	if doc.Source != status.SourceRouter {
		return []chunk{{text: f.quiet(doc)}}
	}
	for _, e := range doc.Events {
		at := " at " + status.Dated(now, e.At)
		switch {
		case arrives(e, id):
			return nil
		case e.Kind == status.EventLimit && e.Account == id && e.Count > 0:
			to := cmp.Or(movedTo(doc, e), " to other accounts")
			return []chunk{
				{text: line{{strconv.Itoa(e.Count) + " moved" + to + at + ",", mutedInk}}},
				{text: line{{"when " + named(doc, id) + " reached its limit", mutedInk}}},
			}
		case e.Kind == status.EventMoved && !counted(e) && e.From == id:
			return []chunk{{text: slices.Concat(line{{sessionID(e.Session) + " moved to " + named(doc, e.To) + at, mutedInk}}, why(doc, e.Reason))}}
		}
	}
	return nil
}

// arrives reports whether the event put a session on the account with the
// given id: one starting on it, or moving to it, as another account's limit
// moved it or not, or the limit that moved every one of its sessions there.
func arrives(e status.Event, id string) bool {
	switch e.Kind {
	case status.EventStarted:
		return e.Account == id
	case status.EventMoved:
		return e.To == id
	case status.EventLimit:
		return e.Account != id && e.To == id && e.Count > 0
	default:
		return false
	}
}
