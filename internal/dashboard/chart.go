package dashboard

import (
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
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

// The lines a GRAPHS chart draws: the limit's, along its top row; and, on its
// empty cells, its account's cap, even pace, and now.
const (
	limitLine = "┄"
	capLine   = "╌"
	paceDot   = "·"
	nowLine   = "│"
)

// chartFade is how far a chart's fill fades into the canvas: 70% of its
// colour shows.
const chartFade = 0.3

// untracedLabel marks a chart of a window with no history.
const untracedLabel = "no history yet"

// chart is a window of an account as GRAPHS charts it at now, in the chart
// burn names: its name, in capitals; when it started and how long it runs
// until it resets, spanned where that's known; the readings of its use since
// it started, the oldest first; where the account has a cap, capped, the
// share of the window it sits at; and where the window's heading, as the
// burn-down draws it ahead of now.
type chart struct {
	name     string
	start    time.Time
	length   time.Duration
	spanned  bool
	readings []score.Reading
	capAt    float64
	capped   bool
	now      time.Time
	burn     Burn
	course   course
}

// course is where a window is heading from now, as the burn-down draws it:
// as its projection has it, the one Runway's words go by, out, when it runs
// out before it resets, zero where it doesn't, on its floor, its account's
// cap where atCap is set, else empty; else left, the room it has left at its
// reset, where its projection keeps it short of running out, nil where
// nothing says; and while Claude's limit holds it, held, from since, zero
// where that isn't known, till back, when it lifts, zero where that isn't
// known.
type course struct {
	out         time.Time
	atCap       bool
	left        *float64
	held        bool
	since, back time.Time
}

// courseOf is where the window w is heading at its moment, as course has
// it, as the moment judged it: held, while a limit holds it; running out at
// its Out, where it runs out; else, while it's open, to the room its
// projection leaves it at its reset. A window at its cap, stale, not
// started, or nothing known of, is heading nowhere the chart draws.
func courseOf(w status.WindowNow) course {
	switch {
	case w.Held:
		return course{held: true, since: w.Since, back: w.Back}
	case w.RunsOut:
		return course{out: w.Out.At, atCap: w.Out.Reserve}
	case w.Kind == status.WindowOpen && w.Projected != nil:
		return course{left: new(1 - *w.Projected)}
	}
	return course{}
}

// ChartSpan is the span GRAPHS charts the window w over, as it stands at its
// moment, as its Start and Length have it, so from its last reset where that
// has passed since it was read, until its next; reporting false where that
// isn't known, as of one not started, which isn't running, as the router's
// next document has it, with no reset. A reading from before it starts isn't
// charted, so what keeps a window's trail for the charts keeps none of those.
func ChartSpan(w status.WindowNow) (start time.Time, length time.Duration, ok bool) {
	return w.Start, w.Length, w.Length > 0
}

// chartOf is account a's window shown, charted as burn names over its span
// at now, as ChartSpan has it, from its trail: none of its readings from
// before it started, so none where it has reset since it was read; and
// nothing but its name where a has no such window, or its span isn't known,
// as of one not started. Where it's heading is as courseOf has it.
func chartOf(a status.AccountNow, shown shownWindow, trail Trail, now time.Time, burn Burn) chart {
	ch := chart{name: strings.ToUpper(shown.name), capAt: a.Cap, capped: a.Reserve > 0, now: now, burn: burn}
	w, _ := a.Window(shown.key)
	if ch.start, ch.length, ch.spanned = ChartSpan(w); ch.spanned {
		ch.readings = slices.DeleteFunc(slices.Clone(trail.Readings), func(r score.Reading) bool { return r.At.Before(ch.start) })
		ch.course = courseOf(w)
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

// draw draws the chart in the bounds given: its top row, its window's name
// and the limit's line along the rest of it; its rows, all but the top row
// and the axis row, a column a cell over the window's whole length, as its
// burn-up or its burn-down; and its axis. A window with no history says so
// across the chart's middle row, over what it crosses; one whose span isn't
// known has its top row alone, and that word.
func (ch chart) draw(c *canvas, look Look, at bounds) {
	chartTop(c, at, ch.name)
	h := at.rows - 2
	if h < 1 || at.width < 1 {
		return
	}
	if ch.spanned {
		p := plot{x: at.x, floor: at.y + h, width: at.width, h: h, now: ch.nowColumn(at.width)}
		if ch.burn == BurnDown {
			ch.burnDown(c, look, p)
		} else {
			ch.burnUp(c, look, p)
		}
		ch.axis(c, p.x, p.floor+1, p.width, p.now)
	}
	if len(ch.readings) == 0 {
		across(c, at.x, at.y+1+h/2, at.width, untracedLabel)
	}
}

// burnUp draws the chart's burn-up on p: each column to now filled to the
// use read at its time, as readAt has it, and none past now; then its lines
// on the cells the fill leaves empty.
func (ch chart) burnUp(c *canvas, look Look, p plot) {
	for col := range p.now + 1 {
		if used, ok := ch.usedAt(ch.readAt(col, p.width)); ok {
			fillColumn(c, look, p.x+col, p.floor, p.h, halfEven(min(max(used, 0), 1)*float64(8*p.h)))
		}
	}
	ch.lines(c, p.x, p.floor, p.width, p.h, p.now)
}

// chartTop draws a chart's top row in the bounds given: its window's name,
// as given, in text.subtle, then a space and the limit's line, faint, to
// the chart's right.
func chartTop(c *canvas, at bounds, name string) {
	end := c.text(at.x, at.y, prose.Cut(name, at.width), dimInk)
	if rest := at.x + at.width - end - 1; rest > 0 {
		c.text(end, at.y, " "+strings.Repeat(limitLine, rest), faintInk)
	}
}

// fillColumn fills a chart's column at x, h rows tall up from row floor,
// eighths eighths of a cell, as fillUp fills it: each row in the ramp's
// colour at its height, (r + 0.5) ÷ h for row r from the floor, chartFade
// into the canvas.
func fillColumn(c *canvas, look Look, x, floor, h, eighths int) {
	fillUp(c, look, x, floor, eighths, func(r int) ink {
		return ink{ramp: true, at: (float64(r) + 0.5) / float64(h), fade: chartFade}
	})
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

// lines draws the chart's lines on the cells of its h rows, width cells from
// x up from row floor, that the fill leaves empty, each in turn, so where they
// meet, the first drawn shows: its account's cap, a row of ╌ min(⌊cap × h⌋,
// h − 1) rows up; even pace, a · in each column i, min(⌊i ÷ (width − 1) ×
// h⌋, h − 1) rows up; and now, │ down its column.
func (ch chart) lines(c *canvas, x, floor, width, h, now int) {
	mark := func(col, r int, glyph string, k ink) {
		if here := c.at(x+col, floor-r); here != nil && *here == blank {
			c.text(x+col, floor-r, glyph, k)
		}
	}
	if ch.capped {
		row := min(int(max(ch.capAt, 0)*float64(h)), h-1)
		for col := range width {
			mark(col, row, capLine, reserveInk)
		}
	}
	for col := range width {
		mark(col, min(col*h/max(width-1, 1), h-1), paceDot, dimInk)
	}
	for r := range h {
		mark(now, r, nowLine, faintInk)
	}
}

// axis draws the chart's axis width cells from x along row y: the window's
// start, faint, at its left; its reset, faint, ending at its right, each a
// time of day for a window of a day or less, else its day too; and now, in
// bold, from the cell before its column, where it clears both.
func (ch chart) axis(c *canvas, x, y, width, now int) {
	when := prose.TimeOfDay
	if ch.length > quota.Day {
		when = prose.Clock
	}
	after := c.text(x, y, when(ch.now, ch.start), faintInk)
	reset := c.right(x+width, y, line{{when(ch.now, ch.start.Add(ch.length)), faintInk}})
	if from := x + now - 1; from > after && from+len("now") < reset {
		c.text(from, y, "now", labelInk)
	}
}

// across draws text in text.subtle across a chart, centred in width cells
// from x along row y, cut to fit, a blank either side to set it off what it's
// drawn over.
func across(c *canvas, x, y, width int, text string) {
	text = " " + prose.Cut(text, width-2) + " "
	c.text(x+(width-ansi.StringWidth(text))/2, y, text, dimInk)
}

// dayCells is how many cells a day's tick and name take on an axis, with the
// gap after them.
const dayCells = 5

// midnight is a midnight along an axis of days: when it is, the column it's
// in, whether its day is today, and whether its name shows.
type midnight struct {
	at           time.Time
	col          int
	today, named bool
}

// midnights are the midnights after start and before end, in now's time
// zone, each the start of its day, as dayStarts gives it, in the column
// column puts it in, of those before width, its name showing where it's
// dayCells clear of the last shown, and what shows it, cells wide from its
// column, fits within width.
func midnights(start, end, now time.Time, column func(time.Time) int, width, cells int) []midnight {
	var all []midnight
	last := -dayCells
	for at := range dayStarts(start.In(now.Location()), 1) {
		col := column(at)
		switch {
		case !at.Before(end) || col >= width:
			return all
		case col < 0:
			continue
		}
		m := midnight{at: at, col: col, today: prose.SameDay(now, at, now), named: col-last >= dayCells && col+cells <= width}
		if m.named {
			last = col
		}
		all = append(all, m)
	}
	return all
}

// dayStarts are the starts of the days in t's time zone, as
// dayfile.DayStart gives them, from the one days after the day t falls on,
// one after another, for as long as they're asked for.
func dayStarts(t time.Time, days int) iter.Seq[time.Time] {
	return func(yield func(time.Time) bool) {
		for at := dayfile.DayStart(t, days); yield(at); at = dayfile.DayStart(at, 1) {
		}
	}
}
