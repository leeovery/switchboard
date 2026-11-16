package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/views"
)

// A chart's columns span its width, from its first cell to its last: where
// they stand apart, each as wide as the rest, the spare widening the gaps;
// where they touch, the spare widening the columns; either way spread
// evenly, never bunched, and none overlapping the next.
func TestChartSlotsSpanTheirWidth(t *testing.T) {
	for _, buckets := range []int{1, 7, 12, 41, 52, 92} {
		for width := chartPart(buckets, 1, 0).Least; width <= 300; width++ {
			t.Run(fmt.Sprintf("%d in %d", buckets, width), func(t *testing.T) {
				slots := chartSlots(width, buckets)
				if len(slots) != buckets {
					t.Fatalf("got %d slots, want %d", len(slots), buckets)
				}
				checkSlotsSpan(t, slots, width)
				checkSlotsEven(t, slots)
				checkSlotsSpacing(t, slots, width)
			})
		}
	}
}

// checkSlotsSpacing fails where the columns aren't as far apart as their
// width holds: two cells, the spare aside, where each holds four cells so;
// else one where each holds one; else touching.
func checkSlotsSpacing(t *testing.T, slots []chartSlot, width int) {
	t.Helper()
	n := len(slots)
	want := 0
	switch {
	case 6*n-2 <= width:
		want = 2
	case 2*n-1 <= width:
		want = 1
	}
	for i := range n - 1 {
		gap := slots[i+1].x - slots[i].x - slots[i].width
		if gap < want || gap > want+1 || (want == 0 && gap > 0) {
			t.Errorf("gap %d is %d cells, want %d apart, the spare aside: %v", i, gap, want, slots)
			return
		}
	}
	if want == 2 && slots[0].width < 4 {
		t.Errorf("columns two apart are %d cells wide, want four at least: %v", slots[0].width, slots)
	}
}

// checkSlotsSpan fails where the slots don't run from the chart's first
// cell to its last, a cell wide at least, each ending before the next.
func checkSlotsSpan(t *testing.T, slots []chartSlot, width int) {
	t.Helper()
	if first, last := slots[0], slots[len(slots)-1]; first.x != 0 || last.x+last.width != width {
		t.Errorf("slots run from %d to %d, want 0 to %d: %v", first.x, last.x+last.width, width, slots)
	}
	for i, s := range slots {
		if s.width < 1 {
			t.Errorf("slot %d is %d cells wide, want one at least: %v", i, s.width, slots)
		}
		if i > 0 && slots[i-1].x+slots[i-1].width > s.x {
			t.Errorf("slot %d overlaps slot %d: %v", i-1, i, slots)
		}
	}
}

// checkSlotsEven fails where columns standing apart differ in width, or the
// gaps, or touching columns, differ by more than a cell, or where the spare
// bunches: across any first gaps, or any first columns where they touch,
// the spare they take is within a cell of their share of it.
func checkSlotsEven(t *testing.T, slots []chartSlot) {
	t.Helper()
	gaps := make([]int, len(slots)-1)
	for i := range gaps {
		gaps[i] = slots[i+1].x - slots[i].x - slots[i].width
	}
	spaced := len(gaps) > 0 && gaps[0] > 0
	widths := make([]int, len(slots))
	for i, s := range slots {
		widths[i] = s.width
	}
	if spaced {
		if lo, hi := spanOf(widths); lo != hi {
			t.Errorf("columns standing apart are %d to %d cells wide, want each as wide as the rest: %v", lo, hi, slots)
		}
		checkSpreadEvenly(t, "gaps", gaps)
		return
	}
	if lo, hi := spanOf(gaps); lo != 0 || hi != 0 {
		t.Errorf("gaps run %d to %d, want none or all apart: %v", lo, hi, slots)
	}
	checkSpreadEvenly(t, "columns", widths)
}

// checkSpreadEvenly fails where sizes differ by more than one, or where
// those above the least bunch: across any first k, the extra cells they
// take are within one of k's share of every extra.
func checkSpreadEvenly(t *testing.T, named string, sizes []int) {
	t.Helper()
	lo, hi := spanOf(sizes)
	if hi-lo > 1 {
		t.Errorf("%s run %d to %d cells, want none more than one wider than another: %v", named, lo, hi, sizes)
	}
	extra := 0
	for _, s := range sizes {
		extra += s - lo
	}
	took := 0
	for k, s := range sizes {
		took += s - lo
		if share := float64(extra*(k+1)) / float64(len(sizes)); float64(took) < share-1 || float64(took) > share+1 {
			t.Errorf("the first %d %s take %d of %d spare cells, want within one of %.1f: %v", k+1, named, took, extra, share, sizes)
			return
		}
	}
}

// spanOf is the least and the most of sizes, naught where there are none.
func spanOf(sizes []int) (lo, hi int) {
	if len(sizes) == 0 {
		return 0, 0
	}
	return slices.Min(sizes), slices.Max(sizes)
}

