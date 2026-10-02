package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/theme"
)

const (
	// tabsAt is the column the tabs start at, beside the dashboard's name.
	tabsAt = 16
	// clockGap is the fewest cells between the tabs and the clock.
	clockGap = 2
)

var (
	// pill is the dashboard's name, set on its own surface.
	pill = line{{" SWITCHBOARD ", ink{token: theme.Canvas, on: theme.AccentPrimary, bold: true}}}
	// hint says what moves between the views.
	hint = line{{"tab ⇥", faintInk}}
)

var (
	tabInk      = ink{token: theme.TextSubtle}
	shownTabInk = ink{token: theme.TextPrimary, on: theme.BgSelection, bold: true}
)

// title draws the title row, and returns the row under it: the dashboard's
// name; the views as tabs, the one shown picked out, then tab's hint while
// there's another view to move to; and at the right the date and the time
// to the second, or the time alone where the date doesn't fit. On a phone,
// the tabs take a row of their own, the hint at its right, and the time
// stands alone, to the minute.
func (f Frame) title(c *canvas, now time.Time) int {
	end := c.line(margin, 0, pill)
	if f.phone() {
		f.clock(c, end, line{{now.Format("15:04"), secondaryInk}})
		if len(f.Views) == 0 {
			return 1
		}
		end = f.tabs(c, margin, 2, 0)
		if len(f.Views) > 1 {
			f.atRight(c, 2, end, hint)
		}
		return 3
	}
	if end = f.tabs(c, tabsAt, 0, 1); len(f.Views) > 1 {
		end = c.line(end+1, 0, hint)
	}
	f.clock(c, end, line{{now.Format("Mon 2 Jan  "), mutedInk}, {now.Format("15:04:05"), secondaryInk}}, line{{now.Format("15:04:05"), secondaryInk}})
	return 1
}

// tabs draws the views' tabs from x along row y, gap cells apart, and returns
// the column after the last and its gap.
func (f Frame) tabs(c *canvas, x, y, gap int) int {
	for _, v := range f.Views {
		k := tabInk
		if v == f.View {
			k = shownTabInk
		}
		x = c.line(x, y, line{{" " + v.Title() + " ", k}}) + gap
	}
	return x
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
