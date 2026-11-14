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
var bucketWord = map[views.BucketUnit]string{views.ByHour: "hour", views.ByDay: "day", views.ByWeek: "week", views.ByMonth: "month"}

// chartSlot is where a bucket's column lies across a chart: from x, width
// cells wide.
type chartSlot struct {
	x, width int
}

// chartSlots are where the buckets' columns lie across a chart width cells
// wide, each as wide as the rest, and as far apart, the spare at the right:
// two cells apart where each is four cells or more, else a cell apart where
// each has one, else touching, as flex fits them.
func chartSlots(width, buckets int) []chartSlot {
	if buckets == 0 {
		return nil
	}
	apart := 0
	for _, try := range []struct{ apart, least int }{{2, 4}, {1, 1}} {
		if _, fits := flex.Lay(width, chartPart(buckets, try.least, try.apart)); fits {
			apart = try.apart
			break
		}
	}
	step := (width + apart) / buckets
	slots := make([]chartSlot, buckets)
	for i := range slots {
		slots[i] = chartSlot{x: i * step, width: max(step-apart, 1)}
	}
	return slots
}

// worthChart draws the worth by's columns from x, from row top down rows
// rows, width cells wide, as chartSlots lays them, each as tall against
// rows as its worth is against top, as worthColumns draws them; and under
// them their axis, as chartAxis draws it of bucketNames'.
func worthChart(c *canvas, look Look, x, top, width, rows int, by views.WorthBy, against ledger.Picodollars) {
	slots := chartSlots(width, len(by.Buckets))
	worthColumns(c, look, x, top+rows-1, rows, slots, by, against)
	chartAxis(c, x, top+rows, width, slots, bucketNames(by))
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
