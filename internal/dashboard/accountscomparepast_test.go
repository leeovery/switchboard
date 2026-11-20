package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// comparedWidths are the widths Compare's rules over a past period hold at:
// turned at 52 and 73, wide at 104, 160 and 173.
var comparedWidths = []int{52, 73, 104, 160, 173}

// goneCharted is chartedPage as the views give it of the period gone given:
// its weeks ending before this week, each whole, none so far, the last named
// on the axis so a bold name would show; its worth's buckets gone, none to
// come; and work's week run out after the period's last day, named by its
// date.
func goneCharted(t *testing.T, period views.Period) views.AccountsPage {
	page := chartedPage(t)
	page.Period, page.ThisWeek = period, false
	last := &page.Reach[len(page.Reach)-1]
	last.Axis = last.Start.Format("Jan")
	for _, row := range chartedRows(&page) {
		for i := range row.Reached {
			row.Reached[i].Open = false
		}
		row.WorthBy[0].Gone, row.WorthBy[0].ToCome = true, 0
	}
	page.Rows[0].WeekLeft, page.Rows[0].RanOutOn = new(0.0), "30 Sep"
	return page
}

// chartedRows are the page's rows Compare charts: each account's, then all
// accounts'.
func chartedRows(page *views.AccountsPage) []*views.AccountsRow {
	return []*views.AccountsRow{&page.Rows[0], &page.Rows[1], &page.Rows[2], &page.All}
}

// toFriday is the page with each column's worth run to Friday, as
// chartedWorth has it and $10 more, so at now the bucket now falls in is
// one the axis names at every width; none to come where the period is gone.
func toFriday(page views.AccountsPage) views.AccountsPage {
	for i, row := range chartedRows(&page) {
		gone := row.WorthBy[0].Gone
		row.WorthBy = views.WorthCharts{weekByDay(append(slices.Clone(chartedWorth[i]), 10))}
		if gone {
			row.WorthBy[0].Gone, row.WorthBy[0].ToCome = true, 0
		}
	}
	return page
}

// lastWeek is the week before now's, as < steps back to it.
func lastWeek(t *testing.T) views.Period {
	t.Helper()
	last, ok := views.LengthThisWeek.Now().Back(now, time.Monday, time.Time{})
	if !ok {
		t.Fatal("this week steps back to nothing")
	}
	return last
}

// comparedOver is Compare of the auto world's three accounts, work's token's
// year up in 21 days, as text alone, of a terminal the width given, over
// the page's period, its dial gone where the period is.
func comparedOver(t *testing.T, width int, page views.AccountsPage) (Frame, status.Document) {
	f, _ := threeCompared(t, width, turnedRows)
	f.Accounts.Page = &page
	return f.over(page.Period), usageWorld(extraOff, tokenLeft("work", 21))
}

// comparedCanvas is Compare's rows under its heads as its form draws them:
// wide, its rows' own canvas; turned, the whole of it.
func comparedCanvas(f Frame, doc status.Document) *canvas {
	if f.CompareWide() {
		return f.comparing(doc, routedAt).rows()
	}
	return f.turned(doc, routedAt).turnedWhole().c
}

// In a past period what tells the moment leaves Compare, wide or turned:
// the state under each name, the token's row and WEEK USED. Wide, the heads
// are the names alone, a click on them their one row, and BY MODEL moves up
// to the row after a blank under them; turned, BY MODEL is its first row.
// At now each shows.
func TestAPastComparesMomentLeavesAndBYMODELMovesUp(t *testing.T) {
	for _, width := range comparedWidths {
		for _, page := range []views.AccountsPage{chartedPage(t), goneCharted(t, lastWeek(t))} {
			f, doc := comparedOver(t, width, page)
			gone := f.Dial.Gone()
			t.Run(fmt.Sprintf("%d %s", width, f.Dial.Name()), func(t *testing.T) {
				rows := textOf(f.Draw(doc, routedAt))
				top := f.top()
				for _, leaves := range []string{"WEEK USED", "token"} {
					if shows := rowWith(rows, leaves) >= 0; shows == gone {
						t.Errorf("%q shows: %v, want %v, in\n%s", leaves, shows, !gone, strings.Join(rows, "\n"))
					}
				}
				byModel := top
				if f.CompareWide() {
					byModel = top + 2
					if stood := strings.TrimSpace(rows[top+1]); gone && stood != "" {
						t.Errorf("under the names reads %q, want a blank row", stood)
					}
				}
				if atTop := strings.HasPrefix(rows[byModel], "BY MODEL"); atTop != gone {
					t.Errorf("row %d reads %q; BY MODEL there: %v, want %v", byModel, rows[byModel], atTop, gone)
				}
				for _, target := range f.Targets(doc, routedAt) {
					if gone && target.On == OnColumn && target.Y != top {
						t.Errorf("a head's click zone is on row %d, want the names' alone, %d", target.Y, top)
					}
				}
			})
		}
	}
}

