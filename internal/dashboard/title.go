package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// titleGap is the blank cells the title row sets its parts apart by: after
// the badge, before tab's hint, and at the fewest, before the clock.
const titleGap = 2

var (
	// pill is the dashboard's name, set on its own surface.
	pill = line{{" SWITCHBOARD ", ink{token: theme.Canvas, on: hue{token: theme.AccentPrimary}, bold: true}}}
	// hint says what moves between the tabs.
	hint = line{{"tab ⇥", faintInk}}
)

var (
	tabInk      = ink{token: theme.TextSubtle}
	shownTabInk = ink{token: theme.TextPrimary, on: hue{token: theme.BgSelection}, bold: true}
)

// title draws the title row, on the rows titleRows says it takes: the
// dashboard's name, from the first column; the tabs, the one shown picked
// out, then tab's hint; and ending at the last column, the clock. On a phone,
// the tabs take a row of their own, without the hint.
func (f Frame) title(c *canvas, now time.Time) {
	end := c.line(0, 0, pill)
	tabsEnd := f.tabs(c)
	if !f.phone() {
		end = c.line(tabsEnd+titleGap, 0, hint)
	}
	f.clock(c, end, now)
}

// titleRows is how many rows the title row takes: on a phone, two, its tabs
// on the second; else one.
func (f Frame) titleRows() int {
	if f.phone() {
		return 2
	}
	return 1
}

// tabsAt is where the tabs are drawn: from x along row y, gap cells apart.
// On a phone, they fill a row of their own from its first column, touching.
func (f Frame) tabsAt() (x, y, gap int) {
	if f.phone() {
		return 0, 1, 0
	}
	return pill.width() + titleGap, 0, 1
}

// tabs draws the tabs, as tabsAt places them, and returns the column after
// the last.
func (f Frame) tabs(c *canvas) int {
	x, y, gap := f.tabsAt()
	for i, v := range Views() {
		if i > 0 {
			x += gap
		}
		x = c.line(x, y, f.tab(v))
	}
	return x
}

// tab is v's tab, its name with a space either side, picked out where it's
// the one shown.
func (f Frame) tab(v View) line {
	if v == f.View {
		return line{{" " + v.Title() + " ", shownTabInk}}
	}
	return line{{" " + v.Title() + " ", tabInk}}
}

// clock draws the clock at now, ending at the title row's last column, at
// least titleGap cells clear of what's drawn before end: the date and the
// time to the second, or where the date doesn't fit, as a two-digit day's
// doesn't at 100 columns, the time alone.
func (f Frame) clock(c *canvas, end int, now time.Time) {
	clock := line{{prose.ToTheSecond(now, now), strongInk}}
	for _, l := range []line{slices.Concat(line{{prose.Day(now) + "  ", mutedInk}}, clock), clock} {
		if f.Width-l.width() >= end+titleGap {
			c.right(f.Width, 0, l)
			return
		}
	}
}
