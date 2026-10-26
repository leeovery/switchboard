package dashboard

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// GLANCE's parts, as the design lays them out: a window's sparkline's cells;
// what goes between the items of its key; and what its key's first item
// draws a sparkline's cell as.
const (
	sparkCells     = 16
	glanceKeyApart = 4
	sparkTile      = "▆"
)

// glanceLaid is GLANCE laid out across a width, of the configured accounts,
// in the config's order, as numbered, whatever the reads say, so it holds
// still as they come: the accounts; the cells of its name column and of each
// window's words; whether its windows stack, the week's rows then the
// 5-hour's; the width it takes; and its key, flowed onto as many rows as it
// takes.
type glanceLaid struct {
	accounts config.Accounts
	column   int
	words    int
	stacked  bool
	width    int
	key      []line
}

// layGlance is GLANCE laid out width cells wide, of the configured accounts
// given, numbered as n numbers them: its windows side by side, as glanceRow
// lays them, where their parts fit at their leasts; else stacked, a window a
// row. It takes no more than its natural, as it doesn't grow, and its key is
// flowed in what it takes, its items glanceKeyApart apart, an item never
// broken.
func layGlance(n status.Numbering, accounts config.Accounts, width int) glanceLaid {
	names := configuredNames(n, accounts)
	l := glanceLaid{accounts: accounts}
	row := glanceRowOf(names, len(runwayWindows))
	column, words, ok := row.lay(width)
	if !ok {
		row, l.stacked = glanceRowOf(names, 1), true
		column, words, _ = row.lay(width)
	}
	l.column, l.words, l.width = column, words, min(width, row.natural())
	l.key = wrapped(spaces(glanceKeyApart), l.width, glanceKey())
	return l
}

// glancePart is GLANCE across the cells flex lays it in, as Graphs' planner
// lays it beside WHEN YOU WORK, of the configured accounts given, numbered
// as n numbers them: its natural its windows side by side, every part at its
// natural; its least its windows stacked, every part at its least; and its
// most its natural, as it doesn't grow.
func glancePart(n status.Numbering, accounts config.Accounts) flex.Part {
	names := configuredNames(n, accounts)
	natural := glanceRowOf(names, len(runwayWindows)).natural()
	return flex.Part{Least: glanceRowOf(names, 1).least(), Natural: natural, Most: natural}
}

// configuredNames are the configured accounts' names, as the config names
// and marks them, in its order, numbered as n numbers them and aligned as
// graphsNamed aligns them, as a row of GLANCE heads each, whatever the reads
// say: ▲'s cells kept on every one, as accountName's cells keeps them.
func configuredNames(n status.Numbering, accounts config.Accounts) []accountName {
	return namesAt(n.Configured(accounts), n.Held())
}

// glanceRow is a row of GLANCE, windows windows across, as flex lays it: its
// name column, as usage's block lays one, narrowing toward its least in
// usage's names turn; each window's sparkline and the cells beside it, a
// cell, its figure and two cells, as afterBar counts them, groupGap between
// two windows; and each window's words, from the widest any says in full,
// as fullWidest has them, so the next window holds still, narrowing toward
// the short form's in usage's words turn. None grows.
type glanceRow struct {
	column, kept, words flex.Part
	windows             int
}

// glanceRowOf is a row of GLANCE of the names given, windows windows across.
func glanceRowOf(names []accountName, windows int) glanceRow {
	column := namesPart(names)
	column.Narrows = usageNamesTurn
	kept := windows*(sparkCells+afterBar) + (windows-1)*groupGap
	return glanceRow{
		column:  column,
		kept:    flex.Part{Least: kept, Natural: kept},
		words:   flex.Part{Least: windows * shortCells, Natural: windows * fullWidest(), Narrows: usageWordsTurn},
		windows: windows,
	}
}

// lay are the cells of the row's name column and of each window's words
// width cells wide, as flex lays them, reporting whether they fit at their
// leasts: where they don't, the words take what the row leaves them, none
// where it leaves none.
func (r glanceRow) lay(width int) (column, words int, ok bool) {
	s, ok := flex.Lay(width, r.column, r.kept, r.words)
	words = s[2].Size
	if !ok {
		words = max(width-s[2].At, 0)
	}
	return s[0].Size, words / r.windows, ok
}

