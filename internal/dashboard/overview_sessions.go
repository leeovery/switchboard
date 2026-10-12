package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The cells of a session's row in SESSIONS from routingRows columns, from the
// name column: its id and the blanks after it; its model's family, padded;
// when it was last seen, right-aligned; the blanks before what it's doing;
// what it's doing, padded, why it's there following; and the blanks why
// leaves, at the least, before its trace.
const (
	runningID     = sessionShown + 2
	familyCells   = 7
	seenCells     = 4
	beforeDoing   = 3
	doingCells    = 9
	runningWhy    = runningID + familyCells + seenCells + beforeDoing + doingCells
	beforeTraceAt = 2
)

// doingInk is what a session running is doing while it answers or asks.
var doingInk = ink{token: theme.AccentKey}

// sessionsSection is the Overview's SESSIONS of doc at now: the sessions
// running, a row on each account a session's models go to, or a row saying
// none is, at its least and its most alike; under routingRows columns, and
// on a phone, two rows a session.
func (f Frame) sessionsSection(doc status.Document, now time.Time) section {
	running := f.running(doc, now)
	rows := sectionHead + max(len(running)*f.runningForm().rows, 1)
	return section{
		Section: SectionSessions, least: rows, most: rows,
		draw: func(c *canvas, body bounds) {
			f.drawRunning(c, f.moment(doc, now), running, body)
		},
		folded: func() line {
			return line{{runningCount(f.moment(doc, now), running), mutedInk}}
		},
	}
}

// running are the sessions running on doc's accounts at now, as views lists
// them, of the router's listing as the request stream has moved and seen
// them since.
func (f Frame) running(doc status.Document, now time.Time) []views.Running {
	return views.RunningOf(f.moment(doc, now), f.seenListing())
}

// seenListing is the listing, each of its sessions' models last seen as
// lastSeen has it: as the router listed it, or later, as the request stream
// told of it.
func (f Frame) seenListing() []status.Session {
	listing := f.listing()
	if len(f.Traffic.Seen) == 0 {
		return listing
	}
	seen := make([]status.Session, len(listing))
	for i, s := range listing {
		assignments := slices.Clone(s.Assignments)
		for j, a := range assignments {
			assignments[j].LastSeen = f.lastSeen(seated{session: s, assignment: a})
		}
		s.Assignments = assignments
		seen[i] = s
	}
	return seen
}

// runningForm is how SESSIONS draws a session running: the rows it takes;
// the column its account's name starts at, on its account's first; and what
// draws the rest of it, of a moment, from a row, the name column given.
type runningForm struct {
	rows, nameAt int
	draw         func(c *canvas, m status.Moment, r views.Running, column, y int)
}

// runningForm is how the frame draws a session running: a row, from
// routingRows columns; else two, its account's name a cell in.
func (f Frame) runningForm() runningForm {
	if f.Width < routingRows {
		return runningForm{rows: 2, nameAt: 1, draw: f.narrowRunning}
	}
	return runningForm{rows: 1, draw: f.wideRunning}
}

// drawRunning draws the sessions running, of the moment m, in the body's
// bounds: as many as fit, the last row saying how many more there are where
// they don't all; or, with none running, a row saying so.
func (f Frame) drawRunning(c *canvas, m status.Moment, running []views.Running, body bounds) {
	if len(running) == 0 {
		c.line(0, body.y, line{{"no sessions running", faintInk}})
		return
	}
	form, shown := f.runningForm(), len(running)
	if shown*form.rows > body.rows {
		shown = max(body.rows-1, 0) / form.rows
	}
	names := overviewNames(m)
	column := nameColumn(names, f.Width)
	for i, r := range running[:shown] {
		y := body.y + i*form.rows
		if i == 0 || running[i-1].Account != r.Account {
			c.line(form.nameAt, y, names[place(m, r.Account)-1].line(column))
		}
		form.draw(c, m, r, column, y)
	}
	if more := len(running) - shown; more > 0 {
		c.line(column, body.y+shown*form.rows, line{{"+" + strconv.Itoa(more) + " more", dimInk}})
	}
}

