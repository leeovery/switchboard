package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// poolShown is Runway's live 5-hour view at routedAt, at 160 columns, its
// lanes those of doc's accounts, its pool's lane as given.
func poolShown(doc status.Document, pool views.Pool) RunwayShown {
	lanes := &views.Lanes{Pool: pool}
	for _, a := range doc.Accounts {
		lanes.Accounts = append(lanes.Accounts, views.Lane{ID: a.ID})
	}
	return RunwayShown{Window: views.RunwayFiveHour, Timeline: views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayShown{}.Columns(160)), Lanes: lanes}
}

// poolDrawn is the canvas of Runway shown as r of doc at routedAt, 160 by
// 37, in the look given, alone, and the row its pool's lane starts on.
func poolDrawn(r RunwayShown, doc status.Document, look Look) (*canvas, int) {
	f := Frame{Width: 160, Height: 37, Look: look, View: Runway, Policy: claudeLike, Runway: r}
	return runwayDrawn(f, doc), f.runwayFitted(f.top()).pool
}

// framesDoc is the router's document of the auto world the frames were drawn
// of at routedAt, as routedWorld has it, work's reserve 5%: work at its cap
// till 17:10, its 5-hour at 95%, its week at 34%; personal's at 39% and
// 66%; and side's, where new sessions go, at 13% and 78%.
func framesDoc() status.Document {
	doc, _ := routedWorld(false)
	doc.Accounts[0].Reserve = 0.05
	return doc.WorkedOut(claudeLike)
}

// The pool's columns are each as high as the share of the pool's accounts
// with room, of its two rows, in eighths of a cell, rounded half to even:
// full cells a background, the top cell part-filled, as GRAPHS fills its
// columns; coloured as usage colours the pool's use, the share without
// room, its ramp's first stop under 70%, its second from 70%, its last from
// 90%, and with any room, ▃ at the least; ▂ in state.destructive on its
// lower row where none of its accounts has room, a limit's ink, a
// quarter-cell along its floor; and blank where none of their states is
// known.
func TestThePoolsColumnsAreAsHighAsTheShareOfItsAccountsWithRoom(t *testing.T) {
	full := func(tok theme.Token) seenCell { return seenCell{" ", ink{on: hue{token: tok}}} }
	part := func(glyph string, tok theme.Token) seenCell { return seenCell{glyph, ink{token: tok}} }
	tests := []struct {
		name        string
		col         views.PoolColumn
		of          int
		top, bottom seenCell
	}{
		{name: "all three", col: views.PoolColumn{With: 3, Known: true}, of: 3, top: full(theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "two of three", col: views.PoolColumn{With: 2, Known: true}, of: 3, top: part("▃", theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "one of three, two thirds without", col: views.PoolColumn{With: 1, Known: true}, of: 3, top: blankCell, bottom: part("▅", theme.VizRamp1)},
		{name: "nine of ten", col: views.PoolColumn{With: 9, Known: true}, of: 10, top: part("▆", theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "eight of ten", col: views.PoolColumn{With: 8, Known: true}, of: 10, top: part("▅", theme.VizRamp1), bottom: full(theme.VizRamp1)},
		{name: "three of ten, 70% without", col: views.PoolColumn{With: 3, Known: true}, of: 10, top: blankCell, bottom: part("▅", theme.VizRamp2)},
		{name: "one of ten, 90% without", col: views.PoolColumn{With: 1, Known: true}, of: 10, top: blankCell, bottom: part("▃", theme.VizRamp4)},
		{name: "none of three", col: views.PoolColumn{Known: true}, of: 3, top: blankCell, bottom: seenCell{"▂", errorInk}},
		{name: "none known", col: views.PoolColumn{}, of: 3, top: blankCell, bottom: blankCell},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := slices.Repeat([]views.PoolColumn{tt.col}, RunwayShown{}.Columns(160))
			c, top := poolDrawn(poolShown(runwayDoc(tt.of), views.Pool{Of: tt.of, Key: "5h", Columns: columns}), runwayDoc(tt.of), Screen(builtin(t, "nord")))
			const x = 100
			if got := cellAt(c, x, top); got != tt.top {
				t.Errorf("the column's top cell is %+v, want %+v", got, tt.top)
			}
			if got := cellAt(c, x, top+1); got != tt.bottom {
				t.Errorf("the column's bottom cell is %+v, want %+v", got, tt.bottom)
			}
		})
	}
}

// seenCell is a canvas cell's glyph and ink, its mark aside.
type seenCell struct {
	glyph string
	ink   ink
}

// blankCell is a cell with nothing drawn in it.
var blankCell = seenCell{" ", ink{}}

// cellAt is the canvas's cell at x along row y, as seenCell has it.
func cellAt(c *canvas, x, y int) seenCell {
	here := c.at(x, y)
	return seenCell{here.glyph, here.ink}
}

// The pool's columns before now's are dimmed, mix(canvas, colour, 0.45), as
// the past is: now's and those after it aren't, and on a day stepped back
// to, which has no now, none is.
func TestThePoolsPastIsDimmed(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{With: 2, Known: true}}, RunwayShown{}.Columns(160))
	live := poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: columns})
	gone := live
	gone.Back = civilDay(9, 30)
	gone.Timeline = gone.TimelineAt(routedAt, time.Monday, 160)
	now := live.Timeline.NowColumn()
	tests := []struct {
		name   string
		shown  RunwayShown
		column int
		fade   float64
	}{
		{name: "the column before now's", shown: live, column: now - 1, fade: pastFade},
		{name: "the first", shown: live, column: 0, fade: pastFade},
		{name: "now's", shown: live, column: now},
		{name: "after now's", shown: live, column: now + 1},
		{name: "a day stepped back to's first", shown: gone, column: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, top := poolDrawn(tt.shown, doc, Screen(builtin(t, "nord")))
			x := RunwayShown{}.lanesFrom(160) + tt.column
			if got := c.at(x, top).ink.fade; got != tt.fade {
				t.Errorf("column %d's top cell fades %v, want %v", tt.column, got, tt.fade)
			}
			if got := c.at(x, top+1).ink.on.fade; got != tt.fade {
				t.Errorf("column %d's full cell fades %v, want %v", tt.column, got, tt.fade)
			}
		})
	}
	if got, want := mixed(t, pastFade, theme.VizRamp1), "#637262"; got != want {
		t.Errorf("the past's green is %s, want %s, as the frames dim it", got, want)
	}
}

