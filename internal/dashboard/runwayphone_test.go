package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// Runway on a phone draws its frame cell for cell, in nord, from the heading
// row to the row over the key line: the labels in 14 cells, the lanes after
// them, the pool's room on a row of its own under its words, the legend on
// two lines and COMING UP the frame's width after a blank row; now's column
// picked out down to the pool's words. The part-filled cell of now's column
// in the pool's lane is passed over, on the canvas in the frame where the
// design has bg.subtle (D4).
func TestRunwayOnAPhoneIsDrawnAsItsFrame(t *testing.T) {
	f, doc, now := runwayFramedAt(t, "the auto world's 5-hour view", 52, 33)
	drawn, frame := screenOf(t, f.Draw(doc, now)), redrawn(reference(t, "runway-phone-52"), '▆', '█', roomRowsPhone...)
	heldTo(t, "runway-phone-52", frame, drawn, region{top: 4, bottom: 31, right: 51}, shadeOf(f.Look.Canvas()), []spot{{20, 15, 15}}, nil)
}

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
// is two above the key line or higher, its room on a row of its own and no
// blank row before its legend of two: three accounts with three lines of
// COMING UP, as the phone's frame, three rows each at 33, the rows each step
// gives them as rows run short, then their lanes scrolling, down to one in
// view at 22 and none under it; and ten, six in view at 33, its lanes
// scrolling an account at a time as the phone's form says.
func TestOnAPhoneRunwayTakesTheFirstOfItsStepsThatFits(t *testing.T) {
	tests := []struct {
		name           string
		accounts, rows int
		want           runwayStep
		shown          int
		fitsNotOneLane bool
	}{
		{name: "the frame's three at 33", accounts: 3, rows: 33, want: runwayRoomy, shown: 3},
		{name: "three at 32, at (a)", accounts: 3, rows: 32, want: runwayTwoRows, shown: 3},
		{name: "three at 31, at (a)", accounts: 3, rows: 31, want: runwayTwoRows, shown: 3},
		{name: "three at 30, at (b)", accounts: 3, rows: 30, want: runwayNoGaps, shown: 3},
		{name: "three at 29, COMING UP folded at (c)", accounts: 3, rows: 29, want: runwayFolded, shown: 3},
		{name: "three at 27, at (c)", accounts: 3, rows: 27, want: runwayFolded, shown: 3},
		{name: "three at 26, at (d)", accounts: 3, rows: 26, want: runwayAxisClose, shown: 3},
		{name: "three at 25, scrolling at (e)", accounts: 3, rows: 25, want: runwayScrolled, shown: 2},
		{name: "three at 24, two in view", accounts: 3, rows: 24, want: runwayScrolled, shown: 2},
		{name: "three at 22, one in view", accounts: 3, rows: 22, want: runwayScrolled, shown: 1},
		{name: "ten at 33, six in view", accounts: 10, rows: 33, want: runwayScrolled, shown: 6},
		{name: "three at 21, none", accounts: 3, rows: 21, fitsNotOneLane: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lanes := lanesOf(tt.accounts)
			lanes.Drawn = views.Drawn{Cells: []views.Cell{views.CellRoom, views.CellCapped}, Resets: true}
			shown := phoneRunwayLive(views.RunwayFiveHour, 52)
			shown.Lanes, shown.Coming = lanes, thingsComing(3)
			f := Frame{Width: 52, Height: tt.rows, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: shown}
			fit := f.phoneRunwayFitted(f.top())
			if tt.fitsNotOneLane {
				if fit.fits {
					t.Errorf("at %d rows, Runway fits at step %d, %d lanes shown; want nothing but the page's chrome", tt.rows, fit.step, fit.shown)
				}
				return
			}
			if !fit.fits || fit.step != tt.want || fit.shown != tt.shown {
				t.Errorf("at %d rows, Runway fits at step %d, %d of %d lanes shown, fitting %t; want step %d, %d shown", tt.rows, fit.step, fit.shown, tt.accounts, fit.fits, tt.want, tt.shown)
			}
			if last := fit.end - 1; last > tt.rows-3 {
				t.Errorf("its last row is %d, past row %d, two above the key line", last, tt.rows-3)
			}
			if fit.room != fit.words+1 {
				t.Errorf("the pool's room is on row %d, want the row under its words, %d", fit.room, fit.words+1)
			}
			if got, want := f.Scrolling(runwayDoc(tt.accounts), routedAt), (Scrolling{Most: tt.accounts - tt.shown, Page: tt.shown}); got != want {
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
