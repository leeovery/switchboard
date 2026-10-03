package dashboard

import (
	"math"
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/theme"
)

// Chart is the style the cards draw the window each features in, as g
// cycles it, for every card at once, so the cards stay comparable, and the
// preferences file keeps it: a burn-down, then its burn rate, then an
// hourglass, and round.
type Chart string

const (
	// Burndown is the room left in the window, falling toward the floor as
	// it's used, and where it's heading.
	Burndown Chart = ""
	// BurnRate is the window's use per 10 minutes, as bars, against the
	// fastest it could be used and still last to its reset.
	BurnRate Chart = "burn-rate"
	// Hourglass is the window as an hourglass: the sand above the room left,
	// the pile below the use, and the stream between them its recent rate.
	Hourglass Chart = "hourglass"
)

// charts are the chart styles, in the order g moves through them.
var charts = []Chart{Burndown, BurnRate, Hourglass}

// Next is the style g moves on to from c, round to burn-down. A style g
// doesn't reach, as one a later switchboard kept, the cards draw as
// burn-down, and it moves on as burn-down does.
func (c Chart) Next() Chart {
	return charts[(max(slices.Index(charts, c), 0)+1)%len(charts)]
}

// Name is what the help calls the style, as the cards draw it: "burn rate",
// "hourglass", or "burn-down", which a style g doesn't reach is drawn as.
func (c Chart) Name() string {
	switch c {
	case BurnRate:
		return "burn rate"
	case Hourglass:
		return "hourglass"
	default:
		return "burn-down"
	}
}

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

// plot is the chart of a window on a card, whichever style it's drawn in:
// how the window stands, its span and its history, how fast it's been used
// lately and whether its account is busy, in the colour of the account's
// state, at the moment drawn.
type plot struct {
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
	// rate is how fast the window has been used lately, a share of it an
	// hour, none where that's not known; and busy is set while a session on
	// its account is busy, as the request stream or its sessions say.
	rate float64
	busy bool
	now  time.Time
}

// elapsed is how much of the window has passed at now, from 0 to 1.
func (p plot) elapsed() float64 {
	return min(max(float64(p.now.Sub(p.start))/float64(p.length), 0), 1)
}

// nowColumn is the column of a chart width cells wide just past the one now
// falls in, which the line for now takes, and what's ahead starts in.
func (p plot) nowColumn(width int) int {
	return min(int(p.elapsed()*float64(width)), width-1) + 1
}

// usedAt is how much of the window its last reading at or before t had used,
// reporting false where it has none; or, without history, how much is used
// now.
func (p plot) usedAt(t time.Time) (float64, bool) {
	if !p.traced {
		return p.window.Utilization, true
	}
	i, found := slices.BinarySearchFunc(p.trail.Readings, t, func(r score.Reading, t time.Time) int { return r.At.Compare(t) })
	if !found {
		i--
	}
	if i < 0 {
		return 0, false
	}
	return p.trail.Readings[i].Utilization, true
}

// chart draws the chart p of the window a card features, width cells wide
// and rows tall from x along row y, in the style the frame's Chart names: a
// burn-down, its burn rate, or an hourglass.
func (f Frame) chart(c *canvas, p plot, x, y, width, rows int) {
	switch f.Chart {
	case BurnRate:
		f.burnRate(c, p, x, y, width, rows)
	case Hourglass:
		f.hourglass(c, p, x, y, width, rows)
	default:
		f.burndown(c, p, x, y, width, rows)
	}
}