// ▂ on the lane's floor means the pool at its limit alone: a column of the
// pool's with any of its accounts with room, in any share, down to 1 of 40,
// fills ▃ at the least on its lower row, or the whole cell. On its upper
// row, ▂ stands on a full cell, so it can't be read as the limit.
func TestAPoolsColumnWithRoomFillsThreeEighthsAtTheLeast(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	pools := []int{40}
	for of := 2; of <= 24; of++ {
		pools = append(pools, of)
	}
	for _, of := range pools {
		doc := runwayDoc(of)
		columns := make([]views.PoolColumn, RunwayShown{}.Columns(160))
		for i := range columns {
			columns[i] = views.PoolColumn{With: 1 + i%of, Known: true}
		}
		c, top := poolDrawn(poolShown(doc, views.Pool{Of: of, Key: "5h", Columns: columns}), doc, look)
		x := RunwayShown{}.lanesFrom(160)
		for i, col := range columns {
			if lower := cellAt(c, x+i, top+1); !atLeastThreeEighths(lower) {
				t.Errorf("%d of %d with room, the column's lower row is %+v, want ▃ or fuller", col.With, of, lower)
			}
		}
	}
}

// atLeastThreeEighths reports whether the cell is filled three eighths of
// the way up or more: ▃ to ▇, or a full cell, its ground.
func atLeastThreeEighths(cell seenCell) bool {
	return slices.Contains([]string{"▃", "▄", "▅", "▆", "▇"}, cell.glyph) || cell.glyph == " " && cell.ink.on != (hue{})
}