// least is the cells the row takes, every part at its least.
func (r glanceRow) least() int {
	return r.column.Least + r.kept.Least + r.words.Least
}

// natural is the cells the row takes, every part at its natural.
func (r glanceRow) natural() int {
	return r.column.Natural + r.kept.Natural + r.words.Natural
}

// windowsDrawn are the places among runwayWindows of the windows each part
// of GLANCE draws, in its order: all of them, side by side; else each a
// part of its own, stacked.
func (l glanceLaid) windowsDrawn() [][]int {
	all := make([]int, len(runwayWindows))
	for i := range all {
		all[i] = i
	}
	if !l.stacked {
		return [][]int{all}
	}
	parts := make([][]int, len(all))
	for i := range all {
		parts[i] = all[i : i+1]
	}
	return parts
}

// partRows are the rows a part of GLANCE takes: its head row; a row an
// account, the first under a blank row, a blank row before each after it;
// and usage's rule and the pool's row, where its accounts are a pool, as
// pooled has it.
func (l glanceLaid) partRows() int {
	rows := 1 + 2*len(l.accounts)
	if pooled(l.accounts) {
		rows += 2
	}
	return rows
}

// rows are the rows GLANCE takes as laid out, as draw draws it: each part
// and a blank row after it; then its key.
func (l glanceLaid) rows() int {
	return len(l.windowsDrawn())*(l.partRows()+1) + len(l.key)
}

// heads are the rows of each part's head row, from GLANCE's first, as draw
// draws them, each over a blank row and its first account's.
func (l glanceLaid) heads() []int {
	heads := make([]int, len(l.windowsDrawn()))
	for i := range heads {
		heads[i] = i * (l.partRows() + 1)
	}
	return heads
}

// group are the cells from a window's sparkline to the next's: the
// sparkline, the cells beside it, its words and groupGap.
func (l glanceLaid) group() int {
	return sparkCells + afterBar + l.words + groupGap
}

// glance is GLANCE as Graphs draws it at a look: the moment; each account's
// windows' trails, as the cards read them; Runway's lanes over each window's
// live view, by the window, as the frame has them built, which place a held
// window's ✕, as a card's; the pool's room over each window's span, by the
// window, as the watch built it; and the look.
type glance struct {
	m      status.Moment
	trails Trails
	lanes  map[views.RunwayWindow]WindowLanes
	pools  map[views.RunwayWindow]views.PoolSpan
	look   Look
}

// glanceLine is a row of GLANCE drawn, an account's or the pool's: its name,
// and each window's, in runwayWindows' order, as glanceGauge has it.
type glanceLine struct {
	name   line
	gauges []glanceGauge
}

// glanceGauge is a window of an account, or the pool's, as a row of GLANCE
// draws it: its sparkline; its figure, the room it has left, none of the
// pool's, whose words say its room; and its words, as a gauge has them.
type glanceGauge struct {
	spark  sparkline
	figure line
	words  gauge
}

// sparkline is a window as GLANCE draws it in a row of eighths, width cells
// from x along row y.
type sparkline interface {
	spark(c *canvas, look Look, x, y, width int)
}

// draw draws GLANCE as l lays it out from x down from row y, and nothing
// outside the rows and the width it takes: each part a blank row after the
// one before, as part draws it; then, under a blank row, its key.
func (g glance) draw(c *canvas, l glanceLaid, x, y int) {
	in := c.within(bounds{x: x, y: y, width: l.width, rows: l.rows()})
	accounts, pool := g.lines(l)
	for _, windows := range l.windowsDrawn() {
		y = g.part(in, l, x, y, windows, accounts, pool) + 1
	}
	for i, k := range l.key {
		in.line(x, y+i, k)
	}
}

