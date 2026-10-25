package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// burnCardAt is a burn-down card of a window ten hours long from 08:00 on 1
// October 2026, as chartAt has it, at now, in the ramp's first stop, its
// readings as given, nothing ahead known.
func burnCardAt(now time.Time, readings ...score.Reading) burnCard {
	return burnCard{window: chartAt(now, 1, readings...), ink: ink{token: theme.VizRamp1}, ahead: steady(0, false)}
}

// steady is a room ahead that's room all along, known as given.
func steady(room float64, known bool) func(time.Time) (float64, bool) {
	return func(time.Time) (float64, bool) { return room, known }
}

// cardIn is cd drawn width cells wide in look, its chart h rows over its
// axis, on a canvas of its own.
func cardIn(cd burnCard, look Look, width, h int) *canvas {
	c := newCanvas(width, h+1)
	cd.draw(c, look, bounds{width: width, rows: h + 1})
	return c
}

// cardRows are the rows of a card drawn h rows tall on c, its chart's and
// its axis's, as text, top first.
func cardRows(c *canvas, h int) []string {
	rows := make([]string, h+1)
	for y := range rows {
		rows[y] = rowOf(c, y)
	}
	return rows
}

// roomFilled are the eighths each column of a card's chart h rows tall on
// c is filled to, read back from its cells: a cell on a surface counts
// eight, and the top one its eighth block, in the fill's ink.
func roomFilled(c *canvas, h int) []int {
	filled := make([]int, c.width)
	for x := range c.width {
		for r := range h {
			here := c.at(x, h-1-r)
			if here.ink.on != (hue{}) {
				filled[x] += 8
				continue
			}
			if i := slices.Index(levels[:], here.glyph); i > 0 && here.ink.fade > 0 {
				filled[x] += i
			}
			break
		}
	}
	return filled
}

func TestABurnCardFillsEachColumnBeforeNowToTheRoomItsMiddleHadInEighths(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		name     string
		readings []score.Reading
		want     []int
	}{
		// Ten columns of an hour each, from 08:00, two rows, 16 eighths; now,
		// 12:40, falls in column round(4.67) = 5, which holds no room.
		{name: "full", readings: []score.Reading{readOn(8, 0, 0)}, want: []int{16, 16, 16, 16, 16, 0, 0, 0, 0, 0}},
		{name: "half", readings: []score.Reading{readOn(8, 0, 0.5)}, want: []int{8, 8, 8, 8, 8, 0, 0, 0, 0, 0}},
		{name: "a sliver: its lowest eighth", readings: []score.Reading{readOn(8, 0, 0.99)}, want: []int{1, 1, 1, 1, 1, 0, 0, 0, 0, 0}},
		{name: "none left: nothing", readings: []score.Reading{readOn(8, 0, 1)}, want: []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{
			// 7.5 eighths left at column 2's middle: a half to even, 8.
			name:     "nothing read before the first reading, a half to even",
			readings: []score.Reading{readOn(10, 30, 0.53125)},
			want:     []int{0, 0, 8, 8, 8, 0, 0, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roomFilled(cardIn(burnCardAt(clockOn(12, 40), tt.readings...), nord, 10, 2), 2); !slices.Equal(got, tt.want) {
				t.Errorf("the columns are filled %v eighths, want %v: round(room × h × 8), one at least where any is left, before now's column alone", got, tt.want)
			}
		})
	}
}

func TestABurnCardsRoomIsInItsWindowsInkWith55PercentShowing(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0.4))
	cd.ink = warningInk
	c := cardIn(cd, nord, 10, 2)
	// 0.6 of two rows is 9.6 eighths, 10: a full cell, and two eighths.
	want := []cell{
		{glyph: " ", ink: ink{on: hue{token: theme.AccentAttention, fade: 0.45}}},
		{glyph: levels[2], ink: ink{token: theme.AccentAttention, fade: 0.45}},
	}
	for r, w := range want {
		if got := *c.at(0, 1-r); got != w {
			t.Errorf("row %d up is %+v, want %+v", r, got, w)
		}
	}
}