// A column of the pool's where none of its accounts has room is dimmed
// before now's column as the past is, and where the look has no colour, its
// ▂ is bold, as a lane's held back from every request is.
func TestThePoolsColumnsWithNoRoomAreDrawnAsALanesHeldBack(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{Known: true}}, RunwayShown{}.Columns(160))
	shown := poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: columns})
	now := shown.Timeline.NowColumn()
	tests := []struct {
		name   string
		look   func(t *testing.T) Look
		column int
		want   seenCell
	}{
		{name: "before now's column", look: func(t *testing.T) Look { return Screen(builtin(t, "nord")) }, column: now - 1, want: seenCell{"▂", ink{token: errorInk.token, fade: pastFade}}},
		{name: "after now's column", look: func(t *testing.T) Look { return Screen(builtin(t, "nord")) }, column: now + 1, want: seenCell{"▂", errorInk}},
		{name: "without colour", look: func(*testing.T) Look { return NoColour() }, column: now + 1, want: seenCell{"▂", exhaustedInk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, top := poolDrawn(shown, doc, tt.look(t))
			x := RunwayShown{}.lanesFrom(160) + tt.column
			if got := cellAt(c, x, top+1); got != tt.want {
				t.Errorf("column %d's lower cell is %+v, want %+v", tt.column, got, tt.want)
			}
			if got := cellAt(c, x, top); got != blankCell {
				t.Errorf("column %d's upper cell is %+v, want it blank", tt.column, got)
			}
		})
	}
}

// mixed is the colour of the token given faded so far into nord's canvas.
func mixed(t *testing.T, fade float64, tok theme.Token) string {
	t.Helper()
	nord := Screen(builtin(t, "nord"))
	return shadeOf(nord.colour(ink{token: tok, fade: fade})).String()
}

// Where the look blends nothing, as NO_COLOR's, a full cell of the pool's
// columns is █, as GRAPHS draws one.
func TestThePoolsFullCellsAreBlocksWhereTheLookBlendsNothing(t *testing.T) {
	doc := runwayDoc(3)
	columns := slices.Repeat([]views.PoolColumn{{With: 2, Known: true}}, RunwayShown{}.Columns(160))
	c, top := poolDrawn(poolShown(doc, views.Pool{Of: 3, Key: "5h", Columns: columns}), doc, NoColour())
	if got := string([]rune(rowOf(c, top+1))[RunwayShown{}.lanesFrom(160):]); got != strings.Repeat("█", RunwayShown{}.Columns(160)) {
		t.Errorf("the pool's bottom row reads %q, want █ in every column", got)
	}
	if got := string([]rune(rowOf(c, top))[RunwayShown{}.lanesFrom(160):]); got != strings.Repeat("▃", RunwayShown{}.Columns(160)) {
		t.Errorf("the pool's top row reads %q, want its part-filled cells", got)
	}
}

// Σ pool heads the pool's lane on its first row, as an account's name heads
// its lane, Σ in text.subtle in the accounts' numbers' column, the ones', and
// pool in text.secondary; and no rule runs over it, the row before it the
// last lane's blank row, with three rows an account, or its words, with two.
func TestSigmaPoolHeadsThePoolsLaneWithNoRuleOverIt(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		pool     int
		over     string
		sigma    string
	}{
		{name: "three, after the last lane's blank row", accounts: 3, pool: 19, sigma: "Σ pool"},
		{name: "ten, after the last lane's words, numbered from ten", accounts: 10, pool: 30, over: "  5-hour", sigma: " Σ pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runwayDoc(tt.accounts)
			c, top := poolDrawn(poolShown(doc, views.Pool{Of: tt.accounts, Key: "5h"}), doc, Screen(builtin(t, "nord")))
			if top != tt.pool {
				t.Errorf("the pool's lane is on row %d, want %d", top, tt.pool)
			}
			if got := rowOf(c, top-1); !strings.HasPrefix(got, tt.over) || strings.Contains(got, "─") {
				t.Errorf("the row over the pool's lane reads %q, want %q and no rule", got, tt.over)
			}
			if got := rowOf(c, top); got != tt.sigma {
				t.Errorf("the pool's first row reads %q, want %q", got, tt.sigma)
			}
			sigma := strings.Index(tt.sigma, "Σ")
			if k, pool := c.at(sigma, top).ink, c.at(sigma+2, top).ink; k != dimInk || pool != noteInk {
				t.Errorf("Σ is in %+v and pool in %+v, want text.subtle and text.secondary", k, pool)
			}
		})
	}
}

