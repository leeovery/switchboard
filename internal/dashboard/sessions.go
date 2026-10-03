package dashboard

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The calls' columns, from callsAt: a session's id, its model, when it was
// last seen, and what its request is doing, each with the blanks after it;
// then its cord's plug, at plugAt.
const (
	callsAt        = margin + 1
	seenColumn     = 5
	activityColumn = 8
	plugAt         = callsAt + idColumn + modelColumn + seenColumn + activityColumn
)

// The lines' panels, at the frame's right: linesWide cells wide, with their
// windows' bars, panelBar cells each, from barsFrom columns; under that,
// linesNarrow, with their windows' use alone. Under cordsFrom columns, the
// view is a plain list rather than a switchboard.
const (
	linesWide   = 64
	panelBar    = 24
	linesNarrow = linesWide - panelBar
	barsFrom    = 110
	cordsFrom   = 90
)

// The cords: a bent cord turns down bendsApart cells left of the one before
// at most, the first bendClear cells clear of a cord hanging loose from a
// jack, which is looseCells long, as a stub is stubCells at most.
const (
	bendsApart = 5
	bendClear  = 3
	looseCells = 9
	stubCells  = 7
)

// The view's labels, over its calls and its lines.
var (
	callsLabel = line{{"CALLS", labelInk}, {"  sessions, by the line they are on", faintInk}}
	linesLabel = line{{"LINES", labelInk}, {"  accounts", faintInk}}
)

var (
	// goneInk is a placeholder's, standing in for a call a move took off.
	goneInk   = ink{token: theme.TextFaint}
	goneIDInk = ink{token: theme.TextFaint, bold: true}
)

// bay is the Sessions view laid out as a switchboard, from the row under its
// labels, row 0: the calls down the left, a row per session's model, grouped
// by the account it's on, in the accounts' order, a blank row between
// groups; the lines down the right, a panel per account, from column x,
// width cells wide, drawing their windows' bars where bars is set; a cord
// from each call to a jack of its line's panel; and LOG under the calls,
// where it shows. It's content rows tall.
type bay struct {
	x, width int
	bars     bool
	calls    []callRow
	panels   []linePanel
	cords    []cord
	log      logBlock
	content  int
}

// callRow is a row of the calls, at row: a seat on its account, as the
// router listed it or a move brought it, plugged into the jack of its line's
// panel numbered jack; or, gone, the placeholder of one a move took off to
// the account to, which keeps its row and its jack until the router's
// sessions are listed again, so the other cords stay put.
type callRow struct {
	seated
	row, jack int
	gone      bool
	to        string
}

// sessions draws the Sessions view of doc at now from row top: as a
// switchboard, its labels over it, where there's a row under them, and from
// the row under them, scrolled where it doesn't fit, a scrollbar beside it;
// or, with one account, or under cordsFrom columns, or where its cords
// don't fit, a plain list of each account's panel with its sessions under
// it. Then the line over the footer, as overFooter has it.
func (f Frame) sessions(c *canvas, doc status.Document, now time.Time, top int) {
	if len(doc.Accounts) == 0 {
		return
	}
	b, ok := f.bayOf(doc, now, top)
	if !ok {
		f.plainList(c, doc, now, top)
		return
	}
	s := f.sightOf(b, top)
	if s.rows > 0 {
		c.line(callsAt, top, callsLabel)
		c.line(b.x, top, linesLabel)
	}
	area := newCanvas(f.Width, b.content)
	f.drawBay(area, doc, b, now)
	c.paste(area, s.offset, s.top, s.rows)
	f.overFooter(c, doc, now, top, b.content, f.sessionsView(top), s.offset, b.hidden)
}

// sightOf is how the bay b, laid out under its labels on row top, shows on
// screen: from the row under them, scrolled as far as the frame has it, as
// many of its rows as the view shows, under the help where it's open.
func (f Frame) sightOf(b bay, top int) sight {
	view := f.sessionsView(top)
	return sight{top: top + 1, offset: min(max(f.Scroll, 0), max(b.content-view, 0)), rows: min(view, b.content), help: f.helpCovers()}
}

