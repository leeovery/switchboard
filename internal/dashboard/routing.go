package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// routingRows is the fewest columns ROUTING gives each account a row at:
// narrower, and on a phone, it gives it two.
const routingRows = 110

// The columns a row of ROUTING's from routingRows columns puts what follows
// an account's state at: ● pinned, on an account the global pin names, and
// its sessions. Its state's words end wordsGap cells short of what follows
// them.
const (
	pinnedAt   = 68
	sessionsAt = 79
	wordsGap   = 2
)

// narrowTail is how many cells short of the frame's edge an account's trace
// ends under routingRows columns, its sessions and its rate after it.
const narrowTail = 20

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
// config's order; the name column their states start at; the pool's
// accounts', where it has a pool; and the moment.
type routingTable struct {
	accounts []accountRouting
	pool     []accountRouting
	names    int
	now      time.Time
}

// routingTableOf is ROUTING of doc at now, as the frame draws it: each
// account's sessions counted as the router listed them, where it listed any,
// else as doc counts them; and its pool, and who's out of it, at now.
func (f Frame) routingTableOf(doc status.Document, now time.Time) routingTable {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	t := routingTable{names: nameColumn(names, f.Width), now: now}
	var listing []status.Session
	if f.Sessions != nil {
		listing = f.listing()
	}
	for i, a := range m.Accounts {
		r := accountRouting{
			id: a.ID, name: names[i].line(t.names), standing: a.Routing, pinned: a.Pinned,
			sessions: a.Sessions, trace: f.Traces.Accounts[a.ID], faded: a.OutOfPool,
		}
		if listing != nil {
			r.sessions = len(f.busy(listing, a.ID, now))
		}
		t.accounts = append(t.accounts, r)
		if a.InPool {
			t.pool = append(t.pool, r)
		}
	}
	return t
}

// routingBesideUsage reports whether ROUTING is drawn beside USAGE's blocks
// rather than as a section of its own: by account, from matrixWidth columns,
// while both show.
func (f Frame) routingBesideUsage() bool {
	return f.By == ByAccount && f.Width >= matrixWidth && f.Sections.Has(SectionUsage) && f.Sections.Has(SectionRouting)
}

// routingSection is the Overview's ROUTING of doc at now: how each account
// stands with the router, and its trace, its heading's description the mode.
// At its least, size 1, it's a row an account and its caption under them,
// a + 3 rows for a accounts; at its most, size 2, two rows of trace an
// account and a blank row, 3a + 2. Under routingRows columns, and on a
// phone, it's two rows an account and a blank row, 3a + 1 at its least and
// its most. Beside USAGE's blocks, it's as routingAside has it. Folded, its
// heading says the mode alone. Its table is built only as it's first drawn.
func (f Frame) routingSection(doc status.Document, now time.Time) section {
	m := f.Moment(doc, now)
	if f.routingBesideUsage() {
		return routingAside(m)
	}
	a := len(m.Accounts)
	least, most := a+3, 3*a+2
	if f.Width < routingRows {
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
				for _, y := range f.accountRows(i, len(m.Accounts), body) {
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
		mode = append(mode, span{" since " + status.Dated(m.Now, m.Pin.Since), dimInk})
	}
	return append(mode, spaces(3), span{"p", keyInk}, span{" back to auto", faintInk})
}

// drawRouting draws ROUTING's body in the bounds the planner gives it: a row
// an account, or two where it has rows for size 2, with its caption under
// the last; under routingRows columns, two rows an account.
func (f Frame) drawRouting(c *canvas, t routingTable, body bounds) {
	if f.Width < routingRows {
		for i, r := range t.accounts {
			f.narrowRouting(c, t, r, body.y+3*i)
		}
		return
	}
	rows, step := 1, 1
	if f.routingSized(len(t.accounts), body) {
		rows, step = 2, 3
	}
	for i, r := range t.accounts {
		f.wideRouting(c, t, r, body.y+step*i, rows)
	}
	c.line(traceFrom, body.y+step*(len(t.accounts)-1)+rows, line{{routingCaption, faintInk}}.fit(f.Width-traceFrom))
}

// routingSized reports whether ROUTING's body of a accounts has rows for
// size 2, from routingRows columns: two rows of trace an account and a blank
// row.
func (f Frame) routingSized(a int, body bounds) bool {
	return body.rows >= 3*a
}

// accountRows are the rows of ROUTING's body the account i of a takes, as
// drawRouting lays them out: its one row, or its two.
func (f Frame) accountRows(i, a int, body bounds) []int {
	if f.Width >= routingRows && !f.routingSized(a, body) {
		return []int{body.y + i}
	}
	return []int{body.y + 3*i, body.y + 3*i + 1}
}

// wideRouting draws account r's row of ROUTING from routingRows columns from
// row y, its trace rows tall: its number, name and marks; its state from the
// name column, its words cut by whole clauses to end wordsGap cells short of
// ● pinned, where it shows, else of its sessions; its sessions; and its
// trace, its rate right-aligned at the last column on its last row.
func (f Frame) wideRouting(c *canvas, t routingTable, r accountRouting, y, rows int) {
	c.line(0, y, r.name)
	end := sessionsAt
	if r.pinned {
		c.line(pinnedAt, y, line{pinnedSpan})
		end = pinnedAt
	}
	f.drawStanding(c, r, t.names, y, end-wordsGap-t.names, false)
	c.line(sessionsAt, y, line{r.sessionsCounted()})
	f.drawTrace(c, r.trace, t.now, bounds{x: traceFrom, y: y, width: f.accountTraceCells(t.names), rows: rows}, r.faded)
	c.right(f.Width, y+rows-1, line{r.rate(t.now)})
}

// narrowRouting draws account r's two rows of ROUTING under routingRows
// columns from row y: on the first, its number, name and marks from column 1,
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
	f.drawStanding(c, r, t.names, y, end-wordsGap-t.names, true)
	f.drawTrace(c, r.trace, t.now, bounds{x: t.names, y: y + 1, width: f.accountTraceCells(t.names), rows: 1}, r.faded)
	c.right(f.Width, y+1, line{r.sessionsCounted(), spaces(2), r.rate(t.now)})
}

