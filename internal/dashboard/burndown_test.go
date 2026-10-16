package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// downAt is the burn-down of a window ten hours long, from 08:00, of an
// account capped at capAt, where it's under 1, at now, heading as given, its
// readings as given.
func downAt(now time.Time, capAt float64, heading course, readings ...score.Reading) chart {
	ch := chartAt(now, capAt, readings...)
	ch.burn, ch.course = BurnDown, heading
	return ch
}

// chartRows are the chart's h rows on c, as text, top first.
func chartRows(c *canvas, h int) []string {
	var rows []string
	for y := 1; y <= h; y++ {
		rows = append(rows, rowOf(c, y))
	}
	return rows
}

// clockOn is the time given on 1 October 2026, in UTC.
func clockOn(hour, minute int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, 0, 0, time.UTC)
}

func TestABurnDownFillsEachColumnToTheRoomLeftByItsMiddleAndNowsByNow(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name     string
		readings []score.Reading
		want     []int
	}{
		{
			// Ten columns of an hour each, from 08:00, two rows, 16 eighths;
			// now, 12:40, falls in column round(4.67) = 5.
			name:     "a window just reset: full to now",
			readings: []score.Reading{readOn(8, 0, 0)},
			want:     []int{16, 16, 16, 16, 16, 16, 0, 0, 0, 0},
		},
		{
			name: "falling as it's used, a little room left its lowest eighth",
			readings: []score.Reading{
				readOn(8, 40, 0.25),     // after column 0's middle: column 0 has nothing read
				readOn(9, 30, 0.5),      // at column 1's middle: 8 eighths left
				readOn(10, 45, 0.53125), // column 3's: 7.5 eighths, a half to even, 8
				readOn(12, 35, 0.97),    // now's: 0.48 eighths, rounding to none, but some left
				readOn(12, 50, 0.1),     // after now: none of it shows
			},
			want: []int{0, 8, 8, 8, 8, 1, 0, 0, 0, 0},
		},
		{
			name:     "a window with one reading: nothing before it, the level from it on",
			readings: []score.Reading{readOn(10, 0, 0.5)},
			want:     []int{0, 0, 8, 8, 8, 8, 0, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filledOf(chartIn(downAt(clockOn(12, 40), 1, course{}, tt.readings...), nord, 10, 2), 2)
			if !slices.Equal(got, tt.want) {
				t.Errorf("the columns are filled %v eighths, want %v: each to the room the last use read by its middle left, now's by now", got, tt.want)
			}
		})
	}
}

func TestABurnDownsLevelRunsFromTheRampsFirstStopToItsLastAsItFalls(t *testing.T) {
	ch := downAt(clockOn(8, 59), 1, course{}, readOn(8, 0, 0.4))
	c := chartIn(ch, Screen(builtin(t, "nord")), 10, 4)
	// 0.6 of 4 rows left is 19.2 eighths, 19: two full rows, and three eighths,
	// each row the ramp at its height read from the top.
	for r, want := range []cell{
		{glyph: " ", ink: ink{on: hue{ramp: true, at: 0.875, fade: 0.3}}},
		{glyph: " ", ink: ink{on: hue{ramp: true, at: 0.625, fade: 0.3}}},
		{glyph: levels[3], ink: ink{ramp: true, at: 0.375, fade: 0.3}},
	} {
		if got := *c.at(0, 4-r); got != want {
			t.Errorf("row %d up is %+v, want %+v: the ramp at 1 − (r + 0.5) / h, 70%% of it showing", r, got, want)
		}
	}
}

