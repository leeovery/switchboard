package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

func TestATracesCellsFillEighthsOfItsRowsToItsScale(t *testing.T) {
	// Two rows are 16 eighths: a request a minute fills two, at a scale of 8.
	cells := []float64{0, 0.25, 1, 0.75, 4, 6, 8, 20}
	c := newCanvas(len(cells), 2)
	Frame{Look: Screen(builtin(t, "nord"))}.drawCells(c, cells, bounds{width: len(cells), rows: 2}, 8, false)
	fill, lane := traceInks(false)
	full := cell{glyph: " ", ink: ink{on: fill.hue()}}
	tests := []struct {
		name     string
		col      int
		top, bot cell
	}{
		{name: "empty: its lane, ▁ on the bottom row", col: 0, top: blank, bot: cell{glyph: traceLane, ink: lane}},
		{name: "half an eighth, rounding to the even none: empty", col: 1, top: blank, bot: cell{glyph: traceLane, ink: lane}},
		{name: "two eighths", col: 2, top: blank, bot: cell{glyph: levels[2], ink: fill}},
		{name: "an eighth and a half, rounding to the even two", col: 3, top: blank, bot: cell{glyph: levels[2], ink: fill}},
		{name: "a row's eight: full, its colour behind it", col: 4, top: blank, bot: full},
		{name: "twelve: a full cell, four eighths on top", col: 5, top: cell{glyph: levels[4], ink: fill}, bot: full},
		{name: "the scale: both rows full", col: 6, top: full, bot: full},
		{name: "past the scale: no fuller", col: 7, top: full, bot: full},
	}
	for _, tt := range tests {
		if got := *c.at(tt.col, 0); got != tt.top {
			t.Errorf("%s: top row %+v, want %+v", tt.name, got, tt.top)
		}
		if got := *c.at(tt.col, 1); got != tt.bot {
			t.Errorf("%s: bottom row %+v, want %+v", tt.name, got, tt.bot)
		}
	}
}

func TestATraceIsDrawnInTheFramesColours(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	// The frames' colours: accent.key at 80%, and the empty lane's
	// text.faint; and out of the pool, each keeping 35% of that.
	tests := []struct {
		name        string
		faded       bool
		fill, empty shade
	}{
		{name: "in the pool", fill: shade{r: 112, g: 139, b: 167, set: true}, empty: shade{r: 76, g: 86, b: 106, set: true}},
		{name: "out of the pool", faded: true, fill: shade{r: 69, g: 82, b: 100, set: true}, empty: shade{r: 56, g: 64, b: 79, set: true}},
	}
	for _, tt := range tests {
		c := newCanvas(3, 1)
		Frame{Look: nord}.drawCells(c, []float64{8, 4, 0}, bounds{width: 3, rows: 1}, 8, tt.faded)
		cells := screenOf(t, c.rows(nord))[0]
		if got := cells[0].bg; !got.near(tt.fill) {
			t.Errorf("%s: a full cell is on %s, want %s", tt.name, got, tt.fill)
		}
		if got := cells[1].fg; !got.near(tt.fill) || cells[1].glyph != '▄' {
			t.Errorf("%s: half a cell is %s, want ▄ in %s", tt.name, cells[1], tt.fill)
		}
		if got := cells[2].fg; !got.near(tt.empty) || cells[2].glyph != '▁' {
			t.Errorf("%s: an empty cell is %s, want ▁ in %s", tt.name, cells[2], tt.empty)
		}
	}
}

func TestWithoutBlendingATracesFullCellIsABlock(t *testing.T) {
	c := newCanvas(1, 1)
	Frame{Look: NoColour()}.drawCells(c, []float64{8}, bounds{width: 1, rows: 1}, 8, false)
	if got := c.at(0, 0).glyph; got != levels[8] {
		t.Errorf("a full cell is %q, want %q", got, levels[8])
	}
}

func TestATracesRateIsBoldUnlessNoneOrOutOfThePool(t *testing.T) {
	tests := []struct {
		n     int
		faded bool
		want  span
	}{
		{n: 15, want: span{"15/min", strongInk}},
		{n: 0, want: span{"0/min", dimInk}},
		{n: 15, faded: true, want: span{"15/min", faintInk}},
	}
	for _, tt := range tests {
		if got := rateOf(tt.n, tt.faded); got != tt.want {
			t.Errorf("rateOf(%d, %t) = %v, want %v", tt.n, tt.faded, got, tt.want)
		}
	}
}