// With one account, there's no pool's lane: no columns, no Σ pool and no
// room, as they'd only repeat the account.
func TestOneAccountHasNoPoolsLane(t *testing.T) {
	doc := runwayDoc(1)
	c, _ := poolDrawn(poolShown(doc, views.Pool{}), doc, Screen(builtin(t, "nord")))
	for y, row := range textOf(c.rows(Look{})) {
		if strings.Contains(row, "Σ") || strings.Contains(row, " left") {
			t.Errorf("row %d reads %q, want nothing of a pool", y, row)
		}
	}
}

// Lanes of a pool of three drawn for a moment of no accounts draw no pool's
// lane, as they draw no account's: there's no account to name it under.
func TestThePoolsLaneIsntDrawnForAMomentOfNoAccounts(t *testing.T) {
	c := runwayDrawn(runwayFrame(t, 37, lanesOf(3), nil), status.Document{})
	for y, row := range textOf(c.rows(Look{})) {
		if strings.Contains(row, "Σ") || strings.Contains(row, " left") {
			t.Errorf("row %d reads %q, want nothing of a pool", y, row)
		}
	}
}

// With the account column collapsed, the pool's lane is headed Σ alone, in
// text.subtle, and on the row of its own under the lane, its room short,
// the room the expanded lane words, of how many accounts the pool is: the
// room bold text.secondary, brightening as it changes, and "/3" in
// text.muted, at every width; and where it has no room to say, as on a day
// stepped back to, nothing under it.
func TestCollapsedThePoolsRoomIsSaidShortUnderItsLane(t *testing.T) {
	doc, pinned := framesDoc(), framesDoc()
	pinned.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	pinned = pinned.WorkedOut(claudeLike)
	collapsed := func(r RunwayShown, width int) RunwayShown {
		r.Collapsed = true
		r.Timeline = r.TimelineAt(routedAt, time.Monday, width)
		return r
	}
	live := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
	week := live
	week.Window, week.Lanes = views.RunwayWeek, &views.Lanes{Accounts: live.Lanes.Accounts, Pool: views.Pool{Of: 3, Key: "7d"}}
	gone := live
	gone.Back = civilDay(9, 30)
	tests := []struct {
		name  string
		doc   status.Document
		shown RunwayShown
		width int
		room  string
	}{
		{name: "the 5-hour", doc: doc, shown: collapsed(live, 160), width: 160, room: "1.5/3"},
		{name: "the week", doc: doc, shown: collapsed(week, 160), width: 160, room: "1.2/3"},
		{name: "pinned to two", doc: pinned, shown: collapsed(live, 160), width: 160, room: "1.5/2"},
		{name: "on a phone", doc: doc, shown: collapsed(live, 52), width: 52, room: "1.5/3"},
		{name: "a day stepped back to", doc: doc, shown: collapsed(gone, 160), width: 160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			c := runwayDrawn(f, tt.doc)
			fit := f.runwayFitted(f.top())
			x := f.Runway.lanesFrom(f.Width)
			if got := textAt(c, 0, fit.pool, x); got != "Σ" || c.at(0, fit.pool).ink != dimInk {
				t.Errorf("the pool's lane is headed %q in %+v, want Σ in text.subtle", got, c.at(0, fit.pool).ink)
			}
			if got := textAt(c, 0, fit.pool+1, x); got != "" {
				t.Errorf("the lane's second row is headed %q, want nothing", got)
			}
			under := fit.pool + runwayPoolRows
			if got := rowOf(c, under); got != tt.room {
				t.Errorf("the row under the pool's lane reads %q, want %q", got, tt.room)
			}
			if tt.room == "" {
				return
			}
			if fit.room != under {
				t.Errorf("the pool's room is laid on row %d, want the row under its lane, %d", fit.room, under)
			}
			if k, of := c.at(0, under).ink, c.at(3, under).ink; k.token != theme.TextSecondary || !k.bold || of != mutedInk {
				t.Errorf("the room is in %+v and its accounts in %+v; want bold text.secondary and text.muted", k, of)
			}
			key := tt.shown.Lanes.Pool.Key
			if got, want := f.Numbers(tt.doc, routedAt)[Number{Kind: NumberRoom, Of: key}], tt.room[:3]; got != want {
				t.Errorf("the room brightens as %q, want %q", got, want)
			}
		})
	}
}