// overFooter draws the line over the footer of the Sessions view, from row
// top, where it's under it: where the view's content, content rows tall,
// scrolls, view of them shown from the row offset, a scrollbar beside the
// view, and what the rows out of view hold, as hidden counts the accounts
// among them, where any is; else which windows hide from every panel, where
// any does; else a rule.
func (f Frame) overFooter(c *canvas, doc status.Document, now time.Time, top, content, view, offset int, hidden func(offset, view int) (int, int)) {
	over := f.Height - 2
	if over < top {
		return
	}
	if content > view {
		scrollbar(c, f.Width-1, over-view, view, content, offset)
	}
	above, below := hidden(offset, view)
	switch note := hiddenNote(doc, hiddenWindows(doc, now, f.Policy)); {
	case content > view && above+below > 0:
		says := outOfView(above, below).fit(f.edge() - margin)
		c.line((f.Width-says.width())/2, over, says)
	case len(note) > 0:
		c.right(f.edge(), over, note.fit(f.edge()-margin))
	default:
		c.text(0, over, rule(f.Width), borderInk)
	}
}

// sessionsView is how many rows the Sessions view shows at once under its
// labels, on row top: down to the line over the footer.
func (f Frame) sessionsView(top int) int {
	return max(f.Height-2-(top+1), 0)
}

// sessionsRows is how many rows the Sessions view of doc at now takes, under
// the heading, which ends at row top, and how many of them show at once: as
// a switchboard, under its labels, or as a plain list.
func (f Frame) sessionsRows(doc status.Document, now time.Time, top int) (content, view int) {
	if b, ok := f.bayOf(doc, now, top); ok {
		return b.content, f.sessionsView(top)
	}
	return f.layOutPlain(doc, now).content, max(f.Height-2-top, 0)
}

// travelling is what moves of what travels the cords of the Sessions
// switchboard of doc at now, on screen: a pulse running along a cord, or a
// cord a move let go fading, frame by frame; and the shimmer down a cord as
// its answer streams back, a step at a time. A call whose cord doesn't show,
// scrolled out of view or under the help, or that has none, as of a session
// the router hasn't listed, moves nothing, nor does the plain list.
func (f Frame) travelling(doc status.Document, now time.Time) Motion {
	top := f.top(doc)
	b, ok := f.bayOf(doc, now, top)
	if !ok {
		return Motion{}
	}
	s := f.sightOf(b, top)
	var m Motion
	for _, k := range b.cords {
		c, ok := f.Traffic.call(k.plug)
		if !ok || !c.Pulsing && c.Doing != Streaming || !k.shows(s, b.x) {
			continue
		}
		m.Smooth = m.Smooth || c.Pulsing
		m.Shimmer = m.Shimmer || c.Doing == Streaming
	}
	for _, l := range f.looseCords(doc, b, now) {
		if l.fades && !f.gone(l.fade) && s.shows(b.x-l.cells, l.y, l.cells, 1) {
			m.Smooth = true
		}
	}
	return m
}

// bayOf lays the Sessions view of doc at now out as a switchboard under its
// labels, on row top, reporting false where it's a plain list instead: with
// one account, every cord would end at the same jack; under cordsFrom
// columns there's no room for cords; and where its cords bend more than its
// columns hold. The lines are linesWide cells wide, with bars, from barsFrom
// columns, where the cords fit beside them, else linesNarrow; at the
// frame's right, but a cell short of it where the view scrolls, clear of the
// scrollbar.
func (f Frame) bayOf(doc status.Document, now time.Time, top int) (bay, bool) {
	if len(doc.Accounts) < 2 || f.Width < cordsFrom {
		return bay{}, false
	}
	if f.Width >= barsFrom {
		if b, ok := f.bayOfWidth(doc, now, top, linesWide, true); ok {
			return b, true
		}
	}
	return f.bayOfWidth(doc, now, top, linesNarrow, false)
}