// evenShare rounds to the nearest: a single spare cell falls mid-way, not
// at either end, and all of them fall before the one past the last.
func TestEvenShareRoundsToTheNearest(t *testing.T) {
	for _, tt := range []struct{ i, cells, parts, want int }{
		{0, 5, 3, 0}, {1, 1, 3, 0}, {2, 1, 3, 1}, {3, 1, 3, 1}, {1, 2, 3, 1},
		{1, 1, 2, 1}, {1, 3, 4, 1}, {2, 3, 4, 2}, {3, 3, 4, 2}, {4, 3, 4, 3},
		{19, 1, 40, 0}, {20, 1, 40, 1}, {40, 1, 40, 1},
	} {
		if got := evenShare(tt.i, tt.cells, tt.parts); got != tt.want {
			t.Errorf("evenShare(%d, %d, %d) = %d, want %d", tt.i, tt.cells, tt.parts, got, tt.want)
		}
	}
}

// Compare's charts lie as they always have: whole steps across the head,
// each column step less apart wide, a cell at least, the spare at the
// right, the gutter between charts side by side.
func TestCompareSlotsLieInWholeSteps(t *testing.T) {
	for buckets := 1; buckets <= 100; buckets++ {
		for width := 0; width <= 400; width++ {
			apart := chartApart(width, buckets)
			step := (width + apart) / buckets
			for i, s := range compareSlots(width, buckets) {
				if want := (chartSlot{x: i * step, width: max(step-apart, 1)}); s != want {
					t.Fatalf("%d buckets in %d: slot %d is %+v, want %+v", buckets, width, i, s, want)
				}
			}
		}
	}
}

// namedChart is a chart to draw, and what it's called.
type namedChart struct {
	name string
	by   views.WorthBy
}

// namedCharts are WORTH BY's charts at 12 weeks by day, a date named each
// week, and at 12 months by week, a month named in three cells where it
// starts, as bucketedPage buckets them; each with its last bucket named, as
// its axis names it, and not. The first bucket is the peak, so the last
// column reaches the chart's top row.
func namedCharts() []namedChart {
	var charts []namedChart
	for _, period := range []struct {
		name, last string
		unit       views.BucketUnit
		n, days    int
		named      func(i int, start time.Time) string
	}{
		{"12 weeks by day", "4 Oct", views.ByDay, 84, 1, func(i int, start time.Time) string {
			if i%7 == 0 {
				return start.Format("2 Jan")
			}
			return ""
		}},
		{"12 months by week", "Oct", views.ByWeek, 53, 7, func(_ int, start time.Time) string {
			if start.Day() <= 7 {
				return start.Format("Jan")
			}
			return ""
		}},
	} {
		for _, lastNamed := range []bool{true, false} {
			by := bucketedPage(period.unit, period.n, 0).WorthBy[0]
			by.Peak, by.Buckets[0].Worth = 0, worthOf(1200)
			for i := range by.Buckets {
				by.Buckets[i].Axis = period.named(i, routedClock(1, 0, 0).AddDate(0, 0, period.days*(i-period.n)))
			}
			last := &by.Buckets[len(by.Buckets)-1]
			last.Axis = ""
			if lastNamed {
				last.Axis = period.last
			}
			charts = append(charts, namedChart{fmt.Sprintf("%s, its last named %v", period.name, lastNamed), by})
		}
	}
	return charts
}

// namedSlots lays the widest chart, from the whole width down to a cell a
// column, whose last name, from its own column, ends within the width; else
// the whole width: as roomFor finds it, for every count of buckets to 100,
// width from a cell a column to 400, and last name's width to 6. Read off
// the last slot's end, as chartSlots ends a chart at its width.
func TestNamedSlotsAreTheWidestThatHoldTheLastName(t *testing.T) {
	const most = 400
	for buckets := 1; buckets <= 100; buckets++ {
		names, lastAt := make([]line, buckets), lastColumns(buckets, most)
		for named := 0; named <= 6; named++ {
			names[buckets-1] = nil
			if named > 0 {
				names[buckets-1] = line{{strings.Repeat("x", named), dimInk}}
			}
			for width := buckets; width <= most; width++ {
				slots := namedSlots(width, names)
				if last, want := slots[buckets-1], roomFor(width, named, lastAt); last.x+last.width != want {
					t.Fatalf("%d buckets in %d, the last name %d cells: the chart takes %d cells, want %d", buckets, width, named, last.x+last.width, want)
				}
			}
		}
	}
}

