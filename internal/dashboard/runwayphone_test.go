package dashboard

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// On a phone, the legend's keys take a line, wrapping whole keys where they
// don't fit one, and what a column is the line after them, though both would
// fit one line; and what a column is wraps too, at its " · ", a part never
// broken.
func TestOnAPhoneTheLegendsKeysThenWhatAColumnIsTakeALineEach(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name  string
		shown RunwayShown
		width int
		drawn views.Drawn
		want  []string
	}{
		{
			name: "the frame's, at 52", shown: phoneRunwayLive(views.RunwayFiveHour, 52), width: 52,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellCapped}, Resets: true},
			want:  []string{"█ room    ─ at its cap    │ its 5-hour resets", "a column ≈ 36 minutes · the past dimmed"},
		},
		{
			name: "one key, that would fit beside what a column is", shown: phoneRunwayLive(views.RunwayFiveHour, 52), width: 52,
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room", "a column ≈ 36 minutes · the past dimmed"},
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
			shown: RunwayShown{Back: civilDay(10, 1), Timeline: views.Stepped(views.RunwayFiveHour, civilDay(10, 1), routedAt, time.Monday, RunwayColumns(52))},
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room", "a column ≈ 38 minutes · the past dimmed", "under each name, the day's highest"},
		},
		{
			name: "a week gone, at 99", width: 99,
			shown: RunwayShown{Window: views.RunwayWeek, Back: civilDay(9, 21), Timeline: views.Stepped(views.RunwayWeek, civilDay(9, 21), routedAt, time.Monday, RunwayColumns(99))},
			drawn: views.Drawn{Cells: []views.Cell{views.CellRoom}},
			want:  []string{"█ room", "a column ≈ 119 minutes · under each name, the week's highest"},
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

// On a phone, Runway takes the first step of its compaction whose last row
// is two above the key line or higher, its room on a row of its own under
// the pool's lane and no blank row before its legend of two: three accounts
// with three lines of COMING UP, as the phone's frame, a row at a time from
// 37 to 15: three rows each to 32; (a) to 29; (b) at 28; (c) to 25; (d) at
// 24; then their lanes scrolling, two in view to 22 and one to 20, and none
// under it; and ten, seven in view at 33, its lanes scrolling an account at a
// time as the phone's form says.
func TestOnAPhoneRunwayTakesTheFirstOfItsStepsThatFits(t *testing.T) {
	type fitted struct {
		step  runwayStep
		shown int
		fits  bool
	}
	at := func(step runwayStep, shown int) fitted { return fitted{step, shown, true} }
	three := map[int]fitted{
		37: at(runwayRoomy, 3), 36: at(runwayRoomy, 3), 35: at(runwayRoomy, 3), 34: at(runwayRoomy, 3),
		33: at(runwayRoomy, 3), 32: at(runwayRoomy, 3),
		31: at(runwayTwoRows, 3), 30: at(runwayTwoRows, 3), 29: at(runwayTwoRows, 3),
		28: at(runwayNoGaps, 3),
		27: at(runwayFolded, 3), 26: at(runwayFolded, 3), 25: at(runwayFolded, 3),
		24: at(runwayAxisClose, 3),
		23: at(runwayScrolled, 2), 22: at(runwayScrolled, 2), 21: at(runwayScrolled, 1), 20: at(runwayScrolled, 1),
		19: {}, 18: {}, 17: {}, 16: {}, 15: {},
	}
	type fitting struct {
		accounts, rows int
		want           fitted
	}
	tests := []fitting{{accounts: 10, rows: 33, want: at(runwayScrolled, 7)}}
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
			fit := f.phoneRunwayFitted(f.top())
			if got := (fitted{fit.step, fit.shown, fit.fits}); got != tt.want {
				t.Errorf("at %d rows, Runway fits %+v, want %+v", tt.rows, got, tt.want)
			}
			if !fit.fits {
				return
			}
			if fit.end > tt.rows-underView {
				t.Errorf("its last row is %d, past row %d, two above the key line", fit.end-1, tt.rows-underView-1)
			}
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
	return RunwayShown{Window: w, Timeline: views.LiveTimeline(w, routedAt, RunwayColumns(width))}
}