// bayOfWidth lays the switchboard of doc at now out under its labels, on row
// top, its lines width cells wide, with bars where bars is set, as bayOf
// places them, reporting false where its cords don't fit.
func (f Frame) bayOfWidth(doc status.Document, now time.Time, top, width int, bars bool) (bay, bool) {
	view := f.sessionsView(top)
	b, ok := f.layOutBay(doc, now, f.Width-width, width, bars, view)
	if ok && b.content > view {
		b, ok = f.layOutBay(doc, now, f.Width-1-width, width, bars, view)
	}
	return b, ok
}

// layOutBay lays the switchboard of doc at now out, view rows of it shown at
// once, its lines from column x, width cells wide, with bars where bars is
// set: the calls, a group each account, from row 1, each a jack in turn of
// its line's panel; the panels in turn from row 0, a blank row between, each
// as tall as its windows or its calls, and moved down where its calls would
// start below its first jack, so every cord runs right, then down; the
// cords, bent as bend says; and LOG under the calls. It reports false where
// the cords' bends don't fit.
func (f Frame) layOutBay(doc status.Document, now time.Time, x, width int, bars bool, view int) (bay, bool) {
	b := bay{x: x, width: width, bars: bars}
	shown := shownWindows(doc, now, f.Policy)
	groups, firsts := f.groupsOf(doc), make([]int, len(doc.Accounts))
	row := 1
	for i, g := range groups {
		firsts[i] = -1
		if len(g) == 0 {
			continue
		}
		firsts[i] = row
		for j := range g {
			g[j].row, g[j].jack = row+j, j
		}
		row += len(g) + 1
	}
	top := 0
	for i, a := range doc.Accounts {
		if firsts[i] >= 0 {
			top = max(top, firsts[i]-1)
		}
		p := f.linePanelOf(doc, a, now, shown)
		p.top, p.rows = top, max(len(shown), len(groups[i]))
		b.panels = append(b.panels, p)
		top += p.rows + 3
	}
	for i, g := range groups {
		for _, r := range g {
			if !r.gone {
				b.cords = append(b.cords, cord{plug: r.plug(), place: i + 1, from: r.row, to: b.panels[i].top + 1 + r.jack, busy: f.lit(r.seated, now)})
			}
		}
		b.calls = append(b.calls, g...)
	}
	if !b.bend() {
		return b, false
	}
	b.content = b.bottom() + 1
	b.log = f.logBlockOf(doc, now, b, view)
	if b.log.lines > 0 {
		b.content = max(b.content, b.log.row+b.log.lines+1)
	}
	return b, true
}

// bottom is the bay's last row with anything on it: its last panel's bottom
// edge, or its last call, whichever is lower.
func (b bay) bottom() int {
	last := 0
	if n := len(b.panels); n > 0 {
		last = b.panels[n-1].bottom()
	}
	if n := len(b.calls); n > 0 {
		last = max(last, b.calls[n-1].row)
	}
	return last
}

// hidden counts the accounts whose panels the bay's view, view rows shown
// from the row offset, leaves out of view, wholly or partly: those above,
// and those below.
func (b bay) hidden(offset, view int) (above, below int) {
	for _, p := range b.panels {
		if p.top < offset {
			above++
		}
		if p.bottom() >= offset+view {
			below++
		}
	}
	return above, below
}

