package dashboard

import (
	"fmt"
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// laneCells is a lane of the cells given, one a column, then room to the
// timeline's last of 136 columns.
func laneCells(cells ...views.Cell) []views.Cell {
	return slices.Concat(cells, slices.Repeat([]views.Cell{views.CellRoom}, 136-len(cells)))
}

// drawnLane is the canvas f draws the lane ln on, as runwayLanes draws it,
// its lanes from framesLanesFrom.
func drawnLane(f Frame, ln laidLane) *canvas {
	c := newCanvas(f.Width, 20)
	f.runwayLanes(c, runwayLaid{m: status.Moment{Now: routedAt}, x: framesLanesFrom, lanes: []laidLane{ln}})
	return c
}

func TestALaneDrawsEachColumnAsItsAccountStoodThen(t *testing.T) {
	nord, plain := Screen(builtin(t, "nord")), NoColour()
	tests := []struct {
		name  string
		look  Look
		cell  views.Cell
		glyph string
		token theme.Token
		bold  bool
	}{
		{name: "nothing known, left blank", look: nord, cell: views.CellUnknown, glyph: " "},
		{name: "with room", look: nord, cell: views.CellRoom, glyph: "█", token: theme.VizRamp1},
		{name: "running out at its pace", look: nord, cell: views.CellRunsOut, glyph: "█", token: theme.AccentAttention},
		{name: "at its cap", look: nord, cell: views.CellCapped, glyph: "─", token: theme.AccentAttention},
		{name: "at its limit", look: nord, cell: views.CellLimited, glyph: "─", token: theme.StateDestructive},
		{name: "its token refused", look: nord, cell: views.CellRefused, glyph: "─", token: theme.StateDestructive},
		{name: "without a token", look: nord, cell: views.CellTokenless, glyph: "─", token: theme.StateDestructive},
		{name: "running out, without colour, shaded", look: plain, cell: views.CellRunsOut, glyph: "▒", token: theme.AccentAttention},
		{name: "at its cap, without colour, plain", look: plain, cell: views.CellCapped, glyph: "─", token: theme.AccentAttention},
		{name: "at its limit, without colour, bold", look: plain, cell: views.CellLimited, glyph: "─", token: theme.StateDestructive, bold: true},
		{name: "its token refused, without colour, bold", look: plain, cell: views.CellRefused, glyph: "─", token: theme.StateDestructive, bold: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(runwayLive(views.RunwayFiveHour))
			f.Look = tt.look
			ln := laneWith()
			ln.Cells = laneCells(views.CellRoom, views.CellRoom, views.CellRoom, tt.cell)
			got := drawnLane(f, ln).at(framesLanesFrom+3, 10)
			blank := tt.glyph == " " && got.ink != ink{}
			if got.glyph != tt.glyph || blank || tt.glyph != " " && (got.ink.token != tt.token || got.ink.bold != tt.bold) {
				t.Errorf("column 3 is %q in %v, want %q in %v, bold %t", got.glyph, got.ink, tt.glyph, tt.token, tt.bold)
			}
		})
	}
}

