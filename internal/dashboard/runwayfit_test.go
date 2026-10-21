package dashboard

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayFrame is a frame showing Runway's live 5-hour view at routedAt, of a
// terminal 160 columns wide and as tall as given, in nord, of lanes as
// given, and COMING UP's lines.
func runwayFrame(t *testing.T, height int, lanes *views.Lanes, coming []status.Upcoming) Frame {
	t.Helper()
	tl := views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayShown{}.Columns(160))
	return Frame{
		Width: 160, Height: height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike,
		Runway: RunwayShown{Window: views.RunwayFiveHour, Timeline: tl, Lanes: lanes, Coming: coming},
	}
}

// lanesOf are lanes of n accounts, and the pool's lane, of all of them, but
// with one account, which has none.
func lanesOf(n int) *views.Lanes {
	lanes := &views.Lanes{Accounts: make([]views.Lane, n)}
	if n > 1 {
		lanes.Pool = views.Pool{Of: n, Key: "5h"}
	}
	return lanes
}

// thingsComing are as many things coming up as given, for COMING UP to list.
func thingsComing(n int) []status.Upcoming {
	coming := make([]status.Upcoming, n)
	for i := range coming {
		coming[i] = status.Upcoming{At: routedAt.Add(time.Duration(i+1) * time.Hour), Kind: status.UpcomingReset, Window: "5h"}
	}
	return coming
}

// runwayDrawn is the canvas of f's Runway of doc at routedAt alone, drawn
// from the view's top.
func runwayDrawn(f Frame, doc status.Document) *canvas {
	c := newCanvas(f.Width, f.Height)
	f.runway(c, doc, routedAt, f.top())
	return c
}

// runwayFitted is what of Runway a fit draws, for a test to hold: the step
// it takes, how many lanes it shows, whether the legend and COMING UP show,
// and whether anything does.
type runwayFitted struct {
	step           runwayStep
	shown          int
	legend, coming bool
	fits           bool
}

// fittedOf is what of Runway the fit draws, as runwayFitted has it.
func fittedOf(fit runwayFit) runwayFitted {
	return runwayFitted{fit.step, fit.shown, fit.draws(fit.legend), fit.draws(fit.coming), fit.fits}
}

// fittedWhole is Runway fitted at the step given, shown lanes shown, the legend
// and COMING UP both shown.
func fittedWhole(step runwayStep, shown int) runwayFitted {
	return runwayFitted{step: step, shown: shown, legend: true, coming: true, fits: true}
}

// holdsTheLegendOverTheKeyLine fails the test where the fit draws the legend
// anywhere but on the rows ending on the one over the key line of a frame
// rows tall, legend rows of it.
func holdsTheLegendOverTheKeyLine(t *testing.T, fit runwayFit, rows, legend int) {
	t.Helper()
	if fit.draws(fit.legend) && fit.legend+legend-1 != rows-underView {
		t.Errorf("at %d rows, the legend's last row is %d, want the row over the key line, %d", rows, fit.legend+legend-1, rows-underView)
	}
}

