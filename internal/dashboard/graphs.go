package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The rows a GRAPHS chart takes: leastChart at the least, and mostChart at
// the most, as taller distorts.
const (
	leastChart = 5
	mostChart  = 19
)

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
// a window that USAGE shows, side by side from USAGE's name column, names
// cells, or stacked on a phone, drawn as burn names.
type graphs struct {
	name    span
	focused bool
	charts  []chart
	names   int
	phone   bool
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
	g := graphs{names: nameColumn(names, f.Width), phone: f.phone(), look: f.Look, burn: f.Burn}
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
// and the blank row under it, then a chart's rows, or on a phone, each
// chart's, one under another: a chart's top row, its window's name and its
// limit's line; its h rows; and its axis row.
func (g graphs) rows(h int) int {
	charts := 1
	if g.phone {
		charts = max(len(g.charts), 1)
	}
	return sectionHead + charts*(h+2)
}

// draw draws each chart in the body's bounds, as placed places it.
func (g graphs) draw(c *canvas, body bounds) {
	for i, ch := range g.charts {
		ch.draw(c, g.look, g.placed(i, body))
	}
}

// placed is where the body puts chart i, its charts as tall as its rows
// give them, mostChart at most: side by side, from the name column n, a
// chart every gw = ⌊(w − 14 + 3) ÷ k⌋ − ⌈(n − 14) ÷ k⌉ columns for k
// windows, each gw − 3 wide; or on a phone, one under another, each from the
// name column to the edge.
func (g graphs) placed(i int, body bounds) bounds {
	k := len(g.charts)
	if g.phone {
		rows := min(body.rows/k, mostChart+2)
		return bounds{x: body.x + g.names, y: body.y + i*rows, width: body.width - margin - g.names, rows: rows}
	}
	every := (body.width-matrixNames+chartGap)/k - (g.names-matrixNames+k-1)/k
	return bounds{x: body.x + g.names + i*every, y: body.y, width: every - chartGap, rows: min(body.rows, mostChart+2)}
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
