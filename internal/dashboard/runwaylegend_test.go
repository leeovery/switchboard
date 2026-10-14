package dashboard

import (
	"slices"
	"testing"

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
			want:  []string{"▆ room    ─ at its cap    │ its 5-hour resets    a column 10 minutes · the past dimmed"},
		},
		{
			name: "the week, live", shown: runwayLive(views.RunwayWeek), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellRunsOut, views.CellCapped, views.CellLimited}, Resets: true},
			want: []string{
				"▆ room    ▆ room, but it runs out at its pace    ─ at its cap    ─ at its limit    │ its week resets    a column ≈ 74 minutes · the past dimmed",
			},
		},
		{
			name: "a day gone", shown: runwayBack(views.RunwayFiveHour, civilDay(9, 30)), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellLimited}, Resets: true},
			want:  []string{"▆ room    ─ at its limit    │ its 5-hour resets    a column ≈ 10.6 minutes · under each name, the day's highest"},
		},
		{
			name: "today, stepped back to", shown: runwayBack(views.RunwayFiveHour, civilDay(10, 1)), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"▆ room    a column ≈ 10.6 minutes · the past dimmed · under each name, the day's highest"},
		},
		{
			name: "a week gone", shown: runwayBack(views.RunwayWeek, civilDay(9, 21)), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"▆ room    a column ≈ 74 minutes · under each name, the week's highest"},
		},
		{
			name: "a refusal and no token, each keyed", shown: runwayLive(views.RunwayFiveHour), look: nord,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRefused, views.CellTokenless}},
			want:  []string{"─ token refused    ─ no token    a column 10 minutes · the past dimmed"},
		},
		{
			name: "a look that can't blend, nothing said dimmed", shown: runwayLive(views.RunwayFiveHour), look: Screen(builtin(t, "terminal")),
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"▆ room    a column 10 minutes"},
		},
		{
			name: "without colour, a run-out shaded", shown: runwayLive(views.RunwayFiveHour), look: NoColour(),
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellRunsOut}},
			want:  []string{"▆ room    ▒ room, but it runs out at its pace    a column 10 minutes"},
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
	keys := "▆ room    ▆ room, but it runs out at its pace    ─ at its cap    ─ at its limit    │ its 5-hour resets"
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
			want: []string{"▆ room    ▆ room, but it runs out at its pace    ─ at its cap    ─ at its limit", "│ its 5-hour resets", column},
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
	for y, want := range map[int]string{22: "", 23: "▆ room    ▆ room, but it runs out at its pace    ─ at its cap    ─ at its limit    ─ token refused", 24: "─ no token    │ its 5-hour resets", 25: "a column 10 minutes · the past dimmed", 26: ""} {
		if got := textAt(c, 0, y, f.Width); got != want {
			t.Errorf("row %d reads %q, want %q", y, got, want)
		}
	}
}
