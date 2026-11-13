package dashboard

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// GLANCE's parts, as the design lays them out: the fewest cells of a
// sparkline; what goes between the items of its key; and what its key's
// first item draws a sparkline's cell as.
const (
	sparkCells     = 16
	glanceKeyApart = 4
	sparkTile      = "▆"
)

// glanceLaid is GLANCE laid out across a width, of the configured accounts,
// in the config's order, whatever the reads say, so it holds still as they
// come: the accounts; the cells of its name column, of its sparklines and of
// its words; the width it takes; and its key, flowed onto as many rows as it
// takes.
type glanceLaid struct {
	accounts config.Accounts
	column   int
	spark    int
	words    int
	width    int
	key      []line
}

// layGlance is GLANCE laid out width cells wide, of the configured accounts
// given, as glanceRow lays its rows out, so it takes the width, and its key
// flowed in it, its items glanceKeyApart apart, an item never broken.
func layGlance(accounts config.Accounts, width int) glanceLaid {
	l := glanceLaid{accounts: accounts, width: max(width, 0)}
	l.column, l.spark, l.words = glanceRowOf(configuredNames(accounts)).lay(width)
	l.key = wrapped(spaces(glanceKeyApart), l.width, glanceKey())
	return l
}

// glancePart is GLANCE across the cells flex lays it in, as Graphs' planner
// lays it beside WHEN YOU WORK, of the configured accounts given: its least
// every part at its least; its natural every part at its natural; and
// growing past that, as its sparklines do.
func glancePart(accounts config.Accounts) flex.Part {
	row := glanceRowOf(configuredNames(accounts))
	return flex.Part{Least: row.least(), Natural: row.natural(), Grows: true}
}

// configuredNames are the configured accounts' names, as the config names
// and marks them, in its order, as a row of GLANCE heads each, whatever the
// reads say: ▲'s cells kept on every one, as accountName's cells keeps them.
func configuredNames(accounts config.Accounts) []accountName {
	names := make([]accountName, len(accounts))
	for i, a := range accounts {
		names[i] = accountName{label: labelled(a.Label, a.ID), primary: a.Primary}
	}
	return names
}

// glanceRow is a row of GLANCE as flex lays it: its name column, as usage's
// block lays one, narrowing toward its least in usage's names turn; its
// sparkline, from sparkCells, growing with what's spare; the cells beside
// it, a cell, its figure and two cells, as afterBar counts them; and its
// words, from the widest any says in full, as fullWidest has them, so they
// line up down the rows, narrowing toward the short form's in usage's words
// turn.
type glanceRow struct {
	column, spark, kept, words flex.Part
}

// glanceRowOf is a row of GLANCE of the names given.
func glanceRowOf(names []accountName) glanceRow {
	column := namesPart(names)
	column.Narrows = usageNamesTurn
	return glanceRow{
		column: column,
		spark:  flex.Part{Least: sparkCells, Natural: sparkCells, Grows: true},
		kept:   flex.Part{Least: afterBar, Natural: afterBar},
		words:  flex.Part{Least: shortCells, Natural: fullWidest(), Narrows: usageWordsTurn},
	}
}

// lay are the cells of the row's name column, its sparkline and its words
// width cells wide, as flex lays them: where they don't fit at their leasts,
// the words take what the row leaves them, none where it leaves none.
func (r glanceRow) lay(width int) (column, spark, words int) {
	s, ok := flex.Lay(width, r.column, r.spark, r.kept, r.words)
	words = s[3].Size
	if !ok {
		words = max(width-s[3].At, 0)
	}
	return s[0].Size, s[1].Size, words
}

// least is the cells the row takes, every part at its least.
func (r glanceRow) least() int {
	return r.column.Least + r.spark.Least + r.kept.Least + r.words.Least
}

// natural is the cells the row takes, every part at its natural.
func (r glanceRow) natural() int {
	return r.column.Natural + r.spark.Natural + r.kept.Natural + r.words.Natural
}

// bodyRows are the rows of GLANCE over its key: its head row; a row an
// account, the first under a blank row, a blank row before each after it;
// and usage's rule and the pool's row, where its accounts are a pool, as
// pooled has it.
func (l glanceLaid) bodyRows() int {
	rows := 1 + 2*len(l.accounts)
	if pooled(l.accounts) {
		rows += 2
	}
	return rows
}

// rows are the rows GLANCE takes as laid out, as draw draws it: its body, a
// blank row, then its key.
func (l glanceLaid) rows() int {
	return l.bodyRows() + 1 + len(l.key)
}

// glance is GLANCE as Graphs draws it at a look: the moment; each account's
// trails, as the cards read them; Runway's lanes over its live view of the
// week, as the frame has them built, which place a held week's ✕, as a
// card's; the pool's room over the week's span, as the watch built it; and
// the look.
type glance struct {
	m      status.Moment
	trails Trails
	lanes  map[views.RunwayWindow]WindowLanes
	pools  map[views.RunwayWindow]views.PoolSpan
	look   Look
}

// glanceLine is a row of GLANCE drawn, an account's or the pool's: its name,
// and its week, as glanceGauge has it.
type glanceLine struct {
	name  line
	gauge glanceGauge
}

// glanceGauge is the week of an account, or the pool's, as a row of GLANCE
// draws it: its sparkline; its figure, the room it has left; and its words,
// as a gauge has them. The pool's has its words at the figure: they say its
// room, from where a figure goes.
type glanceGauge struct {
	spark         sparkline
	figure        line
	words         gauge
	wordsAtFigure bool
}

