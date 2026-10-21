package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
)

// The rows a GRAPHS chart takes: leastChart at the least, and mostChart at
// the most, as taller distorts; and chartEnds more, its top row and its axis
// row.
const (
	leastChart = 5
	mostChart  = 19
	chartEnds  = 2
)

// leastChartCells are the fewest cells a GRAPHS chart is across.
const leastChartCells = 30

// chartGap are the cells between two charts side by side.
const chartGap = 3

// Burn is the chart GRAPHS draws of each window, as v flips it while GRAPHS
// shows, which the preferences keep: its burn-up, its use rising from empty;
// or its burn-down, its room falling from full.
type Burn int

const (
	// BurnUp is each window's use so far, rising from empty at its start,
	// against even pace.
	BurnUp Burn = iota
	// BurnDown is each window's room left, falling from full at its start,
	// and where it's heading.
	BurnDown
)

// burnNames are each chart's name, as the key line and the preferences have
// it.
var burnNames = [...]string{BurnUp: "burn-up", BurnDown: "burn-down"}

// BurnKept is the chart the preferences keep, by its name: the burn-down, or
// the burn-up, as a file without one, or one naming a chart this build
// doesn't know, as a later build's, has it.
func BurnKept(name string) Burn {
	if name == burnNames[BurnDown] {
		return BurnDown
	}
	return BurnUp
}

// Name is the chart's name, as the key line has it after v, as "burn-up".
func (b Burn) Name() string {
	return burnNames[b]
}

// Kept is the chart's name as the preferences keep it: none for the burn-up,
// which a file without one opens on.
func (b Burn) Kept() string {
	if b == BurnUp {
		return ""
	}
	return b.Name()
}

// Flipped is the other chart, as v flips GRAPHS to.
func (b Burn) Flipped() Burn {
	if b == BurnDown {
		return BurnUp
	}
	return BurnDown
}

// graphs are the Overview's GRAPHS: the focused account's use so far, a chart
// a window that USAGE shows, laid across as layAcross has them, drawn as burn
// names.
type graphs struct {
	name    span
	focused bool
	charts  []chart
	across  chartsAcross
	look    Look
	burn    Burn
}

// graphsSection is the Overview's GRAPHS of doc at now: the focused
// account's use so far, a chart a window, leastChart rows tall at its least
// and mostChart at its most; or beside USAGE's blocks by account, as
// graphsAside has it, where graphsBeside says so.
func (f Frame) graphsSection(doc status.Document, now time.Time) section {
	if f.graphsBeside() {
		return graphsAside(f.Burn)
	}
	g := f.graphsOf(doc, now)
	return section{
		Section: SectionGraphs, least: g.rows(leastChart), most: g.rows(mostChart),
		description: g.description, keyed: true,
		draw:   g.draw,
		folded: g.folded,
	}
}

// graphsAside is GRAPHS beside USAGE's blocks by account: no rows of its own,
// nothing drawn and no targets of its own, as USAGE's draw has chartsBeside
// draw it where its blocks lie; its heading the key to the charts' lines
// alone, as no account is focused.
func graphsAside(burn Burn) section {
	key := func() line { return burnKey(burn) }
	return section{
		Section: SectionGraphs, description: key, keyed: true, folded: key,
		draw:    func(*canvas, bounds) {},
		targets: func(bounds) []Target { return nil },
	}
}

// graphsOf are the GRAPHS of doc at now of the account the frame's focus is
// on, or where it's on none of doc's, the first, named in bold text.primary
// as the Overview names it: each window's chart drawn from its trail, as the
// frame's Burn has it.
func (f Frame) graphsOf(doc status.Document, now time.Time) graphs {
	m := f.Moment(doc, now)
	names := overviewNames(m)
	g := graphs{across: layAcross(len(f.overviewWindows(doc, now)), f.Width, nameColumn(names, f.Width)), look: f.Look, burn: f.Burn}
	account, focused := m.Account(f.Focus)
	if !focused && len(m.Accounts) > 0 {
		account, focused = m.Accounts[0], true
	}
	if !focused {
		return g
	}
	g.name, g.focused = overviewNamed(m, names, account.ID, titleInk), true
	g.charts = f.chartsOf(doc, now, account)
	return g
}

// chartsOf are the charts of account a's windows the Overview shows, of doc
// at now, in USAGE's order, each drawn from its trail, as the frame's Burn
// has it.
func (f Frame) chartsOf(doc status.Document, now time.Time, a status.AccountNow) []chart {
	var charts []chart
	for _, w := range f.overviewWindows(doc, now) {
		charts = append(charts, chartOf(a, w, f.Trails[Ref{Account: a.ID, Window: w.key}], now, f.Burn))
	}
	return charts
}

// rows is how many rows GRAPHS takes with charts h rows tall: its heading
// and the blank row under it, then each row of charts, one under another: a
// chart's top row, its window's name and its limit's line; its h rows; and
// its axis row.
func (g graphs) rows(h int) int {
	return sectionHead + g.chartRows()*(h+chartEnds)
}

// chartRows are how many rows of charts GRAPHS lays out, as rowsFor has
// them.
func (g graphs) chartRows() int {
	return g.across.rowsFor(len(g.charts))
}

// draw draws each chart in the body's bounds, as placed places it.
func (g graphs) draw(c *canvas, body bounds) {
	for i, ch := range g.charts {
		ch.draw(c, g.look, g.placed(i, body))
	}
}

