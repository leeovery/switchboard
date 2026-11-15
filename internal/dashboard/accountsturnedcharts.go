package dashboard

// Turned Compare's WEEKS and WORTH: a band a column, as Detail's all
// accounts draws its weeks, each column's figures Compare's own.

// turnedWeeks draws WEEKS from row y, and returns its last row: its
// heading, as Compare's names it; then, a blank row under it, a band each
// column, back to back, as bandsLayout draws them, from the names' column,
// their columns where the bars start, their facts ending at the frame's
// edge: each its peaks in the reach's columns and Weeks' facts on them, the
// cap's level along it where its account keeps a reserve; and on the first
// band's peaks' row, where the reach's columns don't all show, how many
// don't.
func (cp compared) turnedWeeks(c *canvas, y int, l turnedLayout, columns []compareColumn) int {
	cp.weeksHead(c, y, y+partsApart)
	b := bandsIn(turnedNameAt, l.at-turnedNameAt, cp.f.edge())
	top := y + partsApart
	for i, col := range columns {
		band := weekBand{
			name:  turnedName(col, l.names),
			peaks: col.row.Reached,
			facts: col.row.Reaching,
			capAt: capKept(col.account, !col.all && col.known),
		}
		b.draw(c, cp.f.Look, top+i*bandHeight, band)
	}
	b.more(c, top+bandRows, len(cp.page.Reach))
	return top + len(columns)*bandHeight - 1
}

// turnedWorth draws WORTH from row y, and returns its last row: its
// heading, naming its bucket; then a band each column, a blank row before
// each: its name, and from where the bars start to the frame's edge, its
// worth over the period in the finest bucket that fits there, worthChartRows
// tall, as worthChart draws it with its axis, the accounts on one scale and
// all accounts on its own, as worthScale has them; and under the axis its
// peak, in the fullest of peakForms' forms that fits.
func (cp compared) turnedWorth(c *canvas, y int, l turnedLayout, columns []compareColumn) int {
	cells := cp.f.edge() - l.at
	fits := chartFits(cells)
	cp.worthHead(c, y, y+partsApart, fits)
	scale := cp.worthScale(fits)
	for _, col := range columns {
		y += partsApart
		by, against := scale.of(col)
		c.line(turnedNameAt, y, turnedName(col, l.names))
		worthChart(c, cp.f.Look, l.at, y, cells, worthChartRows, by, against)
		y += worthChartRows + 1
		c.line(l.at, y, fitParts(cells, peakForms(by, col.all).fitting(cells)...))
	}
	return y
}