// part draws a part of GLANCE, the windows given by their places among
// runwayWindows side by side, from x down from row y, returning the row
// after its last: each window's name, in capitals, in text.subtle, over its
// sparkline; under a blank row, a row an account, in the config's order, a
// blank row between; and where there's a pool, usage's rule under them and
// the pool's row, as the rows given have it.
func (g glance) part(c *canvas, l glanceLaid, x, y int, windows []int, accounts []glanceLine, pool *glanceLine) int {
	for i, w := range windows {
		c.text(x+l.column+i*l.group(), y, strings.ToUpper(runwayNames[runwayWindows[w]]), dimInk)
	}
	row := y
	for _, a := range accounts {
		row += 2
		g.row(c, l, x, row, windows, a)
	}
	if pooled(l.accounts) {
		if pool != nil {
			c.line(x, row+1, line{{rule(l.width), borderInk}})
			g.row(c, l, x, row+2, windows, *pool)
		}
		row += 2
	}
	return row + 1
}

// row draws a row of GLANCE, its windows given by their places among
// runwayWindows, from x along row y: its name; then each window a group on
// from the name column: its sparkline, sparkCells long; a cell; its figure,
// right-aligned in percentCells; two cells; and its words, in full where
// they fit what the layout gives them, else short, as fitted has them, led
// as led has them; or, of the pool's, its words from where a figure goes.
func (g glance) row(c *canvas, l glanceLaid, x, y int, windows []int, r glanceLine) {
	c.line(x, y, r.name)
	for i, w := range windows {
		gg, at := r.gauges[w], x+l.column+i*l.group()
		gg.spark.spark(c, g.look, at, y, sparkCells)
		figure := at + sparkCells + 1
		words, end := figure, figure+percentCells+2+l.words
		if gg.figure != nil {
			c.right(figure+percentCells, y, gg.figure)
			words += percentCells + 2
		}
		c.line(words, y, gg.words.led(gg.words.words.fitted(end-words)))
	}
}

// lines are GLANCE's rows of the moment, as l lays them out: each configured
// account's, in the config's order, as graphsNamed has it, so one the
// moment doesn't hold yet is one not read yet, numbered and named as Runway
// numbers and names it, faint where it's out of the pool, as the Overview
// names it, and each window as accountGauge has it; and the pool's, as
// poolLine has it.
func (g glance) lines(l glanceLaid) ([]glanceLine, *glanceLine) {
	accounts, names := graphsNamed(g.m, l.accounts)
	lines := make([]glanceLine, len(accounts))
	for i, a := range accounts {
		names[i].faint = names[i].out
		lines[i].name = names[i].line(l.column)
		_, known := g.m.Account(a.ID)
		for _, w := range runwayWindows {
			lines[i].gauges = append(lines[i].gauges, g.accountGauge(a, known, w))
		}
	}
	return lines, g.poolLine(names)
}

// poolLine is the pool's row of GLANCE under the accounts named as given,
// nil where the moment has no pool, as before it holds two accounts: Σ pool,
// as usage's pool row names it; and each window's sparkline of the pool's
// room over its span, as poolSpark draws it, and its room, as usage's pool
// row words it, pinned after the first.
func (g glance) poolLine(names []accountName) *glanceLine {
	shown := make([]shownWindow, len(runwayWindows))
	for i, w := range runwayWindows {
		shown[i] = shownWindow{key: w.KeyIn(g.m.Policy()), name: runwayNames[w]}
	}
	row, ok := poolRow(g.m.Pool(), names, shown)
	if !ok {
		return nil
	}
	pool := &glanceLine{name: row.name}
	for i, w := range runwayWindows {
		pool.gauges = append(pool.gauges, glanceGauge{spark: poolSpark{span: g.pools[w]}, words: row.gauges[i]})
	}
	return pool
}

