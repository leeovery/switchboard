package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
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
	// The clock's date and time are kept bright behind a card, as it ticks on
	// while one is open.
	dateInk = ink{token: theme.TextMuted, kept: true}
	timeInk = ink{token: theme.TextSecondary, bold: true, kept: true}
)

// The turns the title row's pieces give way in where the row is short: tab's
// hint first, then the clock's date, then its time.
const (
	hintLeaves = iota + 1
	dateLeaves
	timeLeaves
)

// titlePiece is a piece of the title row: its words, none where it's room
// alone, and the part flex lays it out as.
type titlePiece struct {
	words line
	part  flex.Part
}

// fixedPiece is a piece of words that takes its width, gap cells after the
// piece before it, and gives way whole in the turn given, 0 for never.
func fixedPiece(words line, turn, gap int) titlePiece {
	return titlePiece{words: words, part: flex.Part{Least: words.width(), Natural: words.width(), Leaves: turn, Gap: gap}}
}

// title draws the title row, on the rows titleRows says it takes: the
// dashboard's name, from the first column; the tabs, the one shown picked
// out, then tab's hint; and ending at the last column, the clock, at the
// frame's Clock where it's set, else at now. Where the row is short, the
// hint, the clock's date and its time give way, in that order. On a phone,
// the tabs take rows of their own, without the hint.
func (f Frame) title(c *canvas, now time.Time) {
	c.line(0, 0, pill)
	end := f.tabs(c)
	pieces := []titlePiece{{part: flex.Part{Least: end, Natural: end}}}
	if !f.phone() {
		pieces = append(pieces, fixedPiece(hint, hintLeaves, titleGap))
	}
	if !f.Clock.IsZero() {
		now = f.Clock
	}
	pieces = append(pieces,
		titlePiece{part: flex.Part{Least: titleGap, Grows: true}},
		fixedPiece(line{{prose.Day(now) + "  ", dateInk}}, dateLeaves, 0),
		fixedPiece(line{{prose.ToTheSecond(now, now), timeInk}}, timeLeaves, 0),
	)
	parts := make([]flex.Part, len(pieces))
	for i, p := range pieces {
		parts[i] = p.part
	}
	spans, _ := flex.Lay(f.Width, parts...)
	for i, p := range pieces {
		if p.words != nil && !spans[i].Left {
			c.line(spans[i].At, 0, p.words)
		}
	}
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
