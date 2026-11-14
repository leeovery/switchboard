package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// WEEKS says a week's peak as a whole percentage without its sign, in
// text.muted, at its cap bold state.destructive, as views marks it, never by
// its figure alone, and a dash where it isn't known, its bar then empty.
func TestWEEKSSaysEachWeeksPeak(t *testing.T) {
	tests := []struct {
		name  string
		share *float64
		atCap bool
		want  line
		empty bool
	}{
		{name: "under its limit", share: new(0.344), want: line{{"34", mutedInk}}},
		{name: "rounding to its limit, short of it", share: new(0.996), want: line{{"100", mutedInk}}},
		{name: "at its limit", share: new(1.0), atCap: true, want: line{{"100", exhaustedInk}}},
		{name: "a reserved account's 96%, at its cap", share: new(0.96), atCap: true, want: line{{"96", exhaustedInk}}},
		{name: "a reserved account's 94%, short of its cap", share: new(0.94), want: line{{"94", mutedInk}}},
		{name: "not known", want: line{noFigure}, empty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := views.WeekPeak{Share: tt.share, AtCap: tt.atCap}
			if got := peakSaid(p); !slices.Equal(got, tt.want) {
				t.Errorf("peakSaid = %v, want %v", got, tt.want)
			}
			if bar := peakBar(p); bar.empty != tt.empty || !tt.empty && (bar.used != *tt.share || bar.capAt != 1) {
				t.Errorf("peakBar = %+v, want empty %t, or filled to the peak, no cap", bar, tt.empty)
			}
		})
	}
}

// A day of BY DAY is its cells in its shade, a red ▪ in its middle where it
// hit a 5-hour limit: of two cells, the first; of three, the second.
func TestBYDAYMarksADaysLimitInItsMiddle(t *testing.T) {
	f := Frame{Look: NoColour()}
	k := dayInk(3)
	tests := []struct {
		cells int
		want  string
	}{
		{cells: 2, want: "▪" + shadeGlyph(3)},
		{cells: 3, want: shadeGlyph(3) + "▪" + shadeGlyph(3)},
		{cells: 4, want: shadeGlyph(3) + "▪" + strings.Repeat(shadeGlyph(3), 2)},
	}
	for _, tt := range tests {
		got := f.daySwatch(k, 3, true, tt.cells)
		if got.plain() != tt.want {
			t.Errorf("%d cells: %q, want %q", tt.cells, got.plain(), tt.want)
		}
		for _, s := range got {
			if s.text == limitMark && s.ink.token != theme.StateDestructive {
				t.Errorf("%d cells: ▪ in %+v, want state.destructive", tt.cells, s.ink)
			}
		}
		if plain := f.daySwatch(k, 3, false, tt.cells).plain(); plain != strings.Repeat(shadeGlyph(3), tt.cells) {
			t.Errorf("%d cells, no limit: %q", tt.cells, plain)
		}
	}
}

// BY DAY draws the days still to come of this week as · in text.faint, a
// cell each of the day's.
func TestBYDAYDotsTheDaysToCome(t *testing.T) {
	doc := usageWorld(extraOff)
	page := detailsPage(t)
	f := detailFrame(Look{}, framedAccounts, "work", "", page, nil)
	d := f.detailDrawn(doc, routedAt, laidIn(f, doc))
	c := d.panel()
	y := d.weeksAt()
	this := d.l.cx() + (d.weeksShown()-1)*d.l.step()
	for day := range 7 {
		toCome := day >= 7-page.ToCome
		for cell := range d.l.dw {
			got := c.at(this+day*d.l.dw+cell, y)
			if dotted := got.glyph == "·" && got.ink == faintInk; dotted != toCome {
				t.Errorf("this week's day %d, cell %d, is %q in %+v, want · in text.faint only where it's to come", day+1, cell, got.glyph, got.ink)
			}
		}
	}
}

