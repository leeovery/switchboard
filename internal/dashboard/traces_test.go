package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

func TestATracesCellsFillEighthsOfItsRowsToItsScale(t *testing.T) {
	// Two rows are 16 eighths: a request a minute fills two, at a scale of 8.
	cells := []float64{0, 0.25, 1, 0.75, 4, 6, 8, 20}
	c := newCanvas(len(cells), 2)
	Frame{Look: Screen(builtin(t, "nord")), TraceScale: 8}.drawCells(c, cells, bounds{width: len(cells), rows: 2}, false)
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
		Frame{Look: nord, TraceScale: 8}.drawCells(c, []float64{8, 4, 0}, bounds{width: 3, rows: 1}, tt.faded)
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
	Frame{Look: NoColour(), TraceScale: 8}.drawCells(c, []float64{8}, bounds{width: 1, rows: 1}, false)
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
	var sent []views.Told
	burst := func(session, account string, n int, at time.Time) {
		for i := range n {
			id := fmt.Sprintf("%s-%s-%s-%d", session, account, at.Format("150405"), i)
			sent = append(sent, views.Told{Request: id, Session: session, Account: account, Sent: at})
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

// Under a slide-over, as the theme picker covers the page's right, a trace
// ends as far short of it as it would of the edge, its oldest columns
// dropped, as many as it covers, its newest kept, and its rate right-aligned
// at the slide-over's edge: in ROUTING, wide and narrow, beside USAGE's
// blocks, the pool's among them, and in SESSIONS. The scale is the busiest
// of the columns that show: side's newest, 16, and the pool's, 18, the
// older bursts on two accounts at once, 24 a session and 30 the pool,
// dropped under it.
func TestATraceUnderASlideOverKeepsNowsEndAndItsRate(t *testing.T) {
	doc, listed, traces := tracedWorld()
	tests := []struct {
		name  string
		width int
		shown Sections
		by    By
		scale float64
	}{
		{name: "ROUTING, two rows an account", width: 160, shown: SectionsNamed([]string{"routing"}), scale: 16},
		{name: "ROUTING under 110 columns", width: 109, shown: SectionsNamed([]string{"routing"}), scale: 16},
		{name: "beside USAGE's blocks", width: 160, shown: SectionsNamed([]string{"usage", "routing"}), by: ByAccount, scale: 18},
		{name: "SESSIONS", width: 160, shown: SectionsNamed([]string{"sessions"}), scale: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 60)
			f.Sections, f.By, f.Sessions, f.Traces = tt.shown, tt.by, listed, traces
			covered := f
			covered.Covered = PickerWidth(tt.width)
			whole, under := f.overviewDrawn(doc, routedAt), covered.overviewDrawn(doc, routedAt)
			if len(*under.traces) == 0 || len(*under.traces) != len(*whole.traces) {
				t.Fatalf("it draws %d traces under the slide-over, want as many as without it, %d", len(*under.traces), len(*whole.traces))
			}
			edge := tt.width - covered.Covered
			for i, w := range *whole.traces {
				u := (*under.traces)[i]
				if u.at.x != w.at.x || u.at.width != w.at.width-covered.Covered || !slices.Equal(u.cells, w.cells[covered.Covered:]) {
					t.Errorf("trace %d is %d cells from column %d, %v; want its newest %d of %v, from column %d", i, u.at.width, u.at.x, u.cells, w.at.width-covered.Covered, w.cells, w.at.x)
				}
				rate := rowOf(under, u.at.y+u.at.rows-1)
				if !strings.HasSuffix(rate, "/min") || ansi.StringWidth(rate) != edge {
					t.Errorf("trace %d's last row reads %q, want its rate ending at the slide-over's edge, column %d", i, rate, edge)
				}
			}
			if got := covered.traceScale(*under.traces); got != tt.scale {
				t.Errorf("scaled to %v, want %v, the busiest of the columns that show", got, tt.scale)
			}
		})
	}
}

// A trace left no cell beside the slide-over keeps its place under it, and
// so does its rate, which never moves onto the words before the trace: on
// every row, what's drawn left of a trace's start, or the whole row where
// none starts, is as it's drawn without the slide-over, from 100 to 130
// columns, ROUTING a row an account, and with SESSIONS, at an ordinary
// height, and ROUTING alone at a tall one.
func TestARateNeverMovesOntoTheWordsBeforeItsTrace(t *testing.T) {
	doc, listed, traces := tracedWorld()
	for _, tc := range []struct {
		name   string
		height int
		shown  Sections
	}{
		{name: "ROUTING and SESSIONS", height: 37, shown: SectionsNamed([]string{"usage", "routing", "sessions"})},
		{name: "ROUTING alone", height: 60, shown: SectionsNamed([]string{"routing"})},
	} {
		for width := 100; width <= 130; width++ {
			t.Run(fmt.Sprintf("%s at %d columns", tc.name, width), func(t *testing.T) {
				f := frameOf(width, tc.height)
				f.Sections, f.Sessions, f.Traces = tc.shown, listed, traces
				covered := f
				covered.Covered = PickerWidth(width)
				whole, under := f.overviewDrawn(doc, routedAt), covered.overviewDrawn(doc, routedAt)
				starts := map[int]int{}
				for _, tr := range *under.traces {
					for y := tr.at.y; y < tr.at.y+tr.at.rows; y++ {
						starts[y] = tr.at.x
					}
				}
				for y := range whole.height() {
					before := width
					if x, ok := starts[y]; ok {
						before = x
					}
					if got, want := ansi.Truncate(rowOf(under, y), before, ""), ansi.Truncate(rowOf(whole, y), before, ""); got != want {
						t.Errorf("row %d reads %q before column %d under the slide-over, want %q", y, got, before, want)
					}
				}
			})
		}
	}
}

// pageScale is the scale the traces share on the Overview of doc at
// routedAt, as f draws it.
func pageScale(f Frame, doc status.Document) float64 {
	return f.traceScale(*f.overviewDrawn(doc, routedAt).traces)
}

// burst are n requests of the session given on the account given at 14:32:15,
// 15 seconds into a 40-second slice of the clock's: so a trace 45 cells wide,
// as beside USAGE's blocks at 160 columns, has no column whose four samples
// all fall in the minute after it, its busiest ¾ of n, where 60 cells wide,
// as ROUTING's own at 160 columns, its busiest is n.
func burst(session, account string, n int) views.Traces {
	sent := make([]views.Told, n)
	for i := range sent {
		sent[i] = views.Told{Request: fmt.Sprintf("%s-%d", session, i), Session: session, Account: account, Sent: tracedClock(14, 32, 15)}
	}
	return views.Traces{}.Told(sent, routedAt)
}

func TestEveryTraceOnScreenSharesTheBusiestColumnAsItsScale(t *testing.T) {
	doc, listed, traces := tracedWorld()
	quiet := views.Traces{}.Told([]views.Told{{Request: "a", Session: "personal-0", Account: "personal", Sent: tracedClock(14, 30, 1)}}, routedAt)
	routingAndSessions := SectionsNamed([]string{"routing", "sessions"})
	tests := []struct {
		name          string
		width, height int
		shown         Sections
		by            By
		quiet         bool
		want          float64
	}{
		{name: "ROUTING's accounts', side's newest", width: 160, height: 60, shown: SectionsNamed([]string{"routing"}), want: 16},
		{name: "and the running sessions', side-1's, on two accounts at once, never gone's", width: 160, height: 60, shown: routingAndSessions, want: 24},
		{name: "the sessions' alone, ROUTING hidden", width: 160, height: 60, shown: SectionsNamed([]string{"sessions"}), want: 24},
		{name: "under 110 columns, where SESSIONS draws no trace: ROUTING's alone", width: 109, height: 60, shown: routingAndSessions, want: 16},
		{name: "beside USAGE's blocks, the pool's, its accounts' summed", width: 160, height: 60, by: ByAccount, want: 30},
		{name: "SESSIONS folded, its rows run short: ROUTING's alone", width: 160, height: 20, shown: routingAndSessions, want: 16},
		{name: "ROUTING folded too: none, the least", width: 160, height: 12, shown: routingAndSessions, want: leastScale},
		{name: "with no trace shown, the least", width: 160, height: 60, shown: SectionsNamed([]string{"usage"}), want: leastScale},
		{name: "a quiet page, never less than 8", width: 160, height: 60, shown: SectionsNamed([]string{"routing"}), quiet: true, want: leastScale},
	}
	for _, tt := range tests {
		f := frameOf(tt.width, tt.height)
		f.Sections, f.By, f.Sessions, f.Traces = tt.shown, tt.by, listed, traces
		if tt.quiet {
			f.Traces = quiet
		}
		if got := pageScale(f, doc); got != tt.want {
			t.Errorf("%s: scaled to %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBesideUSAGEsBlocksTheScaleIsTheBusiestColumnAtTheWidthTheTracesAreDrawn(t *testing.T) {
	doc, listed := autoRouting()
	f := frameOf(160, 60)
	f.By, f.Sessions, f.Traces = ByAccount, listed, burst("side-0", "side", 16)
	if x := f.overviewBlock(doc, routedAt).besideAt(); f.Width-traceTail-x != 45 {
		t.Fatalf("the traces beside USAGE's blocks are %d cells, want 45", f.Width-traceTail-x)
	}
	view := f.overviewDrawn(doc, routedAt)
	if got := f.traceScale(*view.traces); got != 12 {
		t.Errorf("scaled to %v, want 12, side's busiest column 45 cells wide, not 16, as 60 cells wide", got)
	}
	fullest := 0
	for _, tr := range *view.traces {
		fullest = max(fullest, slices.Max(tracedOf(view, tr.at.x, len(tr.cells), tr.at.y)))
	}
	if fullest != 8 {
		t.Errorf("the fullest column beside the blocks fills %d eighths of its row, want 8", fullest)
	}
}

func TestABusyAccountScrolledOutOfUSAGEDoesNotSetTheScale(t *testing.T) {
	doc, listed := autoRouting()
	doc.Pin = status.Pin{Accounts: []string{"personal", "side"}}
	doc = doc.WorkedOut(claudeLike)
	// Work, out of the pool, sends 40 at once, its busiest column 30, 45
	// cells wide; the pool's trace sums personal's and side's alone.
	f := frameOf(160, 28)
	f.By, f.Sessions, f.Traces = ByAccount, listed, burst("work-0", "work", 40)
	if got := f.Scrolling(doc, routedAt); got.Most != 1 {
		t.Fatalf("USAGE scrolls %+v, want an account", got)
	}
	tests := []struct {
		name   string
		scroll int
		want   float64
	}{
		{name: "work in view", want: 30},
		{name: "work scrolled out of view", scroll: 1, want: leastScale},
	}
	for _, tt := range tests {
		f.Scroll = tt.scroll
		if got := pageScale(f, doc); got != tt.want {
			t.Errorf("%s: scaled to %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestASessionPastSESSIONSMoreRowDoesNotSetTheScale(t *testing.T) {
	doc, listed, traces := tracedWorld()
	f := Frame{Width: 160, Height: 40, View: Overview, Sessions: listed, Traces: traces}
	// Three rows show personal's two sessions, 4 their busiest, and say four
	// more run, side-1, 24, among them.
	c := holdingTraces(newCanvas(f.Width, 3))
	f.sessionsSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: 3})
	if got := f.traceScale(*c.traces); got != leastScale {
		t.Errorf("scaled to %v, want %v: the sessions shown", got, float64(leastScale))
	}
}

func TestAPinnedScaleStandsAndADirectDrawIsPaintedAtTheLeast(t *testing.T) {
	doc, listed, traces := tracedWorld()
	for _, pinned := range []float64{4, 32} {
		f := frameOf(160, 60)
		f.Sections, f.Sessions, f.Traces, f.TraceScale = SectionsNamed([]string{"routing", "sessions"}), listed, traces, pinned
		if got := pageScale(f, doc); got != pinned {
			t.Errorf("pinned at %v, scaled to %v", pinned, got)
		}
	}
	// A canvas that doesn't hold its traces paints each as it's drawn, at
	// the least: 16 a minute fills two rows of eight.
	c := newCanvas(2, 2)
	Frame{}.drawCells(c, []float64{16, 4}, bounds{width: 2, rows: 2}, false)
	if got := tracedOf(c, 0, 2, 1, 0); !slices.Equal(got, []int{16, 8}) {
		t.Errorf("drawn straight onto a canvas, a trace fills %v eighths, want [16 8]", got)
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