// drawStanding draws how account r stands from x along row y, its words, or
// short, its short form, cut by whole clauses to width cells, as standingWords
// cuts them, picked out while its state is newly changed, as cueStanding
// says.
func (f Frame) drawStanding(c *canvas, r accountRouting, x, y, width int, short bool) {
	end := c.line(x, y, standingWords(r.standing, short, width))
	f.cueStanding(c, r.id, x, y, end-x)
}

// standingWords are the words of how an account stands, or short, its short
// form, in width cells: its glyph and first words in its kind's colour, as
// routingInk has it, and each clause after them, a dot before it, in
// text.muted, as many of those as fit, cut whole from the end; where even
// the first words don't fit, cut short.
func standingWords(r status.Routing, short bool, width int) line {
	parts := slices.Concat([]string{r.Says}, r.Clauses)
	if short {
		parts = r.Short
	}
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
	if look.coloured {
		return
	}
	for i := range width {
		if here := c.at(x+i, y); here != nil {
			here.ink.bold = true
		}
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

// sessionsSpan is how many sessions are on an account, as in "2 sessions"
// or "1 session", in text.muted; "0 sessions" in text.faint.
func sessionsSpan(n int) span {
	switch n {
	case 0:
		return span{"0 sessions", faintInk}
	case 1:
		return span{"1 session", mutedInk}
	}
	return span{strconv.Itoa(n) + " sessions", mutedInk}
}

// blocksBeside are where USAGE draws its blocks by account, for ROUTING to be
// drawn beside them: from column x, each block in view where USAGE placed it,
// an account's, or the pool's, of no id.
type blocksBeside struct {
	x      int
	blocks []rowAt
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
	for _, block := range at.blocks {
		rows := besideRows(block.rows)
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
// the rows given, as routingBeside has them: its trace only where it has its
// row.
func (f Frame) besideAccount(c *canvas, t routingTable, r accountRouting, x int, rows []int) {
	f.drawStanding(c, r, x, rows[0], f.Width-wordsGap-x, false)
	c.line(x, rows[1], besideSessions(r))
	if len(rows) < 3 {
		return
	}
	f.drawTrace(c, r.trace, t.now, f.besideTrace(x, rows[2]), r.faded)
	c.right(f.Width, rows[2], line{r.rate(t.now)})
}

// besidePool draws the pool's lines beside its block from column x, on the
// rows given, as routingBeside has them, each of the pool's accounts' rows
// of t: its trace only where it has its row.
func (f Frame) besidePool(c *canvas, t routingTable, x int, rows []int) {
	c.line(x, rows[0], countedStates(t.pool, f.Width-wordsGap-x))
	c.line(x, rows[1], line{sessionsSpan(summedSessions(t.pool)).numbered(Number{Kind: NumberSessions})})
	if len(rows) < 3 {
		return
	}
	trace := f.besideTrace(x, rows[2])
	pool := summed(tracesOf(t.pool), func(tr views.Trace) []float64 { return tr.Cells(trace.width, t.now) })
	f.drawCells(c, pool, trace, false)
	c.right(f.Width, rows[2], line{rateOf(summedRate(t.pool, t.now), false).numbered(Number{Kind: NumberRate})})
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

// besideRows are the rows, down from a block's first, that ROUTING's three
// lines beside a block rows tall take: its state, its sessions and its
// trace, two rows apart where it has the rows, as three windows give it,
// else one apart; any past the row after the block is left off, as it
// would be the next block's.
func besideRows(rows int) []int {
	lines := []int{0, 1, 2}
	if rows >= 5 {
		lines = []int{0, 2, 4}
	}
	return slices.DeleteFunc(lines, func(down int) bool { return down > rows })
}

// besideSessions is account r's sessions beside its block: after ● pinned
// and a dot, where the global pin names it.
func besideSessions(r accountRouting) line {
	if !r.pinned {
		return line{r.sessionsCounted()}
	}
	return line{pinnedSpan, {"  ·  ", faintInk}, r.sessionsCounted()}
}

// countedWords are the words the pool's accounts' states are counted in, in
// their order, each in its state's colour, and the kinds each counts: an
// account held back from some models' requests alone, by a limit, a refusal
// or its cap in a model's own window, is open to the rest.
var countedWords = []struct {
	word  string
	ink   ink
	kinds []status.RoutingKind
}{
	{"open", secondaryInk, []status.RoutingKind{status.RoutingOpen, status.RoutingNext, status.RoutingIdle, status.RoutingModelLimited, status.RoutingModelRefused, status.RoutingModelCapped}},
	{"pressure", warningInk, []status.RoutingKind{status.RoutingPressed}},
	{"at cap", warningInk, []status.RoutingKind{status.RoutingCapped, status.RoutingCappedPinned}},
	{"limit", errorInk, []status.RoutingKind{status.RoutingLimited}},
	{"refused", errorInk, []status.RoutingKind{status.RoutingRefused}},
	{"not read", dimInk, []status.RoutingKind{status.RoutingUnread, status.RoutingUnreadable, status.RoutingTokenless}},
}

// countedStates are the accounts' states counted, as in "2 open · 1 at cap",
// in countedWords' order, each count in its state's colour, a dot in
// text.faint between them; where that's wider than width cells, how many are
// open, pressure counting as open, as in "8 of 10 open", in text.secondary.
func countedStates(accounts []accountRouting, width int) line {
	var counted line
	for _, w := range countedWords {
		if n := counting(accounts, w.kinds); n > 0 {
			counted = joined(span{" · ", faintInk}, counted, line{statesCounted(n, w.word, w.ink)})
		}
	}
	if counted.width() <= width {
		return counted
	}
	open := counting(accounts, slices.Concat(countedWords[0].kinds, []status.RoutingKind{status.RoutingPressed}))
	return line{statesCounted(open, fmt.Sprintf("of %d open", len(accounts)), secondaryInk)}.fit(width)
}

// statesCounted is n of the pool's accounts counted in the words given, in
// k, as in "2 open", numbered as the pool's states counted in those words.
func statesCounted(n int, words string, k ink) span {
	return span{fmt.Sprintf("%d %s", n, words), k}.numbered(Number{Kind: NumberStates, Of: words})
}

// counting is how many of the accounts stand as one of the kinds given.
func counting(accounts []accountRouting, kinds []status.RoutingKind) int {
	n := 0
	for _, r := range accounts {
		if slices.Contains(kinds, r.standing.Kind) {
			n++
		}
	}
	return n
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
