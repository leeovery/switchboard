package dashboard

import (
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
)

// The charts beside USAGE's blocks by account: besideGap cells apart;
// leastBeside rows of chart at the least, and at USAGE's least,
// leastTodayCells across, as they were before every chart showed.
const (
	besideGap       = 2
	leastBeside     = 2
	leastTodayCells = 24
)

// besideForm is how GRAPHS' charts sit beside USAGE's blocks by account, from
// USAGE's least to its most, each a step it grows to whole, as the rows it's
// given allow.
type besideForm int

const (
	// besideToday is USAGE's least: as many charts side by side as are
	// leastTodayCells or more, the rest left off, a chart from a block's
	// first row to the row after its last, or leastBeside rows of chart
	// where that's more.
	besideToday besideForm = iota
	// besideStacked is every chart, as many side by side as are
	// leastChartCells or more, the rest on the rows under, each row of them
	// leastBeside rows of chart.
	besideStacked
	// besideTall is as besideStacked, each row of charts leastChart rows of
	// chart.
	besideTall
)

// across lays k charts across room cells beside a block in the form, as
// chartsFitting has them, besideGap cells apart.
func (form besideForm) across(k, room int) chartsAcross {
	if form == besideToday {
		return chartsAcross{spans: chartsFitting(k, room, besideGap, leastTodayCells)}
	}
	return chartsAcross{spans: chartsFitting(k, room, besideGap, leastChartCells), stacks: true}
}

// chartRows are the rows of chart each row of charts beside a block has in
// the form, at the least.
func (form besideForm) chartRows() int {
	if form == besideTall {
		return leastChart
	}
	return leastBeside
}

// graphsBeside reports whether GRAPHS is drawn beside USAGE's blocks rather
// than as a section of its own: by account, from matrixWidth columns, while
// both show. ROUTING is a section of its own while it is, and the Overview
// has no focus.
func (f Frame) graphsBeside() bool {
	// From matrixWidth columns, a chart always fits beside the blocks: the
	// least the room beside them leaves is 42 cells.
	return f.By == ByAccount && f.Width >= matrixWidth && f.Sections.Has(SectionUsage) && f.Sections.Has(SectionGraphs)
}

// Focuses reports whether the Overview has an account focus, which ↑ and ↓
// move, GRAPHS follows and USAGE's matrix picks out: always, but while
// GRAPHS charts every account beside USAGE's blocks, as graphsBeside says.
func (f Frame) Focuses() bool {
	return !f.graphsBeside()
}

// besideUsage is what draws what's beside USAGE's blocks by account of doc at
// now, as at says they lie: GRAPHS' charts, where graphsBeside says they're
// there, as chartsBeside draws them; else ROUTING's lines, where
// routingBesideUsage says they are, as routingBeside draws them, its table
// built only as it's first drawn; nil where nothing is.
func (f Frame) besideUsage(doc status.Document, now time.Time) func(c *canvas, at blocksBeside) {
	switch {
	case f.graphsBeside():
		m := f.Moment(doc, now)
		return func(c *canvas, at blocksBeside) { f.chartsBeside(c, doc, m, at) }
	case f.routingBesideUsage():
		routing := sync.OnceValue(func() routingTable { return f.routingTableOf(doc, now) })
		return func(c *canvas, at blocksBeside) { f.routingBeside(c, routing(), at) }
	}
	return nil
}

// besideChart is a chart GRAPHS draws beside a block of USAGE's by account:
// an account's window's, or the pool's.
type besideChart interface {
	draw(c *canvas, look Look, at bounds)
}

// chartsBeside draws the charts of doc at m's moment beside each block of
// USAGE's by account, as at says the blocks lie, from its column to the
// frame's edge, as chartsOfBlock has them, laid across as at's form has them;
// from the block's first row to the rows under its last that at gives what's
// beside it, the rows of charts sharing them, the last's axis on the last of
// them.
func (f Frame) chartsBeside(c *canvas, doc status.Document, m status.Moment, at blocksBeside) {
	for _, block := range at.blocks {
		charts := f.chartsOfBlock(doc, m, block.id)
		across := at.form.across(len(charts), f.edge()-at.x)
		if len(across.spans) == 0 {
			continue
		}
		area := bounds{x: at.x, y: block.y, rows: block.rows + at.under}
		for i, ch := range charts[:across.shown(len(charts))] {
			ch.draw(c, f.Look, across.placed(i, area, across.rowsFor(len(charts)), flex.Part{Grows: true}))
		}
	}
}

// PoolChart is a chart of the pool's room in a window GRAPHS draws beside
// Σ pool by account: its window's key, and how many columns it's drawn
// across, a column a cell.
type PoolChart struct {
	Key     string
	Columns int
}

// PoolCharts are the charts of the pool's room in its windows GRAPHS draws
// beside Σ pool by account of doc at now, as chartsBeside lays them out in
// the form USAGE takes in the rows the Overview gives it, in USAGE's order:
// none where GRAPHS isn't drawn beside USAGE's blocks, as graphsBeside says,
// or there's no pool.
func (f Frame) PoolCharts(doc status.Document, now time.Time) []PoolChart {
	if !f.graphsBeside() || len(f.Moment(doc, now).Pool().Windows) == 0 {
		return nil
	}
	windows := f.overviewWindows(doc, now)
	block := f.usageDrawn(doc, now)
	across := block.beside.across(len(windows), f.edge()-block.besideAt())
	if len(across.spans) == 0 {
		return nil
	}
	charts := make([]PoolChart, across.shown(len(windows)))
	for i := range charts {
		charts[i] = PoolChart{Key: windows[i].key, Columns: across.spans[i%len(across.spans)].Size}
	}
	return charts
}

// chartsOfBlock are the charts of doc at m's moment beside the block of the
// account with the given id, in USAGE's order: its windows', as chartsOf has
// them; or beside the pool's, its id "", the pool's, as poolChartsOf has
// them.
func (f Frame) chartsOfBlock(doc status.Document, m status.Moment, id string) []besideChart {
	var charts []besideChart
	if id == "" {
		for _, ch := range f.poolChartsOf(doc, m) {
			charts = append(charts, ch)
		}
		return charts
	}
	if a, ok := m.Account(id); ok {
		for _, ch := range f.chartsOf(doc, m.Now, a) {
			charts = append(charts, ch)
		}
	}
	return charts
}
