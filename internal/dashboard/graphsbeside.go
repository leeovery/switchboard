package dashboard

import (
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The charts beside USAGE's blocks by account: each leastBeside cells wide at
// the least, besideGap cells apart.
const (
	leastBeside = 24
	besideGap   = 2
)

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
// frame's edge, as chartsOfBlock has them: as many as fit, as besideFit has
// them, side by side, each as wide as they share the room, besideGap cells
// apart; each from the block's first row to the row after its last, its axis
// on that row, so a block of three windows has four rows of chart.
func (f Frame) chartsBeside(c *canvas, doc status.Document, m status.Moment, at blocksBeside) {
	room := f.edge() - at.x
	for _, block := range at.blocks {
		charts := f.chartsOfBlock(doc, m, block.id)
		n := besideFit(len(charts), room)
		if n == 0 {
			continue
		}
		width := (room+besideGap)/n - besideGap
		for i, ch := range charts[:n] {
			ch.draw(c, f.Look, bounds{x: at.x + i*(width+besideGap), y: block.y, width: width, rows: block.rows + 1})
		}
	}
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

// besideFit is how many of k charts fit side by side in room cells, each
// leastBeside cells at the least, besideGap cells apart.
func besideFit(k, room int) int {
	return max(min(k, (room+besideGap)/(leastBeside+besideGap)), 0)
}
