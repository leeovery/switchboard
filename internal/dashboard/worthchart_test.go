package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
			})
		}
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

// Where a chart's columns are narrower than their names, the last name,
// which would run past the chart's edge from its column's first cell, ends
// at the edge, in its ink, and the names before it give way, from the
// right, so none overlaps the next.
func TestChartAxisEndsTheLastNameAtTheEdge(t *testing.T) {
	months := []string{"Oct", "Nov", "Dec", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct"}
	for _, width := range []int{13, 25, 30, 40} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			slots := chartSlots(width, len(months))
			names := make([]line, len(months))
			for i, m := range months {
				names[i] = axisName(m, i == len(months)-1)
			}
			c := newCanvas(width, 1)
			chartAxis(c, 0, 0, width, slots, names)
			row := textOf(c.rows(Look{}))[0]
			if got := ansi.Cut(row, width-3, width); got != "Oct" || inkAt(c, width-3, 0) != strongInk {
				t.Errorf("the axis reads %q, want now's Oct ending at %d, in bold, in %q", got, width, row)
			}
			before := strings.TrimRight(ansi.Cut(row, 0, width-3), " ")
			if ansi.StringWidth(before) >= width-3 {
				t.Errorf("the axis reads %q, want the name before Oct ending a cell before it", row)
			}
		})
	}
}
