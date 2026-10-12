package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// trailFilled is the trail of a window from start, length long, that a chart
// width cells wide and h rows tall fills as filled gives it, a column's
// eighths each, from its first to now's: a reading at each column's middle,
// and now's at now.
func trailFilled(start time.Time, length time.Duration, width, h int, filled []int) Trail {
	var trail Trail
	for col, eighths := range filled {
		at := start.Add(time.Duration((float64(col) + 0.5) / float64(width) * float64(length)))
		if col == len(filled)-1 {
			at = printNow
		}
		trail.Readings = append(trail.Readings, score.Reading{At: at, Utilization: float64(eighths) / float64(8*h)})
	}
	return trail
}

// trailsOf are the trails of account a's windows with the keys given, each
// charted width cells wide and h rows tall, filled as filled gives each, in
// that order.
func trailsOf(t *testing.T, a status.Account, keys []string, width, h int, filled ...[]int) Trails {
	t.Helper()
	trails := make(Trails)
	for i, key := range keys {
		w, _ := a.Window(key)
		start, length, ok := w.Span()
		if !ok {
			t.Fatalf("%s's %s has no span", a.ID, key)
		}
		trails[Ref{Account: a.ID, Window: key}] = trailFilled(start, length, width, h, filled[i])
	}
	return trails
}

// down are the cells of column col from row top to bottom.
func down(col, top, bottom int) []spot {
	var spots []spot
	for row := top; row <= bottom; row++ {
		spots = append(spots, spot{row, col, col})
	}
	return spots
}

// graphsDrawn is GRAPHS drawn by f of doc at printNow on a canvas of f's
// size, its description from column 9 of row heading, as its heading has it
// after "GRAPHS" and three spaces, and its body from two rows under, rows
// tall.
func graphsDrawn(f Frame, doc status.Document, heading, rows int) *canvas {
	s := f.graphsSection(doc, printNow)
	c := newCanvas(f.Width, f.Height)
	c.line(len("GRAPHS   "), heading, s.description())
	s.draw(c, bounds{y: heading + sectionHead, width: f.Width, rows: rows})
	return c
}

func TestGRAPHSIsDrawnAsItsFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		stem          string
		width, height int
		// heading is the row GRAPHS' heading is on, and h its charts' rows.
		heading, h int
		// chart is how many cells wide each chart is.
		chart int
		// filled are the eighths of each chart's columns, the 5-hour's, the
		// week's and Fable's week's, to now's, as the frame fills them.
		filled [3][]int
		// whole are the cells the frame draws otherwise than the design.
		whole []spot
	}{
		{
			stem: "overview-everything-160x39", width: 160, height: 39, heading: 18, h: leastChart, chart: 45,
			filled: [3][]int{
				{1, 4, 7, 9, 10, 11, 12, 14, 16, 19, 21, 22, 23, 24, 26, 29, 32, 34, 35, 36, 37, 38, 38, 38},
				{0, 0, 0, 1, 3, 4, 4, 4, 4, 4, 6, 7, 7, 8, 8, 8, 9, 10, 11, 11, 11, 11, 11, 13, 14},
				slices.Concat(slices.Repeat([]int{0}, 5), slices.Repeat([]int{2}, 12), slices.Repeat([]int{5}, 8)),
			},
		},
		{
			stem: "overview-sessions-graphs-216", width: 216, height: 70, heading: 18, h: mostChart, chart: 64,
			filled: [3][]int{
				{3, 9, 17, 24, 31, 36, 38, 40, 42, 46, 51, 58, 66, 73, 79, 83, 86, 87, 90, 93, 99, 106, 114, 121, 127, 131, 133, 135, 137, 141, 144, 144, 144},
				{0, 0, 0, 1, 1, 6, 11, 13, 14, 15, 15, 15, 15, 15, 19, 24, 27, 28, 28, 29, 29, 29, 29, 33, 38, 40, 42, 42, 42, 42, 42, 42, 46, 50, 52},
				slices.Concat(slices.Repeat([]int{0}, 7), []int{7}, slices.Repeat([]int{9}, 16), []int{15}, slices.Repeat([]int{18}, 10)),
			},
			// The frame fills the 5-hour chart to column 33, a column past
			// now's, where min(round(0.507 × 64), 63) puts now in 32, and
			// draws now a cell on with it. Its chart at 160 columns agrees
			// with the design.
			whole: append(down(49, 22, 39), spot{40, 47, 50}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			doc := trio()
			work, _ := doc.Account("work")
			f := Frame{
				Width: tt.width, Height: tt.height, Look: nord, View: Overview, Focus: "work",
				Trails: trailsOf(t, work, []string{"5h", "7d", "7d_oi"}, tt.chart, tt.h, tt.filled[:]...),
			}
			drawn := screenOf(t, graphsDrawn(f, doc, tt.heading, tt.h+2).rows(nord))
			frame, canvas := reference(t, tt.stem), shadeOf(nord.Canvas())
			heldTo(t, tt.stem, frame, drawn, region{top: tt.heading, bottom: tt.heading, left: 9, right: 46}, canvas, nil, nil)
			body := region{top: tt.heading + sectionHead, bottom: tt.heading + sectionHead + tt.h + 1, left: 0, right: tt.width - 1}
			heldTo(t, tt.stem, frame, drawn, body, canvas, tt.whole, nil)
		})
	}
}

