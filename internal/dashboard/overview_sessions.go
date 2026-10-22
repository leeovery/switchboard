package dashboard

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The cells of a session's row in SESSIONS on a row a session, as
// runningParts lays them from the name column: its id and the blanks after
// it; the blank after its model's family; when it was last seen,
// right-aligned; the blanks before what it's doing; what it's doing,
// padded, why it's there following; and the blanks why leaves, at the
// least, before its trace.
const (
	runningID     = status.IDShown + 2
	familyGap     = 1
	seenCells     = 4
	beforeDoing   = 3
	doingCells    = 9
	beforeTraceAt = 2
)

// The parts of a session's row in SESSIONS on a row a session, in
// runningParts' order: its id, its model's family, when it was last seen,
// what it's doing, and why it's there.
const (
	runID = iota
	runFamily
	runSeen
	runDoing
	runWhy
)

// doingInk is what a session running is doing while it answers or asks.
var doingInk = ink{token: theme.AccentKey}

// sessionsSection is the Overview's SESSIONS of doc at now: the sessions
// running, a row on each account a session's models go to, busiest first,
// as views.BusiestFirst orders them, or a row saying none is; two rows a
// session where ROUTING's row's parts don't fit a row, as routingColumns
// says. Its rows are as sessionsRows has them, from one session and how
// many more to every one; a layout kept lists them in the order it was made
// with, as views.Steady keeps it. A click on a session's row opens its
// page. Its heading says loading…, in text.subtle, while the frame says so,
// as what names its sessions is read.
func (f Frame) sessionsSection(doc status.Document, now time.Time) section {
	m := f.Moment(doc, now)
	running := views.BusiestFirst(m, f.running(doc, now), f.Traces)
	form := f.runningForm(f.routingColumns(m))
	base := section{
		Section: SectionSessions,
		description: func() line {
			if !f.Loading {
				return nil
			}
			return line{{loadingWord, dimInk}}
		},
		folded: func() line {
			return line{{runningCount(m, running), mutedInk}}
		},
		order: views.RowsOf(running),
	}
	base.least, base.most, base.between = sessionsRows(len(running), form.rows)
	listing := func(listed []views.Running) section {
		s := base
		s.draw = func(c *canvas, body bounds) { f.drawRunning(c, m, form, listed, body) }
		s.targets = func(body bounds) []Target { return form.targets(listed, body) }
		return s
	}
	s := listing(running)
	s.steady = func(kept []views.RunningRow) section { return listing(views.Steady(kept, running)) }
	return s
}

// sessionsRows are the rows SESSIONS takes of n sessions running, a session
// taking rows rows, its heading's among them: at its least, one session and
// the row saying how many more, or every one where that's no fewer; between,
// a session more at a time, and that row; at its most, every one, or the
// row saying none runs.
func sessionsRows(n, rows int) (least, most int, between []int) {
	most = sectionHead + max(n*rows, 1)
	least = min(sectionHead+rows+1, most)
	for k := 2; sectionHead+k*rows+1 < most; k++ {
		between = append(between, sectionHead+k*rows+1)
	}
	return least, most, between
}

// running are the sessions running on doc's accounts at now, as views lists
// them, of the router's listing as the request stream has moved and seen
// them since, as listLive has them, each named as the List held names it: as
// holdRunning holds them, where the frame holds them of doc at now, else
// worked out afresh.
func (f Frame) running(doc status.Document, now time.Time) []views.Running {
	if f.runs != nil && f.runs.at.Of(doc, now, f.Policy) {
		return f.runs.list()
	}
	return views.RunningOf(f.Moment(doc, now), f.listLive(now), f.Held)
}

// listLive is what the views list the sessions running of at now: the
// router's listing the frame holds, its last while it doesn't answer, as
// Unlisted says, and what the request stream told.
func (f Frame) listLive(now time.Time) views.ListLive {
	return views.ListLive{Running: f.Sessions, Unlisted: f.Unlisted, Stream: f.Stream, Now: now}
}

// runningHeld are the sessions running of a document at a moment, as
// holdRunning holds them: listed once, the first time a section asks.
type runningHeld struct {
	at   *status.HeldMoment
	list func() []views.Running
}

