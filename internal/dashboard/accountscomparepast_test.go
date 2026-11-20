package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
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

// In a past period Compare draws no WEEK USED, so it fills no bar for a
// watch to ease; at now it fills every account's week.
func TestAPastCompareFillsNoBar(t *testing.T) {
	for _, width := range comparedWidths {
		for _, page := range []views.AccountsPage{chartedPage(t), goneCharted(t, lastWeek(t))} {
			f, doc := comparedOver(t, width, page)
			if filled, gone := f.Filled(doc, routedAt), f.Dial.Gone(); (len(filled) == 0) != gone {
				t.Errorf("at %d over %s, Compare fills %v, want bars only at now", width, f.Dial.Name(), filled)
			}
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