func TestAWindowsInkIsByTheStateItsIn(t *testing.T) {
	ramp := ink{token: theme.VizRamp1}
	tests := []struct {
		kind    status.WindowKind
		refused bool
		want    ink
	}{
		{kind: status.WindowOpen, want: ramp},
		{kind: status.WindowRunsOut, want: warningInk},
		{kind: status.WindowCapped, want: warningInk},
		{kind: status.WindowLimited, want: errorInk},
		{kind: status.WindowStale, refused: true, want: errorInk},
		{kind: status.WindowStale, want: ramp},
	}
	for _, tt := range tests {
		got := windowInk(status.AccountNow{TokenRefused: tt.refused}, status.WindowNow{Kind: tt.kind})
		if got != tt.want {
			t.Errorf("a window of kind %d, its token refused %v, is in %+v, want %+v", tt.kind, tt.refused, got, tt.want)
		}
	}
}

func TestADotsRowIsTheCeilingOfItsRoomOf4hLessOneFromTheFoot(t *testing.T) {
	d := newDots(2)
	tests := []struct {
		room float64
		want int
	}{
		{room: -0.1, want: 7},
		{room: 0, want: 7},
		{room: 0.01, want: 7},
		{room: 0.125, want: 7},
		{room: 0.13, want: 6},
		{room: 0.5, want: 4},
		{room: 0.99, want: 0},
		{room: 1, want: 0},
		{room: 1.2, want: 0},
	}
	for _, tt := range tests {
		if got := d.footRow(tt.room); got != tt.want {
			t.Errorf("room %v is on dot row %d from the top of 8, want %d", tt.room, got, tt.want)
		}
	}
}