// wideRunning draws the session running r on row y, of the moment m, from
// routingRows columns: from the name column, its id, its family, when it was
// last seen, what it's doing, and why it's there, cut to end before its
// trace; then its trace, and its rate right-aligned at the last column.
func (f Frame) wideRunning(c *canvas, m status.Moment, r views.Running, column, y int) {
	now := m.Now
	seen := runningSeen(r.LastSeen, now)
	doing := f.runningDoing(r)
	c.line(column, y, line{
		f.runningID(r, now),
		{padded(truncate(r.Family, familyCells), familyCells), mutedInk},
		{strings.Repeat(" ", max(seenCells-len(seen), 0)) + seen, dimInk},
		spaces(beforeDoing),
		{padded(doing.text, doingCells), doing.ink},
	})
	why := column + runningWhy
	c.line(why, y, line{{r.Why.String(), dimInk}}.fit(traceFrom-beforeTraceAt-why))
	faded, t := outOfPool(m, r.Account), f.Traces.Sessions[r.Session]
	f.drawTrace(c, t, now, bounds{x: traceFrom, y: y, width: f.wideTraceCells(), rows: 1}, faded)
	c.right(f.Width, y, line{traceRate(t, now, faded).numbered(Number{Kind: NumberRate, Account: r.Account, Of: r.Session})})
}

// narrowRunning draws the session running r on rows y and y + 1, of the
// moment m, under routingRows columns: on the first, its id at the name
// column and what it's doing right-aligned; on the second, why it's there,
// from the name column.
func (f Frame) narrowRunning(c *canvas, m status.Moment, r views.Running, column, y int) {
	c.line(column, y, line{f.runningID(r, m.Now)})
	c.right(f.Width, y, line{f.runningDoing(r)})
	c.line(column, y+1, line{{r.Why.String(), dimInk}}.fit(f.Width-column))
}

// runningID is the session running r's id, and the blanks after it: bold
// text.primary while it's busy, with a request in flight, as the router
// listed it or the request stream tells, or seen within busyWithin of now;
// else text.muted.
func (f Frame) runningID(r views.Running, now time.Time) span {
	id := span{padded(sessionID(r.Session), runningID), mutedInk}
	busy := r.InFlight != "" || now.Sub(r.LastSeen) < busyWithin
	for _, model := range r.Models {
		if c, ok := f.Traffic.call(runningPlug(r, model)); ok && c.Doing.InFlight() {
			busy = true
		}
	}
	if busy {
		id.ink = titleInk
	}
	return id
}

// runningSeen says how long before now a session running was last seen:
// "now" within the minute, else as seenAgo rounds it, as "9m".
func runningSeen(seen, now time.Time) string {
	if now.Sub(seen) < time.Minute {
		return "now"
	}
	return seenAgo(now, seen)
}

// runningDoing is what the session running r is doing, as the request
// stream tells of its models, where it tells of any; else as the router
// listed it: answering, with its tokens so far where the stream has counted
// any, as "↓ ~1.2k", or "↓" alone; asking, "↑ ask"; else idle.
func (f Frame) runningDoing(r views.Running) span {
	told, asking := false, false
	for _, model := range r.Models {
		c, ok := f.Traffic.call(runningPlug(r, model))
		if ok && c.Doing == Streaming {
			return span{answered(c), doingInk}
		}
		told, asking = told || ok, asking || ok && c.Doing.InFlight()
	}
	switch {
	case asking, !told && r.InFlight == status.Asking:
		return span{"↑ ask", doingInk}
	case !told && r.InFlight == status.Answering:
		return span{"↓", doingInk}
	}
	return span{"idle", faintInk}
}

// answered is how much of the call's answer has streamed, as "↓ ~1.2k": "↓"
// alone till the stream has counted any of it.
func answered(c Call) string {
	if c.Tokens == 0 {
		return "↓"
	}
	return c.streamed()
}

// runningPlug is where the session running r's model is plugged in.
func runningPlug(r views.Running, model string) Plug {
	return Plug{Account: r.Account, Seat: Seat{Session: r.Session, Model: model}}
}

// runningCount says how many sessions run, and how many on each of m's
// accounts, as SESSIONS' heading says it where it folds: "6 running: 2 on
// personal, 4 on side", a session whose models go to two accounts counted
// once, and on each; or "no sessions running".
func runningCount(m status.Moment, running []views.Running) string {
	sessions, byAccount := map[string]bool{}, map[string]int{}
	for _, r := range running {
		sessions[r.Session] = true
		byAccount[r.Account]++
	}
	if len(sessions) == 0 {
		return "no sessions running"
	}
	var on []string
	for _, a := range m.Accounts {
		if n := byAccount[a.ID]; n > 0 {
			on = append(on, strconv.Itoa(n)+" on "+name(a))
		}
	}
	return strconv.Itoa(len(sessions)) + " running: " + strings.Join(on, ", ")
}

// outOfPool reports whether the account of m with the given id is out of the
// pool, as its OutOfPool says: not where m lacks it.
func outOfPool(m status.Moment, id string) bool {
	a, _ := m.Account(id)
	return a.OutOfPool
}