// A lane's past, each column before now's, keeps 45% of its colour; a lane
// stale or out of the pool keeps 35% of its, the two compounding, and its
// past 45% of that; a refused token's lane isn't faded for its stale use, as
// it draws the refusal. Where the look can't blend, nothing's dimmed.
func TestALanesPastIsDimmedAndAStaleOrOutOfThePoolLaneFaded(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	canvas := nord.Canvas()
	ramp := builtin(t, "nord").Colour(theme.VizRamp1)
	tests := []struct {
		name               string
		look               Look
		account            status.AccountNow
		use                views.Use
		shown              RunwayShown
		past, now          color.Color
		pastDim, nowFaints bool
	}{
		{name: "live", look: nord, shown: runwayLive(views.RunwayFiveHour), past: theme.Mix(ramp, canvas, 0.55), now: ramp},
		{
			name: "stale", look: nord, use: views.Use{Stale: true}, shown: runwayLive(views.RunwayFiveHour),
			past: theme.Mix(ramp, canvas, 1-0.45*0.35), now: theme.Mix(ramp, canvas, 0.65), pastDim: true, nowFaints: true,
		},
		{
			name: "out of the pool", look: nord, account: status.AccountNow{OutOfPool: true}, shown: runwayLive(views.RunwayFiveHour),
			past: theme.Mix(ramp, canvas, 1-0.45*0.35), now: theme.Mix(ramp, canvas, 0.65), pastDim: true, nowFaints: true,
		},
		{
			name: "stale and out of the pool", look: nord, account: status.AccountNow{OutOfPool: true}, use: views.Use{Stale: true}, shown: runwayLive(views.RunwayFiveHour),
			past: theme.Mix(ramp, canvas, 1-0.45*0.35*0.35), now: theme.Mix(ramp, canvas, 1-0.35*0.35), pastDim: true, nowFaints: true,
		},
		{
			name: "its token refused", look: nord, account: status.AccountNow{TokenRefused: true}, use: views.Use{Stale: true}, shown: runwayLive(views.RunwayFiveHour),
			past: theme.Mix(ramp, canvas, 0.55), now: ramp,
		},
		{name: "a day gone, none past", look: nord, shown: runwayBack(views.RunwayFiveHour, civilDay(9, 30)), past: ramp, now: ramp},
		{name: "a look that can't blend", look: Screen(builtin(t, "terminal")), shown: runwayLive(views.RunwayFiveHour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(tt.shown)
			f.Look = tt.look
			ln := laneWith()
			ln.Cells, ln.Use, ln.account = laneCells(), tt.use, tt.account
			c := drawnLane(f, ln)
			if tt.past == nil {
				before, after := c.at(framesLanesFrom+5, 10), c.at(framesLanesFrom+6, 10)
				if tt.look.colour(before.ink) != tt.look.colour(after.ink) || tt.look.faint(before.ink) != tt.look.faint(after.ink) {
					t.Errorf("the column before now's is drawn otherwise than now's, where the look can't dim it")
				}
				return
			}
			past, now := c.at(framesLanesFrom+5, 10), c.at(framesLanesFrom+6, 10)
			if !shadeOf(f.Look.colour(past.ink)).near(shadeOf(tt.past)) || !shadeOf(f.Look.colour(now.ink)).near(shadeOf(tt.now)) {
				t.Errorf("the past is %s and now %s, want %s and %s", shadeOf(f.Look.colour(past.ink)), shadeOf(f.Look.colour(now.ink)), shadeOf(tt.past), shadeOf(tt.now))
			}
			if past.ink.dim != tt.pastDim || now.ink.dim != tt.nowFaints {
				t.Errorf("the past and now are drawn faint where the look can't fade them, %t and %t, want %t and %t", past.ink.dim, now.ink.dim, tt.pastDim, tt.nowFaints)
			}
		})
	}
}

func TestALaneMarksEachResetOfTheViewsWindowItHas(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	f := runwayShowing(runwayBack(views.RunwayFiveHour, civilDay(10, 1)))
	f.Look = nord
	ln := laneWith()
	ln.Cells, ln.Resets = laneCells(), []views.Reset{{Column: 3, At: routedClock(1, 0, 30)}, {Column: 120, At: routedClock(1, 21, 15), Back: true}}
	c := drawnLane(f, ln)
	subtle, canvas := builtin(t, "nord").Colour(theme.TextSubtle), nord.Canvas()
	for _, tt := range []struct {
		col  int
		want color.Color
	}{{col: 3, want: theme.Mix(subtle, canvas, 0.55)}, {col: 120, want: subtle}} {
		got := c.at(framesLanesFrom+tt.col, 10)
		if got.glyph != "│" || !shadeOf(nord.colour(got.ink)).near(shadeOf(tt.want)) {
			t.Errorf("column %d is %q in %s, want │ in %s, dimmed before now's column", tt.col, got.glyph, shadeOf(nord.colour(got.ink)), shadeOf(tt.want))
		}
	}
	if got := c.at(framesLanesFrom+4, 10).glyph; got != "█" {
		t.Errorf("column 4, which marks no reset, is %q, want █", got)
	}
}