// In a past period Compare draws no WEEK USED, but fills its bars as at
// now, as they're the moment's: so n, within the ease after a reading
// lands, doesn't raise them from empty.
func TestAPastCompareFillsWEEKUSEDsBarsAsAtNow(t *testing.T) {
	for _, width := range comparedWidths {
		f, doc := comparedOver(t, width, chartedPage(t))
		gone, _ := comparedOver(t, width, goneCharted(t, lastWeek(t)))
		atNow, past := f.Filled(doc, routedAt), gone.Filled(doc, routedAt)
		if len(atNow) == 0 || !maps.Equal(past, atNow) {
			t.Errorf("at %d, Compare over %s fills %v, want %v, as at now", width, gone.Dial.Name(), past, atNow)
		}
	}
}

// A past period's WEEKS names no week so far and no month bold, as this
// week doesn't show: its last week's name, or at 12 months back its last
// month's, in text.subtle, as every other's; and wherever WEEKS draws its
// peaks, none is so far.
func TestAPastComparesWEEKSNameNoneBoldOrSoFar(t *testing.T) {
	year, ok := views.LengthTwelveMonths.Now().Back(now, time.Monday, time.Time{})
	if !ok {
		t.Fatal("12 months steps back to nothing")
	}
	for _, width := range comparedWidths {
		for _, period := range []views.Period{lastWeek(t), year} {
			f, doc := comparedOver(t, width, goneCharted(t, period))
			t.Run(fmt.Sprintf("%d %s", width, f.Dial.Name()), func(t *testing.T) {
				c := comparedCanvas(f, doc)
				rows := textOf(c.rows(Look{}))
				weeks, worth := rowWith(rows, "WEEKS  each week at its peak"), rowWith(rows, "WORTH  at API prices")
				if weeks < 0 || worth < weeks {
					t.Fatalf("no WEEKS before WORTH in\n%s", strings.Join(rows, "\n"))
				}
				if so := rowWith(rows[weeks:worth], views.PeakSoFar); so >= 0 {
					t.Errorf("WEEKS marks a week so far on %q", rows[weeks+so])
				}
				if !f.CompareWide() {
					return
				}
				axis := weeks + weeksChartRows + 1
				for _, col := range f.comparing(doc, routedAt).columns() {
					slots := compareSlots(col.head, len(f.Accounts.Page.Reach))
					at := col.x + slots[len(slots)-1].x
					if inkAt(c, at, axis) != dimInk {
						t.Errorf("the last name on the axis of the column from %d reads %q in %+v, want text.subtle", col.x, ansi.Cut(rows[axis], at, at+3), inkAt(c, at, axis))
					}
				}
			})
		}
	}
}

// A past period's WORTH marks no bucket as now's: no name on its axis is
// bold. At now, today's is.
func TestAPastComparesWORTHMarksNoBucketNow(t *testing.T) {
	for _, width := range comparedWidths {
		for _, page := range []views.AccountsPage{toFriday(chartedPage(t)), toFriday(goneCharted(t, lastWeek(t)))} {
			f, doc := comparedOver(t, width, page)
			t.Run(fmt.Sprintf("%d %s", width, f.Dial.Name()), func(t *testing.T) {
				c := comparedCanvas(f, doc)
				rows := textOf(c.rows(Look{}))
				bold := false
				for y, row := range rows {
					if !strings.Contains(row, "Mon") || !strings.Contains(row, "Sun") || y < rowWith(rows, "WORTH") {
						continue
					}
					for x := range c.width {
						bold = bold || inkAt(c, x, y).bold
					}
				}
				if gone := f.Dial.Gone(); bold == gone {
					t.Errorf("WORTH's axis names a bucket bold: %v, want %v", bold, !gone)
				}
			})
		}
	}
}

