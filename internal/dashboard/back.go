package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The columns of a seat's row on a card's back: its session's id, its model,
// at the least, and what it's doing's word, each with the blanks after it;
// and how far in the note under it starts.
const (
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
// id, a row each of its card's back, in the order the router lists them, the
// session seen last first, and of each session, the model used last first.
func Seats(sessions []status.Session, id string) []Seat {
	on := seatedOn(sessions, id)
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

// busy reports whether s was seen at work in the last minute before now.
func (s seated) busy(now time.Time) bool {
	return now.Sub(s.assignment.LastSeen) < busyWithin
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
// all fit without it alone; a row for each of its sessions' models, or why
// it has none; then, where there's room, LATELY, what has befallen it
// lately; and at its foot, the keys.
func (f Frame) back(c *canvas, doc status.Document, fc face, now time.Time, x, y, width, rows int) {
	if rows < 1 {
		return
	}
	seats := seatedOn(f.Sessions, fc.account.ID)
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
	f.lately(c, doc, fc.account.ID, now, x, under, width, foot)
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
		c.line(x, row, seatLine(s, column, chosen, now).fit(width))
		if chosen {
			c.surface(x-1, row, width+2, hue{token: theme.BgSelection})
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
// its dot, lit while busy; its session's id, bold, bright while busy; its
// model, its column cells wide; and what it's doing.
func seatLine(s seated, column int, chosen bool, now time.Time) line {
	lead := spaces(2)
	if chosen {
		lead = span{"▸ ", keyInk}
	}
	busy := s.busy(now)
	id := span{padded(sessionID(s.session.ID), idColumn), titleInk}
	if !busy {
		id.ink = ink{token: theme.TextMuted, bold: true}
	}
	d := doingOf(s, now)
	return line{lead, sessionDot(busy), spaces(1), id, {padded(s.assignment.Name(), column), secondaryInk}, {padded(d.word, doingColumn), d.ink}, {d.detail, secondaryInk}}
}

// doing is what a session's model on an account is doing, as its row on a
// card's back says it: a word, in its ink, and what follows it.
type doing struct {
	word   string
	ink    ink
	detail string
}

// doingOf is what the seat is doing at now, as the router last saw it: busy
// while it was seen in the last minute, "seen now"; else idle so long, as in
// "idle 9m".
func doingOf(s seated, now time.Time) doing {
	if s.busy(now) {
		return doing{word: "seen", ink: secondaryInk, detail: "now"}
	}
	return doing{word: "idle", ink: dimInk, detail: status.Countdown(s.assignment.LastSeen, now)}
}

// note is what's noted under a seat's row on the back of the card of the
// account with the given id, times in now's time zone: where its session
// goes from its next request, where its own pin sends it to another
// account; where its other models go, to other accounts, as in "its opus is
// on side"; that its own pin keeps it here, and since when, where it moved
// here for it; where it moved here from, and when, as the router told of it;
// else since when it's been here.
func note(doc status.Document, id string, s seated, now time.Time) string {
	pin := s.session.Pin
	if pin != "" && pin != id {
		return "goes to " + named(doc, pin) + " from its next request"
	}
	if others := elsewhere(doc, s.session, id); others != "" {
		return others
	}
	move, moved := movedHere(doc, s, id)
	pinned := s.assignment.Pinned || pin == id
	switch {
	case pinned && moved:
		return "pinned here at " + status.When(now, move.At)
	case pinned:
		return "pinned here"
	case moved:
		return "moved from " + named(doc, move.From) + " at " + status.When(now, move.At)
	default:
		return "here since " + status.When(now, s.assignment.AssignedAt)
	}
}

// elsewhere says where the session's models go that go to other accounts
// than the one with the given id, as in "its opus is on side", or "its opus
// and haiku are on side, its sonnet on client": "" where none does.
func elsewhere(doc status.Document, s status.Session, id string) string {
	var accounts []string
	models := make(map[string][]string)
	for _, a := range s.Assignments {
		if a.Account == id {
			continue
		}
		if _, ok := models[a.Account]; !ok {
			accounts = append(accounts, a.Account)
		}
		if name := a.Name(); !slices.Contains(models[a.Account], name) {
			models[a.Account] = append(models[a.Account], name)
		}
	}
	said := make([]string, len(accounts))
	for i, account := range accounts {
		verb := ""
		switch {
		case i > 0:
		case len(models[account]) > 1:
			verb = " are"
		default:
			verb = " is"
		}
		said[i] = "its " + prose.List(models[account]) + verb + " on " + named(doc, account)
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
// in now's time zone: probing, why there's no router to list them, as RECENT
// says; else, of the router's events, the newest that took sessions off it,
// a limit it reached, as in "3 moved to side at 14:12, when personal reached
// its limit", or a session moving, as in "d28c moved to side at 14:39 (pin)".
// It's nothing where none did.
func (f Frame) unseatedBy(doc status.Document, id string, now time.Time) []chunk {
	if doc.Source != status.SourceRouter {
		return []chunk{{text: f.quiet(doc)}}
	}
	for _, e := range doc.Events {
		at := " at " + status.When(now, e.At)
		switch {
		case e.Kind == status.EventLimit && e.Account == id && e.Count > 0:
			to := "to other accounts"
			if e.To != "" {
				to = "to " + named(doc, e.To)
			}
			return []chunk{
				{text: line{{strconv.Itoa(e.Count) + " moved " + to + at + ",", mutedInk}}},
				{text: line{{"when " + named(doc, id) + " reached its limit", mutedInk}}},
			}
		case e.Kind == status.EventMoved && !counted(e) && e.From == id:
			return []chunk{{text: slices.Concat(line{{sessionID(e.Session) + " moved to " + named(doc, e.To) + at, mutedInk}}, why(e.Reason))}}
		}
	}
	return nil
}