// Each lane is headed by its account's name, cut to the label column, its
// number and marks kept, and under it, its use of the view's window from the
// second column: live, its share, bold in its level's colour and numbered;
// stepped back, the highest the days read, so too, or —; not read, why,
// always short, so the column holds still as an account goes stale, at its
// most too; and out of the pool, the whole label faint.
func TestEachLaneIsHeadedByItsNameAndItsUseOfTheViewsWindow(t *testing.T) {
	work := accountName{number: "1", label: "work"}
	stale := views.Use{Key: "5h", Used: 0.95, Read: true, Stale: true, ReadAt: routedClock(1, 14, 20)}
	staleBefore := stale
	staleBefore.ReadAt = time.Date(2026, 9, 30, 14, 20, 0, 0, time.UTC)
	tests := []struct {
		name    string
		window  views.RunwayWindow
		x       int
		lane    laidLane
		head    string
		use     string
		useInks []ink
	}{
		{
			name: "live, 95%, red", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "5h", Used: 0.95, Read: true}}},
			head: "1 work", use: "5-hour 95%", useInks: []ink{dimInk, usedFigure(0.95, "work", "5h")},
		},
		{
			name: "live, the week, 78%, yellow", window: views.RunwayWeek, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "7d", Used: 0.78, Read: true}}},
			head: "1 work", use: "week 78%", useInks: []ink{dimInk, usedFigure(0.78, "work", "7d")},
		},
		{
			name: "stepped back, the day's highest", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "5h", Used: 1, Read: true, Highest: true}}},
			head: "1 work", use: "5-hour 100%", useInks: []ink{dimInk, usedFigure(1, "work", "5h")},
		},
		{
			name: "stepped back, nothing read", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "5h", Highest: true}}},
			head: "1 work", use: "5-hour —", useInks: []ink{dimInk, faintInk},
		},
		{name: "stale, short", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: stale}}, head: "1 work", use: "read 14:20", useInks: []ink{dimInk}},
		{name: "stale since a day before, short", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: staleBefore}}, head: "1 work", use: "read Wed", useInks: []ink{dimInk}},
		{name: "not read yet, short", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "5h", Unread: true}}}, head: "1 work", use: "not read", useInks: []ink{dimInk}},
		{name: "without a token", window: views.RunwayFiveHour, lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Tokenless: true}}}, head: "1 work", use: "no token", useInks: []ink{errorInk}},
		{
			name: "out of the pool, faint", window: views.RunwayFiveHour, lane: laidLane{
				name: accountName{number: "2", label: "personal", primary: true, out: true, faint: true}, account: status.AccountNow{OutOfPool: true},
				Lane: views.Lane{ID: "personal", Use: views.Use{Key: "5h", Used: 0.39, Read: true}},
			},
			head: "2 personal ◆", use: "5-hour 39%", useInks: []ink{faintInk, faintInk},
		},
		{
			name: "the widest use, whole", window: views.RunwayFiveHour, lane: laidLane{
				name: accountName{number: "2", label: "personal", primary: true}, Lane: views.Lane{ID: "personal", Use: views.Use{Key: "5h", Used: 1, Read: true, Highest: true}},
			},
			head: "2 personal ◆", use: "5-hour 100%", useInks: []ink{dimInk, usedFigure(1, "personal", "5h")},
		},
		{
			name: "in a label column too narrow for it, cut, its number, marks and ▲'s cells kept", window: views.RunwayWeek, x: 14, lane: laidLane{
				name: accountName{number: "2", label: "personal", primary: true}, Lane: views.Lane{ID: "personal", Use: views.Use{Key: "7d", Used: 1, Read: true}},
			},
			head: "2 perso… ◆", use: "week 100%", useInks: []ink{dimInk, usedFigure(1, "personal", "7d")},
		},
		{
			name: "not read yet, short at the column's most too", window: views.RunwayFiveHour, x: mostRunwayLabels,
			lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "5h", Unread: true}}}, head: "1 work", use: "not read", useInks: []ink{dimInk},
		},
		{
			name: "stale, short at the column's most too", window: views.RunwayFiveHour, x: mostRunwayLabels,
			lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: stale}}, head: "1 work", use: "read 14:20", useInks: []ink{dimInk},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, x := runwayShowing(runwayLive(tt.window)), cmpOr(tt.x, framesLanesFrom)
			ln := tt.lane
			ln.row = 10
			c := newCanvas(160, 20)
			f.runwayLanes(c, runwayLaid{m: status.Moment{Now: routedAt}, x: x, lanes: []laidLane{ln}})
			if head := textAt(c, 0, 10, x-2); head != tt.head {
				t.Errorf("the lane's head reads %q, want %q", head, tt.head)
			}
			use := ln.useLine(runwayNames[tt.window], routedAt, x-runwayUseFrom-1)
			var inks []ink
			for _, s := range use {
				inks = append(inks, s.ink)
			}
			if got := textAt(c, runwayUseFrom, 11, x-runwayUseFrom); got != tt.use || !slices.Equal(inks, tt.useInks) {
				t.Errorf("the use under it reads %q in %v, want %q in %v", got, inks, tt.use, tt.useInks)
			}
		})
	}
}

