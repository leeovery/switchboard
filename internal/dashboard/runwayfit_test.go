package dashboard

import (
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

// Runway takes the first step of its compaction whose last row is two above
// the key line or higher, with a line of COMING UP an account: at 160 by 37,
// as many accounts as each step holds, four, five and ten, and one more, its
// lanes scrolling; (b) where COMING UP is longer; (d) a row taller; and the
// week frame's three as rows run short.
func TestRunwayTakesTheFirstOfItsStepsThatFits(t *testing.T) {
	tests := []struct {
		name                  string
		accounts, lines, rows int
		want                  runwayStep
		shown                 int
	}{
		{name: "four accounts, three rows each", accounts: 4, lines: 5, rows: 37, want: runwayRoomy, shown: 4},
		{name: "five, at (a)", accounts: 5, lines: 6, rows: 37, want: runwayTwoRows, shown: 5},
		{name: "six, at (b)", accounts: 6, lines: 7, rows: 37, want: runwayNoGaps, shown: 6},
		{name: "six with nine lines, at (c)", accounts: 6, lines: 9, rows: 37, want: runwayFolded, shown: 6},
		{name: "seven, COMING UP folded at (c)", accounts: 7, lines: 8, rows: 37, want: runwayFolded, shown: 7},
		{name: "ten, at (c)", accounts: 10, lines: 11, rows: 37, want: runwayFolded, shown: 10},
		{name: "eleven at 38, at (d)", accounts: 11, lines: 12, rows: 38, want: runwayAxisClose, shown: 11},
		{name: "eleven, scrolling at (e)", accounts: 11, lines: 12, rows: 37, want: runwayScrolled, shown: 10},
		{name: "ten at 34, nine in view", accounts: 10, lines: 3, rows: 34, want: runwayScrolled, shown: 9},
		{name: "ten at 31, seven in view", accounts: 10, lines: 3, rows: 31, want: runwayScrolled, shown: 7},
		{name: "ten at 27, five in view", accounts: 10, lines: 3, rows: 27, want: runwayScrolled, shown: 5},
		{name: "ten at 22, three in view", accounts: 10, lines: 3, rows: 22, want: runwayScrolled, shown: 3},
		{name: "ten at 18, one in view", accounts: 10, lines: 3, rows: 18, want: runwayScrolled, shown: 1},
		{name: "the 5-hour frame's three", accounts: 3, lines: 3, rows: 37, want: runwayRoomy, shown: 3},
		{name: "the week frame's three", accounts: 3, lines: 7, rows: 37, want: runwayRoomy, shown: 3},
		{name: "the week's three at 33", accounts: 3, lines: 7, rows: 33, want: runwayTwoRows, shown: 3},
		{name: "the week's three at 30", accounts: 3, lines: 7, rows: 30, want: runwayNoGaps, shown: 3},
		{name: "the week's three at 29", accounts: 3, lines: 7, rows: 29, want: runwayFolded, shown: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayFrame(t, tt.rows, lanesOf(tt.accounts), thingsComing(tt.lines))
			fit := f.runwayFitted(f.top())
			if !fit.fits || fit.step != tt.want || fit.shown != tt.shown {
				t.Errorf("at %d rows, Runway fits at step %d, %d of %d lanes shown, fitting %t; want step %d, %d shown", tt.rows, fit.step, fit.shown, tt.accounts, fit.fits, tt.want, tt.shown)
			}
			if last := fit.end - 1; last > tt.rows-3 {
				t.Errorf("its last row is %d, past row %d, two above the key line", last, tt.rows-3)
			}
		})
	}
}

// The 5-hour frame's three accounts, with three lines of COMING UP, take each
// step as rows run short, a row at a time from 37 to 15: three rows each to
// 31, the pool's lane taking its own rows and its room's; (a) to 28; (b) to
// 26; (c) to 23; (d) at 22; then their lanes scrolling, two in view to 20
// and one to 18; and under that, nothing but the page's chrome.
func TestRunwayFitsAtEveryHeightDownTo15Rows(t *testing.T) {
	type fitted struct {
		step  runwayStep
		shown int
		fits  bool
	}
	at := func(step runwayStep, shown int) fitted { return fitted{step, shown, true} }
	want := map[int]fitted{
		37: at(runwayRoomy, 3), 36: at(runwayRoomy, 3), 35: at(runwayRoomy, 3), 34: at(runwayRoomy, 3),
		33: at(runwayRoomy, 3), 32: at(runwayRoomy, 3), 31: at(runwayRoomy, 3),
		30: at(runwayTwoRows, 3), 29: at(runwayTwoRows, 3), 28: at(runwayTwoRows, 3),
		27: at(runwayNoGaps, 3), 26: at(runwayNoGaps, 3),
		25: at(runwayFolded, 3), 24: at(runwayFolded, 3), 23: at(runwayFolded, 3),
		22: at(runwayAxisClose, 3),
		21: at(runwayScrolled, 2), 20: at(runwayScrolled, 2), 19: at(runwayScrolled, 1), 18: at(runwayScrolled, 1),
		17: {}, 16: {}, 15: {},
	}
	for rows := 37; rows >= 15; rows-- {
		t.Run(strconv.Itoa(rows), func(t *testing.T) {
			f := runwayFrame(t, rows, lanesOf(3), thingsComing(3))
			fit := f.runwayFitted(f.top())
			if got := (fitted{fit.step, fit.shown, fit.fits}); got != want[rows] {
				t.Errorf("at %d rows, Runway fits %+v, want %+v", rows, got, want[rows])
			}
			if fit.fits && fit.end > rows-underView {
				t.Errorf("at %d rows, its last row is %d, past row %d, two above the key line", rows, fit.end-1, rows-underView-1)
			}
		})
	}
}

