package dashboard

import (
	"fmt"
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

// graphs are the Overview's GRAPHS: the focused account's use so far, a chart
// a window that USAGE shows, side by side from USAGE's name column, names
// cells, or stacked on a phone; and the terminal's height, which its folded
// heading names.
type graphs struct {
	account status.Account
	focused bool
	charts  []chart
	names   int
	phone   bool
	look    Look
	height  int
}

// graphsSection is the Overview's GRAPHS of doc at now: the focused
// account's use so far, a chart a window, leastChart rows tall at its least
// and mostChart at its most.
func (f Frame) graphsSection(doc status.Document, now time.Time) section {
	g := f.graphsOf(doc, now)
	return section{
		Section: SectionGraphs, least: g.rows(leastChart), most: g.rows(mostChart),
		description: g.description,
		draw:        g.draw,
		folded:      g.folded,
	}
}

// graphsOf are the GRAPHS of doc at now of the account the frame's focus is
// on, or where it's on none of doc's, the first: by window or by account
// alike, each window's chart drawn from its trail.
func (f Frame) graphsOf(doc status.Document, now time.Time) graphs {
	g := graphs{names: nameColumn(namesOf(doc), f.Width), phone: f.phone(), look: f.Look, height: f.Height}
	g.account, g.focused = doc.Account(f.Focus)
	if !g.focused && len(doc.Accounts) > 0 {
		g.account, g.focused = doc.Accounts[0], true
	}
	if !g.focused {
		return g
	}
	for _, w := range usageWindows(doc, nil) {
		g.charts = append(g.charts, chartOf(g.account, w, f.Trails[Ref{Account: g.account.ID, Window: w.key}], now))
	}
	return g
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

// description is GRAPHS' heading's description: the focused account, then
// the key to the charts' lines.
func (g graphs) description() line {
	if !g.focused {
		return nil
	}
	return line{
		{name(g.account), titleInk}, spaces(5),
		{paceDot, dimInk}, {" even pace   ", faintInk}, {capLine, reserveInk}, {" cap   " + limitLine + " limit", faintInk},
	}
}

// folded is what GRAPHS' heading says folded: the focused account, and that
// the terminal's rows have no room for its charts.
func (g graphs) folded() line {
	if !g.focused {
		return nil
	}
	return line{{name(g.account), titleInk}, spaces(3), {fmt.Sprintf("no room at %d rows", g.height), dimInk}}
}