// With the account column collapsed, each lane is headed by its number and
// marks, as its name heads it, and under them, its use of the view's
// window, the figure the expanded label gives, in its ink; or — in
// text.faint where the expanded label words why there's none; out of the
// pool, all of it faint. The lane starts a cell after the widest of the
// labels, ▲'s cells kept on each and a use of 100% among them, at every width.
func TestACollapsedLaneIsHeadedByItsNumberAndMarksAndItsUseUnderThem(t *testing.T) {
	work := accountName{number: "1", label: "work"}
	personal := accountName{number: "2", label: "personal", primary: true}
	side := accountName{number: "3", label: "side", best: true}
	three := []accountName{work, personal, side}
	ten := accountName{number: "10", label: "spare", primary: true, best: true}
	read := func(used float64) views.Use { return views.Use{Key: "7d", Used: used, Read: true} }
	tests := []struct {
		name      string
		among     []accountName
		lane      laidLane
		width     int
		head, use string
		figure    ink
		from      int
	}{
		{name: "no marks, green", lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: read(0.34)}}, head: "1", use: "34%", figure: usedFigure(0.34, "work", "7d"), from: 6},
		{name: "primary, yellow", lane: laidLane{name: personal, Lane: views.Lane{ID: "personal", Use: read(0.84)}}, head: "2 ◆", use: "84%", figure: usedFigure(0.84, "personal", "7d"), from: 6},
		{
			name: "new sessions go here, at its limit, red, the widest use", lane: laidLane{name: side, Lane: views.Lane{ID: "side", Use: read(1)}},
			head: "3 ▲", use: "100%", figure: usedFigure(1, "side", "7d"), from: 6,
		},
		{
			name: "both marks and the widest use", among: []accountName{work, {number: "2", label: "personal", primary: true, best: true}},
			lane: laidLane{name: accountName{number: "2", label: "personal", primary: true, best: true}, Lane: views.Lane{ID: "personal", Use: read(1)}},
			head: "2 ◆ ▲", use: "100%", figure: usedFigure(1, "personal", "7d"), from: 6,
		},
		{
			name: "numbered from ten", among: []accountName{{number: " 3", label: "side"}, ten},
			lane: laidLane{name: accountName{number: " 3", label: "side"}, Lane: views.Lane{ID: "side", Use: read(0.05)}}, head: " 3", use: "5%", figure: usedFigure(0.05, "side", "7d"), from: 7,
		},
		{
			name: "numbered from ten, both marks and the widest use", among: []accountName{{number: " 3", label: "side"}, ten},
			lane: laidLane{name: ten, Lane: views.Lane{ID: "spare", Use: read(1)}}, head: "10 ◆ ▲", use: "100%", figure: usedFigure(1, "spare", "7d"), from: 7,
		},
		{name: "on a phone, alike", width: 52, lane: laidLane{name: personal, Lane: views.Lane{ID: "personal", Use: read(0.84)}}, head: "2 ◆", use: "84%", figure: usedFigure(0.84, "personal", "7d"), from: 6},
		{name: "nothing read", lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "7d"}}}, head: "1", use: "—", figure: faintInk, from: 6},
		{name: "stale", lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "7d", Used: 0.4, Read: true, Stale: true}}}, head: "1", use: "—", figure: faintInk, from: 6},
		{name: "not read yet", lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Key: "7d", Unread: true}}}, head: "1", use: "—", figure: faintInk, from: 6},
		{name: "without a token", lane: laidLane{name: work, Lane: views.Lane{ID: "work", Use: views.Use{Tokenless: true}}}, head: "1", use: "—", figure: faintInk, from: 6},
		{
			name: "out of the pool, faint", lane: laidLane{
				name: accountName{number: "2", label: "personal", primary: true, out: true, faint: true}, account: status.AccountNow{OutOfPool: true}, Lane: views.Lane{ID: "personal", Use: read(0.84)},
			},
			head: "2 ◆", use: "84%", figure: faintInk, from: 6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			among := three
			if tt.among != nil {
				among = tt.among
			}
			r := runwayLive(views.RunwayWeek)
			r.Collapsed, r.labels = true, runwayLabelsOf(among)
			f := runwayShowing(r)
			f.Width = cmpOr(tt.width, 160)
			ln := tt.lane
			ln.row, ln.Cells = 10, laneCells()
			c := newCanvas(f.Width, 20)
			x := r.lanesFrom(f.Width)
			f.runwayLanes(c, runwayLaid{m: status.Moment{Now: routedAt}, x: x, lanes: []laidLane{ln}})
			if x != tt.from {
				t.Errorf("the lanes start at column %d, want %d", x, tt.from)
			}
			if got := textAt(c, 0, 10, x); got != tt.head {
				t.Errorf("the lane is headed %q, want %q", got, tt.head)
			}
			if got := textAt(c, 0, 11, x); got != tt.use {
				t.Errorf("under its head, the use reads %q, want %q", got, tt.use)
			}
			if got := ln.collapsedUse().ink; got != tt.figure {
				t.Errorf("the use is in %+v, want %+v", got, tt.figure)
			}
			number := dimInk
			if ln.account.OutOfPool {
				number = faintInk
			}
			if got := c.at(len(ln.name.number)-1, 10).ink; got != number {
				t.Errorf("the number is in %+v, want %+v", got, number)
			}
			if got := c.at(x, 10).glyph; got != roomCell {
				t.Errorf("column %d, the lane's first, is %q, want █", x, got)
			}
		})
	}
}

