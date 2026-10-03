package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// room is how an account stands at a moment, as a lane of Runway draws it.
type room int

const (
	// roomUnknown is an account nothing has been read of: its lane is blank.
	roomUnknown room = iota
	// roomOpen is an account that can take a session.
	roomOpen
	// roomDraining is an account that can take a session, but is heading to
	// run out before its reset.
	roomDraining
	// roomNone is an account that can't take a session.
	roomNone
)

// has reports whether an account standing so can take a session.
func (r room) has() bool {
	return r == roomOpen || r == roomDraining
}

// The day's timeline starts the hour before now, on the ten minutes, and
// spans dayColumns columns of dayStep each, as the lanes take them at 160
// columns: across any other number, it spans as long.
const (
	dayBefore  = time.Hour
	dayStep    = 10 * time.Minute
	dayColumns = 136
)

// The week's timeline starts a day before now, and spans a week.
const (
	weekBefore = 24 * time.Hour
	weekSpan   = 7 * 24 * time.Hour
)

// timeline is the time Runway's lanes show, across columns cells: from
// start, each column step long, now falling in one of them.
type timeline struct {
	start   time.Time
	step    time.Duration
	columns int
	now     time.Time
}

// timelineOf is the span's timeline at now across columns cells: the day,
// from the hour before now, its start on the ten minutes, so that where a
// column is dayStep, the hours fall where columns start; or the week, from a
// day before now.
func timelineOf(s Span, now time.Time, columns int) timeline {
	across := time.Duration(max(columns, 1))
	if s == Week {
		return timeline{start: now.Add(-weekBefore), step: weekSpan / across, columns: columns, now: now}
	}
	from := now.Add(-dayBefore)
	midnight := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	start := midnight.Add(from.Sub(midnight).Truncate(dayStep))
	return timeline{start: start, step: dayColumns * dayStep / across, columns: columns, now: now}
}

// column is the column the moment t falls in: less than zero before the
// first, and columns or more after the last.
func (tl timeline) column(t time.Time) int {
	d := t.Sub(tl.start)
	col := int(d / tl.step)
	if d < 0 && d%tl.step != 0 {
		col--
	}
	return col
}

// nowColumn is the column now falls in.
func (tl timeline) nowColumn() int {
	return tl.column(tl.now)
}

// moment is the moment column col stands for: its middle, or in the column
// now falls in, now, which every column after it is ahead of.
func (tl timeline) moment(col int) time.Time {
	if col == tl.nowColumn() {
		return tl.now
	}
	return tl.start.Add(time.Duration(col)*tl.step + tl.step/2)
}

// stretch is a stretch of time an account can't take a session: from when,
// zero for since before anything Runway shows; until when it's back, zero
// where that isn't known; what starts it, in words, as in "runs out ~16:05";
// and the reset of the window whose running out starts it, which it's back
// at as that resets, zero for a stretch a hold starts. One that stands for
// the account's state, as without a token, says what that means, then, in
// place of when it's back.
type stretch struct {
	from, until time.Time
	cause       string
	resets      time.Time
	then        string
}

// holds reports whether the stretch holds the account back at t.
func (s stretch) holds(t time.Time) bool {
	return (s.from.IsZero() || !t.Before(s.from)) && (s.until.IsZero() || t.Before(s.until))
}

// says is what's said where the stretch starts: what starts it, bold in
// destructive; then what that means, where it stands for the account's
// state; else when it's back, where that's known, and that that's as it
// resets, where it is, as in "runs out ~16:05 · back 17:10, as it resets".
func (s stretch) says(now time.Time) line {
	l := line{{s.cause, exhaustedInk}}
	switch {
	case s.then != "":
		l = append(l, span{status.Separator + s.then, mutedInk})
	case !s.until.IsZero():
		l = append(l, span{status.Separator + "back " + status.Dated(now, s.until), mutedInk})
		if s.until.Equal(s.resets) {
			l = append(l, span{", as it resets", mutedInk})
		}
	}
	return l
}

// lane is an account's room over the time Runway shows, as the document has
// it at a moment: the account, and its place; whether anything is known of
// it; the stretches it can't take a session, in order, those that meet run
// together; and until when, from now, it's heading to run out, zero where it
// isn't.
type lane struct {
	account   status.Account
	place     int
	known     bool
	stretches []stretch
	drains    time.Time
}

// laneOf is doc's account a's lane at now, its room going by the windows the
// span counts, as counts says: without a usable token, or with its usage
// unreadable, it has none all along, as its state says, and nothing is known
// of one not read yet. Read, it has none while a window counted holds it
// back, as stretchesOf says, nor, over the day, while the upstream refuses its
// every request; and from now, it's heading to run out until the last of
// those windows that run out before they reset does.
func (f Frame) laneOf(doc status.Document, a status.Account, now time.Time) lane {
	l := lane{account: a, place: place(doc, a.ID), known: true}
	switch state := doc.StateOf(a, now, f.Policy); state.Condition {
	case status.Tokenless, status.Unreadable:
		l.stretches = []stretch{{cause: state.Says, then: state.Then}}
		return l
	case status.Unread:
		l.known = false
		return l
	}
	var held []stretch
	for _, w := range a.Windows {
		if !f.counts(w) {
			continue
		}
		s := standingOf(doc, a, w, now, f.Policy)
		if s.runsOut && s.out.At.After(l.drains) {
			l.drains = s.out.At
		}
		held = append(held, f.stretchesOf(doc, a, s, now)...)
	}
	if f.Span == Day && a.Refused.Holds(now) && a.Refused.Family == "" {
		held = append(held, stretch{until: a.Refused.Until, cause: a.Refused.Answer()})
	}
	l.stretches = merged(held)
	return l
}

