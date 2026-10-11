package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/theme"
)

const (
	// tabsFrom is the column the tabs start at, beside the dashboard's name.
	tabsFrom = 16
	// clockGap is the fewest cells between the tabs and the clock.
	clockGap = 2
)

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
// dashboard's name; the tabs, the one shown picked out, then tab's hint; and
// at the right the date and the time to the second, or the time alone where
// the date doesn't fit. On a phone, the tabs take a row of their own, from
// its first column, and the time stands alone, to the minute.
func (f Frame) title(c *canvas, now time.Time) {
	end := c.line(margin, 0, pill)
	if f.phone() {
		f.clock(c, end, line{{now.Format("15:04"), secondaryInk}})
		f.atRight(c, 2, f.tabs(c), hint)
		return
	}
	end = c.line(f.tabs(c)+1, 0, hint)
	f.clock(c, end, line{{now.Format("Mon 2 Jan  "), mutedInk}, {now.Format("15:04:05"), secondaryInk}}, line{{now.Format("15:04:05"), secondaryInk}})
}

// titleRows is how many rows the title row takes: on a phone, three, its
// tabs on the third; else one.
func (f Frame) titleRows() int {
	if f.phone() {
		return 3
	}
	return 1
}

// tabsAt is where the tabs are drawn: from x along row y, gap cells apart.
func (f Frame) tabsAt() (x, y, gap int) {
	if f.phone() {
		return 0, 2, 0
	}
	return tabsFrom, 0, 1
}

// tabs draws the tabs, as tabsAt places them, and returns the column after
// the last and its gap.
func (f Frame) tabs(c *canvas) int {
	x, y, gap := f.tabsAt()
	for _, v := range Views() {
		x = c.line(x, y, f.tab(v)) + gap
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

// clock draws the first of the clocks given that fits at the title row's
// right, clockGap cells clear of what's drawn before end.
func (f Frame) clock(c *canvas, end int, clocks ...line) {
	for _, l := range clocks {
		if f.atRight(c, 0, end, l) {
			return
		}
	}
}

// atRight draws l at row y's right, and reports whether it did: it doesn't
// where it would come within clockGap cells of what's drawn before end.
func (f Frame) atRight(c *canvas, y, end int, l line) bool {
	if f.edge()-l.width() < end+clockGap {
		return false
	}
	c.right(f.edge(), y, l)
	return true
}
