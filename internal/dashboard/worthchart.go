package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// worthInk is the ink of a worth chart's columns, Projects' WORTH BY's and
// Compare's WORTH's, and ON ACCOUNTS' bars: accent.primary.
var worthInk = ink{token: theme.AccentPrimary}

// bucketWord names a chart's bucket, as its heading says it.
var bucketWord = map[views.BucketUnit]string{views.ByHour: "hour", views.ByTwoHours: "2 hours", views.ByDay: "day", views.ByWeek: "week", views.ByMonth: "month"}

// chartSlot is where a bucket's column lies across a chart: from x, width
// cells wide.
type chartSlot struct {
	x, width int
}

// chartLay lays a chart's columns across width cells, its axis's names
// given: chartSlots' slots, as namedSlots or compareLay choose them.
type chartLay func(width int, names []line) []chartSlot

// chartApart are how many cells apart a chart's columns stand across width
// cells: two where each is four cells or more, else one where each has one,
// else none, touching, as flex fits them.
func chartApart(width, buckets int) int {
	for _, try := range []struct{ apart, least int }{{2, 4}, {1, 1}} {
		if _, fits := flex.Lay(width, chartPart(buckets, try.least, try.apart)); fits {
			return try.apart
		}
	}
	return 0
}

// chartSlots are where the buckets' columns lie across a chart width cells
// wide, from its first cell to its last, as far apart as chartApart has
// them. Where they stand apart each is as wide as the rest and the spare
// widens the gaps; where they touch it widens the columns; either way spread
// evenly across the chart, none more than a cell wider than another.
func chartSlots(width, buckets int) []chartSlot {
	if buckets == 0 {
		return nil
	}
	apart := chartApart(width, buckets)
	slots := make([]chartSlot, buckets)
	if apart == 0 {
		for i := range slots {
			x := evenShare(i, width, buckets)
			slots[i] = chartSlot{x: x, width: max(evenShare(i+1, width, buckets)-x, 1)}
		}
		return slots
	}
	column := (width+apart)/buckets - apart
	spare, gaps := width-(buckets*(column+apart)-apart), max(buckets-1, 1)
	for i := range slots {
		slots[i] = chartSlot{x: i*(column+apart) + evenShare(i, spare, gaps), width: column}
	}
	return slots
}

// evenShare is how many of cells, spread evenly across parts, fall before part
// i, rounded to the nearest, as Bresenham steps a line: none before the
// first, all of them before the one past the last.
func evenShare(i, cells, parts int) int {
	return (i*cells + parts/2) / parts
}

// namedSlots are chartSlots' slots across the widest chart, of width cells
// down to a cell a column, whose last column's name, from its first cell,
// ends within width, so the chart leaves room for it. Where none does, the
// chart takes the whole width, and chartAxis drops the name.
func namedSlots(width int, names []line) []chartSlot {
	buckets := len(names)
	for cells := width; cells >= buckets && buckets > 0; cells-- {
		slots := chartSlots(cells, buckets)
		if slots[buckets-1].x+names[buckets-1].width() <= width {
			return slots
		}
	}
	return chartSlots(width, buckets)
}

// compareSlots are Compare's charts' slots across a column's head, width
// cells: whole steps, each column as wide as the rest and as far apart, as
// chartApart has them, the spare at the right. In Compare the spare is the
// gutter between charts side by side.
func compareSlots(width, buckets int) []chartSlot {
	if buckets == 0 {
		return nil
	}
	apart := chartApart(width, buckets)
	return chartSlots((width+apart)/buckets*buckets-apart, buckets)
}

// compareLay is compareSlots as a chartLay, for Compare's worth charts.
func compareLay(width int, names []line) []chartSlot {
	return compareSlots(width, len(names))
}

// worthChart draws the worth by's columns from x, from row top down rows
// rows, width cells wide: laid as lay lays them, given the names bucketNames
// has for the axis, each as tall against rows as its worth is against top,
// as worthColumns draws them; and under them their axis, as chartAxis draws
// it.
func worthChart(c *canvas, look Look, x, top, width, rows int, lay chartLay, by views.WorthBy, against ledger.Picodollars) {
	names := bucketNames(by)
	slots := lay(width, names)
	worthColumns(c, look, x, top+rows-1, rows, slots, by, against)
	chartAxis(c, x, top+rows, width, slots, names)
}

// worthColumns draws each bucket's column up from row floor, rows tall at
// most, as slots lay them from x, in accent.primary, as tall against rows
// as its worth is against top, in eighths of a cell, a half to the even, at
// least one where it's worth anything: its full cells and its top's eighths
// as fillUp draws them.
func worthColumns(c *canvas, look Look, x, floor, rows int, slots []chartSlot, by views.WorthBy, top ledger.Picodollars) {
	for i, b := range by.Buckets {
		eighths := barUnits(amountOf(b.Worth), top, rows*8)
		for col := range slots[i].width {
			fillUp(c, look, x+slots[i].x+col, floor, eighths, func(int) ink { return worthInk })
		}
	}
}

// peakWorth is what the worth by's peak bucket was worth: none where none
// was worth anything.
func peakWorth(by views.WorthBy) ledger.Picodollars {
	if by.Peak < 0 {
		return 0
	}
	return amountOf(by.Buckets[by.Peak].Worth)
}

// bucketNames are the worth by's buckets' names on its axis, as each one's
// Axis gives it, in text.subtle, now's in bold text.secondary, as Graphs'
// axes ink today's.
func bucketNames(by views.WorthBy) []line {
	names := make([]line, len(by.Buckets))
	for i, b := range by.Buckets {
		names[i] = axisName(b.Axis, i == by.Now())
	}
	return names
}

// axisName is a column's name on a chart's axis, in text.subtle, or now's
// in bold text.secondary: nothing where it has none.
func axisName(named string, now bool) line {
	switch {
	case named == "":
		return nil
	case now:
		return line{{named, strongInk}}
	}
	return line{{named, dimInk}}
}

// chartAxis draws a chart's axis along row y, width cells from x: each
// column's name, as names has them, from its column's first cell, as slots
// lay them; but of each whose name wouldn't end a cell before the next
// drawn, or within the chart, nothing, the names from the last back giving
// way.
func chartAxis(c *canvas, x, y, width int, slots []chartSlot, names []line) {
	next := width + 1
	for i, named := range slices.Backward(names) {
		if named.width() == 0 || slots[i].x+named.width() >= next {
			continue
		}
		c.line(x+slots[i].x, y, named)
		next = slots[i].x
	}
}

// worthPeak is the line under a worth chart, as peakParts has it, whole.
func worthPeak(by views.WorthBy) line {
	named, worth, ok := peakParts(by)
	if !ok {
		return nil
	}
	return partsSaid([]part{named, worth})
}

// peakParts are the parts of the line under a worth chart: peak, its
// bucket, as PeakNamed names it, in text.muted, then what it was worth, as
// Rounded says it, bold text.primary, a figure; reporting false where
// nothing was worth anything.
func peakParts(by views.WorthBy) (named, worth part, ok bool) {
	said := by.PeakNamed()
	if said == "" {
		return part{}, part{}, false
	}
	return wordsIn("peak "+said+" ", mutedInk), figureIn(by.Buckets[by.Peak].Rounded(), titleInk), true
}