// Runway takes the first step of its compaction whose parts fit, its legend
// over the key line, at 160 by 37 with N accounts and K lines of COMING UP:
// three rows an account while 3N + K ≤ 19; (a) while 2N + K ≤ 19; (b) while
// 2N + K ≤ 21; (c), COMING UP cut to a line and how many more, while
// 2N + 2 ≤ 21; (d) a row on; then its lanes scrolling, as many shown as fit;
// and the week frame's three as rows run short.
func TestRunwayTakesTheFirstOfItsStepsThatFits(t *testing.T) {
	tests := []struct {
		name                  string
		accounts, lines, rows int
		want                  runwayStep
		shown                 int
	}{
		{name: "four accounts, three rows each", accounts: 4, lines: 5, rows: 37, want: runwayRoomy, shown: 4},
		{name: "five, at (a)", accounts: 5, lines: 6, rows: 37, want: runwayTwoRows, shown: 5},
		{name: "six with nine lines, at (b)", accounts: 6, lines: 9, rows: 37, want: runwayNoGaps, shown: 6},
		{name: "six with ten lines, COMING UP cut at (c)", accounts: 6, lines: 10, rows: 37, want: runwayComingCut, shown: 6},
		{name: "ten, at (d)", accounts: 10, lines: 11, rows: 37, want: runwayAxisClose, shown: 10},
		{name: "eleven at 39, at (d)", accounts: 11, lines: 12, rows: 39, want: runwayAxisClose, shown: 11},
		{name: "eleven, scrolling at (e)", accounts: 11, lines: 12, rows: 37, want: runwayScrolled, shown: 10},
		{name: "ten at 34, eight in view", accounts: 10, lines: 3, rows: 34, want: runwayScrolled, shown: 8},
		{name: "ten at 31, seven in view", accounts: 10, lines: 3, rows: 31, want: runwayScrolled, shown: 7},
		{name: "ten at 27, five in view", accounts: 10, lines: 3, rows: 27, want: runwayScrolled, shown: 5},
		{name: "ten at 22, two in view", accounts: 10, lines: 3, rows: 22, want: runwayScrolled, shown: 2},
		{name: "ten at 19, one in view", accounts: 10, lines: 3, rows: 19, want: runwayScrolled, shown: 1},
		{name: "the 5-hour frame's three", accounts: 3, lines: 3, rows: 37, want: runwayRoomy, shown: 3},
		{name: "the week frame's three", accounts: 3, lines: 7, rows: 37, want: runwayRoomy, shown: 3},
		{name: "the week's three at 33", accounts: 3, lines: 7, rows: 33, want: runwayTwoRows, shown: 3},
		{name: "the week's three at 31", accounts: 3, lines: 7, rows: 31, want: runwayTwoRows, shown: 3},
		{name: "the week's three at 30", accounts: 3, lines: 7, rows: 30, want: runwayNoGaps, shown: 3},
		{name: "the week's three at 28", accounts: 3, lines: 7, rows: 28, want: runwayComingCut, shown: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayFrame(t, tt.rows, lanesOf(tt.accounts), thingsComing(tt.lines))
			fit := f.runwayFitted(f.top())
			if got, want := fittedOf(fit), fittedWhole(tt.want, tt.shown); got != want {
				t.Errorf("at %d rows, Runway fits %+v, want %+v", tt.rows, got, want)
			}
			holdsTheLegendOverTheKeyLine(t, fit, tt.rows, 1)
		})
	}
}

// The 5-hour frame's three accounts, with three lines of COMING UP, take each
// step as rows run short, a row at a time from 37 to 14: three rows each to
// 30; (a) to 27; (b) to 25; (c) at 24; (d) at 23; then their lanes
// scrolling, two in view to 21 and one to 19; then one lane, the legend
// given way at 18, and COMING UP too from 17 to 15, where the axis, the lane
// and the pool's lane and room fill the view; and under that, nothing but
// the page's chrome.
func TestRunwayFitsAtEveryHeightDownTo14Rows(t *testing.T) {
	noLegend := runwayFitted{step: runwayOneLane, shown: 1, coming: true, fits: true}
	bare := runwayFitted{step: runwayOneLane, shown: 1, fits: true}
	want := map[int]runwayFitted{
		37: fittedWhole(runwayRoomy, 3), 36: fittedWhole(runwayRoomy, 3), 35: fittedWhole(runwayRoomy, 3), 34: fittedWhole(runwayRoomy, 3),
		33: fittedWhole(runwayRoomy, 3), 32: fittedWhole(runwayRoomy, 3), 31: fittedWhole(runwayRoomy, 3), 30: fittedWhole(runwayRoomy, 3),
		29: fittedWhole(runwayTwoRows, 3), 28: fittedWhole(runwayTwoRows, 3), 27: fittedWhole(runwayTwoRows, 3),
		26: fittedWhole(runwayNoGaps, 3), 25: fittedWhole(runwayNoGaps, 3),
		24: fittedWhole(runwayComingCut, 3),
		23: fittedWhole(runwayAxisClose, 3),
		22: fittedWhole(runwayScrolled, 2), 21: fittedWhole(runwayScrolled, 2), 20: fittedWhole(runwayScrolled, 1), 19: fittedWhole(runwayScrolled, 1),
		18: noLegend, 17: bare, 16: bare, 15: bare,
		14: {},
	}
	for rows := 37; rows >= 14; rows-- {
		t.Run(strconv.Itoa(rows), func(t *testing.T) {
			f := runwayFrame(t, rows, lanesOf(3), thingsComing(3))
			fit := f.runwayFitted(f.top())
			if got := fittedOf(fit); got != want[rows] {
				t.Errorf("at %d rows, Runway fits %+v, want %+v", rows, got, want[rows])
			}
			holdsTheLegendOverTheKeyLine(t, fit, rows, 1)
		})
	}
}