// chartAt is a chart of a window ten hours long, from 08:00, of an account
// capped at capAt, where it's under 1, at now, its readings as given.
func chartAt(now time.Time, capAt float64, readings ...score.Reading) chart {
	start := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	return chart{name: "10-HOUR", start: start, length: 10 * time.Hour, spanned: true, readings: readings, capAt: capAt, capped: capAt < 1, now: now}
}

// readOn is a reading of use, at the time given on 1 October 2026.
func readOn(hour, minute int, used float64) score.Reading {
	return score.Reading{At: time.Date(2026, 10, 1, hour, minute, 0, 0, time.UTC), Utilization: used}
}

// chartIn is ch drawn width cells wide in look, h rows of chart between its
// top row and its axis, on a canvas of its own.
func chartIn(ch chart, look Look, width, h int) *canvas {
	c := newCanvas(width, h+2)
	ch.draw(c, look, bounds{width: width, rows: h + 2})
	return c
}

// filledOf are the eighths each column of a chart h rows tall on c is
// filled to, its first to its last, read back from its cells: a cell with a
// background counts eight, and the top one its eighth block.
func filledOf(c *canvas, h int) []int {
	filled := make([]int, c.width)
	for x := range c.width {
		for r := range h {
			here := c.at(x, h-r)
			if here.ink.on != (hue{}) {
				filled[x] += 8
				continue
			}
			if i := slices.Index(levels[:], here.glyph); i > 0 && here.ink.ramp {
				filled[x] += i
			}
			break
		}
	}
	return filled
}

func TestAChartFillsEachColumnToTheLastUseReadByItsMiddleAndNowsByNow(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	// Ten columns of an hour each, from 08:00, two rows, 16 eighths: their
	// middles are at half past each hour; now, 12:40, falls in column
	// round(4.67) = 5.
	ch := chartAt(time.Date(2026, 10, 1, 12, 40, 0, 0, time.UTC), 1,
		readOn(8, 40, 0.25),     // after column 0's middle: column 0 has nothing read
		readOn(9, 30, 0.5),      // at column 1's middle
		readOn(10, 45, 0.53125), // column 3's: 8.5 eighths, a half to even, 8
		readOn(12, 35, 0.97),    // after column 4's middle, by now: now's, 15.52, 16
		readOn(12, 50, 0.1),     // after now: none of it shows
	)
	got := filledOf(chartIn(ch, nord, 10, 2), 2)
	if want := []int{0, 8, 8, 8, 8, 16, 0, 0, 0, 0}; !slices.Equal(got, want) {
		t.Errorf("the columns are filled %v eighths, want %v: each to the last use read by its middle, now's by now, none past now", got, want)
	}
}