// tracedWorld is the auto world, the sessions the router lists, and the
// requests of the 30 minutes to routedAt, as the request stream tells of
// them: on personal, 4 of personal-0's at 14:30:01 and 2 at 14:41:31; on
// side, 16 of side-0's at 14:41:31; and on each alike, 12 of side-1's at
// 14:20:01, and 15 at 14:15:01 of gone's, a session no longer listed. At
// 160 columns, a cell is 30 seconds, and a burst fills the two cells whose
// four samples, the last at a cell's end, all fall in the minute after it:
// the newest cell's, taken at routedAt, its rate.
func tracedWorld() (status.Document, []status.Session, views.Traces) {
	doc, listed := autoRouting()
	var sent []views.Sent
	burst := func(session, account string, n int, at time.Time) {
		for i := range n {
			id := fmt.Sprintf("%s-%s-%s-%d", session, account, at.Format("150405"), i)
			sent = append(sent, views.Sent{Request: id, Session: session, Account: account, At: at})
		}
	}
	burst("personal-0", "personal", 4, tracedClock(14, 30, 1))
	burst("personal-0", "personal", 2, tracedClock(14, 41, 31))
	burst("side-0", "side", 16, tracedClock(14, 41, 31))
	for _, account := range []string{"personal", "side"} {
		burst("side-1", account, 12, tracedClock(14, 20, 1))
		burst("gone", account, 15, tracedClock(14, 15, 1))
	}
	return doc, listed, views.Traces{}.Told(sent, routedAt)
}

// tracedClock is the time given on routedAt's day.
func tracedClock(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, second, 0, time.UTC)
}

// tracedCells are the eighths of a trace 60 cells wide, as at 160 columns,
// filled in the cells given and no others: the cell of the 30 seconds from
// hh:mm:00 or hh:mm:30 is 59 − (14:42:00 − that) ÷ 30 s, routedAt's the last.
func tracedCells(filled map[int]int) []int {
	cells := make([]int, 60)
	for i, eighths := range filled {
		cells[i] = eighths
	}
	return cells
}

// tracedOf are the eighths each column of a trace drawn on c fills, width
// cells from column x, on the rows given from the bottom up, read back from
// its cells: one on a colour, or █, counts eight, and the one above it its
// eighth block in accent.key; the lane, ▁ in text.faint, none.
func tracedOf(c *canvas, x, width int, rows ...int) []int {
	filled := make([]int, width)
	for i := range filled {
		for _, y := range rows {
			here, eighths := c.at(x+i, y), 0
			switch {
			case here.ink.on != (hue{}):
				eighths = 8
			case here.ink.token == theme.AccentKey:
				eighths = max(slices.Index(levels[:], here.glyph), 0)
			}
			filled[i] += eighths
			if eighths < 8 {
				break
			}
		}
	}
	return filled
}

// routingOf is ROUTING of the sections as they're laid out: the zero planned
// where it's hidden.
func routingOf(laid []planned) planned {
	for _, p := range laid {
		if p.Section == SectionRouting {
			return p
		}
	}
	return planned{}
}

func TestAFramesTracesDrawItsRequestsAMinuteToItsScale(t *testing.T) {
	doc, listed, traces := tracedWorld()
	tests := []struct {
		name  string
		scale float64
		// traces are each account's eighths, two rows' 16 to a trace.
		traces map[string][]int
	}{
		{
			name: "live, scaled to side's busiest, 16 a minute: an eighth a request a minute",
			traces: map[string][]int{
				"work":     tracedCells(nil),
				"personal": tracedCells(map[int]int{5: 15, 6: 15, 15: 12, 16: 12, 35: 4, 36: 4, 58: 2, 59: 2}),
				"side":     tracedCells(map[int]int{5: 15, 6: 15, 15: 12, 16: 12, 58: 16, 59: 16}),
			},
		},
		{
			name: "pinned at 32: an eighth two requests a minute", scale: 32,
			traces: map[string][]int{
				"work":     tracedCells(nil),
				"personal": tracedCells(map[int]int{5: 8, 6: 8, 15: 6, 16: 6, 35: 2, 36: 2, 58: 1, 59: 1}),
				"side":     tracedCells(map[int]int{5: 8, 6: 8, 15: 6, 16: 6, 58: 8, 59: 8}),
			},
		},
	}
	rates := map[string]span{"work": {"0/min", dimInk}, "personal": {"2/min", strongInk}, "side": {"16/min", strongInk}}
	for _, tt := range tests {
		f := frameOf(160, 40)
		f.Look, f.Sections, f.Sessions, f.Traces, f.TraceScale = Screen(builtin(t, "nord")), SectionsNamed([]string{"routing"}), listed, traces, tt.scale
		c := newCanvas(f.Width, f.Height)
		f.view(c, doc, routedAt)
		body := routingOf(f.laidOut(doc, routedAt, f.top())).body
		for i, a := range doc.Accounts {
			rows := f.accountRows(i, len(doc.Accounts), body)
			if len(rows) != 2 {
				t.Fatalf("%s: %s's trace is %d rows, want 2", tt.name, a.ID, len(rows))
			}
			if got := tracedOf(c, traceFrom, 60, rows[1], rows[0]); !slices.Equal(got, tt.traces[a.ID]) {
				t.Errorf("%s: %s's trace fills\n%v\nwant\n%v", tt.name, a.ID, got, tt.traces[a.ID])
			}
			rate, end := rates[a.ID], c.at(f.Width-1, rows[1])
			if got := rowOf(c, rows[1]); !strings.HasSuffix(got, " "+rate.text) || end.ink != rate.ink {
				t.Errorf("%s: %s's last row ends %q, its last cell %+v; want %q in %+v", tt.name, a.ID, got[len(got)-8:], *end, rate.text, rate.ink)
			}
		}
	}
}