// Where not even (f)'s least, a lane in view and the pool's lane and its
// room under it, fits, as under 15 rows with a pool's lane, or before the
// lanes are built, Runway draws nothing but the page's chrome; one account,
// with no pool's lane, fits shorter.
func TestRunwayDrawsThePagesChromeAloneWhereNotOneLaneFits(t *testing.T) {
	tests := []struct {
		name  string
		lanes *views.Lanes
		rows  int
		fits  bool
	}{
		{name: "ten at 15", lanes: lanesOf(10), rows: 15, fits: true},
		{name: "ten at 14", lanes: lanesOf(10), rows: 14},
		{name: "two at 14", lanes: lanesOf(2), rows: 14},
		{name: "one at 12", lanes: lanesOf(1), rows: 12, fits: true},
		{name: "one at 11", lanes: lanesOf(1), rows: 11},
		{name: "before the lanes are built", rows: 37},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayFrame(t, tt.rows, tt.lanes, thingsComing(3))
			fit := f.runwayFitted(f.top())
			if fit.fits != tt.fits {
				t.Errorf("at %d rows, Runway fits %t, want %t", tt.rows, fit.fits, tt.fits)
			}
			if _, ok := fit.laneRow(0); ok != tt.fits {
				t.Errorf("at %d rows, the first lane is drawn %t, want %t", tt.rows, ok, tt.fits)
			}
			if got := f.Scrolling(status.Document{}, routedAt); !tt.fits && got != (Scrolling{}) {
				t.Errorf("drawing nothing, Runway scrolls %+v, want nowhere", got)
			}
			if tt.fits {
				return
			}
			for y, row := range textOf(runwayDrawn(f, status.Document{}).rows(Look{})) {
				if strings.TrimSpace(row) != "" {
					t.Errorf("drawing nothing, Runway's row %d reads %q", y, row)
				}
			}
		})
	}
}

// Each of Runway's parts lies on the row its step gives it, from the axis at
// the view's top: three rows an account, the pool's lane after the last
// one's blank row, no rule over it, its room on a row of its own under it,
// COMING UP after a blank row, every line of it, and the legend over the key
// line, the rows spare between them, as the 5-hour frame lays them out; a
// day stepped back to with neither COMING UP nor the room; ten at (d),
// COMING UP cut to a line and how many more; one account, its last blank
// row the one before COMING UP; and on a phone alike, its legend of two.
func TestRunwaysPartsLieOnTheRowsItsStepGivesThem(t *testing.T) {
	type rows struct{ lanes, apart, pool, room, legend, coming, lines int }
	tests := []struct {
		name   string
		shown  RunwayShown
		width  int
		height int
		shape  runwayShape
		want   rows
	}{
		{
			name: "the 5-hour frame's three", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(3), Coming: thingsComing(3)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{10, 3, 19, 21, 35, 23, 3},
		},
		{
			name: "Wednesday stepped back to", shown: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30), Lanes: lanesOf(3)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{10, 3, 19, noRow, 35, noRow, 0},
		},
		{
			name: "the ten frame's ten", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(10), Coming: thingsComing(9)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{9, 2, 29, 31, 35, 32, 2},
		},
		{
			name: "one account", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(1), Coming: thingsComing(2)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{10, 3, noRow, noRow, 35, 13, 2},
		},
		{
			name: "the legend on two rows, at (a)", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(5), Coming: thingsComing(5)}, width: 120, height: 37,
			shape: runwayShape{legend: 2}, want: rows{10, 2, 20, 22, 34, 24, 5},
		},
		{
			name: "the phone frame's three", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(3), Coming: thingsComing(3)}, width: 52, height: 33,
			shape: runwayShape{legend: 2}, want: rows{11, 3, 20, 22, 30, 24, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := routedAt
			tt.shown.Timeline = tt.shown.TimelineAt(now, time.Monday, tt.width)
			f := Frame{Width: tt.width, Height: tt.height, View: Runway, Runway: tt.shown}
			fit := f.runwayFittedAs(f.top(), tt.shape)
			got := rows{fit.lanes, fit.apart, fit.pool, fit.room, fit.legend, fit.coming, fit.lines}
			if got != tt.want {
				t.Errorf("its parts lie on rows %+v, want %+v", got, tt.want)
			}
			if fit.axis != f.top() {
				t.Errorf("its axis is on row %d, want the view's top, %d", fit.axis, f.top())
			}
		})
	}
}