func TestABurnDownWithNoRoomLeftRunsAlongItsFloorInDestructive(t *testing.T) {
	ch := downAt(clockOn(12, 0), 1, course{}, readOn(8, 0, 0.5), readOn(10, 0, 1))
	c := chartIn(ch, Screen(builtin(t, "nord")), 10, 2)
	if got, want := chartRows(chartIn(ch, Look{}, 10, 2), 2), []string{"    │", "██▁▁▁"}; !slices.Equal(got, want) {
		t.Errorf("the chart's rows are\n%s\nwant\n%s\nthe level, then the floor from where it ran out", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := c.at(3, 2).ink; got != errorInk {
		t.Errorf("the floor is in %+v, want state.destructive", got)
	}
}

func TestABurnDownHeadsFromTheLevelNowToWhereItsProjectionEnds(t *testing.T) {
	// Ten columns of an hour, from 08:00; now, 12:00, is column 4, the level
	// there what's left of what's used from 11:30: but where it's said, half
	// the window, 8 of 16 eighths.
	readings := func(used float64) []score.Reading {
		return []score.Reading{readOn(8, 0, 0), readOn(11, 30, cmp.Or(used, 0.5))}
	}
	tests := []struct {
		name    string
		heading course
		capAt   float64
		h       int
		used    float64
		want    []string
	}{
		{
			// It runs out at 16:00, column 8: a dot a cell from column 5,
			// at (c + 0.25) / 10 of the way, from the level now to the floor.
			name: "running out: to ✕ on the floor", heading: course{out: clockOn(16, 0)}, capAt: 1, h: 2,
			want: []string{"███ │", "█████⠂⠂⠄✕"},
		},
		{
			name: "running out within now's column: ✕ at now, over the level", heading: course{out: clockOn(12, 5)}, capAt: 1, h: 2,
			want: []string{"███ │", "████✕"},
		},
		{
			// A quarter used by now, four rows tall: its floor is the cap, at
			// half the window, rowOf(0.5) dot row 8 of 16, the third row
			// down; the cap's dots along it, at the right of every other
			// cell, on the cells left empty.
			name: "running out at its cap: ✕ on the cap's floor", heading: course{out: clockOn(16, 0), atCap: true}, capAt: 0.5, h: 4, used: 0.25,
			want: []string{"███ │", "█████⠂⠄⡀", "█████ ⠈ ✕", "█████"},
		},
		{
			// On pace for three quarters of it by its reset: to a quarter left,
			// at the chart's end, with no ✕.
			name: "past its reset: to the room left there", heading: course{left: new(0.25)}, capAt: 1, h: 2,
			want: []string{"███ │", "█████⠁⠁⠁⠂⠂"},
		},
		{
			name: "heading nowhere the chart draws: nothing ahead", capAt: 1, h: 2,
			want: []string{"███ │", "█████"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := downAt(clockOn(12, 0), tt.capAt, tt.heading, readings(tt.used)...)
			if got := chartRows(chartIn(ch, Look{}, 10, tt.h), tt.h); !slices.Equal(got, tt.want) {
				t.Errorf("the chart's rows are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
	nord := Screen(builtin(t, "nord"))
	c := chartIn(downAt(clockOn(12, 0), 1, course{out: clockOn(16, 0)}, readings(0)...), nord, 10, 2)
	for _, tt := range []struct {
		x, y int
		want ink
	}{{5, 2, dimInk}, {8, 2, outInk}, {4, 1, faintInk}} {
		if got := c.at(tt.x, tt.y); got.ink != tt.want {
			t.Errorf("%q is in %+v, want %+v", got.glyph, got.ink, tt.want)
		}
	}
}

func TestABurnDownHeldAtItsLimitRunsAlongItsFloorTillItLifts(t *testing.T) {
	// From 08:00, with a fifth used, room for 13 of 16 eighths; the limit
	// reached at 10:00; now, 12:00, is column 4.
	readings := []score.Reading{readOn(8, 0, 0.2)}
	tests := []struct {
		name    string
		heading course
		want    []string
	}{
		{
			// It lifts at 15:00, as column 7 starts: the floor from 10:00 to
			// now, then on through columns 5 and 6.
			name: "lifting at a time it's known", heading: course{held: true, since: clockOn(10, 0), back: clockOn(15, 0)},
			want: []string{"▅▅  │", "██▁▁▁▁▁"},
		},
		{
			name: "lifting at no known time: to the chart's end", heading: course{held: true, since: clockOn(10, 0)},
			want: []string{"▅▅  │", "██▁▁▁▁▁▁▁▁"},
		},
		{
			// Reaching the limit at no known time, the readings say what was
			// left till now, and the floor runs on from the column after.
			name: "reached at no known time: the floor from now on", heading: course{held: true, back: clockOn(15, 0)},
			want: []string{"▅▅▅▅▅", "█████▁▁"},
		},
		{
			name: "lifted: as it reads, and nothing ahead",
			want: []string{"▅▅▅▅▅", "█████"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := downAt(clockOn(12, 0), 1, tt.heading, readings...)
			if got := chartRows(chartIn(ch, Look{}, 10, 2), 2); !slices.Equal(got, tt.want) {
				t.Errorf("the chart's rows are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
	c := chartIn(downAt(clockOn(12, 0), 1, tests[0].heading, readings...), Screen(builtin(t, "nord")), 10, 2)
	for _, x := range []int{2, 6} {
		if got := c.at(x, 2).ink; got != errorInk {
			t.Errorf("the floor in column %d is in %+v, want state.destructive", x, got)
		}
	}
}

func TestABurnDownsCapIsAFaintBrailleLineAlongItsFloorOnTheCellsLeftEmpty(t *testing.T) {
	// Nine-tenths used: room for 3 of 32 eighths; the cap at half the window,
	// dot row 8 of 16, the third row down; now, 12:00, column 4, its line on
	// the rows the cap leaves it.
	readings := []score.Reading{readOn(8, 0, 0.9)}
	tests := []struct {
		name  string
		capAt float64
		held  course
		want  []string
	}{
		{name: "a cap", capAt: 0.5, want: []string{"    │", "    │", "⠈ ⠈ ⠈ ⠈ ⠈", "▃▃▃▃▃"}},
		{name: "no cap", capAt: 1, want: []string{"    │", "    │", "    │", "▃▃▃▃▃"}},
		{
			// A cap near the floor shares its row with the limit's line, which
			// takes the cells it's drawn on.
			name: "under the limit's line", capAt: 0.95, held: course{held: true},
			want: []string{"    │", "    │", "    │", "▃▃▃▃▃▁▁▁▁▁"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := downAt(clockOn(12, 0), tt.capAt, tt.held, readings...)
			if got := chartRows(chartIn(ch, Look{}, 10, 4), 4); !slices.Equal(got, tt.want) {
				t.Errorf("the chart's rows are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
	c := chartIn(downAt(clockOn(12, 0), 0.5, course{}, readings...), Screen(builtin(t, "nord")), 10, 4)
	if got := c.at(2, 3).ink; got != capDotInk {
		t.Errorf("the cap is in %+v, want viz.reserve, faint", got)
	}
}

func TestABurnDownHasNoEvenPace(t *testing.T) {
	ch := downAt(clockOn(12, 0), 1, course{}, readOn(8, 0, 0.5))
	if got := strings.Join(chartRows(chartIn(ch, Look{}, 30, 5), 5), "\n"); strings.Contains(got, paceDot) {
		t.Errorf("the burn-down draws\n%s\nwant no even pace", got)
	}
}

func TestABurnDownWithNoHistorySaysSoAndHeadsNowhere(t *testing.T) {
	ch := downAt(clockOn(12, 0), 1, course{out: clockOn(16, 0)})
	c := chartIn(ch, Look{}, 30, 5)
	all := strings.Join(chartRows(c, 5), "\n")
	if !strings.Contains(all, untracedLabel) || strings.Contains(all, outMark) {
		t.Errorf("the chart draws\n%s\nwant \"no history yet\" and nothing ahead, as the room now isn't known", all)
	}
}

func TestABurnDownHeadsWhereItsWindowsMomentSaysItIs(t *testing.T) {
	personal, reached := limitedAt("personal", now.Add(-20*time.Minute))
	stale := readAccount("stale", sessionOf(0.5, time.Hour), weekOf(0.2, 3*quota.Day))
	stale.Error = "timed out"
	unread := readAccount("unread", sessionOf(0.5, time.Hour))
	unread.FetchedAt = time.Time{}
	routed := withEvents(routerDoc("side", 0, pressedAccount("work"), personal, stale, unread), reached)
	nowhere := func(status.WindowNow) course { return course{} }
	tests := []struct {
		name         string
		doc          status.Document
		at           time.Time
		account, key string
		want         func(w status.WindowNow) course
	}{
		{
			name: "running out: where the moment's Out has it", doc: trio(), at: printNow, account: "personal", key: "7d",
			want: func(w status.WindowNow) course { return course{out: w.Out.At, atCap: w.Out.Reserve} },
		},
		{
			name: "running out at its rate lately", doc: routed, at: now, account: "work", key: "5h",
			want: func(w status.WindowNow) course { return course{out: w.Out.At} },
		},
		{
			name: "open: to the room its projection leaves it at its reset", doc: trio(), at: printNow, account: "side", key: "5h",
			want: func(w status.WindowNow) course { return course{left: new(1 - *w.Projected)} },
		},
		{
			name: "held at its limit: since it was reached, till it lifts", doc: routed, at: now, account: "personal", key: "5h",
			want: func(w status.WindowNow) course { return course{held: true, since: reached.At, back: w.Back} },
		},
		{name: "at its cap: nowhere", doc: trio(), at: printNow, account: "work", key: "5h", want: nowhere},
		{name: "its last read failed: nowhere", doc: routed, at: now, account: "stale", key: "5h", want: nowhere},
		{name: "nothing read: nowhere", doc: routed, at: now, account: "unread", key: "5h", want: nowhere},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := tt.doc.At(tt.at, claudeLike).Account(tt.account)
			w, _ := a.Window(tt.key)
			got, want := chartOf(a, shownWindow{key: tt.key, name: tt.key}, Trail{}, tt.at, BurnDown).course, tt.want(w)
			if !sameCourse(got, want) {
				t.Errorf("heading %s, want %s", courseSaid(got), courseSaid(want))
			}
		})
	}
}

// sameCourse reports whether a and b head the same way.
func sameCourse(a, b course) bool {
	return a.out.Equal(b.out) && a.atCap == b.atCap && a.held == b.held && a.since.Equal(b.since) && a.back.Equal(b.back) &&
		(a.left == nil) == (b.left == nil) && (a.left == nil || *a.left == *b.left)
}

// courseSaid is c as a test says it.
func courseSaid(c course) string {
	left := "nil"
	if c.left != nil {
		left = fmt.Sprint(*c.left)
	}
	return fmt.Sprintf("{out %v at cap %v left %s held %v since %v back %v}", c.out, c.atCap, left, c.held, c.since, c.back)
}