// groupsOf are the calls of doc's accounts, a group each, in their order:
// the seats the router listed on each, and those the request stream told of
// a move bringing on, in the order the calls keep, as ordered has them; each
// a move has taken off since a placeholder, until a listing of the router's
// sessions shows it done.
func (f Frame) groupsOf(doc status.Document) [][]callRow {
	repatching := f.Traffic.repatching()
	moves := latestMoves(repatching)
	groups := make([][]callRow, len(doc.Accounts))
	for i, a := range doc.Accounts {
		listed := seatedOn(f.Sessions, a.ID)
		var brought []seated
		for _, m := range repatching {
			if m.To == a.ID && moves[m.Seat] == m && !slices.ContainsFunc(listed, func(s seated) bool { return s.seat() == m.Seat }) {
				brought = append(brought, f.movedIn(m))
			}
		}
		for _, s := range f.Order.ordered(a.ID, listed, brought) {
			r := callRow{seated: s}
			if m, ok := moves[s.seat()]; ok && m.To != a.ID {
				r.gone, r.to = true, m.To
			}
			groups[i] = append(groups[i], r)
		}
	}
	return groups
}

// movedIn is the seat the move m brought onto the account it went to, put
// there as it moved: as the router listed it before it moved, or, where it
// didn't, as the move tells of it, its model named as the router names it
// for any session it lists.
func (f Frame) movedIn(m Move) seated {
	for _, s := range f.Sessions {
		if s.ID != m.Seat.Session {
			continue
		}
		for _, a := range s.Assignments {
			if a.Model == m.Seat.Model {
				a.Account, a.AssignedAt = m.To, m.At
				return seated{session: s, assignment: a}
			}
		}
	}
	a := status.Assignment{Model: m.Seat.Model, Family: f.familyOf(m.Seat.Model), Account: m.To, AssignedAt: m.At}
	return seated{session: status.Session{ID: m.Seat.Session, Assignments: []status.Assignment{a}}, assignment: a}
}

// familyOf is the family the router names the model with the given id by,
// as it lists any session of it: "" where it lists none.
func (f Frame) familyOf(model string) string {
	for _, s := range f.Sessions {
		for _, a := range s.Assignments {
			if a.Model == model && a.Family != "" {
				return a.Family
			}
		}
	}
	return ""
}

// latestMoves are the newest of the moves of each seat, by the seat.
func latestMoves(moves []Move) map[Seat]Move {
	latest := make(map[Seat]Move)
	for _, m := range moves {
		latest[m.Seat] = m
	}
	return latest
}

// Order is the order Sessions' calls run in, as the watch keeps it from look
// to look: each account's seats, by its id, in the order of their rows.
type Order map[string][]Seat

// OrderOf is the order the calls of doc's accounts run in as the frame
// draws them, for the watch to keep for the next look: each account's
// seats, a move's placeholders among them, which keep their rows until a
// listing shows their moves done.
func (f Frame) OrderOf(doc status.Document) Order {
	order := make(Order)
	for i, g := range f.groupsOf(doc) {
		id := doc.Accounts[i].ID
		for _, r := range g {
			order[id] = append(order[id], r.seat())
		}
	}
	return order
}

// ordered are the seats on the account with the given id in the order its
// calls run, so each keeps its row look to look, however the router's
// listing turns over: those the order has there first, in its order, closing
// up over those gone; then the rest at the foot, those listed as newestFirst
// has them, then those brought on by a move.
func (o Order) ordered(id string, listed, brought []seated) []seated {
	kept := o[id]
	row := func(s seated) int {
		if i := slices.Index(kept, s.seat()); i >= 0 {
			return i
		}
		return len(kept)
	}
	seats := slices.Concat(newestFirst(listed), brought)
	slices.SortStableFunc(seats, func(a, b seated) int { return cmp.Compare(row(a), row(b)) })
	return seats
}

// newestFirst are the seats on an account in the order they join its calls:
// the one put there last first, which gives the calls their order at the
// first look, and between those put there at once, as the router listed
// them, the one seen last first.
func newestFirst(seats []seated) []seated {
	seats = slices.Clone(seats)
	slices.SortStableFunc(seats, func(a, b seated) int { return b.assignment.AssignedAt.Compare(a.assignment.AssignedAt) })
	return seats
}