// Projects' WORTH BY and Detail's WORTH fill their width, drawn at a
// handful of widths: the chart takes the cells roomFor has, and every name
// the axis holds starts at its own column's first cell, the names from the
// last back giving way where they'd meet. Widths where the chart's buckets
// don't fit a cell each are passed over: there the real page falls back to
// a coarser chart, as chartFits chooses.
func TestWorthChartsFillTheirWidthRoomForTheLastName(t *testing.T) {
	doc, used := framedDoc(nil, nil), usageWorld(extraOff)
	widths := []int{100, 127, 160, 166, 167, 168, 169, 170, 173, 206, 260}
	for _, chart := range namedCharts() {
		t.Run("Projects, "+chart.name, func(t *testing.T) {
			for _, width := range widths {
				page := bucketedPage(chart.by.Unit, len(chart.by.Buckets), 0)
				page.WorthBy = views.WorthCharts{chart.by}
				f := projectsFrame(t, width, 53, projectRows(3), 0, &page)
				d := projectDrawnOf(f, f.Moment(doc, routedAt), routedAt, page)
				cells := f.Width
				if d.form == projectWide {
					cells = d.split
				}
				rows := textOf(f.Draw(doc, routedAt))
				head := rowWith(rows, "WORTH BY")
				if head < 0 {
					t.Fatalf("at %d, no WORTH BY in\n%s", width, strings.Join(rows, "\n"))
				}
				checkNamedChart(t, width, rows[head+1], rows[head+worthRows+1], 0, cells, chart.by)
			}
		})
		t.Run("Detail, "+chart.name, func(t *testing.T) {
			for _, width := range widths {
				page := detailsPage(t)
				page.Rows[0].WorthBy = views.WorthCharts{chart.by}
				f := detailFrame(Look{}, framedAccounts, "work", "", page, nil)
				f.Width = width
				if _, wide := f.detailIn(f.Moment(used, routedAt)); !wide {
					rows := textOf(f.stackedOf(used, routedAt).whole().c.rows(Look{}))
					y := slices.Index(rows, "WORTH")
					if y < 0 {
						t.Fatalf("at %d, no WORTH stacked in\n%s", width, strings.Join(rows, "\n"))
					}
					checkNamedChart(t, width, rows[y+1], rows[y+1+detailWorthChart], 0, f.edge(), chart.by)
					continue
				}
				rows, d := panelOf(f, used)
				y := d.worthAt()
				checkNamedChart(t, width, rows[y], rows[y+detailWorthChart], d.l.cx(), d.l.weeksCells(), chart.by)
			}
		})
	}
}

// lastColumns are where a chart's last column starts, as chartSlots lays a
// chart of buckets columns across each count of cells, from a cell a column
// to most, by that count: -1 where there are fewer cells than columns.
func lastColumns(buckets, most int) []int {
	at := make([]int, most+1)
	for cells := range at {
		at[cells] = -1
		if cells >= buckets {
			at[cells] = chartSlots(cells, buckets)[buckets-1].x
		}
	}
	return at
}

// roomFor is the cells of the widest chart, from width down to a cell a
// column, its last column starting as lastAt has it, whose last column's
// name, last cells wide, from its first cell, ends within width: width
// itself where there's no name, or no such chart.
func roomFor(width, last int, lastAt []int) int {
	if last == 0 {
		return width
	}
	for cells := width; cells >= 0 && lastAt[cells] >= 0; cells-- {
		if lastAt[cells]+last <= width {
			return cells
		}
	}
	return width
}

// checkNamedChart fails where the chart drawn from x, width cells wide, its
// top and axis rows given, the frame's width the one named, doesn't take
// the cells roomFor has, or its axis doesn't hold the names a pick from the
// last back would, each at its own column's first cell. It passes over a
// chart whose buckets don't fit, which the real page never draws.
func checkNamedChart(t *testing.T, frame int, top, axis string, x, width int, by views.WorthBy) {
	t.Helper()
	buckets := len(by.Buckets)
	if !chartFits(width)(buckets) {
		return
	}
	names := bucketNames(by)
	cells := roomFor(width, len(names[buckets-1].plain()), lastColumns(buckets, width))
	slots := chartSlots(cells, buckets)
	placed, next := []rune(strings.Repeat(" ", width)), width+1
	for i := buckets - 1; i >= 0; i-- {
		said := names[i].plain()
		if said == "" || slots[i].x+len(said) >= next {
			continue
		}
		copy(placed[slots[i].x:], []rune(said))
		next = slots[i].x
	}
	if got, want := strings.TrimRight(ansi.Cut(axis, x, x+width), " "), strings.TrimRight(string(placed), " "); got != want {
		t.Errorf("at %d, the axis reads\n%q, want each name at its own column's first cell\n%q", frame, got, want)
	}
	if bars := ansi.StringWidth(strings.TrimRight(ansi.Cut(top, x, x+width), " ")); bars != cells {
		t.Errorf("at %d, the bars end at %d of %d cells, want %d, the widest that leaves room for the last name", frame, bars, width, cells)
	}
}