func TestABurnCardHeadsFromNowsColumnADotAHalfColumnAtTheRoomItsMiddleHasAhead(t *testing.T) {
	// Now, 12:20, is column round(4.33) = 4: its half columns' middles, from
	// 12:15, are a half hour apart, the first before now, so taken at now.
	now := clockOn(12, 20)
	var asked []time.Time
	cd := burnCardAt(now, readOn(8, 0, 0))
	cd.ahead = func(t time.Time) (float64, bool) {
		asked = append(asked, t)
		return 1, true
	}
	c := cardIn(cd, Look{}, 10, 2)
	want := []time.Time{now}
	for at := clockOn(12, 45); at.Before(clockOn(18, 0)); at = at.Add(30 * time.Minute) {
		want = append(want, at)
	}
	if !slices.EqualFunc(asked, want, time.Time.Equal) {
		t.Errorf("the room ahead is asked at %v, want %v", asked, want)
	}
	if got, want := cardRows(c, 2)[:2], []string{"████⠉⠉⠉⠉⠉⠉", "████│"}; !slices.Equal(got, want) {
		t.Errorf("the chart is\n%s\nwant\n%s\na dot each half column from now's, all the room on the top row, now's │ under them", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for x := 4; x < 10; x++ {
		if got := c.at(x, 0).ink; got != cd.ink {
			t.Errorf("the heading's column %d is in %+v, want the window's ink, %+v", x, got, cd.ink)
		}
	}
}

func TestABurnCardsReserveDotsFallOnEmptyCellsAlone(t *testing.T) {
	// A quarter of two rows is dot row 6 of 8: the third row of the lower
	// cell, ⠠ the right-hand dot of it, every other column from the first.
	tests := []struct {
		name  string
		ahead func(time.Time) (float64, bool)
		holds bool
		want  []string
	}{
		{name: "where it holds", ahead: steady(0, false), holds: true, want: []string{"█████│", "█████│⠠ ⠠"}},
		{name: "where a pin spends it", ahead: steady(0, false), want: []string{"█████│", "█████│"}},
		{name: "under where it's heading", ahead: steady(0.25, true), holds: true, want: []string{"█████│", "█████⠤⠤⠤⠤⠤"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0))
			cd.ahead, cd.reserve, cd.reserveHolds = tt.ahead, 0.25, tt.holds
			c := cardIn(cd, Look{}, 10, 2)
			if got := cardRows(c, 2)[:2]; !slices.Equal(got, tt.want) {
				t.Errorf("the chart is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if here := c.at(6, 1); tt.holds && here.glyph == "⠠" && here.ink != capDotInk {
				t.Errorf("the reserve is in %+v, want viz.reserve 60%% showing", here.ink)
			}
		})
	}
}

func TestABurnCardsNowLineRunsDownNowsColumnFromItsStartToNearItsReset(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want []string
	}{
		{name: "at its start", now: clockOn(8, 0), want: []string{"│", "│"}},
		// 17:50 is column round(9.83) = 10, the last, 9, at the most.
		{name: "near its reset", now: clockOn(17, 50), want: []string{"█████████│", "█████████│"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cardRows(cardIn(burnCardAt(tt.now, readOn(8, 0, 0)), Look{}, 10, 2), 2)[:2]; !slices.Equal(got, tt.want) {
				t.Errorf("the chart is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestABurnCardsCrossFallsOnItsFloorWhereItRunsOutOrItsHoldBegan(t *testing.T) {
	// Four rows, 16 dot rows; now, 12:40, column 5; a fifth of the window
	// left on the floor, dot row 3 from the foot, so ✕ on the lowest row.
	tests := []struct {
		name      string
		out, held time.Time
		holds     bool
		wantAt    [2]int
		crossed   bool
	}{
		{name: "running out", out: clockOn(15, 30), wantAt: [2]int{7, 3}, crossed: true},
		{name: "running out within now's column: ✕ at now", out: clockOn(12, 45), wantAt: [2]int{5, 3}, crossed: true},
		{name: "held since its hold began", held: clockOn(10, 10), holds: true, wantAt: [2]int{2, 3}, crossed: true},
		{name: "held, nothing telling since when: no ✕", holds: true},
		{name: "heading nowhere it runs out: no ✕"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0))
			cd.out, cd.held, cd.heldSince, cd.floor = tt.out, tt.holds, tt.held, 0.2
			c := cardIn(cd, Look{}, 10, 4)
			var crosses [][2]int
			for y := range 4 {
				for x := range 10 {
					if c.at(x, y).glyph == outMark {
						crosses = append(crosses, [2]int{x, y})
					}
				}
			}
			if !tt.crossed {
				if len(crosses) > 0 {
					t.Errorf("✕ is at %v, want none", crosses)
				}
				return
			}
			if !slices.Equal(crosses, [][2]int{tt.wantAt}) {
				t.Fatalf("✕ is at %v, want it at %v alone", crosses, tt.wantAt)
			}
			if got := c.at(tt.wantAt[0], tt.wantAt[1]).ink; got != outInk {
				t.Errorf("✕ is in %+v, want bold state.destructive", got)
			}
		})
	}
}

func TestABurnCardsCrossIsDrawnLastOverTheRoom(t *testing.T) {
	// Held since 09:10, column 1, on the floor's row, over the room filled
	// there before the hold.
	cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0))
	cd.held, cd.heldSince = true, clockOn(9, 10)
	if got := cardRows(cardIn(cd, Look{}, 10, 2), 2)[1]; got != "█✕███│" {
		t.Errorf("the chart's lower row is %q, want ✕ over the room in the column the hold began", got)
	}
}