// panelOf is Detail's panel of doc at routedAt in the frame given, drawn
// whole, as text alone, and how it's drawn.
func panelOf(f Frame, doc status.Document) ([]string, detailDrawn) {
	d := f.detailDrawn(doc, routedAt, laidIn(f, doc))
	return textOf(d.panel().rows(Look{})), d
}

// fromPanel is the row from the panel's column, trimmed at its right.
func fromPanel(row string, l detailLayout) string {
	return strings.TrimRight(ansi.Cut(row, l.panel, ansi.StringWidth(row)), " ")
}

// Detail's weeks section reaches over as many weeks as fit across the
// width, never fewer than the period's own where they fit, and at least
// six: its rule names those shown, "the last 13 weeks"; BY DAY shows them,
// the latest, to this one; and where those it reaches over, the period's,
// don't all show, as at 12 months, BY DAY says how many more there are at
// the panel's left, on its dates' row, in text.subtle.
func TestDetailsWeeksReachAtEachPeriodAndWidth(t *testing.T) {
	doc := usageWorld(extraOff)
	thisWeek := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		period         views.Period
		reaches, width int
		shown, dw      int
		rule, more     string
	}{
		{period: views.PeriodToday, reaches: 6, width: 173, shown: 8, dw: 2, rule: "the last eight weeks"},
		{period: views.PeriodThisWeek, reaches: 6, width: 107, shown: 8, dw: 1, rule: "the last eight weeks"},
		{period: views.PeriodThisWeek, reaches: 6, width: 160, shown: 7, dw: 2, rule: "the last seven weeks"},
		{period: views.PeriodThirtyDays, reaches: 6, width: 141, shown: 6, dw: 2, rule: "the last six weeks"},
		{period: views.PeriodTwelveWeeks, reaches: 12, width: 120, shown: 9, dw: 1, rule: "the last nine weeks", more: "‹ 3 more"},
		{period: views.PeriodTwelveWeeks, reaches: 12, width: 160, shown: 13, dw: 1, rule: "the last 13 weeks"},
		{period: views.PeriodTwelveMonths, reaches: 53, width: 173, shown: 15, dw: 1, rule: "the last 15 weeks", more: "‹ 38 more"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s at %d", tt.period.Name(), tt.width), func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "work", "", yearPage(t, tt.period, tt.reaches), nil)
			f.Width, f.Period = tt.width, tt.period
			rows, d := panelOf(f, doc)
			if d.l.dw != tt.dw || d.weeksShown() != tt.shown {
				t.Fatalf("its weeks are %d, their days %d cells; want %d, %d cells", d.weeksShown(), d.l.dw, tt.shown, tt.dw)
			}
			if got := fromPanel(rows[d.ruleRow()], d.l); !strings.HasPrefix(got, "┄┄ "+tt.rule+" ┄") {
				t.Errorf("the rule reads %q, want it to name %q", got, tt.rule)
			}
			dates := rows[d.weeksAt()+1]
			var heads []string
			for head := range strings.SplitSeq(ansi.Cut(dates, d.l.cx(), d.l.edge()), "  ") {
				if head = strings.TrimSpace(head); head != "" {
					heads = append(heads, head)
				}
			}
			first := thisWeek.AddDate(0, 0, -7*(tt.shown-1)).Format("2 Jan")
			if len(heads) != tt.shown || heads[0] != first || heads[len(heads)-1] != "28 Sep" {
				t.Errorf("BY DAY's weeks are headed %q, want %d from %s to 28 Sep", heads, tt.shown, first)
			}
			if got := strings.TrimSpace(ansi.Cut(dates, d.l.panel, d.l.cx())); got != tt.more {
				t.Errorf("BY DAY's dates' row reads %q before its weeks, want %q", got, tt.more)
			}
			if tt.more != "" {
				if got := d.panel().at(d.l.panel, d.weeksAt()+1).ink; got != dimInk {
					t.Errorf("%s is in %+v, want text.subtle", tt.more, got)
				}
			}
		})
	}
}

