package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// legendOf is the legend of Runway shown as r on a frame width columns wide
// in the look given, its lanes drawing what's given.
func legendOf(r RunwayShown, width int, look Look, drawn views.Drawn) []string {
	r.Lanes = &views.Lanes{Drawn: drawn}
	f := runwayShowing(r)
	f.Width, f.Look = width, look
	var lines []string
	for _, l := range f.runwayLegendLines() {
		lines = append(lines, l.plain())
	}
	return lines
}

// Every cell a lane draws, and a reset.
var everyCell = views.Drawn{
	Cells:  []views.Cell{views.CellRoom, views.CellRunsOut, views.CellCapped, views.CellLimited, views.CellRefused, views.CellTokenless},
	Resets: true,
}

func TestTheLegendKeysOnlyWhatTheLanesDrawThenWhatAColumnIs(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name  string
		shown RunwayShown
		look  Look
		drawn views.Drawn
		want  []string
	}{
		{
			name: "the 5-hour, live", shown: runwayLive(views.RunwayFiveHour), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellCapped}, Resets: true},
			want:  []string{"█ room    ─ at its cap    │ its 5-hour resets    a column 10 minutes · the past dimmed"},
		},
		{
			name: "the week, live", shown: runwayLive(views.RunwayWeek), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellRunsOut, views.CellCapped, views.CellLimited}, Resets: true},
			want: []string{
				"█ room    █ room, but it runs out at its pace    ─ at its cap    ─ at its limit    │ its week resets    a column ≈ 74 minutes · the past dimmed",
			},
		},
		{
			name: "a day gone", shown: runwayBack(views.RunwayFiveHour, civilDay(9, 30)), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellLimited}, Resets: true},
			want:  []string{"█ room    ─ at its limit    │ its 5-hour resets    a column ≈ 10.6 minutes · under each name, the day's highest"},
		},
		{
			name: "today, stepped back to", shown: runwayBack(views.RunwayFiveHour, civilDay(10, 1)), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room    a column ≈ 10.6 minutes · the past dimmed · under each name, the day's highest"},
		},
		{
			name: "a week gone", shown: runwayBack(views.RunwayWeek, civilDay(9, 21)), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room    a column ≈ 74 minutes · under each name, the week's highest"},
		},
		{
			name: "a refusal and no token, each keyed", shown: runwayLive(views.RunwayFiveHour), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRefused, views.CellTokenless}},
			want:  []string{"─ token refused    ─ no token    a column 10 minutes · the past dimmed"},
		},
		{
			name: "a look that can't blend, nothing said dimmed", shown: runwayLive(views.RunwayFiveHour), look: Screen(builtin(t, "terminal")),
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room    a column 10 minutes"},
		},
		{
			name: "without colour, a run-out shaded", shown: runwayLive(views.RunwayFiveHour), look: NoColour(),
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellRunsOut}},
			want:  []string{"█ room    ▒ room, but it runs out at its pace    a column 10 minutes"},
		},
		{name: "nothing drawn", shown: runwayLive(views.RunwayFiveHour), look: nord, want: []string{"a column 10 minutes · the past dimmed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := legendOf(tt.shown, 160, tt.look, tt.drawn); !slices.Equal(got, tt.want) {
				t.Errorf("the legend reads\n%q, want\n%q", got, tt.want)
			}
		})
	}
}

// The legend is one line where it fits across the frame; else its keys on a
// line, and what a column is on the next; and where its keys don't fit one
// line, they wrap, a key never broken.
func TestTheLegendTakesOneLineWhereItFitsElseWrapsWholeKeys(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	four := views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellRunsOut, views.CellCapped, views.CellLimited}, Resets: true}
	keys := "█ room    █ room, but it runs out at its pace    ─ at its cap    ─ at its limit    │ its 5-hour resets"
	column := "a column 10 minutes · the past dimmed"
	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{name: "143 columns, one line", width: 143, want: []string{keys + "    " + column}},
		{name: "142 columns, two lines", width: 142, want: []string{keys, column}},
		{name: "102 columns, the keys whole on a line", width: 102, want: []string{keys, column}},
		{
			name: "101 columns, the keys wrapped whole", width: 101,
			want: []string{"█ room    █ room, but it runs out at its pace    ─ at its cap    ─ at its limit", "│ its 5-hour resets", column},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := runwayLive(views.RunwayFiveHour)
			shown.Timeline = views.LiveTimeline(views.RunwayFiveHour, routedAt, 136)
			if got := legendOf(shown, tt.width, nord, four); !slices.Equal(got, tt.want) {
				t.Errorf("at %d columns the legend reads\n%q, want\n%q", tt.width, got, tt.want)
			}
			shown.Lanes = &views.Lanes{Drawn: four}
			f := runwayShowing(shown)
			f.Width, f.Look = tt.width, nord
			if rows := f.runwayLegendRows(); rows != len(tt.want) {
				t.Errorf("at %d columns the fit counts the legend %d rows, want %d", tt.width, rows, len(tt.want))
			}
		})
	}
}