// At (e), Runway's lanes scroll an account at a time, no further than shows
// the last, the axis and the pool's foot holding; and on the pool's room's
// row, it says what's out of view, right-aligned, in text.subtle, as a
// scroll line is: above and below, or where that doesn't fit beside the
// room, the counts alone.
func TestRunwaysLanesScrollAnAccountAtATimeAndSayWhatsOutOfView(t *testing.T) {
	tests := []struct {
		name         string
		width        int
		rows, scroll int
		first        int
		says         string
	}{
		{name: "ten at 34, at the top", width: 160, rows: 34, first: 0, says: "▼ 2 more accounts"},
		{name: "ten at 34, scrolled to the last", width: 160, rows: 34, scroll: 2, first: 2, says: "▲ 2 more accounts"},
		{name: "ten at 34, scrolled past it", width: 160, rows: 34, scroll: 5, first: 2, says: "▲ 2 more accounts"},
		{name: "ten at 27, scrolled by two", width: 160, rows: 27, scroll: 2, first: 2, says: "▲ 2 more accounts · ▼ 3 more accounts"},
		{name: "ten at 27 on 60 columns, the counts alone", width: 60, rows: 27, scroll: 2, first: 2, says: "▲ 2 more · ▼ 4 more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runwayDoc(10)
			f := runwayFrame(t, tt.rows, lanesOf(10), thingsComing(3))
			f.Width, f.Scroll = tt.width, tt.scroll
			fit := f.runwayFitted(f.top())
			if got, want := f.Scrolling(doc, routedAt), (Scrolling{Most: 10 - fit.shown, Page: fit.shown}); got != want || f.ScrollsRows(doc, routedAt) {
				t.Errorf("Runway scrolls %+v, rows at a time %t; want %+v, an account at a time", got, f.ScrollsRows(doc, routedAt), want)
			}
			if fit.first != tt.first {
				t.Errorf("the first lane shown is the %d-th, want the %d-th", fit.first, tt.first)
			}
			if y, ok := fit.laneRow(tt.first); !ok || y != f.top()+runwayAxisRows {
				t.Errorf("the first lane shown is on row %d, drawn %t; want the row under the axis, %d", y, ok, f.top()+runwayAxisRows)
			}
			if _, ok := fit.laneRow(tt.first - 1); ok {
				t.Errorf("the lane before the first shown is drawn")
			}
			if fit.out != fit.room {
				t.Errorf("what's out of view is on row %d, want the pool's room's, %d", fit.out, fit.room)
			}
			c := runwayDrawn(f, doc)
			row := rowOf(c, fit.room)
			room, out, _ := strings.Cut(row, " left ")
			if !strings.HasPrefix(room, "  5-hour ") || strings.TrimLeft(out, " ") != tt.says || len([]rune(row)) != tt.width {
				t.Errorf("the room's row reads %q, want the room whole, then %q ending at the edge", row, tt.says)
			}
			if got := c.at(tt.width-1, fit.out).ink; got != dimInk {
				t.Errorf("what's out of view is in %+v, want text.subtle", got)
			}
		})
	}
	if got := outOfView(2, 3, 20, lanesOut...).plain(); got != "▲ 2 more · ▼ 3 more" {
		t.Errorf("in 20 cells, what's out of view reads %q, want the counts alone", got)
	}
}

// Short of (e), nothing's out of view, so nothing says so.
func TestRunwaySaysNothingIsOutOfViewWhereEveryLaneShows(t *testing.T) {
	f := runwayFrame(t, 37, lanesOf(10), thingsComing(3))
	if fit := f.runwayFitted(f.top()); fit.draws(fit.out) {
		t.Errorf("at step %d, every lane shown, what's out of view is said on row %d", fit.step, fit.out)
	}
	for y, row := range textOf(runwayDrawn(f, runwayDoc(10)).rows(Look{})) {
		if strings.Contains(row, "more account") {
			t.Errorf("every lane shown, row %d reads %q", y, row)
		}
	}
}

// runwayDoc is the router's document at routedAt of n accounts, a1 and on,
// each read, its 5-hour at a tenth used.
func runwayDoc(n int) status.Document {
	accounts := make([]status.Account, n)
	for i := range accounts {
		accounts[i] = routedAccount("a"+strconv.Itoa(i+1), routedWindow("5h", 0.1, routedClock(1, 17, 0)), routedWindow("7d", 0.1, routedClock(4, 21, 0)))
	}
	return status.Document{GeneratedAt: routedAt, Source: status.SourceRouter, Best: "a1", Router: status.Health{Healthy: true}, Accounts: accounts}.WorkedOut(claudeLike)
}