// accountGauge is account a's window of Runway's view w as GLANCE draws it,
// known where the moment holds the account, else as one not read yet: its
// sparkline, its burn-down card's, as burnCardOf has it from its trail, its
// ✕ where a hold began as heldSince has it; the room it has left, as
// roomFigure has it, or where nothing's read of it or the account has no
// usable token, — in text.faint, as usage's figure; and usage's words for
// it, as wordsOf has them, faint where it's out of the pool.
func (g glance) accountGauge(a status.AccountNow, known bool, w views.RunwayWindow) glanceGauge {
	key := w.KeyIn(g.m.Policy())
	win, _ := a.Window(key)
	if !known {
		win.Kind = status.WindowUnread
	}
	card := burnCardOf(a, key, g.trails[Ref{Account: a.ID, Window: key}], g.m.Now)
	card.heldSince = g.heldSince(w, a, key)
	gg := glanceGauge{spark: card, figure: line{noFigure}, words: gauge{of: Ref{Account: a.ID, Window: key}, words: wordsOf(win, g.m.Now)}}
	if !drawsEmpty(win.Kind) {
		gg.figure = line{roomFigure(gg.words.of, win.Used, a.OutOfPool, win.Kind == status.WindowStale)}
	}
	if a.OutOfPool {
		gg.words.words = gg.words.words.faint()
	}
	return gg
}

// heldSince is when the hold on account a's window with the given key, of
// Runway's view w, began, as a card's ✕ is placed: as the account's lane over
// that view tells, where the frame has the lanes built, else as the moment's
// events tell, as Lane.HeldSince has it.
func (g glance) heldSince(w views.RunwayWindow, a status.AccountNow, key string) time.Time {
	l, _ := g.lanes[w].lane(a.ID)
	return l.HeldSince(a, key, g.lanes[w].Timeline)
}

// poolSpark is the pool's room over a window's span, as GLANCE's sparkline
// draws it beside Σ pool.
type poolSpark struct {
	span views.PoolSpan
}

// spark draws the pool's room as a sparkline, width cells from x along row
// y, a column a cell of its span's: each column's room, of all its
// accounts', in eighths of the row, as roomEighths counts it, in a lasting
// window's ink, as aheadInk fades it from now's column on, as a card's
// sparkline is drawn; none where it isn't known.
func (p poolSpark) spark(c *canvas, look Look, x, y, width int) {
	now := p.span.Timeline.NowColumn()
	for col, room := range p.span.Columns[:min(width, len(p.span.Columns))] {
		k := lastingInk
		if col >= now {
			k = aheadInk(k)
		}
		if eighths := roomEighths(1-room.Used, 1); room.Known && eighths > 0 {
			fillUp(c, look, x+col, y, eighths, func(int) ink { return k })
		}
	}
}

// glanceDrawn draws GLANCE of the moment m in the section placed, from its
// left, as laid lays it out across the section's columns: each account's
// sparklines of its trails, as the cards read them, each held window's ✕
// placed by the lanes the watch built, as a card's is; and the pool's of its
// room over each window's span, as the watch built it.
func (f Frame) glanceDrawn(c *canvas, m status.Moment, laid graphsLaid, placed graphsPlaced) {
	body := placed.body(laid.glance.width)
	glance{m: m, trails: f.Trails, lanes: f.Graphs.Lanes, pools: f.Graphs.Spans, look: f.Look}.draw(c, laid.glance, body.x, body.y)
}

// aheadInk is k as a sparkline draws where its window's heading: faded
// aheadFade more into the canvas, and faint where the look can't fade it, as
// POOL's heading is, so it's told from the room before now.
func aheadInk(k ink) ink {
	k.fade, k.dim = 1-(1-k.fade)*(1-aheadFade), true
	return k
}

// glanceKey are the items of GLANCE's key, each mark in its ink and what it
// means in text.muted: a sparkline's cell in a lasting window's ink, the room
// left from its start to its reset; in that ink as it heads, as aheadInk has
// it; ✕, where it runs out; and a figure, the room left now, bold
// text.secondary.
func glanceKey() []line {
	return []line{
		{{sparkTile, lastingInk}, {" room left, start to reset", mutedInk}},
		{{sparkTile, aheadInk(lastingInk)}, {" heading", mutedInk}},
		{{outMark, outInk}, {" runs out", mutedInk}},
		{{"66%", strongInk}, {" room left now", mutedInk}},
	}
}