func TestTheLegendsGlyphsAreTheLanes(t *testing.T) {
	f := runwayShowing(runwayLive(views.RunwayFiveHour))
	f.Width, f.Look = 250, Screen(builtin(t, "nord"))
	f.Runway.Lanes = &views.Lanes{Drawn: everyCell}
	lines := f.runwayLegendLines()
	if len(lines) != 1 {
		t.Fatalf("the legend takes %d lines, want 1", len(lines))
	}
	var glyphs []span
	for _, s := range lines[0] {
		if slices.Contains([]string{roomCell, runsOutCell, heldCell, resetCell}, s.text) {
			glyphs = append(glyphs, s)
		}
	}
	want := []span{
		{roomCell, ink{token: theme.VizRamp1}}, {roomCell, warningInk}, {heldCell, warningInk},
		{heldCell, errorInk}, {heldCell, errorInk}, {heldCell, errorInk},
		{resetCell, dimInk},
	}
	if !slices.Equal(glyphs, want) {
		t.Errorf("the legend's glyphs are %v, want %v", glyphs, want)
	}
}

func TestTheLegendIsDrawnFromItsRowALineARow(t *testing.T) {
	f := runwayShowing(runwayLive(views.RunwayFiveHour))
	f.Width, f.Look = 101, Screen(builtin(t, "nord"))
	f.Runway.Lanes = &views.Lanes{Drawn: everyCell}
	c := newCanvas(f.Width, 40)
	f.runwayLegend(c, runwayLaid{fit: runwayFit{fits: true, legend: 23}})
	for y, want := range map[int]string{22: "", 23: "█ room    █ room, but it runs out at its pace    ─ at its cap    ─ at its limit    ─ token refused", 24: "─ no token    │ its 5-hour resets", 25: "a column 10 minutes · the past dimmed", 26: ""} {
		if got := textAt(c, 0, y, f.Width); got != want {
			t.Errorf("row %d reads %q, want %q", y, got, want)
		}
	}
}

// On a phone, as at every width, the legend is one line where it fits;
// else its keys take a line, wrapping whole keys where they don't fit one,
// and what a column is the line after them, wrapping too, at its " · ", a
// part never broken.
func TestOnAPhoneTheLegendIsOneLineWhereItFitsElseWrapsItsParts(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name  string
		shown RunwayShown
		width int
		drawn views.Drawn
		want  []string
	}{
		{
			name: "its keys, then what a column is, at 52", shown: phoneRunwayLive(views.RunwayFiveHour, 52), width: 52,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellCapped}, Resets: true},
			want:  []string{"█ room    ─ at its cap    │ its 5-hour resets", "a column ≈ 36 minutes · the past dimmed"},
		},
		{
			name: "one key, beside what a column is", shown: phoneRunwayLive(views.RunwayFiveHour, 52), width: 52,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room    a column ≈ 36 minutes · the past dimmed"},
		},
		{
			name: "every key, wrapped whole", shown: phoneRunwayLive(views.RunwayFiveHour, 52), width: 52, drawn: everyCell,
			want: []string{
				"█ room    █ room, but it runs out at its pace", "─ at its cap    ─ at its limit    ─ token refused",
				"─ no token    │ its 5-hour resets", "a column ≈ 36 minutes · the past dimmed",
			},
		},
		{
			name: "today stepped back to, what a column is wrapped at its parts", width: 52,
			shown: RunwayShown{Window: views.RunwayFiveHour, Back: civilDay(10, 1), Timeline: views.Stepped(views.RunwayFiveHour, civilDay(10, 1), routedAt, time.Monday, RunwayShown{}.Columns(52))},
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room", "a column ≈ 38 minutes · the past dimmed", "under each name, the day's highest"},
		},
		{
			name: "a week gone, at 99", width: 99,
			shown: RunwayShown{Window: views.RunwayWeek, Back: civilDay(9, 21), Timeline: views.Stepped(views.RunwayWeek, civilDay(9, 21), routedAt, time.Monday, RunwayShown{}.Columns(99))},
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room    a column ≈ 119 minutes · under each name, the week's highest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := legendOf(tt.shown, tt.width, nord, tt.drawn); !slices.Equal(got, tt.want) {
				t.Errorf("the legend reads\n%q, want\n%q", got, tt.want)
			}
			f := runwayShowing(tt.shown)
			f.Width, f.Look, f.Runway.Lanes = tt.width, nord, &views.Lanes{Drawn: tt.drawn}
			if rows := f.runwayLegendRows(); rows != len(tt.want) {
				t.Errorf("the fit counts the legend %d rows, want %d", rows, len(tt.want))
			}
		})
	}
}

