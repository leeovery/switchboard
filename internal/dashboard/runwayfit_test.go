package dashboard

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayFrame is a frame showing Runway's live 5-hour view at routedAt, of a
// terminal 160 columns wide and as tall as given, in nord, of lanes as
// given, and COMING UP's lines.
func runwayFrame(t *testing.T, height int, lanes *views.Lanes, coming []status.Upcoming) Frame {
	t.Helper()
	tl := views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayColumns(160))
	return Frame{
		Width: 160, Height: height, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike,
		Runway: RunwayShown{Timeline: tl, Lanes: lanes, Coming: coming},
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
// as many accounts as each step holds, four, five, six, nine and ten, and
// one more, its lanes scrolling; and with three accounts, the frames' steps
// as rows run short, down to (e)'s least, one lane in view.
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
		{name: "seven, COMING UP folded at (c)", accounts: 7, lines: 8, rows: 37, want: runwayFolded, shown: 7},
		{name: "nine, at (c)", accounts: 9, lines: 10, rows: 37, want: runwayFolded, shown: 9},
		{name: "ten, at (d)", accounts: 10, lines: 11, rows: 37, want: runwayAxisClose, shown: 10},
		{name: "eleven, scrolling at (e)", accounts: 11, lines: 12, rows: 37, want: runwayScrolled, shown: 10},
		{name: "ten at 36, nine in view", accounts: 10, lines: 3, rows: 36, want: runwayScrolled, shown: 9},
		{name: "ten at 33, eight in view", accounts: 10, lines: 3, rows: 33, want: runwayScrolled, shown: 8},
		{name: "ten at 29, six in view", accounts: 10, lines: 3, rows: 29, want: runwayScrolled, shown: 6},
		{name: "ten at 24, three in view", accounts: 10, lines: 3, rows: 24, want: runwayScrolled, shown: 3},
		{name: "ten at 19, one in view", accounts: 10, lines: 3, rows: 19, want: runwayScrolled, shown: 1},
		{name: "the 5-hour frame's three", accounts: 3, lines: 3, rows: 37, want: runwayRoomy, shown: 3},
		{name: "the week frame's three", accounts: 3, lines: 7, rows: 37, want: runwayRoomy, shown: 3},
		{name: "the 5-hour's three at 33", accounts: 3, lines: 3, rows: 33, want: runwayRoomy, shown: 3},
		{name: "the week's three at 33", accounts: 3, lines: 7, rows: 33, want: runwayTwoRows, shown: 3},
		{name: "the 5-hour's three at 29", accounts: 3, lines: 3, rows: 29, want: runwayTwoRows, shown: 3},
		{name: "the week's three at 29", accounts: 3, lines: 7, rows: 29, want: runwayFolded, shown: 3},
		{name: "three at 24", accounts: 3, lines: 3, rows: 24, want: runwayFolded, shown: 3},
		{name: "three at 19, one in view", accounts: 3, lines: 3, rows: 19, want: runwayScrolled, shown: 1},
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

// Where not even (e)'s least, a lane in view, fits, as under 19 rows with a
// pool's lane, or before the lanes are built, Runway draws nothing but the
// page's chrome; one account, with no pool's lane, fits shorter.
func TestRunwayDrawsThePagesChromeAloneWhereNotOneLaneFits(t *testing.T) {
	tests := []struct {
		name  string
		lanes *views.Lanes
		rows  int
		fits  bool
	}{
		{name: "ten at 19", lanes: lanesOf(10), rows: 19, fits: true},
		{name: "ten at 18", lanes: lanesOf(10), rows: 18},
		{name: "two at 18", lanes: lanesOf(2), rows: 18},
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
// the view's top: three rows an account, the rule on the last one's blank
// row, the pool's room on its words' row, a blank row round the legend and
// COMING UP, as the 5-hour frame lays them out; a day stepped back to with
// neither COMING UP nor the room; ten at (d), as their frame lays them out;
// one account, its blank row the legend's; and on a phone, its room under
// its words, no blank row before its legend of two rows, as the phone's
// frame lays them out.
func TestRunwaysPartsLieOnTheRowsItsStepGivesThem(t *testing.T) {
	type rows struct{ lanes, apart, rule, pool, words, room, legend, coming, lines int }
	tests := []struct {
		name   string
		shown  RunwayShown
		width  int
		height int
		shape  runwayShape
		want   rows
	}{
		{
			name: "the 5-hour frame's three", shown: RunwayShown{Lanes: lanesOf(3), Coming: thingsComing(3)}, width: 160, height: 37,
			shape: runwayShape{legendGap: true, legend: 1}, want: rows{10, 3, 18, 19, 21, 21, 23, 25, 3},
		},
		{
			name: "Wednesday stepped back to", shown: RunwayShown{Back: civilDay(9, 30), Lanes: lanesOf(3)}, width: 160, height: 37,
			shape: runwayShape{legendGap: true, legend: 1}, want: rows{10, 3, 18, 19, 21, noRow, 23, noRow, 0},
		},
		{
			name: "the ten frame's ten", shown: RunwayShown{Lanes: lanesOf(10), Coming: thingsComing(9)}, width: 160, height: 37,
			shape: runwayShape{legendGap: true, legend: 1}, want: rows{9, 2, 29, 30, 32, 32, 33, 34, 0},
		},
		{
			name: "one account", shown: RunwayShown{Lanes: lanesOf(1), Coming: thingsComing(2)}, width: 160, height: 37,
			shape: runwayShape{legendGap: true, legend: 1}, want: rows{10, 3, noRow, noRow, noRow, noRow, 13, 15, 2},
		},
		{
			name: "the legend on two rows, at (a)", shown: RunwayShown{Lanes: lanesOf(5), Coming: thingsComing(5)}, width: 120, height: 37,
			shape: runwayShape{legendGap: true, legend: 2}, want: rows{10, 2, 20, 21, 23, 23, 25, 28, 5},
		},
		{
			name: "the phone frame's three", shown: RunwayShown{Lanes: lanesOf(3), Coming: thingsComing(3)}, width: 52, height: 33,
			shape: runwayShape{roomRow: true, legend: 2}, want: rows{11, 3, 19, 20, 22, 23, 24, 27, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := routedAt
			tt.shown.Timeline = tt.shown.TimelineAt(now, time.Monday, tt.width)
			f := Frame{Width: tt.width, Height: tt.height, View: Runway, Runway: tt.shown}
			fit := f.runwayFittedAs(f.top(), tt.shape)
			got := rows{fit.lanes, fit.apart, fit.rule, fit.pool, fit.words, fit.room, fit.legend, fit.coming, fit.lines}
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
		{name: "ten at 36, at the top", rows: 36, first: 0, says: "▼ 1 more account"},
		{name: "ten at 36, scrolled to the last", rows: 36, scroll: 1, first: 1, says: "▲ 1 more account"},
		{name: "ten at 36, scrolled past it", rows: 36, scroll: 5, first: 1, says: "▲ 1 more account"},
		{name: "ten at 29, scrolled by two", rows: 29, scroll: 2, first: 2, says: "▲ 2 more accounts · ▼ 2 more accounts"},
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
		t.Errorf("at (d), the row over the key line reads %q, want nothing", got)
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

// tenRunway is the ten world, as runway-ten-accounts-160 draws it, at
// routedAt: work at its cap, its 5-hour at 95%; personal, the primary, at
// 39%; side, where new sessions go, at 13%; client, spare and lab; team at
// its limit since 13:58 till 15:30; extra, ops and studio. The pool of ten
// has 5.6 left in the 5-hour.
func tenRunway() status.Document {
	type read struct {
		id   string
		used float64
		at   int
	}
	var accounts []status.Account
	for _, r := range []read{
		{"work", 0.95, 1710}, {"personal", 0.39, 1630}, {"side", 0.13, 1850}, {"client", 0.52, 1740}, {"spare", 0.04, 1935},
		{"lab", 0.71, 1655}, {"team", 1, 1530}, {"extra", 0.22, 1755}, {"ops", 0.06, 1910}, {"studio", 0.33, 1620},
	} {
		a := routedAccount(r.id, routedWindow("5h", r.used, routedClock(1, r.at/100, r.at%100)), routedWindow("7d", 0.3, routedClock(2, 10, 0)))
		accounts = append(accounts, a)
	}
	accounts[0].Reserve = 0.05
	accounts[1].Primary = true
	team := &accounts[6]
	team.Windows[0].Status = quota.StatusRejected
	team.Limit = status.Limit{Windows: []string{"5h"}, Until: routedClock(1, 15, 30)}
	return status.Document{GeneratedAt: routedAt, Source: status.SourceRouter, Best: "side", Router: status.Health{Healthy: true}, Sessions: 14, Accounts: accounts}.WorkedOut(claudeLike)
}

// tenPool is the ten world's pool's lane, as runway-ten-accounts-160 draws
// it: all ten with room till team's limit at 13:58, nine till work's cap at
// 14:31:03, eight till team's back at 15:30, nine till lab runs out at
// ~16:05, eight till its 5-hour resets at 16:55, nine till work's resets at
// 17:10, then ten.
func tenPool() views.Pool {
	steps := []views.Step{
		{With: 9, From: views.Edge{At: routedClock(1, 13, 58)}, Until: views.Edge{At: routedAt.Add(-11*time.Minute - 4*time.Second)}, First: 2, Last: 4},
		{With: 8, From: views.Edge{At: routedAt.Add(-11*time.Minute - 4*time.Second)}, Until: views.Edge{At: routedClock(1, 15, 30)}, First: 5, Last: 10},
		{With: 9, From: views.Edge{At: routedClock(1, 15, 30)}, Until: views.Edge{At: routedClock(1, 16, 5), Projected: true}, First: 11, Last: 13},
		{With: 8, From: views.Edge{At: routedClock(1, 16, 5), Projected: true}, Until: views.Edge{At: routedClock(1, 16, 55)}, First: 14, Last: 18},
		{With: 9, From: views.Edge{At: routedClock(1, 16, 55)}, Until: views.Edge{At: routedClock(1, 17, 10)}, First: 19, Last: 20},
	}
	return views.Pool{
		Of: 10, Key: "5h", Columns: poolColumnsOf(136, 10, 0, 9, 2, 8, 5, 9, 11, 8, 14, 9, 19, 10, 21),
		Dips: []views.Dip{{Steps: steps, Column: 1, First: 2, Last: 20}},
	}
}

// poolColumnsOf are n of the pool's columns, each known, counted in runs: a
// count with room, then the column its run starts at, and so on.
func poolColumnsOf(n int, runs ...int) []views.PoolColumn {
	columns := make([]views.PoolColumn, n)
	for i := 0; i < len(runs); i += 2 {
		for c := runs[i+1]; c < n; c++ {
			columns[c] = views.PoolColumn{With: runs[i], Known: true}
		}
	}
	return columns
}

// At (e), the axis and the pool's foot hold, the lanes shown between them:
// the pool's rule on the row after the last lane shown's words, and the rest
// of the foot a row after another, COMING UP folded.
func TestRunwaysPoolsFootHoldsUnderTheLanesShownAtE(t *testing.T) {
	f := runwayFrame(t, 29, lanesOf(10), thingsComing(3))
	f.Scroll = 2
	fit := f.runwayFitted(f.top())
	last, _ := fit.laneRow(fit.first + fit.shown - 1)
	if fit.rule != last+2 || fit.pool != fit.rule+1 || fit.words != fit.pool+runwayPoolRows || fit.legend != fit.words+1 || fit.coming != fit.legend+1 || !fit.folded {
		t.Errorf("at (e), the last lane shown's words are on row %d, the rule on %d, the pool's lane on %d, its words on %d, the legend on %d and COMING UP on %d, folded %t; want each on the row after the one before, folded",
			last+1, fit.rule, fit.pool, fit.words, fit.legend, fit.coming, fit.folded)
	}
}