// holdRunning is the frame holding the sessions running on doc's accounts at
// now, as running lists them, listed once for SESSIONS to list and ROUTING to
// count, so the stream is heard, and the sessions placed, once a frame, and
// the two never disagree. It holds the moment of doc at now, as At does.
func (f Frame) holdRunning(doc status.Document, now time.Time) Frame {
	f = f.At(doc, now)
	of := f
	f.runs = &runningHeld{at: f.held, list: sync.OnceValue(func() []views.Running { return of.running(doc, now) })}
	return f
}

// runningForm is how SESSIONS draws a session running: the rows it takes;
// the column its account's name starts at, on its account's first; and
// where its row lies across the frame, as ROUTING's does.
type runningForm struct {
	rows, nameAt int
	columns      routingColumns
}

// runningForm is how the frame draws a session running, its row lying as
// the columns given say, as ROUTING's does: a row, where they're one row;
// else two, its account's name a cell in.
func (f Frame) runningForm(columns routingColumns) runningForm {
	if !columns.oneRow {
		return runningForm{rows: 2, nameAt: 1, columns: columns}
	}
	return runningForm{rows: 1, columns: columns}
}

// drawSession draws the session running r from row y, of the moment m, but
// its account's name, its parts lying as the columns given lay them: on a
// row, as wideRunning draws it, where they're one row; else on two, as
// narrowRunning does.
func (f Frame) drawSession(c *canvas, m status.Moment, r views.Running, at runningColumns, y int) {
	if at.oneRow {
		f.wideRunning(c, m, r, at, y)
		return
	}
	f.narrowRunning(c, m, r, at, y)
}

// runningColumns are where a session's row lies across the frame: as
// ROUTING's does; and on a row a session, as runningParts lays its parts,
// its family's cells, and the column why it's there starts at, and its
// cells.
type runningColumns struct {
	routingColumns
	family        int
	why, whyCells int
}

// laid is where a session's row lies in the form, as runningColumns has it:
// on a row a session, its parts laid by runningParts from the name column
// to beforeTraceAt cells short of the trace.
func (form runningForm) laid() runningColumns {
	at := runningColumns{routingColumns: form.columns}
	if !at.oneRow {
		return at
	}
	s, _ := flex.Lay(at.trace-beforeTraceAt-at.names, runningParts()...)
	at.family, at.why, at.whyCells = s[runFamily].Size, at.names+s[runWhy].At, s[runWhy].Size
	return at
}

// runningParts are the parts of a session's row on a row a session, as flex
// lays them: the id, runningID; the family, fixed at the widest family the
// version table names, as familiesCells has it, so the columns never move
// with the sessions shown, and a model's id that names no family is cut
// there; familyGap on, when it was last seen, seenCells; beforeDoing on,
// what it's doing, doingCells; and why it's there, which takes the rest.
func runningParts() []flex.Part {
	family := familiesCells()
	return []flex.Part{
		runID:     {Least: runningID, Natural: runningID},
		runFamily: {Least: family, Natural: family},
		runSeen:   {Least: seenCells, Natural: seenCells, Gap: familyGap},
		runDoing:  {Least: doingCells, Natural: doingCells, Gap: beforeDoing},
		runWhy:    {Grows: true},
	}
}

// familiesCells are the cells of the widest of the families the version
// table names.
func familiesCells() int {
	widest := 0
	for _, family := range ledger.Families() {
		widest = max(widest, ansi.StringWidth(family))
	}
	return widest
}

// drawRunning draws the sessions listed, of the moment m, in the form, in
// the body's bounds: as many as fit, a blank row for each the list leaves
// blank, each account's name on the first of its sessions drawn, and the
// last row saying how many more run where they don't all fit; or, with none
// running, a row saying so.
func (f Frame) drawRunning(c *canvas, m status.Moment, form runningForm, listed []views.Running, body bounds) {
	if runningIn(listed) == 0 {
		c.line(0, body.y, line{{noSessionsRunning, faintInk}})
		return
	}
	shown := form.shown(len(listed), body.rows)
	at := form.laid()
	names, named := overviewNames(m), ""
	for i, r := range listed[:shown] {
		if r.Blank() {
			continue
		}
		y := body.y + i*form.rows
		if r.Account != named {
			c.line(form.nameAt, y, names[place(m, r.Account)-1].line(at.names))
			named = r.Account
		}
		f.drawSession(c, m, r, at, y)
	}
	if more := runningIn(listed[shown:]); more > 0 {
		c.line(at.names, body.y+shown*form.rows, outOfView(0, more, body.x+body.width-at.names, runningOut))
	}
}