func TestATokenlessCardHasNoRoomAllAlong(t *testing.T) {
	cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0))
	cd.tokenless = true
	if got, want := cardRows(cardIn(cd, Look{}, 10, 2), 2)[:2], []string{"     │", "     │"}; !slices.Equal(got, want) {
		t.Errorf("the chart is\n%s\nwant\n%s\nnow's │ alone", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestACardWhoseSpanIsntKnownDrawsNothing(t *testing.T) {
	cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0))
	cd.window.spanned = false
	for _, row := range cardRows(cardIn(cd, Look{}, 10, 2), 2) {
		if row != "" {
			t.Fatalf("a row says %q, want nothing drawn", row)
		}
	}
}

func TestAStaleCardOrOneOutOfThePoolIsFaded(t *testing.T) {
	tests := []struct {
		name         string
		kind         status.WindowKind
		refused, out bool
		want         float64
	}{
		{name: "as it stands", kind: status.WindowOpen},
		{name: "stale", kind: status.WindowStale, want: staleFade},
		{name: "its token refused, which its ink says", kind: status.WindowStale, refused: true},
		{name: "out of the pool", kind: status.WindowOpen, out: true, want: staleFade},
		{name: "stale and out of the pool", kind: status.WindowStale, out: true, want: 1 - (1-staleFade)*(1-staleFade)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cardFade(status.AccountNow{TokenRefused: tt.refused, OutOfPool: tt.out}, status.WindowNow{Kind: tt.kind})
			if !sameFade(ink{fade: got}, ink{fade: tt.want}) {
				t.Errorf("the card fades %v, want %v", got, tt.want)
			}
		})
	}
	// Heading at three quarters, on the upper row; the reserve at a
	// quarter, on the lower.
	cd := burnCardAt(clockOn(12, 40), readOn(8, 0, 0.5))
	cd.fade, cd.ahead, cd.reserve, cd.reserveHolds = staleFade, steady(0.75, true), 0.25, true
	c := cardIn(cd, Look{}, 10, 2)
	for _, at := range []struct {
		name string
		x, y int
		want ink
	}{
		{"the room", 0, 1, ink{token: theme.VizRamp1, fade: 1 - 0.55*(1-staleFade), dim: true}},
		{"where it's heading", 6, 0, ink{token: theme.VizRamp1, fade: staleFade, dim: true}},
		{"the reserve", 6, 1, ink{token: theme.VizReserve, fade: 1 - 0.6*(1-staleFade), dim: true}},
	} {
		if got := c.at(at.x, at.y).ink; !sameFade(got, at.want) {
			t.Errorf("%s is in %+v, want %+v", at.name, got, at.want)
		}
	}
}

// sameFade reports whether a and b are the same ink, their fades within a
// rounding of each other.
func sameFade(a, b ink) bool {
	close := a.fade-b.fade < 1e-9 && b.fade-a.fade < 1e-9
	a.fade, b.fade = 0, 0
	return close && a == b
}

// A card's figures are the moment's: the room now; the room ahead, from it;
// and ✕ in the column the window runs out in, on its floor, the reserve's
// where that holds the account back, as the moment's Out has it; held, at
// its cap, its floor the cap's, its dots along it.
func TestABurnCardAgreesWithTheMoment(t *testing.T) {
	m := trio().At(printNow, printPolicy)
	for _, a := range m.Accounts {
		for _, key := range []string{"7d", "5h"} {
			w, _ := a.Window(key)
			cd := burnCardOf(a, key, Trail{}, printNow)
			if cd.room != 1-w.Used {
				t.Errorf("%s's %s has room %v now, want the moment's %v", a.ID, key, cd.room, 1-w.Used)
			}
			if room, ok := cd.ahead(printNow); ok && room != cd.room {
				t.Errorf("%s's %s heads from %v, want the room now, %v", a.ID, key, room, cd.room)
			}
			c := cardIn(cd, Look{}, 50, 8)
			x, y, crossed := crossOn(c, 50, 8)
			if crossed != w.RunsOut {
				t.Errorf("%s's %s has ✕ %v, want %v, as the moment has it run out", a.ID, key, crossed, w.RunsOut)
				continue
			}
			tl := views.Timeline{Start: w.Start, End: w.Start.Add(w.Length), Columns: 50}
			if want := tl.Column(w.Out.At); crossed && x != want {
				t.Errorf("%s's %s has ✕ in column %d, want %d, where the moment's Out.At falls", a.ID, key, x, want)
			}
			if crossed && y != 7 {
				t.Errorf("%s's %s has ✕ on row %d, want its floor's, the foot", a.ID, key, y)
			}
		}
	}
}

