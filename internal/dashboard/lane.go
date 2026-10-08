package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
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
// from the hour before now, its start on the ten minutes since its day's
// start, as dayfile.DayStart gives it, so that where a column is dayStep, the
// hours fall where columns start; or the week, from a day before now.
func timelineOf(s Span, now time.Time, columns int) timeline {
	across := time.Duration(max(columns, 1))
	if s == Week {
		return timeline{start: now.Add(-weekBefore), step: weekSpan / across, columns: columns, now: now}
	}
	from := now.Add(-dayBefore)
	dayStart := dayfile.DayStart(from, 0)
	start := dayStart.Add(from.Sub(dayStart).Truncate(dayStep))
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

// interval is a stretch of time: from when, zero for since before anything
// Runway shows; until when, zero where that isn't known.
type interval struct {
	from, until time.Time
}

// holds reports whether the interval holds the moment t.
func (i interval) holds(t time.Time) bool {
	return (i.from.IsZero() || !t.Before(i.from)) && (i.until.IsZero() || t.Before(i.until))
}

// column is the column of the timeline the interval shows from, reporting
// false where it shows in none: the first whose moment it holds; or, too
// short to hold any, the column it starts in, so none goes unseen.
func (i interval) column(tl timeline) (int, bool) {
	first, last := 0, tl.columns-1
	if !i.from.IsZero() {
		first = max(tl.column(i.from), 0)
	}
	if !i.until.IsZero() {
		last = min(last, tl.column(i.until))
	}
	for col := first; col <= last; col++ {
		if i.holds(tl.moment(col)) {
			return col, true
		}
	}
	col := tl.column(i.from)
	return col, !i.from.IsZero() && col >= 0 && col < tl.columns
}

// cause is what holds an account back over its interval: in words, as in
// "runs out ~16:05"; and the reset of the window whose running out it is,
// which the account's back at as that resets, zero for a hold. One that
// stands for the account's state, as without a token, says what that means,
// then, in place of when the account's back.
type cause struct {
	interval
	says   string
	resets time.Time
	then   string
}

// same reports whether the cause tells the same as other: the same words over
// the same interval, as a limit holding several windows tells of each.
func (c cause) same(other cause) bool {
	return c.says == other.says && c.from.Equal(other.from) && c.until.Equal(other.until)
}

// stretch is a stretch of time an account can't take a session, from the
// start of the first of its causes until the last of their ends, and its
// causes, in the order they start.
type stretch struct {
	interval
	causes []cause
}

// told is what's said of the stretch, a line for each of its causes, to be
// said where it starts: what it is, bold in destructive; and of the cause the
// stretch is back from, as backFrom says, when it's back, where that's known,
// and that that's as its window resets, where it is, as in "week runs out
// ~16:30 · back Thu 13:12, as it resets". One standing for the account's
// state says what that means instead.
func (s stretch) told(now time.Time) []line {
	back := s.backFrom()
	told := make([]line, len(s.causes))
	for i, c := range s.causes {
		told[i] = line{{c.says, exhaustedInk}}
		switch {
		case c.then != "":
			told[i] = append(told[i], span{status.Separator + c.then, mutedInk})
		case i == back && !s.until.IsZero():
			told[i] = append(told[i], span{status.Separator + "back " + status.Dated(now, s.until), mutedInk})
			if s.until.Equal(c.resets) {
				told[i] = append(told[i], span{", as it resets", mutedInk})
			}
		}
	}
	return told
}

// backFrom is the cause the stretch is back from, by its place: the last of
// those that end as it does, where room really returns.
func (s stretch) backFrom() int {
	i := len(s.causes) - 1
	for i > 0 && !s.causes[i].until.Equal(s.until) {
		i--
	}
	return i
}

// lane is an account's room over the time Runway shows, as the document has
// it at a moment: the account, and its place; whether anything is known of
// it; the stretches it can't take a session, in order; and until when, from
// now, it's heading to run out, zero where it isn't, and in words, as in
// "week runs out ~Sun 04:06".
type lane struct {
	account   status.Account
	place     int
	known     bool
	stretches []stretch
	drains    time.Time
	drained   string
}

// laneOf is doc's account a's lane at now, its room going by the windows the
// span counts, as counts says: without a usable token, as its state says, or
// with its usage unreadable, as Unread says, whatever holds it back, it has
// none all along, and nothing is known of one not read yet. Read, it has
// none while a window counted holds it back, as causesOf says, nor, over the
// day, while the upstream refuses its every request; and from now, it's
// heading to run out until the last of those windows that run out before
// they reset does.
func (f Frame) laneOf(doc status.Document, a status.Account, now time.Time) lane {
	l := lane{account: a, place: place(doc, a.ID), known: true}
	state := doc.StateOf(a, now, f.Policy)
	if unread, ok := a.Unread(); ok && a.TokenSet {
		state = unread
	}
	switch state.Condition {
	case status.Tokenless, status.Unreadable:
		l.stretches = merged([]cause{{says: state.Says, then: state.Then}})
		return l
	case status.Unread:
		l.known = false
		return l
	}
	var causes []cause
	for _, w := range a.Windows {
		if !f.counts(w) {
			continue
		}
		s := standingOf(doc, a, w, now, f.Policy)
		if s.runsOut && s.out.At.After(l.drains) {
			l.drains, l.drained = s.out.At, f.runOutSaid(s, now)
		}
		causes = append(causes, f.causesOf(doc, a, s, now)...)
	}
	if f.Span == Day && a.Refused.Holds(now) && a.Refused.Family == "" {
		causes = append(causes, cause{until: a.Refused.Until, says: a.Refused.Answer()})
	}
	l.stretches = merged(causes)
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

// causesOf are what holds doc's account a back, at now, of its window
// standing as s: while it's held at its limit, from when it reached it, as
// its standing says, until it's back; or while it's at its reserve, where
// that holds the account back, until the window resets; and once it runs
// out before it resets, as RunsOut has it, until it resets, as a window a
// limit naming none holds can, once that lifts. Over the day, a window other
// than the one a request starts is named, as in "week runs out ~Fri 04:06".
func (f Frame) causesOf(doc status.Document, a status.Account, s standing, now time.Time) []cause {
	var causes []cause
	w := s.window
	switch {
	case s.held:
		says := "limit reached"
		if !s.since.IsZero() {
			says += " " + status.Dated(now, s.since)
		}
		causes = append(causes, cause{from: s.since, until: s.back, says: says})
	case doc.ReserveHolds(a) && slices.Contains(a.AtReserve, w.Key):
		causes = append(causes, cause{until: w.ResetsAt, says: f.naming(w) + "at its reserve", resets: w.ResetsAt})
	}
	if s.runsOut {
		causes = append(causes, cause{from: s.out.At, until: w.ResetsAt, says: f.runOutSaid(s, now), resets: w.ResetsAt})
	}
	return causes
}

// runOutSaid says where the window standing as s runs out, as Runway says
// it at now: that it runs out, or reaches its account's reserve where that
// holds it back, and when, its window named as naming has it, as in "week
// runs out ~Fri 04:06".
func (f Frame) runOutSaid(s standing, now time.Time) string {
	verb := "runs out ~"
	if s.out.Reserve {
		verb = "reaches its reserve ~"
	}
	return f.naming(s.window) + verb + status.Dated(now, s.out.At)
}

// naming is how the words of what the window w holds an account back for
// lead: over the day, with the window's name, but for the window a request
// starts, as in "week "; over the week, without it, as it's the weeks' alone.
func (f Frame) naming(w quota.Window) string {
	if f.Span == Week || w.Key == f.Policy.Started {
		return ""
	}
	return status.InProse(w.Label) + " "
}

// merged are the stretches the causes hold an account back for, in order,
// the earliest first: those that meet or overlap run together, from the
// first's start until the last of their ends, each cause kept, in the order
// they start, but one telling the same as another, kept once.
func merged(causes []cause) []stretch {
	slices.SortStableFunc(causes, func(x, y cause) int { return x.from.Compare(y.from) })
	var stretches []stretch
	for _, c := range causes {
		n := len(stretches)
		if n == 0 || !stretches[n-1].until.IsZero() && c.from.After(stretches[n-1].until) {
			stretches = append(stretches, stretch{interval: c.interval, causes: []cause{c}})
			continue
		}
		last := &stretches[n-1]
		if slices.ContainsFunc(last.causes, c.same) {
			continue
		}
		last.causes = append(last.causes, c)
		if !last.until.IsZero() && (c.until.IsZero() || c.until.After(last.until)) {
			last.until = c.until
		}
	}
	return stretches
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