// At (e), the axis and the pool's foot hold, the lanes shown between them:
// the pool's lane on the row after the last lane shown's words, its room on
// the row under it, COMING UP on the row after that, at its least, a line
// and how many more, and the legend over the key line.
func TestRunwaysPoolsFootHoldsUnderTheLanesShownAtE(t *testing.T) {
	f := runwayFrame(t, 27, lanesOf(10), thingsComing(3))
	f.Scroll = 2
	fit := f.runwayFitted(f.top())
	last, _ := fit.laneRow(fit.first + fit.shown - 1)
	if fit.pool != last+2 || fit.room != fit.pool+runwayPoolRows || fit.coming != fit.room+1 || fit.lines != 2 || fit.legend != 27-underView {
		t.Errorf("at (e), the last lane shown's words are on row %d, the pool's lane on %d, its room on %d, COMING UP on %d with %d rows of lines, and the legend on %d; want the lane after the words, its room under it, COMING UP after it with 2, and the legend over the key line",
			last+1, fit.pool, fit.room, fit.coming, fit.lines, fit.legend)
	}
}

// On a phone, Runway takes the first step of its compaction whose parts fit,
// as it does at every width, its legend of two: three accounts with three
// lines of COMING UP, as the phone's frame, a row at a time from 37 to 15:
// three rows each to 32; (a) to 29; (b) to 27; (c) at 26; (d) at 25; then
// their lanes scrolling, two in view to 23 and one to 21; then one lane, the
// legend given way to 19, and COMING UP too to 16; and none under it; and
// ten, seven in view at 33, its lanes scrolling an account at a time.
func TestOnAPhoneRunwayTakesTheFirstOfItsStepsThatFits(t *testing.T) {
	noLegend := runwayFitted{step: runwayOneLane, shown: 1, coming: true, fits: true}
	bare := runwayFitted{step: runwayOneLane, shown: 1, fits: true}
	three := map[int]runwayFitted{
		37: fittedWhole(runwayRoomy, 3), 36: fittedWhole(runwayRoomy, 3), 35: fittedWhole(runwayRoomy, 3), 34: fittedWhole(runwayRoomy, 3),
		33: fittedWhole(runwayRoomy, 3), 32: fittedWhole(runwayRoomy, 3),
		31: fittedWhole(runwayTwoRows, 3), 30: fittedWhole(runwayTwoRows, 3), 29: fittedWhole(runwayTwoRows, 3),
		28: fittedWhole(runwayNoGaps, 3), 27: fittedWhole(runwayNoGaps, 3),
		26: fittedWhole(runwayComingCut, 3),
		25: fittedWhole(runwayAxisClose, 3),
		24: fittedWhole(runwayScrolled, 2), 23: fittedWhole(runwayScrolled, 2), 22: fittedWhole(runwayScrolled, 1), 21: fittedWhole(runwayScrolled, 1),
		20: noLegend, 19: noLegend, 18: bare, 17: bare, 16: bare,
		15: {},
	}
	type fitting struct {
		accounts, rows int
		want           runwayFitted
	}
	tests := []fitting{{accounts: 10, rows: 33, want: fittedWhole(runwayScrolled, 7)}}
	for rows := 37; rows >= 15; rows-- {
		tests = append(tests, fitting{accounts: 3, rows: rows, want: three[rows]})
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d at %d", tt.accounts, tt.rows), func(t *testing.T) {
			lanes := lanesOf(tt.accounts)
			lanes.Drawn = views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellCapped}, Resets: true}
			shown := phoneRunwayLive(views.RunwayFiveHour, 52)
			shown.Lanes, shown.Coming = lanes, thingsComing(3)
			f := Frame{Width: 52, Height: tt.rows, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: shown}
			fit := f.runwayFitted(f.top())
			if got := fittedOf(fit); got != tt.want {
				t.Errorf("at %d rows, Runway fits %+v, want %+v", tt.rows, got, tt.want)
			}
			if !fit.fits {
				return
			}
			holdsTheLegendOverTheKeyLine(t, fit, tt.rows, 2)
			if below := fit.pool + runwayPoolRows; fit.room != below {
				t.Errorf("the pool's room is on row %d, want the row under its lane, %d", fit.room, below)
			}
			if got, want := f.Scrolling(runwayDoc(tt.accounts), routedAt), (Scrolling{Most: tt.accounts - tt.want.shown, Page: tt.want.shown}); got != want {
				t.Errorf("Runway on a phone scrolls %+v, want %+v", got, want)
			}
		})
	}
}

// phoneRunwayLive is Runway's live view of the window given at routedAt on a
// phone width columns wide.
func phoneRunwayLive(w views.RunwayWindow, width int) RunwayShown {
	return RunwayShown{Window: w, Timeline: views.LiveTimeline(w, routedAt, RunwayShown{}.Columns(width))}
}
