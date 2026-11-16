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

// namedChart is a chart to draw, and what it's called.
type namedChart struct {
	name string
	by   views.WorthBy
}

// namedCharts are WORTH BY's charts at 12 weeks by day, a date named each
// week, and at 12 months by week, a month named where it starts, as
// bucketedPage buckets them; each with its last bucket named, and not.
func namedCharts() []namedChart {
	var charts []namedChart
	for _, period := range []struct {
		name    string
		unit    views.BucketUnit
		n, days int
		named   func(i int, start time.Time) string
	}{
		{"12 weeks by day", views.ByDay, 84, 1, func(i int, start time.Time) string {
			if i%7 == 0 {
				return start.Format("2 Jan")
			}
			return ""
		}},
		{"12 months by week", views.ByWeek, 53, 7, func(_ int, start time.Time) string {
			if start.Day() <= 7 {
				return start.Format("Jan")
			}
			return ""
		}},
	} {
		for _, lastNamed := range []bool{true, false} {
			by := bucketedPage(period.unit, period.n, 0).WorthBy[0]
			by.Peak, by.Buckets[0].Worth = 0, worthOf(1100)
			for i := range by.Buckets {
				by.Buckets[i].Axis = period.named(i, routedClock(1, 0, 0).AddDate(0, 0, period.days*(i-period.n)))
			}
			last := &by.Buckets[len(by.Buckets)-1]
			last.Axis = ""
			if lastNamed {
				last.Axis = "4 Oct"
			}
			charts = append(charts, namedChart{fmt.Sprintf("%s, its last named %v", period.name, lastNamed), by})
		}
	}
	return charts
}

// Projects' WORTH BY and Detail's WORTH fill their width: every name on the
// axis starts at its own column's first cell; where the last bucket has a
// name, the chart leaves room for it, the name ending at the chart's edge
// and the bars within its overhang of it, how far it would run past from
// its column across the whole width; where it has none, the bars end at the
// edge.
func TestWorthChartsFillTheirWidthRoomForTheLastName(t *testing.T) {
	doc := framedDoc(nil, nil)
	for _, chart := range namedCharts() {
		for _, width := range []int{160, 173} {
			t.Run(fmt.Sprintf("Projects, %s, at %d", chart.name, width), func(t *testing.T) {
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
					t.Fatalf("no WORTH BY in\n%s", strings.Join(rows, "\n"))
				}
				checkNamedChart(t, rows[head+1], rows[head+worthRows+1], 0, cells, chart.by)
			})
			t.Run(fmt.Sprintf("Detail, %s, at %d", chart.name, width), func(t *testing.T) {
				page := detailsPage(t)
				page.Rows[0].WorthBy = views.WorthCharts{chart.by}
				f := detailFrame(Look{}, framedAccounts, "work", "", page, nil)
				f.Width = width
				rows, d := panelOf(f, usageWorld(extraOff))
				y := d.worthAt()
				checkNamedChart(t, rows[y], rows[y+detailWorthChart], d.l.cx(), d.l.weeksCells(), chart.by)
			})
		}
	}
}

// checkNamedChart fails where the chart drawn from x, width cells wide, its
// top and axis rows given, doesn't hold its names to their columns, or
// its last name, or bars, to its edge.
func checkNamedChart(t *testing.T, top, axis string, x, width int, by views.WorthBy) {
	t.Helper()
	names := bucketNames(by)
	slots := namedSlots(width, names)
	placed := []rune(strings.Repeat(" ", width))
	for i, named := range names {
		if said := named.plain(); said != "" && cut(axis, x+slots[i].x, said) == said {
			copy(placed[slots[i].x:], []rune(said))
		}
	}
	if got, want := strings.TrimRight(ansi.Cut(axis, x, x+width), " "), strings.TrimRight(string(placed), " "); got != want {
		t.Errorf("the axis reads\n%q, want every name at its own column's first cell\n%q", got, want)
	}
	bars := ansi.StringWidth(strings.TrimRight(ansi.Cut(top, x, x+width), " "))
	last := names[len(names)-1].plain()
	if roomless := len(names) - 1 + len(last); last == "" || roomless > width {
		if bars != width {
			t.Errorf("the bars end at %d, want the chart's edge, %d, its last bucket unnamed or no room for its name at a cell a column", bars, width)
		}
		return
	}
	if end := slots[len(slots)-1].x + len(last); cut(axis, x+slots[len(slots)-1].x, last) != last || end != width {
		t.Errorf("the axis reads %q, want %s from its own column, ending at the chart's edge, %d", axis, last, width)
	}
	whole := chartSlots(width, len(names))
	overhang := max(whole[len(whole)-1].x+len(last)-width, 0)
	if bars > width || width-bars > overhang {
		t.Errorf("the bars end at %d, want within %d of the chart's edge, %d, its last name's overhang", bars, overhang, width)
	}
}
