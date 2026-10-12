package dashboard

import (
	"slices"
	"testing"
	"time"

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
		{name: "empty: its lane, ▁ on the bottom row", col: 0, top: blank, bot: cell{traceLane, lane}},
		{name: "half an eighth, rounding to the even none: empty", col: 1, top: blank, bot: cell{traceLane, lane}},
		{name: "two eighths", col: 2, top: blank, bot: cell{levels[2], fill}},
		{name: "an eighth and a half, rounding to the even two", col: 3, top: blank, bot: cell{levels[2], fill}},
		{name: "a row's eight: full, its colour behind it", col: 4, top: blank, bot: full},
		{name: "twelve: a full cell, four eighths on top", col: 5, top: cell{levels[4], fill}, bot: full},
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

func TestEveryTraceOnThePageSharesTheBusiestColumnAsItsScale(t *testing.T) {
	doc, _ := autoRouting()
	trace := func(n int) views.Trace { return views.Trace{Sent: make([]time.Time, n)} }
	// A trace's columns are its requests' count, in its first.
	columnsOf := func(t views.Trace) []float64 { return []float64{float64(len(t.Sent)), 0} }
	traces := views.Traces{
		Accounts: map[string]views.Trace{"work": trace(9), "personal": trace(6), "side": trace(5), "gone": trace(40)},
		Sessions: map[string]views.Trace{"c61b": trace(12)},
	}
	routingAndSessions := Sections{}.Toggled(SectionSessions)
	tests := []struct {
		name  string
		frame Frame
		want  float64
	}{
		{name: "the accounts' busiest, while ROUTING shows", frame: Frame{Width: 160}, want: 9},
		{name: "and the sessions', while SESSIONS shows", frame: Frame{Width: 160, Sections: routingAndSessions}, want: 12},
		{name: "the sessions' alone, ROUTING hidden", frame: Frame{Width: 160, Sections: routingAndSessions.Toggled(SectionRouting)}, want: 12},
		{name: "the pool's, its accounts' summed, beside USAGE's blocks", frame: Frame{Width: 160, By: ByAccount}, want: 20},
		{name: "never less than 8", frame: Frame{Width: 160, Sections: Sections{}.Toggled(SectionRouting)}, want: leastScale},
	}
	for _, tt := range tests {
		tt.frame.Traces = traces
		if got := tt.frame.busiestOf(doc, columnsOf); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestAPinnedScaleStandsAndALiveOneIsWorkedOutOnceAFrame(t *testing.T) {
	doc, _ := autoRouting()
	if got := (Frame{TraceScale: 16}).scaled(doc, routedAt).traceScale(); got != 16 {
		t.Errorf("pinned at 16, scaled to %v", got)
	}
	if got := (Frame{Width: 160}).scaled(doc, routedAt).traceScale(); got != leastScale {
		t.Errorf("a quiet page scaled to %v, want %v", got, float64(leastScale))
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