// burndown draws the window's burn-down width cells wide and rows tall from
// x along row y. The past is a level in eighths of a cell, a column for each
// stretch of the window up to now, filled to the room left at the middle of
// that stretch, as roomAt has it, in the state's colour faded halfway:
// without history, the room now, dim, marked "no history yet", the words
// drawn last, over whatever they cross. Where nothing was left, a line runs
// along the floor in destructive, and on until the limit lifts while the
// window's held at it. Now is a thin line in the border's colour; where the
// window's heading, a dotted line from the level now, to ✕ on the floor where
// it runs out, else to the room it has left at its reset. The floor is where
// the account's reserve starts, where that holds it back, drawn as a faint
// dotted line, else its limit. The marks, now's line and what's ahead, are
// drawn over the level under them. A lapsed window is a full level, dim,
// saying when it starts; and one whose reset isn't known has no length to
// draw it over.
func (f Frame) burndown(c *canvas, p plot, x, y, width, rows int) {
	switch {
	case width < 1 || rows < 1:
		return
	case p.lapsed:
		for col := range width {
			level(c, x+col, y, rows, 1, faintLevelInk)
		}
		across(c, x, y+rows/2, width, "full · "+p.starts)
		return
	case !p.spanned:
		return
	}
	past := ink{token: p.tone, fade: projectionFade}
	if !p.traced {
		past = faintLevelInk
	}
	ahead := p.nowColumn(width)
	for col := range ahead {
		if room, ok := p.roomAt(p.middleOf(col, width)); ok {
			level(c, x+col, y, rows, room, past)
		}
	}
	nowRule(c, x, y, width, rows, ahead)
	f.ahead(c, p, x, y, width, rows, ahead)
	if !p.traced {
		untraced(c, x, y, width, rows, ahead)
	}
}

// middleOf is the time at the middle of column col of a chart width cells
// wide, or now, for the column now falls in, where that's sooner.
func (p plot) middleOf(col, width int) time.Time {
	return earliest(p.at((float64(col)+0.5)/float64(width)), p.now)
}

// nowRule draws the line for now down a chart width cells wide and rows
// tall from x along row y, in its column ahead, where that's within it.
func nowRule(c *canvas, x, y, width, rows, ahead int) {
	if ahead >= width {
		return
	}
	for row := range rows {
		c.text(x+ahead, y+row, nowLine, borderInk)
	}
}

// untracedLabel marks a chart drawn without history.
const untracedLabel = "no history yet"

// untraced marks a chart width cells wide and rows tall from x along row y,
// drawn without history, its column for now ahead: across its level, from
// its left to now, where the words fit there clear of its marks, else
// across the whole of it.
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
func (p plot) roomAt(t time.Time) (float64, bool) {
	if p.heldAt(t) {
		return 0, true
	}
	used, ok := p.usedAt(t)
	return 1 - used, ok
}

// heldAt reports whether the window was held at its limit at t, as it is
// from when it was reached, where that's known.
func (p plot) heldAt(t time.Time) bool {
	return p.held && !p.since.IsZero() && !t.Before(p.since)
}

// at is the time a share of the way through the window.
func (p plot) at(share float64) time.Time {
	return p.start.Add(time.Duration(share * float64(p.length)))
}

// fraction is how far through the window t is, from 0 at its start.
func (p plot) fraction(t time.Time) float64 {
	return float64(t.Sub(p.start)) / float64(p.length)
}

// ahead draws what's ahead of a chart from its column ahead: while its
// window's held at its limit, a line along the floor until the limit lifts;
// else where it's heading, as a dotted line, to ✕ on its floor where it runs
// out, and its account's reserve, where that's its floor, as a faint dotted
// line along it.
func (f Frame) ahead(c *canvas, p plot, x, y, width, rows, ahead int) {
	if p.held {
		limitLine(c, p, x, y, width, rows, ahead)
		return
	}
	d := newDots(rows)
	if p.floor < 1 {
		for col := ahead + 2 - ahead%2; col < width; col += 2 {
			d.dot(2*col+1, d.rowOf(1-p.floor), ink{token: theme.VizReserve, fade: reserveFade}, 0)
		}
	}
	end, room := p.headingTo(width)
	if p.runsOut {
		end = max(end, ahead)
	}
	from := 1 - p.window.Utilization
	for col := ahead; col < end; col++ {
		at := (float64(col) + 0.25) / float64(width)
		d.dot(2*col, d.rowOf(from+(room-from)*p.progress(at)), ink{token: p.tone}, 1)
	}
	d.draw(c, x, y)
	if p.runsOut && end < width {
		c.text(x+end, y+d.rowOf(1-p.floor)/4, outMark, outInk)
	}
}

// limitLine draws a line along the floor of a chart width cells wide and rows
// tall from x along row y, its window held at its limit, from its column
// from until the limit lifts, or to its end where that's not known.
func limitLine(c *canvas, p plot, x, y, width, rows, from int) {
	for col := from; col < width && (p.back.IsZero() || p.at(float64(col)/float64(width)).Before(p.back)); col++ {
		c.text(x+col, y+rows-1, floorLine, errorInk)
	}
}

