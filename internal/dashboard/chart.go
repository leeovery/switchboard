package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// levels fill a cell of a chart by eighths, from its floor up.
var levels = [...]string{"", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// filledCell is a cell of a column filled eighths of it, from 1 to 8, in the
// fill's ink, as look draws it: full, a blank on the fill's colour where it
// blends, else █; else its eighth block, on the canvas.
func filledCell(look Look, eighths int, fill ink) (string, ink) {
	if eighths == 8 && look.blends() {
		return " ", ink{on: fill.hue()}
	}
	return levels[eighths], fill
}

// nowLine is now's line down a chart, on its empty cells.
const nowLine = "│"

// chart is a window of an account as Graphs charts it at now: when it
// started and how long it runs until it resets, spanned where that's known;
// and the readings of its use since it started, the oldest first.
type chart struct {
	start    time.Time
	length   time.Duration
	spanned  bool
	readings []score.Reading
	now      time.Time
}

// ChartSpan is the span Graphs charts the window w over, as it stands at its
// moment, as its Start and Length have it, so from its last reset where that
// has passed since it was read, until its next; reporting false where that
// isn't known, as of one not started, which isn't running, as the router's
// next document has it, with no reset. A reading from before it starts isn't
// charted, so what keeps a window's trail for the charts keeps none of those.
func ChartSpan(w status.WindowNow) (start time.Time, length time.Duration, ok bool) {
	return w.Start, w.Length, w.Length > 0
}

// chartOf is account a's window with the given key, charted over its span at
// now, as ChartSpan has it, from its trail: none of its readings from before
// it started, so none where it has reset since it was read; and nothing
// where a has no such window, or its span isn't known, as of one not
// started.
func chartOf(a status.AccountNow, key string, trail Trail, now time.Time) chart {
	ch := chart{now: now}
	w, _ := a.Window(key)
	if ch.start, ch.length, ch.spanned = ChartSpan(w); ch.spanned {
		ch.readings = slices.DeleteFunc(slices.Clone(trail.Readings), func(r score.Reading) bool { return r.At.Before(ch.start) })
	}
	return ch
}

// usedAt is the use the last reading at or before t read, reporting false
// where none was read by then.
func (ch chart) usedAt(t time.Time) (float64, bool) {
	read, _ := slices.BinarySearchFunc(ch.readings, t, func(r score.Reading, t time.Time) int {
		if r.At.After(t) {
			return 1
		}
		return -1
	})
	if read == 0 {
		return 0, false
	}
	return ch.readings[read-1].Utilization, true
}

// nowColumn is the column now falls in on a chart width cells wide: min(round(
// pace × width), width − 1), its pace how much of the window has passed by
// now, moving with the clock, rather than the window's even pace, which
// stays as of when the document was read.
func (ch chart) nowColumn(width int) int {
	return min(halfEven(ch.pace()*float64(width)), width-1)
}

// pace is how much of the window has passed by now, from 0 to 1.
func (ch chart) pace() float64 {
	return min(max(ch.share(ch.now), 0), 1)
}

// share is how far through the window t is, from 0 at its start to 1 at its
// reset.
func (ch chart) share(t time.Time) float64 {
	return float64(t.Sub(ch.start)) / float64(ch.length)
}

// at is the time a share of the way through the window.
func (ch chart) at(share float64) time.Time {
	return ch.start.Add(time.Duration(share * float64(ch.length)))
}

// readAt is the time column col of a chart width cells wide is filled to the
// use at: its middle's, or now, for now's column.
func (ch chart) readAt(col, width int) time.Time {
	return score.Earlier(ch.start.Add(time.Duration((float64(col)+0.5)/float64(width)*float64(ch.length))), ch.now)
}

// plot is where a chart's rows are drawn: h rows up from row floor, width
// cells from column x, now falling in column now.
type plot struct {
	x, floor, width, h, now int
}

// fillUp fills a column at x up from row floor eighths eighths of a cell,
// row r from the floor in the ink tone gives it: a full cell and the top
// cell as filledCell draws them.
func fillUp(c *canvas, look Look, x, floor, eighths int, tone func(r int) ink) {
	for r := 0; 8*r < eighths; r++ {
		glyph, k := filledCell(look, min(eighths-8*r, 8), tone(r))
		c.text(x, floor-r, glyph, k)
	}
}

// dayCells is how many cells a day's tick and name take on an axis, with the
// gap after them.
const dayCells = 5