// Where not even (e)'s least, a lane in view, fits, as under 18 rows with a
// pool's lane, or before the lanes are built, Runway draws nothing but the
// page's chrome; one account, with no pool's lane, fits shorter.
func TestRunwayDrawsThePagesChromeAloneWhereNotOneLaneFits(t *testing.T) {
	tests := []struct {
		name  string
		lanes *views.Lanes
		rows  int
		fits  bool
	}{
		{name: "ten at 18", lanes: lanesOf(10), rows: 18, fits: true},
		{name: "ten at 17", lanes: lanesOf(10), rows: 17},
		{name: "two at 17", lanes: lanesOf(2), rows: 17},
		{name: "one at 15", lanes: lanesOf(1), rows: 15, fits: true},
		{name: "one at 14", lanes: lanesOf(1), rows: 14},
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
// a blank row round the legend and COMING UP, as the 5-hour frame lays them
// out; a day
// stepped back to with neither COMING UP nor the room; ten at (c), the
// pool's lane under the last one's words; one account, its blank row the
// legend's; and on a phone alike, its legend of two rows.
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
			shape: runwayShape{legend: 1}, want: rows{10, 3, 19, 21, 23, 25, 3},
		},
		{
			name: "Wednesday stepped back to", shown: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(9, 30), Lanes: lanesOf(3)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{10, 3, 19, noRow, 22, noRow, 0},
		},
		{
			name: "the ten frame's ten", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(10), Coming: thingsComing(9)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{10, 2, 30, 32, 33, 34, 0},
		},
		{
			name: "one account", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(1), Coming: thingsComing(2)}, width: 160, height: 37,
			shape: runwayShape{legend: 1}, want: rows{10, 3, noRow, noRow, 13, 15, 2},
		},
		{
			name: "the legend on two rows, at (a)", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(5), Coming: thingsComing(5)}, width: 120, height: 37,
			shape: runwayShape{legend: 2}, want: rows{10, 2, 20, 22, 24, 27, 5},
		},
		{
			name: "the phone frame's three", shown: RunwayShown{Window: views.RunwayFiveHour, Lanes: lanesOf(3), Coming: thingsComing(3)}, width: 52, height: 33,
			shape: runwayShape{legend: 2}, want: rows{11, 3, 20, 22, 24, 27, 3},
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
// the last, the axis and the pool's foot holding; and over the key line, it
// says what's out of view, right-aligned, in text.subtle, as a scroll line
// is: above and below, or where that doesn't fit, the counts alone.
func TestRunwaysLanesScrollAnAccountAtATimeAndSayWhatsOutOfView(t *testing.T) {
	tests := []struct {
		name         string
		rows, scroll int
		first        int
		says         string
	}{
		{name: "ten at 34, at the top", rows: 34, first: 0, says: "▼ 1 more account"},
		{name: "ten at 34, scrolled to the last", rows: 34, scroll: 1, first: 1, says: "▲ 1 more account"},
		{name: "ten at 34, scrolled past it", rows: 34, scroll: 5, first: 1, says: "▲ 1 more account"},
		{name: "ten at 27, scrolled by two", rows: 27, scroll: 2, first: 2, says: "▲ 2 more accounts · ▼ 3 more accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runwayDoc(10)
			f := runwayFrame(t, tt.rows, lanesOf(10), thingsComing(3))
			f.Scroll = tt.scroll
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
			c := runwayDrawn(f, doc)
			over := tt.rows - underView
			if got, want := rowOf(c, over), strings.Repeat(" ", 160-len([]rune(tt.says)))+tt.says; got != want {
				t.Errorf("the row over the key line reads %q, want %q", got, want)
			}
			if got := c.at(159, over).ink; got != dimInk {
				t.Errorf("what's out of view is in %+v, want text.subtle", got)
			}
		})
	}
	if got := outOfView(2, 3, 20, lanesOut...).plain(); got != "▲ 2 more · ▼ 3 more" {
		t.Errorf("in 20 cells, what's out of view reads %q, want the counts alone", got)
	}
}

// Short of (e), nothing's said over the key line, as nothing is out of view.
func TestRunwaySaysNothingOverTheKeyLineWhereEveryLaneShows(t *testing.T) {
	f := runwayFrame(t, 37, lanesOf(10), thingsComing(3))
	if got := rowOf(runwayDrawn(f, runwayDoc(10)), 37-underView); got != "" {
		t.Errorf("at (c), the row over the key line reads %q, want nothing", got)
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
// the row under it, and the legend and COMING UP a row after another under
// that, COMING UP folded.
func TestRunwaysPoolsFootHoldsUnderTheLanesShownAtE(t *testing.T) {
	f := runwayFrame(t, 27, lanesOf(10), thingsComing(3))
	f.Scroll = 2
	fit := f.runwayFitted(f.top())
	last, _ := fit.laneRow(fit.first + fit.shown - 1)
	if fit.pool != last+2 || fit.room != fit.pool+runwayPoolRows || fit.legend != fit.room+1 || fit.coming != fit.legend+1 || !fit.folded {
		t.Errorf("at (e), the last lane shown's words are on row %d, the pool's lane on %d, its room on %d, the legend on %d and COMING UP on %d, folded %t; want the lane after the words, its room under it, and each after on the row after the one before, folded",
			last+1, fit.pool, fit.room, fit.legend, fit.coming, fit.folded)
	}
}
