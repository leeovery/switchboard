package dashboard

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The columns of a seat's row in Sessions' plain list: the cells before its
// dot; its session's id, its model, at the least, and what it's doing's word,
// each with the blanks after it; and how far in the note under it starts.
const (
	seatLead    = 2
	idColumn    = 6
	modelColumn = 8
	doingColumn = 11
	noteIndent  = 4
)

// Seat is a session's requests of one model on an account: a row under the
// account's panel in Sessions' plain list, and a call in its switchboard.
type Seat struct {
	Session, Model string
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

// seatedOn are the sessions listed on the account with the given id, a seat
// each of their models there: in the order the router lists them, the
// session seen last first, and of each session, the model used last first.
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

// busyWithin is how lately a session was seen for it to be busy: it's busy
// when it was seen in the last minute, or, as the request stream tells,
// while a request of its is in flight.
const busyWithin = time.Minute

// lit reports whether the seat s is busy at now: a request of its in flight,
// as the request stream tells, or it was seen at work in the last minute.
func (f Frame) lit(s seated, now time.Time) bool {
	if c, ok := f.Traffic.call(s.plug()); ok && c.Doing.InFlight() {
		return true
	}
	return now.Sub(f.lastSeen(s)) < busyWithin
}

// lastSeen is when the seat s was last seen at work: as the router listed
// it, or later, as the request stream told of it.
func (f Frame) lastSeen(s seated) time.Time {
	return score.Later(s.assignment.LastSeen, f.Traffic.Seen[s.plug()])
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

// seatLine is a seat's row in the plain list: its dot, lit while busy; its
// session's id, bright and bold while busy; its model, its column cells
// wide; and what it's doing.
func (f Frame) seatLine(s seated, column int, now time.Time) line {
	busy := f.lit(s, now)
	id := span{padded(status.ShownID(s.session.ID), idColumn), titleInk}
	if !busy {
		id.ink = idleIDInk
	}
	d := f.doing(s, now)
	return line{spaces(seatLead), sessionDot(busy), spaces(1), id, {padded(s.assignment.Name(), column), secondaryInk}, {padded(d.word, doingColumn), d.ink}, {d.detail, secondaryInk}}
}

// sessionDot is a session's dot: lit while it's busy, else hollow and dim.
func sessionDot(lit bool) span {
	if lit {
		return span{"●", positiveInk}
	}
	return span{"○", dimInk}
}

// idleIDInk is an idle session's id: muted, and not bold, as only a busy
// one's is.
var idleIDInk = ink{token: theme.TextMuted}

// doing is what a session's model on an account is doing, as its row in the
// plain list says it: a word, in its ink, and what follows it.
type doing struct {
	word   string
	ink    ink
	detail string
}

// The words of what a request the request stream tells of is doing, as the
// plain list says them: its answer streaming, or just ended, in
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
// within a minute, and from then as prose.SeenAgo rounds it, as "9m".
func lapsed(since, now time.Time) string {
	if d := now.Sub(since); d < time.Minute {
		return strconv.Itoa(max(int(d/time.Second), 0)) + "s"
	}
	return prose.SeenAgo(now, since)
}

// note is what's noted under a seat's row under the panel of the account
// with the given id, times in now's time zone, as Dated shows them.
// Where its session's own pin names another account: that the pin yielded
// here, as the router says it has, the account it names having had no room,
// whether the session moved here then or stayed; else that the session goes
// there from its next request. Else where its other models go, to other
// accounts, as in "its opus is on side"; that its own pin keeps it here, and
// since when, where it was given the pin as it ran; where it moved here
// from, and when, as the router told of it; else since when it's been here.
func note(doc status.Document, id string, s seated, now time.Time) string {
	pin := s.session.Pin
	switch {
	case pin != "" && pin != id && s.assignment.Yielded:
		return "its pin to " + docNamed(doc, pin) + " yielded here"
	case pin != "" && pin != id:
		return "goes to " + docNamed(doc, pin) + " from its next request"
	}
	if others := elsewhere(doc, s.session, id); others != "" {
		return others
	}
	move, moved := movedHere(doc, s, id)
	pinned := s.assignment.Pinned || pin == id
	switch {
	case pinned && !s.assignment.PinnedAt.IsZero():
		return "pinned here at " + prose.Dated(now, s.assignment.PinnedAt)
	case pinned:
		return "pinned here"
	case moved:
		return "moved from " + docNamed(doc, move.From) + " at " + prose.Dated(now, move.At)
	default:
		return "here since " + prose.Dated(now, s.assignment.AssignedAt)
	}
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
		said = append(said, "its "+prose.List(models.Names)+verb+" on "+docNamed(doc, models.Account))
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

// unseatedBy says why the account with the given id has no sessions, times
// in now's time zone, as Dated shows them: probing, why there's no router to
// list them, as RECENT says; while a limit holds it, the sessions that limit
// moved off it, as its event, which LimitEvent finds, tells, where none has
// come to it since; else, of the router's events, the newest that took
// sessions off it, a limit it reached, or a session moving, as in "d28c
// moved to side at 14:39 (pin)". It's nothing where none did, or where a
// session has come to it since.
func (f Frame) unseatedBy(doc status.Document, id string, now time.Time) []chunk {
	if doc.Source != status.SourceRouter {
		return []chunk{{text: f.quiet(f.Moment(doc, now))}}
	}
	if a, ok := doc.Account(id); ok {
		if e, ok := doc.LimitEvent(a, now); ok && e.Count > 0 && !arrivedSince(doc, e, id) {
			return limitMoved(doc, e, id, now)
		}
	}
	for _, e := range doc.Events {
		switch {
		case arrives(e, id):
			return nil
		case e.Kind == status.EventLimit && e.Account == id && e.Count > 0:
			return limitMoved(doc, e, id, now)
		case e.Kind == status.EventMoved && !counted(e) && e.From == id:
			return []chunk{{text: slices.Concat(line{{status.ShownID(e.Session) + " moved to " + docNamed(doc, e.To) + " at " + prose.Dated(now, e.At), mutedInk}}, why(doc, e.Reason))}}
		}
	}
	return nil
}

// limitMoved says what the limit's event e moved off the account with the
// given id, its time in now's time zone, as Dated shows it, as in "3 moved to
// side at 14:12, when personal reached its limit".
func limitMoved(doc status.Document, e status.Event, id string, now time.Time) []chunk {
	to := cmp.Or(movedTo(doc, e), " to other accounts")
	return []chunk{
		{text: line{{strconv.Itoa(e.Count) + " moved" + to + " at " + prose.Dated(now, e.At) + ",", mutedInk}}},
		{text: line{{"when " + docNamed(doc, id) + " reached its limit", mutedInk}}},
	}
}

// arrivedSince reports whether a session has come to the account with the
// given id since the event e, as arrives says of the router's events after
// it.
func arrivedSince(doc status.Document, e status.Event, id string) bool {
	for _, later := range doc.Events {
		if later.ID == e.ID {
			return false
		}
		if arrives(later, id) {
			return true
		}
	}
	return false
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