// All accounts' weeks are bands wherever the wide form draws them, however
// short the view, the panel scrolling past them: all accounts' band first,
// then an account's each, 5 rows apart, its name at the left, each week's
// peak under its column, and its facts at its right, Weeks' facts on the
// weeks it reaches over, all accounts' saying how many some account was
// short. In the stack, it's Weeks' table: WEEKS on its heads' row, all
// accounts' row, a blank row, then an account's each, its facts in two
// headed columns. Neither draws BY DAY, nor any verdict.
func TestDetailOfAllAccountsDrawsBandsWhereTheyFitElseTheTable(t *testing.T) {
	doc := usageWorld(extraOff)
	verdicts := []string{"BY DAY", views.HitsItsLimit, views.PlentyToSpare, views.EndsWithRoom, views.RoomToSpare, views.LittleToSpare}
	bands := map[int]string{
		0: "all accounts", bandRows: "74%     73%     82%", bandHeight: "work", 2 * bandHeight: "personal   ◆", 3 * bandHeight: "side",
	}
	tests := []struct {
		width, height int
		rule          string
		rows          map[int]string
	}{
		{width: 173, height: 53, rule: "the last 14 weeks", rows: bands},
		{width: 173, height: 24, rule: "the last 14 weeks", rows: bands},
		{width: 107, height: 37, rule: "the last six weeks", rows: map[int]string{0: "all accounts", bandHeight: "work"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.width, tt.height), func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "", "", yearPage(t, views.PeriodThisWeek, sixWeeks), nil)
			f.Width, f.Height = tt.width, tt.height
			rows, d := panelOf(f, doc)
			if got := d.weeksForm(); got != weeksInBands {
				t.Fatalf("at %dx%d, the weeks are drawn in form %d, want bands", tt.width, tt.height, got)
			}
			if got := fromPanel(rows[d.ruleRow()], d.l); !strings.HasPrefix(got, "┄┄ "+tt.rule+" ┄") {
				t.Errorf("the rule reads %q, want it to name %q", got, tt.rule)
			}
			for dy, want := range tt.rows {
				if got := fromPanel(rows[d.weeksAt()+dy], d.l); !strings.HasPrefix(strings.TrimLeft(got, " "), want) || (want == "") != (got == "") {
					t.Errorf("the weeks' row %d reads %q, want it to start %q", dy, got, want)
				}
			}
			for _, row := range rows {
				for _, gone := range verdicts {
					if strings.Contains(row, gone) {
						t.Errorf("a row reads %q, want no %s", row, gone)
					}
				}
			}
		})
	}

	t.Run("their facts", func(t *testing.T) {
		am := rereadMoment(t)
		f := Frame{
			Width: 173, Height: wholeRows, View: Accounts, Policy: claude.Policy(), Period: views.PeriodThisWeek,
			Accounts: AccountsShown{Shown: AccountsDetail, Accounts: am.Accounts, Page: momentPage(am, views.PeriodThisWeek)},
		}
		d := f.detailDrawn(am.Doc, am.Now, laidIn(f, am.Doc))
		rows := textOf(d.panel().rows(Look{}))
		at := d.bands().factsAt()
		for i, want := range []string{"short on 3 of 8", "limit hit 3 of 8", "limit hit 0 of 8"} {
			if got := strings.TrimSpace(ansi.Cut(rows[d.weeksAt()+i*bandHeight], at, f.Width)); got != want {
				t.Errorf("band %d's facts read %q from column %d, want %q", i, got, at, want)
			}
		}
	})
}