// runningIn counts the sessions listed, but the rows left blank.
func runningIn(listed []views.Running) int {
	n := 0
	for _, r := range listed {
		if !r.Blank() {
			n++
		}
	}
	return n
}

// shown is how many of n sessions listed SESSIONS shows, drawn in the form
// given, in rows rows: every one where they fit, else as many as leave the
// last row to say how many more there are.
func (form runningForm) shown(n, rows int) int {
	if n*form.rows > rows {
		return max(rows-1, 0) / form.rows
	}
	return n
}

// targets are SESSIONS' click targets, the sessions listed drawn in the
// body's bounds in the form, as drawRunning draws them: every row of each
// session shown, across the body, which opens its page; none on a row left
// blank, nor on the row saying how many more there are.
func (form runningForm) targets(listed []views.Running, body bounds) []Target {
	var targets []Target
	for i, r := range listed[:form.shown(len(listed), body.rows)] {
		if r.Blank() {
			continue
		}
		for row := range form.rows {
			targets = append(targets, Target{X: body.x, Y: body.y + i*form.rows + row, Width: body.width, On: OnSession, Session: r.Session})
		}
	}
	return targets
}

// runningOut is the form of the line saying how many more sessions are
// running than SESSIONS shows, ending it, as in "+2 more".
var runningOut = outForm{lead: "+", inks: subtleOut}

// wideRunning draws the session running r on row y, of the moment m, on a
// row a session, as the columns given lay it: from the name column, its id,
// its family, cut to its column, when it was last seen, what it's doing,
// and why it's there, cut to its column, which ends beforeTraceAt cells
// before its trace, which starts where ROUTING's does; then its trace, and
// its rate right-aligned at the edge rateEdge gives it.
func (f Frame) wideRunning(c *canvas, m status.Moment, r views.Running, at runningColumns, y int) {
	now := m.Now
	seen := runningSeen(r.Activity.LastSeen, now)
	doing := runningDoing(r.Activity)
	c.line(at.names, y, line{
		runningIDOf(r, now),
		{padded(prose.Cut(r.Family, at.family), at.family+familyGap), mutedInk},
		{strings.Repeat(" ", max(seenCells-len(seen), 0)) + seen, dimInk},
		spaces(beforeDoing),
		{padded(doing.text, doingCells), doing.ink},
	})
	c.line(at.why, y, line{{r.Why.String(), dimInk}}.fit(at.whyCells))
	faded, t := outOfPool(m, r.Account), f.Traces.Sessions[r.Session]
	trace := bounds{x: at.trace, y: y, width: at.traceCells, rows: 1}
	f.drawTrace(c, t, now, trace, faded)
	c.right(f.rateEdge(trace), y, line{traceRate(t, now, faded).numbered(Number{Kind: NumberRate, Account: r.Account, Of: r.Session})})
}

// narrowRunning draws the session running r on rows y and y + 1, of the
// moment m, on two rows a session, as the columns given lay it: on the
// first, its id at the name column and what it's doing right-aligned; on
// the second, why it's there, from the name column.
func (f Frame) narrowRunning(c *canvas, m status.Moment, r views.Running, at runningColumns, y int) {
	column := at.names
	c.line(column, y, line{runningIDOf(r, m.Now)})
	c.right(f.Width, y, line{runningDoing(r.Activity)})
	c.line(column, y+1, line{{r.Why.String(), dimInk}}.fit(f.Width-column))
}

// runningIDOf is the session running r's id, and the blanks after it: bold
// text.primary while it's busy at now, as its Activity's Busy says, with a
// request in flight or seen in the last minute; else text.muted.
func runningIDOf(r views.Running, now time.Time) span {
	id := span{padded(status.ShownID(r.Session), runningID), mutedInk}
	if r.Activity.Busy(now) {
		id.ink = titleInk
	}
	return id
}

// runningSeen says how long before now a session running was last seen:
// "now" within the minute, else as prose.SeenAgo rounds it, as "9m".
func runningSeen(seen, now time.Time) string {
	if now.Sub(seen) < time.Minute {
		return "now"
	}
	return prose.SeenAgo(now, seen)
}

// runningDoing is what a session running is doing, as SESSIONS says its
// Activity, in the views' short words: in accent.key while it's under way,
// as Underway says, else faint, refused as well, as it reads idle; and as
// last known as asKnown inks it.
func runningDoing(a views.Activity) span {
	k := faintInk
	if a.Underway() {
		k = doingInk
	}
	return span{a.Short(), asKnown(a, k)}
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
		return noSessionsRunning
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