// counts reports whether the span counts the window w toward an account's
// room: over the day, every window every model shares; over the week, the
// weeks among them, those longer than a day.
func (f Frame) counts(w quota.Window) bool {
	switch length, ok := quota.Length(w.Key); {
	case !f.Policy.IsShared(w.Key):
		return false
	case f.Span == Week:
		return ok && length > day
	default:
		return true
	}
}

// stretchesOf are the stretches doc's account a can't take a session for, at
// now, for its window standing as s: while it's held at its limit, from when
// it reached it, where the router told of that, until it's back; or while
// it's at its reserve, where that holds the account back, until the window
// resets; and once it runs out before it resets, as RunsOut has it, until it
// resets, as a window a limit naming none holds can, once that lifts. Over
// the day, a window other than the one a request starts is named, as in
// "week runs out ~Fri 04:06".
func (f Frame) stretchesOf(doc status.Document, a status.Account, s standing, now time.Time) []stretch {
	var out []stretch
	w := s.window
	switch {
	case s.held:
		since := heldSince(doc, a, w.Key, now, f.Policy)
		cause := "limit reached"
		if !since.IsZero() {
			cause += " " + status.Dated(now, since)
		}
		out = append(out, stretch{from: since, until: s.back, cause: cause})
	case doc.ReserveHolds(a) && slices.Contains(a.AtReserve, w.Key):
		out = append(out, stretch{until: w.ResetsAt, resets: w.ResetsAt, cause: f.naming(w) + "at its reserve"})
	}
	if s.runsOut {
		verb := "runs out ~"
		if s.out.Reserve {
			verb = "reaches its reserve ~"
		}
		out = append(out, stretch{from: s.out.At, until: w.ResetsAt, resets: w.ResetsAt, cause: f.naming(w) + verb + status.Dated(now, s.out.At)})
	}
	return out
}

// naming is how the words of a stretch the window w starts lead: over the
// day, with the window's name, but for the window a request starts, as in
// "week "; over the week, without it, as it's the weeks' alone.
func (f Frame) naming(w quota.Window) string {
	if f.Span == Week || w.Key == f.Policy.Started {
		return ""
	}
	return status.InProse(w.Label) + " "
}

// heldSince is when doc's account a reached the limit that holds its window
// with the given key back at now, as the router told of it: zero where it
// didn't, as while probing, or where no limit it reached holds that window.
func heldSince(doc status.Document, a status.Account, key string, now time.Time, policy score.Policy) time.Time {
	held, ok := a.Held(now, policy)
	if !ok || !held.Holds(key) || !a.Limit.Holds(now) {
		return time.Time{}
	}
	return limitReached(doc, a.ID, now)
}

// merged are the stretches in order, the earliest first, those that meet or
// overlap run together: from the first's start until the last of their
// ends, said as the first is, and back as its window resets only where
// that's when they end.
func merged(stretches []stretch) []stretch {
	slices.SortStableFunc(stretches, func(x, y stretch) int { return x.from.Compare(y.from) })
	var out []stretch
	for _, s := range stretches {
		n := len(out)
		if n == 0 || !out[n-1].until.IsZero() && s.from.After(out[n-1].until) {
			out = append(out, s)
			continue
		}
		if last := &out[n-1]; !last.until.IsZero() && (s.until.IsZero() || s.until.After(last.until)) {
			last.until = s.until
		}
	}
	return out
}

// at is how the lane's account stands at t: unknown where nothing is known
// of it; without room within a stretch; from now on, draining until it runs
// out; else with room.
func (l lane) at(t, now time.Time) room {
	switch {
	case !l.known:
		return roomUnknown
	case slices.ContainsFunc(l.stretches, func(s stretch) bool { return s.holds(t) }):
		return roomNone
	case !t.Before(now) && t.Before(l.drains):
		return roomDraining
	default:
		return roomOpen
	}
}

// cells are how the lane's account stands in each column of the timeline, at
// the moment each stands for, and in the column each stretch shows from, as
// column says, without room.
func (l lane) cells(tl timeline) []room {
	cells := make([]room, max(tl.columns, 0))
	for col := range cells {
		cells[col] = l.at(tl.moment(col), tl.now)
	}
	for _, s := range l.stretches {
		if col, ok := s.column(tl); ok {
			cells[col] = roomNone
		}
	}
	return cells
}

// column is the column of the timeline the stretch shows from, reporting
// false where it shows in none: the first whose moment it holds; or, too
// short to hold any, the column it starts in, so none goes unseen.
func (s stretch) column(tl timeline) (int, bool) {
	first, last := 0, tl.columns-1
	if !s.from.IsZero() {
		first = max(tl.column(s.from), 0)
	}
	if !s.until.IsZero() {
		last = min(last, tl.column(s.until))
	}
	for col := first; col <= last; col++ {
		if s.holds(tl.moment(col)) {
			return col, true
		}
	}
	col := tl.column(s.from)
	return col, !s.from.IsZero() && col >= 0 && col < tl.columns
}
