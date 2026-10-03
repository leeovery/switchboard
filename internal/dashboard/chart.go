package dashboard

import (
	"math"
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/theme"
)

// levels fill a cell of a chart by eighths, from its floor up.
var levels = [...]string{"", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

const (
	// floorLine is drawn along a chart's floor where its window has nothing
	// left, and outMark marks where its window runs out.
	floorLine = "▁"
	outMark   = "✕"
	// nowLine marks a chart's column for now.
	nowLine = "│"
	// reserveFade draws a reserve's floor faint.
	reserveFade = 0.4
)

var (
	// outInk is a chart's ✕ where its window runs out.
	outInk = ink{token: theme.StateDestructive, bold: true}
	// faintLevelInk is a chart's level where it's dim: a lapsed window's,
	// or one drawn without history.
	faintLevelInk = ink{token: theme.TextSubtle, fade: projectionFade}
)

// burndown is a window's chart on a card: the room left in it over its
// length, falling toward the floor as it's used, and where it's heading.
type burndown struct {
	standing
	// start and length are when the window started, and how long it runs
	// until it resets: its span, spanned set where that's known.
	start   time.Time
	length  time.Duration
	spanned bool
	// trail is the window's readings history, traced set where there is one.
	trail  Trail
	traced bool
	// tone is the colour of the account's state, which the chart is drawn in.
	tone theme.Token
	// starts says when a lapsed window starts again, as in "window starts at
	// its prime, 16:20".
	starts string
	now    time.Time
}

// elapsed is how much of the window has passed at now, from 0 to 1.
func (b burndown) elapsed() float64 {
	return min(max(float64(b.now.Sub(b.start))/float64(b.length), 0), 1)
}

// nowColumn is the column of a chart width cells wide just past the one now
// falls in, which the line for now takes, and what's ahead starts in.
func (b burndown) nowColumn(width int) int {
	return min(int(b.elapsed()*float64(width)), width-1) + 1
}

// usedAt is how much of the window its last reading at or before t had used,
// reporting false where it has none; or, without history, how much is used
// now.
func (b burndown) usedAt(t time.Time) (float64, bool) {
	if !b.traced {
		return b.window.Utilization, true
	}
	i, found := slices.BinarySearchFunc(b.trail.Readings, t, func(r score.Reading, t time.Time) int { return r.At.Compare(t) })
	if !found {
		i--
	}
	if i < 0 {
		return 0, false
	}
	return b.trail.Readings[i].Utilization, true
}

// chart draws the burn-down width cells wide and rows tall from x along row
// y. The past is a level in eighths of a cell, a column for each stretch of
// the window up to now, filled to the room left at the middle of that
// stretch, as roomAt has it, in the state's colour faded halfway: without
// history, the room now, dim, marked "no history yet". Where nothing was
// left, a line runs along the floor in destructive, and on until the limit
// lifts while the window's held at it. Now is a thin line in the border's
// colour; where the window's heading, a dotted line from the level now, to
// ✕ on the floor where it runs out, else to the room it has left at its
// reset. The floor is where the account's reserve starts, where that holds
// it back, drawn as a faint dotted line, else its limit. The marks, now's
// line and what's ahead, are drawn last, over what's under them. A lapsed
// window is a full level, dim, saying when it starts; and one whose reset
// isn't known has no length to draw it over.
func (f Frame) chart(c *canvas, b burndown, x, y, width, rows int) {
	switch {
	case width < 1 || rows < 1:
		return
	case b.lapsed:
		for col := range width {
			level(c, x+col, y, rows, 1, faintLevelInk)
		}
		across(c, x, y+rows/2, width, "full · "+b.starts)
		return
	case !b.spanned:
		return
	}
	past := ink{token: b.tone, fade: projectionFade}
	if !b.traced {
		past = faintLevelInk
	}
	ahead := b.nowColumn(width)
	for col := range ahead {
		stretch := (float64(col) + 0.5) / float64(width)
		if room, ok := b.roomAt(earliest(b.at(stretch), b.now)); ok {
			level(c, x+col, y, rows, room, past)
		}
	}
	if !b.traced {
		untraced(c, x, y, width, rows, ahead)
	}
	if ahead < width {
		for row := range rows {
			c.text(x+ahead, y+row, nowLine, borderInk)
		}
	}
	f.ahead(c, b, x, y, width, rows, ahead)
}

// untracedLabel marks a chart drawn without history.
const untracedLabel = "no history yet"

// untraced marks a chart width cells wide and rows tall from x along row y,
// drawn without history, its column for now ahead: across its level, from
// its left to now, where the words fit there clear of its marks, else
// across the whole of it, under the marks drawn after.
func untraced(c *canvas, x, y, width, rows, ahead int) {
	if ahead < ansi.StringWidth(untracedLabel)+2 {
		ahead = width
	}
	across(c, x, y+rows/2, ahead, untracedLabel)
}

// roomAt is the room the window had left at t, as its last reading at or
// before t left it, reporting false where it has none, or, without history,
// the room it has now; and none from when it was held at its limit, where
// that's known.
func (b burndown) roomAt(t time.Time) (float64, bool) {
	if b.held && !b.since.IsZero() && !t.Before(b.since) {
		return 0, true
	}
	used, ok := b.usedAt(t)
	return 1 - used, ok
}

// at is the time a share of the way through the window.
func (b burndown) at(share float64) time.Time {
	return b.start.Add(time.Duration(share * float64(b.length)))
}

// fraction is how far through the window t is, from 0 at its start.
func (b burndown) fraction(t time.Time) float64 {
	return float64(t.Sub(b.start)) / float64(b.length)
}

// ahead draws what's ahead of a chart from its column ahead: while its
// window's held at its limit, a line along the floor until the limit lifts;
// else where it's heading, as a dotted line, to ✕ on its floor where it runs
// out, and its account's reserve, where that's its floor, as a faint dotted
// line along it.
func (f Frame) ahead(c *canvas, b burndown, x, y, width, rows, ahead int) {
	if b.held {
		for col := ahead; col < width && (b.back.IsZero() || b.at(float64(col)/float64(width)).Before(b.back)); col++ {
			c.text(x+col, y+rows-1, floorLine, errorInk)
		}
		return
	}
	plot := newDots(rows)
	if b.floor < 1 {
		for col := ahead + 2 - ahead%2; col < width; col += 2 {
			plot.dot(2*col+1, plot.rowOf(1-b.floor), ink{token: theme.VizReserve, fade: reserveFade}, 0)
		}
	}
	end, room := b.headingTo(width)
	if b.runsOut {
		end = max(end, ahead)
	}
	from := 1 - b.window.Utilization
	for col := ahead; col < end; col++ {
		at := (float64(col) + 0.25) / float64(width)
		plot.dot(2*col, plot.rowOf(from+(room-from)*b.progress(at)), ink{token: b.tone}, 1)
	}
	plot.draw(c, x, y)
	if b.runsOut && end < width {
		c.text(x+end, y+plot.rowOf(1-b.floor)/4, outMark, outInk)
	}
}

// headingTo is where a chart width cells wide draws its window heading to:
// the column it runs out in, and the room on its floor; else its last, and
// the room it has left at its reset; or nowhere, where nothing says, or its
// use has reached its floor already.
func (b burndown) headingTo(width int) (int, float64) {
	share, ok := b.projected()
	switch {
	case b.runsOut:
		return min(int(b.fraction(b.out.At)*float64(width)), width), 1 - b.floor
	case !ok || b.window.Utilization >= b.floor-score.Tolerance:
		return 0, 0
	default:
		return width, 1 - share
	}
}

// progress is how far a share of the way through the window is from now
// toward where the window is heading: 0 now, 1 there.
func (b burndown) progress(share float64) float64 {
	end := 1.0
	if b.runsOut {
		end = b.fraction(b.out.At)
	}
	if end <= b.elapsed() {
		return 1
	}
	return min(max((share-b.elapsed())/(end-b.elapsed()), 0), 1)
}

// level draws a chart's column at x, rows tall from row y, filled from its
// floor to room, a share of the whole, in eighths of a cell, in the ink k;
// where nothing's left, a line along the floor in destructive.
func level(c *canvas, x, y, rows int, room float64, k ink) {
	filled := int(math.Round(min(max(room, 0), 1) * float64(8*rows)))
	if filled == 0 {
		c.text(x, y+rows-1, floorLine, errorInk)
		return
	}
	for row := range rows {
		if eighths := min(filled-8*row, 8); eighths > 0 {
			c.text(x, y+rows-1-row, levels[eighths], k)
		}
	}
}

// across draws text dim across a chart, centred in width cells from x along
// row y, cut to fit, a blank either side to set it off what it's drawn
// over.
func across(c *canvas, x, y, width int, text string) {
	text = " " + truncate(text, width-2) + " "
	c.text(x+(width-ansi.StringWidth(text))/2, y, text, dimInk)
}

// earliest is the earlier of two times.
func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// axis draws a chart's axis width cells wide from x along row y: for a
// window of a day or less, its start, now, under the chart's line for it
// where it clears both, and its reset; for a longer one, its days, each from
// the midnight it starts at, where its name fits, today's picked out. A
// lapsed window has none.
func (f Frame) axis(c *canvas, b burndown, x, y, width int) {
	switch {
	case b.lapsed || !b.spanned || width < 1:
		return
	case b.length > day:
		days(c, b, x, y, width)
		return
	}
	end := b.start.Add(b.length)
	c.text(x, y, b.start.In(b.now.Location()).Format("15:04"), dimInk)
	c.right(x+width, y, line{{end.In(b.now.Location()).Format("15:04"), mutedInk}})
	if ahead := b.nowColumn(width); ahead-2 >= clockCells+axisGap && ahead+1+axisGap <= width-clockCells {
		c.text(x+ahead-2, y, "now", strongInk)
	}
}

// clockCells is how many cells a time of day takes on an axis, and axisGap
// the fewest blank cells between it and now.
const (
	clockCells = 5
	axisGap    = 2
)

// days draws a week's days along its axis, width cells from x along row y:
// a tick at each midnight within it, and the day's name after, where it
// clears the day before's and fits, today's picked out.
func days(c *canvas, b burndown, x, y, width int) {
	loc := b.now.Location()
	first := b.start.In(loc)
	midnight := time.Date(first.Year(), first.Month(), first.Day()+1, 0, 0, 0, 0, loc)
	today := b.now.In(loc).Format(time.DateOnly)
	last := -dayCells
	for ; midnight.Before(b.start.Add(b.length)); midnight = midnight.AddDate(0, 0, 1) {
		col := int(math.Round(b.fraction(midnight) * float64(width-1)))
		if col-last < dayCells || col+dayCells-1 > width {
			continue
		}
		name := ink{token: theme.TextSubtle}
		if midnight.Format(time.DateOnly) == today {
			name = strongInk
		}
		c.line(x+col, y, line{{"╵", faintInk}, {midnight.Format("Mon"), name}})
		last = col
	}
}

// dayCells is how many cells a day's tick and name take on an axis, with the
// gap after them.
const dayCells = 5

// dots is braille drawn over a chart rows tall: each cell's dots, two
// across and four down, in the ink of the most telling line through it.
type dots struct {
	rows  int
	cells map[[2]int]brailleCell
}

// brailleCell is a cell of braille: its dots, and the ink and rank of the most
// telling line through it.
type brailleCell struct {
	bits rune
	ink  ink
	rank int
}

// dotBits are a braille cell's dots, by column and row.
var dotBits = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// newDots is braille over a chart rows tall, no dot drawn.
func newDots(rows int) dots {
	return dots{rows: rows, cells: make(map[[2]int]brailleCell)}
}

// rowOf is the row of dots for room, a share of the chart from its floor:
// the top row for all of it, the bottom for none.
func (d dots) rowOf(room float64) int {
	return int(math.Round((1 - min(max(room, 0), 1)) * float64(4*d.rows-1)))
}

// dot draws a dot at column px and row py of dots, in the ink k where rank
// is at least that of what's been drawn in its cell already.
func (d dots) dot(px, py int, k ink, rank int) {
	if px < 0 || py < 0 || py >= 4*d.rows {
		return
	}
	at := [2]int{px / 2, py / 4}
	cell, ok := d.cells[at]
	if !ok || rank >= cell.rank {
		cell.ink, cell.rank = k, rank
	}
	cell.bits |= dotBits[px%2][py%4]
	d.cells[at] = cell
}

// draw draws the braille from x along row y.
func (d dots) draw(c *canvas, x, y int) {
	for at, cell := range d.cells {
		c.text(x+at[0], y+at[1], string(0x2800+cell.bits), cell.ink)
	}
}
