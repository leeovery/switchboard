package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// titleGap is the blank cells the title row sets its parts apart by: after
// the badge and, at the fewest, before the clock.
const titleGap = 2

// pill is the dashboard's name, set on its own surface.
var pill = line{{" SWITCHBOARD ", ink{token: theme.Canvas, on: hue{token: theme.AccentPrimary}, bold: true}}}

var (
	tabInk      = ink{token: theme.TextSubtle}
	shownTabInk = ink{token: theme.TextPrimary, on: hue{token: theme.BgSelection}, bold: true}
	// The clock's date and time are kept bright behind a card, as it ticks on
	// while one is open.
	dateInk = ink{token: theme.TextMuted, kept: true}
	timeInk = ink{token: theme.TextSecondary, bold: true, kept: true}
)

// The turns the title row's pieces give way in where the row is short: the
// clock's date first, then its time.
const (
	dateLeaves = iota + 1
	timeLeaves
)

// titlePiece is a piece of the title row: its words, none where it's room
// alone, and the part flex lays it out as.
type titlePiece struct {
	words line
	part  flex.Part
}

// fixedPiece is a piece of words that takes its width, and gives way whole
// in the turn given.
func fixedPiece(words line, turn int) titlePiece {
	return titlePiece{words: words, part: flex.Part{Least: words.width(), Natural: words.width(), Leaves: turn}}
}

// title draws the title row, on the rows titleRows says it takes: the
// dashboard's name, from the first column; the tabs, the one shown picked
// out; and ending at the last column, the clock, at the frame's Clock where
// it's set, else at now. Where the row is short, the clock's date and then
// its time give way. On a phone, the tabs take rows of their own.
func (f Frame) title(c *canvas, now time.Time) {
	c.line(0, 0, pill)
	end := f.tabs(c)
	if !f.Clock.IsZero() {
		now = f.Clock
	}
	clocked(c, f.Width, end, now)
}

// TickSlack sets each tick just past the boundary it waits for, so the
// clock it reads, which can run a little behind the timer that brings it,
// has turned over.
const TickSlack = 50 * time.Millisecond

// TickDelay is how long from now until the clock shows another time: just
// past the next second, as it shows the time to the second.
func TickDelay(now time.Time) time.Duration {
	return now.Truncate(time.Second).Add(time.Second).Sub(now) + TickSlack
}

// clocked draws the clock at now, ending at the last column of a title row
// width cells wide, whose own pieces end before the column end: the clock's
// date and then its time give way where the row is short. It reports whether
// the row's own pieces fit, titleGap clear of its edge.
func clocked(c *canvas, width, end int, now time.Time) bool {
	pieces := []titlePiece{
		{part: flex.Part{Least: end, Natural: end}},
		{part: flex.Part{Least: titleGap, Grows: true}},
		fixedPiece(line{{prose.Day(now) + "  ", dateInk}}, dateLeaves),
		fixedPiece(line{{prose.ToTheSecond(now, now), timeInk}}, timeLeaves),
	}
	parts := make([]flex.Part, len(pieces))
	for i, p := range pieces {
		parts[i] = p.part
	}
	spans, fits := flex.Lay(width, parts...)
	for i, p := range pieces {
		if p.words != nil && !spans[i].Left {
			c.line(spans[i].At, 0, p.words)
		}
	}
	return fits
}

// titleRows is how many rows the title row takes: its own, and those its
// tabs take under it.
func (f Frame) titleRows() int {
	tabs := f.tabsAt()
	return tabs[len(tabs)-1].y + 1
}

// tabAt is a tab where the title row draws it: its view's, from column x of
// row y.
type tabAt struct {
	view View
	x, y int
}

// tabsAt are the tabs where they're drawn: two cells after the badge, a cell
// apart. On a phone, they take the rows under the clock's that tabRows splits
// them over instead, each from its first column, touching.
func (f Frame) tabsAt() []tabAt {
	if f.phone() {
		return f.tabsAlong(0, 1, 0, tabRows(f.tabWidths(), f.Width))
	}
	return f.tabsAlong(pill.width()+titleGap, 0, 1, []int{len(Views())})
}

// tabsAlong are the tabs laid along rows from row y, as many to a row as
// counts says, each row from column x, gap cells apart.
func (f Frame) tabsAlong(x, y, gap int, counts []int) []tabAt {
	views := Views()
	tabs := make([]tabAt, 0, len(views))
	for row, count := range counts {
		at := x
		for _, v := range views[len(tabs) : len(tabs)+count] {
			tabs = append(tabs, tabAt{view: v, x: at, y: y + row})
			at += f.tab(v).width() + gap
		}
	}
	return tabs
}

// tabWidths are the tabs' widths, in their order.
func (f Frame) tabWidths() []int {
	views := Views()
	widths := make([]int, len(views))
	for i, v := range views {
		widths[i] = f.tab(v).width()
	}
	return widths
}

// tabRows is how many tabs of the widths given go to each row, in their
// order, over the fewest rows they fit on whole in room cells, touching, as
// evenly as their order allows: their widest row as narrow as it can be, the
// earlier rows the fuller. The split hangs on the count of rows alone, so a
// tab changes row only as that count does. Where a tab is wider than room,
// they split as where the widest just fits, its row cut at the edge.
func tabRows(widths []int, room int) []int {
	if len(widths) == 0 {
		return nil
	}
	widestTab, total := slices.Max(widths), 0
	for _, w := range widths {
		total += w
	}
	rows := len(filled(widths, max(room, widestTab)))
	// No row of a split over rows is narrower than the widest tab, nor than
	// an even share of them all.
	for widest := max(widestTab, (total+rows-1)/rows); ; widest++ {
		if split := filled(widths, widest); len(split) <= rows {
			return split
		}
	}
}

// filled is how many tabs of the widths given go to each row, in their order,
// each row taking as many as fit whole in room cells, touching. No tab may be
// wider than room.
func filled(widths []int, room int) []int {
	counts := []int{0}
	taken := 0
	for _, w := range widths {
		if taken+w > room {
			counts, taken = append(counts, 0), 0
		}
		counts[len(counts)-1]++
		taken += w
	}
	return counts
}

// tabs draws the tabs, as tabsAt places them, and returns the column after
// the last on the title row's own, or after the badge where none is.
func (f Frame) tabs(c *canvas) int {
	end := pill.width()
	for _, t := range f.tabsAt() {
		if x := c.line(t.x, t.y, f.tab(t.view)); t.y == 0 {
			end = x
		}
	}
	return end
}

// tab is v's tab, its name with a space either side, picked out where it's
// the one shown.
func (f Frame) tab(v View) line {
	if v == f.View {
		return line{{" " + v.Title() + " ", shownTabInk}}
	}
	return line{{" " + v.Title() + " ", tabInk}}
}