// WORTH reaches across the panel, from cx to the weeks' edge, a blank row
// under the weeks section: its heading at the panel's left on its chart's
// top row; its chart detailWorthChart rows tall, its peak's column the
// whole of them, its buckets' slots laid across the weeks' cells as
// chartSlots lays them; its axis under it; and its peak under that.
func TestDetailsWORTHReachesAcrossThePanel(t *testing.T) {
	doc := usageWorld(extraOff)
	for _, width := range []int{120, 160, 220} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			page := detailsPage(t)
			page.Rows[0].WorthBy = views.WorthCharts{weekByDay([]float64{40, 80, 148, 60, 30, 20, 10})}
			f := detailFrame(Look{}, framedAccounts, "work", "", page, nil)
			f.Width = width
			rows, d := panelOf(f, doc)
			y := d.worthAt()
			if y != d.weeksAt()+d.weeksRows()+1 || fromPanel(rows[y-1], d.l) != "" {
				t.Errorf("WORTH is on row %d, want a blank row under the weeks section, on %d", y, d.weeksAt()+d.weeksRows()+1)
			}
			if got := fromPanel(rows[y], d.l); !strings.HasPrefix(got, "WORTH") {
				t.Errorf("WORTH's row reads %q, want WORTH at the panel's left", got)
			}
			floor, slots := rows[y+detailWorthChart-1], chartSlots(d.l.weeksCells(), 7)
			last := d.l.cx() + slots[6].x + slots[6].width
			if columnOf(floor, "█") != d.l.cx() || ansi.StringWidth(strings.TrimRight(floor, " ")) != last || last > d.l.edge() {
				t.Errorf("its chart's floor reads %q, want it from %d to %d, its last slot's end, within the weeks' edge, %d", floor, d.l.cx(), last, d.l.edge())
			}
			if got := ansi.Cut(rows[y], d.l.cx(), d.l.edge()); !strings.Contains(got, "█") {
				t.Errorf("its chart's top row reads %q, want its peak's column reaching it, %d rows tall", got, detailWorthChart)
			}
			if got := rows[y+detailWorthChart]; columnOf(got, "Mon") != d.l.cx() || !strings.HasSuffix(strings.TrimRight(got, " "), "Sun") {
				t.Errorf("its axis reads %q, want Mon at %d to Sun", got, d.l.cx())
			}
			if got := rows[y+detailWorthChart+1]; columnOf(got, "peak Wed $148") != d.l.cx() {
				t.Errorf("its peak reads %q, want peak Wed $148 at %d", got, d.l.cx())
			}
		})
	}
}

// LATELY tells every event the document carries, whatever the view's
// height: it fills the rows down to the key line, where it starts in view,
// and the rest scrolls, the row over the key line saying how many more lie
// below.
func TestDetailsLATELYTellsEveryEventTheRestScrolling(t *testing.T) {
	const told = 60
	var events []status.Event
	for i := range told {
		events = append(events, startedAt(13, 59-i, fmt.Sprintf("%08x", i+1), "work", status.ReasonNew))
	}
	doc := usageWorld(extraOff, happened(events...))
	started := func(row string) bool { return strings.Contains(row, "started here") }
	for _, height := range []int{37, 53, 60, wholeRows} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "work", "", detailsPage(t), nil)
			f.Height = height
			panel, d := panelOf(f, doc)
			if got := len(slices.DeleteFunc(slices.Clone(panel[d.latelyAt():]), func(r string) bool { return !started(r) })); got != told {
				t.Errorf("at %d rows, LATELY tells %d lines, want every one of the %d", height, got, told)
			}
			rows := textOf(f.Draw(doc, routedAt))
			foot := f.Height - underView
			if d.latelyAt() < f.viewRows() && !started(rows[foot-1]) {
				t.Errorf("at %d rows, the view's last row reads %q, want LATELY filling it", height, rows[foot-1])
			}
			more := len(panel) - f.viewRows()
			if want := fmt.Sprintf("▼ %d more below · j/k", more); strings.TrimSpace(rows[foot]) != want {
				t.Errorf("at %d rows, the row over the key line reads %q, want %q", height, rows[foot], want)
			}
		})
	}
}