// usedFigure is the ink of a share used of an account's window, bold in its
// level's colour and numbered as its use.
func usedFigure(used float64, account, key string) ink {
	return usedInk(used, ink{token: theme.VizRamp1, bold: true}).numbered(Number{Kind: NumberUsed, Account: account, Of: key})
}

// cmpOr is n, or else where it's none.
func cmpOr(n, otherwise int) int {
	if n == 0 {
		return otherwise
	}
	return n
}

// textAt is the text along row y from column x, cells long or to the
// canvas's edge, its blanks at the end left off.
func textAt(c *canvas, x, y, cells int) string {
	var text []rune
	for i := range min(cells, c.width-x) {
		text = append(text, []rune(c.at(x+i, y).glyph)...)
	}
	for len(text) > 0 && text[len(text)-1] == ' ' {
		text = text[:len(text)-1]
	}
	return string(text)
}

// Now's column is picked out on bg.subtle from the axis's first row down to
// the pool's lane's last row, under every cell without a surface of its own, where
// the timeline has a now: on a day gone, nothing; nor where the look can't
// blend, as it can't dim the past either; nor where nothing fits.
func TestNowsColumnIsPickedOutDownToThePoolsLane(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	fitted := runwayFit{fits: true, axis: 6, lanes: 10, apart: 3, shown: 3, pool: 19}
	for _, tt := range []struct {
		name  string
		shown RunwayShown
		look  Look
		fit   runwayFit
		col   int
	}{
		{name: "live", shown: runwayLive(views.RunwayFiveHour), look: nord, fit: fitted, col: 6},
		{name: "today, stepped back to", shown: runwayBack(views.RunwayFiveHour, civilDay(10, 1)), look: nord, fit: fitted, col: 83},
		{name: "a day gone", shown: runwayBack(views.RunwayFiveHour, civilDay(9, 30)), look: nord, fit: fitted, col: -1},
		{name: "a look that can't blend", shown: runwayLive(views.RunwayFiveHour), look: Screen(builtin(t, "terminal")), fit: fitted, col: -1},
		{name: "nothing fits", shown: runwayLive(views.RunwayFiveHour), look: nord, col: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayShowing(tt.shown)
			f.Look = tt.look
			c := newCanvas(160, 30)
			x := framesLanesFrom + max(tt.col, 6)
			c.text(x, 15, " ", ink{on: hue{token: theme.VizRamp1}})
			f.runwayNow(c, runwayLaid{x: framesLanesFrom, fit: tt.fit})
			for y := range 30 {
				want := hue{}
				switch {
				case y == 15:
					want = hue{token: theme.VizRamp1}
				case tt.col >= 0 && y >= 6 && y <= 20:
					want = hue{token: theme.BgSubtle}
				}
				if got := c.at(x, y).ink.on; got != want {
					t.Errorf("row %d of column %d is on %v, want %v", y, x, got, want)
				}
			}
			if got := c.at(x+1, 10).ink.on; got != (hue{}) {
				t.Errorf("the column after now's is on %v, want the canvas", got)
			}
		})
	}
}