func TestEveryTraceOnThePageSharesTheBusiestColumnAsItsScale(t *testing.T) {
	doc, listed, traces := tracedWorld()
	routing, routingAndSessions := SectionsNamed([]string{"routing"}), SectionsNamed([]string{"routing", "sessions"})
	tests := []struct {
		name  string
		frame Frame
		quiet bool
		want  float64
	}{
		{name: "ROUTING's accounts', side's newest", frame: Frame{Width: 160, Sections: routing}, want: 16},
		{name: "and the running sessions', side-1's, on two accounts at once, never gone's", frame: Frame{Width: 160, Sections: routingAndSessions}, want: 24},
		{name: "the sessions' alone, ROUTING hidden", frame: Frame{Width: 160, Sections: SectionsNamed([]string{"sessions"})}, want: 24},
		{name: "beside USAGE's blocks, the pool's, its accounts' summed", frame: Frame{Width: 160, By: ByAccount}, want: 30},
		{name: "with no trace shown, the least", frame: Frame{Width: 160, Sections: SectionsNamed([]string{"usage"})}, want: leastScale},
		{name: "a quiet page, never less than 8", frame: Frame{Width: 160, Sections: routing}, quiet: true, want: leastScale},
	}
	quiet := views.Traces{}.Told([]views.Sent{{Request: "a", Session: "personal-0", Account: "personal", At: tracedClock(14, 30, 1)}}, routedAt)
	for _, tt := range tests {
		tt.frame.Sessions, tt.frame.Traces = listed, traces
		if tt.quiet {
			tt.frame.Traces = quiet
		}
		if got := tt.frame.scaled(doc, routedAt).traceScale(); got != tt.want {
			t.Errorf("%s: scaled to %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTheSessionsTracesScaleOnlyWhereSESSIONSDrawsThem(t *testing.T) {
	doc, listed, traces := tracedWorld()
	// Six rows, the sessions listed on personal and side; gone's never.
	tests := []struct{ width, want int }{{width: 110, want: 6}, {width: 109}}
	for _, tt := range tests {
		f := Frame{Width: tt.width, Sections: SectionsNamed([]string{"sessions"}), Sessions: listed, Traces: traces}
		if got := len(f.sessionsColumns(doc, routedAt)); got != tt.want {
			t.Errorf("at %d columns, %d of the sessions' traces scale the page, want %d", tt.width, got, tt.want)
		}
	}
}

func TestAPinnedScaleStandsAndALiveOneIsWorkedOutOnceAFrame(t *testing.T) {
	doc, listed, traces := tracedWorld()
	for _, pinned := range []float64{4, 32} {
		f := Frame{Width: 160, Sessions: listed, Traces: traces, TraceScale: pinned}
		if got := f.scaled(doc, routedAt).traceScale(); got != pinned {
			t.Errorf("pinned at %v, scaled to %v", pinned, got)
		}
	}
	if got := (Frame{Width: 160, Traces: traces}).traceScale(); got != leastScale {
		t.Errorf("before it's scaled, a live scale is %v, want %v", got, float64(leastScale))
	}
}

func TestTracesSumColumnByColumn(t *testing.T) {
	columns := map[int][]float64{1: {1, 2, 3}, 2: {4, 5}, 3: nil}
	columnsOf := func(t views.Trace) []float64 { return columns[len(t.Sent)] }
	traces := []views.Trace{{Sent: make([]time.Time, 1)}, {Sent: make([]time.Time, 2)}, {Sent: make([]time.Time, 3)}}
	if got, want := summed(traces, columnsOf), []float64{5, 7, 3}; !slices.Equal(got, want) {
		t.Errorf("summed %v, want %v", got, want)
	}
}