// The pool's room, from column 2, is usage's words for the pool in the
// view's window: the window's name in text.subtle, the room bold
// text.secondary, brightening as it changes, and " of 3 left" in
// text.muted, " · pinned" after where a pin holds; drawn while the timeline
// holds now, live and on today stepped back to, never on a day gone. It's
// on a row of its own under the pool's lane, at every width, whole parts
// left off its end that don't fit before the frame's edge; nothing after
// it.
func TestThePoolsRoomIsUsagesWordsForThePoolInTheViewsWindow(t *testing.T) {
	doc, pinned := framesDoc(), framesDoc()
	pinned.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	pinned = pinned.WorkedOut(claudeLike)
	live := poolShown(doc, views.Pool{Of: 3, Key: "5h"})
	week := live
	week.Window, week.Lanes = views.RunwayWeek, &views.Lanes{Accounts: live.Lanes.Accounts, Pool: views.Pool{Of: 3, Key: "7d"}}
	week.Timeline = week.TimelineAt(routedAt, time.Monday, 160)
	today := live
	today.Back = civilDay(10, 1)
	today.Timeline = today.TimelineAt(routedAt, time.Monday, 160)
	gone := live
	gone.Back = civilDay(9, 30)
	gone.Timeline = gone.TimelineAt(routedAt, time.Monday, 160)
	phone := live
	phone.Timeline = views.LiveTimeline(views.RunwayFiveHour, routedAt, RunwayShown{}.Columns(52))
	tests := []struct {
		name       string
		doc        status.Document
		shown      RunwayShown
		width, row int
		want       string
	}{
		{name: "the 5-hour", doc: doc, shown: live, width: 160, row: 21, want: "  5-hour 1.5 of 3 left"},
		{name: "the week", doc: doc, shown: week, width: 160, row: 21, want: "  week 1.2 of 3 left"},
		{name: "pinned to two", doc: pinned, shown: live, width: 160, row: 21, want: "  5-hour 1.5 of 2 left · pinned"},
		{name: "today stepped back to", doc: doc, shown: today, width: 160, row: 21, want: "  5-hour 1.5 of 3 left"},
		{name: "a day stepped back to", doc: doc, shown: gone, width: 160, row: 21, want: ""},
		{name: "on a phone", doc: doc, shown: phone, width: 52, row: 22, want: "  5-hour 1.5 of 3 left"},
		{name: "pinned to two, on a phone", doc: pinned, shown: phone, width: 52, row: 22, want: "  5-hour 1.5 of 2 left · pinned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 37, Look: Screen(builtin(t, "nord")), View: Runway, Policy: claudeLike, Runway: tt.shown}
			c := runwayDrawn(f, tt.doc)
			if got := rowOf(c, tt.row); got != tt.want {
				t.Errorf("the pool's room's row reads %q, want %q", got, tt.want)
			}
			if tt.want == "" {
				return
			}
			name, room, rest := c.at(2, tt.row).ink, c.at(strings.Index(tt.want, "1."), tt.row), c.at(strings.Index(tt.want, " of")+1, tt.row).ink
			if name != dimInk || room.ink != strongInk || rest != mutedInk {
				t.Errorf("the window's name is in %+v, the room in %+v and the rest in %+v; want text.subtle, bold text.secondary and text.muted", name, room.ink, rest)
			}
			key := tt.shown.Lanes.Pool.Key
			if got, want := f.Numbers(tt.doc, routedAt)[Number{Kind: NumberRoom, Of: key}], strings.Fields(tt.want)[1]; got != want {
				t.Errorf("the room brightens as %q, want %q", got, want)
			}
		})
	}
}