// Runway's lanes are those the fit shows of the lanes' accounts the moment
// has, in the lanes' order, each on the row the fit gives it, with its
// account as the moment has it and its name as the moment numbers it, the
// whole name faint where the pool leaves it out; none before the watch
// builds them, nor where nothing fits.
func TestRunwaysLanesAreThoseTheFitShowsOnItsRows(t *testing.T) {
	m := status.Moment{Accounts: []status.AccountNow{
		{ID: "work", Label: "work"}, {ID: "personal", Label: "personal", OutOfPool: true}, {ID: "side", Label: "side"},
	}}
	every := runwayFit{fits: true, lanes: 10, apart: 3, shown: 4}
	f := runwayShowing(runwayLive(views.RunwayFiveHour))
	if lanes := f.laidLanes(m, every); lanes != nil {
		t.Errorf("before the lanes are built, Runway has %d lanes, want none", len(lanes))
	}
	f.Runway.Lanes = &views.Lanes{Accounts: []views.Lane{{ID: "side"}, {ID: "gone"}, {ID: "personal"}, {ID: "work"}}}
	tests := []struct {
		name string
		fit  runwayFit
		want []string
	}{
		{
			name: "every lane shown, three rows apart", fit: every,
			want: []string{"side side 3 side faint false row 10", "personal personal 2 personal faint true row 16", "work work 1 work faint false row 19"},
		},
		{
			name: "scrolled to the third, two shown, two rows apart", fit: runwayFit{fits: true, lanes: 7, apart: 2, first: 2, shown: 2},
			want: []string{"personal personal 2 personal faint true row 7", "work work 1 work faint false row 9"},
		},
		{name: "nothing fits", fit: runwayFit{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, ln := range f.laidLanes(m, tt.fit) {
				got = append(got, fmt.Sprintf("%s %s %s faint %t row %d", ln.ID, ln.account.ID, ln.name.line(framesLanesFrom).plain(), ln.name.faint, ln.row))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Runway's lanes are\n%q, want\n%q", got, tt.want)
			}
		})
	}
}

// Now's column runs down to the pool's lane's last row, or without a pool's
// lane, to the last lane shown's words.
func TestNowsColumnRunsDownToThePoolsLaneOrTheLastLanesShown(t *testing.T) {
	tests := []struct {
		name string
		fit  runwayFit
		want int
	}{
		{name: "the pool's lane's last row", fit: runwayFit{fits: true, lanes: 10, apart: 3, shown: 3, pool: 19}, want: 20},
		{name: "one account, no pool's lane", fit: runwayFit{fits: true, lanes: 10, apart: 3, shown: 1, pool: noRow}, want: 11},
		{name: "scrolled, the last lane shown's", fit: runwayFit{fits: true, lanes: 7, apart: 2, first: 3, shown: 4, pool: noRow}, want: 14},
	}
	for _, tt := range tests {
		if got := tt.fit.nowTo(); got != tt.want {
			t.Errorf("%s: now's column runs down to row %d, want %d", tt.name, got, tt.want)
		}
	}
}