func TestAChartsFillIsTheRampAtEachRowsHeight70PercentShowing(t *testing.T) {
	ch := chartAt(time.Date(2026, 10, 1, 8, 59, 0, 0, time.UTC), 1, readOn(8, 0, 0.6))
	c := chartIn(ch, Screen(builtin(t, "nord")), 10, 4)
	// 0.6 of 4 rows is 19.2 eighths, 19: two full rows, and three eighths.
	for r, want := range []cell{
		{glyph: " ", ink: ink{on: hue{ramp: true, at: 0.125, fade: 0.3}}},
		{glyph: " ", ink: ink{on: hue{ramp: true, at: 0.375, fade: 0.3}}},
		{glyph: levels[3], ink: ink{ramp: true, at: 0.625, fade: 0.3}},
	} {
		if got := *c.at(0, 4-r); got != want {
			t.Errorf("row %d up is %+v, want %+v: the ramp at (r + 0.5) / h, 70%% of it showing, behind a full cell", r, got, want)
		}
	}
	if got := chartIn(ch, NoColour(), 10, 4).at(0, 4).glyph; got != levels[8] {
		t.Errorf("without colour, a full cell is %q, want %q", got, levels[8])
	}
}

func TestAChartsLinesTakeTheEmptyCellsTheCapsThenEvenPacesThenNows(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	// Nine columns, four rows: the cap at 0.8 is min(⌊3.2⌋, 3) = 3 rows up;
	// even pace is min(⌊i × 4 ÷ 8⌋, 3) rows up in column i; now, 12:00, is
	// column round(3.6) = 4, where even pace is 2 rows up; the fill, 0.1 of
	// 32 eighths, is 3 eighths to now.
	ch := chartAt(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), 0.8, readOn(8, 0, 0.1))
	c := chartIn(ch, nord, 9, 4)
	var got []string
	for y := 1; y <= 4; y++ {
		got = append(got, rowOf(c, y))
	}
	want := []string{"╌╌╌╌╌╌╌╌╌", "    ··", "  ··│", "▃▃▃▃▃"}
	if !slices.Equal(got, want) {
		t.Errorf("the chart's rows are\n%s\nwant\n%s\nthe cap over even pace, even pace over now, and neither over the fill",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, tt := range []struct {
		x, y int
		want ink
	}{{0, 1, reserveInk}, {2, 3, dimInk}, {4, 3, faintInk}} {
		if got := c.at(tt.x, tt.y); got.ink != tt.want {
			t.Errorf("%q is in %+v, want %+v", got.glyph, got.ink, tt.want)
		}
	}
	if uncapped := chartIn(chartAt(ch.now, 1, readOn(8, 0, 0.1)), nord, 9, 4); strings.Contains(rowOf(uncapped, 1), capLine) {
		t.Errorf("an account without a cap has its line: %q", rowOf(uncapped, 1))
	}
}

func TestAChartsAxisIsItsStartNowAndItsReset(t *testing.T) {
	week := chart{
		name: "WEEK", start: time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC), length: 7 * day, spanned: true,
		readings: []score.Reading{readOn(8, 0, 0.1)}, now: time.Date(2026, 10, 1, 14, 42, 0, 0, time.UTC),
	}
	tests := []struct {
		name  string
		ch    chart
		width int
		want  string
	}{
		{name: "a window of a day or less: times of day", ch: chartAt(time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC), 1, readOn(8, 0, 0.1)), width: 30,
			want: "08:00        now         18:00"},
		{name: "a longer one: days and times", ch: week, width: 45,
			want: "Sun 21:00              now          Sun 21:00"},
		{name: "now left out where it doesn't clear the start", ch: chartAt(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), 1, readOn(8, 0, 0.1)), width: 30,
			want: "08:00                    18:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rowOf(chartIn(tt.ch, Look{}, tt.width, 2), 3); got != tt.want {
				t.Errorf("the axis is %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAWindowWithNoHistorySaysSo(t *testing.T) {
	ch := chartAt(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), 1)
	c := chartIn(ch, Screen(builtin(t, "nord")), 30, 5)
	if got := rowOf(c, 3); !strings.Contains(got, untracedLabel) || c.at(strings.Index(got, "no"), 3).ink != dimInk {
		t.Errorf("the middle row is %q, want \"no history yet\" across it in text.subtle", got)
	}
	if got := filledOf(c, 5); slices.Max(got) != 0 {
		t.Errorf("the columns are filled %v, want none, as nothing was read", got)
	}
	if got := rowOf(c, 6); !strings.HasPrefix(got, "08:00") {
		t.Errorf("the axis is %q, want it drawn still", got)
	}
}