// headingTo is where a chart width cells wide draws its window heading to:
// the column it runs out in, and the room on its floor; else its last, and
// the room it has left at its reset; or nowhere, where nothing says, or its
// use has reached its floor already.
func (p plot) headingTo(width int) (int, float64) {
	share, ok := p.projected()
	switch {
	case p.runsOut:
		return min(int(p.fraction(p.out.At)*float64(width)), width), 1 - p.floor
	case !ok || p.window.Utilization >= p.floor-score.Tolerance:
		return 0, 0
	default:
		return width, 1 - share
	}
}

// progress is how far a share of the way through the window is from now
// toward where the window is heading: 0 now, 1 there.
func (p plot) progress(share float64) float64 {
	end := 1.0
	if p.runsOut {
		end = p.fraction(p.out.At)
	}
	if end <= p.elapsed() {
		return 1
	}
	return min(max((share-p.elapsed())/(end-p.elapsed()), 0), 1)
}

// level draws a chart's column at x, rows tall from row y, filled from its
// floor to room, a share of the whole, in eighths of a cell, in the ink k,
// its lowest eighth at least where any room is left; and where none is, as
// at a limit, a line along the floor in destructive.
func level(c *canvas, x, y, rows int, room float64, k ink) {
	if room <= 0 {
		c.text(x, y+rows-1, floorLine, errorInk)
		return
	}
	column(c, x, y, rows, max(int(math.Round(min(room, 1)*float64(8*rows))), 1), k)
}

// column draws a chart's column at x, rows tall from row y, filled eighths
// eighths of a cell up from its floor, in the ink k.
func column(c *canvas, x, y, rows, eighths int, k ink) {
	for row := range rows {
		if filled := min(eighths-8*row, 8); filled > 0 {
			c.text(x, y+rows-1-row, levels[filled], k)
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
func (f Frame) axis(c *canvas, p plot, x, y, width int) {
	switch {
	case p.lapsed || !p.spanned || width < 1:
		return
	case p.length > day:
		days(c, p, x, y, width)
		return
	}
	end := p.start.Add(p.length)
	c.text(x, y, p.start.In(p.now.Location()).Format("15:04"), dimInk)
	c.right(x+width, y, line{{end.In(p.now.Location()).Format("15:04"), mutedInk}})
	if ahead := p.nowColumn(width); ahead-2 >= clockCells+axisGap && ahead+1+axisGap <= width-clockCells {
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
// at each midnight within it, in the chart's column for its time, as the
// column it runs out in is, a tick, and the day's name after it, where it
// clears the day before's and fits, as midnights has them, today's picked
// out.
func days(c *canvas, p plot, x, y, width int) {
	column := func(t time.Time) int { return int(p.fraction(t) * float64(width)) }
	for _, m := range midnights(p.start, p.start.Add(p.length), p.now, column, width, dayCells-1) {
		c.text(x+m.col, y, "╵", faintInk)
		if !m.named {
			continue
		}
		name := ink{token: theme.TextSubtle}
		if m.today {
			name = strongInk
		}
		c.text(x+m.col+1, y, m.at.Format("Mon"), name)
	}
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
// zone, each in the column column puts it in, of those before width, its
// name showing where it's dayCells clear of the last shown, and what shows
// it, cells wide from its column, fits within width.
func midnights(start, end, now time.Time, column func(time.Time) int, width, cells int) []midnight {
	loc := now.Location()
	first, today := start.In(loc), now.In(loc).Format(time.DateOnly)
	var all []midnight
	last := -dayCells
	for at := time.Date(first.Year(), first.Month(), first.Day()+1, 0, 0, 0, 0, loc); at.Before(end); at = at.AddDate(0, 0, 1) {
		col := column(at)
		switch {
		case col >= width:
			return all
		case col < 0:
			continue
		}
		m := midnight{at: at, col: col, today: at.Format(time.DateOnly) == today, named: col-last >= dayCells && col+cells <= width}
		if m.named {
			last = col
		}
		all = append(all, m)
	}
	return all
}

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
