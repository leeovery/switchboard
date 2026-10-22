package dashboard

import (
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The cells a row of ROUTING's, an account's, or of SESSIONS', a session's,
// keeps between its parts: its words' end and what follows them, ● pinned,
// where it shows, or its sessions; ● pinned and the sessions; and the
// sessions and the trace.
const (
	wordsGap      = 2
	pinnedApart   = 3
	sessionsApart = 3
)

// The cells of the parts of a row of ROUTING's that flex, as routingParts
// lays them: the words' least and most, their most what they had at 173
// columns before they flexed, so those rows read as they did; and the
// trace's least.
const (
	wordsLeast = 20
	wordsMost  = 61
	traceLeast = 24
)

// narrowTail is how many cells short of the frame's edge an account's trace
// ends where it's two rows an account, its sessions and its rate after it.
const narrowTail = 20

// The parts of a row of ROUTING's, in routingParts' order: the name column,
// the words of how the account stands, its sessions, its trace, as its share
// beside the words and what's past it, and its rate's tail.
const (
	rowNames = iota
	rowWords
	rowSessions
	rowTrace
	rowTracePast
	rowTail
)

// routingParts are the parts of a row of ROUTING's across the frame, as flex
// lays them, the name column names cells: the name column, its sessions
// sessionsCells wide, wordsGap after the words, and its rate's traceTail,
// fixed; and the words and the trace, half of what's left each, the words
// up to wordsMost, the trace taking the rest. The words' natural is the
// trace's least, so the two stand level before they share, the odd cell the
// trace's; the trace's share grows up to wordsMost, as the words', and what's
// past it is the part after it, which grows with no most.
func routingParts(names int) []flex.Part {
	sessions := sessionsCells()
	return []flex.Part{
		rowNames:     {Least: names, Natural: names},
		rowWords:     {Least: wordsLeast, Natural: traceLeast, Most: wordsMost, Grows: true},
		rowSessions:  {Least: sessions, Natural: sessions, Gap: wordsGap},
		rowTrace:     {Least: traceLeast, Natural: traceLeast, Most: wordsMost, Grows: true, Odd: true, Gap: sessionsApart},
		rowTracePast: {Grows: true},
		rowTail:      {Least: traceTail, Natural: traceTail},
	}
}

// routingColumns are where a row of ROUTING's and of SESSIONS' lies across a
// frame: whether it's one row an account and a session; the name column's
// cells, its words starting after them; and, on one row, the words' cells
// and the column its sessions start at. Its trace starts at trace, traceCells
// long: on one row, after the sessions; on two, on the second, from the name
// column to narrowTail cells short of the edge.
type routingColumns struct {
	oneRow            bool
	names, words      int
	sessions          int
	trace, traceCells int
}

// routingColumns are where the rows of ROUTING and SESSIONS lie across the
// frame, of the moment m, as routingColumnsOf lays them, the name column as
// USAGE's rows have it.
func (f Frame) routingColumns(m status.Moment) routingColumns {
	return routingColumnsOf(f.Width, nameColumn(overviewNames(m), f.Width))
}

// routingColumnsOf are where the rows of ROUTING and SESSIONS lie across
// width columns, the name column names cells: one row an account and a
// session where routingParts fit, laid as they say; else two.
func routingColumnsOf(width, names int) routingColumns {
	s, oneRow := flex.Lay(width, routingParts(names)...)
	if !oneRow {
		return routingColumns{names: names, trace: names, traceCells: width - narrowTail - names}
	}
	return routingColumns{
		oneRow: true, names: names, words: s[rowWords].Size, sessions: s[rowSessions].At,
		trace: s[rowTrace].At, traceCells: s[rowTrace].Size + s[rowTracePast].Size,
	}
}

// sessionsCells are the cells an account's sessions take on its row, as
// sessionsSpan says a count of a digit.
func sessionsCells() int {
	return ansi.StringWidth(sessionsSpan(0).text)
}

// accountRows are the rows of ROUTING's body the account i of a takes, as
// drawRouting lays them out: its one row, or its two.
func (rc routingColumns) accountRows(i, a int, body bounds) []int {
	if rc.oneRow && !routingSized(a, body) {
		return []int{body.y + i}
	}
	return []int{body.y + 3*i, body.y + 3*i + 1}
}

// pinnedSpan marks an account the global pin names, in accent.key.
var pinnedSpan = span{"● pinned", ink{token: theme.AccentKey}}

// routingCaption says what the traces draw, under the last of them.
const routingCaption = "requests a minute, last 30 minutes"

// accountRouting is an account as ROUTING draws it: its id; its number, name
// and marks, as USAGE draws them; how it stands with the router; whether the
// global pin names it; how many sessions are on it; its trace; and whether
// it's out of the pool, which fades its trace and its rate.
type accountRouting struct {
	id       string
	name     line
	standing status.Routing
	pinned   bool
	sessions int
	trace    views.Trace
	faded    bool
}

// routingTable is ROUTING of a document at a moment: each account's, in the
// config's order; where its rows lie across the frame; the pool's accounts',
// where it has a pool, and their states counted; and the moment.
type routingTable struct {
	accounts []accountRouting
	pool     []accountRouting
	states   status.StateCounts
	columns  routingColumns
	now      time.Time
}

// routingTableOf is ROUTING of doc at now, as the frame draws it: each
// account's sessions counted as SESSIONS lists them running, as
// views.RunningCounts counts them, where the router listed any, else as doc
// counts them; and its pool, who's out of it, and its accounts' states
// counted, as status.CountStates counts them, at now.
func (f Frame) routingTableOf(doc status.Document, now time.Time) routingTable {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	t := routingTable{columns: f.routingColumns(m), now: now}
	var running map[string]int
	if f.Sessions != nil {
		running = views.RunningCounts(f.running(doc, now))
	}
	var pooled []status.AccountNow
	for i, a := range m.Accounts {
		r := accountRouting{
			id: a.ID, name: names[i].line(t.columns.names), standing: a.Routing, pinned: a.Pinned,
			sessions: a.Sessions, trace: f.Traces.Accounts[a.ID], faded: a.OutOfPool,
		}
		if running != nil {
			r.sessions = running[a.ID]
		}
		t.accounts = append(t.accounts, r)
		if a.InPool {
			t.pool, pooled = append(t.pool, r), append(pooled, a)
		}
	}
	t.states = status.CountStates(pooled)
	return t
}

// routingBesideUsage reports whether ROUTING is drawn beside USAGE's blocks
// rather than as a section of its own: by account, from matrixWidth columns,
// while both show, but where GRAPHS is beside them, as graphsBeside says.
func (f Frame) routingBesideUsage() bool {
	return f.By == ByAccount && f.Width >= matrixWidth && f.Sections.Has(SectionUsage) && f.Sections.Has(SectionRouting) && !f.graphsBeside()
}

// routingSection is the Overview's ROUTING of doc at now: how each account
// stands with the router, and its trace, its heading's description the mode.
// At its least, size 1, it's a row an account and its caption under them,
// a + 3 rows for a accounts; at its most, size 2, two rows of trace an
// account and a blank row, 3a + 2. Where its row's parts don't fit a row, as
// routingColumns says, it's two rows an account and a blank row, 3a + 1 at
// its least and its most. Beside USAGE's blocks, it's as routingAside has
// it. Folded, its heading says the mode alone. Its table is built only as
// it's first drawn.
func (f Frame) routingSection(doc status.Document, now time.Time) section {
	m := f.Moment(doc, now)
	if f.routingBesideUsage() {
		return routingAside(m)
	}
	a, columns := len(m.Accounts), f.routingColumns(m)
	least, most := a+3, 3*a+2
	if !columns.oneRow {
		least, most = 3*a+1, 3*a+1
	}
	t := sync.OnceValue(func() routingTable { return f.routingTableOf(doc, now) })
	return section{
		Section: SectionRouting, least: least, most: most,
		description: func() line { return routingMode(m, f.phone()) },
		folded:      func() line { return routingMode(m, true) },
		draw: func(c *canvas, body bounds) {
			f.drawRouting(c, t(), body)
		},
		targets: func(body bounds) []Target {
			var targets []Target
			for i, a := range m.Accounts {
				for _, y := range columns.accountRows(i, len(m.Accounts), body) {
					targets = append(targets, Target{Y: y, Width: f.Width, On: OnAccount, Account: a.ID})
				}
			}
			return targets
		},
	}
}

// routingAside is ROUTING of m beside USAGE's blocks: no rows of its own, and
// nothing drawn and no targets of its own, as USAGE's draw has routingBeside
// draw it where its blocks lie; its heading the mode alone, folded or not.
func routingAside(m status.Moment) section {
	mode := func() line { return routingMode(m, true) }
	return section{
		Section: SectionRouting, description: mode, folded: mode,
		draw:    func(*canvas, bounds) {},
		targets: func(bounds) []Target { return nil },
	}
}

// routingMode is the mode of m's router, as ROUTING's heading says it: auto,
// in bold text.secondary; or pinned to the accounts the global pin names, in
// bold accent.attention, since when, then p back to auto, or, where alone is
// set, as by account and on a phone, the mode alone. A document probed has
// none.
func routingMode(m status.Moment, alone bool) line {
	switch {
	case !m.Routed:
		return nil
	case m.Pin.IsZero():
		return line{{"auto", strongInk}}
	}
	mode := line{{"pinned to " + names(m, m.Pin.Accounts), alertInk}}
	if alone {
		return mode
	}
	if !m.Pin.Since.IsZero() {
		mode = append(mode, span{" since " + prose.Dated(m.Now, m.Pin.Since), dimInk})
	}
	return append(mode, spaces(3), span{"p", keyInk}, span{" back to auto", faintInk})
}

// drawRouting draws ROUTING's body in the bounds the planner gives it: a row
// an account, or two where it has rows for size 2, with its caption under
// the last; where its row's parts don't fit a row, two rows an account.
func (f Frame) drawRouting(c *canvas, t routingTable, body bounds) {
	if !t.columns.oneRow {
		for i, r := range t.accounts {
			f.narrowRouting(c, t, r, body.y+3*i)
		}
		return
	}
	rows, step := 1, 1
	if routingSized(len(t.accounts), body) {
		rows, step = 2, 3
	}
	for i, r := range t.accounts {
		f.wideRouting(c, t, r, body.y+step*i, rows)
	}
	c.line(t.columns.trace, body.y+step*(len(t.accounts)-1)+rows, line{{routingCaption, faintInk}}.fit(f.Width-t.columns.trace))
}

// routingSized reports whether ROUTING's body of a accounts on a row each
// has rows for size 2: two rows of trace an account and a blank row.
func routingSized(a int, body bounds) bool {
	return body.rows >= 3*a
}

// wideRouting draws account r's row of ROUTING from row y, on a row an
// account, its trace rows tall, as t's columns lay it: its number, name and
// marks; its state from the name column, in its words' cells as
// rowStanding words it, less ● pinned's and pinnedApart, where it shows,
// wordsGap before it; its sessions; and its trace, its rate right-aligned on
// its last row at the edge rateEdge gives it. ● pinned shows only where the
// words keep their least beside it: ROUTING's heading names the accounts
// pinned all the same.
func (f Frame) wideRouting(c *canvas, t routingTable, r accountRouting, y, rows int) {
	at := t.columns
	c.line(0, y, r.name)
	words := at.words
	beside := words - ansi.StringWidth(pinnedSpan.text) - pinnedApart
	if r.pinned && beside >= wordsLeast {
		words = beside
		c.line(at.names+words+wordsGap, y, line{pinnedSpan})
	}
	f.drawStanding(c, r, at.names, y, rowStanding(r.standing, words))
	c.line(at.sessions, y, line{r.sessionsCounted()})
	trace := bounds{x: at.trace, y: y, width: at.traceCells, rows: rows}
	f.drawTrace(c, r.trace, t.now, trace, r.faded)
	c.right(f.rateEdge(trace), y+rows-1, line{r.rate(t.now)})
}

// narrowRouting draws account r's two rows of ROUTING from row y, as t's
// columns lay them: on the first, its number, name and marks from column 1,
// its short state from the name column, cut by whole clauses, and ● pinned
// right-aligned, where it shows; on the second, its trace from the name
// column to narrowTail cells short of the edge, then its sessions and its
// rate right-aligned.
func (f Frame) narrowRouting(c *canvas, t routingTable, r accountRouting, y int) {
	c.line(1, y, r.name)
	end := f.Width
	if r.pinned {
		end = c.right(f.Width, y, line{pinnedSpan})
	}
	at := t.columns
	f.drawStanding(c, r, at.names, y, standingWords(r.standing, true, end-wordsGap-at.names))
	trace := bounds{x: at.trace, y: y + 1, width: at.traceCells, rows: 1}
	f.drawTrace(c, r.trace, t.now, trace, r.faded)
	c.right(f.rateEdge(trace), y+1, line{r.sessionsCounted(), spaces(2), r.rate(t.now)})
}

// drawStanding draws how account r stands from x along row y, in the words
// given, picked out while its state is newly changed, as cueStanding says.
func (f Frame) drawStanding(c *canvas, r accountRouting, x, y int, words line) {
	end := c.line(x, y, words)
	f.cueStanding(c, r.id, x, y, end-x)
}

// rowStanding are the words of how an account stands on a row of ROUTING's,
// in width cells: the first form that fits whole, its words, then its short
// form, so a time is never cut where a form keeps it; else its words cut by
// whole clauses, as standingWords cuts them.
func rowStanding(r status.Routing, width int) line {
	return firstFit(width, standingWords(r, false, math.MaxInt), standingWords(r, true, math.MaxInt), standingWords(r, false, width))
}

// standingWords are the words of how an account stands, or short, its short
// form, in width cells: its glyph and first words in its kind's colour, as
// routingInk has it, and each clause after them, a dot before it, in
// text.muted, as many of those as fit, cut whole from the end; where even
// the first words don't fit, cut short.
func standingWords(r status.Routing, short bool, width int) line {
	parts := standingParts(r, short)
	if len(parts) == 0 {
		return nil
	}
	words := standingLead(r.Kind, parts[0])
	for _, clause := range parts[1:] {
		more := append(slices.Clone(words), span{" · " + clause, mutedInk})
		if more.width() > width {
			break
		}
		words = more
	}
	return words.fit(width)
}

// standingParts are the clauses of how an account stands, or short, of its
// short form, first words first.
func standingParts(r status.Routing, short bool) []string {
	if short {
		return r.Short
	}
	return slices.Concat([]string{r.Says}, r.Clauses)
}

// standingLeads are the glyph and first words standingWords leads with, of
// how an account stands, or short, of its short form, whole: none where it
// says nothing.
func standingLeads(r status.Routing, short bool) line {
	if parts := standingParts(r, short); len(parts) > 0 {
		return standingLead(r.Kind, parts[0])
	}
	return nil
}

// standingLead is an account's glyph and first words, says, of its kind: an
// open account's glyph in state.positive and its words in text.secondary;
// any other's both in its kind's colour, as routingInk has it.
func standingLead(kind status.RoutingKind, says string) line {
	if kind == status.RoutingOpen || kind == status.RoutingNext {
		return line{{kind.Glyph(), positiveInk}, {" " + says, secondaryInk}}
	}
	return line{{kind.Glyph() + " " + says, routingInk(kind)}}
}

// routingInk is the colour an account's glyph and first words are drawn in,
// of its kind: what holds it back from every request, or that it can't be
// read, in state.destructive; what holds back some of its requests, its cap
// and its pressure in accent.attention; not read yet and idle in
// text.subtle; and open in text.secondary.
func routingInk(kind status.RoutingKind) ink {
	switch kind {
	case status.RoutingTokenless, status.RoutingLimited, status.RoutingRefused, status.RoutingUnreadable:
		return errorInk
	case status.RoutingModelLimited, status.RoutingModelRefused, status.RoutingCapped, status.RoutingCappedPinned, status.RoutingModelCapped, status.RoutingPressed:
		return warningInk
	case status.RoutingUnread, status.RoutingIdle:
		return dimInk
	default:
		return secondaryInk
	}
}

// cueStanding picks out width cells from x along row y, the words of how the
// account with the given id stands, while its state is newly changed, as far
// faded back as the frame's Changed says, as cue picks them out.
func (f Frame) cueStanding(c *canvas, id string, x, y, width int) {
	if fade, ok := f.Changed[id]; ok {
		cue(c, f.Look, x, y, width, fade)
	}
}

// cue picks out width cells from x along row y, words newly cued, as drawn
// in look: on bg.attention, as far faded back as fade; or, where the look
// has no colour, bold. It marks them cued, the page settling while they
// show.
func cue(c *canvas, look Look, x, y, width int, fade float64) {
	c.surface(x, y, width, hue{token: theme.BgAttention, fade: fade})
	c.cued(x, y, width)
	if !look.coloured {
		c.emboldened(x, y, width)
	}
}

// sessionsCounted are how many sessions are on the account, as sessionsSpan
// says it, numbered as its sessions.
func (r accountRouting) sessionsCounted() span {
	return sessionsSpan(r.sessions).numbered(Number{Kind: NumberSessions, Account: r.id})
}

// rate is the account's rate at now, as traceRate says it, numbered as its
// rate.
func (r accountRouting) rate(now time.Time) span {
	return traceRate(r.trace, now, r.faded).numbered(Number{Kind: NumberRate, Account: r.id})
}

// sessionsSpan is how many sessions are on an account, as prose.Counted
// counts them, as in "2 sessions" or "1 session", in text.muted; "0
// sessions" in text.faint.
func sessionsSpan(n int) span {
	k := mutedInk
	if n == 0 {
		k = faintInk
	}
	return span{prose.Counted(n, "session"), k}
}

// blocksBeside are where USAGE draws its blocks by account, for ROUTING or
// GRAPHS' charts to be drawn beside them: from column x, each block in view
// where USAGE placed it, an account's, or the pool's, of no id; the rows
// under each block's last that what's beside it takes, as under has them,
// but where cut is set, none from it on, the row ▼ and how many accounts are
// below sit on; and how GRAPHS' charts lie across there, in the block's
// form, as besideCharts has them.
type blocksBeside struct {
	x      int
	blocks []rowAt
	under  int
	cut    int
	across chartsAcross
}

// routingBeside draws ROUTING's table t beside USAGE's blocks by account, as
// at says they lie, from its column to the edge, three lines beside each,
// as besideRows places them: an account's state, its words cut by whole
// clauses to end wordsGap cells short of the edge; ● pinned, where the
// global pin names it, then its sessions; and a trace a row tall to
// traceTail cells short of the edge, its rate right-aligned. Beside the
// pool's block, its accounts' states counted, as countedStates says; their
// sessions summed; and their traces and rates summed.
func (f Frame) routingBeside(c *canvas, t routingTable, at blocksBeside) {
	for i, block := range at.blocks {
		rows := besideRows(block.rows, at.ruled(i))
		for i := range rows {
			rows[i] += block.y
		}
		if i := slices.IndexFunc(t.accounts, func(r accountRouting) bool { return r.id == block.id }); i >= 0 {
			f.besideAccount(c, t, t.accounts[i], at.x, rows)
			continue
		}
		f.besidePool(c, t, at.x, rows)
	}
}

// besideAccount draws account r's lines beside its block from column x, on
// the rows given, as routingBeside has them: its sessions and its trace only
// where they have their rows.
func (f Frame) besideAccount(c *canvas, t routingTable, r accountRouting, x int, rows []int) {
	drawLines(rows,
		func(y int) { f.drawStanding(c, r, x, y, standingWords(r.standing, false, f.Width-wordsGap-x)) },
		func(y int) { c.line(x, y, besideSessions(r)) },
		func(y int) {
			trace := f.besideTrace(x, y)
			f.drawTrace(c, r.trace, t.now, trace, r.faded)
			c.right(f.rateEdge(trace), y, line{r.rate(t.now)})
		})
}

// besidePool draws the pool's lines beside its block from column x, on the
// rows given, as routingBeside has them, each of the pool's accounts' rows
// of t: its sessions and its trace only where they have their rows.
func (f Frame) besidePool(c *canvas, t routingTable, x int, rows []int) {
	drawLines(rows,
		func(y int) { c.line(x, y, countedStates(t.states, NumberStates, f.Width-wordsGap-x)) },
		func(y int) {
			c.line(x, y, line{sessionsSpan(summedSessions(t.pool)).numbered(Number{Kind: NumberSessions})})
		},
		func(y int) {
			trace := f.besideTrace(x, y)
			pool := summed(tracesOf(t.pool), func(tr views.Trace) []float64 { return tr.Cells(trace.width, t.now) })
			f.drawCells(c, pool, trace, false)
			c.right(f.rateEdge(trace), y, line{rateOf(summedRate(t.pool, t.now), false).numbered(Number{Kind: NumberRate})})
		})
}

// drawLines draws each line on its row of those given, in their order, as
// many as there are rows.
func drawLines(rows []int, lines ...func(y int)) {
	for i, y := range rows {
		lines[i](y)
	}
}

// tracesOf are the accounts' traces, in their order.
func tracesOf(accounts []accountRouting) []views.Trace {
	traces := make([]views.Trace, len(accounts))
	for i, r := range accounts {
		traces[i] = r.trace
	}
	return traces
}

// besideTrace is where a trace beside a block lies along row y, from column
// x: a row tall, to traceTail cells short of the edge.
func (f Frame) besideTrace(x, y int) bounds {
	return bounds{x: x, y: y, width: f.Width - traceTail - x, rows: 1}
}

// ruled reports whether the row after the block at i is the pool's rule, the
// pool's block coming straight after it.
func (at blocksBeside) ruled(i int) bool {
	if i+1 == len(at.blocks) {
		return false
	}
	block, next := at.blocks[i], at.blocks[i+1]
	return next.id == "" && next.y == block.y+block.rows+1
}

// besideParts are ROUTING's three lines beside a block, down its rows, as
// flex lays them: its state, its sessions and its trace, a row each, a blank
// row between each two that gives way, the two together, where the block's
// rows don't hold them.
var besideParts = []flex.Part{
	{Least: 1, Natural: 1},
	{Least: 1, Natural: 1, Leaves: 1},
	{Least: 1, Natural: 1},
	{Least: 1, Natural: 1, Leaves: 1},
	{Least: 1, Natural: 1},
}

// besideRows are the rows, down from a block's first, that ROUTING's three
// lines beside a block rows tall take, as besideParts lays them in its rows:
// two rows apart where it has the rows, as three windows give it, else one
// apart; any past the row after the block is left off, as it would be the
// next block's, and any on that row too where it's ruled, the pool's rule.
func besideRows(rows int, ruled bool) []int {
	s, _ := flex.Lay(rows, besideParts...)
	lines := []int{s[0].At, s[2].At, s[4].At}
	last := rows
	if ruled {
		last--
	}
	return slices.DeleteFunc(lines, func(down int) bool { return down > last })
}

// besideSessions is account r's sessions beside its block: after ● pinned
// and a dot, where the global pin names it.
func besideSessions(r accountRouting) line {
	if !r.pinned {
		return line{r.sessionsCounted()}
	}
	return line{pinnedSpan, {"  ·  ", faintInk}, r.sessionsCounted()}
}

// countedStates are accounts' states counted, as in "2 open · 1 at cap", in
// their order, each count in its state's colour, as stateInk has it, a dot in
// text.faint between them; where that's wider than width cells, how many are
// open, as OpenOf has it, as in "8 of 10 open", in text.secondary. Each is
// numbered as the kind given: the pool's states, or every account's.
func countedStates(counts status.StateCounts, kind NumberKind, width int) line {
	var counted line
	for _, c := range counts {
		counted = joined(span{" · ", faintInk}, counted, line{statesCounted(c.N, c.State.Word(), stateInk(c.State), kind)})
	}
	if counted.width() <= width {
		return counted
	}
	open, of := counts.OpenOf()
	return line{statesCounted(open, fmt.Sprintf("of %d open", of), secondaryInk, kind)}.fit(width)
}

// stateInk is the colour an account's state in a word is counted in: open in
// text.secondary; pressure and at cap in accent.attention; limit and refused
// in state.destructive; and not read in text.subtle.
func stateInk(s status.AccountState) ink {
	switch s {
	case status.StatePressure, status.StateAtCap:
		return warningInk
	case status.StateLimit, status.StateRefused:
		return errorInk
	case status.StateNotRead:
		return dimInk
	}
	return secondaryInk
}

// statesCounted is n accounts counted in the words given, in k, as in "2
// open", numbered as the kind given of states counted in those words.
func statesCounted(n int, words string, k ink, kind NumberKind) span {
	return span{fmt.Sprintf("%d %s", n, words), k}.numbered(Number{Kind: kind, Of: words})
}

// summedSessions are the sessions on the accounts, summed.
func summedSessions(accounts []accountRouting) int {
	n := 0
	for _, r := range accounts {
		n += r.sessions
	}
	return n
}

// summedRate is the accounts' rates at now, summed.
func summedRate(accounts []accountRouting, now time.Time) int {
	n := 0
	for _, r := range accounts {
		n += r.trace.Rate(now)
	}
	return n
}