// placed is where the body puts chart i, as the charts across place it, the
// rows of charts each a chart's ends and leastChart to mostChart rows of
// chart, what's past their mosts blank.
func (g graphs) placed(i int, body bounds) bounds {
	return g.across.placed(i, body, g.chartRows(), flex.Part{Least: leastChart + chartEnds, Most: mostChart + chartEnds, Grows: true})
}

// chartsAcross is how GRAPHS lays its charts across: a row of them from
// column x, each where its span has it; where it stacks, the charts past the
// row's go on the rows under, in the same columns, else they're left off.
type chartsAcross struct {
	x      int
	spans  []flex.Span
	stacks bool
}

// shown is how many of k charts show: all where the charts stack, else as
// many as the row has.
func (a chartsAcross) shown(k int) int {
	if a.stacks {
		return k
	}
	return min(k, len(a.spans))
}

// rowsFor is how many rows of charts k charts take, as many to a row as the
// row has, one at the least.
func (a chartsAcross) rowsFor(k int) int {
	per := max(len(a.spans), 1)
	return max((a.shown(k)+per-1)/per, 1)
}

// placed is where chart i lies in area: across, in its column of the row,
// from area's x; down, in its row of charts, the n rows of them one under
// another, each as row has it, sharing what's past their leasts evenly, the
// odd row to the first.
func (a chartsAcross) placed(i int, area bounds, n int, row flex.Part) bounds {
	parts := make([]flex.Part, n)
	for j := range parts {
		parts[j] = row
	}
	down, _ := flex.Lay(area.rows, parts...)
	per := len(a.spans)
	at, in := a.spans[i%per], down[i/per]
	return bounds{x: area.x + a.x + at.At, y: area.y + in.At, width: at.Size, rows: in.Size}
}

// layAcross lays k windows' charts across a frame width columns wide, its name
// column names cells: side by side from the name column, under USAGE's bars,
// a chart every ⌊(w − 14 + 3) ÷ k⌋ − ⌈(n − 14) ÷ k⌉ columns, as USAGE's
// matrix has its groups, where each is leastChartCells or more; else from
// column 0, as many side by side as fit to the margin, as chartsFitting has
// them, the rest on the rows under; and in a frame too narrow for one, a
// chart a row, across it. Only the width and the windows shown decide it,
// never what the charts hold.
func layAcross(k, width, names int) chartsAcross {
	k = max(k, 1)
	every := (width-matrixNames+chartGap)/k - (names-matrixNames+k-1)/k
	if spans, ok := chartRow(k, k*every-chartGap, chartGap, leastChartCells); ok {
		return chartsAcross{x: names, spans: spans, stacks: true}
	}
	room := width - margin
	if spans := chartsFitting(k, room, chartGap, leastChartCells); len(spans) > 0 {
		return chartsAcross{spans: spans, stacks: true}
	}
	return chartsAcross{spans: []flex.Span{{Size: room}}, stacks: true}
}

// chartsFitting lays as many of k charts as fit side by side in room cells,
// gap cells apart, each least cells or more, as chartRow lays them out: none
// where not one does.
func chartsFitting(k, room, gap, least int) []flex.Span {
	for n := k; n > 0; n-- {
		if spans, ok := chartRow(n, room, gap, least); ok {
			return spans
		}
	}
	return nil
}

// chartRow lays n charts side by side in room cells, gap cells apart, each as
// wide as the others, ⌊(room + gap) ÷ n⌋ − gap, the odd cells left at the
// end, and reports whether each is least cells or more.
func chartRow(n, room, gap, least int) ([]flex.Span, bool) {
	each := (room+gap)/n - gap
	parts := make([]flex.Part, n)
	for i := range parts {
		parts[i] = flex.Part{Least: least, Natural: each, Gap: gap}
	}
	return flex.Lay(room, parts...)
}

// description is GRAPHS' heading's description: the focused account, then,
// five cells on, the key to the charts' lines, as burnKey has it, each an
// item, as its heading gives them way.
func (g graphs) description() line {
	if !g.focused {
		return nil
	}
	return slices.Concat(line{g.name, spaces(5)}, burnKey(g.burn))
}

// burnKey is the key to the lines of the charts burn names, as GRAPHS'
// heading has it, its items three cells apart, its words in text.faint: of
// the burn-up, even pace's ·, the cap's ╌ and the limit's ┄, along its top;
// of the burn-down, where it's heading, ⠢, where it runs out, ✕, the cap,
// ⠤, and full, the ┄ along its top.
func burnKey(burn Burn) line {
	items := []line{
		{{paceDot, dimInk}, {" even pace", faintInk}}, {{capLine, reserveInk}, {" cap", faintInk}}, {{limitLine + " limit", faintInk}},
	}
	if burn == BurnDown {
		items = []line{
			{{headingKey, dimInk}, {" heading", faintInk}}, {{outMark, outInk}, {" runs out", faintInk}},
			{{capKey, capDotInk}, {" cap", faintInk}}, {{limitLine + " full", faintInk}},
		}
	}
	return joined(span{"   ", faintInk}, items...)
}

// folded is what GRAPHS holds, as its heading says it folded: the focused
// account.
func (g graphs) folded() line {
	if !g.focused {
		return nil
	}
	return line{g.name}
}