func TestAWindowResetSinceItWasReadIsChartedFromItsReset(t *testing.T) {
	work, _ := trio().Account("work")
	window, _ := work.Window("5h")
	before := Trail{Readings: []score.Reading{{At: window.ResetsAt.Add(-time.Hour), Utilization: 0.5}}}
	got := chartOf(work, shownWindow{key: "5h", name: "5-hour"}, before, window.ResetsAt.Add(time.Hour))
	if len(got.readings) != 0 || !got.start.Equal(window.ResetsAt) {
		t.Errorf("charted from %v with %d readings, want from its reset, %v, with none of before it", got.start, len(got.readings), window.ResetsAt)
	}
}

func TestGRAPHSPutsAChartUnderEachWindowOfUSAGEOrStacksThemOnAPhone(t *testing.T) {
	doc := trio()
	tests := []struct {
		name        string
		width, rows int
		want        []bounds
		least, most int
	}{
		{name: "160 columns: 45 cells every 48", width: 160, rows: 7,
			want: []bounds{{16, 0, 45, 7}, {64, 0, 45, 7}, {112, 0, 45, 7}}, least: 9, most: 23},
		{name: "216 columns: 64 cells every 67", width: 216, rows: 21,
			want: []bounds{{16, 0, 64, 21}, {83, 0, 64, 21}, {150, 0, 64, 21}}, least: 9, most: 23},
		{name: "under 150 columns, still side by side", width: 120, rows: 7,
			want: []bounds{{16, 0, 32, 7}, {51, 0, 32, 7}, {86, 0, 32, 7}}, least: 9, most: 23},
		{name: "a phone: stacked, from the name column to the edge", width: 52, rows: 21,
			want: []bounds{{16, 0, 35, 7}, {16, 7, 35, 7}, {16, 14, 35, 7}}, least: 2 + 3*7, most: 2 + 3*21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 70, Focus: "work"}
			g := f.graphsOf(doc, printNow)
			var got []bounds
			for i := range g.charts {
				got = append(got, g.placed(i, bounds{width: tt.width, rows: tt.rows}))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the charts are placed %v, want %v", got, tt.want)
			}
			if s := f.graphsSection(doc, printNow); s.least != tt.least || s.most != tt.most {
				t.Errorf("GRAPHS takes %d to %d rows, want %d to %d", s.least, s.most, tt.least, tt.most)
			}
		})
	}
}

func TestGRAPHSChartsTheFocusedAccountOrTheFirst(t *testing.T) {
	doc := trio()
	for _, tt := range []struct{ focus, want string }{{"personal", "personal"}, {"", "work"}, {"gone", "work"}} {
		g := Frame{Width: 160, Height: 39, Focus: tt.focus}.graphsSection(doc, printNow)
		want := line{
			{tt.want, titleInk}, spaces(5),
			{"·", dimInk}, {" even pace   ", faintInk}, {"╌", reserveInk}, {" cap   ┄ limit", faintInk},
		}
		if got := g.description(); !slices.Equal(got, want) {
			t.Errorf("focused on %q, the description is %+v, want %+v", tt.focus, got, want)
		}
	}
	if s := (Frame{Width: 160, Height: 39}).graphsSection(status.Document{}, printNow); s.description() != nil || s.folded() != nil {
		t.Errorf("with no account, GRAPHS says %+v, folded %+v, want nothing", s.description(), s.folded())
	}
}

func TestGRAPHSFoldedSaysWhoseAndThatTheRowsHaveNoRoom(t *testing.T) {
	g := Frame{Width: 160, Height: 35, Focus: "work"}.graphsSection(trio(), printNow)
	want := line{{"work", titleInk}, spaces(3), {"no room at 35 rows", dimInk}}
	if got := g.folded(); !slices.Equal(got, want) {
		t.Errorf("GRAPHS folded says %+v, want %+v", got, want)
	}
}

func TestGRAPHSByAccountDrawsAsByWindow(t *testing.T) {
	doc := trio()
	drawn := func(by By) []string {
		f := Frame{Width: 160, Height: 39, Look: Screen(builtin(t, "nord")), By: by, Focus: "work"}
		return graphsDrawn(f, doc, 0, 9).rows(f.Look)
	}
	if byWindow, byAccount := drawn(ByWindow), drawn(ByAccount); !slices.Equal(byWindow, byAccount) {
		t.Errorf("by account, GRAPHS draws\n%s\nwant as by window\n%s", strings.Join(byAccount, "\n"), strings.Join(byWindow, "\n"))
	}
}