// drawBay draws the bay b of doc at now on c, from its top: the lines'
// panels, the cords hanging loose from their jacks, the cords, and over
// them what travels them; the calls; and LOG.
func (f Frame) drawBay(c *canvas, doc status.Document, b bay, now time.Time) {
	for _, p := range b.panels {
		f.drawPanel(c, doc, p, b.x, b.width, b.bars, now)
	}
	f.hang(c, doc, b, now)
	for _, k := range b.cords {
		f.drawCord(c, k, b.x)
	}
	for _, r := range b.calls {
		f.drawCall(c, doc, r, now)
	}
	if len(b.calls) == 0 {
		c.line(callsAt, 1, line{{noCalls(doc), dimInk}})
	}
	f.drawLog(c, doc, b.log, now)
}

// noCalls says why the switchboard has no calls: the router has routed no
// session in the last hour, or, probing, there's no router to say.
func noCalls(doc status.Document) string {
	if doc.Source == status.SourceRouter {
		return "no sessions in the last hour"
	}
	return "sessions show with the router"
}

// drawCall draws the call r of doc at now along its row: a placeholder's id,
// faint, and where the move took it, as in "d28c  ↪ moved to side", cut to
// the calls' columns; else its session's id, bright and bold while busy; its
// model; when it was last seen; and what its request is doing, as the
// request stream tells.
func (f Frame) drawCall(c *canvas, doc status.Document, r callRow, now time.Time) {
	id := sessionID(r.session.ID)
	if r.gone {
		c.line(callsAt, r.row, line{{id, goneIDInk}, {"  ↪ moved to " + named(doc, r.to), goneInk}}.fit(plugAt-callsAt))
		return
	}
	idInk := titleInk
	if !f.lit(r.seated, now) {
		idInk = idleIDInk
	}
	seen := seenAgo(now, f.lastSeen(r.seated))
	seenInk := dimInk
	if seen == "now" {
		seenInk = secondaryInk
	}
	model := truncate(r.assignment.Name(), modelColumn-2)
	l := line{{padded(id, idColumn), idInk}, {padded(model, modelColumn), mutedInk}, {padded(seen, seenColumn), seenInk}}
	if says, ok := f.activity(r.plug()); ok {
		l = append(l, says)
	}
	c.line(callsAt, r.row, l)
}

// activity says what the request of the seat plugged in at p is doing, as
// its call's row says it: gone out, "↑ ask", or on an account a move has just
// brought it to, "↪ new"; its answer streaming back, or just ended, and its
// tokens, as in "↓ ~1.2k"; refused, as in "✕ 429"; or throttled, dim, as in
// "… 429". It reports false while the request stream tells of none.
func (f Frame) activity(p Plug) (span, bool) {
	c, ok := f.Traffic.call(p)
	switch {
	case !ok:
		return span{}, false
	case c.Doing == Asking && c.New:
		return span{"↪ new", nameInk}, true
	case c.Doing == Asking:
		return span{"↑ ask", nameInk}, true
	case c.Doing == Streaming || c.Doing == Answered:
		return span{c.streamed(), titleInk}, true
	case c.Doing == Refused:
		return span{"✕ " + statusCode(c.Status), exhaustedInk}, true
	case c.Doing == Throttled:
		return span{"… " + statusCode(c.Status), dimInk}, true
	}
	return span{}, false
}

// statusCode is an answer's status, as in "429": "" for none.
func statusCode(code int) string {
	if code == 0 {
		return ""
	}
	return strconv.Itoa(code)
}

// seenAgo says how long before now a call was last seen, as its row has room
// for: "now" within the half minute, else to the nearest minute, as "9m",
// and from the hour, to the nearest hour, as "1h".
func seenAgo(now, seen time.Time) string {
	d := now.Sub(seen)
	switch minutes := int((d + time.Minute/2) / time.Minute); {
	case minutes < 1:
		return "now"
	case minutes < 60:
		return strconv.Itoa(minutes) + "m"
	default:
		return strconv.Itoa(int((d+time.Hour/2)/time.Hour)) + "h"
	}
}
