package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

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
		name: "WEEK", start: time.Date(2026, 9, 27, 21, 0, 0, 0, time.UTC), length: 7 * quota.Day, spanned: true,
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

// accountAt is doc's account with the given id as it stands at the moment
// given.
func accountAt(doc status.Document, id string, at time.Time) status.AccountNow {
	a, _ := doc.At(at, claudeLike).Account(id)
	return a
}

func TestAWindowResetSinceItWasReadIsChartedFromItsReset(t *testing.T) {
	doc := trio()
	work, _ := doc.Account("work")
	window, _ := work.Window("7d")
	before := Trail{Readings: []score.Reading{{At: window.ResetsAt.Add(-time.Hour), Utilization: 0.5}}}
	after := window.ResetsAt.Add(time.Hour)
	got := chartOf(accountAt(doc, "work", after), shownWindow{key: "7d", name: "Week"}, before, after, BurnUp)
	if len(got.readings) != 0 || !got.start.Equal(window.ResetsAt) {
		t.Errorf("charted from %v with %d readings, want from its reset, %v, with none of before it", got.start, len(got.readings), window.ResetsAt)
	}
}

// GRAPHS charts a window as the router's next document has it: on the tick
// after work's session resets, before it's read again, the session has
// lapsed, and its chart is its name alone, with no history, as once the
// router's next document has it empty, without a reset, rather than an axis
// and even pace over a span it hasn't started.
func TestOnTheTickASessionLapsesItsChartIsAsTheRoutersNextDocumentHasIt(t *testing.T) {
	doc := trio()
	work, _ := doc.Account("work")
	session, _ := work.Window("5h")
	after := session.ResetsAt.Add(time.Second)
	trail := Trail{Readings: []score.Reading{{At: session.ResetsAt.Add(-time.Hour), Utilization: 0.5}}}
	shown := shownWindow{key: "5h", name: "5-hour"}
	nord := Screen(builtin(t, "nord"))
	nextDoc := trio()
	nextDoc.Accounts[0] = work.AsOf(claudeLike, after)

	tick := chartOf(accountAt(doc, "work", after), shown, trail, after, BurnUp)
	next := chartOf(accountAt(nextDoc, "work", after), shown, trail, after, BurnUp)
	if tick.spanned {
		t.Errorf("the session is charted from %v over %v, want no span, as it has lapsed", tick.start, tick.length)
	}
	if got, want := chartIn(tick, nord, 30, 5).rows(nord), chartIn(next, nord, 30, 5).rows(nord); !slices.Equal(got, want) {
		t.Errorf("on the tick, the chart is\n%s\nwant it as the router's next document has it\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
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
			f := Frame{Width: tt.width, Height: 70, Focus: "work", Policy: claudeLike, Models: true}
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
		want := slices.Concat(line{{tt.want, titleInk}, spaces(5)}, burnUpKey)
		if got := g.description(); !slices.Equal(got, want) {
			t.Errorf("focused on %q, the description is %+v, want %+v", tt.focus, got, want)
		}
	}
	if s := (Frame{Width: 160, Height: 39}).graphsSection(status.Document{}, printNow); s.description() != nil || s.folded() != nil {
		t.Errorf("with no account, GRAPHS says %+v, folded %+v, want nothing", s.description(), s.folded())
	}
}

func TestGRAPHSNamesAFocusedAccountOutOfThePoolFaint(t *testing.T) {
	doc := trio(pinnedTo("work"))
	tests := []struct {
		focus string
		want  span
	}{
		{focus: "work", want: span{"work", titleInk}},
		{focus: "personal", want: span{"personal", ink{token: theme.TextFaint, bold: true}}},
	}
	for _, tt := range tests {
		g := Frame{Width: 160, Height: 39, Focus: tt.focus}.graphsSection(doc, printNow)
		if said, folded := g.description(), g.folded(); said[0] != tt.want || !slices.Equal(folded, line{tt.want}) {
			t.Errorf("with work alone pinned, focused on %s, GRAPHS says %+v, folded %+v, want each to name %+v", tt.focus, said, folded, tt.want)
		}
	}
}

func TestGRAPHSFoldedHoldsTheFocusedAccount(t *testing.T) {
	g := Frame{Width: 160, Height: 35, Focus: "work"}.graphsSection(trio(), printNow)
	want := line{{"work", titleInk}}
	if got := g.folded(); !slices.Equal(got, want) {
		t.Errorf("GRAPHS folded says %+v, want %+v", got, want)
	}
}

func TestGRAPHSByAccountUnder150ColumnsFallsBackToTheFocusedAccountsChartsAsByWindow(t *testing.T) {
	doc := trio()
	for _, burn := range []Burn{BurnUp, BurnDown} {
		for _, width := range []int{149, 120, 52} {
			drawn := func(by By) []string {
				f := Frame{Width: width, Height: 80, Look: Screen(builtin(t, "nord")), By: by, Focus: "personal", Burn: burn, Sections: Sections{}.Toggled(SectionGraphs)}
				return graphsDrawn(f, doc, 0, 23).rows(f.Look)
			}
			if byWindow, byAccount := drawn(ByWindow), drawn(ByAccount); !slices.Equal(byWindow, byAccount) {
				t.Errorf("by account at %d columns, the %s draws\n%s\nwant as by window\n%s", width, burn.Name(), strings.Join(byAccount, "\n"), strings.Join(byWindow, "\n"))
			}
		}
	}
}

func TestBurnIsKeptByItsNameAndVFlipsIt(t *testing.T) {
	tests := []struct {
		kept        string
		want        Burn
		name, keeps string
		flipped     Burn
	}{
		{kept: "", want: BurnUp, name: "burn-up", keeps: "", flipped: BurnDown},
		{kept: "burn-down", want: BurnDown, name: "burn-down", keeps: "burn-down", flipped: BurnUp},
		{kept: "hourglass", want: BurnUp, name: "burn-up", keeps: "", flipped: BurnDown},
	}
	for _, tt := range tests {
		got := BurnKept(tt.kept)
		if got != tt.want || got.Name() != tt.name || got.Kept() != tt.keeps || got.Flipped() != tt.flipped {
			t.Errorf("kept as %q, GRAPHS draws %v, named %q, kept as %q, flipping to %v; want %v, %q, %q, %v",
				tt.kept, got, got.Name(), got.Kept(), got.Flipped(), tt.want, tt.name, tt.keeps, tt.flipped)
		}
	}
}

// The keys GRAPHS' heading gives, each item three cells after the one
// before.
var (
	burnUpKey = line{
		{"·", dimInk}, {" even pace", faintInk}, {"   ", faintInk}, {"╌", reserveInk}, {" cap", faintInk}, {"   ", faintInk}, {"┄ limit", faintInk},
	}
	burnDownKey = line{
		{"⠢", dimInk}, {" heading", faintInk}, {"   ", faintInk}, {"✕", outInk}, {" runs out", faintInk}, {"   ", faintInk},
		{"⠤", capDotInk}, {" cap", faintInk}, {"   ", faintInk}, {"┄ full", faintInk},
	}
)

func TestGRAPHSHeadingKeysTheLinesOfTheChartItDraws(t *testing.T) {
	tests := []struct {
		burn Burn
		want line
	}{
		{burn: BurnUp, want: burnUpKey},
		{burn: BurnDown, want: burnDownKey},
	}
	for _, tt := range tests {
		g := Frame{Width: 160, Height: 39, Focus: "work", Burn: tt.burn}.graphsSection(trio(), printNow)
		if got, want := g.description(), slices.Concat(line{{"work", titleInk}, spaces(5)}, tt.want); !slices.Equal(got, want) {
			t.Errorf("drawing the %s, GRAPHS' heading says %+v, want %+v", tt.burn.Name(), got, want)
		}
	}
}

// GRAPHS' heading gives its key way by whole items, from its last, never
// cutting one short: at every width with room for its first, beside USAGE's
// blocks, or after the focused account's name, either chart's key reads as
// its first items, each whole.
func TestGRAPHSKeyGivesWayByWholeItems(t *testing.T) {
	for _, burn := range []Burn{BurnUp, BurnDown} {
		items := strings.Split(burnKey(burn).plain(), "   ")
		forms := []struct {
			name    string
			section section
			lead    string
		}{
			{name: "beside USAGE's blocks", section: graphsAside(burn)},
			{name: "after the focused account", section: Frame{Width: 160, Height: 39, Focus: "work", Burn: burn}.graphsSection(trio(), printNow), lead: "work"},
		}
		for _, form := range forms {
			for width := 24; width <= 70; width++ {
				c := newCanvas(width, 1)
				frameOf(width, 40).sectionHeading(c, planned{section: form.section, head: bounds{width: width, rows: 1}})
				said, _, _ := strings.Cut(strings.TrimPrefix(textOf(c.rows(Look{}))[0], "GRAPHS   "), " ─")
				if !wholeItems(said, form.lead, items) {
					t.Errorf("the %s's key %s, %d cells wide, reads %q, want its first items, each whole", burn.Name(), form.name, width, said)
				}
			}
		}
	}
}

// wholeItems reports whether said is lead, then five cells on, where there's
// a lead, the first of the items given, three cells apart, each whole.
func wholeItems(said, lead string, items []string) bool {
	for n := range len(items) + 1 {
		key := strings.Join(items[:n], "   ")
		switch {
		case lead == "" && n > 0 && said == key:
			return true
		case lead != "" && n == 0 && said == lead:
			return true
		case lead != "" && n > 0 && said == lead+"     "+key:
			return true
		}
	}
	return false
}