// sparkline is a window as GLANCE draws it in a row of eighths, width cells
// from x along row y.
type sparkline interface {
	spark(c *canvas, look Look, x, y, width int)
}

// draw draws GLANCE as l lays it out from x down from row y, and nothing
// outside the rows and the width it takes: the week's name, in capitals, in
// text.subtle, over its sparklines; under a blank row, a row an account, in
// the config's order, a blank row between; where there's a pool, usage's
// rule under them and the pool's row; then, under a blank row, its key.
func (g glance) draw(c *canvas, l glanceLaid, x, y int) {
	in := c.within(bounds{x: x, y: y, width: l.width, rows: l.rows()})
	accounts, pool := g.lines(l)
	in.text(x+l.column, y, strings.ToUpper(runwayWeek), dimInk)
	for i, a := range accounts {
		g.row(in, l, x, y+2+2*i, a)
	}
	if pool != nil {
		row := y + 2*len(accounts)
		in.line(x, row+1, line{{rule(l.width), borderInk}})
		g.row(in, l, x, row+2, *pool)
	}
	for i, k := range l.key {
		in.line(x, y+l.bodyRows()+1+i, k)
	}
}

// row draws a row of GLANCE from x along row y: its name; then from the name
// column its sparkline, as wide as the layout gives it; a cell; its figure,
// right-aligned in percentCells; two cells; and its words, in full where
// they fit what the layout gives them, else short, as fitted has them, led
// as led has them; or, of a gauge with its words at the figure, as the
// pool's, its words from where a figure goes.
func (g glance) row(c *canvas, l glanceLaid, x, y int, r glanceLine) {
	c.line(x, y, r.name)
	gg, at := r.gauge, x+l.column
	gg.spark.spark(c, g.look, at, y, l.spark)
	figure := at + l.spark + 1
	words, end := figure, figure+percentCells+2+l.words
	if !gg.wordsAtFigure {
		c.right(figure+percentCells, y, gg.figure)
		words += percentCells + 2
	}
	c.line(words, y, gg.words.led(gg.words.words.fitted(end-words)))
}

// lines are GLANCE's rows of the moment, as l lays them out: each configured
// account's, in the config's order, as graphsNamed has it, so one the
// moment doesn't hold yet is one not read yet, named as Runway names it,
// faint where it's out of the pool, as the Overview names it, and its week
// as accountGauge has it; and the pool's, as poolLine has it.
func (g glance) lines(l glanceLaid) ([]glanceLine, *glanceLine) {
	accounts, names := graphsNamed(g.m, l.accounts)
	lines := make([]glanceLine, len(accounts))
	for i, a := range accounts {
		_, known := g.m.Account(a.ID)
		lines[i] = glanceLine{name: names[i].line(l.column), gauge: g.accountGauge(a, known)}
	}
	return lines, g.poolLine(names)
}

// poolLine is the pool's row of GLANCE under the accounts named as given,
// nil where the moment has no pool, as before it holds two accounts: Σ pool,
// as usage's pool row names it; and the sparkline of the pool's room over
// the week's span, as poolSpark draws it, and its room, as usage's pool row
// words it, pinned after it.
func (g glance) poolLine(names []accountName) *glanceLine {
	week := shownWindow{key: GraphsWindow.KeyIn(g.m), name: runwayWeek}
	row, ok := poolRow(g.m, names, []shownWindow{week})
	if !ok {
		return nil
	}
	return &glanceLine{name: poolNamed, gauge: glanceGauge{spark: poolSpark{span: g.pools[GraphsWindow]}, words: row.gauges[0], wordsAtFigure: true}}
}

// accountGauge is account a's week, of Runway's view of it, as GLANCE draws
// it, known where the moment holds the account, else as one not read yet: its
// sparkline, its burn-down card's, as burnCardOf has it from its trail, its
// ✕ where a hold began as heldSince has it, empty where it lacks the week;
// the room it has left, as roomFigure has it, or where nothing's read of it,
// the account has no usable token, or it lacks the window, — in text.faint,
// as usage's figure; and usage's words for it, as wordsOf has them, faint
// where it's out of the pool.
func (g glance) accountGauge(a status.AccountNow, known bool) glanceGauge {
	key := GraphsWindow.KeyIn(g.m)
	win, _ := a.Window(key)
	if !known {
		win.Kind = status.WindowUnread
	}
	card := burnCardOf(a, key, g.trails[Ref{Account: a.ID, Window: key}], g.m.Now)
	card.heldSince = g.heldSince(a, key)
	gg := glanceGauge{spark: card, words: gauge{of: Ref{Account: a.ID, Window: key}, words: wordsOf(win, g.m.Now)}}
	if drawsEmpty(win.Kind) {
		gg.figure = line{noFigure}
	} else {
		gg.figure = line{roomFigure(gg.words.of, win.Used, a.OutOfPool, win.Kind == status.WindowStale)}
	}
	if a.OutOfPool {
		gg.words.words = gg.words.words.faint()
	}
	return gg
}

// heldSince is when the hold on account a's window with the given key, the
// week's, began, as a card's ✕ is placed: as the account's lane over Runway's
// view of it tells, where the frame has the lanes built, else as the moment's
// events tell, as Lane.HeldSince has it.
func (g glance) heldSince(a status.AccountNow, key string) time.Time {
	lanes := g.lanes[GraphsWindow]
	l, _ := lanes.lane(a.ID)
	return l.HeldSince(a, key, lanes.Timeline)
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
// sparkline of its trail, as the cards read it, a held week's ✕ placed by the
// lanes the watch built, as a card's is; and the pool's of its room over the
// week's span, as the watch built it.
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