// crossOn is where ✕ is on a card's chart drawn width cells wide and h rows
// tall on c, reporting false where it isn't.
func crossOn(c *canvas, width, h int) (x, y int, ok bool) {
	for y := range h {
		for x := range width {
			if c.at(x, y).glyph == outMark {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func TestACardHeldAtItsCapCrossesWhereItsHoldBeganAndHeadsAlongTheCap(t *testing.T) {
	m := trio().At(printNow, printPolicy)
	work, _ := m.Account("work")
	cd := burnCardOf(work, "5h", Trail{}, printNow)
	if !cd.held || cd.floor != 1-work.Cap {
		t.Fatalf("work's 5-hour is held %v on a floor of %v, want held on its cap's, %v", cd.held, cd.floor, 1-work.Cap)
	}
	// The 5-hour runs 12:10 to 17:10, ten columns of half an hour; now,
	// 14:42, column 5; the cap, 5% of four rows, on the lowest dot row,
	// where the reserve's dots show every other column till now's.
	cd.heldSince = on(1, 13, 20)
	want := []string{"     │", "     │", "     │", "⢀ ✕ ⢀⣀⣀⣀⣀⣀"}
	if got := cardRows(cardIn(cd, Look{}, 10, 4), 4)[:4]; !slices.Equal(got, want) {
		t.Errorf("the chart is\n%s\nwant\n%s\n✕ where its hold began, its dots along the cap from now's column", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	cd.heldSince = time.Time{}
	if _, _, crossed := crossOn(cardIn(cd, Look{}, 10, 4), 10, 4); crossed {
		t.Error("with nothing telling when its hold began, it has ✕, want none")
	}
}

func TestACardHeldByItsLimitCrossesOnItsFloor(t *testing.T) {
	doc := threeRouted()
	personal, _ := doc.At(now, claudeLike).Account("personal")
	w, _ := personal.Window("5h")
	if w.Kind != status.WindowLimited {
		t.Fatalf("personal's 5-hour is of kind %d, want held by its limit", w.Kind)
	}
	cd := burnCardOf(personal, "5h", Trail{}, now)
	cd.heldSince = now.Add(-time.Hour)
	if cd.ink != errorInk || cd.floor != 0 {
		t.Errorf("held by its limit, the card is in %+v on a floor of %v, want state.destructive on none", cd.ink, cd.floor)
	}
	// The 5-hour runs from four hours before now for five: now's column is
	// 8 of 10, the hold's start 6; none left till it resets, on the foot.
	want := []string{"        │", "      ✕ ⣀⣀"}
	if got := cardRows(cardIn(cd, Look{}, 10, 2), 2)[:2]; !slices.Equal(got, want) {
		t.Errorf("the chart is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestACardRunningOutHeadsAlongItsFloorFromItsCrossToItsReset(t *testing.T) {
	// side's week runs out on Friday 2 October at 15:00 UTC, 115 hours into
	// it, column 34 of 50; empty from then, on the foot's lowest dot row.
	side, _ := trio().At(printNow, printPolicy).Account("side")
	c := cardIn(burnCardOf(side, "7d", Trail{}, printNow), Look{}, 50, 8)
	if got, want := rowOf(c, 7), strings.Repeat(" ", 27)+"│  ⠉⠒⠤⣀✕"+strings.Repeat("⣀", 15); got != want {
		t.Errorf("the foot is\n%q\nwant\n%q\nfalling from now's column to ✕, then along the floor to its reset", got, want)
	}
}