// In a past period left at its last reset names the day the week ran out
// as the views name it, by its date: "0% (limit 30 Sep)".
func TestAPastComparesLeftAtItsLastResetNamesTheDayItRanOut(t *testing.T) {
	for _, width := range comparedWidths[2:] {
		f, doc := comparedOver(t, width, goneCharted(t, lastWeek(t)))
		rows := textOf(f.Draw(doc, routedAt))
		left := rowWith(rows, "left at its last reset")
		if left < 0 {
			t.Fatalf("at %d, no left at its last reset in\n%s", width, strings.Join(rows, "\n"))
		}
		work := f.comparing(doc, routedAt).columns()[0]
		if got, want := strings.TrimSpace(ansi.Cut(rows[left], work.x, work.x+work.width)), "0% (limit 30 Sep)"; got != want {
			t.Errorf("at %d, work's left at its last reset reads %q, want %q", width, got, want)
		}
	}
}

// A period gone whose last week is this week, as yesterday on any day but
// a week's first, draws this week as now does: under each column's last
// week its so far, …, and the last name on the axis bold where no …
// stands in its place, as of a row whose last week isn't so far.
func TestAGonePeriodEndingThisWeekDrawsItAsNow(t *testing.T) {
	yesterday, ok := views.LengthToday.Now().Back(routedAt, time.Monday, time.Time{})
	if !ok {
		t.Fatal("today steps back to nothing")
	}
	page := chartedPage(t)
	page.Period = yesterday
	last := &page.Reach[len(page.Reach)-1]
	last.Axis = last.Start.Format("Jan")
	for _, width := range comparedWidths[2:] {
		f, doc := comparedOver(t, width, page)
		f.Period, f.Dial = yesterday, views.DialOf(yesterday, routedAt, time.Monday, time.Time{})
		if !f.Dial.Gone() {
			t.Fatalf("%s isn't gone at %v", f.Dial.Name(), routedAt)
		}
		cp := f.comparing(doc, routedAt)
		c := cp.rows()
		rows := textOf(c.rows(Look{}))
		axis := rowWith(rows, "WEEKS  each week at its peak") + weeksChartRows + 1
		for _, col := range cp.columns() {
			slots := compareSlots(col.head, len(page.Reach))
			at := col.x + slots[len(slots)-1].x
			if got := cut(rows[axis], at, views.PeakSoFar); got != views.PeakSoFar || inkAt(c, at, axis) != secondaryInk {
				t.Errorf("at %d, the axis of the column from %d reads %q at %d in %+v, want this week so far, %s", width, col.x, got, at, inkAt(c, at, axis), views.PeakSoFar)
			}
		}
		whole := page.Rows[0]
		whole.Reached = slices.Clone(whole.Reached)
		whole.Reached[len(whole.Reached)-1].Open = false
		if named := cp.reachNames(whole); !slices.Equal(named[len(named)-1], line{{last.Axis, strongInk}}) {
			t.Errorf("at %d, the last name of a row whose last week isn't so far is %+v, want %s bold", width, named[len(named)-1], last.Axis)
		}
	}
}

// Wide, in a past period, a click on a column's heads is on its name alone:
// one zone a column shown, on the names' row; the focused column's
// bg.selection is on that row alone; and the rows scroll from two rows
// under it, BY MODEL's, the blank row between held.
func TestAPastComparesHeadsAreOneRow(t *testing.T) {
	for _, width := range comparedWidths[2:] {
		f, doc := comparedOver(t, width, goneCharted(t, lastWeek(t)))
		f.Accounts.Focused, f.Accounts.Focus = true, "personal"
		top := f.top()
		cp := f.comparing(doc, routedAt)
		columns := cp.columns()
		zones := map[string]int{}
		for _, target := range f.Targets(doc, routedAt) {
			if target.On != OnColumn {
				continue
			}
			zones[target.Account]++
			if target.Y != top {
				t.Errorf("at %d, %q's head's zone is on row %d, want the names', %d", width, target.Account, target.Y, top)
			}
		}
		for _, col := range columns {
			if zones[col.id] != 1 {
				t.Errorf("at %d, %q has %d head zones, want 1", width, col.id, zones[col.id])
			}
		}
		if len(zones) != len(columns) {
			t.Errorf("at %d, head zones are of %v, want the %d columns shown", width, zones, len(columns))
		}
		c := f.At(doc, routedAt).page(doc, routedAt)
		personal := columns[1]
		for y := top - 1; y <= top+2; y++ {
			for x := personal.x; x < personal.x+personal.head; x++ {
				if selected := c.at(x, y).ink.on.token == theme.BgSelection; selected != (y == top) {
					t.Errorf("at %d, row %d, column %d on bg.selection: %v, want the names' row alone", width, y, x, selected)
				}
			}
		}
		if _, y, _, _ := cp.scrolls(); y != top+2 {
			t.Errorf("at %d, Compare's rows scroll from row %d, want %d, under the names and a blank row", width, y, top+2)
		}
	}
}