// Where none of the pool has room in a column (B3), the pool's lane draws ▂
// along its floor, in a limit's red, where an account's lane at its limit
// keeps its ─; and the legend keys it, " the pool at its limit", after an
// account's limit's key, or where that would stand where no lane draws one.
// Where the pool's lane draws none, the legend doesn't key it.
func TestThePoolAtItsLimitIsAQuarterCellTheLegendKeys(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	doc := runwayDoc(3)
	columns := RunwayShown{}.Columns(160)
	limitedFrom := columns / 2
	pool := views.Pool{Of: 3, Key: "5h", Columns: make([]views.PoolColumn, columns)}
	for i := range pool.Columns {
		pool.Columns[i] = views.PoolColumn{With: 1, Known: true}
		if i >= limitedFrom {
			pool.Columns[i].With = 0
		}
	}
	shown := poolShown(doc, pool)
	shown.Lanes.Accounts[0].Cells = slices.Repeat([]views.Cell{views.CellLimited}, columns)
	shown.Lanes.Drawn = views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellCapped, views.CellLimited, views.CellTokenless}}
	f := Frame{Width: 160, Height: 37, Look: nord, View: Runway, Policy: claudeLike, Runway: shown}
	fit := f.runwayFitted(f.top())
	c := runwayDrawn(f, doc)
	x := f.Runway.lanesFrom(160) + limitedFrom
	if got, want := cellAt(c, x, fit.pool+1), (seenCell{"▂", errorInk}); got != want {
		t.Errorf("the pool's lane at its limit is %+v, want %+v", got, want)
	}
	if lane, _ := fit.laneRow(0); cellAt(c, x, lane).glyph != heldCell {
		t.Errorf("an account's lane at its limit is %q, want %q", cellAt(c, x, lane).glyph, heldCell)
	}
	if got, want := rowOf(c, fit.legend), "█ room    ─ at its cap    ─ at its limit    ▂ the pool at its limit    ─ no token    a column ≈ 9.3 minutes · the past dimmed"; got != want {
		t.Errorf("the legend reads %q, want %q", got, want)
	}
	tests := []struct {
		name  string
		pool  views.Pool
		drawn []views.Cell
		want  []string
	}{
		{name: "no lane at its limit", pool: pool, drawn: []views.Cell{views.CellRoom, views.CellRefused}, want: []string{"█ room", "▂ the pool at its limit", "─ token refused"}},
		{name: "after every lane's key", pool: pool, drawn: []views.Cell{views.CellRoom}, want: []string{"█ room", "▂ the pool at its limit"}},
		{name: "the pool's lane not at its limit", pool: views.Pool{Of: 3, Key: "5h", Columns: []views.PoolColumn{{With: 1, Known: true}}}, drawn: []views.Cell{views.CellRoom, views.CellLimited}, want: []string{"█ room", "─ at its limit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(runwayLive(views.RunwayFiveHour))
			f.Look, f.Runway.Lanes = nord, &views.Lanes{Pool: tt.pool, Drawn: views.Drawn{Cells: tt.drawn}}
			var got []string
			for _, key := range f.legendKeys() {
				got = append(got, key.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the legend's keys are %q, want %q", got, tt.want)
			}
		})
	}
}
